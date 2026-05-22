package flag

// Phase 1: HTTP scrape of the canonical half-masting notices page.
// Phase 2 (TODO): register as an EmailHandler with the IMAP dispatcher:
//   SenderPatterns:  []string{"no-reply@canada.ca", "info@canada.ca"}
//   SubjectPatterns: []string{`(?i)half.?mast`, `(?i)flag.*lower`}

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rmrobinson/cupola/internal/collector/astro"
	"github.com/rmrobinson/cupola/internal/domain"
	"github.com/rmrobinson/cupola/internal/store"
)

const defaultNoticesURL = "https://www.canada.ca/en/canadian-heritage/services/half-masting-notices.html"

// Canada scrapes the authoritative half-masting notices page, filters notices
// to those currently active and relevant to the configured location, and
// publishes the combined FlagStatus.
type Canada struct {
	url        string
	lat, lon   float64
	province   string // 2-letter code derived from lat/lon
	interval   time.Duration
	loc        *time.Location
	stateStore *store.StateStore
	netCheck   func() bool
	mu         sync.RWMutex
	state      domain.FlagStatus
	wake       chan struct{}
}

func (c *Canada) SetNetCheck(fn func() bool) { c.netCheck = fn }

func NewCanada(lat, lon float64, interval time.Duration, stateStore *store.StateStore) *Canada {
	return NewCanadaWithURL("", lat, lon, interval, stateStore)
}

func NewCanadaWithURL(url string, lat, lon float64, interval time.Duration, stateStore *store.StateStore) *Canada {
	return NewCanadaWithURLInLocation(url, lat, lon, interval, stateStore, time.Local)
}

func NewCanadaWithURLInLocation(url string, lat, lon float64, interval time.Duration, stateStore *store.StateStore, loc *time.Location) *Canada {
	if url == "" {
		url = defaultNoticesURL
	}
	if loc == nil {
		loc = time.Local
	}
	return &Canada{
		url:        url,
		lat:        lat,
		lon:        lon,
		province:   provinceCode(lat, lon),
		interval:   interval,
		loc:        loc,
		stateStore: stateStore,
		wake:       make(chan struct{}, 1),
	}
}

func (c *Canada) ID() string                { return "canada.flag" }
func (c *Canada) Domain() domain.DomainType { return domain.DomainFlagStatus }

func (c *Canada) OnSubscription() {
	select {
	case c.wake <- struct{}{}:
	default:
	}
}

func (c *Canada) Start(ctx context.Context) error {
	go func() {
		if c.netCheck == nil || c.netCheck() {
			if err := c.fetch(); err != nil {
				log.Printf("[canada.flag] initial fetch: %v", err)
			}
		}
		c.loop(ctx)
	}()
	return nil
}

func (c *Canada) State() domain.DomainState {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.state
}

func (c *Canada) loop(ctx context.Context) {
	t := time.NewTicker(c.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			c.fetchIfReady()
		case <-c.wake:
			c.fetchIfReady()
		}
	}
}

func (c *Canada) fetchIfReady() {
	if c.netCheck != nil && !c.netCheck() {
		return
	}
	if err := c.fetch(); err != nil {
		log.Printf("[canada.flag] fetch: %v", err)
		c.stateStore.PublishSystem(store.SystemEvent{
			CollectorID: c.ID(), Status: "error", Message: err.Error(),
		})
	} else {
		c.stateStore.PublishSystem(store.SystemEvent{CollectorID: c.ID(), Status: "ok"})
	}
}

func (c *Canada) fetch() error {
	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Get(c.url)
	if err != nil {
		return fmt.Errorf("get %s: %w", c.url, err)
	}
	body, readErr := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if readErr != nil {
		return fmt.Errorf("read: %w", readErr)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("get %s: status %d", c.url, resp.StatusCode)
	}

	notices := parseNoticesInLocation(string(body), c.loc, c.lat, c.lon)
	notices = resolveNoticeUpdates(notices)
	relevant := filterActive(notices, c.province)
	state := buildStatus(relevant, c.url)

	c.mu.Lock()
	c.state = state
	c.mu.Unlock()
	c.stateStore.Set(state)
	log.Printf("[canada.flag] province=%s parsed=%d active+relevant=%d at-half-mast=%v",
		c.province, len(notices), len(relevant), state.AtHalfMast)
	return nil
}

