package trackmap

import (
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"run8-ops-center/internal/run8route"
	"run8-ops-center/internal/run8route/run8routetest"
)

func TestBuildImportIndustries(t *testing.T) {
	res := synthRoute(t)
	plan := BuildImport(res.Data, synthProfile())
	if len(plan.Unavailable) != 0 {
		t.Fatalf("unavailable: %v", plan.Unavailable)
	}
	for k, l := range plan.Labels {
		if l.Source != SourceRun8 {
			t.Errorf("%s: source %q", k, l.Source)
		}
	}

	// The multi-section spur.
	for _, s := range []int{run8routetest.SecSiding1, run8routetest.SecSiding2} {
		if l := plan.Labels[sec(s)]; l.Kind != KindIndustry || !reflect.DeepEqual(l.Industries, []string{"BRL"}) {
			t.Errorf("section %d = %+v", s, l)
		}
	}
	// " BBB" is labelled as BBB.
	if l := plan.Labels[sec(run8routetest.SecPaddedSpur)]; !reflect.DeepEqual(l.Industries, []string{"BBB"}) {
		t.Errorf("padded spur = %+v", l)
	}
	// The industry the profile does not know is skipped with a reason, and its section is not labelled.
	if _, ok := plan.Labels[sec(run8routetest.SecUnknownSpur)]; ok {
		t.Error("unknown industry labelled")
	}
	s, ok := skipFor(plan.Skipped, "industry Unknown Industry (ZZZ)")
	if !ok || !strings.Contains(s.Reason, "ZZZ") || !strings.Contains(s.Reason, "profile") {
		t.Errorf("skipped = %+v", plan.Skipped)
	}
}

func TestBuildImportSectionListedByTwoIndustries(t *testing.T) {
	res := synthRoute(t)
	d := res.Data
	d.Industries = append(slices.Clone(d.Industries), run8route.Industry{
		Name: "Second Tenant", Tag: " BBB",
		Tracks: []run8route.IndustryTrack{{Prefix: run8routetest.Prefix, Section: run8routetest.SecSiding1}},
	})
	plan := BuildImport(d, synthProfile())
	l := plan.Labels[sec(run8routetest.SecSiding1)]
	if l.Kind != KindIndustry || !reflect.DeepEqual(l.Industries, []string{"BBB", "BRL"}) {
		t.Fatalf("shared section = %+v", l)
	}
	if l := plan.Labels[sec(run8routetest.SecSiding2)]; !reflect.DeepEqual(l.Industries, []string{"BRL"}) {
		t.Errorf("the other spur section = %+v", l)
	}
	if _, ok := skipFor(plan.Skipped, "industry Unknown Industry (ZZZ)"); !ok {
		t.Errorf("skipped = %+v", plan.Skipped)
	}
}

func TestBuildImportBowl(t *testing.T) {
	res := synthRoute(t)
	plan := BuildImport(res.Data, synthProfile())
	want := map[string][]int{
		"Bowl 1": {run8routetest.SecBowl1},
		"Bowl 2": {run8routetest.SecBowl2a, run8routetest.SecBowl2b}, // 207, 210
		"Bowl 3": {run8routetest.SecBowl3},
		"Bowl 4": {run8routetest.SecBowl4},
	}
	total := 0
	for name, secs := range want {
		got := sectionsWhere(plan.Labels, trackIs(name))
		if !reflect.DeepEqual(got, secs) {
			t.Errorf("%s sections = %v, want %v", name, got, secs)
		}
		for _, s := range secs {
			if l := plan.Labels[sec(s)]; l.Yard != "NOR" || l.Source != SourceRun8 {
				t.Errorf("section %d = %+v", s, l)
			}
		}
		total += len(secs)
	}
	// No switch and no lead section is labelled, and no section is labelled twice.
	got := sectionsWhere(plan.Labels, func(l Label) bool { return l.Kind == KindYard && strings.HasPrefix(l.Track, "Bowl") })
	if len(got) != total {
		t.Errorf("bowl sections = %v", got)
	}
	for _, s := range []int{200, 201, 202, 203, 204, 205} {
		if _, ok := plan.Labels[sec(s)]; ok {
			t.Errorf("lead section %d labelled", s)
		}
	}
	for _, sk := range plan.Skipped {
		if strings.Contains(sk.What, "Bowl") {
			t.Errorf("bowl skipped: %+v", sk)
		}
	}
}

