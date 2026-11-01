package trackmap

import (
	"math"
	"reflect"
	"testing"

	"run8-ops-center/internal/profile"
	"run8-ops-center/internal/run8route"
	"run8-ops-center/internal/run8route/run8routetest"
	"run8-ops-center/internal/run8save"
)

func hintNames(hs []NameHint) []string {
	var out []string
	for _, h := range hs {
		out = append(out, h.Track.Name)
	}
	return out
}

func tusYard() profile.Yard {
	return profile.Yard{Code: "TUS", Name: "Tuscaloosa", Tracks: []profile.YardTrack{
		{Name: "TY01", LengthFt: 1379},
		{Name: "Unmeasured"},
		{Name: "TY08", LengthFt: 710},
		{Name: "Pass", LengthFt: 752, Kind: profile.TrackSiding},
		{Name: "TY06", LengthFt: 543},
		{Name: "TY09", LengthFt: 710},
	}}
}

func TestNameHintsRankByLengthThenProfileOrder(t *testing.T) {
	got := NameHints(tusYard(), nil, 717)
	want := []string{"TY08", "TY09", "Pass", "TY06", "TY01", "Unmeasured"}
	if !reflect.DeepEqual(hintNames(got), want) {
		t.Fatalf("hints = %v, want %v", hintNames(got), want)
	}
	// TY08 and TY09 tie at 7 ft and keep profile order; Unmeasured has no difference.
	if !got[0].HasDiff || got[0].DiffFt != 7 || !got[1].HasDiff || got[1].DiffFt != 7 || got[2].DiffFt != 35 {
		t.Errorf("diffs = %+v", got)
	}
	if last := got[len(got)-1]; last.HasDiff || last.DiffFt != 0 {
		t.Errorf("entry without a length: %+v", last)
	}
	if got[2].Track.Kind != profile.TrackSiding {
		t.Errorf("kind lost: %+v", got[2].Track)
	}
}

func TestNameHintsDropUsedNames(t *testing.T) {
	got := NameHints(tusYard(), map[string]bool{"TY08": true, "  ty01 ": true, "TY06": false}, 717)
	want := []string{"TY09", "Pass", "TY06", "Unmeasured"}
	if !reflect.DeepEqual(hintNames(got), want) {
		t.Fatalf("hints = %v, want %v", hintNames(got), want)
	}
}

func TestNameHintsKeepProfileOrderWithoutMeasuredLength(t *testing.T) {
	got := NameHints(tusYard(), nil, 0)
	want := []string{"TY01", "Unmeasured", "TY08", "Pass", "TY06", "TY09"}
	if !reflect.DeepEqual(hintNames(got), want) {
		t.Fatalf("hints = %v, want %v", hintNames(got), want)
	}
	for _, h := range got {
		if h.HasDiff || h.DiffFt != 0 {
			t.Errorf("%s has a difference without a measured length: %+v", h.Track.Name, h)
		}
	}
	if got := NameHints(profile.Yard{Code: "X"}, nil, 500); len(got) != 0 {
		t.Errorf("yard without tracks: %v", got)
	}
}

func TestTrackNamesInYard(t *testing.T) {
	m := New()
	m.Set(280, 1, Label{Kind: KindYard, Yard: "TUS", Track: "TY08"})
	m.Set(280, 2, Label{Kind: KindYard, Yard: "TUS", Track: "TY08"})
	m.Set(280, 3, Label{Kind: KindYard, Yard: "NOR", Track: "TY01"})
	m.Set(280, 4, Label{Kind: KindIgnored})
	m.Set(280, 5, Label{Kind: KindIndustry, Industries: []string{"BRL"}})
	m.Set(280, 6, Label{Kind: KindYard, Yard: "TUS"})
	if got := m.TrackNamesInYard("TUS"); !reflect.DeepEqual(got, map[string]bool{"TY08": true}) {
		t.Errorf("TUS = %v", got)
	}
	if got := m.TrackNamesInYard("MCC"); len(got) != 0 {
		t.Errorf("MCC = %v", got)
	}
	var none *Map
	if got := none.TrackNamesInYard("TUS"); len(got) != 0 {
		t.Errorf("nil map = %v", got)
	}
}

