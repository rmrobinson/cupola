package flag

import (
	"testing"
	"time"
)

func TestUpdatedNoticeSupersedesOpenEndedOriginal(t *testing.T) {
	createdOriginal := time.Date(2026, time.April, 29, 10, 0, 0, 0, time.UTC)
	createdUpdate := time.Date(2026, time.May, 1, 10, 0, 0, 0, time.UTC)
	notices := resolveNoticeUpdates([]halfMastNotice{
		{
			province:  "ON",
			reason:    "Death of Sergeant Brandon Malcolm of the Ontario Provincial Police",
			since:     day(2026, time.April, 29, false),
			until:     day(2026, time.May, 6, true),
			createdAt: createdUpdate,
			updated:   true,
		},
		{
			province:  "ON",
			reason:    "Death of Sergeant Brandon Malcolm of the Ontario Provincial Police",
			since:     day(2026, time.April, 29, false),
			until:     nil,
			createdAt: createdOriginal,
		},
	})

	active := filterActiveAt(notices, "ON", time.Date(2026, time.May, 8, 12, 0, 0, 0, time.UTC))
	if len(active) != 0 {
		t.Fatalf("active notices = %#v, want none", active)
	}
}

func TestOpenEndedOriginalRemainsWhenNoUpdate(t *testing.T) {
	notices := resolveNoticeUpdates([]halfMastNotice{
		{
			province:  "ON",
			reason:    "Death of Sergeant Brandon Malcolm of the Ontario Provincial Police",
			since:     day(2026, time.April, 29, false),
			until:     nil,
			createdAt: time.Date(2026, time.April, 29, 10, 0, 0, 0, time.UTC),
		},
	})

	active := filterActiveAt(notices, "ON", time.Date(2026, time.May, 8, 12, 0, 0, 0, time.UTC))
	if len(active) != 1 {
		t.Fatalf("active notices len = %d, want 1", len(active))
	}
	if active[0].until != nil {
		t.Fatalf("active notice until = %v, want nil", active[0].until)
	}
}

func TestUpdatedNoticeDoesNotSupersedeDifferentProvince(t *testing.T) {
	notices := resolveNoticeUpdates([]halfMastNotice{
		{
			province:  "ON",
			reason:    "Death of Sergeant Brandon Malcolm of the Ontario Provincial Police",
			since:     day(2026, time.April, 29, false),
			until:     day(2026, time.May, 6, true),
			createdAt: time.Date(2026, time.May, 1, 10, 0, 0, 0, time.UTC),
			updated:   true,
		},
		{
			province:  "BC",
			reason:    "Death of Sergeant Brandon Malcolm of the Ontario Provincial Police",
			since:     day(2026, time.April, 29, false),
			until:     nil,
			createdAt: time.Date(2026, time.April, 29, 10, 0, 0, 0, time.UTC),
		},
	})

	active := filterActiveAt(notices, "BC", time.Date(2026, time.May, 8, 12, 0, 0, 0, time.UTC))
	if len(active) != 1 {
		t.Fatalf("active notices len = %d, want 1", len(active))
	}
	if active[0].province != "BC" {
		t.Fatalf("active notice province = %q, want BC", active[0].province)
	}
}

func TestParseNoticesDetectsUpdatedRows(t *testing.T) {
	notices := parseNotices(`<tbody>
<tr>
  <td class="nws-tbl-desc"><span class="hidden">1777661357532</span>Updated <a href="#">Notice of half-masting: Death of Sergeant Brandon Malcolm of the Ontario Provincial Police</a></td>
  <td class="nws-tbl-desc">Masting period: From April 29, 2026, until sunset on May 6, 2026</td>
  <td class="nws-tbl-desc">Masting location(s): All Government of Canada buildings and establishments in the province of Ontario</td>
  <td class="nws-tbl-desc mrgn-bttm-md">Additional details:</td>
</tr>
</tbody>`)
	if len(notices) != 1 {
		t.Fatalf("notices len = %d, want 1", len(notices))
	}
	if !notices[0].updated {
		t.Fatal("updated = false, want true")
	}
	wantReason := "Death of Sergeant Brandon Malcolm of the Ontario Provincial Police"
	if notices[0].reason != wantReason {
		t.Fatalf("reason = %q, want %q", notices[0].reason, wantReason)
	}
}

func TestParsePeriodUsesExplicitClockWindow(t *testing.T) {
	loc := mustLocation(t, "America/Toronto")
	since, until := parsePeriodAtLocation(
		"Masting period: From 08:00 to 11:30 a.m. on June 1, 2026",
		loc,
		43.4516,
		-80.4925,
	)
	wantSince := time.Date(2026, time.June, 1, 8, 0, 0, 0, loc)
	wantUntil := time.Date(2026, time.June, 1, 11, 30, 0, 0, loc)
	if since == nil || !since.Equal(wantSince) {
		t.Fatalf("since = %v, want %v", since, wantSince)
	}
	if until == nil || !until.Equal(wantUntil) {
		t.Fatalf("until = %v, want %v", until, wantUntil)
	}
}

func TestParsePeriodUsesOrdinalDate(t *testing.T) {
	loc := mustLocation(t, "America/Toronto")
	since, until := parsePeriodAtLocation(
		"Masting period: From 08:00 to 11:30 a.m. on June 1st, 2026",
		loc,
		43.4516,
		-80.4925,
	)
	wantSince := time.Date(2026, time.June, 1, 8, 0, 0, 0, loc)
	wantUntil := time.Date(2026, time.June, 1, 11, 30, 0, 0, loc)
	if since == nil || !since.Equal(wantSince) {
		t.Fatalf("since = %v, want %v", since, wantSince)
	}
	if until == nil || !until.Equal(wantUntil) {
		t.Fatalf("until = %v, want %v", until, wantUntil)
	}
}