func TestBuildImportBowlSectionClaimedTwice(t *testing.T) {
	res := synthRoute(t)
	d := res.Data
	hc := d.Hump[0]
	hc.Tracks = append(slices.Clone(hc.Tracks), run8route.HumpTrack{Name: "5", Switches: slices.Clone(hc.Tracks[0].Switches)})
	d.Hump = []run8route.HumpController{hc}
	plan := BuildImport(d, synthProfile())

	if _, ok := plan.Labels[sec(run8routetest.SecBowl1)]; ok {
		t.Error("the doubly claimed section is labelled")
	}
	s, ok := skipFor(plan.Skipped, "section "+sec(run8routetest.SecBowl1))
	if !ok || !strings.Contains(s.Reason, "NOR / Bowl 1") || !strings.Contains(s.Reason, "NOR / Bowl 5") {
		t.Errorf("skipped = %+v", plan.Skipped)
	}
	// The other tracks are untouched.
	if got := sectionsWhere(plan.Labels, trackIs("Bowl 2")); !reflect.DeepEqual(got, []int{207, 210}) {
		t.Errorf("Bowl 2 = %v", got)
	}
}

func TestBuildImportBowlSkips(t *testing.T) {
	res := synthRoute(t)
	d := res.Data
	hc := run8route.HumpController{Name: run8routetest.HumpControllerName, Tracks: []run8route.HumpTrack{
		{Name: "1"}, // no switches
		{Name: "2", Switches: []run8route.HumpSwitch{{Section: 202}}},                  // not a switch
		{Name: "3", Switches: []run8route.HumpSwitch{{Section: 201, Reversed: false}}}, // leg is a plain section: fine
		{Name: "4", Switches: []run8route.HumpSwitch{{Section: 9999}}},                 // unknown
	}}
	d.Hump = []run8route.HumpController{hc, {Name: "Other Hump"}}
	plan := BuildImport(d, synthProfile())
	for _, name := range []string{"NOR / Bowl 1", "NOR / Bowl 2", "NOR / Bowl 4", "hump controller Other Hump"} {
		if _, ok := skipFor(plan.Skipped, name); !ok {
			t.Errorf("no skip for %s in %+v", name, plan.Skipped)
		}
	}
	// 201's normal leg is 202, a plain section whose track runs to the next switch.
	if got := sectionsWhere(plan.Labels, trackIs("Bowl 3")); !reflect.DeepEqual(got, []int{202}) {
		t.Errorf("Bowl 3 = %v", got)
	}
}

func TestBuildImportReceiving(t *testing.T) {
	res := synthRoute(t)
	plan := BuildImport(res.Data, synthProfile())

	// "South End" and "South end" both match although the pattern is written case-sensitively.
	if got := sectionsWhere(plan.Labels, trackIs("Receiving 1")); !reflect.DeepEqual(got, []int{300, 301, 302}) {
		t.Errorf("Receiving 1 = %v", got)
	}
	if got := sectionsWhere(plan.Labels, trackIs("Receiving 2")); !reflect.DeepEqual(got, []int{310, 311}) {
		t.Errorf("Receiving 2 = %v", got)
	}
	if l := plan.Labels[sec(301)]; l.Kind != KindYard || l.Yard != "NOR" || l.Source != SourceRun8 {
		t.Errorf("301 = %+v", l)
	}
	for _, s := range []int{run8routetest.SecRcv3North, run8routetest.SecRcv4North, run8routetest.SecRcv4Cross, run8routetest.SecRcv4South, run8routetest.SecLadderN, run8routetest.SecLadderS} {
		if l, ok := plan.Labels[sec(s)]; ok {
			t.Errorf("section %d labelled %+v", s, l)
		}
	}

	s3, ok := skipFor(plan.Skipped, "NOR / Receiving 3")
	if !ok || !strings.Contains(s3.Reason, "no south end") {
		t.Errorf("Receiving 3 skip = %+v (all %+v)", s3, plan.Skipped)
	}
	s4, ok := skipFor(plan.Skipped, "NOR / Receiving 4")
	if !ok || !strings.Contains(s4.Reason, "crosses switch 331") {
		t.Errorf("Receiving 4 skip = %+v (all %+v)", s4, plan.Skipped)
	}
	// The hump lead matches no pattern and is not reported.
	for _, sk := range plan.Skipped {
		if strings.Contains(sk.What, "Hump Lead") {
			t.Errorf("unrelated location reported: %+v", sk)
		}
	}
}

