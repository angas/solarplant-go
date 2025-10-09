package timex

import (
	"testing"
	"time"
)

func TestParseBucketTime(t *testing.T) {
	t.Run("successful bucket time parsing", func(t *testing.T) {
		bt, err := ParseBucketTime("2025-10-06T20:00:00Z", BucketSizeHour)
		if err != nil {
			t.Errorf("ParseBucketTime() error = %v", err)
		}
		if bt.Time() != time.Date(2025, 10, 6, 20, 0, 0, 0, time.UTC) {
			t.Errorf("ParseBucketTime() = %v, want %v", bt, time.Date(2025, 10, 6, 20, 0, 0, 0, time.UTC))
		}
	})

	t.Run("unsuccessful bucket time parsing", func(t *testing.T) {
		_, err := ParseBucketTime("2025-10-06T20:0:01Z", BucketSizeHour)
		if err == nil {
			t.Error("expected error, got nil")
		}
	})
}

func TestAddAndSubtractHours(t *testing.T) {
	t.Run("add hours", func(t *testing.T) {
		bt, err := ParseBucketTime("2025-10-06T20:00:00Z", BucketSizeHour)
		if err != nil {
			t.Errorf("ParseBucketTime() error = %v", err)
		}
		bt = bt.AddHours(1)
		expect := time.Date(2025, 10, 6, 21, 0, 0, 0, time.UTC)
		if bt.Time() != expect {
			t.Errorf("AddHours() = %v, want %v", bt, expect)
		}
	})

	t.Run("subtract hours", func(t *testing.T) {
		bt, err := ParseBucketTime("2025-10-06T20:00:00Z", BucketSizeHour)
		if err != nil {
			t.Errorf("ParseBucketTime() error = %v", err)
		}
		bt = bt.SubHours(1)
		expect := time.Date(2025, 10, 6, 19, 0, 0, 0, time.UTC)
		if bt.Time() != expect {
			t.Errorf("SubHours() = %v, want %v", bt, expect)
		}
	})
}
