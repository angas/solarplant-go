package smhi

import (
	"encoding/json"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"
	"testing/iotest"
	"time"
)

// A representative SNOW1gv1 response, including fields the application ignores.
const forecastJSON = `{
	"createdTime": "2026-09-05T06:00:00Z",
	"referenceTime": "2026-09-05T06:00:00Z",
	"geometry": {"type": "Point", "coordinates": [18.0, 59.0]},
	"timeSeries": [
		{"time": "2026-09-05T08:00:00Z", "data": {
			"air_temperature": -2.5, "cloud_area_fraction": 0,
			"precipitation_amount_mean": 0, "wind_speed": 3.1
		}},
		{"time": "2026-09-05T09:00:00Z", "data": {
			"air_temperature": 11.25, "cloud_area_fraction": 8,
			"precipitation_amount_mean": 0.75
		}}
	]
}`

func TestReadForecast(t *testing.T) {
	want := []WeatherForecast{
		{Hour: time.Date(2026, 9, 5, 8, 0, 0, 0, time.UTC), CloudCover: 0, Temperature: -2.5},
		{Hour: time.Date(2026, 9, 5, 9, 0, 0, 0, time.UTC), CloudCover: 8, Temperature: 11.25, Precipitation: 0.75},
	}
	for _, reader := range []struct {
		name string
		wrap func(io.Reader) io.Reader
	}{
		{"whole response", func(r io.Reader) io.Reader { return r }},
		{"one byte at a time", iotest.OneByteReader},
		{"data with EOF", iotest.DataErrReader},
	} {
		t.Run(reader.name, func(t *testing.T) {
			got, err := readForecast(reader.wrap(strings.NewReader(forecastJSON + "\n\t ")))
			if err != nil || !slices.Equal(got, want) {
				t.Fatalf("forecast = %v, error = %v; want %v", got, err, want)
			}
		})
	}
}

func TestReadForecastCompatibility(t *testing.T) {
	for _, payload := range []string{forecastJSON, `{}`, `null`, `{"timeSeries":[]}`, `{"timeSeries":null}`,
		`{"timeSeries":[{"time":"2026-09-05T08:00:00Z","data":{"air_temperature":null}}]}`} {
		var legacy smhi
		if err := json.Unmarshal([]byte(payload), &legacy); err != nil {
			t.Fatal(err)
		}
		want := make([]WeatherForecast, 0, len(legacy.TimeSeries))
		for _, entry := range legacy.TimeSeries {
			want = append(want, WeatherForecast{
				Hour: entry.Time, CloudCover: uint8(entry.Data.CloudAreaFraction),
				Temperature: entry.Data.AirTemperature, Precipitation: entry.Data.PrecipitationAmountMean,
			})
		}
		got, err := readForecast(strings.NewReader(payload))
		if err != nil || got == nil || !slices.Equal(got, want) {
			t.Fatalf("forecast = %#v, error = %v; legacy = %#v", got, err, want)
		}
	}
}

func TestReadForecastRejectsInvalidResponse(t *testing.T) {
	for _, tt := range []struct{ name, payload string }{
		{"empty body", ""},
		{"truncated JSON", forecastJSON[:len(forecastJSON)-1]},
		{"trailing value", forecastJSON + `{}`},
		{"trailing garbage", forecastJSON + `broken`},
		{"invalid time", `{"timeSeries":[{"time":"not a time"}]}`},
		{"invalid number", `{"timeSeries":[{"data":{"air_temperature":"cold"}}]}`},
		{"duplicate name", `{"timeSeries":[],"timeSeries":[]}`},
		{"invalid UTF-8", "{\"ignored\":\"\xff\"}"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := readForecast(strings.NewReader(tt.payload))
			if err == nil || got != nil {
				t.Fatalf("forecast = %v, error = %v; want an error without partial data", got, err)
			}
		})
	}
}

func TestReadForecastReadError(t *testing.T) {
	readErr := errors.New("response body interrupted")
	// Reading must continue to EOF, even after a complete JSON value was received.
	for _, payload := range []string{forecastJSON[:20], forecastJSON} {
		got, err := readForecast(io.MultiReader(strings.NewReader(payload), iotest.ErrReader(readErr)))
		if !errors.Is(err, readErr) || got != nil {
			t.Fatalf("forecast = %v, error = %v; want wrapped read error without partial data", got, err)
		}
	}
}
