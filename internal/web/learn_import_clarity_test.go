package web

import (
	"math"
	"net/url"
	"strings"
	"testing"

	"run8-ops-center/internal/carstatus"
	"run8-ops-center/internal/engine"
	"run8-ops-center/internal/run8route/run8routetest"
	"run8-ops-center/internal/trackmap"
)

// importGroupMarkup returns one location's block from the preview page.
func importGroupMarkup(t *testing.T, body, name string) string {
	t.Helper()
	groups := vwBetween(t, body, `id="import-groups"`, `</section>`)
	i := strings.Index(groups, "<b>"+name+"</b>")
	if i < 0 {
		t.Fatalf("no location %q on the preview:\n%s", name, vwText(groups))
	}
	block := groups[i:]
	if j := strings.Index(block, `<details class="imp-group">`); j >= 0 {
		block = block[:j]
	}
	return block
}

// sectionFeet is the independent check: sections' lengths in feet, summed in metres first.
func sectionFeet(t *testing.T, b *vwBackend, sections ...int) string {
	t.Helper()
	m := 0.0
	for _, s := range sections {
		sec, ok := b.imp.data().Topology.Section(s)
		if !ok {
			t.Fatalf("no section %d", s)
		}
		m += sec.LengthM
	}
	return ptGroup(int(math.Round(m*3.28084))) + " ft"
}

func TestImportPreviewCountsTrackSectionsAndHidesIDs(t *testing.T) {
	_, h := importBackend(t)
	rec := vwPost(t, h, "/trackmap/import/preview", url.Values{})
	if rec.Code != 200 {
		t.Fatalf("status %d: %s", rec.Code, vwText(rec.Body.String()))
	}
	body := rec.Body.String()

	groups := vwBetween(t, body, `id="import-groups"`, `</section>`)
	if !strings.Contains(vwText(groups), "IDs of pieces of track, not cars") {
		t.Errorf("the Locations section does not say the IDs are track, not cars:\n%s", vwText(groups))
	}

	for name, want := range map[string]string{
		"NOR / Bowl 1": "1 track section, all new",
		"BRL":          "2 track sections, all new",
	} {
		block := importGroupMarkup(t, body, name)
		summary := block[:strings.Index(block, "</summary>")]
		if !strings.Contains(vwText(summary), want) {
			t.Errorf("%s summary lacks %q:\n%s", name, want, vwText(summary))
		}
		if strings.Contains(summary, "280/") {
			t.Errorf("%s summary shows a section ID:\n%s", name, summary)
		}
		ids := strings.Index(block, `<details class="imp-ids">`)
		if ids < 0 || !strings.Contains(block[ids:], "<summary>Show section IDs</summary>") {
			t.Fatalf("%s has no Show section IDs toggle:\n%s", name, block)
		}
		if !strings.Contains(block[ids:], "<li>280/") {
			t.Errorf("%s lists no section ID inside the toggle:\n%s", name, block[ids:])
		}
		if strings.Contains(block[:ids], "280/") {
			t.Errorf("%s shows a section ID outside the toggle:\n%s", name, block[:ids])
		}
	}

	// The old "4 new" wording is gone from the location summaries.
	if strings.Contains(vwText(groups), "1 new") || strings.Contains(vwText(groups), "2 new") {
		t.Errorf("locations still count new sections as bare numbers:\n%s", vwText(groups))
	}

	// The counts line and the confirm line name sections, not cars.
	text := vwText(body)
	for _, want := range []string{"A track section is a piece of track, not a car.", "No car is touched."} {
		if !strings.Contains(text, want) {
			t.Errorf("preview lacks %q", want)
		}
	}
}

func TestSectionSummaryNamesTheSplit(t *testing.T) {
	e := func(n int) []importEntry { return make([]importEntry, n) }
	for _, c := range []struct {
		g    importGroup
		want string
	}{
		{importGroup{New: e(4)}, "4 track sections, all new"},
		{importGroup{New: e(1)}, "1 track section, all new"},
		{importGroup{Changed: e(2)}, "2 track sections, all changed"},
		{importGroup{Removed: e(1)}, "1 track section, all removed"},
		{importGroup{New: e(3), Changed: e(1)}, "4 track sections (3 new, 1 changed)"},
		{importGroup{New: e(1), Changed: e(1), Removed: e(2)}, "4 track sections (1 new, 1 changed, 2 removed)"},
	} {
		if got := sectionSummary(c.g); got != c.want {
			t.Errorf("sectionSummary = %q, want %q", got, c.want)
		}
	}
}

