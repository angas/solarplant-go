package config

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/angas/solarplant-go/logging"
	"github.com/spf13/viper"
)

type AppConfigApi struct {
	Address string
	Port    int16
	// If not assigned, the server will serve embedded files.
	// If assigned, the server will serve files from the directory,
	// that must contain a "static" and "templates" directory.
	// This is useful for development.
	WwwDir *string `mapstructure:"www_dir"`
}

type AppConfigDatabase struct {
	Path string
	// How many days data should be stored in database before it gets purged
	DataRetentionDays *int `mapstructure:"data_retention_days"`
	// How many days daily backup files should be stored before they gets deleted
	BackupRetentionDays *int `mapstructure:"backup_retention_days"`
}

func (d AppConfigDatabase) GetDataRetentionDays() int {
	if d.DataRetentionDays == nil {
		return 90
	}
	return *d.DataRetentionDays
}

func (d AppConfigDatabase) GetBackupRetentionDays() int {
	if d.BackupRetentionDays == nil {
		return 90
	}
	return *d.BackupRetentionDays
}

type AppConfigFerroamp struct {
	Host     string
	Port     int16
	Username string
	Password string // Handed over by Ferroamp on request
}

type AppConfigWeatherForecast struct {
	Latitude  float64 // Your approx latitude position (WGS84)
	Longitude float64 // Your approx longitude position (WGS84)
	RunAt     string  `mapstructure:"run_at"`
}

type AppConfigEnergyPrice struct {
	Tax          float64 `mapstructure:"tax_including_vat"` // Energy tax in SEK/kWh including VAT (energiskatt inkl. moms)
	TaxReduction float64 `mapstructure:"tax_reduction"`     // Energy tax reduction in SEK/kWh when selling energy back to the grid (skattereduktion)
	GridBenefit  float64 `mapstructure:"grid_benefit"`      // Grid benefit in SEK/kWh (nätnytta)
	Area         string  `mapstructure:"area"`              // "SE1", "SE2", "SE3", "SE4"
	RunAt        string  `mapstructure:"run_at"`
}

type AppConfigEnergyForecast struct {
	// How many hours ahead to forecast energy production and consumption, can stop earlier if data is missing
	HoursAhead int `mapstructure:"hours_ahead"`
	// How many days back should be consider when estimating future energy production and consumption
	HistoricalDays int `mapstructure:"historical_days"`
	// A value between 0 and 1 where 0 means no impact and 1 means full impact, i.e. no EV production when cloudiness is 8 octas
	CloudCoverImpact float64 `mapstructure:"cloud_cover_impact"`
	RunAt            string  `mapstructure:"run_at"`
}

type AppConfigBatterySpec struct {
	Capacity         float64 `mapstructure:"capacity"`           // Battery maximum capacity in kWh
	MinLevel         float64 `mapstructure:"min_level"`          // Battery minimum level in percentage
	MaxLevel         float64 `mapstructure:"max_level"`          // Battery maximum level in percentage
	MaxChargeRate    float64 `mapstructure:"max_charge_rate"`    // Battery maximum charge power in kW
	MaxDischargeRate float64 `mapstructure:"max_discharge_rate"` // Battery maximum discharge power in kW
	DegradationCost  float64 `mapstructure:"degradation_cost"`   // Cost of charging/discharging the battery in SEK/kWh
}

func (b AppConfigBatterySpec) MaxKWh() float64 {
	return b.Capacity * b.MaxLevel / 100.0
}

func (b AppConfigBatterySpec) MinKWh() float64 {
	return b.Capacity * b.MinLevel / 100.0
}

type AppConfigPlanner struct {
	GridMaxPower float64 `mapstructure:"grid_max_power"` // Maximum power from/to the grid in kW
	HoursAhead   int     `mapstructure:"hours_ahead"`    // Number of hours to plan ahead
	RunAt        string  `mapstructure:"run_at"`         // How often to run the planner
}

type BatteryRegulatorStrategy struct {
	Interval        int     `mapstructure:"interval"`         // How often battery load status should be monitored in sec
	UpdateThreshold float64 `mapstructure:"update_threshold"` // Threshold in watts for when to update battery state, helps avoid frequent updates for small power changes.
}

type AppConfigLogging struct {
	// Min log level for database : "DEBUG", "INFO", "WARN", "ERROR", default: "INFO"
	DbLevel *string `mapstructure:"db_level"`
	// Log attributes format: "TEXT", "JSON", default: "JSON"
	DbAttrsFormat *string `mapstructure:"db_attrs_format"`
	// Maximum number of log entries in the database, default: 10000
	DbMaxEntries *int `mapstructure:"db_max_entries"`
	// Min log level for database console: "DEBUG", "INFO", "WARN", "ERROR", default: "INFO"
	ConsoleLevel *string `mapstructure:"console_level"`
}

func (l AppConfigLogging) GetDbLevel() slog.Level {
	return logging.LevelFromString(l.DbLevel)
}

func (l AppConfigLogging) GetDbAttrsFormat() logging.LogAttrFormat {
	if l.DbAttrsFormat == nil {
		return "JSON"
	}
	if strings.EqualFold(*l.DbAttrsFormat, "text") {
		return "TEXT"
	}
	return "JSON"
}

func (l AppConfigLogging) GetDbMaxEntries() int {
	if l.DbMaxEntries == nil {
		return 10000
	}
	return *l.DbMaxEntries
}

func (l AppConfigLogging) GetConsoleLevel() slog.Level {
	return logging.LevelFromString(l.ConsoleLevel)
}

type AppConfig struct {
	Api                      AppConfigApi
	Database                 AppConfigDatabase
	Ferroamp                 AppConfigFerroamp
	WeatherForecast          AppConfigWeatherForecast `mapstructure:"weather_forecast"`
	EnergyForecast           AppConfigEnergyForecast  `mapstructure:"energy_forecast"`
	EnergyPrice              AppConfigEnergyPrice     `mapstructure:"energy_price"`
	BatterySpec              AppConfigBatterySpec     `mapstructure:"battery_spec"`
	Planner                  AppConfigPlanner         `mapstructure:"planner"`
	BatteryRegulatorStrategy BatteryRegulatorStrategy `mapstructure:"battery_regulator_strategy"`
	Logging                  AppConfigLogging         `mapstructure:"logging"`
}

func Load(path string) (*AppConfig, error) {
	if path != "" {
		viper.SetConfigFile(path)
	} else {
		viper.AddConfigPath("config")
		viper.SetConfigName("config")
		viper.SetConfigType("yaml")
	}
	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	var c AppConfig

	if err := viper.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("unable to read config file: %w", err)
	}

	if err := viper.Unmarshal(&c); err != nil {
		return nil, fmt.Errorf("unable to unmarshal config file: %w", err)
	}

	return &c, nil
}
