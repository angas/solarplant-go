package maybe

import (
	"database/sql"
	"testing"
)

func TestFromSQL(t *testing.T) {
	for _, tt := range []struct {
		name        string
		value       sql.Null[float64]
		wantDefault float64
	}{
		{"null", sql.Null[float64]{}, 99},
		{"zero", sql.Null[float64]{V: 0, Valid: true}, 0},
		{"negative price", sql.Null[float64]{V: -0.25, Valid: true}, -0.25},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := FromSQL(tt.value)
			if got.IsValid() != tt.value.Valid || got.ValueOrDefault(99) != tt.wantDefault {
				t.Fatalf("FromSQL(%v) = %v, fallback = %v", tt.value, got, got.ValueOrDefault(99))
			}
		})
	}
}

func TestMapCloudCover(t *testing.T) {
	for _, tt := range []struct {
		name  string
		value sql.Null[int16]
		want  Maybe[uint8]
	}{
		{"null", sql.Null[int16]{}, None[uint8]()},
		{"invalid with stale value", sql.Null[int16]{V: 8}, None[uint8]()},
		{"clear sky", sql.Null[int16]{V: 0, Valid: true}, Some(uint8(0))},
		{"overcast", sql.Null[int16]{V: 8, Valid: true}, Some(uint8(8))},
	} {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			got := FromSQL(tt.value).Map(func(value int16) uint8 {
				calls++
				return uint8(value)
			})
			wantCalls := 0
			if tt.value.Valid {
				wantCalls = 1
			}
			if got != tt.want || calls != wantCalls {
				t.Fatalf("Map() = %v, calls = %d; want %v, calls = %d", got, calls, tt.want, wantCalls)
			}
		})
	}
}
