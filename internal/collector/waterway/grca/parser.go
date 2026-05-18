// Package grca implements a waterway.GaugeSource for the Grand River
// Conservation Authority. Data is fetched from the KiWIS hydrological API
// used by https://www.grandriver.ca/our-watershed/river-data/.
//
// Register via import side-effect:
//
//	_ "github.com/rmrobinson/cupola/internal/collector/waterway/grca"
package grca

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/rmrobinson/cupola/internal/collector/waterway"
	"github.com/rmrobinson/cupola/internal/domain"
)

func init() {
	waterway.RegisterGaugeSource("grca", func() waterway.GaugeSource {
		return &Parser{client: &http.Client{Timeout: 20 * time.Second}}
	})
}

const (
	flowURL              = "https://apps.grandriver.ca/waterdata/kiwischarts/wiskiData/RF_CurrentValue/RF_CurrentValue.json"
	reservoirURL         = "https://apps.grandriver.ca/waterdata/kiwischarts/wiskiData/LS_ResSummary/LS_ResSummary.json"
	flowSummaryURL       = "https://www.grandriver.ca/our-watershed/river-data/river-and-stream-flows/flow-summary/"
	upperGrandURL        = "https://www.grandriver.ca/our-watershed/river-data/river-and-stream-flows/upper-grand-flows/"
	centralGrandURL      = "https://www.grandriver.ca/our-watershed/river-data/river-and-stream-flows/central-grand-flows/"
	centralLowerGrandURL = "https://www.grandriver.ca/our-watershed/river-data/river-and-stream-flows/central-lower-grand-flows/"
	lowerGrandURL        = "https://www.grandriver.ca/our-watershed/river-data/river-and-stream-flows/lower-grand-flows/"
	conestogoRiverURL    = "https://www.grandriver.ca/our-watershed/river-data/river-and-stream-flows/conestogo-river-flows/"
	nithRiverURL         = "https://www.grandriver.ca/our-watershed/river-data/river-and-stream-flows/nith-river-flows/"
	speedEramosaURL      = "https://www.grandriver.ca/our-watershed/river-data/river-and-stream-flows/speed-and-eramosa-flows/"
	canagagigueCreekURL  = "https://www.grandriver.ca/our-watershed/river-data/river-and-stream-flows/canagagigue-creek-flows/"
	laurelCreekURL       = "https://www.grandriver.ca/our-watershed/river-data/river-and-stream-flows/laurel-creek-flows/"
	mckenzieCreekURL     = "https://www.grandriver.ca/our-watershed/river-data/river-and-stream-flows/mckenzie-creek-flows/"
	millCreekURL         = "https://www.grandriver.ca/our-watershed/river-data/river-and-stream-flows/mill-creek-flows/"
	whitemansCreekURL    = "https://www.grandriver.ca/our-watershed/river-data/river-and-stream-flows/whitemans-creek-flows/"
	conestogoDamURL      = "https://www.grandriver.ca/our-watershed/river-data/reservoir-levels/conestogo-dam/"
	guelphDamURL         = "https://www.grandriver.ca/our-watershed/river-data/reservoir-levels/guelph-dam/"
	laurelDamURL         = "https://www.grandriver.ca/our-watershed/river-data/reservoir-levels/laurel-dam/"
	lutherDamURL         = "https://www.grandriver.ca/our-watershed/river-data/reservoir-levels/luther-dam/"
	shadesMillsDamURL    = "https://www.grandriver.ca/our-watershed/river-data/reservoir-levels/shades-mills-dam/"
	shandDamURL          = "https://www.grandriver.ca/our-watershed/river-data/reservoir-levels/shand-dam/"
	woolwichDamURL       = "https://www.grandriver.ca/our-watershed/river-data/reservoir-levels/woolwich-dam/"
)

