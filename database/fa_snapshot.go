package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/angas/solarplant-go/ferroamp"
	"github.com/angas/solarplant-go/hours"
)

type FaSnapshotRow struct {
	When hours.DateHour
	Data ferroamp.FaData
}

func (r FaSnapshotRow) IsZero() bool {
	return r.When.IsZero()
}

func (d *Database) SaveFaSnapshot(ctx context.Context, row FaSnapshotRow) error {
	d.logger.Debug("saving ferroamp snapshot", "hour", row.When)

	data, err := json.Marshal(row.Data)
	if err != nil {
		return fmt.Errorf("marshalling ferroamp snapshot to JSON: %w", err)
	}

	_, err = d.write.ExecContext(ctx, `
		INSERT INTO fa_snapshot (date, hour, data)
		VALUES (?, ?, ?)`,
		row.When.Date,
		row.When.Hour,
		data,
	)
	if err != nil {
		return fmt.Errorf("saving ferroamp snapshot: %w", err)
	}

	return nil
}

func (d *Database) GetFaSnapshot(ctx context.Context, dh hours.DateHour) (FaSnapshotRow, error) {
	row := d.read.QueryRowContext(ctx, `
		SELECT date, hour, data
		FROM fa_snapshot
		WHERE date = ? AND hour = ?`,
		dh.Date, dh.Hour)

	var jsonData string
	var r FaSnapshotRow
	err := row.Scan(&r.When.Date, &r.When.Hour, &jsonData)
	if err == sql.ErrNoRows {
		return FaSnapshotRow{}, sql.ErrNoRows
	}
	if err != nil {
		return FaSnapshotRow{}, fmt.Errorf("fetching ferroamp snapshot for %s: %w", dh, err)
	}

	err = json.Unmarshal([]byte(jsonData), &r.Data)
	if err != nil {
		return FaSnapshotRow{}, fmt.Errorf("unmarshaling ferroamp snapshot from JSON: %w", err)
	}

	return r, nil
}

func (d *Database) GetFaSnapshotFrom(ctx context.Context, dh hours.DateHour) ([]FaSnapshotRow, error) {
	rows, err := d.read.QueryContext(ctx, `
		SELECT date, hour, data
		FROM fa_snapshot
		WHERE date > ? OR (date = ? AND hour >= ?)`,
		dh.Date, dh.Date, dh.Hour)

	if err != nil {
		return nil, fmt.Errorf("fetching ferroamp snapshots since %s: %w", dh, err)
	}

	defer rows.Close()

	var result []FaSnapshotRow
	for rows.Next() {
		var jsonData string
		var r FaSnapshotRow

		err := rows.Scan(&r.When.Date, &r.When.Hour, &jsonData)
		if err != nil {
			return nil, fmt.Errorf("scanning energy price row: %w", err)
		}

		err = json.Unmarshal([]byte(jsonData), &r.Data)
		if err != nil {
			return []FaSnapshotRow{}, fmt.Errorf("unmarshaling ferroamp snapshot from JSON: %w", err)
		}

		result = append(result, r)
	}

	return result, nil
}

func (d *Database) PurgeFaSnapshot(ctx context.Context, retentionDays int) error {
	return d.purgeTable(ctx, "fa_snapshot", retentionDays)
}
