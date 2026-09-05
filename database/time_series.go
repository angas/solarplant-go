package database

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"

	"github.com/angas/solarplant-go/timex"
)

type TimeSeriesRow struct {
	Timestamp            timex.BucketTime
	CloudCover           uint8
	Temperature          float64
	Precipitation        float64
	EnergyPriceAvg       float64
	Production           float64
	ProductionEstimated  float64
	ProductionLifetime   float64
	Consumption          float64
	ConsumptionEstimated float64
	GridImport           float64
	GridExport           float64
	BatteryLevel         float64
	BatteryNetLoad       float64
	CashFlow             float64
	Strategy             string
}

type DailyStats struct {
	Date             string
	AvgCloudCover    float64
	AvgTemperature   float64
	AvgPrecipitation float64
	AvgEnergyPrice   float64
	TotProduction    float64
	DiffProduction   float64
	TotConsumption   float64
	DiffConsumption  float64
	TotGridImport    float64
	TotGridExport    float64
	TotCashFlow      float64
}

func (d *Database) SaveTimeSeries(ctx context.Context, row TimeSeriesRow) error {
	d.logger.Debug("saving time series",
		"timestamp", row.Timestamp,
		"cloud_cover", row.CloudCover,
		"temperature", row.Temperature,
		"precipitation", row.Precipitation,
		"energy_price_avg", row.EnergyPriceAvg,
		"production", row.Production,
		"production_estimated", row.ProductionEstimated,
		"production_lifetime", row.ProductionLifetime,
		"consumption", row.Consumption,
		"consumption_estimated", row.ConsumptionEstimated,
		"grid_import", row.GridImport,
		"grid_export", row.GridExport,
		"battery_level", row.BatteryLevel,
		"battery_net_load", row.BatteryNetLoad,
		"cash_flow", row.CashFlow,
		"strategy", row.Strategy)

	_, err := d.write.ExecContext(ctx, `
		INSERT INTO time_series (
			timestamp,
			cloud_cover,
			temperature,
			precipitation,
			energy_price_avg,
			production,
			production_estimated,
			production_lifetime,
			consumption,
			consumption_estimated,
			grid_import,
			grid_export,
			battery_level,
			battery_net_load,
			cash_flow,
			strategy
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		row.Timestamp.String(),
		row.CloudCover,
		row.Temperature,
		row.Precipitation,
		row.EnergyPriceAvg,
		row.Production,
		row.ProductionEstimated,
		row.ProductionLifetime,
		row.Consumption,
		row.ConsumptionEstimated,
		row.GridImport,
		row.GridExport,
		row.BatteryLevel,
		row.BatteryNetLoad,
		row.CashFlow,
		row.Strategy,
	)

	if err != nil {
		return fmt.Errorf("saving time series (%s): %w", row.Timestamp.String(), err)
	}

	return nil
}

// Returns time series entries from this hour and every day following
func (d *Database) GetTimeSeriesForHour(ctx context.Context, hour timex.BucketTime) ([]TimeSeriesRow, error) {
	ts, err := d.queryRows(ctx, `
		SELECT
			timestamp,
			cloud_cover,
			temperature,
			precipitation,
			energy_price_avg,
			production,
			production_estimated,
			production_lifetime,
			consumption,
			consumption_estimated,
			grid_import,
			grid_export,
			battery_level,
			battery_net_load,
			cash_flow,
			strategy
		FROM time_series
		WHERE timestamp >= ? AND STRFTIME('%H', timestamp) = ?
		ORDER BY timestamp ASC`,
		func(rows *sql.Rows) (TimeSeriesRow, error) {
			return d.scanTimeSeriesRow(ctx, rows, timex.BucketSize15Minutes)
		}, hour.String(), fmt.Sprintf("%02d", hour.Time().Hour()))
	if err != nil {
		return nil, fmt.Errorf("fetching time series from hour %s: %w", hour.String(), err)
	}

	return ts, nil
}

func (d *Database) GetTimeSeriesFrom(ctx context.Context, from timex.BucketTime) ([]TimeSeriesRow, error) {
	ts, err := d.queryRows(ctx, `
		SELECT
			timestamp,
			cloud_cover,
			temperature,
			precipitation,
			energy_price_avg,
			production,
			production_estimated,
			production_lifetime,
			consumption,
			consumption_estimated,
			grid_import,
			grid_export,
			battery_level,
			battery_net_load,
			cash_flow,
			strategy
		FROM time_series
		WHERE timestamp >= ?
		ORDER BY timestamp DESC`,
		func(rows *sql.Rows) (TimeSeriesRow, error) {
			return d.scanTimeSeriesRow(ctx, rows, timex.BucketSize15Minutes)
		}, from.String())
	if err != nil {
		return nil, fmt.Errorf("fetching time series from %s: %w", from.String(), err)
	}

	return ts, nil
}

func (d *Database) scanTimeSeriesRow(ctx context.Context, rows *sql.Rows, bucketSize timex.BucketSize) (TimeSeriesRow, error) {
	var t TimeSeriesRow
	var tsStr string
	err := rows.Scan(
		&tsStr,
		&t.CloudCover,
		&t.Temperature,
		&t.Precipitation,
		&t.EnergyPriceAvg,
		&t.Production,
		&t.ProductionEstimated,
		&t.ProductionLifetime,
		&t.Consumption,
		&t.ConsumptionEstimated,
		&t.GridImport,
		&t.GridExport,
		&t.BatteryLevel,
		&t.BatteryNetLoad,
		&t.CashFlow,
		&t.Strategy)
	if err != nil {
		return TimeSeriesRow{}, err
	}

	t.Timestamp, err = timex.ParseBucketTime(tsStr, bucketSize)
	if err != nil {
		d.logger.WarnContext(ctx, "parsing time series timestamp", slog.String("timestamp", tsStr), slog.Any("error", err))
		return TimeSeriesRow{}, errSkipRow
	}
	return t, nil
}

// GetHourlySummaryForHour returns historical hourly totals for a given hour-of-day,
// aggregating any 15-min rows within each hour into a single row per date.
// This correctly handles both old hourly rows and new 15-min rows during transition.
func (d *Database) GetHourlySummaryForHour(ctx context.Context, hour timex.BucketTime) ([]TimeSeriesRow, error) {
	tsr, err := d.queryRows(ctx, `
		SELECT
			STRFTIME('%Y-%m-%dT%H:00:00Z', timestamp) AS hour_ts,
			AVG(cloud_cover),
			AVG(temperature),
			AVG(precipitation),
			AVG(energy_price_avg),
			SUM(production),
			SUM(production_estimated),
			MAX(production_lifetime),
			SUM(consumption),
			SUM(consumption_estimated),
			SUM(grid_import),
			SUM(grid_export),
			MAX(battery_level),
			SUM(battery_net_load),
			SUM(cash_flow),
			MAX(strategy)
		FROM time_series
		WHERE timestamp >= ? AND STRFTIME('%H', timestamp) = ?
		GROUP BY hour_ts
		ORDER BY hour_ts ASC`,
		func(rows *sql.Rows) (TimeSeriesRow, error) {
			return d.scanTimeSeriesRow(ctx, rows, timex.BucketSizeHour)
		}, hour.String(), fmt.Sprintf("%02d", hour.Time().Hour()))
	if err != nil {
		return nil, fmt.Errorf("fetching hourly summary for hour %s: %w", hour.String(), err)
	}

	return tsr, nil
}

// Get15MinSummaryForSlot returns historical time series rows for a specific 15-minute
// slot (matching both hour and minute). Each matching row is a single 15-min entry
// from a different historical date. The slot parameter determines the lookback start
// and the hour:minute to match.
func (d *Database) Get15MinSummaryForSlot(ctx context.Context, slot timex.BucketTime) ([]TimeSeriesRow, error) {
	ts, err := d.queryRows(ctx, `
		SELECT
			timestamp,
			cloud_cover,
			temperature,
			precipitation,
			energy_price_avg,
			production,
			production_estimated,
			production_lifetime,
			consumption,
			consumption_estimated,
			grid_import,
			grid_export,
			battery_level,
			battery_net_load,
			cash_flow,
			strategy
		FROM time_series
		WHERE timestamp >= ? AND STRFTIME('%H', timestamp) = ? AND STRFTIME('%M', timestamp) = ?
		ORDER BY timestamp ASC`,
		func(rows *sql.Rows) (TimeSeriesRow, error) {
			return d.scanTimeSeriesRow(ctx, rows, timex.BucketSize15Minutes)
		}, slot.String(),
		fmt.Sprintf("%02d", slot.Time().Hour()),
		fmt.Sprintf("%02d", slot.Time().Minute()))
	if err != nil {
		return nil, fmt.Errorf("fetching 15-min summary for slot %s: %w", slot.String(), err)
	}
	return ts, nil
}

func (d *Database) GetDailyStats(ctx context.Context, noOfDays int) ([]DailyStats, error) {
	dailyStats, err := d.queryRows(ctx, `
		SELECT
			STRFTIME('%Y-%m-%d', timestamp) AS day,
			AVG(cloud_cover),
			AVG(temperature),
			AVG(precipitation),
			AVG(energy_price_avg),
			SUM(production),
			AVG(production-production_estimated),
			SUM(consumption),
			AVG(consumption-consumption_estimated),
			SUM(grid_import),
			SUM(grid_export),
			SUM(cash_flow)
		FROM time_series
		GROUP BY day
		ORDER BY day DESC
		LIMIT ?`,
		func(rows *sql.Rows) (DailyStats, error) {
			var ds DailyStats
			err := rows.Scan(
				&ds.Date,
				&ds.AvgCloudCover,
				&ds.AvgTemperature,
				&ds.AvgPrecipitation,
				&ds.AvgEnergyPrice,
				&ds.TotProduction,
				&ds.DiffProduction,
				&ds.TotConsumption,
				&ds.DiffConsumption,
				&ds.TotGridImport,
				&ds.TotGridExport,
				&ds.TotCashFlow)
			if err != nil {
				return DailyStats{}, fmt.Errorf("scanning daily stats: %w", err)
			}
			return ds, nil
		}, noOfDays)
	if err != nil {
		return []DailyStats{}, fmt.Errorf("fetching daily stats: %w", err)
	}

	return dailyStats, nil
}

func (d *Database) PurgeTimeSeries(ctx context.Context, retentionDays int) error {
	return d.purgeTable(ctx, "time_series", "timestamp", retentionDays)
}