var sourceURLsByGaugeID = map[string]string{
	"grca_dundalk_wsc":             upperGrandURL,
	"grca_riverview_keldon":        upperGrandURL,
	"grca_legatt":                  upperGrandURL,
	"grca_waldemar":                upperGrandURL,
	"grca_marsville_wsc":           upperGrandURL,
	"grca_below_shand_dam_wsc":     upperGrandURL,
	"grca_west_montrose_wsc":       upperGrandURL,
	"grca_salem_wsc":               upperGrandURL,
	"grca_bridgeport":              centralGrandURL,
	"grca_hidden_valley_wsc":       centralGrandURL,
	"grca_doon":                    centralGrandURL,
	"grca_galt_wsc":                centralGrandURL,
	"grca_brantford_wsc":           centralLowerGrandURL,
	"grca_york":                    centralLowerGrandURL,
	"grca_dunnville":               lowerGrandURL,
	"grca_floradale":               canagagigueCreekURL,
	"grca_elmira_arthur_st":        canagagigueCreekURL,
	"grca_below_elmira_wsc":        canagagigueCreekURL,
	"grca_drayton":                 conestogoRiverURL,
	"grca_moorefield":              conestogoRiverURL,
	"grca_glen_allan_wsc":          conestogoRiverURL,
	"grca_st_jacobs_wsc":           conestogoRiverURL,
	"grca_erbsville":               laurelCreekURL,
	"grca_laurel_creek_weber_wsc":  laurelCreekURL,
	"grca_speed_edinburgh_wsc":     speedEramosaURL,
	"grca_armstrong_mills_wsc":     speedEramosaURL,
	"grca_victoria":                speedEramosaURL,
	"grca_eramosa_watson_rd_wsc":   speedEramosaURL,
	"grca_speed_road32":            speedEramosaURL,
	"grca_speed_beaverdale_wsc":    speedEramosaURL,
	"grca_mill_creek_sr10":         millCreekURL,
	"grca_nithburg_wsc":            nithRiverURL,
	"grca_philipsburg":             nithRiverURL,
	"grca_new_hamburg_wsc":         nithRiverURL,
	"grca_ayr":                     nithRiverURL,
	"grca_canning_wsc":             nithRiverURL,
	"grca_whitemans_mt_vernon_wsc": whitemansCreekURL,
	"grca_mckenzie_caledonia_wsc":  mckenzieCreekURL,
	"grca_res_shand":               shandDamURL,
	"grca_res_conestogo":           conestogoDamURL,
	"grca_res_guelph":              guelphDamURL,
	"grca_res_luther":              lutherDamURL,
	"grca_res_woolwich":            woolwichDamURL,
	"grca_res_laurel":              laurelDamURL,
	"grca_res_shades":              shadesMillsDamURL,
}

// stationMeta describes a flow-monitoring station or reservoir.
// levelTSID and flowTSID reference ts_id values in the KiWIS JSON.
// For reservoirs levelTSID is the elevation in m MASL and flowTSID is discharge.
type stationMeta struct {
	id           string
	name         string
	waterwayName string
	lat, lon     float64
	timeTSID     string
	levelTSID    string
	flowTSID     string
}

