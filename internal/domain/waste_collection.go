package domain

type ExtraCollection struct {
	Date      string `json:"date"`
	DayOfWeek string `json:"day_of_week"`
	Type      string `json:"type"`
	IsToday   bool   `json:"is_today"`
}

type WasteCollection struct {
	StateBase
	Date             string            `json:"date"`                       // "YYYY-MM-DD"; empty when no entry found for this week
	DayOfWeek        string            `json:"day_of_week"`                // empty when no entry found
	Collections      []string          `json:"collections"`                // nil when no entry found
	IsToday          bool              `json:"is_today"`
	ExtraCollections []ExtraCollection `json:"extra_collections,omitempty"` // off-schedule pickups this week
}

func (WasteCollection) DomainType() DomainType { return DomainWasteCollection }
