package task

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/angas/solarplant-go/calc"
	"github.com/angas/solarplant-go/config"
	"github.com/angas/solarplant-go/database"
	"github.com/angas/solarplant-go/ferroamp"
	"github.com/angas/solarplant-go/optimize"
	"github.com/angas/solarplant-go/timex"
)

func NewPlanningTask(logger *slog.Logger, db *database.Database, cnfg *config.AppConfig, faInMem *ferroamp.FaInMemData) func() {
	return func() {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		logger.DebugContext(context.Background(), "running planning task...")

		if !faInMem.Healthy() {
			logger.WarnContext(ctx, "ferroamp data is not healthy, skipping planning task")
			return
		}

		optInput := optimize.Input{
			Battery: optimize.Battery{
				AppConfigBatterySpec: cnfg.BatterySpec,
				CurrentLevel:         faInMem.BatteryLevel(),
			},
			EnergyTax:    cnfg.EnergyPrice.Tax,
			GridMaxPower: cnfg.Planner.GridMaxPower,
			SlotDuration: 15 * time.Minute,
			Forecast:     []optimize.Forecast{},
		}

		slot := timex.UTC15Min()
		totalSlots := cnfg.Planner.HoursAhead * 4
		continueTo := slot.Add15Min(int64(totalSlots))
		timeBuckets := []timex.BucketTime{}

		for slot.Before(continueTo) {
			slot = slot.Add15Min(1)

			ep, err := db.GetEnergyPrice(ctx, slot)
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					logger.WarnContext(ctx, "no energy price found, planning with available data", slog.String("startAt", slot.String()))
				} else {
					logger.ErrorContext(ctx, "planning task error, getting energy price", slog.String("startAt", slot.String()), slog.Any("error", err))
				}
				break
			}

			ef, err := db.GetEnergyForecast(ctx, slot)
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					logger.WarnContext(ctx, "no energy forecast found, planning with available data", slog.String("startAt", slot.String()))
				} else {
					logger.ErrorContext(ctx, "planning task error, getting energy forecast", slog.String("startAt", slot.String()), slog.Any("error", err))
				}
				break
			}

			timeBuckets = append(timeBuckets, slot)
			optInput.Forecast = append(optInput.Forecast, optimize.Forecast{
				EnergyPrice:   ep.Price,
				EnergyBalance: calc.TwoDecimals(ef.Production - ef.Consumption),
			})
		}

		if len(timeBuckets) == 0 {
			logger.WarnContext(ctx, "no slots with complete data, skipping planning")
			return
		}

		logger.DebugContext(ctx, fmt.Sprintf("planning for %d time buckets ahead", len(timeBuckets)),
			slog.String("startAt", slot.String()),
			slog.Int("noOfTimeBuckets", len(timeBuckets)),
			slog.Float64("battLvl", optInput.Battery.CurrentLevel))

		optOutput := optimize.BestStrategies(optInput)

		if len(optOutput.Strategy) != len(timeBuckets) {
			logger.ErrorContext(ctx, fmt.Sprintf("planning task error, didn't get strategies for %d time buckets ahead", len(timeBuckets)))
			return
		}

		for i, tb := range timeBuckets {
			if ctx.Err() != nil {
				logger.ErrorContext(ctx, "planning task timeout/cancelled", slog.Any("error", ctx.Err()))
				return
			}

			oi := optInput.Forecast[i]
			ou := optOutput.Strategy[i]
			logger.DebugContext(ctx, fmt.Sprintf("result for period %s", tb.String()),
				slog.Float64("price", oi.EnergyPrice),
				slog.Float64("balance", oi.EnergyBalance),
				slog.Any("strategy", ou))
			if err := db.SavePanning(ctx, database.PlanningRow{
				StartAt:  tb,
				Strategy: ou.String(),
			}); err != nil {
				logger.ErrorContext(ctx, "planning task error", slog.String("startAt", tb.String()), slog.Any("error", err))
			}
		}

		logger.InfoContext(ctx, "planning task done",
			slog.Int("noOfSlotsUpdated", len(timeBuckets)),
			slog.Float64("cost", optOutput.Cost),
			slog.Float64("battLvl", optOutput.BatteryLevel))
	}
}
