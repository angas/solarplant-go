package chartjs

import (
	"fmt"
	"math"
)

const NoOfSlots = 96
const ColorYellow = "#ffc107d4"
const ColorRed = "#f44336d4"

func NewChart(title string) Chart {
	labels := make([]string, NoOfSlots)
	for i := range NoOfSlots {
		labels[i] = fmt.Sprintf("%02d:%02d", i/4, (i%4)*15)
	}

	chart := Chart{
		Type: "line",
		Data: ChartData{
			Labels: labels,
			Datasets: []ChartDataset{
				{
					Data:        make([]*float64, NoOfSlots),
					BorderWidth: 1,
					PointRadius: 1,
					Tension:     0.4,
					Fill:        true,
					BorderColor: ColorYellow,
					YAxisID:     "YAxis1",
				},
				{
					Data:        make([]*float64, NoOfSlots),
					BorderWidth: 1,
					PointRadius: 1,
					Tension:     0.4,
					Fill:        true,
					BorderColor: ColorRed,
					YAxisID:     "YAxis2",
				},
			},
		},
		Options: ChartOptions{
			Responsive: true,
			Plugins: ChartPlugins{
				Legend: ChartLegend{Display: false},
				Title:  ChartTitle{Display: false},
			},
			Scales: map[string]ChartScale{
				"YAxis1": {
					Type:     "linear",
					Display:  true,
					Position: "left",
					Title:    ChartScaleTitle{Display: true, Text: "", Color: ColorYellow}},
				"YAxis2": {
					Type:     "linear",
					Display:  true,
					Position: "right",
					Title:    ChartScaleTitle{Display: true, Text: "", Color: ColorRed}},
			},
		},
	}

	if title != "" {
		chart.Options.Plugins.Title = ChartTitle{Display: true, Text: title}
	}

	return chart
}

func (cs ChartScale) WithTitle(title string) ChartScale {
	cs.Title.Text = title
	return cs
}

func (cs ChartScale) WithMinAndMax(min, max float64) ChartScale {
	cs.Min = &min
	cs.Max = &max
	return cs
}

func FixedFloat64(num float64, precision int) *float64 {
	p := math.Pow(10, float64(precision))
	rounded := math.Round(num * p)
	result := rounded / p
	return &result
}
