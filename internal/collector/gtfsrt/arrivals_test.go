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
		w.Write(minimalGTFSZip(t))
	}))
	defer srv.Close()

	if err := gtfs.LoadAndPersist(ag.Schedule, ag.ID, []string{srv.URL}, t.TempDir(), gtfsDB); err != nil {
		t.Fatalf("LoadAndPersist() error = %v", err)
	}

	c := &ArrivalsCollector{db: gtfsDB, inFallback: make(map[string]bool)}
	stops := map[string]domain.StopArrivals{}
	wanted := map[string]bool{"test:R1:S1": true}
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
			"R1,SVC,T1,Union Station,\n",
		"calendar.txt": "service_id,monday,tuesday,wednesday,thursday,friday,saturday,sunday,start_date,end_date\n" +
			"SVC,1,1,1,1,1,1,1,20260101,20261231\n",
		"calendar_dates.txt": "service_id,date,exception_type\n",
		"stop_times.txt": "trip_id,arrival_time,departure_time,stop_id,stop_sequence\n" +
			"T1,06:00:00,06:00:00,S1,1\n",
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
