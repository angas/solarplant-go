package task

import (
	"context"
	"log/slog"
	"time"

	"github.com/angas/solarplant-go/config"
	"github.com/angas/solarplant-go/database"
	"github.com/angas/solarplant-go/smhi"
	"github.com/angas/solarplant-go/timex"
)

func NewWeatherForecastTask(logger *slog.Logger, db *database.Database, config config.AppConfigWeatherForecast) func() {
	ctx := context.Background()
	if needImmediateForecastUpdate(ctx, db) {
		logger.Info("need an immediate update of weather forecast")
		runForecastTask(ctx, logger, db, config)
	} else {
		logger.Debug("no need for immediate update of weather forecast")
	}

	return func() {
		runForecastTask(ctx, logger, db, config)
	}
}

func runForecastTask(ctx context.Context, logger *slog.Logger, db *database.Database, config config.AppConfigWeatherForecast) {
	logger.DebugContext(ctx, "running weather forecast task...")

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	forecast, err := smhi.Get(ctx, config.Longitude, config.Latitude)
	if err != nil {
		logger.ErrorContext(ctx, "smhi weather forecast task error", slog.Any("error", err))
	} else {
		rows := []database.WeatherForecastRow{}
		for _, f := range forecast {
			startAt := timex.BucketTime(f.Hour)
			if !startAt.IsValid(timex.BucketSizeHour) {
				logger.WarnContext(ctx, "invalid smhi forecast start_at", slog.Any("startAt", f.Hour))
				continue
			}
			rows = append(rows, database.WeatherForecastRow{
				StartAt:       startAt,
				CloudCover:    f.CloudCover,
				Temperature:   f.Temperature,
				Precipitation: f.Precipitation,
			})
		}
		if err = db.SaveForecast(ctx, rows); err != nil {
			logger.ErrorContext(ctx, "weather forecast task error", slog.Any("error", err))
		}
		logger.InfoContext(ctx, "weather forecast task done", slog.Int("noOfHoursUpdated", len(rows)))
	}
}

func needImmediateForecastUpdate(ctx context.Context, db *database.Database) bool {
	startAt := timex.UTCMidnight().AddHours(12)
	if _, err := db.GetWeatherForecast(ctx, startAt); err != nil {
		return true
	}
	return false
}
