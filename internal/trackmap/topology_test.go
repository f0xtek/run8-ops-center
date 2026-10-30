package trackmap

import (
	"reflect"
	"strings"
	"testing"

	"run8-ops-center/internal/profile"
	"run8-ops-center/internal/run8route"
	"run8-ops-center/internal/run8route/run8routetest"
	"run8-ops-center/internal/run8save"
)

func refs(sections ...int) []SectionRef {
	var out []SectionRef
	for _, s := range sections {
		out = append(out, SectionRef{run8routetest.Prefix, s})
	}
	return out
}

func importedMap(t testing.TB, res run8route.Result, p *profile.Profile) *Map {
	t.Helper()
	m := New()
	m.ApplyImport(m.Diff(BuildImport(res.Data, p)), res.Fingerprint, importTime)
	return m
}

func TestCandidateTrackIsTheWholeSwitchBoundedTrack(t *testing.T) {
	res := synthRoute(t)
	p := synthProfile()
	yard := Label{Kind: KindYard, Yard: "NOR", Track: "Pocket"}

	// A cut on the middle of RCV 1's track labels the whole track.
	snap := cutOn("cut", "", 301)
	m := New()
	c := m.Candidates(snap, p, &res.Data)[0]
	if !reflect.DeepEqual(c.Sections, refs(301)) || !reflect.DeepEqual(c.Track, refs(300, 301, 302)) {
		t.Fatalf("sections %v, track %v", c.Sections, c.Track)
	}
	m.Apply(c, yard)
	if got := sectionsWhere(m.Labels, func(l Label) bool { return l.Track == "Pocket" }); !reflect.DeepEqual(got, []int{300, 301, 302}) {
		t.Fatalf("labelled %v", got)
	}
	for k, l := range m.Labels {
		if l.Source != "" {
			t.Errorf("%s: source %q", k, l.Source)
		}
	}
	if left := m.Candidates(snap, p, &res.Data); len(left) != 0 {
		t.Errorf("the cut is still a candidate: %+v", left)
	}

	// Without route data only the occupied section is labelled.
	m = New()
	c = m.Candidates(snap, p, nil)[0]
	if !reflect.DeepEqual(c.Track, refs(301)) {
		t.Fatalf("no route data: track %v", c.Track)
	}
	m.Apply(c, yard)
	if got := sectionsWhere(m.Labels, func(Label) bool { return true }); !reflect.DeepEqual(got, []int{301}) {
		t.Errorf("no route data: labelled %v", got)
	}

	// A profile with no route prefix cannot use the track database either.
	bare := synthProfile()
	bare.Run8 = nil
	if c := New().Candidates(snap, bare, &res.Data)[0]; !reflect.DeepEqual(c.Track, refs(301)) {
		t.Errorf("no run8 block: track %v", c.Track)
	}

	// Sections that already carry a label are left alone.
	m = New()
	m.Set(280, 300, Label{Kind: KindIgnored})
	c = m.Candidates(snap, p, &res.Data)[0]
	if !reflect.DeepEqual(c.Track, refs(301, 302)) {
		t.Fatalf("with 300 labelled: track %v", c.Track)
	}
	m.Apply(c, yard)
	if l, _ := m.Lookup(280, 300); l.Kind != KindIgnored {
		t.Errorf("300 = %+v", l)
	}
	if l, _ := m.Lookup(280, 302); l.Track != "Pocket" {
		t.Errorf("302 = %+v", l)
	}
}

func TestCandidateTrackCoversEveryTrackAndSwitchOccupied(t *testing.T) {
	res := synthRoute(t)
	p := synthProfile()
	m := New()

	// One train on RCV 1 and RCV 2 and on the ladder switch 341.
	snap := &run8save.Snapshot{Trains: []run8save.Train{{ID: "t", Vehicles: []run8save.Vehicle{
		car("a", "", 301, 301), car("b", "", 311, 341),
	}}}}
	c := m.Candidates(snap, p, &res.Data)[0]
	if want := refs(300, 301, 302, 310, 311, 341); !reflect.DeepEqual(c.Track, want) {
		t.Errorf("track = %v, want %v", c.Track, want)
	}
	if want := refs(301, 311, 341); !reflect.DeepEqual(c.Sections, want) {
		t.Errorf("sections = %v, want %v", c.Sections, want)
	}

	// A section the track database does not know, and a section of another prefix, stay as occupied.
	snap = &run8save.Snapshot{Trains: []run8save.Train{{ID: "u", Vehicles: []run8save.Vehicle{
		car("a", "", 77777, 77777),
		{Model: "R8_Boxcar_X.xml", UnitType: "US_Freightcar", UnitNumber: "o",
			Trucks: []run8save.Truck{{RoutePrefix: 999, Section: 301}, {RoutePrefix: 999, Section: 301}}},
	}}}}
	c = m.Candidates(snap, p, &res.Data)[0]
	want := []SectionRef{{280, 77777}, {999, 301}}
	if !reflect.DeepEqual(c.Track, want) {
		t.Errorf("track = %v, want %v", c.Track, want)
	}
}

