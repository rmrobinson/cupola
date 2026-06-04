# Region of Waterloo Data Sources

This document describes the external data sources used by Cupola for the
Region of Waterloo / Grand River watershed area, how each was discovered,
and which collector/parser consumes it.

---

## Grand River Conservation Authority (GRCA) — Waterway Gauges

**Collector:** `waterway` / parser `grca`
**Source file:** `internal/collector/waterway/grca/parser.go`

### How it was found

The GRCA publishes live river and reservoir data at
<https://www.grandriver.ca/our-watershed/river-data/>. That page embeds
charts powered by **KiWIS** (Kisters Water Information System). The
underlying JSON endpoints were identified by inspecting the network
requests made by the chart widgets:

| Endpoint | Contents |
|---|---|
| `https://apps.grandriver.ca/waterdata/kiwischarts/wiskiData/RF_CurrentValue/RF_CurrentValue.json` | Current flow / level readings for all river-monitoring stations |
| `https://apps.grandriver.ca/waterdata/kiwischarts/wiskiData/LS_ResSummary/LS_ResSummary.json` | Current elevation and discharge for all managed reservoirs |

Each JSON array entry carries a `ts_id` (time-series ID). The static
station table in the parser maps each station to three `ts_id` values:
time, level, and flow.

### Flow-monitoring stations

| ID | Name | Waterway | Lat | Lon |
|---|---|---|---|---|
| `grca_dundalk_wsc` | Dundalk WSC | Grand River | 44.105 | −80.368 |
| `grca_riverview_keldon` | Riverview (Keldon) | Grand River | 43.917 | −80.348 |
| `grca_legatt` | Legatt | Grand River | 43.832 | −80.366 |
| `grca_waldemar` | Waldemar | Grand River | 43.799 | −80.395 |
| `grca_marsville_wsc` | Marsville WSC | Grand River | 43.766 | −80.399 |
| `grca_below_shand_dam_wsc` | Below Shand Dam WSC | Grand River | 43.699 | −80.347 |
| `grca_west_montrose_wsc` | West Montrose WSC | Grand River | 43.596 | −80.447 |
| `grca_bridgeport` | Bridgeport | Grand River | 43.469 | −80.536 |
| `grca_hidden_valley_wsc` | Grand River at Hidden Valley WSC | Grand River | 43.428 | −80.460 |
| `grca_doon` | Doon | Grand River | 43.392 | −80.420 |
| `grca_galt_wsc` | Galt WSC | Grand River | 43.355 | −80.311 |
| `grca_brantford_wsc` | Brantford WSC | Grand River | 43.134 | −80.265 |
| `grca_york` | York | Grand River | 43.022 | −80.207 |
| `grca_dunnville` | Dunnville above Dunnville Dam | Grand River | 42.906 | −79.623 |
| `grca_salem_wsc` | Salem WSC | Irvine River | 43.785 | −80.597 |
| `grca_floradale` | Floradale | Canagagigue Creek | 43.647 | −80.649 |
| `grca_elmira_arthur_st` | Elmira at Arthur St. | Canagagigue Creek | 43.600 | −80.558 |
| `grca_below_elmira_wsc` | Below Elmira WSC | Canagagigue Creek | 43.584 | −80.569 |
| `grca_drayton` | Drayton | Conestogo River | 43.768 | −80.694 |
| `grca_moorefield` | Moorefield | Conestogo River | 43.754 | −80.628 |
| `grca_glen_allan_wsc` | Glen Allan WSC | Conestogo River | 43.682 | −80.590 |
| `grca_st_jacobs_wsc` | St. Jacobs WSC | Conestogo River | 43.558 | −80.557 |
| `grca_erbsville` | Erbsville | Laurel Creek | 43.499 | −80.620 |
| `grca_laurel_creek_weber_wsc` | Laurel Creek at Weber St. WSC | Laurel Creek | 43.468 | −80.537 |
| `grca_schneider_ottawa_st` | Schneider Creek at Ottawa St. | Schneider Creek | 43.429 | −80.476 |
| `grca_armstrong_mills_wsc` | Armstrong Mills WSC | Speed River | 43.833 | −80.376 |
| `grca_victoria` | Victoria | Speed River | 43.714 | −80.308 |
| `grca_eramosa_watson_rd_wsc` | Eramosa River at Watson Rd. WSC | Eramosa River | 43.634 | −80.252 |
| `grca_speed_edinburgh_wsc` | Speed River at Edinburgh Rd. WSC | Speed River | 43.558 | −80.213 |
| `grca_speed_road32` | Speed River Road 32 Below Guelph | Speed River | 43.490 | −80.225 |
| `grca_speed_beaverdale_wsc` | Speed River at Beaverdale Rd. WSC | Speed River | 43.415 | −80.257 |
| `grca_mill_creek_sr10` | Mill Creek at Side Road 10 | Mill Creek | 43.497 | −80.332 |
| `grca_nithburg_wsc` | Nithburg WSC | Nith River | 43.875 | −80.653 |
| `grca_philipsburg` | Philipsburg | Nith River | 43.707 | −80.553 |
| `grca_new_hamburg_wsc` | New Hamburg WSC | Nith River | 43.382 | −80.702 |
| `grca_ayr` | Ayr | Nith River | 43.282 | −80.461 |
| `grca_canning_wsc` | Canning WSC | Nith River | 43.198 | −80.375 |
| `grca_whitemans_mt_vernon_wsc` | Whitemans Creek at Mt. Vernon WSC | Whitemans Creek | 43.136 | −80.431 |
| `grca_fairchild_brantford_wsc` | Fairchild near Brantford WSC | Fairchild Creek | 43.133 | −80.268 |
| `grca_mckenzie_caledonia_wsc` | McKenzie Creek near Caledonia WSC | McKenzie Creek | 43.059 | −79.952 |

