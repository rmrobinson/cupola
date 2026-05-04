package kitchenersnow

import (
	"testing"
)

func TestParseLive(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live network test")
	}
	p := &Parser{}
	events, err := p.Parse("https://www.kitchener.ca/news/snow-events/")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	t.Logf("got %d events", len(events))
	for i, e := range events {
		t.Logf("  [%d] %s | %s | published=%s", i, e.EventType, e.Title, e.PublishedAt.Format("2006-01-02"))
		if i >= 4 {
			break
		}
	}
}
