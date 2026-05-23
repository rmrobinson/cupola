# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Cupola is a self-hosted, location-aware ambient dashboard. A single Go binary serves a REST/SSE API backed by pluggable data collectors; a vanilla JS frontend renders a drag-and-drop widget canvas. Full specification: `docs/cupola-spec.md`.

## Build & Run

```bash
# Download frontend vendor dependencies (Leaflet, protomaps-leaflet)
make vendor-frontend

# Build the binary (runs vendor-frontend automatically if needed)
make build
# or: go build ./cmd/cupola

# Run with a config file
./cupola -config config.yaml

# Run tests
go test ./...

# Run a single package's tests
go test ./internal/collector/envcanada/...

# Run frontend JavaScript tests
node --test test/frontend/*.test.js

# Lint (golangci-lint expected)
golangci-lint run ./...
```

Frontend vendor libraries (`cmd/cupola/frontend/js/vendor/`) are downloaded by `make vendor-frontend` and are gitignored. Run `make vendor-frontend` again to upgrade them; bump the version variables in `Makefile`.

The frontend is static HTML/CSS/JS with no bundler or build step. It is embedded from `cmd/cupola/frontend/` by the Go binary.

## Architecture

### Backend

**Entry point:** `cmd/cupola/main.go` — loads config, registers collectors, starts the HTTP server.

**Collector pattern** (`internal/collector/`): Each data source implements `Collector`:
```go
type Collector interface {
    ID()     string
    Domain() DomainType
    Start(ctx context.Context) error
    State() DomainState
}
```
Collectors run independently and push normalized state into a central in-memory store. Only one collector may be registered per `DomainType` — enforced at startup. Most collectors are registered from `config.yaml`; notes, astro, and transit are always registered. Transit remains active so agencies can be managed dynamically through the API without restarting the service.

**Connectivity gate**: Internet-dependent collectors must implement `SetNetCheck(func() bool)` and skip internet fetches while the checker reports down. `cmd/cupola/main.go` wires the shared connectivity checker into every registered collector that exposes this method. Use this standard for new external HTTP, RSS, tile-source, transit, municipal, waterway, or email collectors. Local LAN collectors such as Ecowitt and dump1090/readsb do not need to be gated by the internet checker.

**IMAP dispatcher**: Planned but not yet implemented. The intended design is one shared IMAP connection dispatching emails to `EmailHandler` implementations based on sender/subject patterns, so only one mailbox credential is needed per site.

**State store**: In-memory, keyed by `DomainType`. The REST API reads from it; the SSE stream pushes updates when state changes.

**Subscription system** (`internal/api/subscriptions.go`): Reference-counted per `(domain, params)` pair. Widgets register on load, deregister on removal. SSE disconnect drops all subscriptions for that session. For parameterized domains (e.g. `transit.arrivals` keyed by `"{agency}:{route}:{stop_id}"`), the backend only fetches data for active subscriptions.

**Subscription refresh wakeups**: Any collector that polls external or periodically refreshed data for a widget domain should implement `collector.SubscriptionNotifiable` with `OnSubscription()`. Use a buffered `wake chan struct{}` of size 1 and select on it in the collector's existing poll loop, next to the ticker. This lets widget load, config save, and SSE reconnect re-posted subscriptions trigger an immediate refresh instead of showing stale retained state until the next poll. Keep the send non-blocking to coalesce repeated subscriptions:
```go
func (c *Collector) OnSubscription() {
    select {
    case c.wake <- struct{}{}:
    default:
    }
}
```
For aggregate collectors with multiple configured sources (RSS, municipal, waterway), give each source its own wake channel and have `OnSubscription()` nudge every source. Respect `SetNetCheck` inside the wake path just like the normal ticker path. Pure local/derived domains such as notes and astro generally do not need this pattern unless they depend on periodic recomputation.

**Persistence** (`internal/store/sqlite.go`): SQLite stores dashboard profiles, shared notes, transit agency config, and cached GTFS timetable data used for transit static-schedule fallback. It is not used for sensor, alert, or other time-series data.

**Tiles** (`internal/tiles/pmtiles.go`): On startup, checks for a `.pmtiles` cache. If absent, fetches a tile extract from Protomaps/build sources bounded by the configured location and radius, saves to disk, and serves at `GET /tiles/{z}/{x}/{y}`. Subsequent starts use the cache.

**Router** (`internal/api/router.go`): `chi` router, CORS `Access-Control-Allow-Origin: *`, security headers including CSP, static frontend serving, admin routes, transit agency routes, and tile routes.

### Domain types

All domain structs live in `internal/domain/`. `DomainType` string constants are defined in `internal/domain/types.go`. Key types: `WeatherCurrent`, `WeatherForecast`, `WeatherHourlyForecast`, `WeatherAlerts`, `SolarWeatherCurrent`, `SolarWeatherForecast`, `TransitArrivals`, `TransitVehicles`, `TransitAlerts`, `TrafficIncidents`, `TrafficCameras`, `TrafficRoadConditions`, `Aircraft`, `Astro`, `FlagStatus`, `Feeds`, `Notes`, `Home`, `WaterwayConditions`, `MunicipalEvents`, `MunicipalAlerts`, `WasteCollection`.

