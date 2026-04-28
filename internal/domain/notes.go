package domain

import "time"

type Notes struct {
	StateBase
	Notes []Note `json:"notes"`
}

func (Notes) DomainType() DomainType { return DomainNotes }

type Note struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	Author    string    `json:"author"`
	Pinned    bool      `json:"pinned"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
