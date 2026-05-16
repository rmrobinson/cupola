package store

import "encoding/json"

// Profile is a saved dashboard layout stored in SQLite.
type Profile struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Layout      string         `json:"layout"` // "landscape" or "portrait"
	GridVersion int            `json:"grid_version,omitempty"`
	Widgets     []WidgetConfig `json:"widgets"`
}

// ProfileMeta is the summary form returned by GET /api/v1/profiles.
type ProfileMeta struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Layout      string `json:"layout"`
}

// WidgetConfig describes a single widget on the canvas.
type WidgetConfig struct {
	ID     string          `json:"id"`
	Type   string          `json:"type"`
	Pos    WidgetPos       `json:"pos"`
	Config json.RawMessage `json:"config,omitempty"`
}

// WidgetPos is a CSS-Grid position and size.
type WidgetPos struct {
	Col int `json:"col"`
	Row int `json:"row"`
	W   int `json:"w"`
	H   int `json:"h"`
}

// NoteUpdate holds the optional fields for a PATCH /api/v1/notes/:id request.
type NoteUpdate struct {
	Title  *string
	Body   *string
	Author *string
	Pinned *bool
}
