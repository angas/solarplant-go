package optimize

import (
	"reflect"
	"slices"
	"testing"
)

func TestPermutation(t *testing.T) {
	strategies := slices.Collect(permute(1))
	if len(strategies) != 4 {
		t.Errorf("Expected 4 permutations, got %d", len(strategies))
	}
	expected := [][]Strategy{{StrategyDefault}, {StrategyPreserve}, {StrategyCharge}, {StrategyDischarge}}
	if !reflect.DeepEqual(strategies, expected) {
		t.Errorf("Expected permutations to be %v, got %v", expected, strategies)
	}

	strategies = slices.Collect(permute(2))
	if len(strategies) != 16 {
		t.Errorf("Expected 16 permutations, got %d", len(strategies))
	}
	expected = [][]Strategy{
		{StrategyDefault, StrategyDefault},
		{StrategyDefault, StrategyPreserve},
		{StrategyDefault, StrategyCharge},
		{StrategyDefault, StrategyDischarge},
		{StrategyPreserve, StrategyDefault},
		{StrategyPreserve, StrategyPreserve},
		{StrategyPreserve, StrategyCharge},
		{StrategyPreserve, StrategyDischarge},
		{StrategyCharge, StrategyDefault},
		{StrategyCharge, StrategyPreserve},
		{StrategyCharge, StrategyCharge},
		{StrategyCharge, StrategyDischarge},
		{StrategyDischarge, StrategyDefault},
		{StrategyDischarge, StrategyPreserve},
		{StrategyDischarge, StrategyCharge},
		{StrategyDischarge, StrategyDischarge},
	}
	if !reflect.DeepEqual(strategies, expected) {
		t.Errorf("Expected permutations to be %v, got %v", expected, strategies)
	}
}

func TestPermutationEmpty(t *testing.T) {
	for _, slots := range []int{-1, 0} {
		got := slices.Collect(permute(slots))
		if !reflect.DeepEqual(got, [][]Strategy{{}}) {
			t.Errorf("permute(%d) = %v, want one non-nil empty slice", slots, got)
		}
	}
}

func TestPermutationEarlyStopAndRestart(t *testing.T) {
	// Enumerating 4^64 candidates is infeasible, but consuming a prefix is cheap.
	seq := permute(64)
	for range 2 {
		count := 0
		for candidate := range seq {
			want := make([]Strategy, 64)
			want[63] = Strategy(count)
			if !slices.Equal(candidate, want) {
				t.Fatalf("candidate %d = %v, want %v", count, candidate, want)
			}
			count++
			if count == 2 {
				break
			}
		}
		if count != 2 {
			t.Fatalf("got %d candidates, want 2", count)
		}
	}
}

func TestPermutationOwnership(t *testing.T) {
	var first []Strategy
	for candidate := range permute(1) {
		if first == nil {
			first = candidate
			first[0] = StrategyCharge
			continue
		}
		if candidate[0] != StrategyPreserve {
			t.Fatalf("mutating an earlier candidate changed enumeration: %v", candidate)
		}
		if first[0] != StrategyCharge {
			t.Fatalf("iteration overwrote an earlier candidate: %v", first)
		}
		break
	}
}
