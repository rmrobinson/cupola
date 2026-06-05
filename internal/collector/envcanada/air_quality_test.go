package envcanada

import (
	"strings"
	"testing"

	"github.com/rmrobinson/cupola/internal/domain"
)

func TestParseAirQualitySummaryHTML(t *testing.T) {
	summary, err := parseAirQualitySummaryHTML([]byte(airQualitySummaryFixture), "ON", airQualityBaseURL)
	if err != nil {
		t.Fatalf("parseAirQualitySummaryHTML: %v", err)
	}
	if len(summary.Sites) != 2 {
		t.Fatalf("len(Sites) = %d, want 2", len(summary.Sites))
	}
	row, ok := findAirQualitySite(summary.Sites, "kitchener")
	if !ok {
		t.Fatalf("Kitchener row not found")
	}
	if row.Location != "Kitchener" {
		t.Fatalf("Location = %q", row.Location)
	}
	if row.SourceURL != "https://weather.gc.ca/airquality/pages/onaq-030_e.html" {
		t.Fatalf("SourceURL = %q", row.SourceURL)
	}
	assertAQHIValue(t, "observed", row.Observed, 4, "Moderate Risk")
	if len(row.Forecasts) != 4 {
		t.Fatalf("len(Forecasts) = %d, want 4", len(row.Forecasts))
	}
	if row.Forecasts[1].Label != "Friday" {
		t.Fatalf("forecast label = %q", row.Forecasts[1].Label)
	}
	assertAQHIValue(t, "friday", row.Forecasts[1].Max, 5, "Moderate Risk")
	if summary.IssuedAt.IsZero() {
		t.Fatalf("IssuedAt was not parsed")
	}
}

func TestFindAirQualitySiteMissingListsAvailableLocations(t *testing.T) {
	summary, err := parseAirQualitySummaryHTML([]byte(airQualitySummaryFixture), "ON", airQualityBaseURL)
	if err != nil {
		t.Fatalf("parseAirQualitySummaryHTML: %v", err)
	}
	_, ok := findAirQualitySite(summary.Sites, "Waterloo")
	if ok {
		t.Fatalf("findAirQualitySite unexpectedly matched Waterloo")
	}
	available := availableAirQualitySites(summary.Sites)
	if !strings.Contains(available, "Kitchener") || !strings.Contains(available, "Hamilton Downtown") {
		t.Fatalf("availableAirQualitySites = %q", available)
	}
}

func TestParseAirQualitySummaryHTMLMissingValues(t *testing.T) {
	body := strings.Replace(airQualitySummaryFixture, "4<br>Moderate Risk", "-<br>", 1)
	summary, err := parseAirQualitySummaryHTML([]byte(body), "ON", airQualityBaseURL)
	if err != nil {
		t.Fatalf("parseAirQualitySummaryHTML: %v", err)
	}
	row, ok := findAirQualitySite(summary.Sites, "Kitchener")
	if !ok {
		t.Fatalf("Kitchener row not found")
	}
	if row.Observed != nil {
		t.Fatalf("Observed = %+v, want nil", row.Observed)
	}
}

func TestDiscoverAirQualitySite(t *testing.T) {
	summary, err := parseAirQualitySummaryHTML([]byte(airQualitySummaryFixture), "ON", airQualityBaseURL)
	if err != nil {
		t.Fatalf("parseAirQualitySummaryHTML: %v", err)
	}
	stations := []ECStation{
		{Name: "Hamilton Downtown", Lat: 43.2557, Lon: -79.8711, Province: "ON"},
		{Name: "Kitchener Waterloo", Lat: 43.45, Lon: -80.49, Province: "ON"},
	}
	row, station, err := discoverAirQualitySite(summary.Sites, stations, 43.46, -80.50)
	if err != nil {
		t.Fatalf("discoverAirQualitySite: %v", err)
	}
	if row.Location != "Kitchener" {
		t.Fatalf("selected row = %q", row.Location)
	}
	if station.Name != "Kitchener Waterloo" {
		t.Fatalf("matched station = %q", station.Name)
	}
}

func TestDiscoverAirQualitySiteLowConfidence(t *testing.T) {
	rows := []airQualityRow{{Location: "Kitchener"}}
	stations := []ECStation{{Name: "Ottawa", Lat: 45.42, Lon: -75.69, Province: "ON"}}
	_, _, err := discoverAirQualitySite(rows, stations, 43.46, -80.50)
	if err == nil {
		t.Fatalf("discoverAirQualitySite returned nil error")
	}
	if !strings.Contains(err.Error(), "air_quality_location") {
		t.Fatalf("error = %q", err.Error())
	}
}

func assertAQHIValue(t *testing.T, name string, got *domain.AQHIValue, wantValue int, wantRisk string) {
	t.Helper()
	if got == nil {
		t.Fatalf("%s = nil", name)
	}
	if got.Value == nil || *got.Value != wantValue {
		t.Fatalf("%s value = %+v, want %d", name, got.Value, wantValue)
	}
	if got.Risk != wantRisk {
		t.Fatalf("%s risk = %q, want %q", name, got.Risk, wantRisk)
	}
}

const airQualitySummaryFixture = `
<html>
<body>
<table class="table table-striped table-hover">
<caption><p>This table shows a summary of current Air Quality Health Index values and forecast maximums<sup><abbr title="5:00 PM EDT Thursday 4 June 2026"><a href="#cite_1">1</a></abbr></sup></p></caption>
<thead>
<tr>
<th rowspan="2" id="location">Location and sub-locations</th>
<th rowspan="2" id="current-observation">Observed Conditions</th>
<th id="forecast-maximums" colspan="4">Forecast Maximums</th>
</tr>
<tr>
<th>Thursday night</th>
<th>Friday</th>
<th>Friday night</th>
<th>Saturday</th>
</tr>
</thead>
<tbody>
<tr>
<td headers="location"><a href="/airquality/pages/onaq-030_e.html">Kitchener</a></td>
<td headers="current-observation">4<br>Moderate Risk</td>
<td headers="forecast-maximums">4<br>Moderate Risk</td>
<td headers="forecast-maximums">5<br>Moderate Risk</td>
<td headers="forecast-maximums">5<br>Moderate Risk</td>
<td headers="forecast-maximums">3<br>Low Risk</td>
</tr>
<tr>
<td headers="location"><ul><li><a href="/airquality/pages/onaq-032_e.html">Hamilton Downtown</a></li></ul></td>
<td headers="current-observation">4<br>Moderate Risk</td>
<td headers="forecast-maximums">4<br>Moderate Risk</td>
<td headers="forecast-maximums">5<br>Moderate Risk</td>
<td headers="forecast-maximums">5<br>Moderate Risk</td>
<td headers="forecast-maximums">3<br>Low Risk</td>
</tr>
</tbody>
</table>
<p id="cite_1">1. Forecast issued at: 5:00 PM EDT Thursday 4 June 2026</p>
</body>
</html>`