// flowStations is the complete static table parsed from the GRCA flow-summary page.
var flowStations = []stationMeta{
	// Grand River
	{"grca_dundalk_wsc", "Dundalk WSC", "Grand River", 44.105, -80.368, "8749042", "12242042", "8749042"},
	{"grca_riverview_keldon", "Riverview (Keldon)", "Grand River", 43.917, -80.348, "8767042", "10123042", "8767042"},
	{"grca_legatt", "Legatt", "Grand River", 43.832, -80.366, "8755042", "10124042", "8755042"},
	{"grca_waldemar", "Waldemar", "Grand River", 43.799, -80.395, "8719042", "10125042", "8719042"},
	{"grca_marsville_wsc", "Marsville WSC", "Grand River", 43.766, -80.399, "8761042", "10060042", "8761042"},
	{"grca_below_shand_dam_wsc", "Below Shand Dam WSC", "Grand River", 43.699, -80.347, "8743042", "12241042", "8743042"},
	{"grca_west_montrose_wsc", "West Montrose WSC", "Grand River", 43.596, -80.447, "8725042", "10068042", "8725042"},
	{"grca_bridgeport", "Bridgeport", "Grand River", 43.469, -80.536, "8665042", "12237042", "8665042"},
	{"grca_hidden_valley_wsc", "Grand River at Hidden Valley WSC", "Grand River", 43.428, -80.460, "8695042", "10073042", "8695042"},
	{"grca_doon", "Doon", "Grand River", 43.392, -80.420, "8677042", "12238042", "8677042"},
	{"grca_galt_wsc", "Galt WSC", "Grand River", 43.355, -80.311, "8671042", "10104042", "8671042"},
	{"grca_brantford_wsc", "Brantford WSC", "Grand River", 43.134, -80.265, "24117042", "12235042", "24117042"},
	{"grca_york", "York", "Grand River", 43.022, -80.207, "8731042", "10105042", "8731042"},
	{"grca_dunnville", "Dunnville above Dunnville Dam", "Grand River", 42.906, -79.623, "8683042", "12239042", "8683042"},
	// Irvine River
	{"grca_salem_wsc", "Salem WSC", "Irvine River", 43.785, -80.597, "8773042", "10106042", "8773042"},
	// Canagagigue Creek
	{"grca_floradale", "Floradale", "Canagagigue Creek", 43.647, -80.649, "9808042", "10140042", "9808042"},
	{"grca_elmira_arthur_st", "Elmira at Arthur St.", "Canagagigue Creek", 43.600, -80.558, "8593042", "12226042", "8593042"},
	{"grca_below_elmira_wsc", "Below Elmira WSC", "Canagagigue Creek", 43.584, -80.569, "8599042", "12227042", "8599042"},
	// Conestogo River
	{"grca_drayton", "Drayton", "Conestogo River", 43.768, -80.694, "8629042", "12231042", "8629042"},
	{"grca_moorefield", "Moorefield", "Conestogo River", 43.754, -80.628, "8803042", "10116042", "8803042"},
	{"grca_glen_allan_wsc", "Glen Allan WSC", "Conestogo River", 43.682, -80.590, "8635042", "10114042", "8635042"},
	{"grca_st_jacobs_wsc", "St. Jacobs WSC", "Conestogo River", 43.558, -80.557, "8641042", "11157042", "8641042"},
	// Laurel Creek
	{"grca_erbsville", "Erbsville", "Laurel Creek", 43.499, -80.620, "8785042", "10119042", "8785042"},
	{"grca_laurel_creek_weber_wsc", "Laurel Creek at Weber St. WSC", "Laurel Creek", 43.468, -80.537, "8791042", "10059042", "8791042"},
	// Schneider Creek
	{"grca_schneider_ottawa_st", "Schneider Creek at Ottawa St.", "Schneider Creek", 43.429, -80.476, "8857042", "10121042", "8857042"},
	// Speed River
	{"grca_armstrong_mills_wsc", "Armstrong Mills WSC", "Speed River", 43.833, -80.376, "8911042", "12247042", "8911042"},
	{"grca_victoria", "Victoria", "Speed River", 43.714, -80.308, "8899042", "10118042", "8899042"},
	{"grca_eramosa_watson_rd_wsc", "Eramosa River at Watson Rd. WSC", "Eramosa River", 43.634, -80.252, "8647042", "12234042", "8647042"},
	{"grca_speed_edinburgh_wsc", "Speed River at Edinburgh Rd. WSC", "Speed River", 43.558, -80.213, "8869042", "10065042", "8869042"},
	{"grca_speed_road32", "Speed River Road 32 Below Guelph", "Speed River", 43.490, -80.225, "8893042", "10135042", "8893042"},
	{"grca_speed_beaverdale_wsc", "Speed River at Beaverdale Rd. WSC", "Speed River", 43.415, -80.257, "8863042", "12248042", "8863042"},
	// Mill Creek
	{"grca_mill_creek_sr10", "Mill Creek at Side Road 10", "Mill Creek", 43.497, -80.332, "8797042", "10112042", "8797042"},
	// Nith River
	{"grca_nithburg_wsc", "Nithburg WSC", "Nith River", 43.875, -80.653, "8815042", "10108042", "8815042"},
	{"grca_philipsburg", "Philipsburg", "Nith River", 43.707, -80.553, "8839042", "10109042", "8839042"},
	{"grca_new_hamburg_wsc", "New Hamburg WSC", "Nith River", 43.382, -80.702, "8827042", "10066042", "8827042"},
	{"grca_ayr", "Ayr", "Nith River", 43.282, -80.461, "8821042", "12244042", "8821042"},
	{"grca_canning_wsc", "Canning WSC", "Nith River", 43.198, -80.375, "8845042", "12243042", "8845042"},
	// Whitemans / Fairchild / McKenzie
	{"grca_whitemans_mt_vernon_wsc", "Whitemans Creek at Mt. Vernon WSC", "Whitemans Creek", 43.136, -80.431, "8923042", "12438042", "8923042"},
	{"grca_fairchild_brantford_wsc", "Fairchild near Brantford WSC", "Fairchild Creek", 43.133, -80.268, "11383042", "10141042", "11383042"},
	{"grca_mckenzie_caledonia_wsc", "McKenzie Creek near Caledonia WSC", "McKenzie Creek", 43.059, -79.952, "9992042", "10077042", "9992042"},
}

