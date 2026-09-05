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
	"github.com/angas/solarplant-go/optimize"
	"github.com/angas/solarplant-go/timex"
)

func newRegulatorTestDatabase(t *testing.T) *database.Database {
	t.Helper()
	// New registers process-wide SQLite hooks, which can outlive this test.
	db, err := database.New(context.Background(), filepath.Join(t.TempDir(), "regulator.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	if err := db.Migrate(t.Context()); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestBatteryRegulatorCadence(t *testing.T) {
	db := newRegulatorTestDatabase(t)
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		br := NewBatteryRegulator(slog.New(slog.NewTextHandler(io.Discard, nil)), db,
			config.AppConfigBatterySpec{}, ferroamp.NewFaInMemData(),
			BatteryRegulatorStrategy{Interval: 10 * time.Second, UpdateThreshold: 0.1})
		br.C = make(chan BatteryInstruction, 4)
		done := make(chan struct{})
		go func() {
			defer close(done)
			br.run(ctx)
		}()
		synctest.Wait()
		advance := func(duration time.Duration) {
			time.Sleep(duration)
			synctest.Wait()
		}
		wantQuiet := func() {
			t.Helper()
			select {
			case got := <-br.C:
				t.Fatalf("unexpected instruction: %v", got)
			default:
			}
		}
		wantInstruction := func(want BatteryInstruction) {
			t.Helper()
			select {
			case got := <-br.C:
				if got != want {
					t.Fatalf("instruction = %v, want %v", got, want)
				}
			default:
				t.Fatalf("missing instruction: %v", want)
			}
			wantQuiet()
		}

		advance(59 * time.Second)
		wantQuiet()
		advance(time.Second) // Startup delay is over; the first update waits one interval.
		wantQuiet()
		advance(9 * time.Second)
		wantQuiet()
		advance(time.Second)
		wantInstruction(BatteryInstruction{Action: ActionAuto})
		advance(10 * time.Second)
		wantQuiet() // The same instruction is suppressed.

		if err := db.SavePanning(ctx, database.PlanningRow{StartAt: timex.UTC15Min(), Strategy: optimize.StrategyPreserve.String()}); err != nil {
			t.Fatal(err)
		}
		advance(9 * time.Second)
		wantQuiet()
		advance(time.Second)
		wantInstruction(BatteryInstruction{Action: ActionCharge})

		cancel()
		<-done
		advance(time.Minute)
		wantQuiet()
	})
}
