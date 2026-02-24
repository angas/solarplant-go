package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/angas/solarplant-go/ferroamp"
	"github.com/angas/solarplant-go/timex"
)

const faSnapshotBucketSize = timex.BucketSize15Minutes

type FaSnapshotRow struct {
	Timestamp timex.BucketTime
	Data      ferroamp.FaData
}

func (r FaSnapshotRow) IsZero() bool {
	return r.Timestamp.Empty()
}

func (d *Database) SaveFaSnapshot(ctx context.Context, row FaSnapshotRow) error {
	d.logger.Debug("saving ferroamp snapshot", slog.String("timestamp", row.Timestamp.String()))

	data, err := json.Marshal(row.Data)
	if err != nil {
		return fmt.Errorf("marshalling ferroamp snapshot to json: %w", err)
	}

	_, err = d.write.ExecContext(ctx, "INSERT INTO fa_snapshot (timestamp, data)	VALUES (?, ?)", row.Timestamp.String(), data)
	if err != nil {
		return fmt.Errorf("saving ferroamp snapshot (%s): %w", row.Timestamp.String(), err)
	}

	return nil
}

func (d *Database) GetFaSnapshot(ctx context.Context, hour timex.BucketTime) (FaSnapshotRow, error) {
	row := d.read.QueryRowContext(ctx, "SELECT timestamp, data FROM fa_snapshot WHERE timestamp = ?", hour.String())

	var jsonData string
	var r FaSnapshotRow
	var timestampStr string
	err := row.Scan(&timestampStr, &jsonData)
	if err == sql.ErrNoRows {
		return FaSnapshotRow{}, sql.ErrNoRows
	}
	if err != nil {
		return FaSnapshotRow{}, fmt.Errorf("fetching ferroamp snapshot from %s: %w", timestampStr, err)
	}
	r.Timestamp, err = timex.ParseBucketTime(timestampStr, faSnapshotBucketSize)
	if err != nil {
		return FaSnapshotRow{}, fmt.Errorf("parsing ferroamp snapshot time (%s): %w", timestampStr, err)
	}
	err = json.Unmarshal([]byte(jsonData), &r.Data)
	if err != nil {
		return FaSnapshotRow{}, fmt.Errorf("unmarshaling ferroamp snapshot from JSON: %w", err)
	}

	return r, nil
}

func (d *Database) GetFaSnapshotFrom(ctx context.Context, from timex.BucketTime) ([]FaSnapshotRow, error) {
	rows, err := d.read.QueryContext(ctx, "SELECT timestamp, data FROM fa_snapshot WHERE timestamp >= ?", from.String())
	if err != nil {
		return nil, fmt.Errorf("fetching ferroamp snapshots since %s: %w", from.String(), err)
	}
	defer rows.Close()

	var result []FaSnapshotRow
	for rows.Next() {
		var jsonData string
		var r FaSnapshotRow
		var timestampStr string

		err := rows.Scan(&timestampStr, &jsonData)
		if err != nil {
			return nil, fmt.Errorf("scanning fa_snapshot row (%s): %w", timestampStr, err)
		}

		timestamp, err := timex.ParseBucketTime(timestampStr, timex.BucketSizeNone)
		if err != nil {
			return nil, fmt.Errorf("parsing ferroamp snapshot time (%s): %w", timestampStr, err)
		}
		r.Timestamp = timestamp

		err = json.Unmarshal([]byte(jsonData), &r.Data)
		if err != nil {
			return []FaSnapshotRow{}, fmt.Errorf("unmarshaling ferroamp snapshot from JSON: %w", err)
		}

		result = append(result, r)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating ferroamp snapshots: %w", err)
	}

	return result, nil
}

func (d *Database) PurgeFaSnapshot(ctx context.Context, retentionDays int) error {
	return d.purgeTable(ctx, "fa_snapshot", "timestamp", retentionDays)
}
