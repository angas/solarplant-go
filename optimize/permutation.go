package optimize

import (
	"iter"
	"slices"
)

// permute yields the cartesian product of strategies for the given slot count.
// Each yielded slice is independently owned. Non-positive counts yield one empty
// slice, and each iteration starts again from the all-default combination.
func permute(slots int) iter.Seq[[]Strategy] {
	return func(yield func([]Strategy) bool) {
		perm := make([]Strategy, max(0, slots))
		for {
			if !yield(slices.Clone(perm)) {
				return
			}

			// Increment from the last slot, carrying into earlier slots as needed.
			i := len(perm) - 1
			for i >= 0 && perm[i] == strategyCount-1 {
				perm[i] = StrategyDefault
				i--
			}
			if i < 0 {
				return
			}
			perm[i]++
		}
	}
}
