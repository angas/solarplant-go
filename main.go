package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"runtime/debug"
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
		if r := recover(); r != nil {
			slog.Error("application panicked", slog.Any("error", r), slog.String("stack", string(debug.Stack())))
			os.Exit(1)
		}
	}()

	configPath := flag.String("config", "", "path to config file")
	flag.Parse()

	cnfg := config.Load(*configPath)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	consoleHandler := tint.NewHandler(os.Stdout, &tint.Options{
		Level:      cnfg.Logging.GetConsoleLevel(),
		TimeFormat: time.RFC3339,
	})

	db, err := database.New(ctx, cnfg.Database.Path)
	if err != nil {
		panic(err)
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
		panic(err)
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

	if isDevMode() {
		logger.Info("dev mode, skipping ferroamp connection")
	} else {
		if err := fa.Connect(); err != nil {
			logger.Error("ferroamp disconnected, terminating...", slog.Any("error", err))
			os.Exit(1)
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
				logger.Info("terminating...")
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
