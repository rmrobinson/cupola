package domain

import "time"

type Feeds struct {
	StateBase
	Items []FeedItem `json:"items"`
}

func (Feeds) DomainType() DomainType { return DomainFeeds }

type FeedItem struct {
	ID          string    `json:"id"`
	FeedID      string    `json:"feed_id"`
	Category    string    `json:"category"`
	Title       string    `json:"title"`
	Summary     string    `json:"summary"`
	URL         string    `json:"url"`
	PublishedAt time.Time `json:"published_at"`
}
