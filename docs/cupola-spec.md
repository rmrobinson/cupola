# Cupola — System Specification

## 1. Purpose & Deployment Model

A self-hosted, location-aware dashboard providing ambient situational awareness for occupants of a building. Each instance is configured for a single physical location (home, office, etc.) and exposes only the data sources relevant to that site. Multiple independent instances can be deployed across locations.

A new instance requires internet access on first startup to retrieve map tile data; subsequent restarts are fully local-network capable.

---

## 2. Architecture

Decoupled frontend/backend. No shared codebase dependency between tiers.

### Backend

- **Language:** Go
- **Shape:** Single binary, configured via a YAML file per instance
- **Routing:** `net/http` with `chi` router
- **CORS:** `Access-Control-Allow-Origin: *`
- **API surface:**
  - `GET /api/v1/domains` — list of domains active on this instance
  - `GET /api/v1/state/:domain` — current state snapshot for a domain
  - `GET /api/v1/stream` — Server-Sent Events stream, pushes domain updates
  - `POST /api/v1/subscriptions` — register a widget's data needs
  - `DELETE /api/v1/subscriptions/:widget_id` — deregister a widget
  - `GET/POST/DELETE /api/v1/profiles` — dashboard profile CRUD
  - `GET /api/v1/notes` / `POST /api/v1/notes` / `PATCH /api/v1/notes/:id` / `DELETE /api/v1/notes/:id` — shared notes
  - `GET /tiles/{z}/{x}/{y}` — Protomaps tile serving from local `.pmtiles` file
- **Persistence:** SQLite, used only for profile storage and shared notes. No time-series or historical data retained.
- **Collector pattern:** Each data source implements a `Collector` interface. Collectors run independently, each polling or streaming on its own schedule, pushing normalized data into a central in-memory state store keyed by domain type. The REST API reads from this store; the SSE stream emits updates when state changes. Widgets bind to domain types, not collector IDs, so collector implementations can be swapped without frontend changes.

```go
type Collector interface {
    ID()     string       // e.g. "envcanada.forecast", "noaa.forecast"
    Domain() DomainType   // e.g. "weather.forecast"
    Start(ctx context.Context) error
    State() DomainState
}

type DomainState interface {
    DomainType() DomainType
    UpdatedAt()  time.Time
}
```

Collectors declared in config are registered at startup; unconfigured collectors are not loaded. Only one collector may be registered per domain type — enforced at startup with a fatal error.

### IMAP dispatcher

One IMAP collector per site owns the mailbox connection and polls for new messages. It does not produce domain state itself — instead it dispatches incoming emails to registered handler collectors based on sender address or subject regex patterns. Each handler collector registers its patterns at startup and receives matching emails for parsing into its own domain.

```go
type EmailHandler interface {
    ID()             string
    SenderPatterns() []string  // exact sender addresses to match
    SubjectPatterns() []string // regex patterns to match against subject
    Handle(msg EmailMessage) error
}
```

This means only one IMAP connection and credential is needed per site, regardless of how many email-based sources are configured.

### Frontend

