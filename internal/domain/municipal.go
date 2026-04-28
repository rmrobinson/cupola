package domain

import "time"

// MunicipalEvents holds informational notices — scheduled or low-urgency.
// Not shown in the alerts widget; has its own municipal-events widget.
type MunicipalEvents struct {
	StateBase
	Events []MunicipalEvent `json:"events"`
}

func (MunicipalEvents) DomainType() DomainType { return DomainMunicipalEvents }

type MunicipalEvent struct {
	ID          string     `json:"id"`
	SourceID    string     `json:"source_id"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	EventType   string     `json:"event_type"`
	StartsAt    *time.Time `json:"starts_at,omitempty"`
	EndsAt      *time.Time `json:"ends_at,omitempty"`
	URL         *string    `json:"url,omitempty"`
	PublishedAt time.Time  `json:"published_at"`
}

// MunicipalAlerts holds urgent notices shown in the alerts widget
// alongside weather.alerts and transit.alerts.
type MunicipalAlerts struct {
	StateBase
	Alerts []MunicipalAlert `json:"alerts"`
}

func (MunicipalAlerts) DomainType() DomainType { return DomainMunicipalAlerts }

type MunicipalAlert struct {
	ID          string        `json:"id"`
	SourceID    string        `json:"source_id"`
	Title       string        `json:"title"`
	Description string        `json:"description"`
	AlertType   string        `json:"alert_type"`
	Severity    AlertSeverity `json:"severity"`
	Area        *string       `json:"area,omitempty"`
	StartsAt    *time.Time    `json:"starts_at,omitempty"`
	EndsAt      *time.Time    `json:"ends_at,omitempty"`
	URL         *string       `json:"url,omitempty"`
	PublishedAt time.Time     `json:"published_at"`
}
