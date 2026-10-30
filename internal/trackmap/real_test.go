package trackmap

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"run8-ops-center/internal/profile"
	"run8-ops-center/internal/run8route"
	"run8-ops-center/internal/run8route/run8routetest"
	"run8-ops-center/internal/run8save"
)

// The tests in this file read the git-ignored Run8 files in references/AutoLabel
// and skip when they are absent. They check numbers measured on the AGS route.
// Run8's files are never committed, and neither is anything transcribed from them.

// realRoute copies the local Run8 files into a temp install and loads them with
// the bundled AGS profile. It returns the install root as well.
func realRoute(t *testing.T) (run8route.Result, *profile.Profile, string) {
	t.Helper()
	root := t.TempDir()
	run8routetest.CopyLocal(t, root)
	res := run8route.Load(root, run8routetest.Region, []run8route.Route{{Folder: run8routetest.Route}})
	for _, f := range res.Files {
		if !f.OK {
			t.Fatalf("%s: %v", f.Name, f.Err)
		}
	}
	dir := t.TempDir()
	if err := profile.InstallBundled(&osWriter{}, dir); err != nil {
		t.Fatal(err)
	}
	p, err := profile.Load(filepath.Join(dir, profile.BundledID))
	if err != nil {
		t.Fatal(err)
	}
	return res, p, root
}

func realSnapshot(t *testing.T) *run8save.Snapshot {
	t.Helper()
	snap, err := run8save.ParseFile(worldSave)
	if err != nil {
		t.Fatal(err)
	}
	return snap
}

func TestRealIndustryLabels(t *testing.T) {
	res, p, _ := realRoute(t)
	plan := BuildImport(res.Data, p)
	if len(plan.Unavailable) != 0 {
		t.Fatalf("unavailable: %v", plan.Unavailable)
	}

	// BRL: 8 sections, including 548, 559 and 11776.
	brl := sectionsWhere(plan.Labels, func(l Label) bool { return l.Kind == KindIndustry && slices.Contains(l.Industries, "BRL") })
	if len(brl) != 8 {
		t.Errorf("BRL sections = %v", brl)
	}
	for _, s := range []int{548, 559, 11776} {
		if !slices.Contains(brl, s) {
			t.Errorf("BRL lacks %d", s)
		}
	}
	// BRR's raw tag is " BRR"; its section 8346 is labelled BRR.
	if l := plan.Labels[sec(8346)]; l.Kind != KindIndustry || !slices.Contains(l.Industries, "BRR") || l.Source != SourceRun8 {
		t.Errorf("280/8346 = %+v", l)
	}

	// All 46 industries resolve: none is skipped and every tag is on a label.
	if len(res.Data.Industries) != 46 {
		t.Fatalf("%d industries", len(res.Data.Industries))
	}
	for _, sk := range plan.Skipped {
		if strings.HasPrefix(sk.What, "industry ") {
			t.Errorf("industry skipped: %+v", sk)
		}
	}
	labelled := map[string]bool{}
	for _, l := range plan.Labels {
		for _, tag := range l.Industries {
			labelled[tag] = true
		}
	}
	for _, ind := range res.Data.Industries {
		if tag := run8route.NormTag(ind.Tag); !labelled[tag] {
			t.Errorf("industry %s (%q) has no label", ind.Name, tag)
		}
	}
}

func TestRealBowlAndReceivingLabels(t *testing.T) {
	res, p, _ := realRoute(t)
	plan := BuildImport(res.Data, p)

	count := func(prefix string, n int) {
		t.Helper()
		names := map[string]bool{}
		sections := 0
		for _, l := range plan.Labels {
			if l.Kind == KindYard && l.Yard == "NOR" && strings.HasPrefix(l.Track, prefix+" ") {
				names[l.Track] = true
				sections++
			}
		}
		for i := 1; i <= n; i++ {
			if want := fmt.Sprintf("%s %d", prefix, i); !names[want] {
				t.Errorf("no track %s / %s", "NOR", want)
			}
		}
		if len(names) != n {
			t.Errorf("%s: %d tracks, want %d", prefix, len(names), n)
		}
		t.Logf("%s: %d tracks, %d sections", prefix, len(names), sections)
		want := map[string]int{"Bowl": 336, "Receiving": 120}[prefix]
		if sections != want {
			t.Errorf("%s covers %d sections, want %d", prefix, sections, want)
		}
	}
	count("Bowl", 56)
	count("Receiving", 14)

	// No section is claimed twice, and nothing about the yard tracks is skipped.
	for _, sk := range plan.Skipped {
		t.Errorf("skipped: %+v", sk)
	}
}

