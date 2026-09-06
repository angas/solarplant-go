package smhi

import (
	"context"
	"encoding/json/v2"
	"fmt"
	"io"
	"log/slog"
	"net/http"
)

func Get(ctx context.Context, lon float64, lat float64) ([]WeatherForecast, error) {
	url := fmt.Sprintf(
		"%s/api/category/snow1g/version/1/geotype/point/lon/%0.4f/lat/%0.4f/data.json",
		BASE_URL, lon, lat)

	slog.Default().Info("fetching forecast from SMHI...", "url", url)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create smhi request: %w", err)
	}

	client := &http.Client{}
	res, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error getting SMHI forecast: %v", err)
	}
	defer res.Body.Close()

	return readForecast(res.Body)
}

func readForecast(body io.Reader) ([]WeatherForecast, error) {
	var smhi smhi
	if err := json.UnmarshalRead(body, &smhi); err != nil {
		return nil, fmt.Errorf("error decoding SMHI response: %w", err)
	}

	result := make([]WeatherForecast, 0, len(smhi.TimeSeries))
	for _, entry := range smhi.TimeSeries {
		result = append(result, WeatherForecast{
			Hour:          entry.Time,
			CloudCover:    uint8(entry.Data.CloudAreaFraction),
			Temperature:   entry.Data.AirTemperature,
			Precipitation: entry.Data.PrecipitationAmountMean,
		})
	}

	return result, nil
}