`AlertSeverity` (`internal/domain/alert_severity.go`) is shared across weather, transit, and municipal alert types: `info`, `watch`, `warning`, `emergency`.

### Municipal and waterway collectors (generic frameworks)

- **Municipal** (`internal/collector/municipal/`): Generic HTTP-poll collectors aggregate parser sub-packages into `municipal.events` and/or `municipal.alerts`. New municipalities should add a parser sub-package, not a new collector type. IMAP ingestion per parser is still planned.
- **Waterway** (`internal/collector/waterway/`): Generic collector with a `GaugeSource` interface. GRCA is registered from `internal/collector/waterway/grca`. When `AdvisoryStatus` matches the source config's `alert_on` list, the waterway collector promotes that gauge into `municipal.alerts`; if no municipal alerts collector exists, startup creates an empty one for promoted waterway alerts.

### Frontend

**Stack:** Vanilla HTML/CSS/JS, no framework, no build toolchain.

**Widget module contract:**
```js
{
  type: string,
  domain: string,
  defaultSize: { w, h },
  configSchema: [...],
  subscriptionParams(config),     // returns params object; null for singletons
  render(container, state, config),
  onUpdate(data, config),
}
```

**Widget lifecycle:** add → `GET /api/v1/state/:domain` → `POST /api/v1/subscriptions` → SSE updates → `onUpdate()` → remove → `DELETE /api/v1/subscriptions/:widget_id`.

**Background gradient** (`frontend/css/horizon.css`): Time-of-day CSS gradient ported from [dnlzro/horizon](https://github.com/dnlzro/horizon). `position: fixed`, transitions based on local time and `astro` domain sunrise/sunset.

**Kiosk mode:** `?profile=<id>&kiosk=1` — auto-loads profile, suppresses widget chrome and landing screen.

**System alert banner:** Non-user-dismissable banner above the grid. Appears on any SSE `{ domain: "system", status: "error" }` event; auto-dismisses when all collectors recover.

## Configuration

See `config.example.yaml`. Key sections: `location` (lat/lon/timezone/country_code), `server` (port, data_dir, CSP image sources), `tiles` (radius_km, cache_path, optional source_key), `connectivity` (optional check URL and interval), and `collectors` (one block per collector type). Sensitive values such as IMAP passwords support `${ENV_VAR}` interpolation.

## API Endpoints

| Method | Path | Purpose |
|--------|------|---------|
| GET | `/api/v1/domains` | Active domain types on this instance |
| GET | `/api/v1/state/:domain` | Current state snapshot |
| GET | `/api/v1/details/:domain` | Detail payload for supported domains |
| GET | `/api/v1/stream` | SSE stream of domain + system events |
| POST | `/api/v1/subscriptions` | Register widget data needs |
| DELETE | `/api/v1/subscriptions/:widget_id` | Deregister widget |
| GET/POST/DELETE | `/api/v1/profiles` | Dashboard profile CRUD plus import/export routes |
| GET/POST/PATCH/DELETE | `/api/v1/notes` | Shared notes |
| GET/POST/PATCH/DELETE | `/api/v1/transit/agency-configs` | Transit agency configuration CRUD |
| GET | `/api/v1/transit/agencies/...` | Transit agency route/stop/shape lookup |
| GET/PATCH | `/api/v1/admin/...` | Admin collector/connectivity endpoints |
| GET | `/tiles/{z}/{x}/{y}` | Protomaps tile serving |

## Implementation Phases

Refer to `docs/cupola-spec.md` §11 for the original 26-phase build order. The implemented set is ahead of the original skeleton: core backend/frontend, profiles, tiles, widget picker, core/weather/transit/map/traffic/aircraft/solar/municipal/waterway/RSS/flag/notes/waste collectors, alerts and municipal widgets, GTFS agency management, admin connectivity controls, and kiosk mode are present. Still planned or deferred: house camera collector, shared IMAP dispatcher and IMAP handlers, and any remaining PWA offline polish.

## Key Constraints

- Only one collector per `DomainType` — fatal error at startup if violated.
- Internet-dependent collectors must implement `SetNetCheck(func() bool)` and avoid external fetches while connectivity is down.
- `transit.arrivals` state is keyed by `"{agency}:{route}:{stop_id}"` — only subscribed combinations are populated.
- Space weather region (1–7) is auto-selected from `lat/lon` at startup; can be overridden in config.
- `municipal.events` is intentionally excluded from the alerts widget; it has its own `municipal-events` widget.
- Tile cache is permanent until manually deleted — delete `data/tiles/local.pmtiles` and restart to refresh.
