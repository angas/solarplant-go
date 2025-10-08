package elprisetjustnu

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"time"

	"github.com/angas/solarplant-go/timex"
	"github.com/angas/solarplant-go/types"
)

type rawPrice struct {
	SEKPerKWh float64 `json:"SEK_per_kWh"`
	EURPerKWh float64 `json:"EUR_per_kWh"`
	EXR       float64 `json:"EXR"`
	TimeStart string  `json:"time_start"`
	TimeEnd   string  `json:"time_end"`
}

type ElPrisetJustNu struct {
	area string
}

func New(area string) ElPrisetJustNu {
	return ElPrisetJustNu{area: area}
}

func (e ElPrisetJustNu) GetEnergyPrices(ctx context.Context) ([]types.EnergyPrice, error) {

	t := time.Now()
	today, err := e.getEnergyPrices(ctx, t.Year(), int(t.Month()), t.Day())
	if err != nil {
		return nil, fmt.Errorf("failed to fetch prices from elprisetjustnu for today: %w", err)
	}

	if t.Before(time.Date(t.Year(), t.Month(), t.Day(), 14, 15, 0, 0, time.Local)) {
		return today, nil
	}

	t = t.AddDate(0, 0, 1)
	tomorrow, err := e.getEnergyPrices(ctx, t.Year(), int(t.Month()), t.Day())
	if err != nil {
		return nil, fmt.Errorf("failed to fetch prices from elprisetjustnu for tomorrow: %w", err)
	}

	return append(today, tomorrow...), nil
}

func (e ElPrisetJustNu) getEnergyPrices(ctx context.Context, y, m, d int) ([]types.EnergyPrice, error) {
	url := fmt.Sprintf("https://www.elprisetjustnu.se/api/v1/prices/%d/%02d-%02d_%s.json",
		y, m, d, e.area)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	client := &http.Client{}
	res, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch prices: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode == http.StatusNotFound {
		return []types.EnergyPrice{}, nil
	}

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", res.StatusCode)
	}

	var rawPrices []rawPrice
	if err := json.NewDecoder(res.Body).Decode(&rawPrices); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	prices := make([]types.EnergyPrice, 0, len(rawPrices))
	for _, raw := range rawPrices {
		start, err := timex.ParseBucketTime(raw.TimeStart, timex.BucketSize15Minutes)
		if err != nil {
			return nil, fmt.Errorf("failed to parse elprisetjustnu start time (%s): %w", raw.TimeStart, err)
		}
		if slices.ContainsFunc(prices, func(p types.EnergyPrice) bool { return p.StartAt == start }) {
			continue
		}
		prices = append(prices, types.EnergyPrice{StartAt: start, Price: raw.SEKPerKWh})
	}

	return prices, nil
}
