package envcanada

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rmrobinson/cupola/internal/domain"
	"github.com/rmrobinson/cupola/internal/store"
)

func TestParseHourlyForecastHTML(t *testing.T) {
	html := hourlyFixture(`{
		"location": {
			"hourly": [
				{
					"epochTime": 1716138000,
					"condition": "Chance of showers",
					"precip": "60",
					"temperature": {"metric": "21", "imperial": "70"},
					"feelsLike": {"metric": "24", "imperial": "75"},
					"windDir": "SW",
					"windSpeed": {"metric": "20", "imperial": "12"},
					"windGust": {"metric": "40", "imperial": "25"},
					"humidex": {"metric": "25", "imperial": "77"},
					"windChill": {"metric": "", "imperial": ""},
					"uv": {"index": "3"},
					"iconCode": "09"
				},
				{
					"epochTime": 1716141600,
					"condition": "Clear",
					"precip": "",
					"temperature": {"metric": "19", "imperial": "66"},
					"feelsLike": {"metric": "", "imperial": ""},
					"windDir": "VR",
					"windSpeed": {"metric": "", "imperial": ""},
					"windGust": {"metric": "", "imperial": ""},
					"humidex": {"metric": "", "imperial": ""},
					"windChill": {"metric": "-2", "imperial": "28"},
					"iconCode": ""
				}
			],
			"timezone": "America/Toronto",
			"lat": 43.45,
			"lon": -80.49
		}
	}`)

	hours, err := parseHourlyForecastHTML([]byte(html))
	if err != nil {
		t.Fatalf("parseHourlyForecastHTML: %v", err)
	}
	if len(hours) != 2 {
		t.Fatalf("len(hours) = %d, want 2", len(hours))
	}

	first := hours[0]
	wantStart := time.Unix(1716138000, 0).UTC()
	if !first.StartsAt.Equal(wantStart) {
		t.Fatalf("StartsAt = %s, want %s", first.StartsAt, wantStart)
	}
	if !first.EndsAt.Equal(wantStart.Add(time.Hour)) {
		t.Fatalf("EndsAt = %s, want %s", first.EndsAt, wantStart.Add(time.Hour))
	}
	if first.StartsAt.Location() != time.UTC {
		t.Fatalf("StartsAt location = %s, want UTC", first.StartsAt.Location())
	}
	assertFloatPtr(t, "temperature", first.Temperature, 21)
	assertFloatPtr(t, "feels_like", first.FeelsLike, 24)
	assertIntPtr(t, "precip_chance", first.PrecipChance, 60)
	assertFloatPtr(t, "wind_speed", first.WindSpeed, 20)
	assertFloatPtr(t, "wind_gust", first.WindGust, 40)
	assertFloatPtr(t, "humidex", first.Humidex, 25)
	assertFloatPtr(t, "uv_index", first.UVIndex, 3)
	if first.WindChill != nil {
		t.Fatalf("WindChill = %v, want nil", *first.WindChill)
	}
	if first.IconURL != "https://weather.gc.ca/weathericons/small/09.png" {
		t.Fatalf("IconURL = %q", first.IconURL)
	}

	second := hours[1]
	if !second.StartsAt.After(first.StartsAt) {
		t.Fatalf("hours are not chronological: %s then %s", first.StartsAt, second.StartsAt)
	}
	if second.PrecipChance != nil {
		t.Fatalf("empty precip produced pointer: %v", *second.PrecipChance)
	}
	if second.FeelsLike != nil || second.WindSpeed != nil || second.WindGust != nil || second.Humidex != nil || second.UVIndex != nil {
		t.Fatalf("empty metric strings should become nil: %+v", second)
	}
	assertFloatPtr(t, "wind_chill", second.WindChill, -2)
	if second.IconURL != "" {
		t.Fatalf("empty iconCode produced IconURL %q", second.IconURL)
	}
}

func TestParseHourlyForecastHTMLLiveNestedShape(t *testing.T) {
	html := hourlyFixture(`{
		"location": {
			"location": {
				"43.45000000--80.49000000": {
					"hourly": [
						{
							"epochTime": 1779022800,
							"condition": "Sunny",
							"precip": "0",
							"temperature": {"metric": "16"},
							"windDir": "VR",
							"windSpeed": {"metric": "5"},
							"iconCode": "00"
						}
					],
					"timezone": "America/Toronto"
				}
			}
		}
	}`)

	hours, err := parseHourlyForecastHTML([]byte(html))
	if err != nil {
		t.Fatalf("parseHourlyForecastHTML: %v", err)
	}
	if len(hours) != 1 {
		t.Fatalf("len(hours) = %d, want 1", len(hours))
	}
	want := time.Unix(1779022800, 0).UTC()
	if !hours[0].StartsAt.Equal(want) {
		t.Fatalf("StartsAt = %s, want %s", hours[0].StartsAt, want)
	}
	assertFloatPtr(t, "temperature", hours[0].Temperature, 16)
	assertFloatPtr(t, "wind_speed", hours[0].WindSpeed, 5)
	if hours[0].IconURL != "https://weather.gc.ca/weathericons/small/00.png" {
		t.Fatalf("IconURL = %q", hours[0].IconURL)
	}
}

