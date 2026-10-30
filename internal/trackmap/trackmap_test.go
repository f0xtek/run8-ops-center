package trackmap

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"run8-ops-center/internal/profile"
	"run8-ops-center/internal/run8save"
)

// osWriter writes to the real filesystem (tests use t.TempDir()) and records
// the order of operations.
type osWriter struct{ ops []string }

func (w *osWriter) MkdirAll(p string, perm os.FileMode) error {
	w.ops = append(w.ops, "mkdir")
	return os.MkdirAll(p, perm)
}
func (w *osWriter) WriteFile(p string, d []byte, perm os.FileMode) error {
	w.ops = append(w.ops, "write")
	return os.WriteFile(p, d, perm)
}
func (w *osWriter) Rename(a, b string) error {
	w.ops = append(w.ops, "rename")
	return os.Rename(a, b)
}

func testProfile() *profile.Profile {
	return &profile.Profile{Industries: []profile.Industry{
		{Name: "Bridge Industries", Tag: "BRI"},
		{Name: "Gulf Export Systems", Tag: "GES"},
	}}
}

// car builds a freight car whose trucks sit on the given sections of prefix 280.
func car(num, tag string, s0, s1 int) run8save.Vehicle {
	return run8save.Vehicle{
		Model: "R8_Boxcar_X.xml", UnitType: "US_Freightcar", UnitNumber: num, Tag: tag,
		Trucks: []run8save.Truck{{RoutePrefix: 280, Section: s0}, {RoutePrefix: 280, Section: s1}},
	}
}

