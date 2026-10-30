package web

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"run8-ops-center/internal/store"
	"run8-ops-center/internal/trackmap"
)

func vwEntry(minutes int, kind, msg string) store.ActivityEntry {
	base := time.Date(2026, 10, 30, 18, 0, 0, 0, time.Local)
	return store.ActivityEntry{
		Time: base.Add(time.Duration(minutes) * time.Minute), Game: time.Date(1985, 3, 14, 6, minutes%60, 0, 0, time.UTC),
		Kind: kind, Message: msg,
	}
}

func TestActivityNewestFirst(t *testing.T) {
	b := vwNewBackend(t, trackmap.New())
	// Handed over oldest-first on purpose: the page must still list the newest first.
	b.activity = []store.ActivityEntry{
		vwEntry(1, "snapshot", "first change"), vwEntry(5, "order", "Started AB10"), vwEntry(3, "snapshot", "middle change"),
	}
	rec := vwGet(t, vwServer(t, b), "/activity")
	text := vwText(rec.Body.String())
	i5, i3, i1 := strings.Index(text, "Started AB10"), strings.Index(text, "middle change"), strings.Index(text, "first change")
	if i5 < 0 || !(i5 < i3 && i3 < i1) {
		t.Fatalf("entries are not newest first (%d, %d, %d):\n%s", i5, i3, i1, text)
	}
	// Each row carries its time, game date and kind.
	for _, want := range []string{"2026-10-30 18:05:00", "2026-10-30 18:03:00", "1985-03-14 06:05", "order", "snapshot"} {
		if !strings.Contains(text, want) {
			t.Errorf("page lacks %q:\n%s", want, text)
		}
	}
	if !strings.Contains(rec.Body.String(), `aria-current="page"`) {
		t.Error("the activity tab is not marked current")
	}
}

func TestActivityCapsAndExplainsLongLogs(t *testing.T) {
	b := vwNewBackend(t, trackmap.New())
	for i := 0; i < activityShown+40; i++ {
		b.activity = append(b.activity, vwEntry(i, "snapshot", fmt.Sprintf("entry %04d", i)))
	}
	body := vwGet(t, vwServer(t, b), "/activity").Body.String()
	if got := strings.Count(body, `activity-t"><time `); got != activityShown {
		t.Errorf("%d rows, want %d", got, activityShown)
	}
	last := fmt.Sprintf("entry %04d", activityShown+39)
	if !strings.Contains(body, last) || strings.Contains(body, "entry 0000") {
		t.Error("the cap must keep the newest entries and drop the oldest")
	}
	if !strings.Contains(vwText(body), fmt.Sprintf("Showing the latest %d of %d", activityShown, activityShown+40)) {
		t.Error("the cap is not explained")
	}
}

func TestActivityEmptyAndEscaped(t *testing.T) {
	b := vwNewBackend(t, trackmap.New())
	h := vwServer(t, b)
	if text := vwText(vwGet(t, h, "/activity").Body.String()); !strings.Contains(text, "Nothing has happened yet") {
		t.Errorf("empty state:\n%s", text)
	}
	b.activity = []store.ActivityEntry{vwEntry(1, "order", `Started <b>AB10</b> & "co"`)}
	body := vwGet(t, h, "/activity").Body.String()
	if strings.Contains(body, "<b>AB10</b>") || !strings.Contains(body, "&lt;b&gt;AB10&lt;/b&gt;") {
		t.Errorf("activity message is not escaped")
	}
}
