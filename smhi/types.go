package smhi

import (
	"time"
)

const BASE_URL = "https://opendata-download-metfcst.smhi.se"

type WeatherForecast struct {
	Hour time.Time
	/**
	The total cloud cover, how big part of the sky is covered by clouds, (0-8 octas)
	0 - Sky clear Fine
	1 - 1/8 of sky covered or less, but not zero Fine
	2 - 2/8 of sky covered Fine
	3 - 3/8 of sky covered Partly Cloudy
	4 - 4/8 of sky covered Partly Cloudy
	5 - 5/8 of sky covered Partly Cloudy
	6 - 6/8 of sky covered Cloudy
	7 - 7/8 of sky covered or more, but not 8/8 Cloudy
	8 - 8/8 of sky completely covered, no breaks Overcast
	*/
	CloudCover uint8
	// Air temperature (°C)
	Temperature float64
	// Mean precipitation intensity (mm/h)
	Precipitation float64
}

type smhi struct {
	ApprovedTime  time.Time   `json:"approvedTime"`
	ReferenceTime time.Time   `json:"referenceTime"`
	Geometry      geometry    `json:"geometry"`
	TimeSeries    []timeEntry `json:"timeSeries"`
}

type geometry struct {
	Type        string      `json:"type"`
	Coordinates [][]float64 `json:"coordinates"`
}

type timeEntry struct {
	ValidTime  time.Time   `json:"validTime"`
	Parameters []parameter `json:"parameters"`
}

type parameter struct {
	Name      string    `json:"name"`
	LevelType string    `json:"levelType"`
	Level     int       `json:"level"`
	Unit      string    `json:"unit"`
	Values    []float64 `json:"values"`
}
