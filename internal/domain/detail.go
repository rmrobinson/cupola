package domain

type Detail struct {
	Domain      string          `json:"domain"`
	ID          string          `json:"id"`
	Title       string          `json:"title"`
	Subtitle    string          `json:"subtitle,omitempty"`
	Severity    string          `json:"severity,omitempty"`
	Description string          `json:"description,omitempty"`
	Fields      []DetailField   `json:"fields,omitempty"`
	SourceURL   string          `json:"source_url,omitempty"`
	Location    *DetailLocation `json:"location,omitempty"`
}

type DetailField struct {
	Key   string `json:"key"`
	Value any    `json:"value"`
	Unit  string `json:"unit,omitempty"`
}

type DetailLocation struct {
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
}
