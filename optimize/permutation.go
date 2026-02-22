package optimize

import "math"

// Generates all possible permutations (cartesian product) of strategies
// for a given number of hours.
func permute(hours int) [][]Strategy {
	if hours < 1 {
		return [][]Strategy{{}}
	}

	count := int(math.Pow(float64(strategyCount), float64(hours)))
	result := make([][]Strategy, count)

	for i := range count {
		temp := i
		perm := make([]Strategy, hours)
		for j := hours - 1; j >= 0; j-- {
			perm[j] = Strategy(temp % int(strategyCount))
			temp /= int(strategyCount)
		}

		result[i] = perm
	}

	return result
}
