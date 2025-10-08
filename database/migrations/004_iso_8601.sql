--------------------------------------------------------------------------------
-- energy_forecast
--------------------------------------------------------------------------------
ALTER TABLE energy_forecast RENAME TO energy_forecast_obsolete;
DROP TRIGGER energy_forecast_updated;

CREATE TABLE energy_forecast (
  start_at CHAR(20) NOT NULL PRIMARY KEY,
  production REAL NOT NULL,
  consumption REAL NOT NULL,
  created INTEGER(4) NOT NULL DEFAULT (strftime('%s','now')),
  updated INTEGER(4) NOT NULL DEFAULT (strftime('%s','now'))
);
CREATE TRIGGER energy_forecast_updated AFTER UPDATE ON energy_forecast
BEGIN
  UPDATE energy_forecast SET updated = (strftime('%s','now'))
  WHERE rowid = NEW.rowid;
END;

INSERT INTO energy_forecast (start_at, production, consumption, created, updated)
SELECT date || 'T' || printf('%02d', hour) || ':00:00Z' AS start_at, production, consumption, created, updated FROM energy_forecast_obsolete;

--------------------------------------------------------------------------------
-- energy_price
--------------------------------------------------------------------------------
ALTER TABLE energy_price RENAME TO energy_price_obsolete;
DROP TRIGGER energy_price_updated;

CREATE TABLE energy_price (
  start_at CHAR(20) NOT NULL PRIMARY KEY,
  price REAL,
  created INTEGER(4) NOT NULL DEFAULT (strftime('%s','now')),
  updated INTEGER(4) NOT NULL DEFAULT (strftime('%s','now')));
CREATE TRIGGER energy_price_updated AFTER UPDATE ON energy_price
BEGIN
  UPDATE energy_price SET updated = (strftime('%s','now'))
  WHERE rowid = NEW.rowid;
END;

INSERT INTO energy_price (start_at, price, created, updated)
SELECT date || 'T' || printf('%02d', hour) || ':00:00Z' AS start_at, price, created, updated FROM energy_price_obsolete;

--------------------------------------------------------------------------------
-- fa_snapshot
--------------------------------------------------------------------------------
ALTER TABLE fa_snapshot RENAME TO fa_snapshot_obsolete;

CREATE TABLE fa_snapshot (
  timestamp CHAR(20) NOT NULL PRIMARY KEY,
  data TEXT NOT NULL);

INSERT INTO fa_snapshot (timestamp, data)
SELECT date || 'T' || printf('%02d', hour) || ':00:00Z' AS timestamp, data FROM fa_snapshot_obsolete;

--------------------------------------------------------------------------------
-- planning
--------------------------------------------------------------------------------
ALTER TABLE planning RENAME TO planning_obsolete;
--DROP TRIGGER planning_pk;

CREATE TABLE planning (
  start_at CHAR(20) NOT NULL PRIMARY KEY,
  strategy CHAR(16),
  created INTEGER(4) NOT NULL DEFAULT (strftime('%s','now')),
  updated INTEGER(4) NOT NULL DEFAULT (strftime('%s','now')));
CREATE TRIGGER planning_pk AFTER UPDATE ON planning
BEGIN
  UPDATE planning SET updated = (strftime('%s','now'))
  WHERE rowid = NEW.rowid;
END;

INSERT INTO planning (start_at, strategy, created, updated)
SELECT date || 'T' || printf('%02d', hour) || ':00:00Z' AS start_at, strategy, created, updated FROM planning_obsolete;

--------------------------------------------------------------------------------
-- time_series
--------------------------------------------------------------------------------
ALTER TABLE time_series RENAME TO time_series_obsolete;

CREATE TABLE time_series (
  timestamp CHAR(20) NOT NULL PRIMARY KEY,
  cloud_cover INTEGER NOT NULL,
  temperature REAL NOT NULL,
  precipitation REAL NOT NULL,
  energy_price_avg REAL NOT NULL,
  consumption REAL NOT NULL,
  production REAL NOT NULL,
  production_lifetime REAL NOT NULL,
  battery_level REAL NOT NULL,
  battery_net_load REAL NOT NULL,
  production_estimated REAL NOT NULL,
  consumption_estimated REAL NOT NULL,
  grid_import REAL NOT NULL,
  grid_export REAL NOT NULL,
  cash_flow REAL NOT NULL,
  strategy TEXT NOT NULL DEFAULT 'default'
);

INSERT INTO time_series (
  timestamp,
  cloud_cover,
  temperature,
  precipitation,
  energy_price_avg,
  consumption,
  production,
  production_lifetime,
  battery_level,
  battery_net_load,
  production_estimated,
  consumption_estimated,
  grid_import,
  grid_export,
  cash_flow,
  strategy)
SELECT
  date || 'T' || printf('%02d', hour) || ':00:00Z' AS timestamp,
  cloud_cover,
  temperature,
  precipitation,
  energy_price,
  consumption,
  production,
  production_lifetime,
  battery_level,
  battery_net_load,
  production_estimated,
  consumption_estimated,
  grid_import,
  grid_export,
  cash_flow,
  strategy
FROM time_series_obsolete;

--------------------------------------------------------------------------------
-- weather_forecast
--------------------------------------------------------------------------------
ALTER TABLE weather_forecast RENAME TO weather_forecast_obsolete;
DROP TRIGGER weather_forecast_updated;

CREATE TABLE weather_forecast (
  start_at CHAR(20) NOT NULL PRIMARY KEY,
  cloud_cover INTEGER NOT NULL,
  temperature REAL NOT NULL,
  precipitation REAL NOT NULL,
  created INTEGER(4) NOT NULL DEFAULT (strftime('%s','now')),
  updated INTEGER(4) NOT NULL DEFAULT (strftime('%s','now'))
);
CREATE TRIGGER weather_forecast_updated AFTER UPDATE ON weather_forecast
BEGIN
  UPDATE weather_forecast SET updated = (strftime('%s','now'))
  WHERE rowid = NEW.rowid;
END;

INSERT INTO weather_forecast (start_at, cloud_cover, temperature, precipitation, created, updated)
SELECT date || 'T' || printf('%02d', hour) || ':00:00Z' AS start_at, cloud_cover, temperature, precipitation, created, updated FROM weather_forecast_obsolete;
