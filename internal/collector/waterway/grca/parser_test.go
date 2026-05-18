package grca

import "testing"

var sourceURLFallbackGaugeIDs = map[string]bool{
	"grca_schneider_ottawa_st":     true,
	"grca_fairchild_brantford_wsc": true,
}

func TestSourceURLMappingCoversKnownGauges(t *testing.T) {
	for _, meta := range flowStations {
		if sourceURLsByGaugeID[meta.id] == "" && !sourceURLFallbackGaugeIDs[meta.id] {
			t.Fatalf("flow station %q has no source URL mapping or explicit fallback", meta.id)
		}
	}
	for _, meta := range reservoirs {
		if sourceURLsByGaugeID[meta.id] == "" && !sourceURLFallbackGaugeIDs[meta.id] {
			t.Fatalf("reservoir %q has no source URL mapping or explicit fallback", meta.id)
		}
	}
}

func TestSourceURLForGaugeFallsBackToFlowSummary(t *testing.T) {
	if got := sourceURLForGauge("grca_schneider_ottawa_st"); got != flowSummaryURL {
		t.Fatalf("sourceURLForGauge() = %q, want %q", got, flowSummaryURL)
	}
}
