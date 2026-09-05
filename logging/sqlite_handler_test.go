package logging

import (
	"bytes"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/angas/solarplant-go/database"
	"github.com/lmittmann/tint"
)

func TestSQLiteHandlerWithMultiHandler(t *testing.T) {
	ctx := t.Context()
	db, err := database.New(ctx, filepath.Join(t.TempDir(), "logging.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	if err := db.Migrate(ctx); err != nil {
		t.Fatal(err)
	}

	var console bytes.Buffer
	logger := slog.New(slog.NewMultiHandler(
		tint.NewHandler(&console, &tint.Options{Level: slog.LevelInfo, NoColor: true}),
		NewSQLiteHandler(db, slog.LevelWarn, LogAttrFormatText),
	)).With("component", "integration")

	if logger.Enabled(ctx, slog.LevelDebug) {
		t.Fatal("debug logging should be disabled by both destinations")
	}

	// Exercise the real destinations concurrently without the old shared mutex.
	const workers = 20
	var wg sync.WaitGroup
	for i := range workers {
		wg.Go(func() {
			log := logger.With("worker", i)
			log.DebugContext(ctx, fmt.Sprintf("debug-%d", i))
			log.InfoContext(ctx, fmt.Sprintf("info-%d", i))
			log.WarnContext(ctx, fmt.Sprintf("warn-%d", i))
		})
	}
	wg.Wait()

	lines := strings.Split(strings.TrimSpace(console.String()), "\n")
	if len(lines) != 2*workers {
		t.Fatalf("console has %d lines, want %d", len(lines), 2*workers)
	}
	for i := range workers {
		for _, level := range []string{"info", "warn"} {
			message := fmt.Sprintf("%s-%d ", level, i)
			if !strings.Contains(console.String(), message) {
				t.Errorf("console missing %q", message)
			}
		}
	}
	if strings.Contains(console.String(), "debug-") {
		t.Error("console contains a disabled debug message")
	}

	entries, err := db.GetLogEntries(ctx, slog.LevelDebug, 1, 3*workers)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != workers {
		t.Fatalf("database has %d entries, want %d warnings", len(entries), workers)
	}
	seen := make(map[string]bool)
	for _, entry := range entries {
		if entry.Level != int(slog.LevelWarn) {
			t.Errorf("database contains disabled level %d", entry.Level)
		}
		seen[entry.Message] = true
		worker := strings.TrimPrefix(entry.Message, "warn-")
		wantAttrs := "component=integration; worker=" + worker
		if entry.Attrs != wantAttrs {
			t.Errorf("attributes for %q = %q, want %q", entry.Message, entry.Attrs, wantAttrs)
		}
	}
	for i := range workers {
		if !seen[fmt.Sprintf("warn-%d", i)] {
			t.Errorf("database missing warning from worker %d", i)
		}
	}
}
