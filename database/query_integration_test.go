package database

import (
	"database/sql"
	"errors"
	"log/slog"
	"slices"
	"testing"
	"time"

	"github.com/angas/solarplant-go/ferroamp"
	"github.com/angas/solarplant-go/timex"
)

func TestCollectionQueries(t *testing.T) {
	db := newQueryTestDatabase(t)
	ctx := t.Context()
	if err := db.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	start := timex.BucketTime(time.Date(2026, 9, 5, 8, 0, 0, 0, time.UTC))
	next := start.Add15Min(1)

	prices := []EnergyPriceRow{{StartAt: start, Price: 1}, {StartAt: next, Price: 3}}
	if err := db.SaveEnergyPrices(ctx, prices); err != nil {
		t.Fatal(err)
	}
	gotPrices, err := db.GetEnergyPriceFrom(ctx, start)
	if err != nil || !slices.Equal(gotPrices, prices) {
		t.Fatalf("prices = %v, error = %v", gotPrices, err)
	}
	averages, err := db.GetHourlyAvgEnergyPriceFrom(ctx, start)
	if err != nil || !slices.Equal(averages, []EnergyPriceRow{{StartAt: start, Price: 2}}) {
		t.Fatalf("hourly prices = %v, error = %v", averages, err)
	}
	average, err := db.GetAvgEnergyPriceForHour(ctx, start)
	if err != nil || average.Price != 2 || average.StartAt != start {
		t.Fatalf("single hourly average = %v, error = %v", average, err)
	}
	if _, err := db.GetAvgEnergyPriceForHour(ctx, start.AddHours(1)); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("empty hourly average error = %v; want sql.ErrNoRows", err)
	}
	for _, query := range []func(timex.BucketTime) ([]EnergyPriceRow, error){
		func(from timex.BucketTime) ([]EnergyPriceRow, error) { return db.GetEnergyPriceFrom(ctx, from) },
		func(from timex.BucketTime) ([]EnergyPriceRow, error) {
			return db.GetHourlyAvgEnergyPriceFrom(ctx, from)
		},
	} {
		empty, err := query(start.AddHours(1))
		if err != nil || empty == nil || len(empty) != 0 {
			t.Fatalf("empty prices = %#v, error = %v; want a non-nil empty slice", empty, err)
		}
	}

	forecast := EnergyForecastRow{StartAt: start, Production: 3, Consumption: 5}
	if err := db.SaveEnergyForecast(ctx, []EnergyForecastRow{forecast}); err != nil {
		t.Fatal(err)
	}
	gotForecast, err := db.GetEnergyForecastFrom(ctx, start)
	if err != nil || !slices.Equal(gotForecast, []EnergyForecastRow{forecast}) {
		t.Fatalf("energy forecast = %v, error = %v", gotForecast, err)
	}
	weather := WeatherForecastRow{StartAt: start, CloudCover: 2, Temperature: 11, Precipitation: 1}
	if err := db.SaveForecast(ctx, []WeatherForecastRow{weather}); err != nil {
		t.Fatal(err)
	}
	gotWeather, err := db.GetWeatherForecastFrom(ctx, start)
	if err != nil || !slices.Equal(gotWeather, []WeatherForecastRow{weather}) {
		t.Fatalf("weather forecast = %v, error = %v", gotWeather, err)
	}

	planning := []PlanningRow{{StartAt: start, Strategy: "charge"}, {StartAt: next, Strategy: "preserve"}}
	for _, row := range planning {
		if err := db.SavePanning(ctx, row); err != nil {
			t.Fatal(err)
		}
	}
	gotPlanning, err := db.GetPlanningFrom(ctx, start)
	if err != nil || !slices.Equal(gotPlanning, planning) {
		t.Fatalf("planning = %v, error = %v", gotPlanning, err)
	}
	detailed, err := db.GetDetailedPlanningFrom(ctx, start)
	if err != nil || len(detailed) != 2 {
		t.Fatalf("detailed planning = %v, error = %v", detailed, err)
	}
	wantDetailed := DetailedPlanningRow{
		PlanningRow: planning[0], EnergyPrice: sql.Null[float64]{V: 1, Valid: true},
		ProductionEstimated: sql.Null[float64]{V: 3, Valid: true}, ConsumptionEstimated: sql.Null[float64]{V: 5, Valid: true},
		CloudCover: sql.Null[int16]{V: 2, Valid: true}, Temperature: sql.Null[float64]{V: 11, Valid: true}, Precipitation: sql.Null[float64]{V: 1, Valid: true},
	}
	if detailed[0] != wantDetailed || detailed[1].ProductionEstimated.Valid || detailed[1].ConsumptionEstimated.Valid || detailed[1].CloudCover != wantDetailed.CloudCover {
		t.Fatalf("joined values or NULLs changed: %v", detailed)
	}

	first := TimeSeriesRow{
		Timestamp: start, CloudCover: 2, Temperature: 11, Precipitation: 1, EnergyPriceAvg: 2,
		Production: 3, ProductionEstimated: 4, ProductionLifetime: 100,
		Consumption: 5, ConsumptionEstimated: 6, GridImport: 7, GridExport: 8,
		BatteryLevel: 42, BatteryNetLoad: 9, CashFlow: 10, Strategy: "charge",
	}
	second := first
	second.Timestamp, second.Production, second.Consumption = next, 5, 7
	second.ProductionLifetime, second.BatteryLevel, second.CashFlow, second.Strategy = 101, 50, 12, "preserve"
	for _, row := range []TimeSeriesRow{first, second} {
		if err := db.SaveTimeSeries(ctx, row); err != nil {
			t.Fatal(err)
		}
	}
	hourly := first
	hourly.Production, hourly.ProductionEstimated, hourly.ProductionLifetime = 8, 8, 101
	hourly.Consumption, hourly.ConsumptionEstimated = 12, 12
	hourly.GridImport, hourly.GridExport, hourly.BatteryLevel = 14, 16, 50
	hourly.BatteryNetLoad, hourly.CashFlow, hourly.Strategy = 18, 22, "preserve"
	for _, tc := range []struct {
		name  string
		query func() ([]TimeSeriesRow, error)
		want  []TimeSeriesRow
	}{
		{"descending history", func() ([]TimeSeriesRow, error) { return db.GetTimeSeriesFrom(ctx, start) }, []TimeSeriesRow{second, first}},
		{"hour history", func() ([]TimeSeriesRow, error) { return db.GetTimeSeriesForHour(ctx, start) }, []TimeSeriesRow{first, second}},
		{"quarter-hour history", func() ([]TimeSeriesRow, error) { return db.Get15MinSummaryForSlot(ctx, start) }, []TimeSeriesRow{first}},
		{"hourly aggregation", func() ([]TimeSeriesRow, error) { return db.GetHourlySummaryForHour(ctx, start) }, []TimeSeriesRow{hourly}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.query()
			if err != nil || !slices.Equal(got, tc.want) {
				t.Fatalf("rows = %+v, error = %v; want %+v", got, err, tc.want)
			}
		})
	}
	daily, err := db.GetDailyStats(ctx, 1)
	wantDaily := []DailyStats{{Date: "2026-09-05", AvgCloudCover: 2, AvgTemperature: 11, AvgPrecipitation: 1, AvgEnergyPrice: 2, TotProduction: 8, TotConsumption: 12, TotGridImport: 14, TotGridExport: 16, TotCashFlow: 22}}
	if err != nil || !slices.Equal(daily, wantDaily) {
		t.Fatalf("daily stats = %+v, error = %v; want %+v", daily, err, wantDaily)
	}

	// Snapshot collection deliberately accepts timestamps outside a 15-minute boundary.
	snapshot := FaSnapshotRow{Timestamp: start.Add(3 * time.Minute), Data: *ferroamp.NewFaData()}
	snapshot.Data.Ehub.Soc.Value = 42
	if err := db.SaveFaSnapshot(ctx, snapshot); err != nil {
		t.Fatal(err)
	}
	snapshots, err := db.GetFaSnapshotFrom(ctx, start)
	if err != nil || len(snapshots) != 1 || snapshots[0].Timestamp != snapshot.Timestamp || snapshots[0].Data.Ehub.Soc.Value != 42 {
		t.Fatalf("snapshots = %+v, error = %v", snapshots, err)
	}
	for _, row := range []LogEntryRow{
		{Timestamp: start.Time(), Level: int(slog.LevelInfo), Message: "older", Attrs: "a=1"},
		{Timestamp: next.Time(), Level: int(slog.LevelWarn), Message: "newer", Attrs: "a=2"},
	} {
		if err := db.SaveLogEntry(ctx, row); err != nil {
			t.Fatal(err)
		}
	}
	logs, err := db.GetLogEntries(ctx, slog.LevelInfo, 2, 1)
	if err != nil || len(logs) != 1 || logs[0].Message != "older" || logs[0].Timestamp != start.Time() || logs[0].Attrs != "a=1" {
		t.Fatalf("paginated logs = %+v, error = %v", logs, err)
	}
}

