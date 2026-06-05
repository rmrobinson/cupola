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

	events, err := (&Parser{}).parse(strings.NewReader(html))
	if err != nil {
		t.Fatalf("parse() error = %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d: %+v", len(events), events)
	}

	if events[0].Title != "FREDERICK ST road closure" || events[0].EventType != "road-closure" {
		t.Fatalf("unexpected emergency event: %+v", events[0])
	}
	if !strings.Contains(events[0].Description, "From EDNA ST to ANN ST") {
		t.Fatalf("expected emergency from/to in description: %+v", events[0])
	}
	if events[1].Title != "KING ST W road closure" || events[1].EventType != "road-closure" {
		t.Fatalf("unexpected special event: %+v", events[1])
	}
	if events[1].StartsAt == nil || events[1].EndsAt == nil {
		t.Fatalf("expected parsed date range: %+v", events[1])
	}
	if !events[0].PublishedAt.IsZero() {
		t.Fatalf("emergency event without date should not synthesize PublishedAt: %+v", events[0])
	}
	if events[1].StartsAt == nil || !events[1].PublishedAt.Equal(events[1].StartsAt.UTC()) {
		t.Fatalf("special event PublishedAt should use StartsAt, got event: %+v", events[1])
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

	events, err := (&Parser{}).parse(strings.NewReader(html))
	if err != nil {
		t.Fatalf("parse() error = %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected duplicate row to be dropped, got %d events: %+v", len(events), events)
	}
}

func TestDedupeMunicipalEventsByIDKeepsFirstDuplicateID(t *testing.T) {
	events := []testMunicipalEvent{
		{ID: "a", Title: "FIRST"},
		{ID: "b", Title: "SECOND"},
		{ID: "a", Title: "DUPLICATE"},
		{ID: "c", Title: "THIRD"},
	}

	got, duplicateCount := dedupeMunicipalEventsByID(toDomainEvents(events))
	if duplicateCount != 1 {
		t.Fatalf("expected 1 duplicate, got %d", duplicateCount)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 events, got %d: %+v", len(got), got)
	}
	if got[0].Title != "FIRST" || got[1].Title != "SECOND" || got[2].Title != "THIRD" {
		t.Fatalf("dedupe did not preserve order and first occurrence: %+v", got)
	}
}

func TestDedupeMunicipalEventsByIDPreservesEmptyIDs(t *testing.T) {
	events := []testMunicipalEvent{
		{Title: "NO ID 1"},
		{ID: "a", Title: "WITH ID"},
		{Title: "NO ID 2"},
		{ID: "a", Title: "DUPLICATE"},
	}

	got, duplicateCount := dedupeMunicipalEventsByID(toDomainEvents(events))
	if duplicateCount != 1 {
		t.Fatalf("expected 1 duplicate, got %d", duplicateCount)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 events, got %d: %+v", len(got), got)
	}
	if got[0].Title != "NO ID 1" || got[1].Title != "WITH ID" || got[2].Title != "NO ID 2" {
		t.Fatalf("dedupe should preserve empty-ID events: %+v", got)
	}
}

func TestDedupeMunicipalEventsByIDReturnsOriginalWhenNoDuplicates(t *testing.T) {
	events := []testMunicipalEvent{
		{ID: "a", Title: "FIRST"},
		{ID: "b", Title: "SECOND"},
	}

	got, duplicateCount := dedupeMunicipalEventsByID(toDomainEvents(events))
	if duplicateCount != 0 {
		t.Fatalf("expected 0 duplicates, got %d", duplicateCount)
	}
	if len(got) != 2 || got[0].ID != "a" || got[1].ID != "b" {
		t.Fatalf("unexpected alerts: %+v", got)
	}
}

type testMunicipalEvent struct {
	ID    string
	Title string
}

func toDomainEvents(events []testMunicipalEvent) []domain.MunicipalEvent {
	out := make([]domain.MunicipalEvent, len(events))
	for i, ev := range events {
		out[i].ID = ev.ID
		out[i].Title = ev.Title
	}
	return out
}
