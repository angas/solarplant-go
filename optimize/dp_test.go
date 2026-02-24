package optimize

import (
	"math"
	"testing"
	"time"

	"github.com/angas/solarplant-go/config"
)

func testBattery() Battery {
	return Battery{
		CurrentLevel: 10.0,
		AppConfigBatterySpec: config.AppConfigBatterySpec{
			Capacity:         10.0,
			MinLevel:         10.0,
			MaxLevel:         100.0,
			MaxChargeRate:    3.0,
			MaxDischargeRate: 3.0,
			DegradationCost:  0.1,
		},
	}
}

func testInput(forecasts []Forecast) Input {
	return Input{
		GridMaxPower:       25.0,
		EnergyTax:          0.0,
		EnergyTaxReduction: 0.0,
		GridBenefit:        0.0,
		Battery:            testBattery(),
		Forecast:           forecasts,
	}
}

// TestDPvsBruteForce cross-validates DP against brute-force for small inputs.
func TestDPvsBruteForce(t *testing.T) {
	testCases := []struct {
		name     string
		forecast []Forecast
	}{
		{
			name: "1 slot",
			forecast: []Forecast{
				{EnergyPrice: 1.0, EnergyBalance: 2.0},
			},
		},
		{
			name: "3 slots simple",
			forecast: []Forecast{
				{EnergyPrice: -2.0, EnergyBalance: 2.0},
				{EnergyPrice: 0.0, EnergyBalance: 2.0},
				{EnergyPrice: 2.0, EnergyBalance: -2.0},
			},
		},
		{
			name: "4 slots varied",
			forecast: []Forecast{
				{EnergyPrice: 0.5, EnergyBalance: 1.0},
				{EnergyPrice: 3.0, EnergyBalance: -1.0},
				{EnergyPrice: -1.0, EnergyBalance: 3.0},
				{EnergyPrice: 2.0, EnergyBalance: 0.0},
			},
		},
		{
			name: "5 slots negative prices",
			forecast: []Forecast{
				{EnergyPrice: -1.0, EnergyBalance: 0.0},
				{EnergyPrice: -2.0, EnergyBalance: 0.0},
				{EnergyPrice: 3.0, EnergyBalance: 0.0},
				{EnergyPrice: 4.0, EnergyBalance: 0.0},
				{EnergyPrice: 5.0, EnergyBalance: 0.0},
			},
		},
		{
			name: "6 slots",
			forecast: []Forecast{
				{EnergyPrice: 1.0, EnergyBalance: 2.0},
				{EnergyPrice: 2.0, EnergyBalance: -1.0},
				{EnergyPrice: -1.0, EnergyBalance: 3.0},
				{EnergyPrice: 0.5, EnergyBalance: 0.5},
				{EnergyPrice: 3.0, EnergyBalance: -2.0},
				{EnergyPrice: -0.5, EnergyBalance: 1.0},
			},
		},
		{
			name: "8 slots",
			forecast: []Forecast{
				{EnergyPrice: 0.1, EnergyBalance: 1.0},
				{EnergyPrice: 0.2, EnergyBalance: -1.0},
				{EnergyPrice: 0.3, EnergyBalance: 2.0},
				{EnergyPrice: 0.4, EnergyBalance: -2.0},
				{EnergyPrice: 0.5, EnergyBalance: 1.0},
				{EnergyPrice: 0.6, EnergyBalance: -1.0},
				{EnergyPrice: 0.7, EnergyBalance: 2.0},
				{EnergyPrice: 0.8, EnergyBalance: -2.0},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			input := testInput(tc.forecast)

			dpResult := bestStrategiesDP(input)
			bfResult := bestStrategiesBruteForce(input)

			if !almostEqual(dpResult.Cost, bfResult.Cost) {
				t.Errorf("cost mismatch: DP=%f, BruteForce=%f", dpResult.Cost, bfResult.Cost)
				t.Errorf("DP strategies: %v", dpResult.Strategy)
				t.Errorf("BF strategies: %v", bfResult.Strategy)
			}

			if !almostEqual(dpResult.BatteryLevel, bfResult.BatteryLevel) {
				t.Errorf("battery level mismatch: DP=%f, BruteForce=%f", dpResult.BatteryLevel, bfResult.BatteryLevel)
			}
		})
	}
}