// sumFt is the independent check: a run's section lengths in feet.
func sumFt(t *testing.T, res run8route.Result, sections ...int) int {
	t.Helper()
	m := 0.0
	for _, s := range sections {
		sec, ok := res.Data.Topology.Section(s)
		if !ok {
			t.Fatalf("no section %d", s)
		}
		m += sec.LengthM
	}
	return int(math.Round(m * 3.28084))
}

func TestCandidateTrackLengthIsTheSumOfWholeRuns(t *testing.T) {
	res := synthRoute(t)
	p := synthProfile()

	// A cut on the middle of RCV 1: sections 300, 301 and 302.
	want := sumFt(t, res, 300, 301, 302)
	if want < 800 {
		t.Fatalf("synthetic run is %d ft", want)
	}
	c := New().Candidates(cutOn("cut", "", 301), p, &res.Data)[0]
	if c.TrackLengthFt != want {
		t.Errorf("length = %d ft, want %d", c.TrackLengthFt, want)
	}

	// Labelled sections still count: the length is the whole run, not the unmapped part.
	m := New()
	m.Set(280, 300, Label{Kind: KindIgnored})
	c = m.Candidates(cutOn("cut", "", 301), p, &res.Data)[0]
	if len(c.Track) != 2 || c.TrackLengthFt != want {
		t.Errorf("with 300 labelled: track %v, length %d ft, want %d", c.Track, c.TrackLengthFt, want)
	}

	// RCV 1, RCV 2 and the switch 341 the train occupies: every run and the switch once.
	snap := &run8save.Snapshot{Trains: []run8save.Train{{ID: "t", Vehicles: []run8save.Vehicle{
		car("a", "", 301, 301), car("b", "", 311, 341), car("c", "", 310, 311),
	}}}}
	c = New().Candidates(snap, p, &res.Data)[0]
	if want := sumFt(t, res, 300, 301, 302, 310, 311, 341); c.TrackLengthFt != want {
		t.Errorf("three runs: length = %d ft, want %d", c.TrackLengthFt, want)
	}

	// A switch the train stands on counts by itself, and no neighbour does.
	c = New().Candidates(cutOn("sw", "", 341), p, &res.Data)[0]
	if want := sumFt(t, res, 341); c.TrackLengthFt != want {
		t.Errorf("switch: length = %d ft, want %d", c.TrackLengthFt, want)
	}

	// Sections of another prefix and unknown sections add nothing.
	snap = &run8save.Snapshot{Trains: []run8save.Train{{ID: "u", Vehicles: []run8save.Vehicle{
		car("a", "", 77777, 77777),
		{Model: "R8_Boxcar_X.xml", UnitType: "US_Freightcar", UnitNumber: "o",
			Trucks: []run8save.Truck{{RoutePrefix: 999, Section: 301}, {RoutePrefix: 999, Section: 301}}},
	}}}}
	if c := New().Candidates(snap, p, &res.Data)[0]; c.TrackLengthFt != 0 {
		t.Errorf("unknown sections: length = %d ft", c.TrackLengthFt)
	}

	// Without route data, and with a profile that has no route prefix, it is 0.
	if c := New().Candidates(cutOn("cut", "", 301), p, nil)[0]; c.TrackLengthFt != 0 {
		t.Errorf("no route data: length = %d ft", c.TrackLengthFt)
	}
	bare := synthProfile()
	bare.Run8 = nil
	if c := New().Candidates(cutOn("cut", "", 301), bare, &res.Data)[0]; c.TrackLengthFt != 0 {
		t.Errorf("no run8 block: length = %d ft", c.TrackLengthFt)
	}
}

