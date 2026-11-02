package web

import (
	"math"
	"strings"
	"testing"

	"run8-ops-center/internal/run8route/run8routetest"
	"run8-ops-center/internal/trackmap"
)

// vwListedBlock is the text of one yard's "Listed tracks" table.
func vwListedBlock(t *testing.T, body, code string) string {
	t.Helper()
	yard := vwBetween(t, body, `id="yard-`+code+`"`, `id="yard-`)
	return vwText(vwBetween(t, yard, `<details class="yards-listed"`, `</details>`))
}

func TestYardsListedTracksShowLabelledAndNotLabelledYet(t *testing.T) {
	tm := trackmap.New()
	for _, s := range []int{run8routetest.SecRcv1North, run8routetest.SecRcv1Mid, run8routetest.SecRcv1South} {
		tm.Set(280, s, trackmap.Label{Kind: trackmap.KindYard, Yard: "TUS", Track: "TY08"})
	}
	b := vwNewBackend(t, tm)
	b.useInstall(t, run8routetest.Synthetic(), vwCutSnapshot("T1", "AB45 BRL", run8routetest.SecRcv1Mid, run8routetest.SecRcv1Mid))
	h := vwServer(t, b)

	// The run is 300, 301 and 302: measure it independently of the view.
	topo := b.imp.data().Topology
	metres := 0.0
	for _, s := range []int{300, 301, 302} {
		sec, _ := topo.Section(s)
		metres += sec.LengthM
	}
	measured := int(math.Round(metres * 3.28084))
	if measured < 500 {
		t.Fatalf("fixture: run measures %d ft", measured)
	}

	text := vwListedBlock(t, vwGet(t, h, "/yards").Body.String(), "TUS")
	if !strings.Contains(text, "Listed tracks: 1 of 12 labelled") {
		t.Errorf("summary missing:\n%s", text)
	}
	// Labelled: the listed length beside the measured one, and the cars on it.
	if want := "TY08 710 ft " + ptGroup(measured) + " ft 2 cars"; !strings.Contains(text, want) {
		t.Errorf("no %q in:\n%s", want, text)
	}
	// The TY06 scenario: listed, not labelled.
	if want := "TY06 543 ft – not labelled yet"; !strings.Contains(text, want) {
		t.Errorf("no %q in:\n%s", want, text)
	}
	if got := strings.Count(text, "not labelled yet"); got != 11 {
		t.Errorf("%d tracks not labelled yet, want 11:\n%s", got, text)
	}
	// A siding is marked as such.
	if !strings.Contains(text, "Pass (siding) 752 ft – not labelled yet") {
		t.Errorf("siding row missing:\n%s", text)
	}
	// Every listed track is there, in profile order.
	last := -1
	for _, y := range b.base.Yards {
		if y.Code != "TUS" {
			continue
		}
		for _, lt := range y.Tracks {
			i := strings.Index(text, lt.Name)
			if i < 0 || i < last {
				t.Errorf("track %s at %d, after %d", lt.Name, i, last)
			}
			last = i
		}
	}
}

func TestYardsListedTracksWithoutRouteDataHaveNoMeasuredLength(t *testing.T) {
	tm := trackmap.New()
	tm.Set(280, 5101, trackmap.Label{Kind: trackmap.KindYard, Yard: "TUS", Track: "ty01"})
	b := vwNewBackend(t, tm)
	body := vwGet(t, vwServer(t, b), "/yards").Body.String()
	text := vwListedBlock(t, body, "TUS")
	// The label's case differs from the list's: it still counts as labelled.
	if !strings.Contains(text, "TY01 1,379 ft – 0 cars") {
		t.Errorf("TY01 row:\n%s", text)
	}
	if got := strings.Count(text, "not labelled yet"); got != 11 {
		t.Errorf("%d not labelled yet, want 11:\n%s", got, text)
	}
	// NOR lists tracks too; the 70 entries start closed, and the short TUS list starts open.
	norBlock := vwBetween(t, body, `id="yard-NOR"`, `id="yard-`)
	if !strings.Contains(norBlock, `class="yards-listed"`) {
		t.Error("NOR lists tracks in the bundled profile and should show the table")
	}
	// NOR lists 70 tracks, so its table starts closed.
	if strings.Contains(norBlock, `class="yards-listed" open`) {
		t.Error("the long NOR table should start closed")
	}
	if !strings.Contains(vwBetween(t, body, `id="yard-TUS"`, `id="yard-`), `class="yards-listed" open`) {
		t.Error("the short TUS table should start open")
	}
}