func TestImportPreviewShowsLengthAndCars(t *testing.T) {
	b, h := importBackend(t)
	rec := vwPost(t, h, "/trackmap/import/preview", url.Values{})
	if rec.Code != 200 {
		t.Fatalf("status %d: %s", rec.Code, vwText(rec.Body.String()))
	}
	body := rec.Body.String()
	num := func(unit string) string { return carstatus.Number("R8_Boxcar_X.xml", unit) }

	// BRL is sections 105 and 106, with cars 1000 and 1001 on 105 and 1002 on 106.
	brl := vwText(importGroupMarkup(t, body, "BRL"))
	for _, want := range []string{
		"Length of these sections: " + sectionFeet(t, b, run8routetest.SecSiding1, run8routetest.SecSiding2),
		"3 cars there now: " + num("1000") + ", " + num("1001") + ", " + num("1002"),
	} {
		if !strings.Contains(brl, want) {
			t.Errorf("BRL lacks %q:\n%s", want, brl)
		}
	}

	// Bowl 1 has one section and nothing on it.
	bowl := vwText(importGroupMarkup(t, body, "NOR / Bowl 1"))
	for _, want := range []string{"Length of these sections: " + sectionFeet(t, b, run8routetest.SecBowl1), "No cars there now"} {
		if !strings.Contains(bowl, want) {
			t.Errorf("Bowl 1 lacks %q:\n%s", want, bowl)
		}
	}
}

func TestImportPreviewSamplesFiveCars(t *testing.T) {
	b := vwNewBackend(t, trackmap.New())
	sections := []int{}
	for range 7 {
		sections = append(sections, run8routetest.SecSiding1)
	}
	b.useInstall(t, run8routetest.Synthetic(), vwCutSnapshot("T1", "AB45 BRL", sections...))
	rec := vwPost(t, vwServer(t, b), "/trackmap/import/preview", url.Values{})
	if rec.Code != 200 {
		t.Fatalf("status %d: %s", rec.Code, vwText(rec.Body.String()))
	}
	brl := vwText(importGroupMarkup(t, rec.Body.String(), "BRL"))
	var nums []string
	for i := range 5 {
		nums = append(nums, carstatus.Number("R8_Boxcar_X.xml", "100"+string(rune('0'+i))))
	}
	if want := "7 cars there now: " + strings.Join(nums, ", ") + " and 2 more"; !strings.Contains(brl, want) {
		t.Errorf("BRL lacks %q:\n%s", want, brl)
	}
}

func TestImportPreviewWithoutASaveSaysNothingAboutCars(t *testing.T) {
	b := vwNewBackend(t, trackmap.New())
	b.useInstall(t, run8routetest.Synthetic(), nil)
	b.snap = nil // the fake starts with an empty save; this one has none at all
	rec := vwPost(t, vwServer(t, b), "/trackmap/import/preview", url.Values{})
	if rec.Code != 200 {
		t.Fatalf("status %d: %s", rec.Code, vwText(rec.Body.String()))
	}
	bowl := vwText(importGroupMarkup(t, rec.Body.String(), "NOR / Bowl 1"))
	if strings.Contains(bowl, "cars there now") || strings.Contains(bowl, "No cars") {
		t.Errorf("Bowl 1 claims something about cars without a save:\n%s", bowl)
	}
	if !strings.Contains(bowl, "Length of these sections:") {
		t.Errorf("Bowl 1 lost its length:\n%s", bowl)
	}
}

func TestImportBuildOmitsLengthWithoutRouteData(t *testing.T) {
	bowl := trackmap.Label{Kind: trackmap.KindYard, Yard: "NOR", Track: "Bowl 1"}
	p := engine.ImportPreview{
		Diff: trackmap.ImportDiff{New: []trackmap.DiffEntry{{Key: "280/206", New: &bowl}}},
	}
	if g := importBuild(p).Groups[0]; g.Length != "" || g.Cars != "" {
		t.Errorf("no route data and no save: length %q, cars %q", g.Length, g.Cars)
	}
	p.Locations = map[string]engine.ImportLocation{bowl.Location().Key(): {LengthFt: 1217}}
	p.CarsKnown = true
	if g := importBuild(p).Groups[0]; g.Length != "1,217 ft" || g.Cars != "No cars there now" {
		t.Errorf("length %q, cars %q", g.Length, g.Cars)
	}
}