func TestBuildImportReceivingUnreachableAndTooFar(t *testing.T) {
	res := synthRoute(t)
	d := res.Data
	// RCV 3's south end on a section no link reaches: unreachable.
	d.Locations = append(slices.Clone(d.Locations), run8route.SpecialLocation{
		Name: "Norris RCV 3 South end", Type: 1, Prefix: run8routetest.Prefix, Section: run8routetest.SecMainWestEnd,
	})
	plan := BuildImport(d, synthProfile())
	s, ok := skipFor(plan.Skipped, "NOR / Receiving 3")
	if !ok || !strings.Contains(s.Reason, "no path") {
		t.Errorf("Receiving 3 skip = %+v", s)
	}

	// A track longer than the limit: chain of 70 plain sections.
	var secs []run8routetest.TrackSection
	const first = 1000
	for i := range 70 {
		next := []int{}
		if i > 0 {
			next = append(next, first+i-1)
		}
		if i < 69 {
			next = append(next, first+i+1)
		}
		secs = append(secs, run8routetest.Place(first+i, next, false, float64(i*10), -100, float64(i*10+10), -100, run8routetest.TileW, run8routetest.TileH))
	}
	spec := run8routetest.Synthetic()
	spec.Tracks = append(spec.Tracks, secs...)
	spec.Locations = []run8routetest.SpecialLocation{
		{Name: "Norris RCV 9 North end", Type: 1, Prefix: run8routetest.Prefix, Section: first},
		{Name: "Norris RCV 9 South end", Type: 1, Prefix: run8routetest.Prefix, Section: first + 69},
		{Name: "Norris RCV 8 North end", Type: 1, Prefix: run8routetest.Prefix, Section: first},
		{Name: "Norris RCV 8 South end", Type: 1, Prefix: run8routetest.Prefix, Section: first + 59},
	}
	root := t.TempDir()
	run8routetest.WriteInstall(t, root, spec)
	long := run8route.Load(root, run8routetest.Region, []run8route.Route{{Folder: run8routetest.Route}})
	plan = BuildImport(long.Data, synthProfile())
	if s, ok := skipFor(plan.Skipped, "NOR / Receiving 9"); !ok || !strings.Contains(s.Reason, "60") {
		t.Errorf("70-section track skip = %+v (all %+v)", s, plan.Skipped)
	}
	if got := sectionsWhere(plan.Labels, trackIs("Receiving 8")); len(got) != 60 {
		t.Errorf("a 60-section track has %d sections", len(got))
	}
}

func TestBuildImportUnavailable(t *testing.T) {
	res := synthRoute(t)
	p := synthProfile()

	noTrack := res.Data
	noTrack.Topology = nil
	plan := BuildImport(noTrack, p)
	if len(plan.Unavailable) != 1 || !strings.Contains(plan.Unavailable[0], run8route.TrackDBFile) {
		t.Errorf("unavailable = %v", plan.Unavailable)
	}
	if len(plan.Labels) != 3 { // the industry labels still come
		t.Errorf("labels = %d", len(plan.Labels))
	}

	noInd := res.Data
	noInd.Industries = nil
	plan = BuildImport(noInd, p)
	if len(plan.Unavailable) != 1 || !strings.Contains(plan.Unavailable[0], run8route.ConfigFile) {
		t.Errorf("unavailable = %v", plan.Unavailable)
	}
	if got := sectionsWhere(plan.Labels, func(l Label) bool { return l.Kind == KindIndustry }); len(got) != 0 {
		t.Errorf("industry labels = %v", got)
	}

	noHump, noLoc := res.Data, res.Data
	noHump.Hump, noLoc.Locations = nil, nil
	if u := BuildImport(noHump, p).Unavailable; len(u) != 1 || !strings.Contains(u[0], run8route.HumpFile) {
		t.Errorf("unavailable = %v", u)
	}
	if u := BuildImport(noLoc, p).Unavailable; len(u) != 1 || !strings.Contains(u[0], run8route.LocationsFile) {
		t.Errorf("unavailable = %v", u)
	}

	noRun8 := synthProfile()
	noRun8.Run8 = nil
	plan = BuildImport(res.Data, noRun8)
	if len(plan.Unavailable) != 1 || len(plan.Labels) != 3 {
		t.Errorf("no run8 block: unavailable %v, labels %d", plan.Unavailable, len(plan.Labels))
	}
	if plan := BuildImport(res.Data, nil); len(plan.Labels) != 0 || len(plan.Unavailable) == 0 {
		t.Errorf("nil profile: %+v", plan)
	}
}

