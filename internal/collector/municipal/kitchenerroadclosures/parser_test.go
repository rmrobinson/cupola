package kitchenerroadclosures

import (
	"strings"
	"testing"

	"github.com/rmrobinson/cupola/internal/domain"
)

func TestParseKitchenerRoadClosuresFiltersEmergencyAndSpecialEvents(t *testing.T) {
	html := `
		<table>
			<caption>Emergency Road Closures</caption>
			<thead><tr><td>Street name</td><td>From / To</td><td>Reason / Details</td></tr></thead>
			<tbody>
				<tr>
					<td>FREDERICK ST</td>
					<td>EDNA ST to <span>ANN ST</span></td>
					<td><strong>Reason: </strong>Watermain Break<br>&nbsp;</td>
				</tr>
			</tbody>
		</table>
		<section>
			<p class="tab"><a class="nav-link">Existing road closures</a></p>
			<table>
				<tbody>
					<tr>
						<td>KING ST W</td>
						<td>QUEEN ST to ONTARIO ST</td>
						<td><strong>Reason: </strong>Special Event<br><strong>Date:&nbsp;</strong>2026-May-10 to 2026-May-11<br><strong>Details:</strong>&nbsp;Downtown event closure</td>
					</tr>
					<tr>
						<td>LANCASTER ST W</td>
						<td>ELIZABETH ST to UNION ST</td>
						<td><strong>Reason: </strong>Utility Work<br><strong>Date:&nbsp;</strong>2026-May-10 to 2026-May-11</td>
					</tr>
				</tbody>
			</table>
		</section>
	`

	alerts, err := (&Parser{}).parse(strings.NewReader(html))
	if err != nil {
		t.Fatalf("parse() error = %v", err)
	}
	if len(alerts) != 2 {
		t.Fatalf("expected 2 alerts, got %d: %+v", len(alerts), alerts)
	}

	if alerts[0].Title != "FREDERICK ST road closure" || alerts[0].Severity != domain.SeverityWarning {
		t.Fatalf("unexpected emergency alert: %+v", alerts[0])
	}
	if alerts[0].Area == nil || *alerts[0].Area != "FREDERICK ST: EDNA ST to ANN ST" {
		t.Fatalf("unexpected emergency area: %+v", alerts[0].Area)
	}
	if alerts[1].Title != "KING ST W road closure" || alerts[1].Severity != domain.SeverityInfo {
		t.Fatalf("unexpected special event alert: %+v", alerts[1])
	}
	if alerts[1].StartsAt == nil || alerts[1].EndsAt == nil {
		t.Fatalf("expected parsed date range: %+v", alerts[1])
	}
	if alerts[1].AlertType != "road-closure" {
		t.Fatalf("AlertType = %q, want road-closure", alerts[1].AlertType)
	}
	if !alerts[0].PublishedAt.IsZero() {
		t.Fatalf("emergency alert without date should not synthesize PublishedAt: %+v", alerts[0])
	}
	if alerts[1].StartsAt == nil || !alerts[1].PublishedAt.Equal(alerts[1].StartsAt.UTC()) {
		t.Fatalf("special event PublishedAt should use StartsAt, got alert: %+v", alerts[1])
	}
	for _, alert := range alerts {
		if len(alert.Polygon) != 0 {
			t.Fatalf("road closure parser should not infer map geometry: %+v", alert.Polygon)
		}
	}
}

func TestParseKitchenerRoadClosuresDedupesRepeatedRowsByID(t *testing.T) {
	html := `
		<table>
			<caption>Emergency Road Closures</caption>
			<tbody>
				<tr>
					<td>FREDERICK ST</td>
					<td>EDNA ST to ANN ST</td>
					<td><strong>Reason: </strong>Watermain Break<br>&nbsp;</td>
				</tr>
				<tr>
					<td>FREDERICK ST</td>
					<td>EDNA ST to ANN ST</td>
					<td><strong>Reason: </strong>Watermain Break<br>&nbsp;</td>
				</tr>
			</tbody>
		</table>
	`

	alerts, err := (&Parser{}).parse(strings.NewReader(html))
	if err != nil {
		t.Fatalf("parse() error = %v", err)
	}
	if len(alerts) != 1 {
		t.Fatalf("expected duplicate row to be dropped, got %d alerts: %+v", len(alerts), alerts)
	}
}

func TestDedupeMunicipalAlertsByIDKeepsFirstDuplicateID(t *testing.T) {
	alerts := []domain.MunicipalAlert{
		{ID: "a", Title: "FIRST"},
		{ID: "b", Title: "SECOND"},
		{ID: "a", Title: "DUPLICATE"},
		{ID: "c", Title: "THIRD"},
	}

	got, duplicateCount := dedupeMunicipalAlertsByID(alerts)
	if duplicateCount != 1 {
		t.Fatalf("expected 1 duplicate, got %d", duplicateCount)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 alerts, got %d: %+v", len(got), got)
	}
	if got[0].Title != "FIRST" || got[1].Title != "SECOND" || got[2].Title != "THIRD" {
		t.Fatalf("dedupe did not preserve order and first occurrence: %+v", got)
	}
}

func TestDedupeMunicipalAlertsByIDPreservesEmptyIDs(t *testing.T) {
	alerts := []domain.MunicipalAlert{
		{Title: "NO ID 1"},
		{ID: "a", Title: "WITH ID"},
		{Title: "NO ID 2"},
		{ID: "a", Title: "DUPLICATE"},
	}

	got, duplicateCount := dedupeMunicipalAlertsByID(alerts)
	if duplicateCount != 1 {
		t.Fatalf("expected 1 duplicate, got %d", duplicateCount)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 alerts, got %d: %+v", len(got), got)
	}
	if got[0].Title != "NO ID 1" || got[1].Title != "WITH ID" || got[2].Title != "NO ID 2" {
		t.Fatalf("dedupe should preserve empty-ID alerts: %+v", got)
	}
}

func TestDedupeMunicipalAlertsByIDReturnsOriginalWhenNoDuplicates(t *testing.T) {
	alerts := []domain.MunicipalAlert{
		{ID: "a", Title: "FIRST"},
		{ID: "b", Title: "SECOND"},
	}

	got, duplicateCount := dedupeMunicipalAlertsByID(alerts)
	if duplicateCount != 0 {
		t.Fatalf("expected 0 duplicates, got %d", duplicateCount)
	}
	if len(got) != 2 || got[0].ID != "a" || got[1].ID != "b" {
		t.Fatalf("unexpected alerts: %+v", got)
	}
}
