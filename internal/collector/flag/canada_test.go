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

func day(year int, month time.Month, day int, endOfDay bool) *time.Time {
	h, min, sec := 0, 0, 0
	if endOfDay {
		h, min, sec = 23, 59, 59
	}
	t := time.Date(year, month, day, h, min, sec, 0, time.UTC)
	return &t
}
