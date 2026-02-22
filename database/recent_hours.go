package database

import (
	"context"
	"fmt"
	"sync"

	"github.com/angas/solarplant-go/timex"
)

type RecentHour struct {
	Hour timex.BucketTime
	Ts   TimeSeriesRow
	Fa   FaSnapshotRow
}

func (rh RecentHour) Empty() bool {
	return rh.Hour.Empty()
}

// A cache of the most recent hours
type RecentHours struct {
	mu    sync.RWMutex
	db    *Database
	hours map[timex.BucketTime]RecentHour
}

func NewRecentHours(db *Database) *RecentHours {
	return &RecentHours{
		db:    db,
		hours: make(map[timex.BucketTime]RecentHour),
	}
}

func (h *RecentHours) Get(hour timex.BucketTime) RecentHour {
	h.mu.RLock()
	defer h.mu.RUnlock()

	res, ok := h.hours[hour.TruncTo15Min()]
	if !ok {
		return RecentHour{}
	}
	return res
}

func (h *RecentHours) Reload(ctx context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	from := timex.UTC15Min().SubHours(24)
	h.hours = make(map[timex.BucketTime]RecentHour)

	tsRows, err := h.db.GetTimeSeriesFrom(ctx, from)
	if err != nil {
		return fmt.Errorf("reloading recent time series hours: %w", err)
	}

	for _, ts := range tsRows {
		slot := ts.Timestamp.TruncTo15Min()
		h.hours[slot] = RecentHour{Hour: slot, Ts: ts}
	}

	faRows, err := h.db.GetFaSnapshotFrom(ctx, from)
	if err != nil {
		return fmt.Errorf("reloading recent fa snapshot hours: %w", err)
	}

	for _, fa := range faRows {
		slot := fa.Timestamp.TruncTo15Min()
		entry, exists := h.hours[slot]
		if !exists {
			entry = RecentHour{Hour: slot}
		}
		entry.Fa = fa
		h.hours[slot] = entry
	}

	return nil
}

// Range returns min and max hours stored in in the cache.
func (h *RecentHours) Range() []timex.BucketTime {
	h.mu.RLock()
	defer h.mu.RUnlock()

	res := []timex.BucketTime{timex.BucketTime{}, timex.BucketTime{}}
	for _, hour := range h.hours {
		if res[0].Empty() || hour.Hour.Before(res[0]) {
			res[0] = hour.Hour
		}
		if res[1].Empty() || hour.Hour.After(res[1]) {
			res[1] = hour.Hour
		}
	}

	return res
}
