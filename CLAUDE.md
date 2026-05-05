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

# Lint (golangci-lint expected)
golangci-lint run ./...
```

Frontend vendor libraries (`frontend/js/vendor/`) are downloaded by `make vendor-frontend` and are gitignored. Run `make vendor-frontend` again to upgrade them; bump the version variables in `Makefile`.

The frontend is static HTML/CSS/JS — no build step. Serve from `frontend/` via the Go binary (embedded or from disk).

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
Collectors run independently and push normalized state into a central in-memory store. Only one collector may be registered per `DomainType` — enforced at startup. Collectors declared in `config.yaml` are registered; unconfigured collectors are not loaded.

**IMAP dispatcher** (`internal/collector/imap/`): One shared IMAP connection dispatches emails to `EmailHandler` implementations based on sender/subject patterns. Municipal collectors register handlers at startup so only one IMAP credential is needed.

**State store**: In-memory, keyed by `DomainType`. The REST API reads from it; the SSE stream pushes updates when state changes.

**Subscription system** (`internal/api/subscriptions.go`): Reference-counted per `(domain, params)` pair. Widgets register on load, deregister on removal. SSE disconnect drops all subscriptions for that session. For parameterized domains (e.g. `transit.arrivals` keyed by `"{agency}:{route}:{stop_id}"`), the backend only fetches data for active subscriptions.

**Persistence** (`internal/store/sqlite.go`): SQLite used only for profile storage and shared notes. No time-series data.

**Tiles** (`internal/tiles/pmtiles.go`): On startup, checks for a `.pmtiles` cache. If absent, fetches a tile extract from `api.protomaps.com` bounded by `lat/lon ± tiles_radius_km`, saves to disk, and serves at `GET /tiles/{z}/{x}/{y}`. Subsequent starts use the cache.

**Router** (`internal/api/router.go`): `chi` router, CORS `Access-Control-Allow-Origin: *`.

### Domain types

All domain structs live in `internal/domain/`. `DomainType` string constants are defined in `internal/domain/types.go`. Key types: `WeatherCurrent`, `WeatherForecast`, `WeatherAlerts`, `TransitArrivals`, `TransitVehicles`, `TransitAlerts`, `TrafficIncidents`, `TrafficCameras`, `Aircraft`, `Astro`, `FlagStatus`, `Feeds`, `Notes`, `Home`, `WaterwayConditions`, `MunicipalEvents`, `MunicipalAlerts`.

`AlertSeverity` (`internal/domain/alert_severity.go`) is shared across weather, transit, and municipal alert types: `info`, `watch`, `warning`, `emergency`.

### Municipal and waterway collectors (generic frameworks)

- **Municipal** (`internal/collector/municipal/`): Generic HTTP-poll collector with a `Parser` interface. New municipalities add a parser sub-package, not a new collector type. Phase 2 adds IMAP ingestion per parser.
- **Waterway** (`internal/collector/waterway/`): Generic collector with a `GaugeSource` interface. Gauge auto-selection picks the N closest to site lat/lon when no explicit gauge list is configured. When `AdvisoryStatus` is `warning` or `emergency`, the waterway collector also emits into `municipal.alerts`.

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

See `config.example.yaml` (to be created). Key sections: `location` (lat/lon/timezone), `server` (port, data_dir), `tiles` (radius_km, cache_path), `collectors` (one block per collector type). Sensitive values (e.g. IMAP password) support `${ENV_VAR}` interpolation.

## API Endpoints

| Method | Path | Purpose |
|--------|------|---------|
| GET | `/api/v1/domains` | Active domain types on this instance |
| GET | `/api/v1/state/:domain` | Current state snapshot |
| GET | `/api/v1/stream` | SSE stream of domain + system events |
| POST | `/api/v1/subscriptions` | Register widget data needs |
| DELETE | `/api/v1/subscriptions/:widget_id` | Deregister widget |
| GET/POST/DELETE | `/api/v1/profiles` | Dashboard profile CRUD |
| GET/POST/PATCH/DELETE | `/api/v1/notes` | Shared notes |
| GET | `/tiles/{z}/{x}/{y}` | Protomaps tile serving |

## Implementation Phases

Refer to `docs/cupola-spec.md` §11 for the suggested 26-phase build order. In brief:

1. Domain types → 2. Backend skeleton (config, registry, state store, SSE, chi) → 3. Domain/state endpoints → 4. Subscription system → 5. Core collectors (`ephem`, `ecowitt`, `envcanada`) → 6. Profile API + SQLite → 7. Tiles → 8. Frontend skeleton → 9. Widget picker → 10. Core widgets → ... → 26. Kiosk mode

## Key Constraints

- Only one collector per `DomainType` — fatal error at startup if violated.
- `transit.arrivals` state is keyed by `"{agency}:{route}:{stop_id}"` — only subscribed combinations are populated.
- Space weather region (1–7) is auto-selected from `lat/lon` at startup; can be overridden in config.
- `municipal.events` is intentionally excluded from the alerts widget; it has its own `municipal-events` widget.
- Tile cache is permanent until manually deleted — delete `data/tiles/local.pmtiles` and restart to refresh.
