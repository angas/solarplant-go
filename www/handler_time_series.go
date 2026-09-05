package www

import (
	"log/slog"
	"net/http"
	"slices"
	"time"

	_ "embed"

	"github.com/angas/solarplant-go/database"
	"github.com/angas/solarplant-go/timex"
	"github.com/angas/solarplant-go/types/maybe"
)

type timeSeriesTemplRow struct {
	Timestamp            timex.BucketTime
	CloudCover           maybe.Maybe[uint8]
	Temperature          maybe.Maybe[float64]
	Precipitation        maybe.Maybe[float64]
	EnergyPrice          maybe.Maybe[float64]
	Production           maybe.Maybe[float64]
	ProductionEstimated  maybe.Maybe[float64]
	Consumption          maybe.Maybe[float64]
	ConsumptionEstimated maybe.Maybe[float64]
	BatteryLevel         maybe.Maybe[float64]
	BatteryNetLoad       maybe.Maybe[float64]
	GridExport           maybe.Maybe[float64]
	GridImport           maybe.Maybe[float64]
	CashFlow             maybe.Maybe[float64]
	Strategy             maybe.Maybe[string]
	ComparedToThisSlot   int
	ComparedToThisHour   int
	HourRowSpan          int // >0 on the first row of each hour group (rendered with rowspan); 0 on subsequent rows
}

func NewTimeSeriesHandler(logger *slog.Logger, db *database.Database, tm *TemplateManager, recentHours *database.RecentHours) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")

		thisSlot := timex.UTC15Min()
		var startSlot timex.BucketTime
		if thisSlot.Time().Hour() < 12 {
			startSlot = thisSlot.SubHours(12)
		} else {
			startSlot = timex.UTCMidnight()
		}

		var rows []timeSeriesTemplRow
		slot := startSlot

		// Historical rows: iterate by 15-min slot, tolerating gaps (e.g. old hourly data)
		endSlot := timex.UTC15Min()
		for !slot.After(endSlot) {
			recentSlot := recentHours.Get(slot)
			if !recentSlot.Empty() {
				row := timeSeriesTemplRow{
					Timestamp:            slot,
					CloudCover:           maybe.Some(recentSlot.Ts.CloudCover),
					Temperature:          maybe.Some(recentSlot.Ts.Temperature),
					Precipitation:        maybe.Some(recentSlot.Ts.Precipitation),
					EnergyPrice:          maybe.Some(recentSlot.Ts.EnergyPriceAvg),
					Production:           maybe.Some(recentSlot.Ts.Production),
					ProductionEstimated:  maybe.Some(recentSlot.Ts.ProductionEstimated),
					Consumption:          maybe.Some(recentSlot.Ts.Consumption),
					ConsumptionEstimated: maybe.Some(recentSlot.Ts.ConsumptionEstimated),
					GridExport:           maybe.Some(recentSlot.Ts.GridExport),
					GridImport:           maybe.Some(recentSlot.Ts.GridImport),
					BatteryLevel:         maybe.Some(recentSlot.Ts.BatteryLevel),
					BatteryNetLoad:       maybe.Some(recentSlot.Ts.BatteryNetLoad),
					CashFlow:             maybe.Some(recentSlot.Ts.CashFlow),
					Strategy:             maybe.Some(recentSlot.Ts.Strategy),
					ComparedToThisSlot:   slot.Time().Compare(thisSlot.Time()),
				}
				rows = append(rows, row)
			}
			slot = slot.Add15Min(1)
		}

		// Append forecast data (already at 15-min granularity from planning)
		if len(rows) > 0 {
			from := rows[len(rows)-1].Timestamp.Add(15 * time.Minute)

			forecast, err := db.GetDetailedPlanningFrom(r.Context(), from)
			if err != nil {
				logger.Error("fetching detailed planning", slog.Any("error", err))
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			for _, f := range forecast {
				row := timeSeriesTemplRow{
					Timestamp:            f.StartAt,
					CloudCover:           maybe.FromSQL(f.CloudCover).Map(func(value int16) uint8 { return uint8(value) }),
					Temperature:          maybe.FromSQL(f.Temperature),
					Precipitation:        maybe.FromSQL(f.Precipitation),
					EnergyPrice:          maybe.FromSQL(f.EnergyPrice),
					Production:           maybe.None[float64](),
					ProductionEstimated:  maybe.FromSQL(f.ProductionEstimated),
					Consumption:          maybe.None[float64](),
					ConsumptionEstimated: maybe.FromSQL(f.ConsumptionEstimated),
					GridExport:           maybe.None[float64](),
					GridImport:           maybe.None[float64](),
					BatteryLevel:         maybe.None[float64](),
					BatteryNetLoad:       maybe.None[float64](),
					CashFlow:             maybe.None[float64](),
					Strategy:             maybe.Some(f.Strategy),
					ComparedToThisSlot:   f.StartAt.Time().Compare(thisSlot.Time()),
				}

				rows = append(rows, row)
			}
		}

		// Sort descending (newest first)
		slices.SortFunc(rows, func(i, j timeSeriesTemplRow) int {
			return j.Timestamp.Time().Compare(i.Timestamp.Time())
		})

		// Group rows by hour and assign HourRowSpan on the first row of each group.
		// ComparedToThisHour is set for merged (rowspan) cells; ComparedToThisSlot
		// remains slot-level for per-row cells (Time, Price, Strategy).
		thisHour := thisSlot.TruncToHour().Time()
		for i := 0; i < len(rows); {
			hourKey := rows[i].Timestamp.TruncToHour().String()
			j := i + 1
			for j < len(rows) && rows[j].Timestamp.TruncToHour().String() == hourKey {
				j++
			}
			groupSize := j - i
			rows[i].HourRowSpan = groupSize
			groupHour := rows[i].Timestamp.TruncToHour().Time()
			hourCmp := groupHour.Compare(thisHour)
			for k := i; k < j; k++ {
				rows[k].ComparedToThisHour = hourCmp
			}
			// For single-row groups (old hourly data), use hour-level for slot comparison too
			if groupSize == 1 {
				rows[i].ComparedToThisSlot = hourCmp
			}
			i = j
		}

		if err := tm.ExecuteToWriter("time_series.html", rows, &w); err != nil {
			logger.Error("handling time_series request", slog.Any("error", err))
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
}
