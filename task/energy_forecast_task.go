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
	"github.com/angas/solarplant-go/timex"
)

type historyAverage struct {
	Production  float64
	Consumption float64 // Compensated for battery charging
	CloudCover  float64
	Temperature float64
}

func NewEnergyForecastTask(logger *slog.Logger, db *database.Database, config config.AppConfigEnergyForecast) func() {
	return func() {
		runEnergyForecastTask(context.Background(), logger, db, config)
	}
}

func runEnergyForecastTask(ctx context.Context, logger *slog.Logger, db *database.Database, cnfg config.AppConfigEnergyForecast) {
	logger.Debug("running energy forecast task...")

	slot := timex.UTC15Min()
	var rows []database.EnergyForecastRow

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Iterate by 15-min slot, each slot gets its own historical average
	for range int(cnfg.HoursAhead) * 4 {
		slot = slot.Add15Min(1)

		// Weather forecast is hourly — truncate to hour for lookup
		hourSlot := slot.TruncToHour()
		forecast, err := db.GetWeatherForecast(ctx, hourSlot)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				logger.WarnContext(ctx, "energy forecast task problem, forecast not found", "startAt", hourSlot.String())
			} else {
				logger.ErrorContext(ctx, "energy forecast task error", slog.Any("error", err))
			}
		}

		avg, err := calcHistoryAverage(ctx, logger, db, cnfg, slot)
		if err != nil {
			logger.ErrorContext(ctx, "energy forecast task error, calculate history average", slog.Any("error", err))
		}

		// Normalize the production based on average cloud cover during the historical slots
		avgProduction := avg.Production + avg.Production*cnfg.CloudCoverImpact*avg.CloudCover/8.0
		// Adjust the estimated production based on the forecasted cloud cover
		estProduction := avgProduction - avgProduction*cnfg.CloudCoverImpact*float64(forecast.CloudCover)/8.0

		rows = append(rows, database.EnergyForecastRow{
			StartAt:     slot,
			Production:  calc.TwoDecimals(estProduction),
			Consumption: calc.TwoDecimals(avg.Consumption),
		})
	}

	if err := db.SaveEnergyForecast(ctx, rows); err != nil {
		logger.ErrorContext(ctx, "energy forecast task error", slog.Any("error", err))
		return
	}

	logger.DebugContext(ctx, "energy forecast task done", slog.Int("noOfRowsUpdated", len(rows)))
}

func calcHistoryAverage(ctx context.Context, logger *slog.Logger, db *database.Database, cnfg config.AppConfigEnergyForecast, slot timex.BucketTime) (historyAverage, error) {
	lookback := slot.SubHours(int64(24 * cnfg.HistoricalDays))
	avg := historyAverage{}

	// Try 15-min resolution first
	tsh, err := db.Get15MinSummaryForSlot(ctx, lookback)
	if err != nil {
		return avg, err
	}

	// Fall back to hourly summary divided by 4 during the transition period
	if len(tsh) == 0 {
		hourLookback := slot.TruncToHour().SubHours(int64(24 * cnfg.HistoricalDays))
		hourly, err := db.GetHourlySummaryForHour(ctx, hourLookback)
		if err != nil {
			return avg, err
		}
		if len(hourly) == 0 {
			return avg, fmt.Errorf("no historical data found for slot %s", slot)
		}

		logger.DebugContext(ctx, "using hourly fallback for 15-min slot", "slot", slot.String(), "hourlyRows", len(hourly))

		for _, h := range hourly {
			avg.Production += h.Production
			avg.Consumption += h.Consumption
			avg.Temperature += h.Temperature
			avg.CloudCover += float64(h.CloudCover)
		}

		count := float64(len(hourly))
		avg.Production = avg.Production / count / 4
		avg.Consumption = avg.Consumption / count / 4
		avg.Temperature = avg.Temperature / count
		avg.CloudCover = avg.CloudCover / count

		return avg, nil
	}

	for _, h := range tsh {
		avg.Production += h.Production
		avg.Consumption += h.Consumption
		avg.Temperature += h.Temperature
		avg.CloudCover += float64(h.CloudCover)
	}

	count := float64(len(tsh))
	avg.Production = avg.Production / count
	avg.Consumption = avg.Consumption / count
	avg.Temperature = avg.Temperature / count
	avg.CloudCover = avg.CloudCover / count

	return avg, nil
}
