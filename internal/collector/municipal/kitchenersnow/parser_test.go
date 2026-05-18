package kitchenersnow

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/rmrobinson/cupola/internal/domain"
)

func TestParseSuppressesInactiveSnowEvent(t *testing.T) {
	loc := mustToronto(t)
	srv := snowArchiveServer(t, []snowArchiveItem{
		{
			Href:  "/news/posts/cancel/",
			Title: "City of Kitchener cancels snow event and parking ban as of March 15, 2026 at 3 p.m.",
			Desc:  "The City of Kitchener has cancelled its snow event effective 3 p.m. on Sunday, March 15, 2026. The street parking ban will be lifted once the snow event is cancelled.",
			Date:  "Mar 15, 2026",
		},
		{
			Href:  "/news/posts/extend/",
			Title: "City extends snow event until 2 a.m. Monday, March 16, 2026",
			Desc:  "A snow event was declared for 2 a.m. on Saturday, March 14 based on credible weather forecast information. This snow event has now been extended until 2 a.m. on Monday, March 16.",
			Date:  "Mar 14, 2026",
		},
		{
			Href:  "/news/posts/declare/",
			Title: "City of Kitchener declares snow event with parking ban starting at 2 a.m. March 14, 2026",
			Desc:  "The City of Kitchener has declared a snow event. Residents have until 2 a.m. Saturday, March 14, 2026 to remove their parked cars from city streets.",
			Date:  "Mar 13, 2026",
		},
	})
	defer srv.Close()

	p := &Parser{
		Now:      func() time.Time { return time.Date(2026, time.May, 18, 12, 0, 0, 0, loc) },
		Location: loc,
	}
	alerts, err := p.Parse(srv.URL)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(alerts) != 0 {
		t.Fatalf("len(alerts) = %d, want 0: %#v", len(alerts), alerts)
	}
}

func TestParseSynthesizesActiveSnowAlert(t *testing.T) {
	loc := mustToronto(t)
	srv := snowArchiveServer(t, []snowArchiveItem{
		{
			Href:  "/news/posts/extend/",
			Title: "City extends snow event until 12:00 p.m. Thursday, December 11, 2025",
			Desc:  "The City of Kitchener has extended its snow event until 12:00 p.m. (noon) on Thursday, December 11. Street parking is prohibited during a snow event and parking exemptions are cancelled.",
			Date:  "Dec 10, 2025",
		},
		{
			Href:  "/news/posts/declare/",
			Title: "City of Kitchener declares snow event starting at 2:00 a.m. on December 10, 2025",
			Desc:  "The City of Kitchener has declared a snow event. Residents have until 2:00 a.m. Wednesday, December 10, 2025, to remove their parked cars from city streets.",
			Date:  "Dec 09, 2025",
		},
	})
	defer srv.Close()

	p := &Parser{
		Now:      func() time.Time { return time.Date(2025, time.December, 10, 13, 0, 0, 0, loc) },
		Location: loc,
	}
	alerts, err := p.Parse(srv.URL)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(alerts) != 1 {
		t.Fatalf("len(alerts) = %d, want 1", len(alerts))
	}
	alert := alerts[0]
	if alert.ID != "kitchener.snow:active" {
		t.Fatalf("ID = %q, want kitchener.snow:active", alert.ID)
	}
	if alert.AlertType != "snow-event" {
		t.Fatalf("AlertType = %q, want snow-event", alert.AlertType)
	}
	if alert.Severity != domain.SeverityWarning {
		t.Fatalf("Severity = %q, want warning", alert.Severity)
	}
	if alert.StartsAt == nil || !alert.StartsAt.Equal(time.Date(2025, time.December, 10, 2, 0, 0, 0, loc)) {
		t.Fatalf("StartsAt = %v", alert.StartsAt)
	}
	if alert.EndsAt == nil || !alert.EndsAt.Equal(time.Date(2025, time.December, 11, 12, 0, 0, 0, loc)) {
		t.Fatalf("EndsAt = %v", alert.EndsAt)
	}
	if alert.URL == nil || *alert.URL != srv.URL+"/news/posts/extend/" {
		t.Fatalf("URL = %v, want extension URL", alert.URL)
	}
}

func TestParseLive(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live network test")
	}
	p := &Parser{}
	alerts, err := p.Parse("https://www.kitchener.ca/news/snow-events/")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	t.Logf("got %d alerts", len(alerts))
	for i, a := range alerts {
		t.Logf("  [%d] %s | %s | published=%s", i, a.AlertType, a.Title, a.PublishedAt.Format("2006-01-02"))
		if i >= 4 {
			break
		}
	}
}

type snowArchiveItem struct {
	Href  string
	Title string
	Desc  string
	Date  string
}

func snowArchiveServer(t *testing.T, items []snowArchiveItem) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var b strings.Builder
		b.WriteString("<html><body><ul>")
		for _, item := range items {
			fmt.Fprintf(&b, `<li class="gs-feed-list-item">
				<a href="%s" class="gs-feed-list-title">%s</a>
				<span class="gs-feed-list-description"><p>%s</p></span>
				<span class="gs-feed-list-date">%s</span>
			</li>`, item.Href, item.Title, item.Desc, item.Date)
		}
		b.WriteString("</ul></body></html>")
		_, _ = w.Write([]byte(b.String()))
	}))
}

func mustToronto(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("America/Toronto")
	if err != nil {
		t.Fatal(err)
	}
	return loc
}
