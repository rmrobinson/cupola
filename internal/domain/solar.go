package domain

import "time"

type SolarWeatherCurrent struct {
	StateBase
	KpIndex        float64 `json:"kp_index"`
	KpDescription  string  `json:"kp_description"`
	FlareClass     *string `json:"flare_class,omitempty"`
	AuroraViewable bool    `json:"aurora_viewable"`
	Region         int     `json:"region"`
}

func (SolarWeatherCurrent) DomainType() DomainType { return DomainSolarWeatherCurrent }

type SolarWeatherForecast struct {
	StateBase
	Periods []SolarForecastPeriod `json:"periods"`
}

func (SolarWeatherForecast) DomainType() DomainType { return DomainSolarWeatherForecast }

type SolarForecastPeriod struct {
	StartsAt       time.Time `json:"starts_at"`
	EndsAt         time.Time `json:"ends_at"`
	KpExpected     float64   `json:"kp_expected"`
	KpDescription  string    `json:"kp_description"`
	AuroraViewable bool      `json:"aurora_viewable"`
	Summary        string    `json:"summary"`
}