// ----- Diff and ApplyImport -----

var importTime = time.Date(2026, 10, 30, 14, 0, 0, 0, time.UTC)

func TestDiffManualLabelKept(t *testing.T) {
	res := synthRoute(t)
	plan := BuildImport(res.Data, synthProfile())
	m := New()
	manual := Label{Kind: KindYard, Yard: "NOR", Track: "Track 9"}
	m.Set(280, run8routetest.SecSiding1, manual)

	d := m.Diff(plan)
	if len(d.KeptManual) != 1 || d.KeptManual[0].Key != sec(run8routetest.SecSiding1) {
		t.Fatalf("kept manual = %+v", d.KeptManual)
	}
	e := d.KeptManual[0]
	if e.Old == nil || !reflect.DeepEqual(*e.Old, manual) || e.New == nil || e.New.Industries[0] != "BRL" {
		t.Errorf("entry = old %+v new %+v", e.Old, e.New)
	}
	if len(d.New) != len(plan.Labels)-1 || len(d.Changed)+len(d.Removed)+len(d.Unchanged) != 0 {
		t.Errorf("diff counts: new %d changed %d removed %d unchanged %d", len(d.New), len(d.Changed), len(d.Removed), len(d.Unchanged))
	}

	m.ApplyImport(d, res.Fingerprint, importTime)
	if l, _ := m.Lookup(280, run8routetest.SecSiding1); !reflect.DeepEqual(l, manual) {
		t.Errorf("manual label changed to %+v", l)
	}
	if l, _ := m.Lookup(280, run8routetest.SecSiding2); l.Source != SourceRun8 || l.Industries[0] != "BRL" {
		t.Errorf("new label = %+v", l)
	}
	if len(m.Labels) != len(plan.Labels) {
		t.Errorf("%d labels, want %d", len(m.Labels), len(plan.Labels))
	}
	if m.Run8Import == nil || !m.Run8Import.At.Equal(importTime) || !run8route.SameFingerprint(m.Run8Import.Files, res.Fingerprint) {
		t.Errorf("import meta = %+v", m.Run8Import)
	}
}

func TestReimportAfterIndustryMoves(t *testing.T) {
	res := synthRoute(t)
	p := synthProfile()
	m := New()
	m.ApplyImport(m.Diff(BuildImport(res.Data, p)), res.Fingerprint, importTime)

	// A second import of the same data changes nothing.
	again := m.Diff(BuildImport(res.Data, p))
	if len(again.New)+len(again.Changed)+len(again.Removed)+len(again.KeptManual) != 0 || len(again.Unchanged) == 0 {
		t.Fatalf("a repeat import differs: %+v", again)
	}

	// BRL now lists 106 and 103 instead of 105 and 106.
	moved := res.Data
	moved.Industries = slices.Clone(moved.Industries)
	moved.Industries[0].Tracks = []run8route.IndustryTrack{
		{Prefix: run8routetest.Prefix, Section: run8routetest.SecSiding2},
		{Prefix: run8routetest.Prefix, Section: run8routetest.SecMainEast},
	}
	d := m.Diff(BuildImport(moved, p))
	if len(d.Removed) != 1 || d.Removed[0].Key != sec(run8routetest.SecSiding1) || d.Removed[0].Old.Industries[0] != "BRL" || d.Removed[0].New != nil {
		t.Errorf("removed = %+v", d.Removed)
	}
	if len(d.New) != 1 || d.New[0].Key != sec(run8routetest.SecMainEast) || d.New[0].Old != nil {
		t.Errorf("new = %+v", d.New)
	}
	if len(d.Changed) != 0 {
		t.Errorf("changed = %+v", d.Changed)
	}

	m.ApplyImport(d, res.Fingerprint, importTime.Add(time.Hour))
	if _, ok := m.Lookup(280, run8routetest.SecSiding1); ok {
		t.Error("the moved section is still labelled")
	}
	if l, ok := m.Lookup(280, run8routetest.SecMainEast); !ok || l.Source != SourceRun8 {
		t.Errorf("the new section = %+v, %v", l, ok)
	}
	if !m.Run8Import.At.Equal(importTime.Add(time.Hour)) {
		t.Errorf("import time = %v", m.Run8Import.At)
	}
}

