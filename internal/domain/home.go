package domain

import "time"

type Home struct {
	StateBase
	Cameras  []HomeCamera    `json:"cameras"`
	Sensors  []HomeSensor    `json:"sensors"`
	Presence []PresenceEntry `json:"presence"`
}

func (Home) DomainType() DomainType { return DomainHome }

type HomeCamera struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	StreamURL   string `json:"stream_url"`
	SnapshotURL string `json:"snapshot_url"`
}

type HomeSensor struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Type      string     `json:"type"`
	Value     any        `json:"value"`
	Unit      *string    `json:"unit,omitempty"`
	UpdatedAt time.Time  `json:"updated_at"`
}

type PresenceEntry struct {
	Name    string    `json:"name"`
	Present bool      `json:"present"`
	Since   time.Time `json:"since"`
}
