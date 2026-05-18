package rss

import (
	"context"
	"encoding/xml"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/rmrobinson/cupola/internal/config"
	"github.com/rmrobinson/cupola/internal/domain"
	"github.com/rmrobinson/cupola/internal/store"
)

// Collector polls one or more RSS/Atom feeds and aggregates their items into
// the domain.Feeds state, newest items first.
type Collector struct {
	feeds      []config.RSSFeedConfig
	stateStore *store.StateStore
	netCheck   func() bool
	mu         sync.RWMutex
	items      map[string][]domain.FeedItem // feedID → items
}

func New(feeds []config.RSSFeedConfig, stateStore *store.StateStore) *Collector {
	return &Collector{
		feeds:      feeds,
		stateStore: stateStore,
		items:      make(map[string][]domain.FeedItem),
	}
}

func (c *Collector) SetNetCheck(fn func() bool) { c.netCheck = fn }

func (c *Collector) ID() string                { return "rss" }
func (c *Collector) Domain() domain.DomainType { return domain.DomainFeeds }

func (c *Collector) Start(ctx context.Context) error {
	for _, f := range c.feeds {
		go c.runFeed(ctx, f)
	}
	return nil
}

func (c *Collector) State() domain.DomainState {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.buildState()
}

func (c *Collector) runFeed(ctx context.Context, f config.RSSFeedConfig) {
	if c.netCheck == nil || c.netCheck() {
		if err := c.fetchFeed(f); err != nil {
			log.Printf("[rss] %s initial fetch: %v", f.ID, err)
			c.stateStore.PublishSystem(store.SystemEvent{
				CollectorID: c.sourceID(f.ID), Status: "error", Message: err.Error(),
			})
		}
	}
	interval := f.PollInterval.Duration
	if interval == 0 {
		interval = 15 * time.Minute
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if c.netCheck != nil && !c.netCheck() {
				continue
			}
			if err := c.fetchFeed(f); err != nil {
				log.Printf("[rss] %s fetch: %v", f.ID, err)
				c.stateStore.PublishSystem(store.SystemEvent{
					CollectorID: c.sourceID(f.ID), Status: "error", Message: err.Error(),
				})
			} else {
				c.stateStore.PublishSystem(store.SystemEvent{CollectorID: c.sourceID(f.ID), Status: "ok"})
			}
		}
	}
}

func (c *Collector) fetchFeed(f config.RSSFeedConfig) error {
	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Get(f.URL)
	if err != nil {
		return fmt.Errorf("get %s: %w", f.URL, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("get %s: status %d", f.URL, resp.StatusCode)
	}

	items, err := parseFeed(resp.Body, f.ID, f.Category)
	if err != nil {
		return fmt.Errorf("parse %s: %w", f.URL, err)
	}

	c.mu.Lock()
	c.items[f.ID] = items
	state := c.buildState()
	c.mu.Unlock()

	c.stateStore.Set(state)
	log.Printf("[rss] %s updated: %d items", f.ID, len(items))
	return nil
}

func (c *Collector) sourceID(feedID string) string {
	return c.ID() + ":" + feedID
}

func (c *Collector) buildState() domain.Feeds {
	var all []domain.FeedItem
	for _, items := range c.items {
		all = append(all, items...)
	}
	sort.Slice(all, func(i, j int) bool {
		return all[i].PublishedAt.After(all[j].PublishedAt)
	})
	if all == nil {
		all = []domain.FeedItem{}
	}
	return domain.Feeds{
		StateBase: domain.StateBase{UpdatedAt: time.Now()},
		Items:     all,
	}
}

// ── RSS / Atom parsing ────────────────────────────────────────────────────────

// feedDoc accepts both RSS 2.0 and Atom envelopes.
type feedDoc struct {
	XMLName xml.Name

	// RSS 2.0
	RSSItems []rssItem `xml:"channel>item"`

	// Atom
	AtomEntries []atomEntry `xml:"entry"`
}

type rssItem struct {
	GUID        string `xml:"guid"`
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	Description string `xml:"description"`
	PubDate     string `xml:"pubDate"`
}

type atomEntry struct {
	ID        string   `xml:"id"`
	Title     string   `xml:"title"`
	Link      atomLink `xml:"link"`
	Summary   string   `xml:"summary"`
	Published string   `xml:"published"`
	Updated   string   `xml:"updated"`
}

type atomLink struct {
	Href string `xml:"href,attr"`
}

func parseFeed(r interface{ Read([]byte) (int, error) }, feedID, category string) ([]domain.FeedItem, error) {
	var doc feedDoc
	if err := xml.NewDecoder(r).Decode(&doc); err != nil {
		return nil, err
	}

	var items []domain.FeedItem

	for _, ri := range doc.RSSItems {
		pub := parseTime(ri.PubDate)
		id := ri.GUID
		if id == "" {
			id = ri.Link
		}
		items = append(items, domain.FeedItem{
			ID:          feedID + ":" + id,
			FeedID:      feedID,
			Category:    category,
			Title:       strings.TrimSpace(ri.Title),
			Summary:     strings.TrimSpace(stripTags(ri.Description)),
			URL:         strings.TrimSpace(ri.Link),
			PublishedAt: pub,
		})
	}

	for _, ae := range doc.AtomEntries {
		pub := parseTime(ae.Published)
		if pub.IsZero() {
			pub = parseTime(ae.Updated)
		}
		items = append(items, domain.FeedItem{
			ID:          feedID + ":" + ae.ID,
			FeedID:      feedID,
			Category:    category,
			Title:       strings.TrimSpace(ae.Title),
			Summary:     strings.TrimSpace(stripTags(ae.Summary)),
			URL:         ae.Link.Href,
			PublishedAt: pub,
		})
	}

	return items, nil
}

var timeFormats = []string{
	time.RFC1123Z,
	time.RFC1123,
	time.RFC3339,
	"Mon, 2 Jan 2006 15:04:05 -0700",
	"Mon, 2 Jan 2006 15:04:05 MST",
	"2006-01-02T15:04:05Z",
	"2006-01-02",
}

func parseTime(s string) time.Time {
	s = strings.TrimSpace(s)
	for _, f := range timeFormats {
		if t, err := time.Parse(f, s); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}

// stripTags removes HTML tags from a string for plain-text display.
func stripTags(s string) string {
	var b strings.Builder
	inTag := false
	for _, r := range s {
		switch {
		case r == '<':
			inTag = true
		case r == '>':
			inTag = false
		case !inTag:
			b.WriteRune(r)
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}
