package ferroamp

import (
	"context"
	"log/slog"
)

// minLevelHandler drops records below a minimum level before passing them on
// to the wrapped handler. It is used to quiet chatty third-party loggers
// without changing the level of the rest of the application.
type minLevelHandler struct {
	handler  slog.Handler
	minLevel slog.Level
}

func newMinLevelHandler(handler slog.Handler, minLevel slog.Level) *minLevelHandler {
	return &minLevelHandler{handler: handler, minLevel: minLevel}
}

func (h *minLevelHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return level >= h.minLevel && h.handler.Enabled(ctx, level)
}

func (h *minLevelHandler) Handle(ctx context.Context, r slog.Record) error {
	// Enabled is normally checked by slog.Logger, but handlers that wrap this
	// one may call Handle directly.
	if r.Level < h.minLevel {
		return nil
	}
	return h.handler.Handle(ctx, r)
}

func (h *minLevelHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &minLevelHandler{handler: h.handler.WithAttrs(attrs), minLevel: h.minLevel}
}

func (h *minLevelHandler) WithGroup(name string) slog.Handler {
	return &minLevelHandler{handler: h.handler.WithGroup(name), minLevel: h.minLevel}
}
