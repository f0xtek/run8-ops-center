package web

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"testing"

	"run8-ops-center/internal/engine"
	"run8-ops-center/internal/profile"
	"run8-ops-center/internal/run8save"
	"run8-ops-center/internal/trackmap"
)

const vwTwoBay = profile.CarType("Two Bay Covered Hopper")

// vwLabelFor resolves a car to its label the way the track-mapping spec says: first
// truck, then second. It works from the raw snapshot, not carstatus.
func vwLabelFor(tm *trackmap.Map, v run8save.Vehicle) (trackmap.Label, int, bool) {
	for _, tr := range v.Trucks {
		if l, ok := tm.Lookup(tr.RoutePrefix, tr.Section); ok {
			return l, tr.Section, true
		}
	}
	return trackmap.Label{}, 0, false
}

func vwHoppersAtHTI(snap *run8save.Snapshot, prof *profile.Profile, tm *trackmap.Map) int {
	n := 0
	for _, c := range snap.Cars() {
		if prof.CarTypeOf(c.Model) != vwTwoBay {
			continue
		}
		if l, _, ok := vwLabelFor(tm, c); ok && l.Kind == trackmap.KindIndustry && len(l.Industries) == 1 && l.Industries[0] == "HTI" {
			n++
		}
	}
	return n
}

func TestYardsHTIOccupancyAgainstCapacity(t *testing.T) {
	b := vwNewBackend(t, vwLabelledMap())
	h := vwServer(t, b)

	want := vwHoppersAtHTI(b.snap, b.base, b.tm)
	capacity, ok := b.base.IndustryByTag("HTI").Capacity(vwTwoBay)
	if !ok || capacity != 24 || want != 10 {
		t.Fatalf("fixture drifted: HTI capacity %d (%v), hoppers %d; the spec example is 10 of 24", capacity, ok, want)
	}

	rec := vwGet(t, h, "/yards")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	body := rec.Body.String()
	job := vwBetween(t, body, `id="job-ab45"`, `id="job-`)
	hti := vwBetween(t, job, `id="ind-HTI"`, `id="ind-`)
	text := vwText(hti)
	if wantText := fmt.Sprintf("Two Bay Covered Hopper %d/%d", want, capacity); !strings.Contains(text, wantText) {
		t.Fatalf("HTI block lacks %q:\n%s", wantText, text)
	}
	// Every hopper listed on HTI's table is a car at HTI: the cars table lists them by number.
	listed := 0
	for _, c := range b.snap.Cars() {
		if b.base.CarTypeOf(c.Model) == vwTwoBay && strings.Contains(text, c.UnitNumber) {
			listed++
		}
	}
	if listed < want {
		t.Errorf("HTI table lists %d of the %d hoppers parked there", listed, want)
	}
	// Cars parked at HTI that the industry has no capacity for are shown as such.
	if !strings.Contains(text, "not handled here") {
		t.Errorf("types at HTI outside its capacity list are not flagged:\n%s", text)
	}
}

func TestYardsOccupancyUpdatesWithLabels(t *testing.T) {
	b := vwNewBackend(t, trackmap.New())
	h := vwServer(t, b)
	text := func() string {
		body := vwGet(t, h, "/yards").Body.String()
		return vwText(vwBetween(t, body, `id="ind-HTI"`, `id="ind-`))
	}
	if got := text(); !strings.Contains(got, "Two Bay Covered Hopper 0/24") {
		t.Fatalf("with no labels HTI should be empty:\n%s", got)
	}
	for _, sec := range vwHTISections {
		b.tm.Set(280, sec, trackmap.Label{Kind: trackmap.KindIndustry, Industries: []string{"HTI"}})
	}
	if got := text(); !strings.Contains(got, "Two Bay Covered Hopper 10/24") {
		t.Fatalf("after labelling HTI should hold 10:\n%s", got)
	}
}

