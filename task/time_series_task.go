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

func NewTimeSeriesTask(
	logger *slog.Logger,
	db *database.Database,
	cnfg config.AppConfigEnergyPrice,
	faInMem *ferroamp.FaInMemData,
	recentHours *database.RecentHours) func() {

	return func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		// The completed slot is the previous 15-min period.
		// The snapshot contains accumulated values for that slot,
		// and this new slot has not yet been accumulated.
		completedSlot := timex.UTC15Min().Add15Min(-1)

		log := logger.With(slog.String("completedSlot", completedSlot.String()))

		log.DebugContext(ctx, "running time series task...")

		if faInMem.Healthy() {
			if err := db.SaveFaSnapshot(ctx, database.FaSnapshotRow{
				Timestamp: completedSlot,
				Data:      *faInMem.CurrentState(),
			}); err != nil {
				log.ErrorContext(ctx, "time series task error, saving snapshot", slog.Any("error", err))
			}
		} else {
			log.WarnContext(ctx, "ferroamp data is not healthy, skipping snapshot")
			return
		}

		// Weather forecast stays hourly
		fc, err := db.GetWeatherForecast(ctx, completedSlot.TruncToHour())
		if err != nil {
			log.ErrorContext(ctx, "time series task error, getting weather forecast", slog.Any("error", err))
			fc = database.WeatherForecastRow{}
		}

		ef, err := db.GetEnergyForecast(ctx, completedSlot)
		if err != nil {
			log.ErrorContext(ctx, "time series task error, getting energy forecast", slog.Any("error", err))
			ef = database.EnergyForecastRow{}
		}

		ep, err := db.GetEnergyPrice(ctx, completedSlot)
		if err != nil {
			log.ErrorContext(ctx, "time series task error, getting energy price", slog.Any("error", err))
			ep = database.EnergyPriceRow{}
		}

		planning, err := db.GetPlanning(ctx, completedSlot)
		if err != nil {
			log.ErrorContext(ctx, "time series task error, getting planning", slog.Any("error", err))
			planning = database.PlanningRow{}
		}

		priorSlot := recentHours.Get(completedSlot.Add15Min(-1))
		if priorSlot.Empty() {
			r := recentHours.Range()
			log.WarnContext(ctx, "don't save time series, no snapshot from previous slot",
				slog.String("recentHoursCacheMin", r[0].String()),
				slog.String("recentHoursCacheMax", r[1].String()))
		} else {
			gridImport := faInMem.ImportedSince(priorSlot.Fa.Data)
			gridExport := faInMem.ExportedSince(priorSlot.Fa.Data)

			err = db.SaveTimeSeries(ctx, database.TimeSeriesRow{
				Timestamp:            completedSlot,
				CloudCover:           fc.CloudCover,
				Temperature:          fc.Temperature,
				Precipitation:        fc.Precipitation,
				EnergyPriceAvg:       ep.Price,
				Production:           faInMem.ProducedSince(priorSlot.Fa.Data),
				ProductionEstimated:  ef.Production,
				ProductionLifetime:   faInMem.ProductionLifetime(),
				Consumption:          faInMem.ConsumedSince(priorSlot.Fa.Data),
				ConsumptionEstimated: ef.Consumption,
				GridImport:           gridImport,
				GridExport:           gridExport,
				BatteryLevel:         faInMem.BatteryLevel(),
				BatteryNetLoad:       faInMem.BatteryNetLoadSince(priorSlot.Fa.Data),
				CashFlow:             calc.CashFlow(gridImport, gridExport, ep.Price, cnfg.Tax, cnfg.TaxReduction, cnfg.GridBenefit),
				Strategy:             planning.Strategy,
			})
			if err != nil {
				log.ErrorContext(ctx, "time series task error, saving time series", slog.Any("error", err))
			} else {
				log.DebugContext(ctx, "timeseries stored")
			}
		}

		if err = recentHours.Reload(ctx); err != nil {
			log.ErrorContext(ctx, "time series task error, reload recent hours", slog.Any("error", err))
		} else {
			r := recentHours.Range()
			log.DebugContext(ctx, "recent hours cache reloaded",
				slog.String("recentHoursCacheMin", r[0].String()),
				slog.String("recentHoursCacheMax", r[1].String()))
		}

		log.InfoContext(ctx, "time series task done")
	}
}
