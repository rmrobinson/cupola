package traffic511

import (
	"strings"
	"testing"
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

	incidents, err := parseKitchenerRoadClosures(strings.NewReader(html))
	if err != nil {
		t.Fatalf("parseKitchenerRoadClosures() error = %v", err)
	}
	if len(incidents) != 2 {
		t.Fatalf("expected 2 incidents, got %d: %+v", len(incidents), incidents)
	}

	if incidents[0].RoadName != "FREDERICK ST" || incidents[0].Severity != "major" {
		t.Fatalf("unexpected emergency incident: %+v", incidents[0])
	}
	if incidents[1].RoadName != "KING ST W" || incidents[1].Severity != "moderate" {
		t.Fatalf("unexpected special event incident: %+v", incidents[1])
	}
	if incidents[1].StartsAt == nil || incidents[1].EndsAt == nil {
		t.Fatalf("expected parsed date range: %+v", incidents[1])
	}
	for _, inc := range incidents {
		if inc.Lat != kitchenerCityCentreLat || inc.Lon != kitchenerCityCentreLon {
			t.Fatalf("expected Kitchener city-centre coordinates, got %+v", inc)
		}
		if !inc.ApproximateLocation || inc.LocationLabel != "Kitchener city centre" {
			t.Fatalf("expected approximate location metadata, got %+v", inc)
		}
	}
}