// ── Province detection ────────────────────────────────────────────────────────

func provinceCode(lat, lon float64) string {
	if lat >= 60 {
		if lon <= -128 {
			return "YT"
		}
		if lon <= -96 {
			return "NT"
		}
		return "NU"
	}
	switch {
	case lon <= -116:
		return "BC"
	case lon <= -110:
		return "AB"
	case lon <= -101:
		return "SK"
	case lon <= -95:
		return "MB"
	case lon > -60:
		return "NL"
	}
	if lat > 45.9 && lat < 47.2 && lon > -64.5 && lon <= -62 {
		return "PE"
	}
	if lat < 47.5 && lon > -64 && lon <= -59.5 {
		return "NS"
	}
	if lon > -68 && lon <= -64 {
		return "NB"
	}
	if lon <= -74 {
		return "ON"
	}
	if lat > 45 {
		return "QC"
	}
	return "ON"
}

// ── HTML table parsing ────────────────────────────────────────────────────────

// The notices page uses <table id="eventslist"> with rows of exactly 4 <td> cells.
// The first 3 use class="nws-tbl-desc" (no extra classes); the 4th adds "mrgn-bttm-md".
// Matching on class="nws-tbl-desc"> (exact) captures only the first 3 per row.
var (
	tableBodyRe  = regexp.MustCompile(`(?s)<tbody>(.*?)</tbody>`)
	trRe         = regexp.MustCompile(`(?s)<tr[^>]*>(.*?)</tr>`)
	tdRe         = regexp.MustCompile(`(?s)<td class="nws-tbl-desc">(.*?)</td>`)
	hiddenSpanRe = regexp.MustCompile(`(?s)<span[^>]*class="hidden"[^>]*>.*?</span>`)
	hiddenTSRe   = regexp.MustCompile(`<span[^>]*class="hidden"[^>]*>(\d+)</span>`)
	anyTagRe     = regexp.MustCompile(`<[^>]+>`)
	dateRe       = regexp.MustCompile(
		`(January|February|March|April|May|June|July|August|September|October|November|December)\s+(\d{1,2})(?:st|nd|rd|th)?,?\s+(\d{4})`)
	timeRe = regexp.MustCompile(`(?i)\b(\d{1,2})(?::(\d{2}))?\s*(a\.?m\.?|p\.?m\.?)?\b`)
)

// maxNoticAge is how far back we look for notices without an explicit end date.
// The notices table is a historical archive; we only want recent open-ended notices.
const maxNoticeAge = 90 * 24 * time.Hour

var monthNums = map[string]time.Month{
	"January": time.January, "February": time.February, "March": time.March,
	"April": time.April, "May": time.May, "June": time.June,
	"July": time.July, "August": time.August, "September": time.September,
	"October": time.October, "November": time.November, "December": time.December,
}

type halfMastNotice struct {
	province  string
	reason    string
	since     *time.Time
	until     *time.Time
	createdAt time.Time // from the hidden Unix-ms timestamp in each table row
	updated   bool
}

func parseNotices(html string) []halfMastNotice {
	return parseNoticesInLocation(html, time.UTC, 0, 0)
}

