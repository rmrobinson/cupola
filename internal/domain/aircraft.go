package domain

import "time"

type Aircraft struct {
	StateBase
	Aircraft []AircraftTarget `json:"aircraft"`
}

func (Aircraft) DomainType() DomainType { return DomainAircraft }

type AircraftTarget struct {
	ICAO      string    `json:"icao"`
	Callsign  *string   `json:"callsign,omitempty"`
	Flight    *string   `json:"flight,omitempty"`
	Lat       float64   `json:"lat"`
	Lon       float64   `json:"lon"`
	AltFt     int       `json:"alt_ft"`
	Track     *float64  `json:"track,omitempty"`
	SpeedKts  *float64  `json:"speed_kts,omitempty"`
	VertRate  *int      `json:"vert_rate,omitempty"`
	Squawk    *string   `json:"squawk,omitempty"`
	OnGround  bool      `json:"on_ground"`
	UpdatedAt time.Time `json:"updated_at"`
}
