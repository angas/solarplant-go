package task

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"slices"
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
		instructions := make(chan BatteryInstruction, 4)
		br := NewBatteryRegulator(slog.New(slog.NewTextHandler(io.Discard, nil)), db,
			config.AppConfigBatterySpec{}, ferroamp.NewFaInMemData(),
			BatteryRegulatorStrategy{Interval: 10 * time.Second, UpdateThreshold: 0.1},
			func(_ context.Context, bi BatteryInstruction) error { instructions <- bi; return nil })
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
			case got := <-instructions:
				t.Fatalf("unexpected instruction: %v", got)
			default:
			}
		}
		wantInstruction := func(want BatteryInstruction) {
			t.Helper()
			select {
			case got := <-instructions:
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

func TestBatteryRegulatorPower(t *testing.T) {
	spec := config.AppConfigBatterySpec{MinLevel: 10, MaxLevel: 100, MaxChargeRate: 4, MaxDischargeRate: 3.5}
	tests := []struct {
		name     string
		strategy optimize.Strategy
		soc      float64
		gridKW   []float64 // Grid power at each update
		want     []BatteryInstruction
	}{
		{"charge at max rate", optimize.StrategyCharge, 50, []float64{2, 2}, []BatteryInstruction{{ActionCharge, 4}}},
		{"charge limited by grid", optimize.StrategyCharge, 50, []float64{22}, []BatteryInstruction{{ActionCharge, 2}}},
		{"stop charging when full only once", optimize.StrategyCharge, 100, []float64{2, 2, 2}, []BatteryInstruction{{ActionCharge, 0}}},
		{"discharge at max rate", optimize.StrategyDischarge, 50, []float64{2, 2}, []BatteryInstruction{{ActionDischarge, 3.5}}},
		{"discharge never exceeds max rate", optimize.StrategyDischarge, 50, []float64{10, 0}, []BatteryInstruction{{ActionDischarge, 3.5}}},
		{"stop discharging when empty only once", optimize.StrategyDischarge, 10, []float64{2, 2, 2}, []BatteryInstruction{{ActionDischarge, 0}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := newRegulatorTestDatabase(t)
			if err := db.SavePanning(t.Context(), database.PlanningRow{StartAt: timex.UTC15Min(), Strategy: tt.strategy.String()}); err != nil {
				t.Fatal(err)
			}
			faData := ferroamp.NewFaInMemData()
			faData.SetEsm(&ferroamp.EsmMessage{Soc: ferroamp.FltObj{Value: tt.soc}})
			var got []BatteryInstruction
			br := NewBatteryRegulator(slog.New(slog.NewTextHandler(io.Discard, nil)), db, spec, faData,
				BatteryRegulatorStrategy{UpdateThreshold: 0.1, GridMaxPower: 25},
				func(_ context.Context, bi BatteryInstruction) error { got = append(got, bi); return nil })
			for _, gridKW := range tt.gridKW {
				faData.SetEHub(&ferroamp.EhubMessage{Pext: ferroamp.Phases{L1: gridKW * 1e3}})
				br.adjustLoad(t.Context())
			}
			if !slices.Equal(got, tt.want) {
				t.Fatalf("instructions = %v, want %v", got, tt.want)
			}
		})
	}
}