// importedReal applies the full import to an empty map.
func importedReal(t *testing.T) (*Map, run8route.Result, *profile.Profile, string) {
	t.Helper()
	res, p, root := realRoute(t)
	m := New()
	m.ApplyImport(m.Diff(BuildImport(res.Data, p)), res.Fingerprint, importTime)
	return m, res, p, root
}

func TestRealImportPlacesCars(t *testing.T) {
	m, _, _, _ := importedReal(t)
	snap := realSnapshot(t)
	cov := m.Coverage(snap)
	if cov.Cars != 458 || cov.Cars-cov.Unmapped != 221 {
		t.Fatalf("coverage = %+v, want 221 of 458 placed", cov)
	}
	// The import is idempotent.
	res, p, _ := realRoute(t)
	d := m.Diff(BuildImport(res.Data, p))
	if len(d.New)+len(d.Changed)+len(d.Removed)+len(d.KeptManual) != 0 || len(d.Unchanged) != len(m.Labels) {
		t.Errorf("repeat import: new %d changed %d removed %d kept %d unchanged %d of %d labels",
			len(d.New), len(d.Changed), len(d.Removed), len(d.KeptManual), len(d.Unchanged), len(m.Labels))
	}
}

func TestRealBowlCarsSitOnTheirAssignedTrack(t *testing.T) {
	m, _, _, root := importedReal(t)
	assigned := readHumpAssignments(t, filepath.Join(filepath.Dir(run8routetest.ConfigPath(root, run8routetest.Region)), "Hump.r8"))
	if len(assigned) != 56 {
		t.Fatalf("%d bowl tracks in Hump.r8", len(assigned))
	}

	snap := realSnapshot(t)
	onBowl := 0
	for _, v := range snap.Cars() {
		var l Label
		var ok bool
		for i := range min(len(v.Trucks), 2) {
			if l, ok = m.Lookup(v.Trucks[i].RoutePrefix, v.Trucks[i].Section); ok {
				break
			}
		}
		if !ok || l.Kind != KindYard || !strings.HasPrefix(l.Track, "Bowl ") {
			continue
		}
		onBowl++
		// A car's tag is "<local> <industry>" or "<road> ."; the assignment names the first word.
		tag := strings.Fields(v.Tag)
		if len(tag) == 0 || !slices.Contains(assigned[strings.TrimPrefix(l.Track, "Bowl ")], tag[0]) {
			t.Errorf("car %s tagged %q is on %s, which Hump.r8 assigns to %v", v.UnitNumber, v.Tag, l.Track, assigned[strings.TrimPrefix(l.Track, "Bowl ")])
		}
	}
	if onBowl != 49 {
		t.Errorf("%d cars on the bowl, want 49", onBowl)
	}
}