### Managed reservoirs

| ID | Name | Waterway | Lat | Lon | Notes |
|---|---|---|---|---|---|
| `grca_res_shand` | Shand Reservoir | Grand River | 43.766 | −80.369 | Level in m MASL |
| `grca_res_conestogo` | Conestogo Lake | Conestogo River | 43.737 | −80.505 | Level in m MASL |
| `grca_res_guelph` | Guelph Lake | Speed River | 43.582 | −80.214 | Level in m MASL |
| `grca_res_luther` | Luther Lake | Grand River | 43.979 | −80.400 | Level in m MASL |
| `grca_res_woolwich` | Woolwich Reservoir | Canagagigue Creek | 43.571 | −80.490 | Level in m MASL |
| `grca_res_laurel` | Laurel Creek Reservoir | Laurel Creek | 43.484 | −80.574 | Level in m MASL |
| `grca_res_shades` | Shades Mills Reservoir | Mill Creek | 43.420 | −80.311 | Level in m MASL |

---

## GRCA Flood Messages

**Collector:** `municipal` (alerts) / parser `grca.flood`
**Source file:** `internal/collector/municipal/grcaflood/parser.go`

### How it was found

The GRCA publishes flood-watch, flood-warning, and flood-emergency
statements as news items at
<https://www.grandriver.ca/news/categories/flood-messages/>. The page
uses the standard GRCA GovDelivery-powered news feed (CSS class
`gs-feed-list-item`) — the same markup structure used by the Kitchener
snow-events feed. Severity (`info` / `watch` / `warning` / `emergency`)
is inferred from the presence of those keywords in the item title.

**Config URL:** `https://www.grandriver.ca/news/categories/flood-messages/`

---

## Enova Power — Outage Map

**Collector:** `municipal` (alerts) / parser `enova.power`
**Source file:** `internal/collector/municipal/enovapower/parser.go`

### How it was found

Enova Power serves Waterloo Region under the Cambridge and North Dumfries
franchise areas. Their public outage map at
<https://oms.enovapower.com/Outages/> loads pushpin data from a
non-authenticated XML endpoint discovered via browser network inspection:

**API endpoint:** `POST https://oms.enovapower.com/Outages/Home/UpdatePushpin`

The response is an XML document with `<OMSCASES>` elements, each
containing location coordinates, customer counts, cause code, public
message, and an estimated restore time. Outage polygon boundaries are
provided as a flat `lat,lon,...` string in `<COORDLIST>`.

Area names are resolved via **Nominatim** reverse-geocoding
(`nominatim.openstreetmap.org`) with a 24-hour cache to respect usage
limits.

**Config URL:** `https://oms.enovapower.com/Outages/`
(the parser appends `/Home/UpdatePushpin` automatically)

---

## City of Kitchener — Snow Events

**Collector:** `municipal` (alerts) / parser `kitchener.snow`
**Source file:** `internal/collector/municipal/kitchenersnow/parser.go`

### How it was found

The City of Kitchener publishes snow-clearing event notices at
<https://www.kitchener.ca/news/snow-events/>. The page uses the same
GovDelivery news-feed markup (`gs-feed-list-item`, `gs-feed-list-title`,
`gs-feed-list-date`, `gs-feed-list-description`) as the GRCA flood page,
so the parsers share the same HTML extraction pattern.

The parser synthesizes current state from declare, extend, and cancel
posts. It emits a `municipal.alerts` warning only while a snow event is
currently active; inactive or expired snow events emit no alert.

**Config URL:** `https://www.kitchener.ca/news/snow-events/`

---

## City of Kitchener — Road Closures

**Collector:** `municipal` (alerts) / parser `kitchener.roadclosures`
**Source file:** `internal/collector/municipal/kitchenerroadclosures/parser.go`

### How it was found

The City of Kitchener publishes current road closures through an
ASP.NET list endpoint at
<https://app2.kitchener.ca/roadclosures/list.asp>. The public landing
page is <https://www.kitchener.ca/roadclosures>.

The parser reads the closure tables and emits `municipal.alerts` for
emergency closures and special-event closures. Special-event closures are
informational alerts. The source does not expose reliable coordinates in
the table, so the parser does not infer map geometry.

**Config URL:** `https://app2.kitchener.ca/roadclosures/list.asp`

---

## Kitchener Utilities — Water Service Disruptions

**Collector:** `municipal` (alerts) / parser `kitchener.utilities`
**Source file:** `internal/collector/municipal/kitchenerutilities/parser.go`

### How it was found

Kitchener Utilities (water and natural-gas distribution) exposes an
ASP.NET-based disruption status page at
<https://app2.kitchener.ca/utilities/Default.aspx?wmode=transparent>.
When disruptions are active the page renders `<div class="bs-example">`
blocks with `<dt>`/`<dd>` field pairs (Location, Between what streets,
Status, Posted on). When nothing is active the element `#lblList`
contains the text "nothing to report".

**Config URL:** `https://app2.kitchener.ca/utilities/Default.aspx?wmode=transparent`
