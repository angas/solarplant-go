package optimize

import "math"

// bestStrategiesDP uses dynamic programming to find the optimal strategy sequence.
// It discretizes the battery level into 1% steps and evaluates all strategies at each
// slot for each reachable level, tracking minimum-cost paths.
// Complexity: O(slots x levels x strategies) -- sub-millisecond for typical inputs.
func bestStrategiesDP(input Input) Output {
	slots := len(input.Forecast)
	if slots == 0 {
		return Output{Cost: 0, Strategy: []Strategy{}}
	}

	slotHours := input.slotHours()
	batt := input.Battery
	minLvl := batt.MinLevel
	maxLvl := batt.MaxLevel

	// Discretize battery levels to 1% steps
	levelCount := int(maxLvl-minLvl) + 1
	levelToIdx := func(level float64) int {
		idx := int(math.Round(level - minLvl))
		if idx < 0 {
			return 0
		}
		if idx >= levelCount {
			return levelCount - 1
		}
		return idx
	}
	idxToLevel := func(idx int) float64 {
		return minLvl + float64(idx)
	}

	inf := math.Inf(1)

	// dp[levelIdx] = minimum cost to reach this level at the current slot
	// prev[slot][levelIdx] = {fromLevelIdx, strategy} for backtracking
	type backtrack struct {
		fromIdx  int
		strategy Strategy
	}

	dpCur := make([]float64, levelCount)
	dpNext := make([]float64, levelCount)
	prevAll := make([][]backtrack, slots)

	// Initialize: only the starting level is reachable
	for i := range dpCur {
		dpCur[i] = inf
	}
	startIdx := levelToIdx(batt.CurrentLevel)
	dpCur[startIdx] = 0

	for slot := range slots {
		for i := range dpNext {
			dpNext[i] = inf
		}
		prevAll[slot] = make([]backtrack, levelCount)

		for lvlIdx := range levelCount {
			if dpCur[lvlIdx] == inf {
				continue
			}

			for s := range strategyCount {
				// Create a battery copy at this level
				simBatt := Battery{
					AppConfigBatterySpec: batt.AppConfigBatterySpec,
					CurrentLevel:         idxToLevel(lvlIdx),
				}

				result := simulateSlot(&input, &simBatt, s, input.Forecast[slot], slotHours)
				if result.disqualified {
					continue
				}

				newIdx := levelToIdx(simBatt.CurrentLevel)
				newCost := dpCur[lvlIdx] + result.cost

				if newCost < dpNext[newIdx] {
					dpNext[newIdx] = newCost
					prevAll[slot][newIdx] = backtrack{fromIdx: lvlIdx, strategy: s}
				}
			}
		}

		dpCur, dpNext = dpNext, dpCur
	}

	// Find the minimum cost final state
	bestCost := inf
	bestEndIdx := 0
	for i := range levelCount {
		if dpCur[i] < bestCost {
			bestCost = dpCur[i]
			bestEndIdx = i
		}
	}

	if bestCost == inf {
		return Output{Cost: inf, Strategy: make([]Strategy, slots)}
	}

	// Backtrack to reconstruct the strategy sequence
	strategies := make([]Strategy, slots)
	idx := bestEndIdx
	for slot := slots - 1; slot >= 0; slot-- {
		bt := prevAll[slot][idx]
		strategies[slot] = bt.strategy
		idx = bt.fromIdx
	}

	return Output{
		Cost:         bestCost,
		BatteryLevel: idxToLevel(bestEndIdx),
		Strategy:     strategies,
	}
}
