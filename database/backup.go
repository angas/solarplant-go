package database

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

func (d *Database) Backup(ctx context.Context) error {
	dir := filepath.Join(filepath.Dir(d.path), "backups")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create backup directory: %w", err)
	}

	dest := filepath.Join(dir, fmt.Sprintf("%s_solarplant.db", time.Now().Format("20060102_150405")))
	_, err := d.write.ExecContext(ctx, "VACUUM INTO ?", dest)
	if err != nil {
		return fmt.Errorf("vacuuming database into '%s': %w", dest, err)
	}

	zipPath := dest + ".zip"
	zipFile, err := os.Create(zipPath)
	if err != nil {
		return fmt.Errorf("create zip file: %w", err)
	}
	defer zipFile.Close()

	zipWriter := zip.NewWriter(zipFile)
	defer zipWriter.Close()

	dbFile, err := os.Open(dest)
	if err != nil {
		return fmt.Errorf("open database backup for compression: %w", err)
	}
	defer dbFile.Close()

	fileInfo, err := dbFile.Stat()
	if err != nil {
		return fmt.Errorf("get file info: %w", err)
	}

	header, err := zip.FileInfoHeader(fileInfo)
	if err != nil {
		return fmt.Errorf("create zip header: %w", err)
	}
	header.Name = filepath.Base(d.path)
	header.Method = zip.Deflate

	writer, err := zipWriter.CreateHeader(header)
	if err != nil {
		return fmt.Errorf("create zip file entry: %w", err)
	}

	if _, err := io.Copy(writer, dbFile); err != nil {
		return fmt.Errorf("write database to zip: %w", err)
	}

	if err := zipWriter.Close(); err != nil {
		return fmt.Errorf("finalize zip file: %w", err)
	}

	if err := os.Remove(dest); err != nil {
		d.logger.WarnContext(ctx, "could not remove original backup after compression", slog.String("error", err.Error()))
	}

	d.logger.InfoContext(ctx, "database backup complete", slog.String("filename", zipPath))

	return nil
}

func (d *Database) PurgeBackups(ctx context.Context, retentionDays int) error {
	if retentionDays < 1 {
		return nil
	}
	retentionDuration := time.Duration(retentionDays) * 24 * time.Hour

	dir := filepath.Join(filepath.Dir(d.path), "backups")
	d.logger.Debug("purging old backups", slog.String("dir", dir))

	files, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read backup directory: %w", err)
	}

	re := regexp.MustCompile(`^(\d{8}_\d{6})`)
	for _, file := range files {
		match := re.FindString(file.Name())
		if match == "" {
			d.logger.Debug("this is not a backup file", slog.String("filename", file.Name()))
			continue
		}
		t, err := time.Parse("20060102_150405", match)
		if err != nil {
			d.logger.Debug("failed to parse backup timestamp", slog.String("filename", file.Name()), slog.String("error", err.Error()))
			continue
		}
		if time.Since(t) > retentionDuration {
			path := filepath.Join(dir, file.Name())
			d.logger.Debug("deleting old backup", slog.String("path", path))
			if err := os.Remove(path); err != nil {
				return fmt.Errorf("remove old backup '%s': %w", path, err)
			}
		}
	}

	d.logger.Info("backup purge complete")
	return nil
}