func parseNoticesInLocation(html string, loc *time.Location, lat, lon float64) []halfMastNotice {
	if loc == nil {
		loc = time.UTC
	}
	// Restrict parsing to the table body so we don't pick up nav/footer links.
	tbody := tableBodyRe.FindString(html)
	if tbody == "" {
		log.Printf("[canada.flag] <tbody> not found — page structure may have changed")
		return nil
	}

	var notices []halfMastNotice
	for _, tr := range trRe.FindAllString(tbody, -1) {
		cells := tdRe.FindAllStringSubmatch(tr, -1)
		if len(cells) < 3 {
			continue // header row or incomplete
		}
		reasonRaw := cellText(cells[0][1])
		periodRaw := cellText(cells[1][1])
		locationRaw := cellText(cells[2][1])

		// Extract the publication timestamp from the hidden span so we can
		// reject old open-ended notices (the table is a full historical archive).
		var createdAt time.Time
		if tsm := hiddenTSRe.FindStringSubmatch(tr); len(tsm) >= 2 {
			if ms, err := strconv.ParseInt(tsm[1], 10, 64); err == nil {
				createdAt = time.Unix(ms/1000, 0)
			}
		}

		updated := strings.HasPrefix(reasonRaw, "Updated ")
		reason := extractNoticeReason(reasonRaw)
		since, until := parsePeriodAtLocationWithPublished(periodRaw, loc, lat, lon, createdAt)
		province := detectProvince(locationRaw)

		notices = append(notices, halfMastNotice{
			province:  province,
			reason:    reason,
			since:     since,
			until:     until,
			createdAt: createdAt,
			updated:   updated,
		})
	}
	return notices
}

// cellText strips the hidden timestamp span, all HTML tags, and normalises whitespace.
func cellText(html string) string {
	s := hiddenSpanRe.ReplaceAllString(html, "")
	s = anyTagRe.ReplaceAllString(s, " ")
	for old, repl := range map[string]string{
		"&amp;": "&", "&lt;": "<", "&gt;": ">", "&#39;": "'", "&nbsp;": " ",
	} {
		s = strings.ReplaceAll(s, old, repl)
	}
	return strings.Join(strings.Fields(s), " ")
}

// extractNoticeReason returns the text after "Notice of half-masting:" in the cell.
func extractNoticeReason(text string) string {
	text = strings.TrimSpace(strings.TrimPrefix(text, "Updated"))
	const marker = "Notice of half-masting:"
	if i := strings.Index(text, marker); i >= 0 {
		text = strings.TrimSpace(text[i+len(marker):])
	}
	if len(text) > 0 {
		return strings.ToUpper(text[:1]) + text[1:]
	}
	return text
}

func parsePeriodAtLocation(text string, loc *time.Location, lat, lon float64) (since, until *time.Time) {
	return parsePeriodAtLocationWithPublished(text, loc, lat, lon, time.Time{})
}

func parsePeriodAtLocationWithPublished(text string, loc *time.Location, lat, lon float64, published time.Time) (since, until *time.Time) {
	if loc == nil {
		loc = time.UTC
	}
	lower := strings.ToLower(text)
	// "date to be determined" → until remains nil (indefinitely active)
	tbd := strings.Contains(lower, "date to be determined") ||
		strings.Contains(lower, "to be determined")

	matches := dateRe.FindAllStringSubmatch(text, -1)
	if len(matches) == 0 {
		return nil, nil
	}

	parseDate := func(m []string, h, min, sec int) *time.Time {
		month, ok := monthNums[m[1]]
		if !ok {
			return nil
		}
		day, _ := strconv.Atoi(m[2])
		year, _ := strconv.Atoi(m[3])
		t := time.Date(year, month, day, h, min, sec, 0, loc)
		return &t
	}

	startDate := parseDate(matches[0], 0, 1, 0)
	endDate := parseDate(matches[len(matches)-1], 23, 59, 59)
	if startDate == nil || endDate == nil {
		return nil, nil
	}

	s := *startDate
	u := *endDate
	if strings.Contains(lower, "from now") && !published.IsZero() {
		s = published.In(loc)
	}

	if startClock, endClock, ok := explicitTimeWindow(text); ok {
		s = withClock(s, startClock, loc)
		u = withClock(u, endClock, loc)
	} else {
		if strings.Contains(lower, "sunrise") {
			if rise, ok := astro.Sunrise(s, lat, lon); ok {
				s = rise.In(loc)
			}
		}
		if strings.Contains(lower, "sunset") {
			if set, ok := astro.Sunset(u, lat, lon); ok {
				u = set.In(loc)
			}
		}
	}

	if !u.After(s) {
		u = u.Add(24 * time.Hour)
	}
	if tbd {
		return &s, nil
	}
	return &s, &u
}

