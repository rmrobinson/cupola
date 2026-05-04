package enovapower

import (
	"testing"
)

func TestParseLive(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live network test")
	}
	p := &Parser{}
	alerts, err := p.Parse("https://oms.enovapower.com/Outages/")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	t.Logf("got %d alerts", len(alerts))
	for i, a := range alerts {
		t.Logf("  [%d] %s | %s", i, a.AlertType, a.Title)
	}
}
