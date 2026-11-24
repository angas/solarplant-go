package ferroamp

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestMinLevelHandler(t *testing.T) {
	var out bytes.Buffer
	inner := slog.NewTextHandler(&out, &slog.HandlerOptions{Level: slog.LevelDebug})
	handler := newMinLevelHandler(inner, slog.LevelInfo)
	logger := slog.New(handler).With("module", "test").WithGroup("grp")

	if logger.Enabled(context.Background(), slog.LevelDebug) {
		t.Error("debug is enabled below the minimum level")
	}

	logger.Debug("dropped")
	logger.Info("kept info", "key", "value")
	logger.Warn("kept warn")

	// Handle must filter even when the caller skips Enabled.
	if err := handler.Handle(context.Background(), slog.NewRecord(time.Now(), slog.LevelDebug, "dropped direct", 0)); err != nil {
		t.Fatal(err)
	}

	logs := out.String()
	if strings.Contains(logs, "dropped") {
		t.Errorf("record below the minimum level was logged: %s", logs)
	}
	for _, want := range []string{"kept info", "kept warn", "module=test", "grp.key=value"} {
		if !strings.Contains(logs, want) {
			t.Errorf("log output is missing %q: %s", want, logs)
		}
	}
}

func TestMinLevelHandlerRespectsWrappedLevel(t *testing.T) {
	var out bytes.Buffer
	inner := slog.NewTextHandler(&out, &slog.HandlerOptions{Level: slog.LevelWarn})
	logger := slog.New(newMinLevelHandler(inner, slog.LevelInfo))

	if logger.Enabled(context.Background(), slog.LevelInfo) {
		t.Error("info is enabled although the wrapped handler requires warn")
	}
	logger.Info("dropped")
	if out.Len() != 0 {
		t.Errorf("record below the wrapped handler's level was logged: %s", out.String())
	}
}
