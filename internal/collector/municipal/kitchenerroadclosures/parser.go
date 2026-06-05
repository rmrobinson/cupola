// Package kitchenerroadclosures parses the City of Kitchener road-closures
// page into municipal.alerts.
//
// Register via import side-effect:
//
//	_ "github.com/rmrobinson/cupola/internal/collector/municipal/kitchenerroadclosures"
package kitchenerroadclosures

import (
	"fmt"
	"hash/fnv"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"golang.org/x/net/html"

	"github.com/rmrobinson/cupola/internal/collector/municipal"
	"github.com/rmrobinson/cupola/internal/domain"
)

const publicURL = "https://www.kitchener.ca/roadclosures"

func init() {
	municipal.RegisterAlertsParser("kitchener.roadclosures", func() municipal.AlertsParser {
		return &Parser{}
	})
}

// Parser implements municipal.AlertsParser for Kitchener road closures.
type Parser struct {
	Now func() time.Time
}

func (p *Parser) Parse(rawURL string) ([]domain.MunicipalAlert, error) {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("kitchener.roadclosures: create request %s: %w", rawURL, err)
	}
	resp, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("kitchener.roadclosures: get %s: %w", rawURL, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		io.Copy(io.Discard, io.LimitReader(resp.Body, 512)) //nolint:errcheck
		return nil, fmt.Errorf("kitchener.roadclosures: get %s: status %d", rawURL, resp.StatusCode)
	}
	return p.parse(resp.Body)
}

func (p *Parser) parse(r io.Reader) ([]domain.MunicipalAlert, error) {
	doc, err := html.Parse(r)
	if err != nil {
		return nil, fmt.Errorf("kitchener.roadclosures: parse HTML: %w", err)
	}

	url := publicURL
	var alerts []domain.MunicipalAlert
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
			if street == "" || !includeClosure(section, reason, details) {
				continue
			}

			startsAt, endsAt := parseDateRange(fields["date"])
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

			var area *string
			if fromTo != "" {
				a := street + ": " + fromTo
				area = &a
			} else {
				a := street
				area = &a
			}

			alerts = append(alerts, domain.MunicipalAlert{
				ID:          "kitchener.roadclosures:" + stableClosureID(section, street, fromTo, fields["date"], reason),
				Title:       street + " road closure",
				Description: description,
				AlertType:   "road-closure",
				Severity:    closureSeverity(section, reason, details),
				Area:        area,
				StartsAt:    startsAt,
				EndsAt:      endsAt,
				URL:         &url,
				PublishedAt: publishedAtFromStartsAt(startsAt),
			})
		}
	}

	alerts, _ = dedupeMunicipalAlertsByID(alerts)
	return alerts, nil
}

func includeClosure(section, reason, details string) bool {
	text := strings.ToLower(section + " " + reason + " " + details)
	return strings.Contains(text, "emergency") || strings.Contains(text, "special event")
}

func publishedAtFromStartsAt(startsAt *time.Time) time.Time {
	if startsAt == nil {
		return time.Time{}
	}
	return startsAt.UTC()
}

func closureSeverity(section, reason, details string) domain.AlertSeverity {
	text := strings.ToLower(section + " " + reason + " " + details)
	if strings.Contains(text, "emergency") {
		return domain.SeverityWarning
	}
	return domain.SeverityInfo
}

func dedupeMunicipalAlertsByID(alerts []domain.MunicipalAlert) ([]domain.MunicipalAlert, int) {
	if len(alerts) < 2 {
		return alerts, 0
	}
	seen := make(map[string]struct{}, len(alerts))
	deduped := make([]domain.MunicipalAlert, 0, len(alerts))
	var duplicates int
	for _, alert := range alerts {
		if alert.ID == "" {
			deduped = append(deduped, alert)
			continue
		}
		if _, ok := seen[alert.ID]; ok {
			duplicates++
			continue
		}
		seen[alert.ID] = struct{}{}
		deduped = append(deduped, alert)
	}
	if duplicates == 0 {
		return alerts, 0
	}
	return deduped, duplicates
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

func parseDateRange(raw string) (*time.Time, *time.Time) {
	if raw == "" {
		return nil, nil
	}
	parts := strings.SplitN(raw, " to ", 2)
	start := parseDate(parts[0])
	var end *time.Time
	if len(parts) == 2 {
		end = parseDate(parts[1])
	}
	return start, end
}

func parseDate(raw string) *time.Time {
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
