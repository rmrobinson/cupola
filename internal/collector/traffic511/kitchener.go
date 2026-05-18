package traffic511

import (
	"context"
	"fmt"
	"hash/fnv"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"golang.org/x/net/html"

	"github.com/rmrobinson/cupola/internal/domain"
)

const (
	kitchenerCityCentreLat = 43.4516
	kitchenerCityCentreLon = -80.4925
)

type KitchenerRoadClosuresSource struct {
	listURL   string
	publicURL string
	lat       float64
	lon       float64
}

func NewKitchenerRoadClosuresSource() *KitchenerRoadClosuresSource {
	return &KitchenerRoadClosuresSource{
		listURL:   kitchenerRoadClosuresListURL,
		publicURL: kitchenerRoadClosuresURL,
		lat:       kitchenerCityCentreLat,
		lon:       kitchenerCityCentreLon,
	}
}

func (s *KitchenerRoadClosuresSource) ID() string { return "kitchener.roadclosures" }

func (s *KitchenerRoadClosuresSource) Fetch(ctx context.Context) ([]domain.TrafficIncident, error) {
	incidents, err := fetchKitchenerRoadClosures(ctx, s.listURL)
	if err != nil {
		return nil, err
	}
	for i := range incidents {
		incidents[i].Lat = s.lat
		incidents[i].Lon = s.lon
		incidents[i].SourceURL = s.publicURL
		incidents[i].ApproximateLocation = true
		if incidents[i].LocationLabel == "" {
			incidents[i].LocationLabel = "Kitchener city centre"
		}
	}
	return incidents, nil
}

func fetchKitchenerRoadClosures(ctx context.Context, url string) ([]domain.TrafficIncident, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request %s: %w", url, err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		io.Copy(io.Discard, io.LimitReader(resp.Body, 512)) //nolint:errcheck
		return nil, fmt.Errorf("get %s: status %d", url, resp.StatusCode)
	}
	return parseKitchenerRoadClosures(resp.Body)
}

func parseKitchenerRoadClosures(r io.Reader) ([]domain.TrafficIncident, error) {
	doc, err := html.Parse(r)
	if err != nil {
		return nil, fmt.Errorf("parse html: %w", err)
	}

	var incidents []domain.TrafficIncident
	for _, table := range htmlElements(doc, "table") {
		section := roadClosureSectionTitle(table)
		for _, row := range htmlElements(table, "tr") {
			cells := directChildElements(row, "td")
			if len(cells) != 3 || hasAttr(cells[0], "colspan") {
				continue
			}

			street := normalizeSpace(textContent(cells[0]))
			fromTo := normalizeSpace(textContent(cells[1]))
			fields := roadClosureDetailFields(cells[2])
			reason := fields["reason"]
			details := fields["details"]

			if reason == "" && details == "" && fields["date"] == "" {
				continue
			}
			if street == "" || !includeKitchenerClosure(section, reason, details) {
				continue
			}

			startsAt, endsAt := parseKitchenerDateRange(fields["date"])
			severity := "moderate"
			if strings.Contains(strings.ToLower(section), "emergency") {
				severity = "major"
			}

			description := reason
			if details != "" {
				if description != "" {
					description += ": "
				}
				description += details
			}
			if fromTo != "" {
				description = appendSentence(description, "From "+fromTo)
			}

			incidents = append(incidents, domain.TrafficIncident{
				ID:                  "kitchener-" + stableClosureID(section, street, fromTo, fields["date"], reason),
				Type:                "closure",
				Severity:            severity,
				Description:         description,
				RoadName:            street,
				StartsAt:            startsAt,
				EndsAt:              endsAt,
				SourceURL:           kitchenerRoadClosuresURL,
				Lat:                 kitchenerCityCentreLat,
				Lon:                 kitchenerCityCentreLon,
				ApproximateLocation: true,
				LocationLabel:       "Kitchener city centre",
			})
		}
	}
	return incidents, nil
}

func includeKitchenerClosure(section, reason, details string) bool {
	text := strings.ToLower(section + " " + reason + " " + details)
	return strings.Contains(text, "emergency") || strings.Contains(text, "special event")
}

