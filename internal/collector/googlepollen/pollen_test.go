package googlepollen

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/rmrobinson/cupola/internal/domain"
	"github.com/rmrobinson/cupola/internal/store"
	"google.golang.org/api/option"
	pollen "google.golang.org/api/pollen/v1"
)

type fakeClient struct {
	req   Request
	resp  *pollen.LookupForecastResponse
	err   error
	calls int
}

func (f *fakeClient) Lookup(_ context.Context, req Request) (*pollen.LookupForecastResponse, error) {
	f.req = req
	f.calls++
	return f.resp, f.err
}

func TestResolveOptionsDefaultsAndCapsDays(t *testing.T) {
	got := ResolveOptions(43.45, -80.49, "America/Toronto", -time.Hour, 8, " fr-CA ")
	if got.Interval != 12*time.Hour {
		t.Fatalf("interval = %s, want 12h", got.Interval)
	}
	if got.Days != 5 {
		t.Fatalf("days = %d, want 5", got.Days)
	}
	if got.LanguageCode != "fr-CA" {
		t.Fatalf("language = %q, want fr-CA", got.LanguageCode)
	}
}

func TestNewWithClientDefaultsNegativeInterval(t *testing.T) {
	col := NewWithClient(&fakeClient{}, Options{Latitude: 1, Longitude: 2, Timezone: "UTC", Interval: -time.Minute, Days: 1}, store.NewStateStore())
	if col.opts.Interval != 12*time.Hour {
		t.Fatalf("interval = %s, want 12h", col.opts.Interval)
	}
}

func TestEstimatedMonthlyRequestsFreeTierThreshold(t *testing.T) {
	if got := EstimatedMonthlyRequests(12 * time.Hour); got != 60 {
		t.Fatalf("requests = %d, want 60", got)
	}
	if ExceedsFreeTier(9 * time.Minute) {
		t.Fatalf("9m should stay under free tier")
	}
	if !ExceedsFreeTier(8 * time.Minute) {
		t.Fatalf("8m should exceed free tier")
	}
}

func TestSDKAdapterBuildsLookupRequest(t *testing.T) {
	var query url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/forecast:lookup" {
			t.Fatalf("path = %q, want /v1/forecast:lookup", r.URL.Path)
		}
		query = r.URL.Query()
		_ = json.NewEncoder(w).Encode(&pollen.LookupForecastResponse{RegionCode: "CA"})
	}))
	defer srv.Close()

	svc, err := pollen.NewService(context.Background(), option.WithHTTPClient(srv.Client()), option.WithEndpoint(srv.URL+"/"), option.WithAPIKey("test-key"))
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	client := sdkClient{service: svc}
	resp, err := client.Lookup(context.Background(), Request{Latitude: 43.45, Longitude: -80.49, Days: 4, LanguageCode: "en-CA"})
	if err != nil {
		t.Fatalf("Lookup() error = %v", err)
	}
	if resp.RegionCode != "CA" {
		t.Fatalf("region = %q, want CA", resp.RegionCode)
	}
	assertQuery := func(key, want string) {
		t.Helper()
		if got := query.Get(key); got != want {
			t.Fatalf("%s = %q, want %q; query=%v", key, got, want, query)
		}
	}
	assertQuery("location.latitude", "43.45")
	assertQuery("location.longitude", "-80.49")
	assertQuery("days", "4")
	assertQuery("languageCode", "en-CA")
	assertQuery("plantsDescription", "false")
}