- **Stack:** Vanilla HTML, JavaScript, CSS — no framework
- **Layout:** CSS Grid-based drag-and-drop widget canvas
- **Background:** Time-of-day CSS gradient (port of [horizon](https://github.com/dnlzro/horizon)), fixed behind the widget layer (`background-attachment: fixed`), persists on scroll
- **Targets:**
  - Desktop browser (primary)
  - Wall-mounted landscape screen (full-screen/kiosk mode, auto-loads a named profile)
  - Mobile PWA (responsive layout, portrait orientation)

---

## 3. Map Tile Setup (Protomaps)

On startup, the backend checks for a `.pmtiles` file in the configured data directory. If absent:

1. Derive a bounding box from the configured `lat`/`lon` plus `tiles_radius_km` (default: 50 km)
2. Fetch a tile extract from `api.protomaps.com` for that bounding box
3. Save the file to disk (e.g. `data/tiles/local.pmtiles`)
4. Serve tiles via the built-in HTTP handler at `GET /tiles/{z}/{x}/{y}`

Subsequent startups use the cached file and do not fetch from the network. To refresh tiles, delete the cached file and restart.

**Operational runbook — new location setup:**

```
1. Configure lat/lon and tiles_radius_km in config.yaml
2. Start the backend (tiles fetched automatically on first run)
3. Confirm tile file exists at data/tiles/local.pmtiles before deploying kiosk
```

---

## 4. Collector Inventory

| ID | Domain(s) | Source | Method | Default interval |
|---|---|---|---|---|
| `ecowitt.current` | `weather.current` | Ecowitt Wittboy (local GW2000 API) | Poll | 1 min |
| `envcanada.forecast` | `weather.forecast` | Environment Canada forecast API | Poll | 1 hr |
| `envcanada.hourly_forecast` | `weather.forecast.hourly` | Environment Canada hourly forecast page SSR JSON | Poll | 20 min |
| `envcanada.alerts` | `weather.alerts` | Environment Canada weather alerts | Poll | 15 min |
| `envcanada.solar` | `solar.weather.current`, `solar.weather.forecast` | Space Weather Canada RSS (region auto-selected by lat/lon) | Poll | 1 hr |
| `gtfsrt.<agency>` | `transit.arrivals`, `transit.vehicles`, `transit.alerts` | GTFS-RT | Poll | 30 s |
| `gtfs.<agency>` | *(static schedule, feeds gtfsrt)* | GTFS static feed | Refresh | 24 hr |
| `511.<province>` | `traffic.incidents`, `traffic.cameras` | 511 API — ON, AB, or BC | Poll | 2 min |
| `dump1090` | `aircraft` | dump1090 local HTTP JSON | Poll | 5 s |
| `house` | `home` | rmrobinson/house API | Poll / webhook | 30 s |
| `ephem` | `astro` | Computed locally (no network) | On schedule | Per event |
| `canada.flag` | `flag.status` | canada.ca half-masting page (HTML scrape) | Poll | 4 hr |
| `rss.<id>` | `feeds` | Configured RSS URLs | Poll | 15 min |
| `notes` | `notes` | Internal SQLite store | Read/write | Push via SSE |
| `imap` | *(dispatcher only)* | Configured IMAP mailbox | Poll | 2 min |
| `waterway.<id>` | `waterway.conditions` | Configured waterway source (e.g. GRCA) | Poll | 15 min |
| `municipal.<id>` | `municipal.events`, `municipal.alerts` | Configured municipal source (HTTP scrape or IMAP handler) | Poll / push | Varies |

### Municipal collector pattern

The municipal collector is a reusable framework. Each source is configured with a `parser` identifier that selects a location-specific implementation. New municipalities add a parser, not a new collector type.

**Phase 1 (HTTP scraping):** Poll a configured URL, run the named parser to extract events/alerts.

**Phase 2 (IMAP):** Register sender/subject patterns with the IMAP dispatcher; the parser handles matched emails.

Initial parser implementations:
- `kitchener.snow` — parses https://www.kitchener.ca/news/snow-events/ → `municipal.events`; IMAP phase 2
- `grca.flood` — parses https://www.grandriver.ca/news/categories/flood-messages/ → `municipal.alerts`; IMAP phase 2
- `enova.power` — parses https://oms.enovapower.com/Outages/ → `municipal.alerts`
- `kitchener.utilities` — parses https://www.kitchenerutilities.ca/en/outages-and-news.aspx → `municipal.alerts`

### Space weather region mapping

Space Weather Canada publishes 7 regional forecasts. The backend selects the correct region at startup based on configured lat/lon:

| Region | Coverage | Approx lat/lon bounds |
|---|---|---|
| 1 | BC Coast | lon < -120, lat 48–60 |
| 2 | BC Interior / Prairies West | lon -120 to -110 |
| 3 | Prairies East / MB | lon -110 to -95 |
| 4 | Ontario | lon -95 to -74 |
| 5 | Quebec | lon -74 to -60 |
| 6 | Atlantic | lon -60 to -52 |
| 7 | North | lat > 60 |

The RSS feed URL is parameterised by region number. Kitchener (lon ≈ -80.5) maps to region 4.

### Waterway gauge selection

The backend fetches all available gauges from the configured waterway source and exposes them via `waterway.conditions`. The widget can operate in two modes:
- **Auto:** selects the N closest gauges to the site lat/lon (default N=3)
- **Explicit:** configured with a list of gauge IDs in the widget config

### Collector unavailability

When a collector fails:
- Its widget(s) render in a **deactivated state**: greyed out, "source unavailable" label, timestamp of last successful update.
- A **system alert banner** slides down from the top of the frontend. Non-dismissable, auto-dismisses when the collector recovers.
- System alerts never appear in the alerts widget.

---

## 5. Domain Types & Schemas

### Domain type constants

```go
type DomainType string

const (
    DomainWeatherCurrent      DomainType = "weather.current"
    DomainWeatherForecast     DomainType = "weather.forecast"
    DomainWeatherForecastHourly DomainType = "weather.forecast.hourly"
    DomainWeatherAlerts       DomainType = "weather.alerts"
    DomainSolarWeatherCurrent DomainType = "solar.weather.current"
    DomainSolarWeatherForecast DomainType = "solar.weather.forecast"
    DomainTransitArrivals     DomainType = "transit.arrivals"
    DomainTransitVehicles     DomainType = "transit.vehicles"
    DomainTransitAlerts       DomainType = "transit.alerts"
    DomainTrafficIncidents    DomainType = "traffic.incidents"
    DomainTrafficCameras      DomainType = "traffic.cameras"
    DomainAircraft            DomainType = "aircraft"
    DomainAstro               DomainType = "astro"
    DomainFlagStatus          DomainType = "flag.status"
    DomainFeeds               DomainType = "feeds"
    DomainNotes               DomainType = "notes"
    DomainHome                DomainType = "home"
    DomainWaterwayConditions  DomainType = "waterway.conditions"
    DomainMunicipalEvents     DomainType = "municipal.events"
    DomainMunicipalAlerts     DomainType = "municipal.alerts"
)
```

### Shared types

```go
type AlertSeverity string

const (
    SeverityInfo      AlertSeverity = "info"
    SeverityWatch     AlertSeverity = "watch"
    SeverityWarning   AlertSeverity = "warning"
    SeverityEmergency AlertSeverity = "emergency"
)
```

### `weather.current`

```go
type WeatherCurrent struct {
    Temperature   float64   // °C
    FeelsLike     float64
    Humidity      float64   // %
    WindSpeed     float64   // km/h
    WindDirection int       // degrees
    WindGust      float64
    Pressure      float64   // hPa
    Precipitation float64   // mm in last hour
    UV            float64
    Visibility    float64   // km
    Condition     string    // clear, cloudy, rain, snow, fog, ...
    UpdatedAt     time.Time
}
```

*Implementations:* `ecowitt.current`, `openmeteo.current` (future fallback)

### `weather.forecast`

```go
type WeatherForecast struct {
    Periods   []ForecastPeriod
    UpdatedAt time.Time
}

type ForecastPeriod struct {
    StartsAt      time.Time
    EndsAt        time.Time
    Label         string    // "Tonight", "Wednesday", "Wednesday Night"
    High          *float64  // nil if night period
    Low           *float64  // nil if day period
    Condition     string
    PrecipChance  int       // %
    PrecipAmount  float64   // mm expected
    WindSpeed     float64
    WindDirection int
    Summary       string
}
```

*Implementations:* `envcanada.forecast`, `noaa.forecast` (future)

### `weather.forecast.hourly`

```go
type WeatherHourlyForecast struct {
    Hours     []HourlyForecastPeriod
    UpdatedAt time.Time
}

type HourlyForecastPeriod struct {
    StartsAt      time.Time
    EndsAt        time.Time
    Condition     string
    Temperature   *float64  // °C
    FeelsLike     *float64  // °C
    PrecipChance  *int      // %
    WindDirection string
    WindSpeed     *float64  // km/h
    WindGust      *float64  // km/h
    Humidex       *float64
    WindChill     *float64
    UVIndex       *float64
    IconURL       string
}
```

*Implementations:* `envcanada.hourly_forecast`

Environment Canada does not currently publish a public RSS/Atom endpoint for hourly data. The collector discovers the nearest station from configured lat/lon, fetches `weather.gc.ca/en/forecast/hourly/index.html?coords={stationLat},{stationLon}`, and parses `location.hourly` from the embedded Vue SSR state. Current pages may nest the hourly array under `location.location["{lat}--{lon}"].hourly`; the parser supports both shapes. `epochTime` is exposed as UTC `starts_at`; `ends_at` is `starts_at + 1h`. The public model intentionally omits provider-specific fields such as `iconCode`, station coordinates, source URL, location name, timezone, and source update timestamps.

### `weather.alerts`

```go
type WeatherAlerts struct {
    Alerts    []WeatherAlert
    UpdatedAt time.Time
}

type WeatherAlert struct {
    ID        string
    Title     string
    Severity  AlertSeverity
    Onset     time.Time
    Expires   time.Time
    Summary   string
    SourceURL string
}
```

*Implementations:* `envcanada.alerts`, `noaa.alerts` (future)

### `solar.weather.current`

```go
type SolarWeatherCurrent struct {
    KpIndex        float64   // 0–9 planetary geomagnetic index
    KpDescription  string    // quiet, unsettled, active, minor storm, major storm, ...
    FlareClass     *string   // nil if none recent, e.g. "M2.3"
    AuroraViewable bool      // derived from KpIndex + configured latitude
    Region         int       // 1–7, Space Weather Canada region
    UpdatedAt      time.Time
}
```

*Implementations:* `envcanada.solar`, `noaa.solar` (future)

### `solar.weather.forecast`

```go
type SolarWeatherForecast struct {
    Periods   []SolarForecastPeriod
    UpdatedAt time.Time
}

type SolarForecastPeriod struct {
    StartsAt       time.Time
    EndsAt         time.Time
    KpExpected     float64
    KpDescription  string
    AuroraViewable bool
    Summary        string
}
```

*Implementations:* `envcanada.solar`, `noaa.solar` (future)

### `transit.arrivals`

State is a map keyed by `"{agency}:{route}:{stop_id}"`. The backend only populates keys that have active widget subscriptions.

```go
type TransitArrivals struct {
    Stops     map[string]StopArrivals
    UpdatedAt time.Time
}

type StopArrivals struct {
    AgencyID  string
    RouteID   string
    RouteName string
    StopID    string
    StopName  string
    Arrivals  []Arrival
}

type Arrival struct {
    TripID    string
    Headsign  string
    Scheduled time.Time
    Predicted *time.Time
    Delay     *int       // seconds
    VehicleID *string
    Occupancy *string    // empty, low, medium, high
}
```

*Implementations:* `gtfsrt` (universal)

### `transit.vehicles`

```go
type TransitVehicles struct {
    Vehicles  []TransitVehicle
    UpdatedAt time.Time
}

type TransitVehicle struct {
    AgencyID  string
    VehicleID string
    RouteID   string
    RouteName string
    Lat       float64
    Lon       float64
    Bearing   *float64
    Speed     *float64
    UpdatedAt time.Time
}
```

### `transit.alerts`

```go
type TransitAlerts struct {
    Alerts    []TransitAlert
    UpdatedAt time.Time
}

type TransitAlert struct {
    ID             string
    AgencyID       string
    Title          string
    Description    string
    Severity       AlertSeverity
    AffectedRoutes []string
    StartsAt       *time.Time
    EndsAt         *time.Time
}
```

### `traffic.incidents`

```go
type TrafficIncidents struct {
    Incidents []TrafficIncident
    UpdatedAt time.Time
}

type TrafficIncident struct {
    ID          string
    Type        string  // collision, construction, hazard, congestion, closure
    Severity    string  // minor, moderate, major
    Lat         float64
    Lon         float64
    Description string
    RoadName    string
    StartsAt    *time.Time
    EndsAt      *time.Time
}
```

*Implementations:* `511on`, `511ab`, `511bc`

### `traffic.cameras`

```go
type TrafficCameras struct {
    Cameras   []TrafficCamera
    UpdatedAt time.Time
}

type TrafficCamera struct {
    ID          string
    Name        string
    Lat         float64
    Lon         float64
    SnapshotURL string
    UpdatedAt   time.Time
}
```

*Implementations:* `511on`, `511ab`, `511bc`

### `aircraft`

```go
type Aircraft struct {
    Aircraft  []AircraftTarget
    UpdatedAt time.Time
}

type AircraftTarget struct {
    ICAO      string
    Callsign  *string
    Flight    *string
    Lat       float64
    Lon       float64
    AltFt     int
    Track     *float64
    SpeedKts  *float64
    VertRate  *int      // ft/min, positive = climbing
    Squawk    *string
    OnGround  bool
    UpdatedAt time.Time
}
```

*Implementations:* `dump1090`

### `astro`

```go
type Astro struct {
    Sunrise          time.Time
    Sunset           time.Time
    SolarNoon        time.Time
    CivilDawn        time.Time
    CivilDusk        time.Time
    MoonPhase        float64    // 0.0–1.0; 0=new, 0.25=first quarter, 0.5=full, 0.75=last quarter
    MoonPhaseName    string     // new, waxing crescent, first quarter, waxing gibbous,
                               // full, waning gibbous, last quarter, waning crescent
    MoonIllumination float64    // 0.0–1.0, fraction of disc illuminated
    MoonRise         *time.Time
    MoonSet          *time.Time
    UpdatedAt        time.Time
}
```

*Implementations:* `ephem` (computed locally, no network)

### `flag.status`

```go
type FlagStatus struct {
    AtHalfMast bool
    Reason     *string
    Since      *time.Time
    Until      *time.Time
    SourceURL  string
    UpdatedAt  time.Time
}
```

*Implementations:* `canada.flag`, `usa.flag` (future)

### `feeds`

```go
type Feeds struct {
    Items     []FeedItem
    UpdatedAt time.Time
}

type FeedItem struct {
    ID          string
    FeedID      string
    Category    string    // municipal, emergency, news, ...
    Title       string
    Summary     string
    URL         string
    PublishedAt time.Time
}
```

*Implementations:* `rss` (universal)

### `notes`

```go
type Notes struct {
    Notes     []Note
    UpdatedAt time.Time
}

type Note struct {
    ID        string
    Title     string
    Body      string    // markdown
    Author    string
    Pinned    bool
    CreatedAt time.Time
    UpdatedAt time.Time
}
```

### `home`

```go
type Home struct {
    Cameras   []HomeCamera
    Sensors   []HomeSensor
    Presence  []PresenceEntry
    UpdatedAt time.Time
}

type HomeCamera struct {
    ID          string
    Name        string
    StreamURL   string
    SnapshotURL string
}

type HomeSensor struct {
    ID        string
    Name      string
    Type      string
    Value     interface{}
    Unit      *string
    UpdatedAt time.Time
}

type PresenceEntry struct {
    Name    string
    Present bool
    Since   time.Time
}
```

*Implementations:* `house`

### `waterway.conditions`

```go
type WaterwayConditions struct {
    Gauges    []WaterwayGauge
    UpdatedAt time.Time
}

type WaterwayGauge struct {
    ID            string
    Name          string
    WaterwayName  string    // e.g. "Grand River"
    Lat           float64
    Lon           float64
    LevelM        *float64  // water level in metres, nil if unavailable
    FlowCMS       *float64  // flow rate in cubic metres/second
    TempC         *float64  // water temperature, nil if unavailable
    AdvisoryStatus string   // none, advisory, watch, warning, emergency
    AdvisoryText  *string
    UpdatedAt     time.Time
}
```

*Implementations:* `grca` (Grand River Conservation Authority)

Alerts: when `AdvisoryStatus` is `warning` or `emergency`, the collector also emits an entry into `municipal.alerts`.

### `municipal.events`

Informational notices — scheduled or low-urgency. Not shown in the alerts widget by default.

```go
type MunicipalEvents struct {
    Events    []MunicipalEvent
    UpdatedAt time.Time
}

type MunicipalEvent struct {
    ID          string
    SourceID    string        // parser identifier, e.g. "kitchener.snow"
    Title       string
    Description string
    EventType   string        // snow-event, road-closure, maintenance, ...
    StartsAt    *time.Time
    EndsAt      *time.Time
    URL         *string
    PublishedAt time.Time
}
```

*Initial parsers:* `kitchener.snow` (HTTP scrape → IMAP phase 2)

### `municipal.alerts`

Urgent notices requiring attention. Shown in the alerts widget by default alongside `weather.alerts` and `transit.alerts`.

```go
type MunicipalAlerts struct {
    Alerts    []MunicipalAlert
    UpdatedAt time.Time
}

type MunicipalAlert struct {
    ID          string
    SourceID    string        // parser identifier, e.g. "grca.flood"
    Title       string
    Description string
    AlertType   string        // flood, power-outage, gas-outage, water-outage, snow-emergency, ...
    Severity    AlertSeverity
    Area        *string       // affected area description
    StartsAt    *time.Time
    EndsAt      *time.Time
    URL         *string
    PublishedAt time.Time
}
```

*Initial parsers:* `grca.flood` (HTTP scrape → IMAP phase 2), `enova.power` (HTTP scrape), `kitchener.utilities` (HTTP scrape)

---

## 6. API Design

### Domain registry

```
GET /api/v1/domains
```

Returns the list of domain types with active collectors on this instance. Used by the frontend widget picker to mark widgets as eligible or ineligible.

```json
{
  "domains": [
    "weather.current",
    "weather.forecast",
    "weather.forecast.hourly",
    "weather.alerts",
    "solar.weather.current",
    "solar.weather.forecast",
    "transit.arrivals",
    "transit.vehicles",
    "transit.alerts",
    "astro",
    "flag.status",
    "feeds",
    "notes",
    "waterway.conditions",
    "municipal.events",
    "municipal.alerts"
  ]
}
```

Domain presence means "a collector for this domain is configured at startup", not "the collector is currently healthy". Health is communicated via SSE system events.

### Widget subscriptions

```
POST   /api/v1/subscriptions
DELETE /api/v1/subscriptions/:widget_id
```

Registration payload:

```json
{
  "widget_id": "w1",
  "domain": "transit.arrivals",
  "params": {
    "agency": "grt",
    "route": "7",
    "stop_id": "1234"
  }
}
```

Domains that support params:

| Domain | Params |
|---|---|
| `transit.arrivals` | `agency`, `route`, `stop_id` |
| `transit.vehicles` | `agency` (optional filter) |
| `transit.alerts` | `agency` (optional filter) |
| `traffic.incidents` | `province` |
| `traffic.cameras` | `province` |
| `home` | `camera_id` (optional filter) |
| `waterway.conditions` | `gauge_ids` (optional list; omit for auto-select by location) |

All other domains are singletons.

The backend maintains a reference-counted subscription table per domain+params. When the last subscription for a combination drops to zero, the backend stops fetching it. On SSE disconnect, all subscriptions for that session are dropped.

### SSE stream

```
GET /api/v1/stream
```

Domain update event:

```json
{
  "domain": "weather.forecast",
  "ts": 1234567890,
  "data": { ... }
}
```

System health events:

```json
{
  "domain": "system",
  "ts": 1234567890,
  "collector_id": "envcanada.forecast",
  "status": "error",
  "message": "connection refused"
}
```

```json
{
  "domain": "system",
  "ts": 1234567890,
  "collector_id": "envcanada.forecast",
  "status": "ok"
}
```

### State snapshot

```
GET /api/v1/state/:domain
```

Current in-memory state for a domain. Used by widgets on initial load.

### Profiles

```
GET    /api/v1/profiles      →  [{ id, name, description, layout }]
GET    /api/v1/profiles/:id  →  full layout + widget config JSON
POST   /api/v1/profiles      →  create or update
DELETE /api/v1/profiles/:id  →  delete
```

Profile payload:

```json
{
  "id": "home-wall",
  "name": "Living Room Screen",
  "layout": "landscape",
  "widgets": [
    {
      "id": "w1",
      "type": "transit",
      "pos": { "col": 0, "row": 0, "w": 2, "h": 3 },
      "config": {
        "agency": "grt",
        "route": "7",
        "stop_id": "1234",
        "max_trips": 3
      }
    }
  ]
}
```

### Shared notes

```
GET    /api/v1/notes         →  [note]
POST   /api/v1/notes         →  create note
PATCH  /api/v1/notes/:id     →  update note
DELETE /api/v1/notes/:id     →  delete note
```

Changes pushed via SSE on the `notes` domain immediately after write.

---

## 7. Widget System

Each widget type is a JS module that exports:

```js
{
  type: "transit",
  domain: "transit.arrivals",
  defaultSize: { w: 2, h: 3 },
  configSchema: [ ... ],
  subscriptionParams(config),        // returns params object; null for singletons
  render(container, state, config),  // called on init with GET /state/:domain snapshot
  onUpdate(data, config),            // called on SSE update
}
```

On startup the frontend fetches `GET /api/v1/domains`. Widgets whose domain is absent are shown greyed out in the widget picker with a "not available on this instance" indicator and cannot be added.

**Widget lifecycle:**
1. Widget added → `GET /state/:domain` → `POST /subscriptions`
2. SSE updates → `onUpdate()`
3. Widget removed → `DELETE /subscriptions/:widget_id`

### Widget inventory

| Widget type | Domain(s) | Configurable fields |
|---|---|---|
| `clock` | `astro` | Timezone, 12/24h |
| `sunrise-sunset` | `astro` | — |
| `moon-phase` | `astro` | — |
| `weather-current` | `weather.current` | Units |
| `weather-forecast` | `weather.forecast` | Days to show |
| `weather-hourly-forecast` | `weather.forecast.hourly` | — |
| `alerts` | `weather.alerts`, `transit.alerts`, `municipal.alerts` | Source type filter (default: all) |
| `transit` | `transit.arrivals` | Agency, route, stop ID, max trips |
| `camera` | `home` or `traffic.cameras` | Camera ID, stream type, refresh interval |
| `flag-status` | `flag.status` | — |
| `radar-map` | `transit.vehicles`, `aircraft`, `traffic.incidents` | Centre, zoom, layers |
| `shared-notes` | `notes` | — |
| `solar-activity` | `solar.weather.current`, `solar.weather.forecast` | — |
| `waterway` | `waterway.conditions` | Gauge IDs (optional; omit for auto) |
| `municipal-events` | `municipal.events` | Source filter |

### Alerts widget

Shows items from `weather.alerts`, `transit.alerts`, and `municipal.alerts` by default. The widget config allows filtering by source type. Items are sorted by severity then recency. `municipal.events` is intentionally excluded from this widget — it has its own `municipal-events` widget.

### Waterway widget

Displays current level, flow rate, and advisory status per gauge. In auto mode, shows the closest N gauges to the site lat/lon. In explicit mode, shows the configured gauge IDs. When advisory status is `warning` or `emergency`, the gauge card is highlighted.

---

## 8. Frontend — Layout & UX

### Grid

- 12-column CSS Grid (landscape/desktop), 4-column (portrait/mobile)
- Drag-and-drop reposition and resize; layout saved to active profile on change
- Widget chrome: type icon, drag handle, config gear. Hidden in kiosk mode.

### Profile management & landing screen

On first load (or no active profile in `localStorage`):
1. **Use default layout** — detects viewport, loads bundled landscape or portrait default JSON
2. **Load a saved profile** — lists from `GET /api/v1/profiles`

Default layouts are bundled in the frontend. If the user selects a default, they are prompted to name and save it before the canvas opens.

### Widget picker

Fetches `GET /api/v1/domains` on open. Groups widgets:
- **Available** — domain present
- **Not available on this instance** — domain absent, greyed out, tooltip shows missing domain name

### Kiosk / wall-screen mode

URL: `?profile=<id>&kiosk=1` — auto-loads profile, suppresses chrome and landing screen.

### System alert banner

- Above the widget grid, outside layout
- Slides down on any collector error (SSE `domain: system, status: error`)
- Lists failing collectors by name + error
- Auto-dismisses when all recover
- Not user-dismissable

### Background gradient

Full-viewport horizon gradient behind the grid, transitions continuously based on local time and `astro` domain sunrise/sunset. Port of [dnlzro/horizon](https://github.com/dnlzro/horizon). `position: fixed`.

### PWA

`manifest.json` + service worker. Responsive single-column on mobile. Offline: last-known state with staleness timestamps.

---

## 9. Instance Configuration File

```yaml
location:
  name: "Home"
  lat: 43.45
  lon: -80.49
  timezone: "America/Toronto"

server:
  port: 8080
  data_dir: "./data"

tiles:
  radius_km: 50
  cache_path: "data/tiles/local.pmtiles"

collectors:
  weather_ecowitt:
    enabled: true
    url: "http://192.168.1.x"
    poll_interval: 60s

  weather_envcanada:
    enabled: true
    poll_interval_forecast: 1h
    poll_interval_hourly_forecast: 20m
    poll_interval_alerts: 15m

  solar_envcanada:
    enabled: true
    poll_interval: 1h
    # region auto-selected from lat/lon; override with:
    # region: 4

  transit:
    rt_poll_interval: 30s
    static_refresh_interval: 24h

  traffic_511:
    enabled: true
    provinces: [ON]

  aircraft_dump1090:
    enabled: true
    url: "http://localhost:8080"
    poll_interval: 5s

  house:
    enabled: true
    url: "http://localhost:9000"
    poll_interval: 30s

  rss_feeds:
    - id: region_of_waterloo
      url: "https://..."
      category: municipal

  flag_canada:
    enabled: true
    poll_interval: 4h

  waterways:
    - id: grand_river
      parser: grca
      url: "https://www.grandriver.ca/our-watershed/river-data/"
      poll_interval: 15m
      alert_on: [warning, emergency]

  municipal:
    - id: kitchener_snow
      parser: kitchener.snow
      url: "https://www.kitchener.ca/news/snow-events/"
      poll_interval: 30m
      domain: municipal.events
    - id: grca_flood
      parser: grca.flood
      url: "https://www.grandriver.ca/news/categories/flood-messages/"
      poll_interval: 15m
      domain: municipal.alerts
    - id: enova_power
      parser: enova.power
      url: "https://oms.enovapower.com/Outages/"
      poll_interval: 10m
      domain: municipal.alerts
    - id: kitchener_utilities
      parser: kitchener.utilities
      url: "https://www.kitchenerutilities.ca/en/outages-and-news.aspx"
      poll_interval: 10m
      domain: municipal.alerts

  imap:
    enabled: false   # enable in phase 2
    host: "imap.example.com"
    port: 993
    username: "alerts@example.com"
    password: "${IMAP_PASSWORD}"
    poll_interval: 2m
    # handlers registered automatically by enabled municipal collectors
    # that declare imap support
```

---

## 10. Repository Structure (suggested)

```
cupola/
├── cmd/
│   └── cupola/
│       └── main.go
├── internal/
│   ├── domain/
│   │   ├── types.go
│   │   ├── weather.go
│   │   ├── solar.go
│   │   ├── transit.go
│   │   ├── traffic.go
│   │   ├── aircraft.go
│   │   ├── astro.go
│   │   ├── flag.go
│   │   ├── feeds.go
│   │   ├── notes.go
│   │   ├── home.go
│   │   ├── waterway.go
│   │   ├── municipal.go
│   │   └── alert_severity.go
│   ├── collector/
│   │   ├── collector.go          # Collector + EmailHandler interfaces, registry
│   │   ├── ecowitt/
│   │   ├── envcanada/
│   │   ├── solar/
│   │   ├── gtfs/
│   │   ├── gtfsrt/
│   │   ├── traffic511/
│   │   ├── dump1090/
│   │   ├── house/
│   │   ├── astro/
│   │   ├── flag/
│   │   ├── rss/
│   │   ├── notes/
│   │   ├── imap/                 # shared dispatcher
│   │   ├── waterway/
│   │   │   ├── waterway.go       # generic collector + GaugeSource interface
│   │   │   └── grca/             # GRCA parser implementation
│   │   └── municipal/
│   │       ├── municipal.go      # generic collector + Parser interface
│   │       ├── kitchener_snow/
│   │       ├── grca_flood/
│   │       ├── enova_power/
│   │       └── kitchener_utilities/
│   ├── api/
│   │   ├── router.go
│   │   ├── domains.go
│   │   ├── state.go
│   │   ├── subscriptions.go
│   │   ├── profiles.go
│   │   ├── notes.go
│   │   ├── stream.go
│   │   └── tiles.go
│   ├── store/
│   │   └── sqlite.go
│   ├── tiles/
│   │   └── pmtiles.go
│   └── config/
│       └── config.go
├── frontend/
│   ├── index.html
│   ├── css/
│   │   ├── main.css
│   │   ├── grid.css
│   │   └── horizon.css
│   ├── js/
│   │   ├── main.js
│   │   ├── grid.js
│   │   ├── stream.js
│   │   ├── profile.js
│   │   ├── subscriptions.js
│   │   ├── widget-picker.js
│   │   └── widgets/
│   │       ├── clock.js
│   │       ├── transit.js
│   │       ├── weather-current.js
│   │       ├── weather-forecast.js
│   │       ├── weather-hourly-forecast.js
│   │       ├── alerts.js
│   │       ├── camera.js
│   │       ├── radar-map.js
│   │       ├── moon-phase.js
│   │       ├── sunrise-sunset.js
│   │       ├── flag-status.js
│   │       ├── solar-activity.js
│   │       ├── shared-notes.js
│   │       ├── waterway.js
│   │       └── municipal-events.js
│   └── manifest.json
├── config.example.yaml
└── README.md
```

---

## 11. Implementation Phases (suggested order for Claude Code)

1. **Domain types** — all structs in `internal/domain/`, `DomainState` interface, `AlertSeverity`
2. **Backend skeleton** — config loading, collector registry, in-memory state store, SSE stream, chi router
3. **Domain registry + state endpoints** — `GET /api/v1/domains`, `GET /api/v1/state/:domain`
4. **Subscription system** — `POST/DELETE /api/v1/subscriptions`, reference counting, session cleanup
5. **Core collectors** — `ephem`, `ecowitt.current`, `envcanada.forecast`, `envcanada.alerts`
6. **Profile API + SQLite store**
7. **Tiles** — pmtiles fetch-on-startup, cache, HTTP handler
8. **Frontend skeleton** — grid, horizon gradient, SSE client, landing screen, profile load/save, system alert banner
9. **Widget picker** — domain eligibility, available/unavailable grouping
10. **Core widgets** — `clock`, `weather-current`, `weather-forecast`, `moon-phase`, `sunrise-sunset`
11. **Transit collectors + widget** — GTFS static + GTFS-RT, transit arrival widget
12. **Map widget** — Leaflet + Protomaps, SSE-driven marker updates
13. **511 collector** — incidents + camera widget
14. **dump1090 collector** — aircraft map layer
15. **house collector** — camera widget (WebRTC/go2rtc)
16. **Solar weather collector** — region mapping, RSS parsing, `solar.weather.current` + forecast, solar-activity widget
17. **Waterway collector** — GRCA parser, gauge auto-selection, waterway widget, alert promotion to `municipal.alerts`
18. **Municipal collector framework** — generic HTTP scrape collector + Parser interface
19. **Municipal parsers** — `kitchener.snow`, `grca.flood`, `enova.power`, `kitchener.utilities`
20. **Alerts widget** — aggregates `weather.alerts`, `transit.alerts`, `municipal.alerts`; source filter
21. **Municipal events widget**
22. **RSS + flag + notes collectors**
23. **IMAP dispatcher** — shared inbox, EmailHandler interface, handler registration
24. **IMAP handlers** — upgrade `kitchener.snow` and `grca.flood` to support email ingestion
25. **PWA** — manifest, service worker, offline state
26. **Kiosk mode** — URL param handling, chrome suppression
