// Package kitchenersnow parses the City of Kitchener snow-events news feed
// (https://www.kitchener.ca/news/snow-events/) into municipal.events.
// Register via import side-effect: _ "github.com/rmrobinson/cupola/internal/collector/municipal/kitchenersnow"
package kitchenersnow

import (
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/html"

	"github.com/rmrobinson/cupola/internal/collector/municipal"
	"github.com/rmrobinson/cupola/internal/domain"
)

func init() {
	municipal.RegisterEventsParser("kitchener.snow", func() municipal.EventsParser {
		return &Parser{}
	})
}

// Parser implements municipal.EventsParser for the Kitchener snow-events page.
type Parser struct{}

func (p *Parser) Parse(rawURL string) ([]domain.MunicipalEvent, error) {
	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Get(rawURL)
	if err != nil {
		return nil, fmt.Errorf("kitchener.snow: get %s: %w", rawURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("kitchener.snow: get %s: status %d", rawURL, resp.StatusCode)
	}

	base, _ := url.Parse(rawURL)

	doc, err := html.Parse(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("kitchener.snow: parse HTML: %w", err)
	}

	var items []domain.MunicipalEvent
	walkNodes(doc, func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "li" && hasClass(n, "gs-feed-list-item") {
			if item := extractEvent(n, base); item != nil {
				items = append(items, *item)
			}
		}
	})
	return items, nil
}

func extractEvent(li *html.Node, base *url.URL) *domain.MunicipalEvent {
	var title, href, description, dateStr string
	walkNodes(li, func(n *html.Node) {
		if n.Type != html.ElementNode {
			return
		}
		switch {
		case n.Data == "a" && hasClass(n, "gs-feed-list-title"):
			title = strings.TrimSpace(textContent(n))
			href = attrVal(n, "href")
		case n.Data == "span" && hasClass(n, "gs-feed-list-description"):
			description = strings.TrimSpace(textContent(n))
		case n.Data == "span" && hasClass(n, "gs-feed-list-date"):
			dateStr = strings.TrimSpace(textContent(n))
		}
	})

	if title == "" {
		return nil
	}

	var itemURL *string
	if href != "" {
		if ref, err := url.Parse(href); err == nil {
			abs := base.ResolveReference(ref).String()
			itemURL = &abs
		}
	}

	pub := parseDate(dateStr)
	id := "kitchener.snow:" + href

	return &domain.MunicipalEvent{
		ID:          id,
		Title:       title,
		Description: description,
		EventType:   "snow-event",
		URL:         itemURL,
		PublishedAt: pub,
	}
}

func parseDate(s string) time.Time {
	s = strings.TrimSpace(s)
	for _, f := range []string{"Jan 2, 2006", "January 2, 2006"} {
		if t, err := time.Parse(f, s); err == nil {
			return t.UTC()
		}
	}
	log.Printf("[kitchener.snow] unparseable date %q", s)
	return time.Time{}
}

// ── HTML helpers ──────────────────────────────────────────────────────────────

func walkNodes(n *html.Node, fn func(*html.Node)) {
	fn(n)
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		walkNodes(c, fn)
	}
}

func hasClass(n *html.Node, class string) bool {
	for _, a := range n.Attr {
		if a.Key == "class" {
			for _, c := range strings.Fields(a.Val) {
				if c == class {
					return true
				}
			}
		}
	}
	return false
}

func attrVal(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func textContent(n *html.Node) string {
	var sb strings.Builder
	walkNodes(n, func(c *html.Node) {
		if c.Type == html.TextNode {
			sb.WriteString(c.Data)
		}
	})
	return sb.String()
}
