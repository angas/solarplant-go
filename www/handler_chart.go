package www

import (
	"encoding/json"
	"log/slog"
	"math"
	"net/http"

	"github.com/angas/solarplant-go/database"
	"github.com/angas/solarplant-go/timex"
	"github.com/angas/solarplant-go/www/chartjs"
)

func NewChartHandler(logger *slog.Logger, db *database.Database) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		midnight := timex.CETMidnight()

		timeSeries, err := db.GetTimeSeriesFrom(r.Context(), midnight)
		if err != nil {
			logger.Error("handling chart request", slog.Any("error", err))
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		findTs := func(bt timex.BucketTime) database.TimeSeriesRow {
			for _, t := range timeSeries {
				if t.Timestamp == bt {
					return t
				}
			}
			return database.TimeSeriesRow{}
		}

		energyPrice, err := db.GetHourlyAvgEnergyPriceFrom(r.Context(), midnight)
		if err != nil {
			logger.Error("handling chart request", slog.Any("error", err))
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		findEp := func(bt timex.BucketTime) database.EnergyPriceRow {
			for _, ep := range energyPrice {
				if ep.StartAt == bt {
					return ep
				}
			}
			return database.EnergyPriceRow{}
		}

		// Chart 1: Battery Level and Energy Price
		chart1 := chartjs.NewChart("")
		for i := range chartjs.NoOfHours {
			if ts := findTs(midnight.AddHours(int64(i))); ts.Timestamp.Empty() {
				chart1.Data.Datasets[0].Data[i] = nil
			} else {
				chart1.Data.Datasets[0].Data[i] = chartjs.FixedFloat64(ts.BatteryLevel, 2)
			}
			if ep := findEp(midnight.AddHours(int64(i))); ep.StartAt.Empty() {
				chart1.Data.Datasets[1].Data[i] = nil
			} else {
				chart1.Data.Datasets[1].Data[i] = chartjs.FixedFloat64(ep.Price, 2)
			}
		}
		chart1.Options.Scales["YAxis1"] = chart1.Options.Scales["YAxis1"].
			WithTitle("Battery Level (%)").
			WithMinAndMax(0, 100)
		chart1.Options.Scales["YAxis2"] = chart1.Options.Scales["YAxis2"].
			WithTitle("Energy Price (SEK/kWh)")

		// Chart 2: Energy Production and Consumption
		chart2 := chartjs.NewChart("")
		maxVal := 0.0
		for i := range chartjs.NoOfHours {
			if ts := findTs(midnight.AddHours(int64(i))); ts.Timestamp.Empty() {
				chart2.Data.Datasets[0].Data[i] = nil
				chart2.Data.Datasets[1].Data[i] = nil
			} else {
				maxVal = max(maxVal, ts.Production, ts.Consumption)
				chart2.Data.Datasets[0].Data[i] = chartjs.FixedFloat64(ts.Production, 2)
				chart2.Data.Datasets[1].Data[i] = chartjs.FixedFloat64(ts.Consumption, 2)
			}
		}
		maxVal = math.Ceil(maxVal/2) * 2 // Round up to nearest even number
		chart2.Options.Scales["YAxis1"] = chart2.Options.Scales["YAxis1"].
			WithTitle("Energy Produced (kWh)").
			WithMinAndMax(0, maxVal)
		chart2.Options.Scales["YAxis2"] = chart1.Options.Scales["YAxis2"].
			WithTitle("Energy Consumed (kWh)").
			WithMinAndMax(0, maxVal)

		w.Header().Set("Content-Type", "application/json")
		err = json.NewEncoder(w).Encode([]chartjs.Chart{chart1, chart2})
		if err != nil {
			logger.Error("handling chart request", slog.Any("error", err))
			http.Error(w, "unable to encode data points", http.StatusInternalServerError)
			return
		}
	}
}
