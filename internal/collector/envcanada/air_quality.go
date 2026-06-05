package envcanada

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/rmrobinson/cupola/internal/domain"
	"github.com/rmrobinson/cupola/internal/store"
	"golang.org/x/net/html"
)

const airQualityBaseURL = "https://weather.gc.ca"

type AirQualityOptions struct {
	Province string
	Location string
	Station  StationOverride
}

type AirQualityCollector struct {
	userLat    float64
	userLon    float64
	options    AirQualityOptions
	interval   time.Duration
	stateStore *store.StateStore
	netCheck   func() bool
	mu         sync.RWMutex
	state      domain.WeatherAirQuality
	wake       chan struct{}
}

func NewAirQualityCollector(lat, lon float64, interval time.Duration, stateStore *store.StateStore, options AirQualityOptions) *AirQualityCollector {
	return &AirQualityCollector{
		userLat:    lat,
		userLon:    lon,
		options:    options,
		interval:   interval,
		stateStore: stateStore,
		wake:       make(chan struct{}, 1),
	}
}

func (c *AirQualityCollector) SetNetCheck(fn func() bool) { c.netCheck = fn }

func (c *AirQualityCollector) OnSubscription() {
	select {
	case c.wake <- struct{}{}:
	default:
	}
}

func (c *AirQualityCollector) ID() string                { return "envcanada.air_quality" }
func (c *AirQualityCollector) Domain() domain.DomainType { return domain.DomainWeatherAirQuality }

func (c *AirQualityCollector) Start(ctx context.Context) error {
	go func() {
		var site *airQualitySite
		if c.netCheck == nil || c.netCheck() {
			resolved, err := c.resolveSite()
			if err != nil {
				log.Printf("[envcanada.air_quality] site discovery: %v", err)
				c.stateStore.PublishSystem(store.SystemEvent{
					CollectorID: c.ID(), Status: "error", Message: err.Error(),
				})
			} else {
				site = resolved
			}
		}
		if site != nil {
			if err := c.fetch(*site); err != nil {
				log.Printf("[envcanada.air_quality] initial fetch: %v", err)
			}
		}
		c.loop(ctx, site)
	}()
	return nil
}

func (c *AirQualityCollector) State() domain.DomainState {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.state
}

func (c *AirQualityCollector) loop(ctx context.Context, site *airQualitySite) {
	t := time.NewTicker(c.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			site = c.fetchIfReady(site)
		case <-c.wake:
			site = c.fetchIfReady(site)
		}
	}
}

func (c *AirQualityCollector) fetchIfReady(site *airQualitySite) *airQualitySite {
	if c.netCheck != nil && !c.netCheck() {
		return site
	}
	if site == nil {
		resolved, err := c.resolveSite()
		if err != nil {
			log.Printf("[envcanada.air_quality] site discovery: %v", err)
			c.stateStore.PublishSystem(store.SystemEvent{
				CollectorID: c.ID(), Status: "error", Message: err.Error(),
			})
			return site
		}
		site = resolved
	}
	if err := c.fetch(*site); err != nil {
		log.Printf("[envcanada.air_quality] fetch: %v", err)
		c.stateStore.PublishSystem(store.SystemEvent{
			CollectorID: c.ID(), Status: "error", Message: err.Error(),
		})
	} else {
		c.stateStore.PublishSystem(store.SystemEvent{CollectorID: c.ID(), Status: "ok"})
	}
	return site
}

func (c *AirQualityCollector) resolveSite() (*airQualitySite, error) {
	province, err := c.resolveProvince()
	if err != nil {
		return nil, err
	}
	summary, err := fetchAirQualitySummary(province)
	if err != nil {
		return nil, err
	}
	location := strings.TrimSpace(c.options.Location)
	if location != "" {
		row, ok := findAirQualitySite(summary.Sites, location)
		if !ok {
			return nil, fmt.Errorf("air quality location %q not found in province %s; available: %s",
				location, province, availableAirQualitySites(summary.Sites))
		}
		log.Printf("[envcanada.air_quality] configured AQHI site: %s, %s", row.Location, province)
		return &airQualitySite{Province: province, Location: row.Location}, nil
	}
	stations, err := provinceStations(province)
	if err != nil {
		return nil, err
	}
	site, matched, err := discoverAirQualitySite(summary.Sites, stations, c.userLat, c.userLon)
	if err != nil {
		return nil, err
	}
	log.Printf("[envcanada.air_quality] selected AQHI site: %s, %s (matched station %s)",
		site.Location, province, matched.Name)
	return &airQualitySite{Province: province, Location: site.Location}, nil
}

