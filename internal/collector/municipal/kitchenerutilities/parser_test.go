package kitchenerutilities

import (
	"testing"
)

func TestParseLive(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live network test")
	}
	p := &Parser{}
	alerts, err := p.Parse("https://app2.kitchener.ca/utilities/Default.aspx?wmode=transparent")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	t.Logf("got %d alerts", len(alerts))
	for i, a := range alerts {
		t.Logf("  [%d] %s | %s", i, a.AlertType, a.Title)
	}
}
