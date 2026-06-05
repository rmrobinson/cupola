package envcanada

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/rmrobinson/cupola/internal/domain"
	"github.com/rmrobinson/cupola/internal/store"
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

func TestParseAirQualityIssuedAtUsesSiteLocation(t *testing.T) {
	loc, err := time.LoadLocation("America/Toronto")
	if err != nil {
		t.Fatalf("LoadLocation: %v", err)
	}
	summary, err := parseAirQualitySummaryHTMLInLocation([]byte(airQualitySummaryFixture), "ON", airQualityBaseURL, loc)
	if err != nil {
		t.Fatalf("parseAirQualitySummaryHTMLInLocation: %v", err)
	}
	want := time.Date(2026, 6, 4, 17, 0, 0, 0, loc)
	if !summary.IssuedAt.Equal(want) {
		t.Fatalf("IssuedAt = %s, want %s", summary.IssuedAt, want)
	}
}

func TestParseAirQualitySummaryHTMLIgnoresNestedTableCells(t *testing.T) {
	body := strings.Replace(airQualitySummaryFixture,
		"4<br>Moderate Risk</td>",
		"4<br>Moderate Risk<table><tr><td>99<br>Very High Risk</td></tr></table></td>",
		1,
	)
	summary, err := parseAirQualitySummaryHTML([]byte(body), "ON", airQualityBaseURL)
	if err != nil {
		t.Fatalf("parseAirQualitySummaryHTML: %v", err)
	}
	row, ok := findAirQualitySite(summary.Sites, "Kitchener")
	if !ok {
		t.Fatalf("Kitchener row not found")
	}
	assertAQHIValue(t, "tonight", row.Forecasts[0].Max, 4, "Moderate Risk")
}

func TestParseAQHIValueVeryHighPlus(t *testing.T) {
	got := parseAQHIValue("10+ Very High Risk")
	assertAQHIValue(t, "10+", got, 10, "Very High Risk")
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

func TestResolveSiteUsesConfiguredStationCodeCoordinates(t *testing.T) {
	resetStationTestState()
	oldAQBase := airQualityBaseURL
	defer func() {
		airQualityBaseURL = oldAQBase
		resetStationTestState()
	}()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/airquality/pages/provincial_summary/on_e.html":
			_, _ = fmt.Fprint(w, airQualitySummaryFixture)
		case "/forecast/canada/index_e.html":
			_, _ = fmt.Fprint(w, `<html><script>[`+
				`{"display_name":"Hamilton Downtown","code":"hamilton","lat":43.2557,"lon":-79.8711,"province":"ON"},`+
				`{"display_name":"Kitchener Waterloo","code":"s0000585","lat":43.45,"lon":-80.49,"province":"ON"}`+
				`]</script></html>`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	airQualityBaseURL = srv.URL
	stationPageBaseURL = srv.URL

	collector := NewAirQualityCollector(43.2557, -79.8711, time.Hour, store.NewStateStore(), AirQualityOptions{
		Station: StationOverride{Code: "s0000585", Province: "ON"},
	})
	site, err := collector.resolveSite()
	if err != nil {
		t.Fatalf("resolveSite: %v", err)
	}
	if site.Location != "Kitchener" {
		t.Fatalf("site.Location = %q, want Kitchener", site.Location)
	}
	if site.Lat != 43.45 || site.Lon != -80.49 {
		t.Fatalf("site coordinates = %.4f, %.4f; want configured station coordinates", site.Lat, site.Lon)
	}
}

func TestDiscoverAirQualitySiteLowConfidence(t *testing.T) {
	rows := []airQualityRow{{Location: "Kitchener"}}
	stations := []ECStation{{Name: "Ottawa", Lat: 45.42, Lon: -75.69, Province: "ON"}}
	_, _, err := discoverAirQualitySite(rows, stations, 43.46, -80.50)
	if err == nil {
		t.Fatalf("discoverAirQualitySite returned nil error")
	}
	if !strings.Contains(err.Error(), "air_quality_envcanada.location") {
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
