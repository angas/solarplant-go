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

func TestTruncTo15Min(t *testing.T) {
	tests := []struct {
		input  string
		expect string
	}{
		{"2025-10-06T20:00:00Z", "2025-10-06T20:00:00Z"},
		{"2025-10-06T20:07:00Z", "2025-10-06T20:00:00Z"},
		{"2025-10-06T20:14:00Z", "2025-10-06T20:00:00Z"},
		{"2025-10-06T20:15:00Z", "2025-10-06T20:15:00Z"},
		{"2025-10-06T20:29:00Z", "2025-10-06T20:15:00Z"},
		{"2025-10-06T20:30:00Z", "2025-10-06T20:30:00Z"},
		{"2025-10-06T20:44:00Z", "2025-10-06T20:30:00Z"},
		{"2025-10-06T20:45:00Z", "2025-10-06T20:45:00Z"},
		{"2025-10-06T20:59:00Z", "2025-10-06T20:45:00Z"},
	}
	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			inputTime, _ := time.Parse(time.RFC3339, tc.input)
			bt := BucketTime(inputTime)
			result := bt.TruncTo15Min()
			expectTime, _ := time.Parse(time.RFC3339, tc.expect)
			if result.Time() != expectTime {
				t.Errorf("TruncTo15Min(%s) = %v, want %v", tc.input, result, expectTime)
			}
		})
	}
}

func TestAdd15Min(t *testing.T) {
	bt, err := ParseBucketTime("2025-10-06T20:00:00Z", BucketSize15Minutes)
	if err != nil {
		t.Fatalf("ParseBucketTime() error = %v", err)
	}

	result := bt.Add15Min(1)
	expect := time.Date(2025, 10, 6, 20, 15, 0, 0, time.UTC)
	if result.Time() != expect {
		t.Errorf("Add15Min(1) = %v, want %v", result, expect)
	}

	result = bt.Add15Min(4)
	expect = time.Date(2025, 10, 6, 21, 0, 0, 0, time.UTC)
	if result.Time() != expect {
		t.Errorf("Add15Min(4) = %v, want %v", result, expect)
	}

	result = bt.Add15Min(-1)
	expect = time.Date(2025, 10, 6, 19, 45, 0, 0, time.UTC)
	if result.Time() != expect {
		t.Errorf("Add15Min(-1) = %v, want %v", result, expect)
	}
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