func (c *AirQualityCollector) resolveProvince() (string, error) {
	if province := normalizeProvince(c.options.Province); province != "" {
		return province, nil
	}
	if province := normalizeProvince(c.options.Station.Province); province != "" {
		return province, nil
	}
	lat, lon, name, err := discoverNearestStation(c.userLat, c.userLon)
	if err != nil {
		return "", err
	}
	stations, err := allStations()
	if err != nil {
		return "", err
	}
	for _, s := range stations {
		if s.Lat == lat && s.Lon == lon && s.Name == name {
			return normalizeProvince(s.Province), nil
		}
	}
	return "", fmt.Errorf("could not determine province for nearest station %q", name)
}

func (c *AirQualityCollector) fetch(site airQualitySite) error {
	summary, err := fetchAirQualitySummary(site.Province)
	if err != nil {
		return err
	}
	row, ok := findAirQualitySite(summary.Sites, site.Location)
	if !ok {
		return fmt.Errorf("air quality location %q not found in province %s; available: %s",
			site.Location, site.Province, availableAirQualitySites(summary.Sites))
	}

	now := time.Now().UTC()
	state := domain.WeatherAirQuality{
		StateBase: domain.StateBase{UpdatedAt: now},
		Location:  row.Location,
		Province:  site.Province,
		SourceURL: row.SourceURL,
		Observed:  row.Observed,
		Forecasts: row.Forecasts,
		IssuedAt:  summary.IssuedAt,
	}

	c.mu.Lock()
	c.state = state
	c.mu.Unlock()
	c.stateStore.Set(state)
	log.Printf("[envcanada.air_quality] updated: %s observed=%s forecasts=%d",
		row.Location, formatAQHIForLog(row.Observed), len(row.Forecasts))
	return nil
}

type airQualitySite struct {
	Province string
	Location string
}

type airQualitySummary struct {
	Province string
	Sites    []airQualityRow
	IssuedAt time.Time
}

type airQualityRow struct {
	Location  string
	SourceURL string
	Observed  *domain.AQHIValue
	Forecasts []domain.AQHIForecastPeriod
}

func fetchAirQualitySummary(province string) (*airQualitySummary, error) {
	province = normalizeProvince(province)
	u := airQualitySummaryURL(province)
	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Get(u)
	if err != nil {
		return nil, fmt.Errorf("get %s: %w", u, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get %s: status %d", u, resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", u, err)
	}
	return parseAirQualitySummaryHTML(body, province, airQualityBaseURL)
}

func airQualitySummaryURL(province string) string {
	return fmt.Sprintf("%s/airquality/pages/provincial_summary/%s_e.html", airQualityBaseURL, strings.ToLower(normalizeProvince(province)))
}

func parseAirQualitySummaryHTML(body []byte, province, baseURL string) (*airQualitySummary, error) {
	root, err := html.Parse(strings.NewReader(string(body)))
	if err != nil {
		return nil, fmt.Errorf("parse AQHI HTML: %w", err)
	}
	table := findAirQualitySummaryTable(root)
	if table == nil {
		return nil, fmt.Errorf("air quality summary table not found")
	}
	forecastLabels := forecastHeaders(table)
	if len(forecastLabels) == 0 {
		return nil, fmt.Errorf("air quality forecast headers not found")
	}

	var rows []airQualityRow
	for _, tr := range childElements(table, "tr") {
		cells := childElements(tr, "td")
		if len(cells) < 2 {
			continue
		}
		location := cleanText(nodeText(cells[0]))
		if location == "" {
			continue
		}
		row := airQualityRow{
			Location:  location,
			SourceURL: absoluteURL(baseURL, firstLinkHref(cells[0])),
			Observed:  parseAQHIValue(nodeText(cells[1])),
		}
		for i, label := range forecastLabels {
			cellIndex := i + 2
			if cellIndex >= len(cells) {
				break
			}
			row.Forecasts = append(row.Forecasts, domain.AQHIForecastPeriod{
				Label: label,
				Max:   parseAQHIValue(nodeText(cells[cellIndex])),
			})
		}
		rows = append(rows, row)
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("air quality summary has no locations")
	}
	return &airQualitySummary{
		Province: normalizeProvince(province),
		Sites:    rows,
		IssuedAt: parseAirQualityIssuedAt(root),
	}, nil
}

