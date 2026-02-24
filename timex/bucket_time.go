package timex

import (
	"fmt"
	"time"
)

type BucketSize time.Duration

const (
	BucketSizeNone      = BucketSize(0)
	BucketSize15Minutes = BucketSize(time.Minute * 15)
	BucketSizeHour      = BucketSize(time.Hour)
	BucketSizeDay       = BucketSize(time.Hour * 24)
)

// A BucketTime represents a time bucket (with a specific granularity).
// It could be a starting point for a time period, such as a 15-minute energy price interval
// or an ending point of an hour of produced energy.
type BucketTime time.Time

// Parses a string representation of a Bucket that must be in RFC3339 format.
func ParseBucketTime(s string, g BucketSize) (BucketTime, error) {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return BucketTime{}, fmt.Errorf("parsing bucket time string (%s): %w", s, err)
	}
	bt := BucketTime(t.UTC())
	if !bt.IsValid(g) {
		return BucketTime{}, fmt.Errorf("bucket time (%s) does not match granularity (%s)", s, time.Duration(g).String())
	}

	return bt, nil
}

func UTCHour() BucketTime {
	now := time.Now().UTC()
	return BucketTime(time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), 0, 0, 0, time.UTC))
}

func UTC15Min() BucketTime {
	now := time.Now().UTC()
	minute := now.Minute() / 15 * 15
	return BucketTime(time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), minute, 0, 0, time.UTC))
}

func UTCMidnight() BucketTime {
	now := time.Now().UTC()
	return BucketTime(time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC))
}

func LocalMidnight(loc *time.Location) BucketTime {
	now := time.Now()
	return BucketTime(time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc).UTC())
}

// Returns a string representation of the Bucket in RFC3339 format.
func (bt BucketTime) String() string {
	return time.Time(bt).Format(time.RFC3339)
}

// Returns a BucketTime truncated to the start of the current hour in UTC.
func (bt BucketTime) TruncToHour() BucketTime {
	t := bt.Time()
	return BucketTime(time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), 0, 0, 0, time.UTC))
}

// Returns a BucketTime truncated to the start of the current 15-minute boundary in UTC.
func (bt BucketTime) TruncTo15Min() BucketTime {
	t := bt.Time()
	minute := t.Minute() / 15 * 15
	return BucketTime(time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), minute, 0, 0, time.UTC))
}

// Returns a BucketTime truncated to the start of the day in UTC.
func (bt BucketTime) TruncToMidnight() BucketTime {
	t := bt.Time()
	return BucketTime(time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC))
}

// Returns the time.Time representation of the Bucket.
func (bt BucketTime) Time() time.Time {
	return time.Time(bt)
}

func (bt BucketTime) Add(d time.Duration) BucketTime {
	return BucketTime(time.Time(bt).Add(d))
}

func (bt BucketTime) AddHours(h int64) BucketTime {
	return bt.Add(time.Duration(h) * time.Hour)
}

func (bt BucketTime) Add15Min(n int64) BucketTime {
	return bt.Add(time.Duration(n) * 15 * time.Minute)
}

func (bt BucketTime) SubHours(h int64) BucketTime {
	return bt.Add(-time.Duration(h) * time.Hour)
}

func (bt BucketTime) Empty() bool {
	return bt.Time().Equal(time.Time{})
}

func (bt BucketTime) Before(b BucketTime) bool {
	return bt.Time().Before(b.Time())
}

func (bt BucketTime) After(b BucketTime) bool {
	return bt.Time().After(b.Time())
}

func (bt BucketTime) DateOnlyString(loc *time.Location) string {
	return bt.Time().In(loc).Format("2006-01-02")
}

func (bt BucketTime) IsValid(g BucketSize) bool {
	if bt.Empty() {
		return false
	}

	t := bt.Time()

	switch g {
	case BucketSizeNone:
		return true
	case BucketSizeHour:
		return t.Minute() == 0 && t.Second() == 0 && t.Second() == 0 && t.Nanosecond() == 0
	case BucketSize15Minutes:
		return t.Minute()%15 == 0 && t.Second() == 0 && t.Nanosecond() == 0
	default:
		return false
	}
}