func mixed(id string, sections ...int) *run8save.Snapshot {
	var vs []run8save.Vehicle
	for i, s := range sections {
		tag := "A39 GES"
		if i%2 == 1 {
			tag = "BRI BRI"
		}
		vs = append(vs, car(string(rune('a'+i)), tag, s, s))
	}
	return &run8save.Snapshot{Trains: []run8save.Train{{ID: id, Vehicles: vs}}}
}

func TestSuggestionFromTags(t *testing.T) {
	res := synthRoute(t)
	p := synthProfile()
	m := New()
	c := m.Candidates(cutOn("c", "AB45 BRL", 104), p, &res.Data)[0]
	s := c.Suggestion
	if s == nil || s.Source != SourceTags || s.NeedsTrackName || s.Label.Kind != KindIndustry || !reflect.DeepEqual(s.Label.Industries, []string{"BRL"}) {
		t.Fatalf("suggestion = %+v", s)
	}
	if len(m.Labels) != 0 {
		t.Error("a suggestion labelled something")
	}
}

func TestNoTagSuggestionAwayFromImportedTrack(t *testing.T) {
	res := synthRoute(t)
	p := synthProfile()
	m := importedMap(t, res, p)

	// 320 has no link to BRL's spur: no BRL, and the nearest anchor pre-fills yard NOR.
	c := m.Candidates(cutOn("away", "AB45 BRL", 320), p, &res.Data)[0]
	s := c.Suggestion
	if s == nil || s.Source != SourceAnchor || s.Label.Kind != KindYard || s.Label.Yard != "NOR" || s.Label.Track != "" || !s.NeedsTrackName {
		t.Fatalf("away from BRL: suggestion = %+v", s)
	}
	if !containsAll(s.Reason, "milepost Receiving Yard", "NOR") {
		t.Errorf("reason = %q", s.Reason)
	}

	// 103 is linked across switch 102 to the spur section 105: the tag rule applies.
	c = m.Candidates(cutOn("near", "AB45 BRL", 103), p, &res.Data)[0]
	if s := c.Suggestion; s == nil || s.Source != SourceTags || !reflect.DeepEqual(s.Label.Industries, []string{"BRL"}) {
		t.Fatalf("next to BRL: suggestion = %+v", s)
	}
	if !strings.Contains(c.Suggestion.Reason, "280/105") {
		t.Errorf("reason = %q", c.Suggestion.Reason)
	}
}

func TestSuggestionFromConnectedYardTrack(t *testing.T) {
	res := synthRoute(t)
	p := synthProfile()
	m := New()
	m.Set(280, 300, norRcv("3"))
	m.Set(280, 301, norRcv("3"))
	m.Set(280, 302, norRcv("3"))

	// 346 is a lead joined by switch 341 to the end of that track; the tags are mixed.
	c := m.Candidates(mixed("lead", 346, 346), p, &res.Data)[0]
	s := c.Suggestion
	if s == nil || s.Source != SourceConnected || s.Label.Kind != KindYard || s.Label.Yard != "NOR" || s.Label.Track != "" || !s.NeedsTrackName {
		t.Fatalf("suggestion = %+v", s)
	}
	if !containsAll(s.Reason, "280/302", "NOR / Receiving 3") {
		t.Errorf("reason = %q", s.Reason)
	}

	// The connected label outranks the anchor: both would name NOR here, so use
	// a yard whose anchor is another yard's.
	other := synthProfile(
		profile.Yard{Code: "NOR", Name: "Norris"},
		profile.Yard{Code: "MCC", Name: "McCalla", AnchorMileposts: []string{"Receiving Yard"}},
	)
	c = m.Candidates(mixed("lead", 346, 346), other, &res.Data)[0]
	if s := c.Suggestion; s == nil || s.Source != SourceConnected || s.Label.Yard != "NOR" {
		t.Fatalf("connected vs anchor: %+v", s)
	}
}

