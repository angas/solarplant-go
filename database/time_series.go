package database

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/angas/solarplant-go/hours"
)

type TimeSeriesRow struct {
	When                 hours.DateHour
	CloudCover           uint8
	Temperature          float64
	Precipitation        float64
	EnergyPrice          float64
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
		"hour", row.When,
		"cloud_cover", row.CloudCover,
		"temperature", row.Temperature,
		"precipitation", row.Precipitation,
		"energy_price", row.EnergyPrice,
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
			date,
			hour,
			cloud_cover,
			temperature,
			precipitation,
			energy_price,
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
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		row.When.Date,
		row.When.Hour,
		row.CloudCover,
		row.Temperature,
		row.Precipitation,
		row.EnergyPrice,
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
		return fmt.Errorf("saving time series: %w", err)
	}

	return nil
}

/** Returns time series entries from this date and hour and every following same hour */
func (d *Database) GetTimeSeriesForHour(ctx context.Context, dh hours.DateHour) ([]TimeSeriesRow, error) {
	rows, err := d.read.Query(`
		SELECT 
			date, 
			hour, 
			cloud_cover, 
			temperature, 
			precipitation, 
			energy_price, 
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
		WHERE date >= ? AND hour = ?
		ORDER BY date, hour ASC`,
		dh.Date, dh.Hour)
	if err != nil {
		return nil, fmt.Errorf("fetching time series for %s: %w", dh, err)
	}

	defer rows.Close()

	ts, err := scanTimeSeriesHours(rows)
	if err != nil {
		return ts, fmt.Errorf("scanning time series row: %w", err)
	}

	return ts, nil
}

func (d *Database) GetTimeSeriesFrom(ctx context.Context, dh hours.DateHour) ([]TimeSeriesRow, error) {
	rows, err := d.read.QueryContext(ctx, `
		SELECT 
			date, 
			hour, 
			cloud_cover, 
			temperature, 
			precipitation, 
			energy_price, 
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
		WHERE (date >= ? AND hour >= ?) OR (date > ?)
		ORDER BY date DESC, hour DESC`,
		dh.Date, dh.Hour, dh.Date)
	if err != nil {
		return nil, fmt.Errorf("fetching time series since %s: %w", dh, err)
	}

	defer rows.Close()

	ts, err := scanTimeSeriesHours(rows)
	if err != nil {
		return ts, fmt.Errorf("scanning time series row: %w", err)
	}

	return ts, nil
}

func scanTimeSeriesHours(rows *sql.Rows) ([]TimeSeriesRow, error) {
	var ts []TimeSeriesRow
	for rows.Next() {
		var t TimeSeriesRow
		err := rows.Scan(
			&t.When.Date,
			&t.When.Hour,
			&t.CloudCover,
			&t.Temperature,
			&t.Precipitation,
			&t.EnergyPrice,
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

		ts = append(ts, t)
	}

	return ts, nil
}

func (d *Database) GetDailyStats(ctx context.Context, noOfDays int) ([]DailyStats, error) {
	rows, err := d.read.QueryContext(ctx, `
		SELECT 
			date, 
			avg(cloud_cover),
			avg(temperature),
			avg(precipitation),
			avg(energy_price),
			sum(production),
			avg(production-production_estimated),
			sum(consumption),
			avg(consumption-consumption_estimated),
			sum(grid_import),
			sum(grid_export),
			sum(cash_flow)
		FROM time_series
		GROUP BY date
		ORDER BY date DESC
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
	return d.purgeTable(ctx, "time_series", retentionDays)
}
