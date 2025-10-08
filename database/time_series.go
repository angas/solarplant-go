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
	rows, err := d.read.Query(`
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
		hour.String(), fmt.Sprintf("%02d", hour.Time().Hour()))
	if err != nil {
		return nil, fmt.Errorf("fetching time series from hour %s: %w", hour.String(), err)
	}

	defer rows.Close()

	ts, err := d.scanTimeSeriesHours(rows)
	if err != nil {
		return ts, fmt.Errorf("scanning time series row: %w", err)
	}

	return ts, nil
}

func (d *Database) GetTimeSeriesFrom(ctx context.Context, from timex.BucketTime) ([]TimeSeriesRow, error) {
	rows, err := d.read.QueryContext(ctx, `
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
		from.String())
	if err != nil {
		return nil, fmt.Errorf("fetching time series from %s: %w", from.String(), err)
	}

	defer rows.Close()

	ts, err := d.scanTimeSeriesHours(rows)
	if err != nil {
		return ts, fmt.Errorf("scanning time series row: %w", err)
	}

	return ts, nil
}

func (d *Database) scanTimeSeriesHours(rows *sql.Rows) ([]TimeSeriesRow, error) {
	var tsr []TimeSeriesRow
	for rows.Next() {
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
			return nil, err
		}

		t.Timestamp, err = timex.ParseBucketTime(tsStr, timex.BucketSizeHour)
		if err != nil {
			d.logger.Warn("parsing timestamp", slog.String("timestamp", tsStr), slog.String("error", err.Error()))
			continue
		}

		tsr = append(tsr, t)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("scanning time series rows: %w", err)
	}

	return tsr, nil
}

func (d *Database) GetDailyStats(ctx context.Context, noOfDays int) ([]DailyStats, error) {
	rows, err := d.read.QueryContext(ctx, `
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
		noOfDays)
	if err != nil {
		return []DailyStats{}, fmt.Errorf("fetching daily stats: %w", err)
	}

	defer rows.Close()

	var dailyStats []DailyStats
	for rows.Next() {
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
			return []DailyStats{}, fmt.Errorf("scanning daily stats: %w", err)
		}
		dailyStats = append(dailyStats, ds)
	}

	return dailyStats, nil
}

func (d *Database) PurgeTimeSeries(ctx context.Context, retentionDays int) error {
	return d.purgeTable(ctx, "time_series", "timestamp", retentionDays)
}
