package database

import (
	"context"
	"fmt"
	"sync"

	"github.com/angas/solarplant-go/hours"
)

type RecentHour struct {
	When hours.DateHour
	Ts   TimeSeriesRow
	Fa   FaSnapshotRow
}

/** A cache of the most recent hours */
type RecentHours struct {
	mu    sync.RWMutex
	db    *Database
	hours map[hours.DateHour]RecentHour
}

func NewRecentHours(db *Database) *RecentHours {
	return &RecentHours{
		db:    db,
		hours: make(map[hours.DateHour]RecentHour),
	}
}

func (h *RecentHours) Get(hour hours.DateHour) (RecentHour, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	history, ok := h.hours[hour]
	return history, ok
}

func (h *RecentHours) Reload(ctx context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	from := hours.FromNow().Sub(24)

	tsRows, err := h.db.GetTimeSeriesFrom(ctx, from)
	if err != nil {
		return fmt.Errorf("reloading recent hours: %w", err)
	}

	faRows, err := h.db.GetFaSnapshotFrom(ctx, from)
	if err != nil {
		return fmt.Errorf("reloading recent hours: %w", err)
	}

	h.hours = make(map[hours.DateHour]RecentHour)

	for _, ts := range tsRows {
		h.hours[ts.When] = RecentHour{When: ts.When, Ts: ts}
	}

	for _, fa := range faRows {
		hour := fa.When
		entry, exists := h.hours[hour]
		if !exists {
			entry = RecentHour{}
		}
		entry.Fa = fa
		h.hours[hour] = entry
	}

	return nil
}
