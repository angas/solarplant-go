package task

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/angas/solarplant-go/config"
	"github.com/angas/solarplant-go/database"
	"github.com/angas/solarplant-go/ferroamp"
	"github.com/angas/solarplant-go/types"
	"github.com/robfig/cron/v3"
)

type Tasks struct {
	cron                *cron.Cron
	cnfg                *config.AppConfig
	logger              *slog.Logger
	WeatherForecastTask func()
	EnergyForecastTask  func()
	EnergyPriceTask     func()
	TimeSeriesTask      func()
	PlanningTask        func()
	MaintenanceTask     func()
}

func NewTasks(
	db *database.Database,
	energyPriceProviders []types.EnergyPriceProvider,
	faInMem *ferroamp.FaInMemData,
	recentHours *database.RecentHours,
	cnfg *config.AppConfig,
) *Tasks {
	logger := slog.Default().With("module", "tasks")
	return &Tasks{
		cron:                cron.New(),
		cnfg:                cnfg,
		logger:              logger,
		WeatherForecastTask: NewWeatherForecastTask(logger.With(slog.String("task", "weather_forecast")), db, cnfg.WeatherForecast),
		EnergyForecastTask:  NewEnergyForecastTask(logger.With(slog.String("task", "energy_forecast")), db, cnfg.EnergyForecast),
		EnergyPriceTask:     NewEnergyPriceTask(logger.With(slog.String("task", "energy_price")), db, energyPriceProviders),
		TimeSeriesTask:      NewHourlyTask(logger.With(slog.String("task", "time_series")), db, cnfg.EnergyPrice, faInMem, recentHours),
		PlanningTask:        NewPlanningTask(logger.With(slog.String("task", "planning")), db, cnfg, faInMem),
		MaintenanceTask:     NewMaintenanceTask(logger.With(slog.String("task", "maintenance")), db, cnfg),
	}
}

func (t *Tasks) Run() {
	_, err := t.cron.AddFunc(t.cnfg.WeatherForecast.RunAt, t.WeatherForecastTask)
	if err != nil {
		panic(fmt.Sprintf("failed to schedule weather forecast task: %v", err))
	}
	_, err = t.cron.AddFunc(t.cnfg.EnergyForecast.RunAt, t.EnergyForecastTask)
	if err != nil {
		panic(fmt.Sprintf("failed to schedule energy forecast task: %v", err))
	}
	_, err = t.cron.AddFunc(t.cnfg.EnergyPrice.RunAt, t.EnergyPriceTask)
	if err != nil {
		panic(fmt.Sprintf("failed to schedule energy price task: %v", err))
	}
	_, err = t.cron.AddFunc("@hourly", t.TimeSeriesTask)
	if err != nil {
		panic(fmt.Sprintf("failed to schedule time series task: %v", err))
	}
	_, err = t.cron.AddFunc(t.cnfg.Planner.RunAt, t.PlanningTask)
	if err != nil {
		panic(fmt.Sprintf("failed to schedule planning task: %v", err))
	}
	_, err = t.cron.AddFunc("30 2 * * *", t.MaintenanceTask)
	if err != nil {
		panic(fmt.Sprintf("failed to schedule maintenance task: %v", err))
	}
	t.cron.Start()
}

func (t *Tasks) Stop() context.Context {
	return t.cron.Stop()
}