func TestMapResponseSelectsCurrentByLocalDateAndAggregates(t *testing.T) {
	loc, _ := time.LoadLocation("America/Toronto")
	resp := &pollen.LookupForecastResponse{
		RegionCode: "CA",
		DailyInfo: []*pollen.DayInfo{
			dayInfo(2026, 6, 6,
				[]*pollen.PollenTypeInfo{
					typeInfo("GRASS", "Grass", true, 5, "Very high", "Grass very high", 1, 0.6, 0),
					typeInfo("TREE", "Tree", false, 5, "Very high", "Tree very high", 1, 0, 0),
					{Code: "WEED", DisplayName: "Weed", InSeason: true},
				},
				[]*pollen.PlantInfo{
					plantInfo("RAGWEED", "Ragweed", true, 5, "Very high", "Ragweed very high", 0.8, 0.2, 0.2),
				}),
			dayInfo(2026, 6, 7, []*pollen.PollenTypeInfo{typeInfo("GRASS", "Grass", true, 1, "Very low", "", 0, 1, 0)}, nil),
		},
	}
	state := MapResponse(resp, time.Date(2026, 6, 6, 15, 0, 0, 0, time.UTC), loc)
	if state.RegionCode != "CA" || state.Source != sourceName {
		t.Fatalf("unexpected state metadata: %+v", state)
	}
	if state.Current == nil || state.Current.Date != "2026-06-06" {
		t.Fatalf("current = %+v, want 2026-06-06", state.Current)
	}
	if got := len(state.Days); got != 2 {
		t.Fatalf("days = %d, want 2", got)
	}
	if state.Current.Aggregate == nil || state.Current.Aggregate.Code != "GRASS" || state.Current.Aggregate.Value != 5 {
		t.Fatalf("aggregate = %+v, want in-season type GRASS value 5", state.Current.Aggregate)
	}
	if state.Current.Types[2].UPI != nil {
		t.Fatalf("missing indexInfo should leave UPI nil")
	}
	if got := state.Current.Types[0].UPI.Color; got != "rgba(255,153,0,1)" {
		t.Fatalf("color = %q", got)
	}
	if got := state.Current.HealthRecommendations; len(got) != 1 || got[0] != "Close windows" {
		t.Fatalf("recommendations = %#v", got)
	}
}

func TestMapResponseAggregateTieBreakUsesStableCodeOrder(t *testing.T) {
	loc, _ := time.LoadLocation("America/Toronto")
	resp := &pollen.LookupForecastResponse{
		DailyInfo: []*pollen.DayInfo{
			dayInfo(2026, 6, 6,
				[]*pollen.PollenTypeInfo{
					typeInfo("WEED", "Weed", true, 3, "Moderate", "", 0.5, 0.5, 0),
					typeInfo("GRASS", "Grass", true, 3, "Moderate", "", 0.5, 0.5, 0),
					typeInfo("TREE", "Tree", true, 3, "Moderate", "", 0.5, 0.5, 0),
				},
				nil),
		},
	}
	state := MapResponse(resp, time.Date(2026, 6, 6, 15, 0, 0, 0, time.UTC), loc)
	if state.Current == nil || state.Current.Aggregate == nil {
		t.Fatalf("current aggregate missing: %+v", state.Current)
	}
	if state.Current.Aggregate.Code != "GRASS" {
		t.Fatalf("aggregate code = %q, want GRASS", state.Current.Aggregate.Code)
	}
}

func TestMapResponsePlantAggregateTieBreakUsesStableCodeOrder(t *testing.T) {
	loc, _ := time.LoadLocation("America/Toronto")
	resp := &pollen.LookupForecastResponse{
		DailyInfo: []*pollen.DayInfo{
			dayInfo(2026, 6, 6,
				nil,
				[]*pollen.PlantInfo{
					plantInfo("OAK", "Oak", true, 4, "High", "", 0.8, 0.2, 0.2),
					plantInfo("BIRCH", "Birch", true, 4, "High", "", 0.8, 0.2, 0.2),
				}),
		},
	}
	state := MapResponse(resp, time.Date(2026, 6, 6, 15, 0, 0, 0, time.UTC), loc)
	if state.Current == nil || state.Current.Aggregate == nil {
		t.Fatalf("current aggregate missing: %+v", state.Current)
	}
	if state.Current.Aggregate.Code != "BIRCH" {
		t.Fatalf("aggregate code = %q, want BIRCH", state.Current.Aggregate.Code)
	}
}

func TestMapResponseTomorrowFirstHasNoCurrent(t *testing.T) {
	loc, _ := time.LoadLocation("America/Toronto")
	state := MapResponse(&pollen.LookupForecastResponse{
		DailyInfo: []*pollen.DayInfo{dayInfo(2026, 6, 7, []*pollen.PollenTypeInfo{typeInfo("GRASS", "Grass", true, 2, "Low", "", 0, 1, 0)}, nil)},
	}, time.Date(2026, 6, 6, 15, 0, 0, 0, time.UTC), loc)
	if state.Current != nil {
		t.Fatalf("current = %+v, want nil", state.Current)
	}
	if len(state.Days) != 1 {
		t.Fatalf("days = %d, want 1", len(state.Days))
	}
}

