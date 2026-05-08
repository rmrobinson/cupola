package domain

import "time"

type TrafficIncidents struct {
	StateBase
	Incidents []TrafficIncident `json:"incidents"`
}

func (TrafficIncidents) DomainType() DomainType { return DomainTrafficIncidents }

type TrafficIncident struct {
	ID          string     `json:"id"`
	Type        string     `json:"type"`
	Severity    string     `json:"severity"`
	Lat         float64    `json:"lat"`
	Lon         float64    `json:"lon"`
	Description string     `json:"description"`
	RoadName    string     `json:"road_name"`
	StartsAt    *time.Time `json:"starts_at,omitempty"`
	EndsAt      *time.Time `json:"ends_at,omitempty"`
	SourceURL   string     `json:"source_url,omitempty"`
}

type TrafficCameras struct {
	StateBase
	Cameras []TrafficCamera `json:"cameras"`
}

func (TrafficCameras) DomainType() DomainType { return DomainTrafficCameras }

type TrafficCamera struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Lat         float64   `json:"lat"`
	Lon         float64   `json:"lon"`
	SnapshotURL string    `json:"snapshot_url"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type TrafficRoadConditions struct {
	StateBase
	Conditions []TrafficRoadCondition `json:"conditions"`
}

func (TrafficRoadConditions) DomainType() DomainType { return DomainTrafficRoadConditions }

type TrafficRoadCondition struct {
	LocationDescription string    `json:"location_description"`
	Conditions          []string  `json:"conditions"`
	Visibility          string    `json:"visibility,omitempty"`
	Drifting            string    `json:"drifting,omitempty"`
	Region              string    `json:"region"`
	RoadwayName         string    `json:"roadway_name"`
	LastUpdated         time.Time `json:"last_updated"`
}