type clockTime struct {
	hour   int
	minute int
}

func explicitTimeWindow(text string) (clockTime, clockTime, bool) {
	matches := timeRe.FindAllStringSubmatch(text, -1)
	raw := make([]struct {
		hour     int
		minute   int
		meridiem string
	}, 0, 2)
	for _, m := range matches {
		if len(m) < 4 || (m[2] == "" && m[3] == "") {
			continue
		}
		hour, err := strconv.Atoi(m[1])
		if err != nil || hour < 0 || hour > 23 {
			continue
		}
		if m[3] != "" && (hour < 1 || hour > 12) {
			continue
		}
		minute := 0
		if m[2] != "" {
			minute, err = strconv.Atoi(m[2])
			if err != nil || minute < 0 || minute > 59 {
				continue
			}
		}
		raw = append(raw, struct {
			hour     int
			minute   int
			meridiem string
		}{hour: hour, minute: minute, meridiem: m[3]})
	}
	if len(raw) < 2 {
		return clockTime{}, clockTime{}, false
	}
	startMeridiem := raw[0].meridiem
	endMeridiem := raw[1].meridiem
	if startMeridiem == "" {
		startMeridiem = endMeridiem
	}
	if endMeridiem == "" {
		endMeridiem = startMeridiem
	}
	if startMeridiem == "" && endMeridiem == "" {
		return clockTime{hour: raw[0].hour, minute: raw[0].minute}, clockTime{hour: raw[1].hour, minute: raw[1].minute}, true
	}
	if startMeridiem == "" || endMeridiem == "" || raw[0].hour == 0 || raw[1].hour == 0 {
		return clockTime{}, clockTime{}, false
	}
	start := normalizeClock(raw[0].hour, raw[0].minute, startMeridiem)
	end := normalizeClock(raw[1].hour, raw[1].minute, endMeridiem)
	return start, end, true
}

func normalizeClock(hour, minute int, meridiem string) clockTime {
	if isPM(meridiem) && hour != 12 {
		hour += 12
	}
	if !isPM(meridiem) && hour == 12 {
		hour = 0
	}
	return clockTime{hour: hour, minute: minute}
}

func withClock(date time.Time, clock clockTime, loc *time.Location) time.Time {
	return time.Date(date.Year(), date.Month(), date.Day(), clock.hour, clock.minute, 0, 0, loc)
}

func isPM(s string) bool {
	return strings.HasPrefix(strings.ToLower(strings.ReplaceAll(s, ".", "")), "p")
}

// ── Province detection ────────────────────────────────────────────────────────

var provinceVariants = map[string][]string{
	"BC": {"british columbia", "b.c."},
	"AB": {"alberta"},
	"SK": {"saskatchewan"},
	"MB": {"manitoba"},
	"ON": {"ontario"},
	"QC": {"québec", "quebec"},
	"NB": {"new brunswick", "n.b."},
	"NS": {"nova scotia", "n.s."},
	"PE": {"prince edward island", "p.e.i."},
	"NL": {"newfoundland", "labrador", "n.l."},
	"YT": {"yukon"},
	"NT": {"northwest territories", "n.w.t."},
	"NU": {"nunavut"},
}

func detectProvince(text string) string {
	lower := strings.ToLower(text)

	// Explicit country-wide language → always national
	if strings.Contains(lower, "across canada") ||
		strings.Contains(lower, "across the country") ||
		strings.Contains(lower, "throughout canada") ||
		strings.Contains(lower, "throughout the country") ||
		strings.Contains(lower, "all federal") {
		return "NATIONAL"
	}

	// Province name match → specific province
	for code, variants := range provinceVariants {
		for _, v := range variants {
			if strings.Contains(lower, v) {
				return code
			}
		}
	}

	// "The Peace Tower in Ottawa" alone (no province) → treat as national symbol
	if strings.Contains(lower, "peace tower") || strings.Contains(lower, "parliament") {
		return "NATIONAL"
	}

	return "NATIONAL" // no location specified → show to all
}