func roadClosureSectionTitle(table *html.Node) string {
	for _, caption := range directChildElements(table, "caption") {
		if title := normalizeSpace(textContent(caption)); title != "" {
			return title
		}
	}
	for n := table.Parent; n != nil; n = n.Parent {
		if n.Type == html.ElementNode && n.Data == "section" {
			for _, a := range htmlElements(n, "a") {
				if hasClass(a, "nav-link") {
					return normalizeSpace(textContent(a))
				}
			}
		}
	}
	return ""
}

func roadClosureDetailFields(cell *html.Node) map[string]string {
	fields := make(map[string]string)
	var current string
	for _, line := range strings.Split(textWithBreaks(cell), "\n") {
		line = normalizeSpace(line)
		if line == "" {
			continue
		}
		key, value, ok := splitClosureDetailLabel(line)
		if ok {
			current = key
			fields[current] = appendField(fields[current], value)
			continue
		}
		if current != "" {
			fields[current] = appendField(fields[current], line)
		}
	}
	return fields
}

var closureDetailLabelRe = regexp.MustCompile(`(?i)^(reason|details|date|detour|contact|alternate contact):\s*(.*)$`)

func splitClosureDetailLabel(line string) (key string, value string, ok bool) {
	m := closureDetailLabelRe.FindStringSubmatch(line)
	if len(m) == 0 {
		return "", "", false
	}
	return strings.ToLower(m[1]), strings.TrimSpace(m[2]), true
}

func parseKitchenerDateRange(raw string) (*time.Time, *time.Time) {
	if raw == "" {
		return nil, nil
	}
	parts := strings.SplitN(raw, " to ", 2)
	start := parseKitchenerDate(parts[0])
	var end *time.Time
	if len(parts) == 2 {
		end = parseKitchenerDate(parts[1])
	}
	return start, end
}

func parseKitchenerDate(raw string) *time.Time {
	t, err := time.Parse("2006-Jan-02", strings.TrimSpace(raw))
	if err != nil {
		return nil
	}
	return &t
}

func stableClosureID(parts ...string) string {
	h := fnv.New64a()
	for _, part := range parts {
		io.WriteString(h, strings.ToLower(strings.TrimSpace(part))) //nolint:errcheck
		io.WriteString(h, "\x00")                                   //nolint:errcheck
	}
	return fmt.Sprintf("%x", h.Sum64())
}

func appendSentence(base, sentence string) string {
	if sentence == "" {
		return base
	}
	if base == "" {
		return sentence
	}
	return base + ". " + sentence
}

func appendField(base, value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return base
	}
	if base == "" {
		return value
	}
	return base + " " + value
}

func htmlElements(n *html.Node, name string) []*html.Node {
	var out []*html.Node
	var walk func(*html.Node)
	walk = func(cur *html.Node) {
		if cur.Type == html.ElementNode && cur.Data == name {
			out = append(out, cur)
		}
		for child := cur.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(n)
	return out
}

func directChildElements(n *html.Node, name string) []*html.Node {
	var out []*html.Node
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == html.ElementNode && child.Data == name {
			out = append(out, child)
		}
	}
	return out
}

func textContent(n *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(cur *html.Node) {
		if cur.Type == html.TextNode {
			b.WriteString(cur.Data)
			b.WriteString(" ")
		}
		for child := cur.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(n)
	return b.String()
}

func textWithBreaks(n *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(cur *html.Node) {
		if cur.Type == html.TextNode {
			b.WriteString(cur.Data)
			return
		}
		if cur.Type == html.ElementNode && cur.Data == "br" {
			b.WriteString("\n")
			return
		}
		for child := cur.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(n)
	return b.String()
}

func normalizeSpace(s string) string {
	return strings.Join(strings.Fields(strings.ReplaceAll(s, "\u00a0", " ")), " ")
}

func hasAttr(n *html.Node, name string) bool {
	for _, attr := range n.Attr {
		if strings.EqualFold(attr.Key, name) {
			return true
		}
	}
	return false
}

func hasClass(n *html.Node, class string) bool {
	for _, attr := range n.Attr {
		if attr.Key != "class" {
			continue
		}
		for _, c := range strings.Fields(attr.Val) {
			if c == class {
				return true
			}
		}
	}
	return false
}
