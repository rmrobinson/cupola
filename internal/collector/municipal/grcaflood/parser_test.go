package grcaflood

import (
	"testing"
)

func TestParseLive(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live network test")
	}
	p := &Parser{}
	alerts, err := p.Parse("https://www.grandriver.ca/news/categories/flood-messages/")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	t.Logf("got %d alerts", len(alerts))
	for i, a := range alerts {
		t.Logf("  [%d] %s | %s | severity=%s", i, a.AlertType, a.Title, a.Severity)
	}
}
