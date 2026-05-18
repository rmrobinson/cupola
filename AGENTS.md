# Cupola Agent Context

Cupola is a Go 1.26 single-binary ambient dashboard. The backend lives under `internal/`, the entry point is `cmd/cupola/main.go`, and the embedded vanilla JS/CSS frontend lives under `cmd/cupola/frontend/`.

## Commands

- Build: `make build`
- Go tests: `go test ./...`
- Frontend JS tests: `node --test test/frontend/*.test.js`
- Lint: `make lint`
- Frontend dependencies: `make vendor-frontend`

The frontend has no bundler or build step. Vendor libraries in `cmd/cupola/frontend/js/vendor/` are generated and gitignored.

## Backend Conventions

- Collectors implement `internal/collector.Collector` and publish normalized domain state through `store.StateStore`.
- Only one collector may own each `domain.DomainType`; duplicate registration is fatal.
- Any collector that depends on internet reachability must implement `SetNetCheck(func() bool)` and skip internet fetches while the checker reports down. `cmd/cupola/main.go` wires this automatically for collectors that expose the method. Local LAN collectors do not need to be gated by the internet checker.
- SQLite stores profiles, notes, GTFS schedule/cache data, and transit agency config. Do not use it as general sensor, alert, or time-series storage.
- Transit collectors are always registered so agencies can be managed dynamically through the API without restart.
- Parameterized transit arrivals are keyed as `{agency}:{route}:{stop_id}`.

## Frontend Conventions

- Widgets register into `window.CupolaWidgets`; modules are plain browser JS globals.
- Widget flow: initial state snapshot, subscription registration, SSE updates, cleanup on removal.
- Escape user/source text before interpolating HTML; many widgets use a local `esc()` helper.

## Current Status

- `config.example.yaml` exists and is the canonical sample config.
- Waterway collection is implemented and wired, including GRCA parser registration and optional promotion into `municipal.alerts`.
- IMAP dispatcher/handlers and the house camera collector are still planned/deferred.
- PWA files exist; verify offline behavior before treating PWA work as complete.