func TestDiffChangedLabel(t *testing.T) {
	res := synthRoute(t)
	p := synthProfile()
	m := New()
	m.ApplyImport(m.Diff(BuildImport(res.Data, p)), res.Fingerprint, importTime)

	// The profile renames the bowl prefix: every bowl label changes.
	p.Run8.Humps[0].TrackPrefix = "Hump"
	d := m.Diff(BuildImport(res.Data, p))
	if len(d.Changed) != 5 || len(d.New)+len(d.Removed) != 0 {
		t.Fatalf("changed %d, new %d, removed %d", len(d.Changed), len(d.New), len(d.Removed))
	}
	if e := d.Changed[0]; e.Old.Track != "Bowl 1" || e.New.Track != "Hump 1" {
		t.Errorf("first change = %+v -> %+v", e.Old, e.New)
	}
	m.ApplyImport(d, res.Fingerprint, importTime)
	if l, _ := m.Lookup(280, run8routetest.SecBowl1); l.Track != "Hump 1" || l.Source != SourceRun8 {
		t.Errorf("206 = %+v", l)
	}
}

func TestManualLabelsUntouchedByReimport(t *testing.T) {
	res := synthRoute(t)
	p := synthProfile()
	m := New()
	m.ApplyImport(m.Diff(BuildImport(res.Data, p)), res.Fingerprint, importTime)

	// The user relabels an imported section, labels one the import does not
	// know, and the import then stops producing the BRL spur.
	m.Set(280, run8routetest.SecSiding1, Label{Kind: KindIndustry, Industries: []string{"GES"}})
	m.Set(280, 9000, Label{Kind: KindIgnored})
	m.Set(280, run8routetest.SecBowl1, Label{Kind: KindYard, Yard: "NOR", Track: "Bowl 1"}) // same as the import, now manual
	less := res.Data
	less.Industries = slices.Clone(res.Data.Industries[1:]) // BRL gone
	d := m.Diff(BuildImport(less, p))
	if len(d.Removed) != 1 || d.Removed[0].Key != sec(run8routetest.SecSiding2) {
		t.Errorf("removed = %+v", d.Removed)
	}
	m.ApplyImport(d, res.Fingerprint, importTime)
	if l, _ := m.Lookup(280, run8routetest.SecSiding1); l.Source != "" || l.Industries[0] != "GES" {
		t.Errorf("relabelled section = %+v", l)
	}
	if l, _ := m.Lookup(280, 9000); l.Kind != KindIgnored {
		t.Errorf("manual ignored label = %+v", l)
	}
	if l, _ := m.Lookup(280, run8routetest.SecBowl1); l.Source != "" {
		t.Errorf("section 206 = %+v", l)
	}
	// A later full import treats the manual ones as kept.
	d = m.Diff(BuildImport(res.Data, p))
	var kept []string
	for _, e := range d.KeptManual {
		kept = append(kept, e.Key)
	}
	if !slices.Equal(kept, []string{sec(run8routetest.SecSiding1), sec(run8routetest.SecBowl1)}) {
		t.Errorf("kept manual = %v", kept)
	}
}

