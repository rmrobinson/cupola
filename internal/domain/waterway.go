package domain

import "time"

type WaterwayConditions struct {
	StateBase
	Gauges []WaterwayGauge `json:"gauges"`
}

func (WaterwayConditions) DomainType() DomainType { return DomainWaterwayConditions }

type WaterwayGauge struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	WaterwayName   string    `json:"waterway_name"`
	Lat            float64   `json:"lat"`
	Lon            float64   `json:"lon"`
	LevelM         *float64  `json:"level_m,omitempty"`
	FlowCMS        *float64  `json:"flow_cms,omitempty"`
	TempC          *float64  `json:"temp_c,omitempty"`
	AdvisoryStatus string    `json:"advisory_status"`
	AdvisoryText   *string   `json:"advisory_text,omitempty"`
	UpdatedAt      time.Time `json:"updated_at"`
}
