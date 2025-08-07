package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/angas/solarplant-go/config"
	"github.com/angas/solarplant-go/database"
	"github.com/angas/solarplant-go/elprisetjustnu"
	"github.com/angas/solarplant-go/ferroamp"
	"github.com/angas/solarplant-go/logging"
	"github.com/angas/solarplant-go/nordpool"
	"github.com/angas/solarplant-go/task"
	"github.com/angas/solarplant-go/types"
	"github.com/angas/solarplant-go/www"
	"github.com/lmittmann/tint"
)

var Version = "?.?.?"

func main() {
	defer func() {
		if err := recover(); err != nil {
			exitWithError(slog.Default(), fmt.Errorf("application panicked: %v", err))
		} else {
			slog.Default().Info("application is shutting down...")
		}
	}()

	configPath := flag.String("config", "", "path to config file")
	flag.Parse()

	cnfg, err := config.Load(*configPath)
	if err != nil {
		panic(fmt.Sprintf("failed to load config: %v", err))
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	consoleHandler := tint.NewHandler(os.Stdout, &tint.Options{
		Level:      cnfg.Logging.GetConsoleLevel(),
		TimeFormat: time.RFC3339,
	})
	slog.New(consoleHandler).Debug("solarplant is starting...", slog.String("version", Version))

	db, err := database.New(ctx, cnfg.Database.Path)
	if err != nil {
		panic(fmt.Sprintf("failed to connect to database: %v", err))
	}
	defer db.Close()

	logger := slog.New(logging.NewMultiHandler(
		consoleHandler,
		logging.NewSQLiteHandler(db, cnfg.Logging.GetDbLevel(), cnfg.Logging.GetDbAttrsFormat())))
	slog.SetDefault(logger)

	// Now we can use the logger to log database operations into the database itself
	db.SetLogger(logger.With("module", "database"))

	recentHours := database.NewRecentHours(db)
	if err := recentHours.Reload(ctx); err != nil {
		panic(fmt.Sprintf("failed to load recent hours: %v", err))
	}

	fa := ferroamp.New(
		cnfg.Ferroamp.Host,
		cnfg.Ferroamp.Port,
		cnfg.Ferroamp.Username,
		cnfg.Ferroamp.Password)

	faInMem := ferroamp.NewFaInMemData()
	fa.OnEhubMessage = faInMem.SetEHub
	fa.OnSsoMessage = faInMem.SetSso
	fa.OnEsmMessage = faInMem.SetEsm
	fa.OnEsoMessage = faInMem.SetEso
	fa.OnControlResponse = func(msg *ferroamp.ControlResponseMessage) {
		if msg.Status == "ack" {
			logger.Info("control request succeeded (ack)", slog.String("transId", msg.TransId), slog.String("message", msg.Message))
		} else {
			logger.Warn("control request failed (nak)", slog.String("transId", msg.TransId), slog.String("message", msg.Message))
		}
	}
	fa.OnInactivity = func() {
		fa.Disconnect()
		exitWithError(logger, fmt.Errorf("ferroamp mqtt traffic is dead, terminating..."))
	}

	if isDevMode() {
		logger.Info("dev mode, skipping ferroamp connection")
	} else {
		if err := fa.Connect(); err != nil {
			panic(fmt.Sprintf("ferroamp connection error: %v", err))
		}
		defer fa.Disconnect()
	}

	energyPriceProviders := []types.EnergyPriceProvider{
		elprisetjustnu.New(cnfg.EnergyPrice.Area), // Primary provider
		nordpool.New(cnfg.EnergyPrice.Area),       // Secondary provider
	}

	tasks := task.NewTasks(db, energyPriceProviders, faInMem, recentHours, cnfg)
	if isDevMode() {
		logger.Info("dev mode, skipping task scheduling")
	} else {
		tasks.Run()
		defer tasks.Stop()
	}

	regulatorStrategy := task.BatteryRegulatorStrategy{
		Interval:        time.Second * 10,
		UpdateThreshold: 0.1,
		GridMaxPower:    cnfg.Planner.GridMaxPower,
	}
	batteryRegulator := task.NewBatteryRegulator(logger, db, cnfg.BatterySpec, faInMem, regulatorStrategy)
	if isDevMode() {
		logger.Info("dev mode, skipping battery regulator")
	} else {
		batteryRegulator.Run(ctx)
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	go func() {
		for {
			select {
			case <-ctx.Done():
				logger.Info("main context done")
				return
			case sig := <-sigCh:
				logger.Info("received signal", slog.Any("signal", sig))
				cancel()
			case batt := <-batteryRegulator.C:
				switch batt.Action {
				case task.ActionAuto:
					if err := fa.SetBatteryAuto(); err != nil {
						logger.Error("failed to set battery to auto mode", slog.Any("error", err))
					}
				case task.ActionCharge:
					if err := fa.SetBatteryLoad(-batt.Power); err != nil {
						logger.Error("failed to set battery to charge mode", slog.Any("error", err))
					}
				case task.ActionDischarge:
					if err := fa.SetBatteryLoad(batt.Power); err != nil {
						logger.Error("failed to set battery to discharge mode", slog.Any("error", err))
					}
				}
			}
		}
	}()

	server := www.StartServer(db, tasks, faInMem, recentHours, cnfg, Version)
	server.Run(ctx)
}

func isDevMode() bool {
	return strings.EqualFold(os.Getenv("APP_ENV"), "development")
}

func exitWithError(logger *slog.Logger, err error) {
	if err != nil {
		logger.Error("application shutting down with error", slog.Any("error", err))
	}
	if syncer, ok := logger.Handler().(interface{ Sync() error }); ok {
		if syncErr := syncer.Sync(); syncErr != nil {
			logger.Error("failed to flush logger", slog.Any("error", syncErr))
		}
	}

	time.Sleep(2 * time.Second)
	os.Exit(1)
}
