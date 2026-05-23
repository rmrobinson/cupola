package gtfsrt

import (
	"archive/zip"
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rmrobinson/cupola/internal/collector/gtfs"
	"github.com/rmrobinson/cupola/internal/domain"
	"github.com/rmrobinson/cupola/internal/store"
)

func TestFetchStaticForAgencyPublishesEmptyStopArrivalsWithNames(t *testing.T) {
	gtfsDB, err := store.NewGTFSSQLiteStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewGTFSSQLiteStore() error = %v", err)
	}

	ag := &Agency{ID: "test", Schedule: gtfs.New()}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/zip")
		_, _ = w.Write(minimalGTFSZip(t))
	}))
	defer srv.Close()

	if err := gtfs.LoadAndPersist(ag.Schedule, ag.ID, []string{srv.URL}, t.TempDir(), gtfsDB); err != nil {
		t.Fatalf("LoadAndPersist() error = %v", err)
	}

	c := &ArrivalsCollector{db: gtfsDB, inFallback: make(map[string]bool)}
	stops := map[string]domain.StopArrivals{}
	wanted := map[string]int{"test:R1:S1": 4}
	afterFeedServiceEnds := time.Date(2027, time.January, 1, 12, 0, 0, 0, time.UTC)

	c.fetchStaticForAgency(ag, wanted, stops, afterFeedServiceEnds, false)

	sa, ok := stops["test:R1:S1"]
	if !ok {
		t.Fatal("expected static fallback to publish empty stop arrivals")
	}
	if sa.AgencyID != "test" || sa.RouteID != "R1" || sa.StopID != "S1" {
		t.Fatalf("stop arrivals identifiers = %+v", sa)
	}
	if sa.RouteName != "KI" {
		t.Fatalf("RouteName = %q, want %q", sa.RouteName, "KI")
	}
	if sa.StopName != "Kitchener GO" {
		t.Fatalf("StopName = %q, want %q", sa.StopName, "Kitchener GO")
	}
	if len(sa.Arrivals) != 0 {
		t.Fatalf("Arrivals length = %d, want 0", len(sa.Arrivals))
	}
}

func TestRequestedArrivalLimit(t *testing.T) {
	tests := []struct {
		name string
		raw  any
		want int
	}{
		{name: "missing defaults to four", raw: nil, want: 4},
		{name: "float from json is accepted", raw: float64(12), want: 12},
		{name: "string is accepted", raw: "7", want: 7},
		{name: "invalid string defaults", raw: "many", want: 4},
		{name: "clamped to max", raw: 99, want: maxArrivalsPerStop},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := requestedArrivalLimit(tt.raw); got != tt.want {
				t.Fatalf("requestedArrivalLimit(%v) = %d, want %d", tt.raw, got, tt.want)
			}
		})
	}
}

func TestFetchStaticForAgencyTopsUpRealtimeArrivalsWithoutDuplicates(t *testing.T) {
	gtfsDB, err := store.NewGTFSSQLiteStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewGTFSSQLiteStore() error = %v", err)
	}

	ag := &Agency{ID: "test", Schedule: gtfs.New()}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/zip")
		_, _ = w.Write(minimalGTFSZip(t))
	}))
	defer srv.Close()

	if err := gtfs.LoadAndPersist(ag.Schedule, ag.ID, []string{srv.URL}, t.TempDir(), gtfsDB); err != nil {
		t.Fatalf("LoadAndPersist() error = %v", err)
	}

	c := &ArrivalsCollector{db: gtfsDB, inFallback: make(map[string]bool)}
	stops := map[string]domain.StopArrivals{
		"test:R1:S1": {
			AgencyID:  "test",
			RouteID:   "R1",
			RouteName: "KI",
			StopID:    "S1",
			StopName:  "Kitchener GO",
			Arrivals: []domain.Arrival{{
				TripID:    "T1",
				Headsign:  "Union",
				Scheduled: time.Date(2026, time.January, 5, 10, 0, 0, 0, time.UTC),
			}},
		},
	}

	c.fetchStaticForAgency(
		ag,
		map[string]int{"test:R1:S1": 3},
		stops,
		time.Date(2026, time.January, 5, 9, 0, 0, 0, time.UTC),
		false,
	)

	arrivals := stops["test:R1:S1"].Arrivals
	if len(arrivals) != 3 {
		t.Fatalf("Arrivals length = %d, want 3: %+v", len(arrivals), arrivals)
	}
	if arrivals[0].TripID != "T1" || arrivals[1].TripID != "T2" || arrivals[2].TripID != "T3" {
		t.Fatalf("TripIDs = %q, %q, %q; want T1, T2, T3", arrivals[0].TripID, arrivals[1].TripID, arrivals[2].TripID)
	}
}

func minimalGTFSZip(t *testing.T) []byte {
	t.Helper()

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	files := map[string]string{
		"routes.txt": "route_id,agency_id,route_short_name,route_long_name,route_type\n" +
			"R1,test,KI,Kitchener Line,2\n",
		"stops.txt": "stop_id,stop_code,stop_name,stop_lat,stop_lon\n" +
			"S1,KI,Kitchener GO,43.4516,-80.4925\n",
		"trips.txt": "route_id,service_id,trip_id,trip_headsign,shape_id\n" +
			"R1,SVC,T1,Union Station,\n" +
			"R1,SVC,T2,Union Station,\n" +
			"R1,SVC,T3,Union Station,\n",
		"calendar.txt": "service_id,monday,tuesday,wednesday,thursday,friday,saturday,sunday,start_date,end_date\n" +
			"SVC,1,1,1,1,1,1,1,20260101,20261231\n",
		"calendar_dates.txt": "service_id,date,exception_type\n",
		"stop_times.txt": "trip_id,arrival_time,departure_time,stop_id,stop_sequence\n" +
			"T1,10:00:00,10:00:00,S1,1\n" +
			"T2,11:00:00,11:00:00,S1,1\n" +
			"T3,12:00:00,12:00:00,S1,1\n",
	}
	for name, contents := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("create %s in zip: %v", name, err)
		}
		if _, err := w.Write([]byte(contents)); err != nil {
			t.Fatalf("write %s in zip: %v", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	return buf.Bytes()
}
