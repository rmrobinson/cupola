# Collector Network Requirements

This document lists each collector and whether it requires access to the public internet, a local LAN device/file, or no network at all.

## No Network Required

| Collector | Package | What it does |
|-----------|---------|--------------|
| `astro` | `internal/collector/astro` | Computes sunrise/sunset, moon phase, and solar noon locally from lat/lon/timezone. No HTTP calls. |
| `notes` | `internal/collector/notes` | Reads and writes shared notes from a local SQLite database. No HTTP calls. |
| `wastecollection` | `internal/collector/wastecollection` | Reads a local JSON schedule file to determine which waste types are collected each week. No HTTP calls. |

## Local LAN Only

These collectors contact devices or services on the local network. They do not require a route to the public internet.

| Collector | Package | Endpoint |
|-----------|---------|----------|
| `dump1090` | `internal/collector/dump1090` | Polls a local ADS-B receiver (dump1090 / readsb) at `http://{baseURL}/data/aircraft.json`. The host is a LAN device configured in `config.yaml`. |
| `ecowitt` | `internal/collector/ecowitt` | Polls a local Ecowitt GW2000 weather station at `http://{baseURL}/get_livedata_info`. The host is a LAN device configured in `config.yaml`. |

## Public Internet Required

These collectors make outbound HTTPS (or HTTP) requests to external services. They will not function without internet access.

| Collector | Package | External services |
|-----------|---------|-------------------|
| `envcanada.forecast` | `internal/collector/envcanada` | `weather.gc.ca` forecast Atom feeds. |
| `envcanada.hourly_forecast` | `internal/collector/envcanada` | `weather.gc.ca` hourly forecast HTML pages. No RSS endpoint has been found for hourly data, so the collector parses embedded SSR JSON from the public hourly page. |
| `envcanada.alerts` | `internal/collector/envcanada` | `weather.gc.ca` alert Atom feeds. |
| `envcanada.solar` | `internal/collector/envcanada` | `services.swpc.noaa.gov` NOAA planetary K-index for solar/aurora data. |
| `flag` | `internal/collector/flag` | `canada.ca` — scrapes the Canadian Heritage half-masting notices page. |
| `gtfs` | `internal/collector/gtfs` | Transit agency GTFS static ZIP feeds. URLs are configured per agency in SQLite through the GTFS Feeds admin/API. |
| `gtfsrt` | `internal/collector/gtfsrt` | Transit agency GTFS-RT protobuf feeds (trip updates, vehicle positions, alerts). URLs are configured per agency in SQLite through the GTFS Feeds admin/API. |
| `municipal` | `internal/collector/municipal` | Agency-specific URLs determined by the registered parser. Configured in `config.yaml`. |
| `rss` | `internal/collector/rss` | Any number of RSS/Atom feed URLs. Fully configurable in `config.yaml`. |
| `traffic511` | `internal/collector/traffic511` | `511on.ca` — incidents, cameras, and road conditions APIs. URLs are hardcoded to the Ontario 511 service. |
| `waterway` | `internal/collector/waterway` | Gauge data provider URLs determined by the registered source implementation. Configured in `config.yaml`. |

## Connectivity checker

`internal/connectivity` runs an independent background probe (default: `http://connectivitycheck.gstatic.com/generate_204` every 30 s). When the probe fails, all collectors in the **Public Internet Required** table skip their fetch loop silently rather than logging repeated connection errors. On recovery they resume immediately at their next tick. The `VehiclesCollector` additionally clears its state when internet goes down, since vehicle positions go stale within seconds.

The probe URL and interval are configurable via `connectivity.check_url` and `connectivity.interval` in `config.yaml`.

## Notes

- The `imap` dispatcher (Phase 23/24) is not yet implemented. When it is, it will require outbound access to an IMAP server, which may be local or internet-hosted depending on the mail provider. It will also be gated by the connectivity checker.
- Collectors not listed in `config.yaml` are not loaded at startup and impose no network requirements.
- For collectors with configurable URLs (`gtfs`, `gtfsrt`, `municipal`, `rss`, `waterway`), network requirements depend entirely on where those URLs resolve.
- The `traffic511` collector includes `kitchener.roadclosures` as a sub-source alongside `511on.ca`; both are gated at the `IncidentsCollector` level.
- Weather widgets that render provider icons require the icon origin to be allowed by `server.csp_img_src`. Environment Canada hourly icons use `https://weather.gc.ca`; future providers should either emit CSP-allowed `icon_url` values or omit `icon_url`.
