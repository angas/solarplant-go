CREATE TABLE time_series (
  date CHAR(10) NOT NULL,
  hour INTEGER NOT NULL,
  cloud_cover INTEGER NOT NULL,
  temperature REAL NOT NULL,
  precipitation REAL NOT NULL,
  energy_price REAL NOT NULL,
  consumption REAL NOT NULL,
  production REAL NOT NULL,
  production_lifetime REAL NOT NULL,
  battery_level REAL NOT NULL,
  battery_net_load REAL NOT NULL,
  CONSTRAINT time_series_pk PRIMARY KEY (date, hour)
);

CREATE TABLE energy_price (
  date CHAR(10) NOT NULL,
  hour INTEGER NOT NULL,
  price REAL,
  created INTEGER(4) NOT NULL DEFAULT (strftime('%s','now')),
  updated INTEGER(4) NOT NULL DEFAULT (strftime('%s','now')),
  CONSTRAINT energy_price_pk PRIMARY KEY (date, hour));
CREATE TRIGGER energy_price_updated AFTER UPDATE ON energy_price
BEGIN
  UPDATE energy_price SET updated = (strftime('%s','now'))
  WHERE rowid = NEW.rowid;
END;

CREATE TABLE weather_forecast (
  date CHAR(10) NOT NULL,
  hour INTEGER NOT NULL,
  cloud_cover INTEGER NOT NULL,
  temperature REAL NOT NULL,
  precipitation REAL NOT NULL,
  created INTEGER(4) NOT NULL DEFAULT (strftime('%s','now')),
  updated INTEGER(4) NOT NULL DEFAULT (strftime('%s','now')),
  CONSTRAINT weather_forecast_pk PRIMARY KEY (date, hour)
);
CREATE TRIGGER weather_forecast_updated AFTER UPDATE ON weather_forecast
BEGIN
  UPDATE weather_forecast SET updated = (strftime('%s','now'))
  WHERE rowid = NEW.rowid;
END;

CREATE TABLE energy_forecast (
  date CHAR(10) NOT NULL,
  hour INTEGER NOT NULL,
  production REAL NOT NULL,
  consumption REAL NOT NULL,
  created INTEGER(4) NOT NULL DEFAULT (strftime('%s','now')),
  updated INTEGER(4) NOT NULL DEFAULT (strftime('%s','now')),
  CONSTRAINT energy_forecast_pk PRIMARY KEY (date, hour));
CREATE TRIGGER energy_forecast_updated AFTER UPDATE ON energy_forecast
BEGIN
  UPDATE energy_forecast SET updated = (strftime('%s','now'))
  WHERE rowid = NEW.rowid;
END;

CREATE TABLE planning (
  date CHAR(10) NOT NULL,
  hour INTEGER NOT NULL,
  strategy CHAR(16),
  created INTEGER(4) NOT NULL DEFAULT (strftime('%s','now')),
  updated INTEGER(4) NOT NULL DEFAULT (strftime('%s','now')),
  CONSTRAINT planning_pk PRIMARY KEY (date, hour)
);
CREATE TRIGGER planning_updated AFTER	UPDATE ON planning
BEGIN
  UPDATE planning SET updated = (strftime('%s','now'))
  WHERE rowid = NEW.rowid;
END;

CREATE TABLE fa_snapshot (
  date CHAR(10) NOT NULL,
  hour INTEGER NOT NULL,
  data TEXT NOT NULL,
  CONSTRAINT fa_snapshot_pk PRIMARY KEY (date, hour)
);
