package database

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/angas/solarplant-go/calc"
	"github.com/angas/solarplant-go/timex"
)

type WeatherForecastRow struct {
	StartAt       timex.BucketTime
	CloudCover    uint8
	Temperature   float64
	Precipitation float64
}

func (d *Database) SaveForecast(ctx context.Context, rows []WeatherForecastRow) error {
	for _, row := range rows {
		d.logger.Debug("saving weather forecast",
			"start_at", row.StartAt,
			"cloud_cover", row.CloudCover,
			"temperature", row.Temperature,
			"precipitation", row.Precipitation)

		_, err := d.write.ExecContext(ctx, `
		INSERT INTO weather_forecast (
			start_at,
			cloud_cover,
			temperature,
			precipitation
		) VALUES (?, ?, ?, ?)
		ON CONFLICT(start_at) DO UPDATE SET
    	cloud_cover = excluded.cloud_cover,
    	temperature = excluded.temperature,
			precipitation = excluded.precipitation`,
			row.StartAt.String(),
			row.CloudCover,
			calc.TwoDecimals(row.Temperature),
			calc.TwoDecimals(row.Precipitation))
		if err != nil {
			return fmt.Errorf("saving weather forecast: %w", err)
		}
	}

	return nil
}

func (d *Database) GetWeatherForecast(ctx context.Context, startAt timex.BucketTime) (WeatherForecastRow, error) {
	row := d.read.QueryRowContext(ctx, `
		SELECT start_at, cloud_cover, temperature, precipitation
		FROM weather_forecast
		WHERE start_at = ?`,
		startAt.String())

	var fc WeatherForecastRow
	var startAtStr string
	err := row.Scan(&startAtStr, &fc.CloudCover, &fc.Temperature, &fc.Precipitation)
	if err == sql.ErrNoRows {
		return WeatherForecastRow{}, sql.ErrNoRows
	}
	if err != nil {
		return WeatherForecastRow{}, fmt.Errorf("fetching weather forecast for %s: %w", startAt.String(), err)
	}
	fc.StartAt, err = timex.ParseBucketTime(startAtStr, timex.BucketSizeHour)
	if err != nil {
		return WeatherForecastRow{}, fmt.Errorf("fetching weather forecast for %s: %w", startAt.String(), err)
	}

	return fc, nil
}

func (d *Database) GetWeatherForecastFrom(ctx context.Context, startAt timex.BucketTime) ([]WeatherForecastRow, error) {
	forecasts, err := d.queryRows(ctx, `
		SELECT start_at, cloud_cover, temperature, precipitation
		FROM weather_forecast
		WHERE start_at >= ?`,
		func(rows *sql.Rows) (WeatherForecastRow, error) {
			var row WeatherForecastRow
			var startAtStr string
			err := rows.Scan(
				&startAtStr,
				&row.CloudCover,
				&row.Temperature,
				&row.Precipitation)
			if err != nil {
				return WeatherForecastRow{}, fmt.Errorf("scanning weather forecast row: %w", err)
			}

			row.StartAt, err = timex.ParseBucketTime(startAtStr, timex.BucketSizeHour)
			if err != nil {
				return WeatherForecastRow{}, fmt.Errorf("parsing weather forecast start time: %w", err)
			}
			return row, nil
		}, startAt.String())
	if err != nil {
		return nil, fmt.Errorf("fetching weather forecast from %s: %w", startAt.String(), err)
	}

	return forecasts, nil
}

func (d *Database) PurgeWeatherForecast(ctx context.Context, retentionDays int) error {
	return d.purgeTable(ctx, "weather_forecast", "start_at", retentionDays)
}
