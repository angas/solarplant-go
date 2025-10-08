package task

import (
	"context"
	"database/sql"
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
			Forecast:     []optimize.Forecast{},
		}

		hour := timex.UTCHour()
		continueTo := hour.AddHours(int64(cnfg.Planner.HoursAhead))
		timeBuckets := []timex.BucketTime{}

		for hour.Before(continueTo) {
			hour = hour.AddHours(1)
			timeBuckets = append(timeBuckets, hour)

			// Get average energy price for the next hour.
			// For now this is enough, in the furure we will break yhis down into 15 minute intervals
			ep, err := db.GetAvgEnergyPriceForHour(ctx, hour)
			if err != nil {
				if err == sql.ErrNoRows {
					logger.WarnContext(ctx, "can't plan upcoming hours, no energy price found", slog.String("startAt", hour.String()))
				} else {
					logger.ErrorContext(ctx, "planning task error, getting energy price", slog.String("startAt", hour.String()), slog.Any("error", err))
				}
				return
			}

			ef, err := db.GetEnergyForecast(ctx, hour)
			if err != nil {
				if err == sql.ErrNoRows {
					logger.WarnContext(ctx, "can't plan upcoming hours, no energy forecast found", slog.String("startAt", hour.String()))
				} else {
					logger.ErrorContext(ctx, "planning task error, getting energy forecast", slog.String("startAt", hour.String()), slog.Any("error", err))
				}
				return
			}

			optInput.Forecast = append(optInput.Forecast, optimize.Forecast{
				EnergyPrice:   ep.Price,
				EnergyBalance: calc.TwoDecimals(ef.Production - ef.Consumption),
			})
		}

		logger.DebugContext(ctx, fmt.Sprintf("planning for %d time buckets ahead", len(timeBuckets)),
			slog.String("startAt", hour.String()),
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
			slog.Int("noOfHoursUpdated", cnfg.Planner.HoursAhead),
			slog.Float64("cost", optOutput.Cost),
			slog.Float64("battLvl", optOutput.BatteryLevel))
	}
}
