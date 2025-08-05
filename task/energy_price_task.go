package task

import (
	"context"
	"log/slog"
	"time"

	"github.com/angas/solarplant-go/database"
	"github.com/angas/solarplant-go/hours"
	"github.com/angas/solarplant-go/types"
)

func NewEnergyPriceTask(logger *slog.Logger, db *database.Database, providers []types.EnergyPriceProvider) func() {
	if len(providers) == 0 {
		panic("no energy price providers configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if needImmediateEnergyPriceUpdate(ctx, db) {
		logger.Info("need an immediate update of energy prices")
		runEnergyPriceTask(logger, db, providers)
	} else {
		logger.Debug("no need for immediate update of energy prices")
	}

	return func() { runEnergyPriceTask(logger, db, providers) }
}

func runEnergyPriceTask(logger *slog.Logger, db *database.Database, providers []types.EnergyPriceProvider) {
	logger.Debug("running energy price task...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var rows []database.EnergyPriceRow
	for _, provider := range providers {
		prices, err := provider.GetEnergyPrices(ctx)
		if err != nil {
			logger.Error("energy price task error, fetching energy prices", slog.Any("error", err))
		} else {
			rows = make([]database.EnergyPriceRow, len(prices))
			for i, ep := range prices {
				logger.Debug("energy price", slog.String("hour", ep.Hour.String()), slog.Float64("price", ep.Price))
				rows[i] = database.EnergyPriceRow{When: ep.Hour, Price: ep.Price}
			}
			break
		}
	}

	if len(rows) == 0 {
		logger.Error("energy price task error, no prices fetched")
		return
	}

	err := db.SaveEnergyPrices(ctx, rows)
	if err != nil {
		logger.Error("energy price task error", slog.Any("error", err))
		return
	}

	logger.Info("energy price task done", slog.Int("noOfHoursUpdated", len(rows)))
}

func needImmediateEnergyPriceUpdate(ctx context.Context, db *database.Database) bool {
	dh := hours.FromNow().Add(12)
	if _, err := db.GetEnergyPrice(ctx, dh); err != nil {
		return true
	}
	return false
}
