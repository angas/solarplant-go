package database

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"

	"github.com/angas/solarplant-go/calc"
	"github.com/angas/solarplant-go/timex"
)

type EnergyPriceRow struct {
	StartAt timex.BucketTime // Start time of the energy price in UTC
	Price   float64          // Price of energy in SEC per kWh, not including taxes or fees
}

const energyPriceBucketSize = timex.BucketSize15Minutes

func (d *Database) SaveEnergyPrices(ctx context.Context, rows []EnergyPriceRow) error {
	for _, row := range rows {
		d.logger.DebugContext(ctx, "saving energy price",
			slog.String("startAt", row.StartAt.String()),
			slog.Float64("price", row.Price))

		_, err := d.write.ExecContext(ctx, `
			INSERT INTO energy_price (start_at, price) VALUES (?, ?)
			ON CONFLICT(start_at) DO UPDATE SET price = excluded.price`,
			row.StartAt.String(),
			calc.RoundFloat64(row.Price, 4))
		if err != nil {
			return fmt.Errorf("saving energy prices (%s): %w", row.StartAt.String(), err)
		}
	}

	return nil
}

func (d *Database) GetEnergyPriceForHour(ctx context.Context, hour timex.BucketTime) (EnergyPriceRow, error) {
	row := d.read.QueryRowContext(ctx, "SELECT start_at, price FROM energy_price WHERE start_at = ?", hour.TruncToHour().String())
	return d.scanEnergyPriceRow(ctx, row.Scan)
}

func (d *Database) GetAvgEnergyPriceForHour(ctx context.Context, hour timex.BucketTime) (EnergyPriceRow, error) {
	hour = hour.TruncToHour()
	row := d.read.QueryRowContext(ctx, `
		SELECT STRFTIME('%Y-%m-%dT%H:00:00Z', start_at), AVG(price)
		FROM energy_price
		WHERE start_at >= ? AND start_at < ?`,
		hour.String(), hour.AddHours(1).String())

	return d.scanEnergyPriceRow(ctx, row.Scan)
}

func (d *Database) GetEnergyPriceFrom(ctx context.Context, startAt timex.BucketTime) ([]EnergyPriceRow, error) {
	rows, err := d.read.QueryContext(ctx, `
		SELECT start_at, price
		FROM energy_price
		WHERE start_at >= ?
		ORDER BY start_at ASC`,
		startAt.String())
	if err != nil {
		return nil, fmt.Errorf("fetching energy price (%s): %w", startAt.String(), err)
	}
	defer rows.Close()

	return d.scanEnergyPriceRows(ctx, rows)
}

func (d *Database) GetHourlyAvgEnergyPriceFrom(ctx context.Context, startAt timex.BucketTime) ([]EnergyPriceRow, error) {
	rows, err := d.read.QueryContext(ctx, `
		SELECT STRFTIME('%Y-%m-%dT%H:00:00Z', start_at) hour, AVG(price)
    FROM energy_price
    WHERE start_at >= ?
    GROUP BY hour
    ORDER BY hour ASC`,
		startAt.String())
	if err != nil {
		return nil, fmt.Errorf("fetching average energy price (%s): %w", startAt.String(), err)
	}
	defer rows.Close()

	return d.scanEnergyPriceRows(ctx, rows)
}

func (d *Database) scanEnergyPriceRow(ctx context.Context, scan func(dest ...any) error) (EnergyPriceRow, error) {
	var startAtStr sql.NullString
	var price sql.NullFloat64
	err := scan(&startAtStr, &price)
	if err != nil {
		return EnergyPriceRow{}, fmt.Errorf("scanning energy price row: %w", err)
	}

	if !startAtStr.Valid {
		return EnergyPriceRow{}, sql.ErrNoRows // or a custom error
	}

	startAt, err := timex.ParseBucketTime(startAtStr.String, energyPriceBucketSize)
	if err != nil {
		d.logger.WarnContext(ctx, "getting energy price, parsing start_at", slog.String("startAt", startAtStr.String), slog.Any("error", err))
		return EnergyPriceRow{}, err
	}

	return EnergyPriceRow{StartAt: startAt, Price: price.Float64}, nil
}

func (d *Database) scanEnergyPriceRows(ctx context.Context, rows *sql.Rows) ([]EnergyPriceRow, error) {
	eps := []EnergyPriceRow{}
	for rows.Next() {
		ep, err := d.scanEnergyPriceRow(ctx, rows.Scan)
		if err != nil {
			return nil, err
		}

		eps = append(eps, ep)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating energy price rows: %w", err)
	}

	return eps, nil
}

func (d *Database) PurgeEnergyPrice(ctx context.Context, retentionDays int) error {
	return d.purgeTable(ctx, "energy_price", "start_at", retentionDays)
}