func TestCollectorFetchPublishesAndUsesRequest(t *testing.T) {
	fake := &fakeClient{resp: &pollen.LookupForecastResponse{DailyInfo: []*pollen.DayInfo{dayInfo(2026, 6, 6, nil, nil)}}}
	st := store.NewStateStore()
	col := NewWithClient(fake, ResolveOptions(43.45, -80.49, "America/Toronto", time.Hour, 3, "en"), st)
	col.now = func() time.Time { return time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC) }
	if err := col.Fetch(context.Background()); err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if fake.req.Latitude != 43.45 || fake.req.Longitude != -80.49 || fake.req.Days != 3 || fake.req.LanguageCode != "en" {
		t.Fatalf("request = %+v", fake.req)
	}
	if _, ok := st.Get(domain.DomainWeatherPollen).(domain.WeatherPollen); !ok {
		t.Fatalf("weather.pollen state was not published")
	}
}

func TestCollectorSkipsFetchWhenNetworkDown(t *testing.T) {
	fake := &fakeClient{resp: &pollen.LookupForecastResponse{}}
	col := NewWithClient(fake, ResolveOptions(1, 2, "UTC", time.Hour, 1, ""), store.NewStateStore())
	col.SetNetCheck(func() bool { return false })
	col.fetchIfReady(context.Background())
	if fake.calls != 0 {
		t.Fatalf("calls = %d, want 0", fake.calls)
	}
}

func TestCollectorThrottlesSubscriptionWakeFetches(t *testing.T) {
	fake := &fakeClient{resp: &pollen.LookupForecastResponse{}}
	col := NewWithClient(fake, ResolveOptions(1, 2, "UTC", 12*time.Hour, 1, ""), store.NewStateStore())
	now := time.Date(2026, 6, 6, 14, 0, 0, 0, time.UTC)
	col.now = func() time.Time { return now }

	col.fetchIfReady(context.Background())
	col.fetchIfReady(context.Background())
	col.fetchIfReady(context.Background())

	if fake.calls != 1 {
		t.Fatalf("calls = %d, want 1", fake.calls)
	}

	now = now.Add(12*time.Hour - time.Second)
	col.fetchIfReady(context.Background())
	if fake.calls != 1 {
		t.Fatalf("calls before interval = %d, want 1", fake.calls)
	}

	now = now.Add(time.Second)
	col.fetchIfReady(context.Background())
	if fake.calls != 2 {
		t.Fatalf("calls after interval = %d, want 2", fake.calls)
	}
}

func TestCollectorDoesNotThrottleAfterFailure(t *testing.T) {
	fake := &fakeClient{err: errors.New("boom")}
	col := NewWithClient(fake, ResolveOptions(1, 2, "UTC", 12*time.Hour, 1, ""), store.NewStateStore())
	now := time.Date(2026, 6, 6, 14, 0, 0, 0, time.UTC)
	col.now = func() time.Time { return now }

	col.fetchIfReady(context.Background())
	col.fetchIfReady(context.Background())

	if fake.calls != 2 {
		t.Fatalf("calls after repeated failures = %d, want 2", fake.calls)
	}

	fake.err = nil
	fake.resp = &pollen.LookupForecastResponse{}
	col.fetchIfReady(context.Background())
	col.fetchIfReady(context.Background())

	if fake.calls != 3 {
		t.Fatalf("calls after success and immediate wake = %d, want 3", fake.calls)
	}
}

func TestCollectorStartRequiresClient(t *testing.T) {
	col := NewWithClient(nil, ResolveOptions(1, 2, "UTC", time.Hour, 1, ""), store.NewStateStore())
	if err := col.Start(context.Background()); err == nil {
		t.Fatal("Start() error = nil, want error")
	}
}

func dayInfo(year, month, day int64, types []*pollen.PollenTypeInfo, plants []*pollen.PlantInfo) *pollen.DayInfo {
	return &pollen.DayInfo{Date: &pollen.Date{Year: year, Month: month, Day: day}, PollenTypeInfo: types, PlantInfo: plants}
}

func typeInfo(code, name string, inSeason bool, value int64, category, desc string, red, green, blue float64) *pollen.PollenTypeInfo {
	return &pollen.PollenTypeInfo{
		Code:                  code,
		DisplayName:           name,
		InSeason:              inSeason,
		HealthRecommendations: []string{"Close windows"},
		IndexInfo:             index(value, category, desc, red, green, blue),
	}
}

func plantInfo(code, name string, inSeason bool, value int64, category, desc string, red, green, blue float64) *pollen.PlantInfo {
	return &pollen.PlantInfo{Code: code, DisplayName: name, InSeason: inSeason, IndexInfo: index(value, category, desc, red, green, blue)}
}

func index(value int64, category, desc string, red, green, blue float64) *pollen.IndexInfo {
	return &pollen.IndexInfo{Value: value, Category: category, IndexDescription: desc, Color: &pollen.Color{Red: red, Green: green, Blue: blue}}
}
