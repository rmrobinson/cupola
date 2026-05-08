package main

import (
	"context"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/rmrobinson/cupola/internal/api"
	"github.com/rmrobinson/cupola/internal/collector"
	"github.com/rmrobinson/cupola/internal/collector/astro"
	"github.com/rmrobinson/cupola/internal/collector/dump1090"
	ecowittcollector "github.com/rmrobinson/cupola/internal/collector/ecowitt"
	"github.com/rmrobinson/cupola/internal/collector/envcanada"
	flagcollector "github.com/rmrobinson/cupola/internal/collector/flag"
	"github.com/rmrobinson/cupola/internal/collector/gtfsrt"
	municipalcollector "github.com/rmrobinson/cupola/internal/collector/municipal"
	_ "github.com/rmrobinson/cupola/internal/collector/municipal/enovapower"
	_ "github.com/rmrobinson/cupola/internal/collector/municipal/grcaflood"
	_ "github.com/rmrobinson/cupola/internal/collector/municipal/kitchenersnow"
	_ "github.com/rmrobinson/cupola/internal/collector/municipal/kitchenerutilities"
	notescollector "github.com/rmrobinson/cupola/internal/collector/notes"
	rsscollector "github.com/rmrobinson/cupola/internal/collector/rss"
	"github.com/rmrobinson/cupola/internal/collector/traffic511"
	wastecollector "github.com/rmrobinson/cupola/internal/collector/wastecollection"
	waterwaycollector "github.com/rmrobinson/cupola/internal/collector/waterway"
	_ "github.com/rmrobinson/cupola/internal/collector/waterway/grca"
	"github.com/rmrobinson/cupola/internal/config"
	"github.com/rmrobinson/cupola/internal/domain"
	"github.com/rmrobinson/cupola/internal/store"
	"github.com/rmrobinson/cupola/internal/tiles"
)

var transitAgencyIDRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`)

func main() {
	cfgPath := flag.String("config", "config.yaml", "path to config file")
	flag.Parse()

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	log.Printf("cupola starting for %s (%.4f, %.4f)", cfg.Location.Name, cfg.Location.Lat, cfg.Location.Lon)

	loc, err := time.LoadLocation(cfg.Location.Timezone)
	if err != nil {
		log.Printf("invalid timezone %q, using UTC: %v", cfg.Location.Timezone, err)
		loc = time.UTC
	}

	sqliteStore, err := store.NewSQLiteStore(cfg.Server.DataDir)
	if err != nil {
		log.Fatalf("open sqlite: %v", err)
	}

	registry := collector.NewRegistry()
	stateStore := store.NewStateStore()

	notesCol := notescollector.New(sqliteStore, stateStore)
	registry.Register(notesCol)

	// Ephem: always active — pure local computation, no network.
	registry.Register(astro.New(cfg.Location.Lat, cfg.Location.Lon, cfg.Location.Timezone, stateStore))

	// Environment Canada forecast + alerts via coordinate-based RSS feeds.
	// No station discovery needed — the EC server resolves lat/lon to the nearest station.
	if c := cfg.Collectors.WeatherEnvCanada; c != nil && c.Enabled {
		fcInterval := c.PollIntervalForecast.Duration
		if fcInterval == 0 {
			fcInterval = time.Hour
		}
		alertInterval := c.PollIntervalAlerts.Duration
		if alertInterval == 0 {
			alertInterval = 15 * time.Minute
		}
		log.Printf("envcanada: registering RSS collectors for %.3f,%.3f",
			cfg.Location.Lat, cfg.Location.Lon)
		registry.Register(envcanada.NewForecastCollector(cfg.Location.Lat, cfg.Location.Lon, fcInterval, stateStore))
		registry.Register(envcanada.NewAlertsCollector(cfg.Location.Lat, cfg.Location.Lon, alertInterval, stateStore))
	}

	// Canada flag status: HTML scrape of canada.ca.
	if c := cfg.Collectors.FlagCanada; c != nil && c.Enabled {
		interval := c.PollInterval.Duration
		if interval == 0 {
			interval = 4 * time.Hour
		}
		registry.Register(flagcollector.NewCanadaWithURL(c.URL, cfg.Location.Lat, cfg.Location.Lon, interval, stateStore))
	}

	// Ecowitt GW2000 local weather station.
	if c := cfg.Collectors.WeatherEcowitt; c != nil && c.Enabled && c.URL != "" {
		interval := c.PollInterval.Duration
		if interval == 0 {
			interval = time.Minute
		}
		registry.Register(ecowittcollector.New(c.URL, interval, stateStore))
	}

	// Space Weather Canada solar activity (current + forecast).
	if c := cfg.Collectors.SolarEnvCanada; c != nil && c.Enabled {
		interval := c.PollInterval.Duration
		if interval == 0 {
			interval = time.Hour
		}
		cur, fc := envcanada.NewSolarCollectors(cfg.Location.Lat, cfg.Location.Lon, interval, stateStore, c.Region)
		registry.Register(cur)
		registry.Register(fc)
	}

	// RSS feeds.
	if len(cfg.Collectors.RSSFeeds) > 0 {
		registry.Register(rsscollector.New(cfg.Collectors.RSSFeeds, stateStore))
	}

	// ADS-B aircraft via local dump1090 or readsb instance.
	if a := cfg.Collectors.AircraftDump1090; a != nil && a.Enabled && a.URL != "" {
		interval := a.PollInterval.Duration
		if interval == 0 {
			interval = 5 * time.Second
		}
		radiusKM := a.RadiusKM
		if radiusKM == 0 {
			radiusKM = 250
		}
		log.Printf("dump1090: registering collector for %s (radius=%.0fkm)", a.URL, radiusKM)
		registry.Register(dump1090.New(a.URL, interval, cfg.Location.Lat, cfg.Location.Lon, radiusKM, stateStore))
	}

	// ON511 traffic: incidents, cameras, and road conditions.
	if t := cfg.Collectors.Traffic511; t != nil && t.Enabled {
		inc, cam, road := traffic511.NewCollectors(t.PollIntervalIncidents.Duration, t.PollIntervalCameras.Duration, stateStore)
		registry.Register(inc)
		registry.Register(cam)
		registry.Register(road)
		log.Printf("traffic511: registered incidents, cameras, and road conditions collectors")
	}

	webFS, err := fs.Sub(frontendFS, "frontend")
	if err != nil {
		log.Fatalf("frontend fs: %v", err)
	}

	subManager := store.NewSubscriptionManager()

	if err := seedTransitAgencies(sqliteStore, cfg.Collectors.Transit); err != nil {
		log.Fatalf("seed transit agencies: %v", err)
	}
	transitAgencies, err := gtfsrt.NewAgencyManager(sqliteStore, cfg.Server.DataDir)
	if err != nil {
		log.Fatalf("load transit agencies: %v", err)
	}

	// Transit collectors are always registered so agencies can be added via the
	// API later without restarting the service.
	{
		var rtInterval, staticInterval time.Duration
		if t := cfg.Collectors.Transit; t != nil {
			rtInterval = t.RTPollInterval.Duration
			staticInterval = t.StaticRefreshInterval.Duration
		}
		if rtInterval == 0 {
			rtInterval = 30 * time.Second
		}
		if staticInterval == 0 {
			staticInterval = 24 * time.Hour
		}

		log.Printf("transit: registering collectors for %d enabled agencies (rt=%s, static=%s)",
			len(transitAgencies.List()), rtInterval, staticInterval)
		arr, veh, alt := gtfsrt.NewCollectors(transitAgencies, subManager, stateStore, rtInterval, staticInterval, cfg.Server.DataDir, sqliteStore, loc)
		registry.Register(arr)
		registry.Register(veh)
		registry.Register(alt)
	}

	// Municipal collectors: one EventsCollector and/or one AlertsCollector,
	// each aggregating across all configured parsers for that domain.
	// munAlertsCollector is captured so the waterway collector can promote alerts into it.
	var munAlertsCollector *municipalcollector.AlertsCollector
	if len(cfg.Collectors.Municipal) > 0 {
		var eventsCfgs, alertsCfgs []config.MunicipalConfig
		for _, mc := range cfg.Collectors.Municipal {
			switch mc.Domain {
			case string(domain.DomainMunicipalEvents):
				eventsCfgs = append(eventsCfgs, mc)
			case string(domain.DomainMunicipalAlerts):
				alertsCfgs = append(alertsCfgs, mc)
			default:
				log.Printf("municipal: unknown domain %q for source %s — skipping", mc.Domain, mc.ID)
			}
		}
		if len(eventsCfgs) > 0 {
			log.Printf("municipal.events: registering %d source(s)", len(eventsCfgs))
			registry.Register(municipalcollector.NewEventsCollector(eventsCfgs, stateStore))
		}
		if len(alertsCfgs) > 0 {
			log.Printf("municipal.alerts: registering %d source(s)", len(alertsCfgs))
			munAlertsCollector = municipalcollector.NewAlertsCollector(alertsCfgs, stateStore)
			registry.Register(munAlertsCollector)
		}
	}

	// Waterway collector: GRCA gauge + reservoir data, with optional alert promotion.
	// If alert_on is configured but no municipal.alerts collector exists, create an empty
	// one so promoted alerts have a domain owner.
	if len(cfg.Collectors.Waterways) > 0 {
		if munAlertsCollector == nil {
			for _, wc := range cfg.Collectors.Waterways {
				if len(wc.AlertOn) > 0 {
					log.Printf("municipal.alerts: creating collector for waterway alert promotion")
					munAlertsCollector = municipalcollector.NewAlertsCollector(nil, stateStore)
					registry.Register(munAlertsCollector)
					break
				}
			}
		}
		log.Printf("waterway: registering %d source(s)", len(cfg.Collectors.Waterways))
		registry.Register(waterwaycollector.NewCollector(cfg.Collectors.Waterways, stateStore, munAlertsCollector))
	}

	// Waste collection schedule from a local JSON file.
	if w := cfg.Collectors.WasteCollection; w != nil && w.Enabled && w.DataPath != "" {
		ws := wastecollector.ParseWeekday(w.WeekStart)
		log.Printf("waste.collection: registering collector (data=%s week_start=%s)", w.DataPath, ws)
		registry.Register(wastecollector.New(w.DataPath, ws, stateStore))
	}

	// ctx is shared by collectors, the HTTP server's BaseContext, and tile extraction.
	// Cancelling it signals SSE connections and collector goroutines to stop cleanly.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Tiles: use cached file if present; extract from build.protomaps.com on first run.
	// Extraction blocks startup (first-run only). On error, /tiles returns 404.
	var tileHandler *tiles.Handler
	if cfg.Tiles.CachePath != "" {
		radiusKM := cfg.Tiles.RadiusKM
		if radiusKM == 0 {
			radiusKM = 50
		}
		h, err := tiles.New(ctx, cfg.Tiles.CachePath, cfg.Location.Lat, cfg.Location.Lon, radiusKM, cfg.Tiles.SourceKey)
		if err != nil {
			log.Printf("tiles: %v — tile serving disabled", err)
		} else {
			tileHandler = h
		}
	}

	handler := api.NewHandler(registry, stateStore, sqliteStore, subManager, notesCol.Refresh, tileHandler, webFS,
		transitAgencies, cfg.Location.Lat, cfg.Location.Lon, cfg.Location.CountryCode, cfg.Server.CSPImgSrc)

	port := cfg.Server.Port
	if port == 0 {
		port = 8080
	}
	srv := &http.Server{
		Addr:        fmt.Sprintf(":%d", port),
		Handler:     handler.Router(),
		BaseContext: func(_ net.Listener) context.Context { return ctx },
	}

	if err := registry.StartAll(ctx); err != nil {
		log.Fatalf("start collectors: %v", err)
	}

	go func() {
		log.Printf("listening on %s", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("shutting down")
	cancel() // unblocks SSE handlers and collector goroutines

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("shutdown: %v", err)
	}
}

func seedTransitAgencies(db *store.SQLiteStore, cfg *config.TransitConfig) error {
	if cfg == nil || len(cfg.Agencies) == 0 {
		return nil
	}
	log.Printf("transit: collectors.transit.agencies is deprecated; seeding missing agencies into SQLite")
	for _, ac := range cfg.Agencies {
		if err := validateYAMLTransitAgency(ac); err != nil {
			log.Printf("transit: skipping YAML agency %q: %v", ac.ID, err)
			continue
		}
		inserted, err := db.InsertTransitAgencyIfMissing(store.TransitAgencyConfig{
			ID:                         strings.TrimSpace(ac.ID),
			Enabled:                    true,
			GTFSStaticURLs:             normalizeYAMLURLs(ac.GTFSStaticURLs),
			GTFSRTTripUpdatesURLs:      normalizeYAMLURLs(ac.GTFSRTTripUpdatesURLs),
			GTFSRTVehiclePositionsURLs: normalizeYAMLURLs(ac.GTFSRTVehiclePositionsURLs),
			GTFSRTAlertsURL:            strings.TrimSpace(ac.GTFSRTAlertsURL),
		})
		if err != nil {
			return err
		}
		if inserted {
			log.Printf("transit: migrated YAML agency %s into SQLite", ac.ID)
		}
	}
	return nil
}

func validateYAMLTransitAgency(ac config.TransitAgencyConfig) error {
	if !transitAgencyIDRe.MatchString(strings.TrimSpace(ac.ID)) {
		return fmt.Errorf("invalid agency id")
	}
	if len(ac.GTFSStaticURLs) == 0 {
		return fmt.Errorf("missing gtfs_static_urls")
	}
	if len(ac.GTFSRTTripUpdatesURLs) == 0 {
		return fmt.Errorf("missing gtfs_rt_trip_updates_urls")
	}
	for _, raw := range append(append(normalizeYAMLURLs(ac.GTFSStaticURLs), normalizeYAMLURLs(ac.GTFSRTTripUpdatesURLs)...), normalizeYAMLURLs(ac.GTFSRTVehiclePositionsURLs)...) {
		if err := validateYAMLTransitURL(raw); err != nil {
			return err
		}
	}
	if strings.TrimSpace(ac.GTFSRTAlertsURL) != "" {
		if err := validateYAMLTransitURL(ac.GTFSRTAlertsURL); err != nil {
			return err
		}
	}
	return nil
}

func normalizeYAMLURLs(in []string) []string {
	out := make([]string, 0, len(in))
	for _, raw := range in {
		out = append(out, strings.TrimSpace(raw))
	}
	return out
}

func validateYAMLTransitURL(raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return fmt.Errorf("feed URLs must be absolute http or https URLs")
	}
	return nil
}
