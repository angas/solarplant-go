package task

import (
	"context"
	"log/slog"
	"time"

	"github.com/angas/solarplant-go/calc"
	"github.com/angas/solarplant-go/config"
	"github.com/angas/solarplant-go/database"
	"github.com/angas/solarplant-go/ferroamp"
	"github.com/angas/solarplant-go/hours"
)

func NewHourlyTask(
	logger *slog.Logger,
	db *database.Database,
	cnfg config.AppConfigEnergyPrice,
	faInMem *ferroamp.FaInMemData,
	recentHours *database.RecentHours) func() {

	return func() {
		logger.Debug("running hourly task...")

		// This task runs every hour, so we need to subtract one hour
		// to get the correct time for the snapshot. This is because
		// the snapshot contains accumulated values for the last hour and
		// this new hour has not yet been accumulated.
		currHour := hours.FromNow().Sub(1)

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if faInMem.Healthy() {
			if err := db.SaveFaSnapshot(ctx, database.FaSnapshotRow{
				When: currHour,
				Data: *faInMem.CurrentState(),
			}); err != nil {
				logger.Error("hourly task error, saving snapshot", slog.Any("error", err))
			}
		} else {
			logger.Warn("ferroamp data is not healthy, skipping snapshot")
			return
		}

		prevHour, ok := recentHours.Get(currHour.Sub(1))
		if !ok {
			logger.Info("don't save time series, no snapshot from previous hour...")
			return
		}

		fc, err := db.GetWeatherForecast(ctx, currHour)
		if err != nil {
			logger.Error("hourly task error, getting weather forecast", slog.Any("error", err))
			fc = database.WeatherForecastRow{}
		}

		ef, err := db.GetEnergyForecast(ctx, currHour)
		if err != nil {
			logger.Error("hourly task error, getting energy forecast", slog.Any("error", err))
			ef = database.EnergyForecastRow{}
		}

		ep, err := db.GetEnergyPrice(ctx, currHour)
		if err != nil {
			logger.Error("hourly task error, getting energy price", slog.Any("error", err))
			ep = database.EnergyPriceRow{}
		}

		planning, err := db.GetPlanning(ctx, currHour)
		if err != nil {
			logger.Error("hourly task error, getting planning", slog.Any("error", err))
			planning = database.PlanningRow{}
		}

		gridImport := faInMem.ImportedSince(prevHour.Fa.Data)
		gridExport := faInMem.ExportedSince(prevHour.Fa.Data)

		err = db.SaveTimeSeries(ctx, database.TimeSeriesRow{
			When:                 currHour,
			CloudCover:           fc.CloudCover,
			Temperature:          fc.Temperature,
			Precipitation:        fc.Precipitation,
			EnergyPrice:          ep.Price,
			Production:           faInMem.ProducedSince(prevHour.Fa.Data),
			ProductionEstimated:  ef.Production,
			ProductionLifetime:   faInMem.ProductionLifetime(),
			Consumption:          faInMem.ConsumedSince(prevHour.Fa.Data),
			ConsumptionEstimated: ef.Consumption,
			GridImport:           gridImport,
			GridExport:           gridExport,
			BatteryLevel:         faInMem.BatteryLevel(),
			BatteryNetLoad:       faInMem.BatteryNetLoadSince(prevHour.Fa.Data),
			CashFlow:             calc.CashFlow(gridImport, gridExport, ep.Price, cnfg.Tax, cnfg.TaxReduction, cnfg.GridBenefit),
			Strategy:             planning.Strategy,
		})
		if err != nil {
			logger.Error("hourly task error, saving time series", slog.Any("error", err))
		}

		if err = recentHours.Reload(ctx); err != nil {
			logger.Error("hourly task error, reload recent hours", slog.Any("error", err))
		}

		logger.Info("hourly task done")
	}
}
