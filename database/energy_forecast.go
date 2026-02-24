package database

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"

	"github.com/angas/solarplant-go/calc"
	"github.com/angas/solarplant-go/timex"
)

const energyForecastBucketSize = timex.BucketSize15Minutes

type EnergyForecastRow struct {
	StartAt timex.BucketTime
	// Estimated production in kWh normalized to a situation without clouds.
	Production float64
	// Estimated consumption in kWh
	Consumption float64
}

func (d *Database) SaveEnergyForecast(ctx context.Context, rows []EnergyForecastRow) error {
	for _, row := range rows {
		d.logger.Debug("saving energy forecast",
			slog.String("startAt", row.StartAt.String()),
			slog.Float64("production", row.Production),
			slog.Float64("consumption", row.Consumption))

		_, err := d.write.ExecContext(ctx, `
		INSERT INTO energy_forecast (start_at, production, consumption)
		VALUES (?, ?, ?)
		ON CONFLICT(start_at) DO UPDATE SET
			production = excluded.production,
			consumption = excluded.consumption;`,
			row.StartAt.String(),
			calc.TwoDecimals(row.Production),
			calc.TwoDecimals(row.Consumption),
		)
		if err != nil {
			return fmt.Errorf("saving energy forecast: %w", err)
		}
	}
	return nil
}

func (d *Database) GetEnergyForecast(ctx context.Context, startAt timex.BucketTime) (EnergyForecastRow, error) {
	row := d.read.QueryRowContext(ctx, `
		SELECT start_at, production, consumption
		FROM energy_forecast
		WHERE (start_at = ?)`,
		startAt.String())

	var ef EnergyForecastRow
	var startAtStr string
	err := row.Scan(&startAtStr, &ef.Production, &ef.Consumption)
	if err == sql.ErrNoRows {
		return EnergyForecastRow{}, sql.ErrNoRows
	} else if err != nil {
		return EnergyForecastRow{}, fmt.Errorf("scanning energy forecast row: %w", err)
	}
	ef.StartAt, err = timex.ParseBucketTime(startAtStr, energyForecastBucketSize)
	if err != nil {
		return EnergyForecastRow{}, fmt.Errorf("parsing start_at (%s): %w", startAtStr, err)
	}

	return ef, nil
}

func (d *Database) GetEnergyForecastFrom(ctx context.Context, startAt timex.BucketTime) ([]EnergyForecastRow, error) {
	rows, err := d.read.QueryContext(ctx, `
		SELECT start_at, production,consumption
		FROM energy_forecast
		WHERE (start_at >= ?)`,
		startAt.String())
	if err != nil {
		return nil, fmt.Errorf("fetching energy forecast from %s: %w", startAt.String(), err)
	}
	defer rows.Close()

	var efs []EnergyForecastRow
	var startAtStr string
	for rows.Next() {
		var ef EnergyForecastRow
		err := rows.Scan(&startAtStr, &ef.Production, &ef.Consumption)
		if err != nil {
			return nil, err
		}
		ef.StartAt, err = timex.ParseBucketTime(startAtStr, energyForecastBucketSize)
		if err != nil {
			d.logger.WarnContext(ctx, "failed to parse start_at", slog.String("startAt", startAtStr), slog.Any("error", err))
			continue
		}
		efs = append(efs, ef)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating energy forecast rows: %w", err)
	}

	return efs, nil
}

func (d *Database) PurgeEnergyForecast(ctx context.Context, retentionDays int) error {
	return d.purgeTable(ctx, "energy_forecast", "start_at", retentionDays)
}
