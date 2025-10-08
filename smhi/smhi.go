package smhi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
)

func Get(ctx context.Context, lon float64, lat float64) ([]WeatherForecast, error) {
	url := fmt.Sprintf(
		"%s/api/category/pmp3g/version/2/geotype/point/lon/%0.4f/lat/%0.4f/data.json",
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

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading SMHI response body: %v", err)
	}

	var smhi smhi
	if err := json.Unmarshal(body, &smhi); err != nil {
		return nil, fmt.Errorf("error unmarshaling SMHI json: %v", err)
	}

	result := make([]WeatherForecast, 0)
	for _, entry := range smhi.TimeSeries {
		result = append(result, WeatherForecast{
			Hour:          entry.ValidTime,
			CloudCover:    uint8(getParameter(entry.Parameters, "tcc_mean")),
			Temperature:   getParameter(entry.Parameters, "t"),
			Precipitation: getParameter(entry.Parameters, "pmean"),
		})
	}

	return result, nil
}

func getParameter(params []parameter, name string) float64 {
	for _, param := range params {
		if param.Name == name {
			return param.Values[0]
		}
	}

	return 0
}
