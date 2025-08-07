package optimize

import (
	"reflect"
	"testing"
)

func TestPermutation(t *testing.T) {
	strategies := permute(1)
	if len(strategies) != 4 {
		t.Errorf("Expected 4 permutations, got %d", len(strategies))
	}
	expected := [][]Strategy{{StrategyDefault}, {StrategyPreserve}, {StrategyCharge}, {StrategyDischarge}}
	if !reflect.DeepEqual(strategies, expected) {
		t.Errorf("Expected permutations to be %v, got %v", expected, strategies)
	}

	strategies = permute(2)
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