// reservoirs maps the seven GRCA managed reservoirs.
// levelTSID = elevation (m MASL), flowTSID = discharge (m³/s).
var reservoirs = []stationMeta{
	{"grca_res_shand", "Shand Reservoir", "Grand River", 43.766, -80.369, "12169042", "12169042", "14154042"},
	{"grca_res_conestogo", "Conestogo Lake", "Conestogo River", 43.737, -80.505, "11389042", "11389042", "14146042"},
	{"grca_res_guelph", "Guelph Lake", "Speed River", 43.582, -80.214, "11391042", "11391042", "14148042"},
	{"grca_res_luther", "Luther Lake", "Grand River", 43.979, -80.400, "11396042", "11396042", "14150042"},
	{"grca_res_woolwich", "Woolwich Reservoir", "Canagagigue Creek", 43.571, -80.490, "11402042", "11402042", "12653042"},
	{"grca_res_laurel", "Laurel Creek Reservoir", "Laurel Creek", 43.484, -80.574, "11395042", "11395042", "12654042"},
	{"grca_res_shades", "Shades Mills Reservoir", "Mill Creek", 43.420, -80.311, "11535042", "11535042", "14152042"},
}

// Parser implements waterway.GaugeSource for the GRCA KiWIS data API.
type Parser struct {
	client *http.Client
}

// kiwisSeries is one entry in the KiWIS JSON array.
type kiwisSeries struct {
	TSID string   `json:"ts_id"`
	Rows string   `json:"rows"`
	Data [][2]any `json:"data"`
}

func (p *Parser) AllGauges(ctx context.Context) ([]domain.WaterwayGauge, error) {
	var (
		flowData, resData map[string][2]any
		flowErr, resErr   error
		wg                sync.WaitGroup
	)
	wg.Add(2)
	go func() {
		defer wg.Done()
		flowData, flowErr = fetchKiwis(ctx, p.client, flowURL)
	}()
	go func() {
		defer wg.Done()
		resData, resErr = fetchKiwis(ctx, p.client, reservoirURL)
	}()
	wg.Wait()

	if flowErr != nil {
		return nil, fmt.Errorf("grca flow fetch: %w", flowErr)
	}
	if resErr != nil {
		return nil, fmt.Errorf("grca reservoir fetch: %w", resErr)
	}

	var gauges []domain.WaterwayGauge
	for _, meta := range flowStations {
		gauges = append(gauges, buildGauge(meta, flowData))
	}
	for _, meta := range reservoirs {
		gauges = append(gauges, buildGauge(meta, resData))
	}
	return gauges, nil
}

func fetchKiwis(ctx context.Context, client *http.Client, url string) (map[string][2]any, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}

	var series []kiwisSeries
	if err := json.NewDecoder(resp.Body).Decode(&series); err != nil {
		return nil, err
	}

	out := make(map[string][2]any, len(series))
	for _, s := range series {
		if s.Rows == "1" && len(s.Data) == 1 {
			out[s.TSID] = s.Data[0]
		}
	}
	return out, nil
}

// buildGauge constructs a WaterwayGauge from the static metadata and the KiWIS value map.
// For reservoirs, LevelM contains elevation in m MASL and FlowCMS contains discharge.
func buildGauge(meta stationMeta, data map[string][2]any) domain.WaterwayGauge {
	g := domain.WaterwayGauge{
		ID:             meta.id,
		Name:           meta.name,
		WaterwayName:   meta.waterwayName,
		Lat:            meta.lat,
		Lon:            meta.lon,
		AdvisoryStatus: "none",
		SourceURL:      sourceURLForGauge(meta.id),
	}

	// Timestamp from the time ts_id entry.
	if entry, ok := data[meta.timeTSID]; ok {
		if ms, err := toFloat64(entry[0]); err == nil {
			g.UpdatedAt = time.UnixMilli(int64(ms)).UTC()
		}
	}
	if g.UpdatedAt.IsZero() {
		g.UpdatedAt = time.Now().UTC()
	}

	if entry, ok := data[meta.levelTSID]; ok {
		if v, err := toFloat64(entry[1]); err == nil {
			g.LevelM = &v
		}
	}

	if entry, ok := data[meta.flowTSID]; ok {
		if v, err := toFloat64(entry[1]); err == nil && meta.flowTSID != meta.levelTSID {
			g.FlowCMS = &v
		}
	}

	return g
}

func sourceURLForGauge(id string) string {
	if url := sourceURLsByGaugeID[id]; url != "" {
		return url
	}
	return flowSummaryURL
}

func toFloat64(v any) (float64, error) {
	switch x := v.(type) {
	case float64:
		return x, nil
	case string:
		return strconv.ParseFloat(x, 64)
	default:
		return 0, fmt.Errorf("unexpected type %T", v)
	}
}
