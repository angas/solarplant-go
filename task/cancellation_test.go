package task

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/angas/solarplant-go/config"
	"github.com/angas/solarplant-go/ferroamp"
)

func TestBatteryRegulatorCancelStartup(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		br := &BatteryRegulator{
			logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
			strategy: BatteryRegulatorStrategy{Interval: 10 * time.Second},
		}
		done := make(chan struct{})
		start := time.Now()
		go func() {
			defer close(done)
			br.run(ctx)
		}()
		synctest.Wait() // The regulator is waiting for its startup timer.
		cancel()
		<-done
		if elapsed := time.Since(start); elapsed != 0 {
			t.Fatalf("cancellation waited for startup: %v", elapsed)
		}
	})
}

func TestBatteryRegulatorInstructionDelivery(t *testing.T) {
	db := newRegulatorTestDatabase(t)
	applyErr := errors.New("apply failed")
	auto := BatteryInstruction{Action: ActionAuto} // No planning exists, so the fallback is auto.

	t.Run("cancelled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		br := NewBatteryRegulator(slog.New(slog.NewTextHandler(io.Discard, nil)), db,
			config.AppConfigBatterySpec{}, ferroamp.NewFaInMemData(), BatteryRegulatorStrategy{},
			func(_ context.Context, bi BatteryInstruction) error {
				t.Errorf("instruction applied after cancellation: %v", bi)
				return nil
			})
		br.adjustLoad(ctx)
		if br.lastInstruction != (BatteryInstruction{}) {
			t.Fatalf("last instruction = %v, want none", br.lastInstruction)
		}
	})

	t.Run("retry failed instruction", func(t *testing.T) {
		var logs bytes.Buffer
		var applied []BatteryInstruction
		var result error
		br := NewBatteryRegulator(slog.New(slog.NewTextHandler(&logs, nil)), db,
			config.AppConfigBatterySpec{}, ferroamp.NewFaInMemData(), BatteryRegulatorStrategy{UpdateThreshold: 0.1},
			func(_ context.Context, bi BatteryInstruction) error {
				applied = append(applied, bi)
				return result
			})
		steps := []struct {
			name        string
			result      error
			resend      bool
			wantApplied int
			wantLast    BatteryInstruction
		}{
			{"first attempt fails", applyErr, false, 1, BatteryInstruction{}},
			{"failed instruction is sent again", applyErr, false, 2, BatteryInstruction{}},
			{"retry succeeds", nil, false, 3, auto},
			{"applied instruction is suppressed", nil, false, 3, auto},
			{"resend after a reconnect fails", applyErr, true, 4, BatteryInstruction{}},
			{"resend is retried", nil, false, 5, auto},
			{"resent instruction is suppressed", nil, false, 5, auto},
		}
		for _, step := range steps {
			result = step.result
			if step.resend {
				br.Resend()
			}
			br.adjustLoad(t.Context())
			if len(applied) != step.wantApplied {
				t.Fatalf("%s: applied %d instructions, want %d", step.name, len(applied), step.wantApplied)
			}
			if br.lastInstruction != step.wantLast {
				t.Fatalf("%s: last instruction = %v, want %v", step.name, br.lastInstruction, step.wantLast)
			}
		}
		for _, got := range applied {
			if got != auto {
				t.Fatalf("instruction = %v, want %v", got, auto)
			}
		}
		if got := strings.Count(logs.String(), "failed to apply battery instruction"); got != 3 {
			t.Fatalf("logged %d failures, want 3: %s", got, logs.String())
		}
		if got := strings.Count(logs.String(), "sending it again"); got != 1 {
			t.Fatalf("logged %d resends, want 1: %s", got, logs.String())
		}
	})
}

func TestEnergyForecastHonorsParentCancellation(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	for _, expired := range []bool{false, true} {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		if expired {
			ctx, cancel = context.WithDeadline(t.Context(), time.Now().Add(-time.Second))
			defer cancel()
		}
		// A cancelled parent must stop the task before it touches the database.
		runEnergyForecastTask(ctx, logger, nil, config.AppConfigEnergyForecast{HoursAhead: 1})
	}
}
