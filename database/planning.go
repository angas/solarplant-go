package database

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"

	"github.com/angas/solarplant-go/timex"
)

const planningBucketSize = timex.BucketSize15Minutes // Should match the bucket size of energy price

type PlanningRow struct {
	StartAt  timex.BucketTime
	Strategy string
}

type DetailedPlanningRow struct {
	PlanningRow
	EnergyPrice          sql.NullFloat64
	ProductionEstimated  sql.NullFloat64
	ConsumptionEstimated sql.NullFloat64
	CloudCover           sql.NullInt16
	Temperature          sql.NullFloat64
	Precipitation        sql.NullFloat64
}

func (d *Database) SavePanning(ctx context.Context, row PlanningRow) error {
	d.logger.Debug("saving planning",
		slog.String("startAt", row.StartAt.String()),
		slog.String("strategy", row.Strategy))

	_, err := d.write.ExecContext(ctx, `
		INSERT INTO planning (start_at, strategy)
		VALUES (?, ?)
		ON CONFLICT(start_at) DO UPDATE SET strategy = excluded.strategy;`,
		row.StartAt.String(),
		row.Strategy,
	)
	if err != nil {
		return fmt.Errorf("saving planning row (%s): %w", row.StartAt.String(), err)
	}
	return nil
}

func (d *Database) GetPlanning(ctx context.Context, startAt timex.BucketTime) (PlanningRow, error) {
	row := d.read.QueryRowContext(ctx, `
		SELECT start_at, strategy
		FROM planning
		WHERE start_at = ?`,
		startAt.String())

	var pl PlanningRow
	var startAtStr string
	err := row.Scan(&startAtStr, &pl.Strategy)
	if err == sql.ErrNoRows {
		return PlanningRow{}, sql.ErrNoRows
	}
	if err != nil {
		return PlanningRow{}, fmt.Errorf("scanning planning row (%s): %w", startAtStr, err)
	}

	pl.StartAt, err = timex.ParseBucketTime(startAtStr, planningBucketSize)
	if err != nil {
		return PlanningRow{}, fmt.Errorf("parsing planning row start_at (%s): %w", startAtStr, err)
	}

	return pl, nil
}

func (d *Database) GetPlanningFrom(ctx context.Context, startAt timex.BucketTime) ([]PlanningRow, error) {
	rows, err := d.read.QueryContext(ctx, `
		SELECT start_at, strategy
		FROM planning
		WHERE start_at >= ?
		ORDER BY start_at ASC`,
		startAt.String())
	if err != nil {
		return nil, fmt.Errorf("fetching planning from %s: %w", startAt.String(), err)
	}
	defer rows.Close()

	var res []PlanningRow
	for rows.Next() {
		var row PlanningRow
		var startAtStr string
		err := rows.Scan(&startAtStr, &row.Strategy)
		if err != nil {
			return nil, err
		}
		row.StartAt, err = timex.ParseBucketTime(startAtStr, planningBucketSize)
		if err != nil {
			d.logger.WarnContext(ctx, "parsing planning row start_at", slog.String("startAt", startAtStr), slog.Any("error", err))
			continue
		}
		res = append(res, row)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating planning from %s: %w", startAt.String(), err)
	}

	return res, nil
}

func (d *Database) GetDetailedPlanningFrom(ctx context.Context, startAt timex.BucketTime) ([]DetailedPlanningRow, error) {
	rows, err := d.read.QueryContext(ctx, `
		SELECT
	    pl.start_at,
	    pl.strategy,
	    ep.price,
	    ef.production,
	    ef.consumption,
	    wf.cloud_cover,
	    wf.temperature,
	    wf.precipitation
		FROM planning pl
		LEFT OUTER JOIN energy_price ep ON ep.start_at = pl.start_at
		LEFT OUTER JOIN energy_forecast ef ON SUBSTR(ef.start_at, 1, 13) = SUBSTR(pl.start_at, 1, 13)
		LEFT OUTER JOIN weather_forecast wf ON SUBSTR(wf.start_at, 1, 13) = SUBSTR(pl.start_at, 1, 13)
		WHERE (pl.start_at >= ?)
		ORDER BY pl.start_at ASC;`,
		startAt.String())
	if err != nil {
		return nil, fmt.Errorf("fetching detailed planning from %s: %w", startAt.String(), err)
	}
	defer rows.Close()

	var res []DetailedPlanningRow
	for rows.Next() {
		var row DetailedPlanningRow
		var startAtStr string
		err := rows.Scan(
			&startAtStr,
			&row.Strategy,
			&row.EnergyPrice,
			&row.ProductionEstimated,
			&row.ConsumptionEstimated,
			&row.CloudCover,
			&row.Temperature,
			&row.Precipitation)
		if err != nil {
			return nil, err
		}
		row.StartAt, err = timex.ParseBucketTime(startAtStr, planningBucketSize)
		if err != nil {
			d.logger.WarnContext(ctx, "parsing detailed planning row start_at", slog.String("startAt", startAtStr), slog.Any("error", err))
			continue
		}

		res = append(res, row)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating detailed planning from %s: %w", startAt.String(), err)
	}

	return res, nil
}

func (d *Database) PurgePlanning(ctx context.Context, retentionDays int) error {
	return d.purgeTable(ctx, "planning", "start_at", retentionDays)
}
