package task

import (
	"context"
	"log/slog"
	"time"

	"github.com/angas/solarplant-go/calc"
	"github.com/angas/solarplant-go/config"
	"github.com/angas/solarplant-go/database"
	"github.com/angas/solarplant-go/ferroamp"
	"github.com/angas/solarplant-go/timex"
)

func NewHourlyTask(
	logger *slog.Logger,
	db *database.Database,
	cnfg config.AppConfigEnergyPrice,
	faInMem *ferroamp.FaInMemData,
	recentHours *database.RecentHours) func() {

	return func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		// We need to subtract one hour from the current time
		// to get the correct time for the snapshot. This is because
		// the snapshot contains accumulated values for the last hour and
		// this new hour has not yet been accumulated.
		completedHourEnd := timex.UTCHour().SubHours(1)

		log := logger.With(slog.String("completedHourEnd", completedHourEnd.String()))

		log.DebugContext(ctx, "running hourly task...")

		if faInMem.Healthy() {
			if err := db.SaveFaSnapshot(ctx, database.FaSnapshotRow{
				Timestamp: completedHourEnd,
				Data:      *faInMem.CurrentState(),
			}); err != nil {
				log.ErrorContext(ctx, "hourly task error, saving snapshot", slog.Any("error", err))
			}
		} else {
			log.WarnContext(ctx, "ferroamp data is not healthy, skipping snapshot")
			return
		}

		fc, err := db.GetWeatherForecast(ctx, completedHourEnd)
		if err != nil {
			log.ErrorContext(ctx, "hourly task error, getting weather forecast", slog.Any("error", err))
			fc = database.WeatherForecastRow{}
		}

		ef, err := db.GetEnergyForecast(ctx, completedHourEnd)
		if err != nil {
			log.ErrorContext(ctx, "hourly task error, getting energy forecast", slog.Any("error", err))
			ef = database.EnergyForecastRow{}
		}

		ep, err := db.GetAvgEnergyPriceForHour(ctx, timex.UTCHour())
		if err != nil {
			log.ErrorContext(ctx, "hourly task error, getting average energy price", slog.Any("error", err))
			ep = database.EnergyPriceRow{}
		}

		planning, err := db.GetPlanning(ctx, completedHourEnd)
		if err != nil {
			log.ErrorContext(ctx, "hourly task error, getting planning", slog.Any("error", err))
			planning = database.PlanningRow{}
		}

		priorHourEnd := recentHours.Get(completedHourEnd.SubHours(1))
		if priorHourEnd.Empty() {
			r := recentHours.Range()
			log.WarnContext(ctx, "don't save time series, no snapshot from previous hour",
				slog.String("recentHoursCacheMin", r[0].String()),
				slog.String("recentHoursCacheMax", r[1].String()))
		} else {
			gridImport := faInMem.ImportedSince(priorHourEnd.Fa.Data)
			gridExport := faInMem.ExportedSince(priorHourEnd.Fa.Data)

			err = db.SaveTimeSeries(ctx, database.TimeSeriesRow{
				Timestamp:            completedHourEnd,
				CloudCover:           fc.CloudCover,
				Temperature:          fc.Temperature,
				Precipitation:        fc.Precipitation,
				EnergyPriceAvg:       ep.Price,
				Production:           faInMem.ProducedSince(priorHourEnd.Fa.Data),
				ProductionEstimated:  ef.Production,
				ProductionLifetime:   faInMem.ProductionLifetime(),
				Consumption:          faInMem.ConsumedSince(priorHourEnd.Fa.Data),
				ConsumptionEstimated: ef.Consumption,
				GridImport:           gridImport,
				GridExport:           gridExport,
				BatteryLevel:         faInMem.BatteryLevel(),
				BatteryNetLoad:       faInMem.BatteryNetLoadSince(priorHourEnd.Fa.Data),
				CashFlow:             calc.CashFlow(gridImport, gridExport, ep.Price, cnfg.Tax, cnfg.TaxReduction, cnfg.GridBenefit),
				Strategy:             planning.Strategy,
			})
			if err != nil {
				log.ErrorContext(ctx, "hourly task error, saving time series", slog.Any("error", err))
			} else {
				log.DebugContext(ctx, "timeseries stored")
			}
		}

		if err = recentHours.Reload(ctx); err != nil {
			log.ErrorContext(ctx, "hourly task error, reload recent hours", slog.Any("error", err))
		} else {
			r := recentHours.Range()
			log.DebugContext(ctx, "recent hours cache reloaded",
				slog.String("recentHoursCacheMin", r[0].String()),
				slog.String("recentHoursCacheMax", r[1].String()))
		}

		log.InfoContext(ctx, "hourly task done")
	}
}
