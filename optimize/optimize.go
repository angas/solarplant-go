package optimize

import (
	"math"
	"time"

	"github.com/angas/solarplant-go/calc"
)

type Forecast struct {
	EnergyPrice   float64 // Price of energy per kWh
	EnergyBalance float64 // Difference between produced and consumed power (kWh) not including the battery effect
}

type Input struct {
	Battery            Battery
	GridMaxPower       float64       // Maximum power to and from grid in kW
	EnergyTax          float64       // Energy tax in SEK/kWh including VAT (energiskatt)
	EnergyTaxReduction float64       // Energy tax reduction in SEK/kWh (skattereduktion)
	GridBenefit        float64       // Grid benefit in SEK/kWh (nätnytta)
	SlotDuration       time.Duration // Duration of each time slot (defaults to 1 hour if zero)
	Forecast           []Forecast
}

func (i *Input) slotHours() float64 {
	if i.SlotDuration == 0 {
		return 1.0
	}
	return i.SlotDuration.Hours()
}

func (i *Input) BuyPrice(price float64, kWh float64) float64 {
	return calc.BuyPrice(kWh, price, i.EnergyTax, i.GridBenefit)
}

func (i *Input) SellPrice(price float64, kWh float64) float64 {
	return calc.SellPrice(kWh, price, i.EnergyTaxReduction)
}

type Output struct {
	Cost         float64    // Total cost of energy
	BatteryLevel float64    // Final battery level in percentage
	Strategy     []Strategy // Optimal strategy for each hour in the forecast
}

// BestStrategies finds the optimal strategy for each time slot using dynamic programming.
func BestStrategies(input Input) Output {
	return bestStrategiesDP(input)
}

// bestStrategiesBruteForce evaluates permutations lazily and retains the lowest-cost one.
// Kept for cross-validation in tests.
func bestStrategiesBruteForce(input Input) Output {
	best := Output{Cost: math.Inf(1), Strategy: []Strategy{}}
	for p := range permute(len(input.Forecast)) {
		cost, battLvl := costForPermutation(input, p)
		if cost < best.Cost {
			best = Output{Cost: cost, BatteryLevel: battLvl, Strategy: p}
		}
	}

	return best
}

// slotResult holds the outcome of simulating a single slot.
type slotResult struct {
	cost         float64
	disqualified bool
}

// simulateSlot simulates a single time slot for a given strategy.
// It mutates batt in place and returns the cost incurred and whether the strategy was disqualified.
func simulateSlot(input *Input, batt *Battery, strategy Strategy, forecast Forecast, slotHours float64) slotResult {
	price := forecast.EnergyPrice
	balance := forecast.EnergyBalance

	switch strategy {
	case StrategyDefault:
		battDiffKWh := batt.UpdateLevelForDuration(balance/slotHours, slotHours)
		buyKwh := max(0.0, battDiffKWh-balance)
		cost := 0.0
		if buyKwh > 0 {
			cost += input.BuyPrice(price, buyKwh)
		}
		sellKwh := max(0.0, balance-battDiffKWh)
		if sellKwh > 0 {
			cost -= input.SellPrice(price, sellKwh)
		}
		cost += batt.DegradationCost * math.Abs(battDiffKWh)
		return slotResult{cost: cost}

	case StrategyPreserve:
		cost := 0.0
		if balance < 0 {
			cost += input.BuyPrice(price, -balance)
		}
		if balance > 0 {
			cost -= input.SellPrice(price, balance)
		}
		return slotResult{cost: cost}

	case StrategyCharge:
		if batt.AvailableCapacity() <= 0 {
			return slotResult{disqualified: true}
		}
		battDiffKWh := batt.UpdateLevelForDuration(batt.MaxChargeRate, slotHours)
		buyKwh := max(0.0, battDiffKWh-balance)
		if buyKwh <= 0 {
			return slotResult{disqualified: true}
		}
		cost := input.BuyPrice(price, buyKwh)
		cost += batt.DegradationCost * math.Abs(battDiffKWh)
		return slotResult{cost: cost}

	case StrategyDischarge:
		if batt.RemainingCapacity() <= 0 {
			return slotResult{disqualified: true}
		}
		battDiffKWh := batt.UpdateLevelForDuration(-batt.MaxDischargeRate, slotHours)
		sellKwh := max(0.0, balance-battDiffKWh)
		if sellKwh <= 0 {
			return slotResult{disqualified: true}
		}
		cost := -input.SellPrice(price, sellKwh)
		cost += batt.DegradationCost * math.Abs(battDiffKWh)
		return slotResult{cost: cost}
	}

	return slotResult{disqualified: true}
}

// Calculates the total cost for a given permutation of strategies,
// i.e. how much money is spent (or earned) to/from the grid.
// Also returns new battery level in percentage.
func costForPermutation(input Input, permutation []Strategy) (float64, float64) {
	batt := input.Battery
	totCost := 0.0
	slotHours := input.slotHours()

	for i, strategy := range permutation {
		result := simulateSlot(&input, &batt, strategy, input.Forecast[i], slotHours)
		if result.disqualified {
			return math.Inf(1), batt.CurrentLevel
		}
		totCost += result.cost
	}

	return totCost, batt.CurrentLevel
}
