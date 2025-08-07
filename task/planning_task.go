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
	"github.com/angas/solarplant-go/hours"
	"github.com/angas/solarplant-go/optimize"
)

func NewPlanningTask(logger *slog.Logger, db *database.Database, cnfg *config.AppConfig, faInMem *ferroamp.FaInMemData) func() {
	return func() {
		logger.Debug("running planning task...")
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		if !faInMem.Healthy() {
			logger.Warn("ferroamp data is not healthy, skipping planning task")
			return
		}

		startHour := hours.FromNow().Add(1)

		optInput := optimize.Input{
			Battery: optimize.Battery{
				AppConfigBatterySpec: cnfg.BatterySpec,
				CurrentLevel:         faInMem.BatteryLevel(),
			},
			EnergyTax:    cnfg.EnergyPrice.Tax,
			GridMaxPower: cnfg.Planner.GridMaxPower,
			Forecast:     make([]optimize.Forecast, cnfg.Planner.HoursAhead),
		}

		for h := range int(cnfg.Planner.HoursAhead) {
			hour := startHour.Add(h)

			ef, err := db.GetEnergyForecast(ctx, hour)
			if err != nil {
				if err == sql.ErrNoRows {
					logger.Warn("can't plan upcoming hours, no energy forecast found", slog.String("hour", hour.String()))
				} else {
					logger.Error("planning task error, getting energy forecast", slog.String("hour", hour.String()), slog.Any("error", err))
				}
				return
			}

			ep, err := db.GetEnergyPrice(ctx, hour)
			if err != nil {
				if err == sql.ErrNoRows {
					logger.Warn("can't plan upcoming hours, no energy price found", slog.String("hour", hour.String()))
				} else {
					logger.Error("planning task error, getting energy price", slog.String("hour", hour.String()), slog.Any("error", err))
				}
				return
			}

			optInput.Forecast[h] = optimize.Forecast{
				EnergyPrice:   ep.Price,
				EnergyBalance: calc.TwoDecimals(ef.Production - ef.Consumption),
			}
		}

		logger.Debug(fmt.Sprintf("planning for %d hours ahead", cnfg.Planner.HoursAhead),
			slog.String("hour", startHour.String()),
			slog.Float64("battLvl", optInput.Battery.CurrentLevel))

		optOutput := optimize.BestStrategies(optInput)

		if len(optOutput.Strategy) != int(cnfg.Planner.HoursAhead) {
			logger.Error(fmt.Sprintf("planning task error, didn't get strategies for %d hours ahead", cnfg.Planner.HoursAhead))
			return
		}

		for h := range int(cnfg.Planner.HoursAhead) {
			if ctx.Err() != nil {
				logger.Error("planning task timeout/cancelled", slog.Any("error", ctx.Err()))
				return
			}

			dh := startHour.Add(h)
			oi := optInput.Forecast[h]
			ou := optOutput.Strategy[h]
			logger.Debug(fmt.Sprintf("result for hour %s", dh),
				slog.Float64("price", oi.EnergyPrice),
				slog.Float64("balance", oi.EnergyBalance),
				slog.Any("strategy", ou))
			if err := db.SavePanning(ctx, database.PlanningRow{
				When:     dh,
				Strategy: ou.String(),
			}); err != nil {
				logger.Error("planning task error", slog.String("hour", dh.String()), slog.Any("error", err))
			}
		}

		logger.Info("planning task done",
			slog.Int("noOfHoursUpdated", cnfg.Planner.HoursAhead),
			slog.Float64("cost", optOutput.Cost),
			slog.Float64("battLvl", optOutput.BatteryLevel))
	}
}