func loco(num, tag string, s int) run8save.Vehicle {
	return run8save.Vehicle{
		Model: "R8_SD70.xml", UnitType: "US_DieselEngine", UnitNumber: num, Tag: tag,
		Trucks: []run8save.Truck{{RoutePrefix: 280, Section: s}, {RoutePrefix: 280, Section: s}},
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "trackmap.json")
	m := New()
	m.Set(280, 4535, Label{Kind: KindIndustry, Industries: []string{"GES"}})
	m.Set(280, 5101, Label{Kind: KindYard, Yard: "NOR", Track: "Bowl 14"})
	m.Set(0, 7, Label{Kind: KindIgnored})
	w := &osWriter{}
	if err := m.Save(w, path); err != nil {
		t.Fatal(err)
	}
	if want := []string{"mkdir", "write", "rename"}; !reflect.DeepEqual(w.ops, want) {
		t.Fatalf("ops = %v, want %v", w.ops, want)
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("temp file left behind: %v", err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Labels, m.Labels) {
		t.Fatalf("round trip mismatch:\n got %+v\nwant %+v", got.Labels, m.Labels)
	}
	if l, ok := got.Lookup(280, 4535); !ok || l.Kind != KindIndustry || l.Industries[0] != "GES" {
		t.Fatalf("Lookup(280,4535) = %+v, %v", l, ok)
	}
	if _, ok := got.Lookup(280, 1); ok {
		t.Fatal("unlabelled section found")
	}
}

func TestLoadMissingAndInvalid(t *testing.T) {
	dir := t.TempDir()
	m, err := Load(filepath.Join(dir, "none.json"))
	if err != nil || m == nil || len(m.Labels) != 0 {
		t.Fatalf("missing file: %+v, %v", m, err)
	}
	m.Set(1, 2, Label{Kind: KindIgnored}) // must be writable
	for name, body := range map[string]string{
		"badjson.json": `{`,
		"badkey.json":  `{"labels":{"abc":{"kind":"ignored"}}}`,
		"badkind.json": `{"labels":{"1/2":{"kind":"bogus"}}}`,
	} {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(p); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
	p := filepath.Join(dir, "empty.json")
	if err := os.WriteFile(p, []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if m, err := Load(p); err != nil || m.Labels == nil {
		t.Fatalf("empty object: %+v, %v", m, err)
	}
}

func TestSaveReplacesExisting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "trackmap.json")
	m := New()
	m.Set(1, 1, Label{Kind: KindIgnored})
	if err := m.Save(&osWriter{}, path); err != nil {
		t.Fatal(err)
	}
	m.Remove(1, 1)
	m.Set(1, 2, Label{Kind: KindIgnored})
	if err := m.Save(&osWriter{}, path); err != nil {
		t.Fatal(err)
	}
	got, _ := Load(path)
	if _, ok := got.Lookup(1, 1); ok || len(got.Labels) != 1 {
		t.Fatalf("labels = %+v", got.Labels)
	}
}

func TestBulkLabelSuggestionBRI(t *testing.T) {
	// A cut of 19 cars tagged "BRI BRI" spread over sections 5101-5105.
	var cars []run8save.Vehicle
	for i := range 19 {
		s := 5101 + i/4
		cars = append(cars, car(string(rune('A'+i))+"1", "BRI BRI", s, s))
	}
	snap := &run8save.Snapshot{Trains: []run8save.Train{{ID: "cut1", Vehicles: cars}}}
	m := New()
	cs := m.Candidates(snap, testProfile())
	if len(cs) != 1 {
		t.Fatalf("got %d candidates", len(cs))
	}
	c := cs[0]
	if c.Cars != 19 || c.Powered {
		t.Fatalf("cars=%d powered=%v", c.Cars, c.Powered)
	}
	if c.Suggestion == nil || c.Suggestion.Source != SourceTags ||
		c.Suggestion.Label.Kind != KindIndustry || !reflect.DeepEqual(c.Suggestion.Label.Industries, []string{"BRI"}) {
		t.Fatalf("suggestion = %+v", c.Suggestion)
	}
	if !reflect.DeepEqual(c.Tags, []TagCount{{"BRI BRI", 19}}) {
		t.Fatalf("tags = %+v", c.Tags)
	}
	if len(c.Samples) != 5 {
		t.Fatalf("samples = %v", c.Samples)
	}
	if len(m.Labels) != 0 {
		t.Fatal("suggestion must not label anything")
	}
	want := []SectionRef{{280, 5101}, {280, 5102}, {280, 5103}, {280, 5104}, {280, 5105}}
	if !reflect.DeepEqual(c.Sections, want) {
		t.Fatalf("sections = %v, want %v", c.Sections, want)
	}

	m.Apply(c, c.Suggestion.Label)
	for _, s := range want {
		l, ok := m.Lookup(s.Prefix, s.Section)
		if !ok || l.Kind != KindIndustry || l.Industries[0] != "BRI" {
			t.Fatalf("section %v label = %+v, %v", s, l, ok)
		}
	}
	if left := m.Candidates(snap, testProfile()); len(left) != 0 {
		t.Fatalf("cut still a candidate: %+v", left)
	}
	if cov := m.Coverage(snap); cov != (Coverage{Cars: 19}) {
		t.Fatalf("coverage = %+v", cov)
	}
}

func TestTagSuggestionRules(t *testing.T) {
	p := testProfile()
	m := New()
	cut := func(tags ...string) *run8save.Snapshot {
		var vs []run8save.Vehicle
		for i, tag := range tags {
			vs = append(vs, car(string(rune('a'+i)), tag, 10, 10))
		}
		return &run8save.Snapshot{Trains: []run8save.Train{{ID: "c", Vehicles: vs}}}
	}
	cases := []struct {
		name string
		tags []string
		want string // "" = no suggestion
	}{
		{"local tag", []string{"A39 GES", "A39  GES"}, "GES"},
		{"bare industry tag", []string{"BRI", "BRI BRI"}, "BRI"},
		{"mixed industries", []string{"A39 GES", "BRI BRI"}, ""},
		{"one car untagged", []string{"A39 GES", "None"}, ""},
		{"unknown industry", []string{"A39 ZZZ"}, ""},
		{"blank", []string{""}, ""},
	}
	for _, tc := range cases {
		c := m.Candidates(cut(tc.tags...), p)[0]
		got := ""
		if c.Suggestion != nil {
			got = c.Suggestion.Label.Industries[0]
		}
		if got != tc.want {
			t.Errorf("%s: suggestion %q, want %q", tc.name, got, tc.want)
		}
	}
	// Nil profile disables tag suggestions without panicking.
	if c := m.Candidates(cut("A39 GES"), nil)[0]; c.Suggestion != nil {
		t.Errorf("nil profile gave %+v", c.Suggestion)
	}
}

func TestNeighbourSuggestion(t *testing.T) {
	m := New()
	norris := Label{Kind: KindYard, Yard: "NOR", Track: "Bowl 14"}
	m.Set(280, 5100, norris)
	snap := &run8save.Snapshot{Trains: []run8save.Train{
		{ID: "next", Vehicles: []run8save.Vehicle{car("1", "A39 GES", 5101, 5101), car("2", "BRI BRI", 5101, 5102)}},
		{ID: "far", Vehicles: []run8save.Vehicle{car("3", "", 6000, 6000)}},
	}}
	cs := m.Candidates(snap, testProfile())
	if len(cs) != 2 {
		t.Fatalf("candidates = %+v", cs)
	}
	s := cs[0].Suggestion
	if s == nil || s.Source != SourceNeighbour || !reflect.DeepEqual(s.Label, norris) {
		t.Fatalf("neighbour suggestion = %+v", s)
	}
	if !containsAll(s.Reason, "hint", "280/5101", "280/5100", "NOR / Bowl 14") {
		t.Fatalf("reason = %q", s.Reason)
	}
	if cs[1].Suggestion != nil {
		t.Fatalf("far cut got suggestion %+v", cs[1].Suggestion)
	}
	// A different prefix at index +/-1 is not a neighbour.
	other := New()
	other.Set(281, 6001, norris)
	if c := other.Candidates(snap, nil); c[1].Suggestion != nil {
		t.Fatalf("cross-prefix suggestion: %+v", c[1].Suggestion)
	}
	// Tag suggestion outranks the hint when every car is tagged.
	tagged := &run8save.Snapshot{Trains: []run8save.Train{{ID: "t", Vehicles: []run8save.Vehicle{car("1", "A39 GES", 5101, 5101)}}}}
	if c := m.Candidates(tagged, testProfile())[0]; c.Suggestion.Source != SourceTags {
		t.Fatalf("source = %q", c.Suggestion.Source)
	}
}

func containsAll(s string, parts ...string) bool {
	for _, p := range parts {
		if !strings.Contains(s, p) {
			return false
		}
	}
	return true
}

func TestCandidatesMappedIgnoredAndPowered(t *testing.T) {
	m := New()
	m.Set(280, 1, Label{Kind: KindIgnored})
	m.Set(280, 2, Label{Kind: KindIndustry, Industries: []string{"BRI"}})
	snap := &run8save.Snapshot{Trains: []run8save.Train{
		{ID: "ignored-only", Vehicles: []run8save.Vehicle{car("1", "", 1, 1)}},
		{ID: "mapped", Vehicles: []run8save.Vehicle{car("2", "", 2, 1)}},
		{ID: "straddle", Vehicles: []run8save.Vehicle{car("3", "", 2, 9), loco("L1", "BRR", 9)}},
		{ID: "empty"},
	}}
	cs := m.Candidates(snap, testProfile())
	if len(cs) != 1 || cs[0].TrainID != "straddle" {
		t.Fatalf("candidates = %+v", cs)
	}
	c := cs[0]
	if !c.Powered || c.Symbol != "BRR" || c.Cars != 1 || !reflect.DeepEqual(c.Sections, []SectionRef{{280, 9}}) {
		t.Fatalf("candidate = %+v", c)
	}
	if !reflect.DeepEqual(c.Samples, []string{"3"}) {
		t.Fatalf("samples = %v", c.Samples)
	}
}

func TestApplySkipsSectionsLabelledSince(t *testing.T) {
	m := New()
	snap := &run8save.Snapshot{Trains: []run8save.Train{{ID: "c", Vehicles: []run8save.Vehicle{car("1", "", 10, 11)}}}}
	c := m.Candidates(snap, nil)[0]
	m.Set(280, 11, Label{Kind: KindIgnored})
	m.Apply(c, Label{Kind: KindYard, Yard: "NOR", Track: "Bowl 1"})
	if l, _ := m.Lookup(280, 10); l.Kind != KindYard {
		t.Fatalf("10 = %+v", l)
	}
	if l, _ := m.Lookup(280, 11); l.Kind != KindIgnored {
		t.Fatalf("11 overwritten: %+v", l)
	}
}

func TestRelabelRemoveAndLocations(t *testing.T) {
	m := New()
	b14 := Label{Kind: KindYard, Yard: "NOR", Track: "Bowl 14"}
	b15 := Label{Kind: KindYard, Yard: "NOR", Track: "Bowl 15"}
	m.Set(280, 5103, b14)
	m.Set(280, 5101, b14)
	m.Set(280, 5102, b14)
	m.Set(280, 4535, Label{Kind: KindIndustry, Industries: []string{"GES"}})
	m.Set(0, 3, Label{Kind: KindIgnored})

	// Relabel one section (spec: correcting a mistake).
	m.Set(280, 5101, b15)
	if l, _ := m.Lookup(280, 5101); l.Track != "Bowl 15" {
		t.Fatalf("relabel = %+v", l)
	}

	groups := m.Locations()
	var names []string
	for _, g := range groups {
		names = append(names, g.Location.String())
	}
	if want := []string{"NOR / Bowl 14", "NOR / Bowl 15", "GES", "Ignored"}; !reflect.DeepEqual(names, want) {
		t.Fatalf("locations = %v, want %v", names, want)
	}
	if want := []SectionRef{{280, 5102}, {280, 5103}}; !reflect.DeepEqual(groups[0].Sections, want) {
		t.Fatalf("Bowl 14 sections = %v", groups[0].Sections)
	}

	// Rename moves every section of the location and nothing else.
	n := m.Rename(b14.Location(), Label{Kind: KindYard, Yard: "NOR", Track: "Bowl 20"}.Location())
	if n != 2 {
		t.Fatalf("renamed %d", n)
	}
	for _, s := range []int{5102, 5103} {
		if l, _ := m.Lookup(280, s); l.Track != "Bowl 20" {
			t.Fatalf("%d = %+v", s, l)
		}
	}
	if l, _ := m.Lookup(280, 5101); l.Track != "Bowl 15" {
		t.Fatalf("5101 should be untouched: %+v", l)
	}

	// Industry locations compare irrespective of tag order.
	m.Set(280, 1, Label{Kind: KindIndustry, Industries: []string{"B", "A"}})
	if m.Rename(Location{Kind: KindIndustry, Industries: []string{"A", "B"}}, Location{Kind: KindIgnored}) != 1 {
		t.Fatal("industry rename by unordered tags failed")
	}

	m.Remove(280, 5101)
	if _, ok := m.Lookup(280, 5101); ok {
		t.Fatal("Remove left label")
	}
	m.Remove(280, 5101) // removing an absent label is a no-op
}

func TestSetCopiesIndustries(t *testing.T) {
	m := New()
	tags := []string{"GES"}
	m.Set(1, 1, Label{Kind: KindIndustry, Industries: tags})
	tags[0] = "XXX"
	if l, _ := m.Lookup(1, 1); l.Industries[0] != "GES" {
		t.Fatalf("label aliased caller slice: %+v", l)
	}
	var nilMap Map
	nilMap.Set(1, 1, Label{Kind: KindIgnored}) // zero value Map must accept Set
}

func TestCoverage(t *testing.T) {
	m := New()
	m.Set(280, 1, Label{Kind: KindYard, Yard: "NOR", Track: "T"})
	m.Set(280, 3, Label{Kind: KindIgnored})
	snap := &run8save.Snapshot{Trains: []run8save.Train{{ID: "x", Vehicles: []run8save.Vehicle{
		loco("L", "", 99),                            // locos are not cars
		car("a", "", 1, 1),                           // mapped
		car("b", "", 2, 1),                           // truck 0 unmapped, truck 1 mapped
		car("c", "", 1, 2),                           // truck 0 mapped
		car("d", "", 2, 2),                           // unmapped
		car("e", "", 3, 4),                           // ignored counts as mapped
		{UnitType: "US_Freightcar", UnitNumber: "f"}, // no trucks: unmapped
	}}}}
	if got, want := m.Coverage(snap), (Coverage{Cars: 6, Unmapped: 2}); got != want {
		t.Fatalf("coverage = %+v, want %+v", got, want)
	}
	if got := m.Coverage(nil); got != (Coverage{}) {
		t.Fatalf("nil snapshot coverage = %+v", got)
	}
}
