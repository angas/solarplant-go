package database

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/angas/solarplant-go/calc"
	"github.com/angas/solarplant-go/hours"
)

type EnergyPriceRow struct {
	When  hours.DateHour
	Price float64
}

func (d *Database) SaveEnergyPrices(ctx context.Context, rows []EnergyPriceRow) error {
	for _, row := range rows {
		d.logger.Debug("saving energy price",
			"hour", row.When,
			"price", row.Price)

		_, err := d.write.ExecContext(ctx, `
			INSERT INTO energy_price (date, hour, price) VALUES (?, ?, ?)
			ON CONFLICT(date, hour) DO UPDATE SET price = excluded.price`,
			row.When.Date,
			row.When.Hour,
			calc.RoundFloat64(row.Price, 4))
		if err != nil {
			return fmt.Errorf("saving energy prices: %w", err)
		}
	}

	return nil
}

func (d *Database) GetEnergyPrice(ctx context.Context, dh hours.DateHour) (EnergyPriceRow, error) {
	row := d.read.QueryRowContext(ctx, `SELECT
		date, hour, price
		FROM energy_price
		WHERE date = ? AND hour = ?`,
		dh.Date, dh.Hour)

	var ep EnergyPriceRow
	err := row.Scan(&ep.When.Date, &ep.When.Hour, &ep.Price)
	if err == sql.ErrNoRows {
		return EnergyPriceRow{}, sql.ErrNoRows
	} else if err != nil {
		return EnergyPriceRow{}, fmt.Errorf("fetching energy price for %s: %w", dh, err)
	}

	return ep, nil
}

func (d *Database) GetEnergyPriceFrom(ctx context.Context, dh hours.DateHour) ([]EnergyPriceRow, error) {
	rows, err := d.read.QueryContext(ctx, `SELECT
		date, hour, price
		FROM energy_price
		WHERE (date = ? AND hour >= ?) OR date > ?
		ORDER BY date, hour ASC`,
		dh.Date, dh.Hour, dh.Date)
	if err != nil {
		return nil, fmt.Errorf("fetching energy price from %s: %w", dh, err)
	}

	defer rows.Close()

	var energyPrices []EnergyPriceRow
	for rows.Next() {
		var ep EnergyPriceRow
		err := rows.Scan(&ep.When.Date, &ep.When.Hour, &ep.Price)
		if err != nil {
			return nil, fmt.Errorf("scanning energy price row: %w", err)
		}

		energyPrices = append(energyPrices, ep)
	}

	return energyPrices, nil
}

func (d *Database) PurgeEnergyPrice(ctx context.Context, retentionDays int) error {
	return d.purgeTable(ctx, "energy_price", retentionDays)
}
