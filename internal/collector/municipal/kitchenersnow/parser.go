// Package kitchenersnow parses the City of Kitchener snow-events news feed
// (https://www.kitchener.ca/news/snow-events/) into municipal.events.
// Register via import side-effect: _ "github.com/rmrobinson/cupola/internal/collector/municipal/kitchenersnow"
package kitchenersnow

import (
	"fmt"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"sort"
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
type Parser struct {
	Now      func() time.Time
	Location *time.Location
}

type snowPost struct {
	event       domain.MunicipalEvent
	action      snowAction
	effectiveAt time.Time
}

type snowAction int

const (
	snowActionUnknown snowAction = iota
	snowActionDeclare
	snowActionExtend
	snowActionCancel
)

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

	loc := p.location()
	now := p.now().In(loc)

	var posts []snowPost
	walkNodes(doc, func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "li" && hasClass(n, "gs-feed-list-item") {
			if post := extractPost(n, base, loc); post != nil {
				posts = append(posts, *post)
			}
		}
	})
	return synthesizeCurrentEvent(posts, now), nil
}

func (p *Parser) now() time.Time {
	if p.Now != nil {
		return p.Now()
	}
	return time.Now()
}

func (p *Parser) location() *time.Location {
	if p.Location != nil {
		return p.Location
	}
	loc, err := time.LoadLocation("America/Toronto")
	if err != nil {
		return time.Local
	}
	return loc
}

func extractPost(li *html.Node, base *url.URL, loc *time.Location) *snowPost {
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
	ev := domain.MunicipalEvent{
		ID:          id,
		Title:       title,
		Description: description,
		EventType:   "snow-event",
		URL:         itemURL,
		PublishedAt: pub,
	}

	action, effectiveAt := classifySnowPost(title, description, pub, loc)
	return &snowPost{
		event:       ev,
		action:      action,
		effectiveAt: effectiveAt,
	}
}

func synthesizeCurrentEvent(posts []snowPost, now time.Time) []domain.MunicipalEvent {
	sort.SliceStable(posts, func(i, j int) bool {
		if posts[i].event.PublishedAt.Equal(posts[j].event.PublishedAt) {
			return posts[i].effectiveAt.Before(posts[j].effectiveAt)
		}
		return posts[i].event.PublishedAt.Before(posts[j].event.PublishedAt)
	})

	var (
		active bool
		start  time.Time
		end    time.Time
		latest *snowPost
	)

	for i := range posts {
		post := &posts[i]
		if post.action == snowActionUnknown || post.effectiveAt.IsZero() {
			continue
		}

		switch post.action {
		case snowActionDeclare:
			start = post.effectiveAt
			end = start.Add(24 * time.Hour)
			active = true
		case snowActionExtend:
			if start.IsZero() {
				start = post.event.PublishedAt
			}
			end = post.effectiveAt
			active = true
		case snowActionCancel:
			end = post.effectiveAt
			active = false
		}
		latest = post
	}

	if !active || latest == nil || start.IsZero() || end.IsZero() || now.Before(start) || !now.Before(end) {
		return []domain.MunicipalEvent{}
	}

	url := latest.event.URL
	return []domain.MunicipalEvent{{
		ID:          "kitchener.snow:active",
		Title:       "Kitchener snow event active",
		Description: latest.event.Description,
		EventType:   "snow-event",
		StartsAt:    &start,
		EndsAt:      &end,
		URL:         url,
		PublishedAt: latest.event.PublishedAt,
	}}
}

func classifySnowPost(title, description string, published time.Time, loc *time.Location) (snowAction, time.Time) {
	text := normalizeSpaces(title + " " + description)
	titleLower := strings.ToLower(normalizeSpaces(title))
	lower := strings.ToLower(text)

	switch {
	case isSnowCancellation(titleLower, lower):
		return snowActionCancel, extractSnowTime(text, published, loc, "effective", "as of")
	case strings.Contains(lower, "extend") && strings.Contains(lower, "snow event"):
		return snowActionExtend, extractSnowTime(text, published, loc, "until")
	case strings.Contains(lower, "declared a snow event"):
		return snowActionDeclare, extractSnowTime(text, published, loc, "starting at", "starting", "residents have until")
	default:
		return snowActionUnknown, time.Time{}
	}
}

func isSnowCancellation(titleLower, lower string) bool {
	if strings.Contains(titleLower, "cancel") && strings.Contains(titleLower, "snow event") {
		return true
	}
	for _, phrase := range []string{
		"cancelled its snow event",
		"cancelling the current snow event",
		"cancels snow event",
	} {
		if strings.Contains(lower, phrase) {
			return true
		}
	}
	return false
}

func extractSnowTime(text string, published time.Time, loc *time.Location, markers ...string) time.Time {
	lower := strings.ToLower(text)
	for _, marker := range markers {
		idx := strings.Index(lower, marker)
		if idx < 0 {
			continue
		}
		phrase := strings.TrimSpace(text[idx+len(marker):])
		if t, ok := parseSnowTimestamp(phrase, published, loc); ok {
			return t
		}
	}
	return time.Time{}
}

var (
	timeRE  = regexp.MustCompile(`(?i)(\d{1,2})(?::(\d{2}))?\s*(a\.m\.|p\.m\.|am|pm)`)
	monthRE = regexp.MustCompile(`(?i)\b(January|February|March|April|May|June|July|August|September|October|November|December|Jan|Feb|Mar|Apr|Jun|Jul|Aug|Sept|Sep|Oct|Nov|Dec)\s+(\d{1,2})(?:,\s*(\d{4}))?`)
)

func parseSnowTimestamp(s string, published time.Time, loc *time.Location) (time.Time, bool) {
	s = normalizeSpaces(strings.ReplaceAll(s, "(noon)", ""))
	tm := timeRE.FindStringSubmatch(s)
	md := monthRE.FindStringSubmatch(s)
	if tm == nil || md == nil {
		return time.Time{}, false
	}

	hour := atoi(tm[1])
	minute := 0
	if tm[2] != "" {
		minute = atoi(tm[2])
	}
	ampm := strings.ToLower(strings.ReplaceAll(tm[3], ".", ""))
	if ampm == "pm" && hour != 12 {
		hour += 12
	}
	if ampm == "am" && hour == 12 {
		hour = 0
	}

	year := published.Year()
	if md[3] != "" {
		year = atoi(md[3])
	}
	month := parseMonth(md[1])
	day := atoi(md[2])
	if month == 0 || day == 0 {
		return time.Time{}, false
	}

	return time.Date(year, month, day, hour, minute, 0, 0, loc), true
}

func normalizeSpaces(s string) string {
	return strings.Join(strings.Fields(strings.ReplaceAll(s, "\u00a0", " ")), " ")
}

func parseMonth(s string) time.Month {
	switch strings.ToLower(s) {
	case "january", "jan":
		return time.January
	case "february", "feb":
		return time.February
	case "march", "mar":
		return time.March
	case "april", "apr":
		return time.April
	case "may":
		return time.May
	case "june", "jun":
		return time.June
	case "july", "jul":
		return time.July
	case "august", "aug":
		return time.August
	case "september", "sept", "sep":
		return time.September
	case "october", "oct":
		return time.October
	case "november", "nov":
		return time.November
	case "december", "dec":
		return time.December
	default:
		return 0
	}
}

func atoi(s string) int {
	var n int
	for _, r := range s {
		if r < '0' || r > '9' {
			break
		}
		n = n*10 + int(r-'0')
	}
	return n
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
