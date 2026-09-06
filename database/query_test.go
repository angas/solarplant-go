package database

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func newQueryTestDatabase(t *testing.T) *Database {
	t.Helper()
	path := filepath.Join(t.TempDir(), "query.db")
	conn, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	// A single connection makes cursor leaks observable in Stats().InUse.
	conn.SetMaxOpenConns(1)
	return &Database{read: conn, write: conn, path: path, logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
}

func TestQueryRows(t *testing.T) {
	db := newQueryTestDatabase(t)
	decodeErr := errors.New("invalid row")
	scanInt := func(rows *sql.Rows) (int, error) {
		var value int
		err := rows.Scan(&value)
		return value, err
	}

	for _, tc := range []struct {
		name    string
		query   string
		args    []any
		decode  func(*sql.Rows) (int, error)
		want    []int
		wantErr string
		cause   error
	}{
		{name: "order and parameters", query: "SELECT ? UNION ALL SELECT ?", args: []any{3, 1}, decode: scanInt, want: []int{3, 1}},
		{name: "empty", query: "SELECT 1 WHERE 0", decode: scanInt},
		{name: "skip", query: "SELECT 1 UNION ALL SELECT 2 UNION ALL SELECT 3", want: []int{1, 3}, decode: func(rows *sql.Rows) (int, error) {
			value, err := scanInt(rows)
			if value == 2 && err == nil {
				return 0, errSkipRow
			}
			return value, err
		}},
		{name: "decode failure discards partial results", query: "SELECT 1 UNION ALL SELECT 2 UNION ALL SELECT 3", wantErr: "decoding row", cause: decodeErr, decode: func(rows *sql.Rows) (int, error) {
			value, err := scanInt(rows)
			if value == 2 && err == nil {
				return 0, decodeErr
			}
			return value, err
		}},
		{name: "scan failure", query: "SELECT 'not a number'", decode: scanInt, wantErr: "decoding row"},
		{name: "query failure", query: "SELECT * FROM missing_table", decode: scanInt, wantErr: "querying rows"},
		// SQLite yields the first row, then fails while advancing to the second.
		{name: "iteration failure discards partial results", query: "SELECT 1 UNION ALL SELECT abs(-9223372036854775808)", decode: scanInt, wantErr: "iterating rows"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := db.queryRows(t.Context(), tc.query, tc.decode, tc.args...)
			if tc.wantErr == "" && err != nil {
				t.Fatal(err)
			}
			if tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)) {
				t.Fatalf("error = %v, want %q", err, tc.wantErr)
			}
			if tc.cause != nil && !errors.Is(err, tc.cause) {
				t.Errorf("error %v does not wrap %v", err, tc.cause)
			}
			if !slices.Equal(got, tc.want) || (got == nil) != (tc.want == nil) {
				t.Errorf("rows = %#v, want %#v", got, tc.want)
			}
			if n := db.read.Stats().InUse; n != 0 {
				t.Fatalf("query left %d connections in use", n)
			}
		})
	}
}

func TestQueryRowsCancellation(t *testing.T) {
	db := newQueryTestDatabase(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	got, err := db.queryRows(ctx, "SELECT 1 UNION ALL SELECT 2", func(rows *sql.Rows) (int, error) {
		var value int
		err := rows.Scan(&value)
		cancel()
		return value, err
	})
	if !errors.Is(err, context.Canceled) || got != nil {
		t.Fatalf("rows = %v, error = %v; want no partial rows and context.Canceled", got, err)
	}
	if n := db.read.Stats().InUse; n != 0 {
		t.Fatalf("canceled query left %d connections in use", n)
	}
}
