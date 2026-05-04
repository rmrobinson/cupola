package domain

type WasteCollection struct {
	StateBase
	Date        string   `json:"date"`         // "YYYY-MM-DD"; empty when no entry found for this week
	DayOfWeek   string   `json:"day_of_week"`  // empty when no entry found
	Collections []string `json:"collections"`  // nil when no entry found
	IsToday     bool     `json:"is_today"`
}

func (WasteCollection) DomainType() DomainType { return DomainWasteCollection }