func TestParseHourlyForecastHTMLErrors(t *testing.T) {
	tests := []struct {
		name string
		html string
	}{
		{name: "missing state", html: `<html></html>`},
		{name: "malformed json", html: `<script>window.__INITIAL_STATE__ = {"location":</script>`},
		{name: "empty hourly", html: hourlyFixture(`{"location":{"hourly":[]}}`)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := parseHourlyForecastHTML([]byte(tt.html)); err == nil {
				t.Fatalf("parseHourlyForecastHTML succeeded, want error")
			}
		})
	}
}

func TestStationHourlyForecastURLUsesRSSCoordinateFormatting(t *testing.T) {
	got := stationHourlyForecastURL(43.4500, -80.4900001)
	want := "https://weather.gc.ca/en/forecast/hourly/index.html?coords=43.45,-80.4900001"
	if got != want {
		t.Fatalf("stationHourlyForecastURL = %q, want %q", got, want)
	}
}

func TestHourlyForecastCollectorFetchPublishesState(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, hourlyFixture(`{"location":{"hourly":[{"epochTime":1716138000,"condition":"Sunny","temperature":{"metric":"20"},"iconCode":"00"}]}}`))
	}))
	defer srv.Close()

	stateStore := store.NewStateStore()
	c := NewHourlyForecastCollector(1, 2, time.Minute, stateStore, StationOverride{})
	if err := c.fetch(srv.URL); err != nil {
		t.Fatalf("fetch: %v", err)
	}

	state, ok := stateStore.Get(domain.DomainWeatherForecastHourly).(domain.WeatherHourlyForecast)
	if !ok {
		t.Fatalf("stored state has type %T", stateStore.Get(domain.DomainWeatherForecastHourly))
	}
	if len(state.Hours) != 1 {
		t.Fatalf("len(Hours) = %d, want 1", len(state.Hours))
	}
	if state.UpdatedAt.IsZero() {
		t.Fatalf("UpdatedAt is zero")
	}
}

func TestRetainActivePreviousHour(t *testing.T) {
	currentStart := time.Date(2026, 5, 17, 17, 0, 0, 0, time.UTC)
	nextStart := currentStart.Add(time.Hour)
	previous := []domain.HourlyForecastPeriod{
		{StartsAt: currentStart, EndsAt: nextStart, Condition: "Current"},
	}
	next := domain.HourlyForecastPeriod{
		StartsAt: nextStart, EndsAt: nextStart.Add(time.Hour), Condition: "Next",
	}

	got := retainActivePreviousHour(
		[]domain.HourlyForecastPeriod{next},
		previous,
		currentStart.Add(34*time.Minute),
	)
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2", len(got))
	}
	if got[0].Condition != "Current" || got[1].Condition != "Next" {
		t.Fatalf("unexpected order: %+v", got)
	}

	expired := retainActivePreviousHour(
		[]domain.HourlyForecastPeriod{next},
		previous,
		nextStart,
	)
	if len(expired) != 1 || expired[0].Condition != "Next" {
		t.Fatalf("expired current hour was retained: %+v", expired)
	}
}

func TestRetainActivePreviousHourDoesNotDuplicate(t *testing.T) {
	currentStart := time.Date(2026, 5, 17, 17, 0, 0, 0, time.UTC)
	current := domain.HourlyForecastPeriod{
		StartsAt: currentStart, EndsAt: currentStart.Add(time.Hour), Condition: "Current",
	}
	got := retainActivePreviousHour(
		[]domain.HourlyForecastPeriod{current},
		[]domain.HourlyForecastPeriod{current},
		currentStart.Add(30*time.Minute),
	)
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1", len(got))
	}
}

func hourlyFixture(jsonState string) string {
	return `<html><body><script>window.__INITIAL_STATE__ = ` + jsonState + `;(function(){})()</script></body></html>`
}

func assertFloatPtr(t *testing.T, name string, got *float64, want float64) {
	t.Helper()
	if got == nil {
		t.Fatalf("%s = nil, want %v", name, want)
	}
	if *got != want {
		t.Fatalf("%s = %v, want %v", name, *got, want)
	}
}

func assertIntPtr(t *testing.T, name string, got *int, want int) {
	t.Helper()
	if got == nil {
		t.Fatalf("%s = nil, want %v", name, want)
	}
	if *got != want {
		t.Fatalf("%s = %v, want %v", name, *got, want)
	}
}
