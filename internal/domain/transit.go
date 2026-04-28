package domain

import "time"

// TransitArrivals state is keyed by "{agency}:{route}:{stop_id}".
// Only subscribed combinations are populated.
type TransitArrivals struct {
	StateBase
	Stops map[string]StopArrivals `json:"stops"`
}

func (TransitArrivals) DomainType() DomainType { return DomainTransitArrivals }

type StopArrivals struct {
	AgencyID  string    `json:"agency_id"`
	RouteID   string    `json:"route_id"`
	RouteName string    `json:"route_name"`
	StopID    string    `json:"stop_id"`
	StopName  string    `json:"stop_name"`
	Arrivals  []Arrival `json:"arrivals"`
}

type Arrival struct {
	TripID    string     `json:"trip_id"`
	Headsign  string     `json:"headsign"`
	Scheduled time.Time  `json:"scheduled"`
	Predicted *time.Time `json:"predicted,omitempty"`
	Delay     *int       `json:"delay,omitempty"`
	VehicleID *string    `json:"vehicle_id,omitempty"`
	Occupancy *string    `json:"occupancy,omitempty"`
}

type TransitVehicles struct {
	StateBase
	Vehicles []TransitVehicle `json:"vehicles"`
}

func (TransitVehicles) DomainType() DomainType { return DomainTransitVehicles }

type TransitVehicle struct {
	AgencyID  string   `json:"agency_id"`
	VehicleID string   `json:"vehicle_id"`
	RouteID   string   `json:"route_id"`
	RouteName string   `json:"route_name"`
	Lat       float64  `json:"lat"`
	Lon       float64  `json:"lon"`
	Bearing   *float64 `json:"bearing,omitempty"`
	Speed     *float64 `json:"speed,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
}

type TransitAlerts struct {
	StateBase
	Alerts []TransitAlert `json:"alerts"`
}

func (TransitAlerts) DomainType() DomainType { return DomainTransitAlerts }

type TransitAlert struct {
	ID             string        `json:"id"`
	AgencyID       string        `json:"agency_id"`
	Title          string        `json:"title"`
	Description    string        `json:"description"`
	Severity       AlertSeverity `json:"severity"`
	AffectedRoutes []string      `json:"affected_routes"`
	StartsAt       *time.Time    `json:"starts_at,omitempty"`
	EndsAt         *time.Time    `json:"ends_at,omitempty"`
}
