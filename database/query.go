package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// errSkipRow lets a decoder omit a malformed row after logging why it was skipped.
// Other decoding errors abort the query without returning partial results.
var errSkipRow = errors.New("skip row")

// queryRows owns the SQL cursor; decode must scan only the current row.
// An empty result is nil. Callers can normalize it if their API requires a slice.
func (d *Database) queryRows[T any](ctx context.Context, query string, decode func(*sql.Rows) (T, error), args ...any) ([]T, error) {
	rows, err := d.read.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("querying rows: %w", err)
	}
	defer rows.Close()

	var result []T
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		row, err := decode(rows)
		if errors.Is(err, errSkipRow) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("decoding row: %w", err)
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating rows: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return result, nil
}
