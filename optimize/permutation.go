package optimize

import "math"

// Generates all possible permutations of strategies
// for a given number of hours.
func permute(hours int) [][]Strategy {
	if hours < 1 || hours > 24 {
		return [][]Strategy{{}}
	}

	count := int(math.Pow(float64(strategyCount), float64(hours)))
	result := make([][]Strategy, count)

	for i := range count {
		perm := make([]Strategy, hours)
		temp := i
		for j := hours - 1; j >= 0; j-- {
			perm[j] = Strategy(temp % int(strategyCount))
			temp /= int(strategyCount)
		}

		result[i] = perm
	}

	return result
}
