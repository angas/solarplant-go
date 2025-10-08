package task

import (
	"context"
	"log/slog"
	"time"

	"github.com/angas/solarplant-go/database"
	"github.com/angas/solarplant-go/timex"
	"github.com/angas/solarplant-go/types"
)

func NewEnergyPriceTask(logger *slog.Logger, db *database.Database, providers []types.EnergyPriceProvider) func() {
	if len(providers) == 0 {
		panic("no energy price providers configured")
	}

	ctx := context.Background()
	if needImmediateEnergyPriceUpdate(ctx, db) {
		logger.InfoContext(ctx, "need an immediate update of energy prices")
		runEnergyPriceTask(ctx, logger, db, providers)
	} else {
		logger.DebugContext(ctx, "no need for immediate update of energy prices")
	}

	return func() { runEnergyPriceTask(ctx, logger, db, providers) }
}

func runEnergyPriceTask(ctx context.Context, logger *slog.Logger, db *database.Database, providers []types.EnergyPriceProvider) {
	logger.DebugContext(ctx, "running energy price task...")

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	var rows []database.EnergyPriceRow
	for _, provider := range providers {
		prices, err := provider.GetEnergyPrices(ctx)
		if err != nil {
			logger.ErrorContext(ctx, "energy price task error, fetching energy prices", slog.Any("error", err))
		} else {
			rows = make([]database.EnergyPriceRow, len(prices))
			for i, ep := range prices {
				logger.DebugContext(ctx, "energy price", slog.String("startAt", ep.StartAt.String()), slog.Float64("price", ep.Price))
				rows[i] = database.EnergyPriceRow{StartAt: ep.StartAt, Price: ep.Price}
			}
			break
		}
	}

	if len(rows) == 0 {
		logger.ErrorContext(ctx, "energy price task error, no prices fetched")
		return
	}

	err := db.SaveEnergyPrices(ctx, rows)
	if err != nil {
		logger.ErrorContext(ctx, "energy price task error", slog.Any("error", err))
		return
	}

	logger.InfoContext(ctx, "energy price task done", slog.Int("noOfHoursUpdated", len(rows)))
}

func needImmediateEnergyPriceUpdate(ctx context.Context, db *database.Database) bool {
	startAt := timex.UTCMidnight().AddHours(12)
	rows, err := db.GetEnergyPriceFrom(ctx, startAt)
	if err != nil || len(rows) == 0 {
		return true
	}
	return false
}
