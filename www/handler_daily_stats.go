package www

import (
	"log/slog"
	"net/http"
	"time"

	_ "embed"

	"github.com/angas/solarplant-go/database"
	"github.com/angas/solarplant-go/timex"
	"github.com/angas/solarplant-go/types/maybe"
)

type dailyStatsTemplRow struct {
	Date             string
	AvgCloudCover    maybe.Maybe[float64]
	AvgTemperature   maybe.Maybe[float64]
	AvgPrecipitation maybe.Maybe[float64]
	AvgEnergyPrice   maybe.Maybe[float64]
	TotProduction    maybe.Maybe[float64]
	DiffProduction   maybe.Maybe[float64]
	TotConsumption   maybe.Maybe[float64]
	DiffConsumption  maybe.Maybe[float64]
	TotGridImport    maybe.Maybe[float64]
	TotGridExport    maybe.Maybe[float64]
	TotCashFlow      maybe.Maybe[float64]
	IsToday          bool
}

func NewDailyStatsHandler(logger *slog.Logger, db *database.Database, tm *TemplateManager, loc *time.Location) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")

		rows, err := db.GetDailyStats(r.Context(), 14)
		if err != nil {
			logger.Error("handling daily_stats request", slog.Any("error", err))
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		thisHour := timex.UTCHour()
		templRows := make([]dailyStatsTemplRow, len(rows))
		for i, row := range rows {
			templRows[i] = dailyStatsTemplRow{
				Date:             row.Date,
				AvgCloudCover:    maybe.Some(row.AvgCloudCover),
				AvgTemperature:   maybe.Some(row.AvgTemperature),
				AvgPrecipitation: maybe.Some(row.AvgPrecipitation),
				AvgEnergyPrice:   maybe.Some(row.AvgEnergyPrice),
				TotProduction:    maybe.Some(row.TotProduction),
				DiffProduction:   maybe.Some(row.DiffProduction),
				TotConsumption:   maybe.Some(row.TotConsumption),
				DiffConsumption:  maybe.Some(row.DiffConsumption),
				TotGridImport:    maybe.Some(row.TotGridImport),
				TotGridExport:    maybe.Some(row.TotGridExport),
				TotCashFlow:      maybe.Some(row.TotCashFlow),
				IsToday:          row.Date == thisHour.DateOnlyString(loc),
			}
		}

		if err := tm.ExecuteToWriter("daily_stats.html", templRows, &w); err != nil {
			logger.Error("handling daily_stats request", slog.Any("error", err))
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
}