func TestYardsListsEveryYardAndTheirTracks(t *testing.T) {
	b := vwNewBackend(t, vwLabelledMap())
	b.tm.Set(280, 99999, trackmap.Label{Kind: trackmap.KindYard, Yard: "NOR", Track: "Bowl 20"}) // no cars here
	h := vwServer(t, b)
	body := vwGet(t, h, "/yards").Body.String()

	for _, y := range b.base.Yards {
		sec := vwBetween(t, body, fmt.Sprintf(`id="yard-%s"`, y.Code), `id="yard-`)
		if !strings.Contains(vwText(sec), y.Name) {
			t.Errorf("yard %s block does not name %q", y.Code, y.Name)
		}
	}
	nor := vwBetween(t, body, `id="yard-NOR"`, `id="yard-TUS"`)
	text := vwText(nor)
	// Tracks sort naturally: Bowl 2, Bowl 10, Bowl 14, Bowl 20.
	last := -1
	for _, name := range []string{"Bowl 2", "Bowl 10", "Bowl 14", "Bowl 20"} {
		i := strings.Index(text, name+" ")
		if i < 0 || i < last {
			t.Fatalf("track %q missing or out of order (at %d, previous %d) in:\n%s", name, i, last, text)
		}
		last = i
	}
	// Car counts per track come from the raw snapshot.
	for track, sec := range map[string]int{"Bowl 2": 3452, "Bowl 14": 3451, "Bowl 10": 3426} {
		n := 0
		for _, c := range b.snap.Cars() {
			if _, s, ok := vwLabelFor(b.tm, c); ok && s == sec {
				n++
			}
		}
		if n == 0 {
			t.Fatalf("fixture has no cars on %d", sec)
		}
		if want := fmt.Sprintf("%s %d car", track, n); !strings.Contains(text, want) {
			t.Errorf("want %q in NOR block:\n%s", want, text)
		}
	}
	if !strings.Contains(text, "Bowl 20 0 cars Empty.") {
		t.Errorf("a labelled track with no cars should read Empty:\n%s", text)
	}
	tus := vwText(vwBetween(t, body, `id="yard-TUS"`, `id="yard-MCC"`))
	if !strings.Contains(tus, "No track in TUS is labelled yet") {
		t.Errorf("a yard with no labelled tracks should say so:\n%s", tus)
	}
}

func TestYardsIndustriesGroupedByServingJob(t *testing.T) {
	b := vwNewBackend(t, vwLabelledMap())
	h := vwServer(t, b)
	body := vwGet(t, h, "/yards").Body.String()

	if n := strings.Count(body, `id="ind-`); n != len(b.base.Industries) {
		t.Errorf("%d industry blocks, profile has %d industries", n, len(b.base.Industries))
	}
	for _, j := range b.base.LocalJobs {
		sec := vwBetween(t, body, fmt.Sprintf(`id="job-%s"`, j.ID), `id="job-`)
		for _, tag := range j.Industries {
			if !strings.Contains(sec, fmt.Sprintf(`id="ind-%s"`, tag)) {
				t.Errorf("job %s section lacks industry %s", j.Symbol, tag)
			}
		}
	}
	// MB (Mercedes Benz) is served by no local job.
	rest := vwBetween(t, body, `id="job-other"`, `</section>`)
	if !strings.Contains(rest, `id="ind-MB"`) || !strings.Contains(vwText(rest), "Not served by a local job") {
		t.Errorf("unserved industry MB is not under the unserved group:\n%s", vwText(rest))
	}
}

func TestYardsServiceStatesAtIndustry(t *testing.T) {
	b := vwNewBackend(t, vwLabelledMap())
	h := vwServer(t, b)
	body := vwGet(t, h, "/yards").Body.String()
	hti := vwBetween(t, body, `id="ind-HTI"`, `id="ind-`)
	// Every car parked at HTI is tagged for CHA or similar, so none is "working" and all can be pulled.
	if !regexp.MustCompile(`ready to pull <b class="mono">\d+</b>`).MatchString(hti) {
		t.Errorf("HTI shows no ready-to-pull count:\n%s", vwText(hti))
	}
	if strings.Contains(vwText(hti), "working") {
		t.Errorf("no car at HTI is tagged for HTI, yet one is marked working")
	}
}

// vwNoSnapshot is a backend that has not ingested a save yet.
type vwNoSnapshot struct{ *vwBackend }

func (n vwNoSnapshot) View() *engine.View {
	v := n.vwBackend.View()
	return &engine.View{Profile: v.Profile, Profiles: v.Profiles, Settings: v.Settings, Source: v.Source}
}

func TestYardsBeforeFirstSnapshot(t *testing.T) {
	b := vwNewBackend(t, trackmap.New())
	s, err := New(Config{}, b)
	if err != nil {
		t.Fatal(err)
	}
	RegisterYards(s, vwNoSnapshot{b})
	rec := vwGet(t, s.Handler(), "/yards")
	if rec.Code != http.StatusOK || !strings.Contains(vwText(rec.Body.String()), "No save has been ingested yet") {
		t.Fatalf("status %d:\n%s", rec.Code, vwText(rec.Body.String()))
	}
}

// vwEmpty is a backend with an empty View: no profile, no snapshot.
type vwEmpty struct{ *vwBackend }

func (vwEmpty) View() *engine.View { return &engine.View{} }