func TestRealBRICutLabelsWholeTrackAndSuggestsYard(t *testing.T) {
	res, p, _ := realRoute(t)
	snap := realSnapshot(t)
	find := func(m *Map) Candidate {
		t.Helper()
		for _, c := range m.Candidates(snap, p, &res.Data) {
			if c.TrainID == "999950" {
				return c
			}
		}
		t.Fatal("train 999950 is not a candidate")
		return Candidate{}
	}

	// Empty map: the cut occupies 4 sections and its track has 6.
	m := New()
	c := find(m)
	if c.Cars != 19 || !reflect.DeepEqual(c.Tags, []TagCount{{"BRI BRI", 19}}) {
		t.Fatalf("cut = %d cars, tags %v", c.Cars, c.Tags)
	}
	if want := refs(11211, 11293, 11303, 11319); !reflect.DeepEqual(c.Sections, want) {
		t.Errorf("sections = %v, want %v", c.Sections, want)
	}
	whole := refs(11174, 11199, 11211, 11293, 11303, 11319)
	if !reflect.DeepEqual(c.Track, whole) {
		t.Fatalf("track = %v, want %v", c.Track, whole)
	}
	if c.Suggestion == nil || c.Suggestion.Source != SourceTags {
		t.Errorf("empty map suggestion = %+v", c.Suggestion)
	}
	m.Apply(c, Label{Kind: KindYard, Yard: "NOR", Track: "East Forwarding 1"})
	for _, s := range whole {
		if l, ok := m.Lookup(s.Prefix, s.Section); !ok || l.Track != "East Forwarding 1" || l.Source != "" {
			t.Errorf("%v = %+v, %v", s, l, ok)
		}
	}

	// After the import BRI's own track is elsewhere: no BRI suggestion, and NOR is pre-filled.
	m, _, _, _ = importedReal(t)
	c = find(m)
	s := c.Suggestion
	if s == nil {
		t.Fatal("no suggestion after the import")
	}
	if s.Label.Kind == KindIndustry {
		t.Errorf("suggested industry %v after the import: %+v", s.Label.Industries, s)
	}
	if s.Label.Kind != KindYard || s.Label.Yard != "NOR" || s.Label.Track != "" || !s.NeedsTrackName {
		t.Errorf("suggestion = %+v, want yard NOR without a track name", s)
	}
	t.Logf("suggestion: %s (%s)", s.Reason, s.Source)
}

// readHumpAssignments is the test-only decoder of Hump.r8: for each bowl track,
// the destination tags assigned to it. Layout, every string an R8String: an
// int32 reserved and an int32 hump count; each hump has an int32, its name, an
// int32 config count; each config has an int32, a byte, its name and an int32
// track count; each track has an int32, its name, an int32 tag count and the
// tags. The file must end exactly after the last track. Only the first
// config of the first hump is used.
func readHumpAssignments(t *testing.T, path string) map[string][]string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("Hump.r8 not present: %v", err)
	}
	r := &humpReader{t: t, b: b}
	r.i32() // reserved
	humps := r.i32()
	if humps != 1 {
		t.Fatalf("Hump.r8 holds %d humps", humps)
	}
	r.i32()
	r.str() // hump name
	configs := r.i32()
	if configs != 1 {
		t.Fatalf("Hump.r8 holds %d configs", configs)
	}
	r.i32()
	r.u8()
	r.str() // config name
	tracks := r.i32()
	out := map[string][]string{}
	for range tracks {
		r.i32()
		name := r.str()
		n := r.i32()
		for range n {
			out[name] = append(out[name], r.str())
		}
		if _, ok := out[name]; !ok {
			out[name] = nil
		}
	}
	if len(r.b) != 0 {
		t.Fatalf("Hump.r8: %d bytes left after the last track", len(r.b))
	}
	return out
}

type humpReader struct {
	t *testing.T
	b []byte
}

func (r *humpReader) take(n int) []byte {
	r.t.Helper()
	if n < 0 || n > len(r.b) {
		r.t.Fatalf("Hump.r8: %d bytes wanted, %d left", n, len(r.b))
	}
	s := r.b[:n]
	r.b = r.b[n:]
	return s
}

func (r *humpReader) i32() int {
	r.t.Helper()
	return int(int32(binary.LittleEndian.Uint32(r.take(4))))
}
func (r *humpReader) u8() byte { r.t.Helper(); return r.take(1)[0] }

// str reads a byte count, then two bytes per character: b0<<4 | b1>>4.
func (r *humpReader) str() string {
	r.t.Helper()
	n := r.i32()
	if n%2 != 0 {
		r.t.Fatalf("Hump.r8: odd string length %d", n)
	}
	s := r.take(n)
	var sb strings.Builder
	for i := 0; i < n; i += 2 {
		sb.WriteRune(rune(s[i])<<4 | rune(s[i+1])>>4)
	}
	return sb.String()
}