func findAirQualitySummaryTable(root *html.Node) *html.Node {
	var table *html.Node
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if table != nil {
			return
		}
		if n.Type == html.ElementNode && n.Data == "table" && strings.Contains(strings.ToLower(nodeText(n)), "observed conditions") {
			table = n
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(root)
	return table
}

func forecastHeaders(table *html.Node) []string {
	var labels []string
	for _, tr := range childElements(table, "tr") {
		ths := childElements(tr, "th")
		if len(ths) == 0 {
			continue
		}
		rowLabels := make([]string, 0, len(ths))
		for _, th := range ths {
			text := cleanText(nodeText(th))
			if text == "" || strings.Contains(strings.ToLower(text), "forecast maximums") {
				continue
			}
			if strings.Contains(strings.ToLower(text), "location") || strings.Contains(strings.ToLower(text), "observed") {
				continue
			}
			rowLabels = append(rowLabels, text)
		}
		if len(rowLabels) > len(labels) {
			labels = rowLabels
		}
	}
	return labels
}

func childElements(root *html.Node, name string) []*html.Node {
	var out []*html.Node
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == name {
			out = append(out, n)
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	for c := root.FirstChild; c != nil; c = c.NextSibling {
		walk(c)
	}
	return out
}

func nodeText(n *html.Node) string {
	var parts []string
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			if text := strings.TrimSpace(n.Data); text != "" {
				parts = append(parts, text)
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return strings.Join(parts, " ")
}

func firstLinkHref(n *html.Node) string {
	var href string
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if href != "" {
			return
		}
		if n.Type == html.ElementNode && n.Data == "a" {
			for _, a := range n.Attr {
				if a.Key == "href" {
					href = a.Val
					return
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return href
}

var aqhiValueRe = regexp.MustCompile(`(?:10\+|\b\d{1,2}\b)`)

func parseAQHIValue(text string) *domain.AQHIValue {
	text = cleanText(text)
	if text == "" || text == "-" || text == "N/A" {
		return nil
	}
	value := domain.AQHIValue{}
	if m := aqhiValueRe.FindString(text); m != "" {
		if strings.HasSuffix(m, "+") {
			v := 10
			value.Value = &v
		} else if v, err := strconv.Atoi(m); err == nil {
			value.Value = &v
		}
	}
	risk := strings.TrimSpace(aqhiValueRe.ReplaceAllString(text, ""))
	risk = strings.Trim(risk, " -")
	value.Risk = cleanText(risk)
	if value.Value == nil && value.Risk == "" {
		return nil
	}
	return &value
}

func parseAirQualityIssuedAt(root *html.Node) time.Time {
	var candidates []string
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			for _, a := range n.Attr {
				if a.Key != "title" {
					continue
				}
				lower := strings.ToLower(a.Val)
				if strings.Contains(lower, "edt") || strings.Contains(lower, "est") || strings.Contains(lower, "issued") {
					candidates = append(candidates, a.Val)
				}
			}
		}
		if n.Type == html.TextNode && strings.Contains(strings.ToLower(n.Data), "forecast issued at") {
			candidates = append(candidates, n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(root)
	for _, c := range candidates {
		if ts := parseIssuedTime(c); !ts.IsZero() {
			return ts
		}
	}
	return time.Time{}
}

func parseIssuedTime(s string) time.Time {
	s = cleanText(strings.TrimPrefix(s, "1."))
	s = strings.TrimPrefix(s, "Forecast issued at:")
	s = strings.TrimSpace(s)
	layouts := []string{
		"3:04 PM MST Monday 2 January 2006",
		"3:04 PM Monday 2 January 2006",
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

func findAirQualitySite(rows []airQualityRow, location string) (airQualityRow, bool) {
	want := normalizeAQHIName(location)
	for _, row := range rows {
		if normalizeAQHIName(row.Location) == want {
			return row, true
		}
	}
	for _, row := range rows {
		got := normalizeAQHIName(row.Location)
		if strings.Contains(got, want) || strings.Contains(want, got) {
			return row, true
		}
	}
	return airQualityRow{}, false
}

func discoverAirQualitySite(rows []airQualityRow, stations []ECStation, userLat, userLon float64) (airQualityRow, ECStation, error) {
	type candidate struct {
		row     airQualityRow
		station ECStation
		score   float64
		dist    float64
	}
	var candidates []candidate
	for _, row := range rows {
		station, score := bestStationMatch(row.Location, stations)
		if score < 0.45 {
			continue
		}
		candidates = append(candidates, candidate{
			row:     row,
			station: station,
			score:   score,
			dist:    haversineKm(userLat, userLon, station.Lat, station.Lon),
		})
	}
	if len(candidates) == 0 {
		return airQualityRow{}, ECStation{}, fmt.Errorf("could not confidently match AQHI sites to stations; configure air_quality_location explicitly. Available: %s", availableAirQualitySites(rows))
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].dist == candidates[j].dist {
			return candidates[i].score > candidates[j].score
		}
		return candidates[i].dist < candidates[j].dist
	})
	return candidates[0].row, candidates[0].station, nil
}

func bestStationMatch(location string, stations []ECStation) (ECStation, float64) {
	var best ECStation
	var bestScore float64
	for _, station := range stations {
		score := nameMatchScore(location, station.Name)
		if score > bestScore {
			best = station
			bestScore = score
		}
	}
	return best, bestScore
}

func nameMatchScore(a, b string) float64 {
	na := normalizeAQHIName(a)
	nb := normalizeAQHIName(b)
	if na == "" || nb == "" {
		return 0
	}
	if na == nb {
		return 1
	}
	if strings.Contains(nb, na) || strings.Contains(na, nb) {
		return 0.82
	}
	at := tokenSet(na)
	bt := tokenSet(nb)
	if len(at) == 0 || len(bt) == 0 {
		return 0
	}
	common := 0
	for t := range at {
		if bt[t] {
			common++
		}
	}
	if common == 0 {
		return 0
	}
	return float64(common) / float64(minInt(len(at), len(bt)))
}

func tokenSet(s string) map[string]bool {
	out := make(map[string]bool)
	for _, token := range strings.Fields(s) {
		if len(token) > 2 {
			out[token] = true
		}
	}
	return out
}

func normalizeAQHIName(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, "'", "")
	s = strings.ReplaceAll(s, "’", "")
	var b strings.Builder
	lastSpace := true
	for _, r := range s {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			lastSpace = false
		case !lastSpace:
			b.WriteByte(' ')
			lastSpace = true
		}
	}
	return strings.TrimSpace(b.String())
}

func cleanText(s string) string {
	return strings.Join(strings.Fields(strings.ReplaceAll(s, "\u00a0", " ")), " ")
}

func normalizeProvince(province string) string {
	return strings.ToUpper(strings.TrimSpace(province))
}

func availableAirQualitySites(rows []airQualityRow) string {
	names := make([]string, 0, len(rows))
	for _, row := range rows {
		names = append(names, row.Location)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

func absoluteURL(base, href string) string {
	if href == "" {
		return ""
	}
	u, err := url.Parse(href)
	if err != nil {
		return href
	}
	if u.IsAbs() {
		return u.String()
	}
	b, err := url.Parse(base)
	if err != nil {
		return href
	}
	return b.ResolveReference(u).String()
}

func formatAQHIForLog(v *domain.AQHIValue) string {
	if v == nil {
		return "missing"
	}
	if v.Value == nil {
		return v.Risk
	}
	return fmt.Sprintf("%d %s", *v.Value, v.Risk)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
