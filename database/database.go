package database

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"log/slog"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"time"

	"github.com/angas/solarplant-go/hours"
	sqlite "modernc.org/sqlite"
)

//go:embed migrations
var migrationsDir embed.FS

type Database struct {
	logger *slog.Logger
	read   *sql.DB
	write  *sql.DB
	path   string
}

const initSQL = `
	PRAGMA journal_mode = WAL;
	PRAGMA synchronous = NORMAL;
	PRAGMA temp_store = MEMORY;
	PRAGMA busy_timeout = 5000;
	PRAGMA automatic_index = true;
	PRAGMA foreign_keys = ON;
	PRAGMA analysis_limit = 1000;
	PRAGMA trusted_schema = OFF;
`

/**
 * A new database connection.
 * Inspired by: https://theitsolutions.io/blog/modernc.org-sqlite-with-go
 */
func New(ctx context.Context, path string) (*Database, error) {
	sqlite.RegisterConnectionHook(func(conn sqlite.ExecQuerierContext, _ string) error {
		_, err := conn.ExecContext(ctx, initSQL, nil)
		return err
	})

	read, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("error when opening database (read): %w", err)
	}
	read.SetMaxOpenConns(10) // readers can be concurrent
	read.SetConnMaxIdleTime(time.Minute)

	write, err := sql.Open("sqlite", path)
	if err != nil {
		read.Close()
		return nil, fmt.Errorf("error when opening database (write): %w", err)
	}
	write.SetMaxOpenConns(1) // only a single writer ever, no concurrency
	write.SetConnMaxIdleTime(time.Minute)

	d := &Database{
		logger: slog.Default().With(slog.String("module", "database")),
		read:   read,
		write:  write,
		path:   path,
	}

	err = d.migrate(ctx)
	if err != nil {
		panic(fmt.Errorf("database migration error: %w", err))
	}

	return d, nil
}

func (d *Database) SetLogger(logger *slog.Logger) {
	d.logger = logger
}

func (d *Database) Close() {
	d.read.Close()
	d.write.Close()
}

func (d *Database) migrate(ctx context.Context) error {
	var currVer int
	err := d.read.QueryRowContext(ctx, "PRAGMA user_version").Scan(&currVer)
	if err != nil {
		return fmt.Errorf("get current version: %w", err)
	}

	files, err := migrationsDir.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("read migrations directory: %w", err)
	}

	var sqlFiles []string
	for _, f := range files {
		if !f.IsDir() && filepath.Ext(f.Name()) == ".sql" {
			sqlFiles = append(sqlFiles, f.Name())
		}
	}

	slices.Sort(sqlFiles)

	backupBeforeMigration := false
	re := regexp.MustCompile(`^(\d+)[-_]`)

	for _, name := range sqlFiles {
		matches := re.FindStringSubmatch(name)
		if len(matches) < 2 {
			return fmt.Errorf("parse version from migration file: %s", name)
		}
		nextVer, err := strconv.Atoi(matches[1])
		if err != nil {
			return fmt.Errorf("convert migration version from file %s: %w", name, err)
		}
		if nextVer <= currVer {
			continue // Skip migration if already applied
		}

		if !backupBeforeMigration {
			backupBeforeMigration = true
			err = d.Backup(ctx)
			if err != nil {
				return fmt.Errorf("backup database before migration: %w", err)
			}
		}

		d.logger.Debug(fmt.Sprintf("applying migration %d", nextVer))

		data, err := migrationsDir.ReadFile(path.Join("migrations", name))
		if err != nil {
			return fmt.Errorf("read migration file %s: %w", name, err)
		}

		tx, err := d.write.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("start transaction for migration %d: %w", nextVer, err)
		}

		_, err = tx.ExecContext(ctx, string(data))
		if err != nil {
			if err := tx.Rollback(); err != nil {
				return fmt.Errorf("rollback migration %d: %w", nextVer, err)
			}
			return fmt.Errorf("apply migration %d: %w", nextVer, err)
		}

		_, err = tx.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d;", nextVer))
		if err != nil {
			if err = tx.Rollback(); err != nil {
				return fmt.Errorf("rollback migration %d: %w", nextVer, err)
			}
			return fmt.Errorf("update database version for migration %d: %w", nextVer, err)
		}

		err = tx.Commit()
		if err != nil {
			return fmt.Errorf("commit migration %d: %w", nextVer, err)
		}
	}

	return nil
}

func (d *Database) purgeTable(ctx context.Context, table string, retentionDays int) error {
	d.logger.Debug(fmt.Sprintf("purging table %s", table))
	duration := 24 * time.Hour * time.Duration(retentionDays)
	before := hours.FromTime(time.Now().Add(-duration))
	res, err := d.write.ExecContext(ctx, fmt.Sprintf(`
		DELETE FROM %s 
		WHERE (date = ? AND hour < ?) OR date < ?`, table),
		before.Date, before.Hour, before.Date)
	if err != nil {
		return fmt.Errorf("error when purging %s: %w", table, err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		d.logger.Warn("can't get rows affected by purge", slog.String("table", table), slog.Any("error", err))
	} else {
		d.logger.Debug(fmt.Sprintf("purged %d rows from %s", rows, table))
	}

	return nil
}
