package database

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"
)

type LogEntryRow struct {
	Timestamp time.Time
	Level     int
	Message   string
	Attrs     string
}

func (d *Database) SaveLogEntry(ctx context.Context, r LogEntryRow) error {
	_, err := d.write.ExecContext(ctx, `
		INSERT INTO log (timestamp, level, message, attrs)
		VALUES (?, ?, ?, ?)`,
		r.Timestamp.UTC().Format(time.RFC3339),
		r.Level,
		r.Message,
		r.Attrs)
	if err != nil {
		return fmt.Errorf("saving log entry: %w", err)
	}
	return nil
}

func (d *Database) GetLogEntries(ctx context.Context, minLvl slog.Level, page, pageSize int) ([]LogEntryRow, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}

	entries, err := d.queryRows(ctx, `
		SELECT timestamp, level, message, attrs
		FROM log
		WHERE level >= ?
		ORDER BY id DESC
		LIMIT ? OFFSET ?`,
		func(rows *sql.Rows) (LogEntryRow, error) {
			var ts string
			var r LogEntryRow
			err := rows.Scan(&ts, &r.Level, &r.Message, &r.Attrs)
			if err != nil {
				return LogEntryRow{}, err
			}
			r.Timestamp, err = time.Parse(time.RFC3339, ts)
			if err != nil {
				return LogEntryRow{}, fmt.Errorf("parsing timestamp: %w", err)
			}
			return r, nil
		}, minLvl, pageSize, (page-1)*pageSize)
	if err != nil {
		return nil, fmt.Errorf("fetching log entries: %w", err)
	}

	return entries, nil
}

func (d *Database) PurgeLog(ctx context.Context, maxLogEntries int) error {
	if maxLogEntries < 1 {
		return nil
	}

	d.logger.Debug("purging log")
	_, err := d.write.ExecContext(ctx, `
		DELETE FROM log WHERE id <= (SELECT id FROM log ORDER BY id DESC LIMIT 1 OFFSET ?)`, maxLogEntries)
	if err != nil {
		return fmt.Errorf("purging log: %w", err)
	}
	return nil
}