func TestSuggestionFromConnectedIndustry(t *testing.T) {
	res := synthRoute(t)
	p := synthProfile()
	m := importedMap(t, res, p)

	// Mixed tags on 103, linked across switch 102 to BRL's spur: the industry whole.
	c := m.Candidates(mixed("main", 103, 103), p, &res.Data)[0]
	s := c.Suggestion
	if s == nil || s.Source != SourceConnected || s.NeedsTrackName || s.Label.Kind != KindIndustry || !reflect.DeepEqual(s.Label.Industries, []string{"BRL"}) {
		t.Fatalf("suggestion = %+v", s)
	}
	if s.Label.Source != "" {
		t.Errorf("suggested label has source %q", s.Label.Source)
	}
	// Accepting it labels the whole switch-bounded track with a manual label.
	m.Apply(c, s.Label)
	for _, n := range []int{103, 104} {
		if l, _ := m.Lookup(280, n); l.Kind != KindIndustry || l.Source != "" {
			t.Errorf("%d = %+v", n, l)
		}
	}

	// A connected ignored label is not offered.
	ign := New()
	ign.Set(280, 105, Label{Kind: KindIgnored})
	if c := ign.Candidates(mixed("main", 103, 103), synthProfile(profile.Yard{Code: "FAR", AnchorMileposts: []string{"Far Junction"}}), &res.Data)[0]; c.Suggestion != nil {
		t.Errorf("ignored neighbour gave %+v", c.Suggestion)
	}
}

func TestSuggestionFromAnchors(t *testing.T) {
	res := synthRoute(t)
	m := New()

	// An industry anchor: BBB's spur is 50 m from the cut on 510, the milepost of
	// another yard is farther away.
	p := synthProfile(
		profile.Yard{Code: "FAR", Name: "Far", AnchorMileposts: []string{"Far Junction"}},
		profile.Yard{Code: "MCC", Name: "McCalla", AnchorIndustries: []string{"BBB"}},
		profile.Yard{Code: "NOR", Name: "Norris", AnchorMileposts: []string{"Receiving Yard"}},
	)
	c := m.Candidates(mixed("c", 510, 510), p, &res.Data)[0]
	s := c.Suggestion
	if s == nil || s.Source != SourceAnchor || s.Label.Yard != "MCC" || s.Label.Track != "" || !s.NeedsTrackName {
		t.Fatalf("suggestion = %+v", s)
	}
	if !containsAll(s.Reason, "industry BBB", "MCC") {
		t.Errorf("reason = %q", s.Reason)
	}

	// A milepost anchor, whose station name has a doubled space in the file.
	c = m.Candidates(mixed("c", 320, 320), p, &res.Data)[0]
	if s := c.Suggestion; s == nil || s.Label.Yard != "NOR" || !strings.Contains(s.Reason, "milepost Receiving Yard") {
		t.Fatalf("milepost anchor: %+v", s)
	}

	// Nothing within 3 km: the only anchor is Far Junction, about 4.9 km away.
	far := synthProfile(profile.Yard{Code: "FAR", Name: "Far", AnchorMileposts: []string{"Far Junction"}})
	for _, n := range []int{320, 510} {
		if c := m.Candidates(mixed("c", n, n), far, &res.Data)[0]; c.Suggestion != nil {
			t.Errorf("section %d: suggestion %+v beyond 3 km", n, c.Suggestion)
		}
	}
	// An anchor name that matches nothing is no anchor.
	none := synthProfile(profile.Yard{Code: "NOR", Name: "Norris", AnchorMileposts: []string{"Misspelled"}, AnchorIndustries: []string{"GES"}})
	if c := m.Candidates(mixed("c", 320, 320), none, &res.Data)[0]; c.Suggestion != nil {
		t.Errorf("unmatched anchors gave %+v", c.Suggestion)
	}
}

func TestNoNeighbourOrAnchorWithoutRouteData(t *testing.T) {
	res := synthRoute(t)
	p := synthProfile()
	m := New()
	m.Set(280, 301, norRcv("1"))

	// 300 and 302 are index-adjacent or track-adjacent to the label, but no route data is given.
	for _, n := range []int{300, 302, 320, 510} {
		if c := m.Candidates(mixed("c", n, n), p, nil)[0]; c.Suggestion != nil {
			t.Errorf("section %d: suggestion %+v without route data", n, c.Suggestion)
		}
	}
	// A route whose track database failed to load counts as no route data.
	broken := res.Data
	broken.Topology = nil
	if c := m.Candidates(mixed("c", 320, 320), p, &broken)[0]; c.Suggestion != nil {
		t.Errorf("no track database: suggestion %+v", c.Suggestion)
	}
}

func TestCandidateTrackFeedsTheSuggestions(t *testing.T) {
	res := synthRoute(t)
	p := synthProfile()
	m := New()
	// The cut stands on 301; the label is on 302 of the same track, so it is adjacent to the track.
	m.Set(280, 302, norRcv("1"))
	c := m.Candidates(mixed("c", 301, 301), p, &res.Data)[0]
	if s := c.Suggestion; s == nil || s.Source != SourceConnected || s.Label.Yard != "NOR" {
		t.Fatalf("suggestion = %+v", c.Suggestion)
	}
	if !reflect.DeepEqual(c.Track, refs(300, 301)) {
		t.Errorf("track = %v", c.Track)
	}
}
