package types

import (
	"context"

	"github.com/angas/solarplant-go/timex"
)

type EnergyPrice struct {
	StartAt timex.BucketTime
	Price   float64 // Price in SEK per kWh excluding VAT
}

type EnergyPriceProvider interface {
	GetEnergyPrices(ctx context.Context) ([]EnergyPrice, error)
}