func TestUnparseableNoticeIsNotActive(t *testing.T) {
	active := filterActiveAt([]halfMastNotice{{
		province:  "NATIONAL",
		reason:    "Unknown period",
		createdAt: time.Date(2026, time.May, 19, 12, 0, 0, 0, time.UTC),
	}}, "ON", time.Date(2026, time.May, 19, 12, 0, 0, 0, time.UTC))
	if len(active) != 0 {
		t.Fatalf("active len = %d, want 0", len(active))
	}
}

func TestParsePeriodUsesTwentyFourHourClockWindow(t *testing.T) {
	loc := mustLocation(t, "America/Toronto")
	since, until := parsePeriodAtLocation(
		"Masting period: From 13:00 to 14:30 on June 1, 2026",
		loc,
		43.4516,
		-80.4925,
	)
	wantSince := time.Date(2026, time.June, 1, 13, 0, 0, 0, loc)
	wantUntil := time.Date(2026, time.June, 1, 14, 30, 0, 0, loc)
	if since == nil || !since.Equal(wantSince) {
		t.Fatalf("since = %v, want %v", since, wantSince)
	}
	if until == nil || !until.Equal(wantUntil) {
		t.Fatalf("until = %v, want %v", until, wantUntil)
	}
}

func TestTimeLimitedNoticeOnlyActiveInsideClockWindow(t *testing.T) {
	loc := mustLocation(t, "America/Toronto")
	since, until := parsePeriodAtLocation(
		"Masting period: From 08:00 to 11:30 a.m. on June 1, 2026",
		loc,
		43.4516,
		-80.4925,
	)
	notices := []halfMastNotice{{
		province:  "ON",
		reason:    "Test notice",
		since:     since,
		until:     until,
		createdAt: time.Date(2026, time.May, 19, 12, 0, 0, 0, loc),
	}}

	before := filterActiveAt(notices, "ON", time.Date(2026, time.June, 1, 7, 59, 0, 0, loc))
	if len(before) != 0 {
		t.Fatalf("before window active len = %d, want 0", len(before))
	}
	during := filterActiveAt(notices, "ON", time.Date(2026, time.June, 1, 8, 30, 0, 0, loc))
	if len(during) != 1 {
		t.Fatalf("during window active len = %d, want 1", len(during))
	}
	after := filterActiveAt(notices, "ON", time.Date(2026, time.June, 1, 11, 31, 0, 0, loc))
	if len(after) != 0 {
		t.Fatalf("after window active len = %d, want 0", len(after))
	}
}

func TestParsePeriodUsesDateStartAndSunsetEnd(t *testing.T) {
	loc := mustLocation(t, "America/Toronto")
	since, until := parsePeriodAtLocation(
		"Masting period: From April 29, 2026, until sunset on May 6, 2026",
		loc,
		43.4516,
		-80.4925,
	)
	wantSince := time.Date(2026, time.April, 29, 0, 1, 0, 0, loc)
	if since == nil || !since.Equal(wantSince) {
		t.Fatalf("since = %v, want %v", since, wantSince)
	}
	if until == nil {
		t.Fatal("until = nil, want sunset")
	}
	localUntil := until.In(loc)
	if y, m, d := localUntil.Date(); y != 2026 || m != time.May || d != 6 {
		t.Fatalf("until date = %s, want 2026-05-06", localUntil.Format("2006-01-02 15:04"))
	}
	if localUntil.Hour() != 20 || localUntil.Minute() < 20 || localUntil.Minute() > 50 {
		t.Fatalf("until = %s, want Kitchener sunset around 20:20-20:50", localUntil.Format("15:04"))
	}
}

func TestParsePeriodUsesPublishedTimeForFromNow(t *testing.T) {
	loc := mustLocation(t, "America/Toronto")
	published := time.Date(2025, time.February, 19, 14, 45, 0, 0, time.UTC)
	since, until := parsePeriodAtLocationWithPublished(
		"Masting period: From now until sunset on February 21, 2025.",
		loc,
		43.4516,
		-80.4925,
		published,
	)
	wantSince := published.In(loc)
	if since == nil || !since.Equal(wantSince) {
		t.Fatalf("since = %v, want %v", since, wantSince)
	}
	if until == nil {
		t.Fatal("until = nil, want sunset")
	}
	localUntil := until.In(loc)
	if y, m, d := localUntil.Date(); y != 2025 || m != time.February || d != 21 {
		t.Fatalf("until date = %s, want 2025-02-21", localUntil.Format("2006-01-02 15:04"))
	}
	if localUntil.Hour() != 18 && localUntil.Hour() != 17 {
		t.Fatalf("until = %s, want a local sunset time", localUntil.Format("15:04"))
	}
}

func day(year int, month time.Month, day int, endOfDay bool) *time.Time {
	h, min, sec := 0, 0, 0
	if endOfDay {
		h, min, sec = 23, 59, 59
	}
	t := time.Date(year, month, day, h, min, sec, 0, time.UTC)
	return &t
}

func mustLocation(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Fatalf("load location %q: %v", name, err)
	}
	return loc
}
