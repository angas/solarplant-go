package chartjs

type Chart struct {
	Type    string       `json:"type"`
	Data    ChartData    `json:"data"`
	Options ChartOptions `json:"options"`
}

type ChartData struct {
	Labels   []string       `json:"labels"`
	Datasets []ChartDataset `json:"datasets"`
}

type ChartDataset struct {
	Data        []*float64 `json:"data,omitempty"`
	BorderWidth int        `json:"borderWidth"`
	PointRadius float64    `json:"pointRadius"`
	Tension     float64    `json:"tension"`
	Fill        bool       `json:"fill"`
	BorderColor string     `json:"borderColor"`
	YAxisID     string     `json:"yAxisID,omitempty"`
}

type ChartOptions struct {
	Responsive bool                  `json:"responsive"`
	Plugins    ChartPlugins          `json:"plugins"`
	Scales     map[string]ChartScale `json:"scales"`
}

type ChartPlugins struct {
	Legend ChartLegend `json:"legend"`
	Title  ChartTitle  `json:"title"`
}

type ChartLegend struct {
	Display bool `json:"display"`
}

type ChartTitle struct {
	Display bool   `json:"display"`
	Text    string `json:"text"`
}

type ChartScale struct {
	Type     string          `json:"type"`
	Display  bool            `json:"display"`
	Position string          `json:"position"`
	Min      *float64        `json:"min,omitempty"`
	Max      *float64        `json:"max,omitempty"`
	Title    ChartScaleTitle `json:"title"`
}

type ChartScaleTitle struct {
	Display bool   `json:"display"`
	Text    string `json:"text"`
	Color   string `json:"color,omitempty"`
}