// TestDP15MinSlotDuration verifies DP works correctly at 15-min slot duration.
func TestDP15MinSlotDuration(t *testing.T) {
	input := testInput([]Forecast{
		{EnergyPrice: -2.0, EnergyBalance: 0.5},  // 0.5 kWh in 15 min
		{EnergyPrice: -1.0, EnergyBalance: 0.5},
		{EnergyPrice: 1.0, EnergyBalance: -0.5},
		{EnergyPrice: 2.0, EnergyBalance: -0.5},
	})
	input.SlotDuration = 15 * time.Minute

	result := BestStrategies(input)

	if math.IsInf(result.Cost, 1) {
		t.Fatal("expected finite cost, got infinity")
	}
	if len(result.Strategy) != 4 {
		t.Fatalf("expected 4 strategies, got %d", len(result.Strategy))
	}
}

// TestDP15MinVsBruteForce cross-validates at 15-min granularity.
func TestDP15MinVsBruteForce(t *testing.T) {
	input := testInput([]Forecast{
		{EnergyPrice: -2.0, EnergyBalance: 0.5},
		{EnergyPrice: 0.0, EnergyBalance: 0.5},
		{EnergyPrice: 2.0, EnergyBalance: -0.5},
	})
	input.SlotDuration = 15 * time.Minute

	dpResult := bestStrategiesDP(input)
	bfResult := bestStrategiesBruteForce(input)

	if !almostEqual(dpResult.Cost, bfResult.Cost) {
		t.Errorf("15-min cost mismatch: DP=%f, BruteForce=%f", dpResult.Cost, bfResult.Cost)
	}
}

// TestUpdateLevelForDuration verifies that kW * slotHours = kWh.
func TestUpdateLevelForDuration(t *testing.T) {
	batt := testBattery()
	startLevel := batt.CurrentLevel

	// 4 kW * 0.25 hours = 1 kWh
	diff := batt.UpdateLevelForDuration(4.0, 0.25)
	if !almostEqual(diff, 1.0) {
		t.Errorf("expected diff 1.0 kWh, got %f", diff)
	}

	expectedLevel := startLevel + batt.ToPercentage(1.0)
	if !almostEqual(batt.CurrentLevel, expectedLevel) {
		t.Errorf("expected level %f, got %f", expectedLevel, batt.CurrentLevel)
	}
}

// TestDPBatteryFullEmpty verifies edge cases when battery is full or empty.
func TestDPBatteryFullEmpty(t *testing.T) {
	t.Run("battery full cannot charge", func(t *testing.T) {
		input := testInput([]Forecast{
			{EnergyPrice: -1.0, EnergyBalance: 0.0},
		})
		input.Battery.CurrentLevel = input.Battery.MaxLevel

		result := BestStrategies(input)
		// Should not pick Charge when battery is full
		if result.Strategy[0] == StrategyCharge {
			t.Error("should not pick Charge strategy when battery is full")
		}
	})

	t.Run("battery empty cannot discharge", func(t *testing.T) {
		input := testInput([]Forecast{
			{EnergyPrice: 5.0, EnergyBalance: 0.0},
		})
		input.Battery.CurrentLevel = input.Battery.MinLevel

		result := BestStrategies(input)
		// Should not pick Discharge when battery is empty
		if result.Strategy[0] == StrategyDischarge {
			t.Error("should not pick Discharge strategy when battery is empty")
		}
	})
}

// TestDPEmptyForecast verifies the edge case of an empty forecast.
func TestDPEmptyForecast(t *testing.T) {
	input := testInput([]Forecast{})
	result := BestStrategies(input)

	if result.Cost != 0 {
		t.Errorf("expected 0 cost for empty forecast, got %f", result.Cost)
	}
	if len(result.Strategy) != 0 {
		t.Errorf("expected empty strategy for empty forecast, got %v", result.Strategy)
	}
}

// TestDPLargeSlotCount verifies DP handles many slots efficiently (would be impossible for brute-force).
func TestDPLargeSlotCount(t *testing.T) {
	forecasts := make([]Forecast, 48) // 48 x 15-min slots = 12 hours
	for i := range forecasts {
		forecasts[i] = Forecast{
			EnergyPrice:   float64(i%10) * 0.1,
			EnergyBalance: float64(i%5) * 0.25,
		}
	}
	input := testInput(forecasts)
	input.SlotDuration = 15 * time.Minute

	result := BestStrategies(input)

	if math.IsInf(result.Cost, 1) {
		t.Fatal("expected finite cost for 48-slot input")
	}
	if len(result.Strategy) != 48 {
		t.Fatalf("expected 48 strategies, got %d", len(result.Strategy))
	}
}
