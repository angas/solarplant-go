package task

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"testing/synctest"
	"time"

	"github.com/angas/solarplant-go/config"
	"github.com/angas/solarplant-go/database"
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
	db, err := database.New(t.Context(), filepath.Join(t.TempDir(), "regulator.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	if err := db.Migrate(t.Context()); err != nil {
		t.Fatal(err)
	}
	for _, deliver := range []bool{false, true} {
		name := "cancel blocked send"
		if deliver {
			name = "deliver instruction"
		}
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				br := NewBatteryRegulator(slog.New(slog.NewTextHandler(io.Discard, nil)), db,
					config.AppConfigBatterySpec{}, ferroamp.NewFaInMemData(), BatteryRegulatorStrategy{})
				done := make(chan struct{})
				go func() {
					defer close(done)
					br.adjustLoad(ctx)
				}()
				synctest.Wait() // No planning exists, so an auto instruction awaits a receiver.
				want := BatteryInstruction{}
				if deliver {
					want = BatteryInstruction{Action: ActionAuto}
					if got := <-br.C; got != want {
						t.Fatalf("instruction = %v, want %v", got, want)
					}
				} else {
					cancel()
				}
				<-done
				if br.lastInstruction != want {
					t.Fatalf("last instruction = %v, want %v", br.lastInstruction, want)
				}
			})
		})
	}
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