// ── Filtering and status building ─────────────────────────────────────────────

func resolveNoticeUpdates(notices []halfMastNotice) []halfMastNotice {
	superseded := make([]bool, len(notices))
	for i, updated := range notices {
		if !updated.updated {
			continue
		}
		for j, original := range notices {
			if i == j || original.updated {
				continue
			}
			if noticeSupersedes(updated, original) {
				superseded[j] = true
			}
		}
	}

	out := make([]halfMastNotice, 0, len(notices))
	for i, n := range notices {
		if !superseded[i] {
			out = append(out, n)
		}
	}
	return out
}

func noticeSupersedes(updated, original halfMastNotice) bool {
	if !sameNoticeKey(updated, original) {
		return false
	}
	if updated.createdAt.IsZero() || original.createdAt.IsZero() {
		return true
	}
	return updated.createdAt.After(original.createdAt)
}

func sameNoticeKey(a, b halfMastNotice) bool {
	if normalizeNoticeText(a.reason) != normalizeNoticeText(b.reason) {
		return false
	}
	if a.province != b.province {
		return false
	}
	if a.since == nil || b.since == nil {
		return true
	}
	return sameNoticeDay(*a.since, *b.since)
}

func normalizeNoticeText(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(s)), " ")
}

func sameNoticeDay(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}

// filterActive returns notices that are:
//  1. Not expired    — until is nil, or until is still in the future
//  2. Already active — since is nil, or since is in the past
//  3. Not historical — notices with no end date are only kept when they were
//     published within maxNoticeAge (the table is a full archive)
//  4. Relevant       — national scope or matching the given province
func filterActive(notices []halfMastNotice, province string) []halfMastNotice {
	return filterActiveAt(notices, province, time.Now())
}

func filterActiveAt(notices []halfMastNotice, province string, now time.Time) []halfMastNotice {
	var out []halfMastNotice
	for _, n := range notices {
		// If the period could not be parsed at all, do not treat a recent row
		// as open-ended. The Canada page includes future notices with ordinals
		// and historical "see rules" rows; unknown periods are safer hidden.
		if n.since == nil && n.until == nil {
			continue
		}
		// Drop expired notices
		if n.until != nil && !n.until.After(now) {
			continue
		}
		// Drop notices that haven't started yet
		if n.since != nil && n.since.After(now) {
			continue
		}
		// Drop open-ended historical notices: no end date + published long ago.
		// This handles the archive rows going back to 2011.
		if n.until == nil {
			if !n.createdAt.IsZero() && now.Sub(n.createdAt) > maxNoticeAge {
				continue
			}
			// If we have no publication timestamp, use the since date as a proxy.
			if n.createdAt.IsZero() && n.since != nil && now.Sub(*n.since) > maxNoticeAge {
				continue
			}
		}
		// Province filter
		if n.province == "NATIONAL" || n.province == "" || n.province == province {
			out = append(out, n)
		}
	}
	return out
}

// buildStatus combines relevant notices into a single FlagStatus.
func buildStatus(notices []halfMastNotice, sourceURL string) domain.FlagStatus {
	base := domain.FlagStatus{
		StateBase: domain.StateBase{UpdatedAt: time.Now().UTC()},
		SourceURL: sourceURL,
	}
	if len(notices) == 0 {
		return base
	}

	base.AtHalfMast = true
	var reasons []string
	var earliest *time.Time
	var latest *time.Time
	indefinite := false

	for _, n := range notices {
		if n.reason != "" {
			reasons = append(reasons, n.reason)
		}
		if n.since != nil && (earliest == nil || n.since.Before(*earliest)) {
			earliest = n.since
		}
		if n.until == nil {
			indefinite = true
		} else if !indefinite && (latest == nil || n.until.After(*latest)) {
			latest = n.until
		}
	}

	if len(reasons) > 0 {
		r := strings.Join(reasons, "; ")
		base.Reason = &r
	}
	base.Since = earliest
	if !indefinite {
		base.Until = latest
	}
	return base
}
