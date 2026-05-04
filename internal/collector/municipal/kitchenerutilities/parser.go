// Package kitchenerutilities parses the Kitchener Utilities unplanned service
// disruption feed into municipal.alerts.
//
// The configured URL should point to the watermain/disruption status endpoint:
//
//	https://app2.kitchener.ca/utilities/Default.aspx?wmode=transparent
//
// Register via import side-effect:
//
//	_ "github.com/rmrobinson/cupola/internal/collector/municipal/kitchenerutilities"
package kitchenerutilities

import (
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"golang.org/x/net/html"

	"github.com/rmrobinson/cupola/internal/collector/municipal"
	"github.com/rmrobinson/cupola/internal/domain"
)

func init() {
	municipal.RegisterAlertsParser("kitchener.utilities", func() municipal.AlertsParser {
		return &Parser{}
	})
}

// Parser implements municipal.AlertsParser for the Kitchener Utilities disruptions page.
type Parser struct{}

func (p *Parser) Parse(rawURL string) ([]domain.MunicipalAlert, error) {
	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Get(rawURL)
	if err != nil {
		return nil, fmt.Errorf("kitchener.utilities: get %s: %w", rawURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("kitchener.utilities: get %s: status %d", rawURL, resp.StatusCode)
	}

	doc, err := html.Parse(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("kitchener.utilities: parse HTML: %w", err)
	}

	// If the status label says nothing to report, return empty.
	lblList := findByID(doc, "lblList")
	if lblList != nil {
		text := strings.ToLower(textContent(lblList))
		if strings.Contains(text, "nothing to report") {
			return nil, nil
		}
	}

	var alerts []domain.MunicipalAlert
	walkNodes(doc, func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "div" && hasClass(n, "bs-example") {
			if a := extractAlert(n); a != nil {
				alerts = append(alerts, *a)
			}
		}
	})
	return alerts, nil
}

func extractAlert(div *html.Node) *domain.MunicipalAlert {
	var title string
	fields := map[string]string{}
	var lastDT string

	walkNodes(div, func(n *html.Node) {
		if n.Type != html.ElementNode {
			return
		}
		switch n.Data {
		case "h3":
			title = strings.TrimSpace(textContent(n))
		case "dt":
			lastDT = strings.TrimSpace(strings.TrimSuffix(textContent(n), ":"))
		case "dd":
			if lastDT != "" {
				fields[strings.ToLower(lastDT)] = strings.TrimSpace(textContent(n))
				lastDT = ""
			}
		}
	})

	if title == "" {
		return nil
	}

	area := fields["location"]
	if between := fields["between what streets or addresses"]; between != "" {
		if area != "" {
			area += " — " + between
		} else {
			area = between
		}
	}

	pub := parseKUDate(fields["posted on"])
	desc := "Status: " + fields["status"]

	var areaPtr *string
	if area != "" {
		areaPtr = &area
	}

	return &domain.MunicipalAlert{
		ID:          "kitchener.utilities:" + title,
		Title:       title,
		Description: desc,
		AlertType:   "water-outage",
		Severity:    domain.SeverityWarning,
		Area:        areaPtr,
		PublishedAt: pub,
	}
}

// parseKUDate parses dates in the format:
// "WEDNESDAY, 06 NOV, 2019 13:24:53 01:24 PM"
func parseKUDate(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}
	// Drop the weekday prefix: split on first ", " and take the rest
	if idx := strings.Index(s, ", "); idx >= 0 {
		s = s[idx+2:]
	}
	// s is now like "06 NOV, 2019 13:24:53 01:24 PM"
	// Title-case month abbreviations for time.Parse
	monthMap := map[string]string{
		"JAN": "Jan", "FEB": "Feb", "MAR": "Mar", "APR": "Apr",
		"MAY": "May", "JUN": "Jun", "JUL": "Jul", "AUG": "Aug",
		"SEP": "Sep", "OCT": "Oct", "NOV": "Nov", "DEC": "Dec",
	}
	for upper, title := range monthMap {
		s = strings.ReplaceAll(s, " "+upper+",", " "+title+",")
		s = strings.ReplaceAll(s, " "+upper+" ", " "+title+" ")
	}
	// Try "02 Jan, 2006 15:04:05" and variations
	for _, f := range []string{
		"02 Jan, 2006 15:04:05",
		"02 Jan, 2006",
		"Jan 2, 2006 15:04:05",
	} {
		// Take only as many fields as the format needs
		candidate := strings.Join(strings.Fields(s)[:min(len(strings.Fields(f)), len(strings.Fields(s)))], " ")
		if t, err := time.Parse(f, candidate); err == nil {
			return t.UTC()
		}
	}
	log.Printf("[kitchener.utilities] unparseable date %q", s)
	return time.Time{}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ── HTML helpers ──────────────────────────────────────────────────────────────

func walkNodes(n *html.Node, fn func(*html.Node)) {
	fn(n)
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		walkNodes(c, fn)
	}
}

func findByID(n *html.Node, id string) *html.Node {
	if n.Type == html.ElementNode {
		for _, a := range n.Attr {
			if a.Key == "id" && a.Val == id {
				return n
			}
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if found := findByID(c, id); found != nil {
			return found
		}
	}
	return nil
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

func textContent(n *html.Node) string {
	var sb strings.Builder
	walkNodes(n, func(c *html.Node) {
		if c.Type == html.TextNode {
			sb.WriteString(c.Data)
		}
	})
	return sb.String()
}
