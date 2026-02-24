package optimize

import (
	"github.com/angas/solarplant-go/config"
)

type Battery struct {
	config.AppConfigBatterySpec
	CurrentLevel float64 // Current battery level in percentage
}

// Returns the battery level in kWh for a given percentage
func (b Battery) ToKWh(percentage float64) float64 {
	return percentage / 100.0 * b.Capacity
}

// Returns the battery level in percentage for a given kWh
func (b Battery) ToPercentage(kWh float64) float64 {
	return kWh / b.Capacity * 100.0
}

// Returns the space available for charging in kWh
func (b Battery) AvailableCapacity() float64 {
	return b.ToKWh(b.MaxLevel) - b.ToKWh(b.CurrentLevel)
}

// Returns the space available for discharging in kWh
func (b Battery) RemainingCapacity() float64 {
	return b.ToKWh(b.CurrentLevel) - b.ToKWh(b.MinLevel)
}

// Calculates and updates battery level for a given load over a time duration, returns diff in kWh.
// loadKW is the power in kW, slotHours is the duration of the time slot in hours (e.g. 0.25 for 15 min).
func (b *Battery) UpdateLevelForDuration(loadKW float64, slotHours float64) float64 {
	loadKWh := loadKW * slotHours
	return b.UpdateLevel(loadKWh)
}

// Calculates and updates battery level for a given balance, returns diff in kWh
func (b *Battery) UpdateLevel(load float64 /* Charge or discharge load in kW */) float64 {
	var newLvlKWh float64
	oldLvlKWh := b.ToKWh(b.CurrentLevel)
	if load > 0 {
		newLvlKWh = min(b.ToKWh(b.MaxLevel), oldLvlKWh+load)
	} else {
		newLvlKWh = max(b.ToKWh(b.MinLevel), oldLvlKWh+load)
	}

	b.CurrentLevel = b.ToPercentage(newLvlKWh)

	return newLvlKWh - oldLvlKWh
}