func TestCollectionTimestampPolicies(t *testing.T) {
	db := newQueryTestDatabase(t)
	ctx := t.Context()
	if err := db.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	start := timex.BucketTime(time.Date(2026, 9, 5, 8, 0, 0, 0, time.UTC))
	bad := start.Add(time.Minute)
	for _, slot := range []timex.BucketTime{start, bad, start.Add15Min(1)} {
		if err := db.SavePanning(ctx, PlanningRow{StartAt: slot, Strategy: "charge"}); err != nil {
			t.Fatal(err)
		}
		if err := db.SaveEnergyForecast(ctx, []EnergyForecastRow{{StartAt: slot}}); err != nil {
			t.Fatal(err)
		}
		if err := db.SaveTimeSeries(ctx, TimeSeriesRow{Timestamp: slot}); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		name  string
		query func() (int, error)
	}{
		{"planning", func() (int, error) { rows, err := db.GetPlanningFrom(ctx, start); return len(rows), err }},
		{"detailed planning", func() (int, error) { rows, err := db.GetDetailedPlanningFrom(ctx, start); return len(rows), err }},
		{"energy forecast", func() (int, error) { rows, err := db.GetEnergyForecastFrom(ctx, start); return len(rows), err }},
		{"time series", func() (int, error) { rows, err := db.GetTimeSeriesFrom(ctx, start); return len(rows), err }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			count, err := tc.query()
			if err != nil || count != 2 {
				t.Fatalf("count = %d, error = %v; want both valid rows", count, err)
			}
		})
	}
	if err := db.SaveEnergyPrices(ctx, []EnergyPriceRow{{StartAt: start}, {StartAt: bad}}); err != nil {
		t.Fatal(err)
	}
	if rows, err := db.GetEnergyPriceFrom(ctx, start); err == nil || len(rows) != 0 {
		t.Fatalf("malformed prices returned rows = %v, error = %v", rows, err)
	}
	if err := db.SaveForecast(ctx, []WeatherForecastRow{{StartAt: start}, {StartAt: bad}}); err != nil {
		t.Fatal(err)
	}
	if rows, err := db.GetWeatherForecastFrom(ctx, start); err == nil || len(rows) != 0 {
		t.Fatalf("malformed weather returned rows = %v, error = %v", rows, err)
	}
	if _, err := db.write.ExecContext(ctx, "INSERT INTO fa_snapshot (timestamp, data) VALUES (?, '{}')", "2026-09-05T08:01:00X"); err != nil {
		t.Fatal(err)
	}
	if rows, err := db.GetFaSnapshotFrom(ctx, start); err == nil || len(rows) != 0 {
		t.Fatalf("malformed snapshot returned rows = %v, error = %v", rows, err)
	}
}
