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
	"github.com/angas/solarplant-go/hours"
)

type historyAverage struct {
	Production float64
	/** Compensated for battery charging */
	Consumption float64
	CloudCover  float64
	Temperature float64
}

func NewEnergyForecastTask(logger *slog.Logger, db *database.Database, config config.AppConfigEnergyForecast) func() {
	return func() {
		runEnergyForecastTask(logger, db, config)
	}
}

func runEnergyForecastTask(logger *slog.Logger, db *database.Database, cnfg config.AppConfigEnergyForecast) {
	logger.Debug("running energy forecast task...")

	hour := hours.FromNow()
	rows := make([]database.EnergyForecastRow, 0, int(cnfg.HoursAhead))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	for range int(cnfg.HoursAhead) {
		hour = hour.Add(1)

		forecast, err := db.GetWeatherForecast(ctx, hour)
		if err != nil {
			if err == sql.ErrNoRows {
				logger.Warn("energy forecast task problem, forecast not found", "hour", hour.String())
			} else {
				logger.Error("energy forecast task error", slog.Any("error", err))
			}
		}

		avg, err := calcHistoryAverage(ctx, db, cnfg, hour)
		if err != nil {
			logger.Error("energy forecast task error, calculate history average", slog.Any("error", err))
		}

		// Normalize the production based on average cloud cover during the historical hours
		avgProduction := avg.Production + avg.Production*cnfg.CloudCoverImpact*avg.CloudCover/8.0
		// Adjust the estimated production based on the forecasted cloud cover
		estProduction := avgProduction - avgProduction*cnfg.CloudCoverImpact*float64(forecast.CloudCover)/8.0

		row := database.EnergyForecastRow{
			When:        hour,
			Production:  calc.TwoDecimals(estProduction),
			Consumption: calc.TwoDecimals(avg.Consumption),
		}

		rows = append(rows, row)
	}

	if err := db.SaveEnergyForecast(ctx, rows); err != nil {
		logger.Error("energy forecast task error", slog.Any("error", err))
		return
	}

	logger.Info("energy forecast task done", slog.Int("noOfHoursUpdated", len(rows)))
}

func calcHistoryAverage(ctx context.Context, db *database.Database, config config.AppConfigEnergyForecast, hour hours.DateHour) (historyAverage, error) {
	hour = hour.Sub(24 * config.HistoricalDays)

	tsh, err := db.GetTimeSeriesForHour(ctx, hour)
	avg := historyAverage{}
	if err != nil {
		return avg, err
	}

	if (len(tsh)) == 0 {
		return avg, fmt.Errorf("no historical data found for %s", hour)
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