func TestLocationLengthsFtSumsLabelledSections(t *testing.T) {
	res := synthRoute(t)
	p := synthProfile()
	m := New()
	for _, s := range []int{300, 301, 302} {
		m.Set(280, s, Label{Kind: KindYard, Yard: "NOR", Track: "Receiving 1"})
	}
	m.Set(280, 310, Label{Kind: KindYard, Yard: "NOR", Track: "Receiving 2"})
	m.Set(999, 310, Label{Kind: KindYard, Yard: "NOR", Track: "Receiving 2"}) // another prefix adds nothing
	m.Set(280, 77777, Label{Kind: KindYard, Yard: "NOR", Track: "Unknown"})   // not in the database
	m.Set(280, 105, Label{Kind: KindIndustry, Industries: []string{"BRL"}})
	m.Set(280, 100, Label{Kind: KindIgnored})

	got := LocationLengthsFt(m.Locations(), p, &res.Data)
	key := func(track string) string { return Location{Kind: KindYard, Yard: "NOR", Track: track}.Key() }
	want := map[string]int{
		key("Receiving 1"): sumFt(t, res, 300, 301, 302),
		key("Receiving 2"): sumFt(t, res, 310),
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("lengths = %v, want %v", got, want)
	}
	if got := LocationLengthsFt(m.Locations(), p, nil); got != nil {
		t.Errorf("no route data: %v", got)
	}
	bare := synthProfile()
	bare.Run8 = nil
	if got := LocationLengthsFt(m.Locations(), bare, &res.Data); got != nil {
		t.Errorf("no run8 block: %v", got)
	}
}

// A cut on a 717 ft track in Tuscaloosa ranks TY08 (710 ft) then Pass (752 ft)
// first. The fixtures hold no 717 ft run with a cut on it, so the measured
// length is given. The test reads the bundled AGS lists through the local route
// helper, and skips without references/AutoLabel.
func TestRealTuscaloosaHintsRankTY08ThenPass(t *testing.T) {
	res, p, _ := realRoute(t)
	var tus *profile.Yard
	for i := range p.Yards {
		if p.Yards[i].Code == "TUS" {
			tus = &p.Yards[i]
		}
	}
	if tus == nil {
		t.Fatal("no TUS yard in the bundled profile")
	}
	got := NameHints(*tus, New().TrackNamesInYard("TUS"), 717)
	if len(got) != 12 || got[0].Track.Name != "TY08" || got[1].Track.Name != "Pass" {
		t.Fatalf("hints = %v", hintNames(got))
	}
	if got[0].DiffFt != 7 || got[1].DiffFt != 35 || got[1].Track.Kind != profile.TrackSiding {
		t.Errorf("hints = %+v", got[:2])
	}

	// The import labels no TUS track, and TY08 drops out once it is labelled.
	m := New()
	m.ApplyImport(m.Diff(BuildImport(res.Data, p)), res.Fingerprint, importTime)
	if used := m.TrackNamesInYard("TUS"); len(used) != 0 {
		t.Errorf("the import labels TUS tracks %v", used)
	}
	m.Set(run8routetest.Prefix, 1, Label{Kind: KindYard, Yard: "TUS", Track: "TY08"})
	if got := NameHints(*tus, m.TrackNamesInYard("TUS"), 717); got[0].Track.Name != "Pass" || len(got) != 11 {
		t.Errorf("after labelling TY08: %v", hintNames(got))
	}

	// Measured on the real route: the 730 ft cut in the fixtures ranks the same two first.
	snap := realSnapshot(t)
	for _, c := range New().Candidates(snap, p, &res.Data) {
		if c.TrainID != "999948" {
			continue
		}
		if c.TrackLengthFt != 730 {
			t.Errorf("train 999948 measures %d ft", c.TrackLengthFt)
		}
		h := NameHints(*tus, nil, c.TrackLengthFt)
		if h[0].Track.Name != "TY08" || h[1].Track.Name != "Pass" {
			t.Errorf("730 ft hints = %v", hintNames(h))
		}
	}
}
