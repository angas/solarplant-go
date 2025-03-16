package database

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/angas/solarplant-go/calc"
	"github.com/angas/solarplant-go/hours"
)

type WeatherForecastRow struct {
	When          hours.DateHour
	CloudCover    uint8
	Temperature   float64
	Precipitation float64
}

func (d *Database) SaveForecast(ctx context.Context, rows []WeatherForecastRow) error {
	for _, row := range rows {
		d.logger.Debug("saving weather forecast",
			"hour", row.When,
			"cloud_cover", row.CloudCover,
			"temperature", row.Temperature,
			"precipitation", row.Precipitation)

		_, err := d.write.ExecContext(ctx, `
		INSERT INTO weather_forecast (
			date,
			hour, 
			cloud_cover, 
			temperature,
			precipitation
		) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(date, hour) DO UPDATE SET
    	cloud_cover = excluded.cloud_cover,
    	temperature = excluded.temperature,
			precipitation = excluded.precipitation`,
			row.When.Date,
			row.When.Hour,
			row.CloudCover,
			calc.TwoDecimals(row.Temperature),
			calc.TwoDecimals(row.Precipitation))
		if err != nil {
			return fmt.Errorf("saving weather forecast: %w", err)
		}
	}

	return nil
}

func (d *Database) GetWeatherForecast(ctx context.Context, dh hours.DateHour) (WeatherForecastRow, error) {
	row := d.read.QueryRowContext(ctx, `
		SELECT date, hour, cloud_cover, temperature, precipitation
		FROM weather_forecast 
		WHERE date = ? AND hour = ?`,
		dh.Date, dh.Hour)

	var fc WeatherForecastRow
	err := row.Scan(&fc.When.Date, &fc.When.Hour, &fc.CloudCover, &fc.Temperature, &fc.Precipitation)
	if err == sql.ErrNoRows {
		return WeatherForecastRow{}, sql.ErrNoRows
	}
	if err != nil {
		return WeatherForecastRow{}, fmt.Errorf("fetching weather forecast for %s: %w", dh, err)
	}

	return fc, nil
}

func (d *Database) GetWeatherForecastFrom(ctx context.Context, dh hours.DateHour) ([]WeatherForecastRow, error) {
	rows, err := d.read.QueryContext(ctx, `
		SELECT date, hour, cloud_cover, temperature, precipitation
		FROM weather_forecast 
		WHERE (date = ? AND hour >= ?) OR date > ?`,
		dh.Date, dh.Hour, dh.Date)
	if err != nil {
		return nil, fmt.Errorf("fetching weather forecast from %s: %w", dh, err)
	}

	defer rows.Close()

	var forecasts []WeatherForecastRow
	for rows.Next() {
		var row WeatherForecastRow
		err := rows.Scan(
			&row.When.Date,
			&row.When.Hour,
			&row.CloudCover,
			&row.Temperature,
			&row.Precipitation)
		if err != nil {
			return nil, fmt.Errorf("scanning weather forecast row: %w", err)
		}

		forecasts = append(forecasts, row)
	}

	return forecasts, nil
}

func (d *Database) PurgeWeatherForecast(ctx context.Context, retentionDays int) error {
	return d.purgeTable(ctx, "weather_forecast", retentionDays)
}
