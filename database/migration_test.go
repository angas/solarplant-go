package database

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestRollbackMigration(t *testing.T) {
	for _, alreadyRolledBack := range []bool{false, true} {
		name := "rollback succeeds"
		if alreadyRolledBack {
			name = "rollback fails"
		}
		t.Run(name, func(t *testing.T) {
			db := newQueryTestDatabase(t)
			ctx := t.Context()
			if _, err := db.write.ExecContext(ctx, "CREATE TABLE migration_test (value INTEGER)"); err != nil {
				t.Fatal(err)
			}
			tx, err := db.write.BeginTx(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback()
			if _, err := tx.ExecContext(ctx, "INSERT INTO migration_test VALUES (1)"); err != nil {
				t.Fatal(err)
			}
			_, originalErr := tx.ExecContext(ctx, "INSERT INTO missing_table VALUES (2)")
			if originalErr == nil {
				t.Fatal("expected failing migration statement")
			}
			if alreadyRolledBack {
				if err := tx.Rollback(); err != nil {
					t.Fatal(err)
				}
			}
			cause := fmt.Errorf("apply migration 5: %w", originalErr)
			got := rollbackMigration(tx, 5, cause)
			if !errors.Is(got, originalErr) || !strings.Contains(got.Error(), "apply migration 5") {
				t.Fatalf("original migration error lost: %v", got)
			}
			if alreadyRolledBack {
				if !errors.Is(got, sql.ErrTxDone) || !strings.Contains(got.Error(), "rollback migration 5") {
					t.Fatalf("rollback error lost: %v", got)
				}
			} else if got != cause {
				t.Fatalf("successful rollback changed the original error: %v", got)
			}
			var count int
			if err := db.read.QueryRowContext(ctx, "SELECT COUNT(*) FROM migration_test").Scan(&count); err != nil {
				t.Fatal(err)
			}
			if count != 0 {
				t.Fatalf("failed migration retained %d inserted rows", count)
			}
		})
	}
}
