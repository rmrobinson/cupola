package domain

import "time"

type Astro struct {
	StateBase
	Sunrise          time.Time  `json:"sunrise"`
	Sunset           time.Time  `json:"sunset"`
	SolarNoon        time.Time  `json:"solar_noon"`
	CivilDawn        time.Time  `json:"civil_dawn"`
	CivilDusk        time.Time  `json:"civil_dusk"`
	MoonPhase        float64    `json:"moon_phase"`
	MoonPhaseName    string     `json:"moon_phase_name"`
	MoonIllumination float64    `json:"moon_illumination"`
	MoonRise         *time.Time `json:"moon_rise,omitempty"`
	MoonSet          *time.Time `json:"moon_set,omitempty"`
}

func (Astro) DomainType() DomainType { return DomainAstro }
