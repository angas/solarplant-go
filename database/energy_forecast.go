package database

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/angas/solarplant-go/calc"
	"github.com/angas/solarplant-go/hours"
)

type EnergyForecastRow struct {
	When hours.DateHour
	/** Estimated production in kWh normalized to a situation without clouds */
	Production float64
	/** Estimated consumption in kWh */
	Consumption float64
}

func (d *Database) SaveEnergyForecast(ctx context.Context, rows []EnergyForecastRow) error {
	for _, row := range rows {
		d.logger.Debug("saving energy forecast",
			"hour", row.When,
			"production", row.Production,
			"consumption", row.Consumption)

		_, err := d.write.ExecContext(ctx, `
		INSERT INTO energy_forecast (
			date,
			hour,
			production,
			consumption
		) VALUES (?, ?, ?, ?)
		ON CONFLICT(date, hour) DO UPDATE SET
			production = excluded.production,
			consumption = excluded.consumption;`,
			row.When.Date,
			row.When.Hour,
			calc.TwoDecimals(row.Production),
			calc.TwoDecimals(row.Consumption),
		)
		if err != nil {
			return fmt.Errorf("saving energy forecast: %w", err)
		}
	}
	return nil
}

func (d *Database) GetEnergyForecast(ctx context.Context, dh hours.DateHour) (EnergyForecastRow, error) {
	row := d.read.QueryRowContext(ctx, `
		SELECT
			date,
			hour,
			production,
			consumption
		FROM energy_forecast
		WHERE (date = ? AND hour = ?)`,
		dh.Date, dh.Hour)

	var ef EnergyForecastRow
	err := row.Scan(&ef.When.Date, &ef.When.Hour, &ef.Production, &ef.Consumption)
	if err == sql.ErrNoRows {
		return EnergyForecastRow{}, sql.ErrNoRows
	}
	if err != nil {
		return EnergyForecastRow{}, fmt.Errorf("scanning energy forecast row: %w", err)
	}

	return ef, nil
}

func (d *Database) GetEnergyForecastFrom(ctx context.Context, dh hours.DateHour) ([]EnergyForecastRow, error) {
	rows, err := d.read.QueryContext(ctx, `
		SELECT
			date,
			hour,
			production,
			consumption
		FROM energy_forecast
		WHERE (date = ? AND hour >= ?) OR date > ?`,
		dh.Date, dh.Hour, dh.Date)
	if err != nil {
		return nil, fmt.Errorf("fetching energy forecast from %s: %w", dh, err)
	}
	defer rows.Close()

	var ef []EnergyForecastRow
	for rows.Next() {
		var row EnergyForecastRow
		err := rows.Scan(
			&row.When.Date,
			&row.When.Hour,
			&row.Production,
			&row.Consumption)
		if err != nil {
			return nil, err
		}
		ef = append(ef, row)
	}

	return ef, nil
}

func (d *Database) PurgeEnergyForecast(ctx context.Context, retentionDays int) error {
	return d.purgeTable(ctx, "energy_forecast", retentionDays)
}
