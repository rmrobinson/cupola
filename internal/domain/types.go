package domain

import "time"

// DomainType identifies a category of data produced by a collector.
type DomainType string

const (
	DomainWeatherCurrent        DomainType = "weather.current"
	DomainWeatherForecast       DomainType = "weather.forecast"
	DomainWeatherForecastHourly DomainType = "weather.forecast.hourly"
	DomainWeatherAlerts         DomainType = "weather.alerts"
	DomainWeatherAirQuality     DomainType = "weather.air_quality"
	DomainWeatherPollen         DomainType = "weather.pollen"
	DomainSolarWeatherCurrent   DomainType = "solar.weather.current"
	DomainSolarWeatherForecast  DomainType = "solar.weather.forecast"
	DomainTransitArrivals       DomainType = "transit.arrivals"
	DomainTransitVehicles       DomainType = "transit.vehicles"
	DomainTransitAlerts         DomainType = "transit.alerts"
	DomainTrafficIncidents      DomainType = "traffic.incidents"
	DomainTrafficCameras        DomainType = "traffic.cameras"
	DomainTrafficRoadConditions DomainType = "traffic.road_conditions"
	DomainAircraft              DomainType = "aircraft"
	DomainAstro                 DomainType = "astro"
	DomainFlagStatus            DomainType = "flag.status"
	DomainFeeds                 DomainType = "feeds"
	DomainNotes                 DomainType = "notes"
	DomainHome                  DomainType = "home"
	DomainWaterwayConditions    DomainType = "waterway.conditions"
	DomainMunicipalEvents       DomainType = "municipal.events"
	DomainMunicipalAlerts       DomainType = "municipal.alerts"
	DomainWasteCollection       DomainType = "waste.collection"
)

// DomainState is the common interface satisfied by all domain state types.
// StateUpdatedAt is used instead of UpdatedAt to avoid a name collision with
// the UpdatedAt field present on every concrete state struct.
type DomainState interface {
	DomainType() DomainType
	StateUpdatedAt() time.Time
}

// StateBase embeds into every domain state struct to satisfy DomainState.StateUpdatedAt
// and to provide a consistent JSON field name across all state payloads.
type StateBase struct {
	UpdatedAt time.Time `json:"updated_at"`
}

func (s StateBase) StateUpdatedAt() time.Time { return s.UpdatedAt }