// ApplyImport never overwrites a label made after the diff was built.
func TestApplyImportSkipsLabelsMadeSinceTheDiff(t *testing.T) {
	res := synthRoute(t)
	p := synthProfile()
	m := New()
	d := m.Diff(BuildImport(res.Data, p))
	m.Set(280, run8routetest.SecSiding1, Label{Kind: KindIgnored})
	m.ApplyImport(d, res.Fingerprint, importTime)
	if l, _ := m.Lookup(280, run8routetest.SecSiding1); l.Kind != KindIgnored || l.Source != "" {
		t.Errorf("section 105 = %+v", l)
	}

	// A changed label turned manual since, and a removed one too.
	m2 := New()
	m2.ApplyImport(m2.Diff(BuildImport(res.Data, p)), res.Fingerprint, importTime)
	p.Run8.Humps[0].TrackPrefix = "Hump"
	less := res.Data
	less.Industries = res.Data.Industries[1:] // BRL gone
	d2 := m2.Diff(BuildImport(less, p))
	m2.Set(280, run8routetest.SecBowl1, Label{Kind: KindYard, Yard: "NOR", Track: "Mine"})
	m2.Set(280, run8routetest.SecSiding2, Label{Kind: KindIgnored})
	m2.ApplyImport(d2, res.Fingerprint, importTime)
	if l, _ := m2.Lookup(280, run8routetest.SecBowl1); l.Track != "Mine" {
		t.Errorf("206 = %+v", l)
	}
	if l, _ := m2.Lookup(280, run8routetest.SecSiding2); l.Kind != KindIgnored {
		t.Errorf("106 = %+v", l)
	}
}

// A feature whose data could not be read must not remove its imported labels.
func TestDiffKeepsLabelsOfUnavailableFeatures(t *testing.T) {
	res := synthRoute(t)
	p := synthProfile()
	m := New()
	m.ApplyImport(m.Diff(BuildImport(res.Data, p)), res.Fingerprint, importTime)
	before := m.Revision()

	noInd := res.Data
	noInd.Industries = nil
	d := m.Diff(BuildImport(noInd, p))
	if len(d.Removed) != 0 || len(d.Unavailable) != 1 {
		t.Fatalf("no industries: removed %d unavailable %v", len(d.Removed), d.Unavailable)
	}

	noTrack := res.Data
	noTrack.Topology = nil
	d = m.Diff(BuildImport(noTrack, p))
	if len(d.Removed) != 0 || len(d.Unavailable) != 1 {
		t.Fatalf("no track db: removed %d unavailable %v", len(d.Removed), d.Unavailable)
	}
	if m.Revision() != before {
		t.Error("Diff changed the map")
	}
}

func TestDiffGroups(t *testing.T) {
	lab := func(track string) *Label {
		return &Label{Kind: KindYard, Yard: "NOR", Track: track, Source: SourceRun8}
	}
	ind := func(tags ...string) *Label { return &Label{Kind: KindIndustry, Industries: tags, Source: SourceRun8} }
	d := ImportDiff{
		New: []DiffEntry{
			{Key: "280/30", New: lab("Bowl 10")},
			{Key: "280/9", New: ind("BRL")},
			{Key: "280/10", New: lab("Bowl 2")},
			{Key: "280/11", New: lab("Bowl 2")},
		},
		Changed:    []DiffEntry{{Key: "280/5", Old: ind("BRL"), New: lab("Bowl 2")}},
		Removed:    []DiffEntry{{Key: "280/4", Old: ind("ZZZ")}},
		KeptManual: []DiffEntry{{Key: "280/6", Old: &Label{Kind: KindIgnored}, New: ind("BRL")}},
		Unchanged:  []DiffEntry{{Key: "280/12", Old: lab("Bowl 10"), New: lab("Bowl 10")}},
	}
	groups := d.Groups()
	var names []string
	for _, g := range groups {
		names = append(names, g.Location.String())
	}
	if want := []string{"NOR / Bowl 2", "NOR / Bowl 10", "BRL", "ZZZ"}; !reflect.DeepEqual(names, want) {
		t.Fatalf("groups = %v, want %v", names, want)
	}
	b2 := groups[0]
	if len(b2.New) != 2 || b2.New[0].Key != "280/10" || b2.New[1].Key != "280/11" || len(b2.Changed) != 1 {
		t.Errorf("Bowl 2 = %+v", b2)
	}
	if b10 := groups[1]; len(b10.New) != 1 || len(b10.Unchanged) != 1 {
		t.Errorf("Bowl 10 = %+v", b10)
	}
	if brl := groups[2]; len(brl.New) != 1 || len(brl.KeptManual) != 1 {
		t.Errorf("BRL = %+v", brl)
	}
	if zzz := groups[3]; len(zzz.Removed) != 1 {
		t.Errorf("ZZZ = %+v", zzz)
	}
}
