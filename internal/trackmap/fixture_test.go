package trackmap

import (
	"os"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"run8-ops-center/internal/run8save"
)

const worldSave = "../../testdata/WorldSave.xml"

var (
	trainBlockRE = regexp.MustCompile(`(?s)<TrainLoader>.*?</TrainLoader>`)
	trainIDRE    = regexp.MustCompile(`<trainID>(\d+)</trainID>`)
	prefixRE     = regexp.MustCompile(`(?s)<currentRoutePrefix>(.*?)</currentRoutePrefix>`)
	sectionRE    = regexp.MustCompile(`(?s)<currentTrackSectionIndex>(.*?)</currentTrackSectionIndex>`)
	intRE        = regexp.MustCompile(`<int>(-?\d+)</int>`)
)

func ints(s string) []int {
	var out []int
	for _, m := range intRE.FindAllStringSubmatch(s, -1) {
		n, _ := strconv.Atoi(m[1])
		out = append(out, n)
	}
	return out
}

// rawTrain is what one TrainLoader block says, read straight from the XML
// text without using the parser under test.
type rawTrain struct {
	id       string
	sections map[SectionRef]struct{}
	cars     int
	locos    int
}

func readRawTrains(t *testing.T) []rawTrain {
	t.Helper()
	data, err := os.ReadFile(worldSave)
	if err != nil {
		t.Fatal(err)
	}
	var out []rawTrain
	for _, blk := range trainBlockRE.FindAllString(string(data), -1) {
		rt := rawTrain{id: trainIDRE.FindStringSubmatch(blk)[1], sections: map[SectionRef]struct{}{}}
		rt.cars = strings.Count(blk, "<unitType>US_Freightcar</unitType>")
		rt.locos = strings.Count(blk, "<unitType>US_DieselEngine</unitType>")
		ps, ss := prefixRE.FindAllStringSubmatch(blk, -1), sectionRE.FindAllStringSubmatch(blk, -1)
		if len(ps) != len(ss) {
			t.Fatalf("train %s: %d prefix blocks, %d section blocks", rt.id, len(ps), len(ss))
		}
		for i := range ps {
			pv, sv := ints(ps[i][1]), ints(ss[i][1])
			if len(pv) != len(sv) {
				t.Fatalf("train %s: mismatched truck arrays", rt.id)
			}
			for j := range pv {
				rt.sections[SectionRef{pv[j], sv[j]}] = struct{}{}
			}
		}
		out = append(out, rt)
	}
	return out
}

func TestCandidatesOnWorldSaveEmptyMap(t *testing.T) {
	snap, err := run8save.ParseFile(worldSave)
	if err != nil {
		t.Fatal(err)
	}
	raw := readRawTrains(t)
	if len(raw) != len(snap.Trains) || len(raw) == 0 {
		t.Fatalf("raw trains = %d, parsed = %d", len(raw), len(snap.Trains))
	}

	m := New()
	cands := m.Candidates(snap, nil)

	// Every train that has any vehicle is a candidate, in snapshot order.
	var wantIDs, gotIDs []string
	for _, tr := range snap.Trains {
		if len(tr.Vehicles) > 0 {
			wantIDs = append(wantIDs, tr.ID)
		}
	}
	for _, c := range cands {
		gotIDs = append(gotIDs, c.TrainID)
	}
	if !reflect.DeepEqual(gotIDs, wantIDs) {
		t.Fatalf("candidate trains = %v, want %v", gotIDs, wantIDs)
	}

	// Section lists match the raw XML for every train, and the sampled cut
	// (largest train without a locomotive) is checked in full.
	rawByID := map[string]rawTrain{}
	for _, rt := range raw {
		rawByID[rt.id] = rt
	}
	var cut *Candidate
	for i, c := range cands {
		rt, ok := rawByID[c.TrainID]
		if !ok {
			t.Fatalf("candidate %s not in raw XML", c.TrainID)
		}
		if len(c.Sections) != len(rt.sections) {
			t.Errorf("train %s: %d sections, raw XML has %d", c.TrainID, len(c.Sections), len(rt.sections))
		}
		for _, s := range c.Sections {
			if _, ok := rt.sections[s]; !ok {
				t.Errorf("train %s: section %v not in raw XML", c.TrainID, s)
			}
		}
		if c.Cars != rt.cars || c.Powered != (rt.locos > 0) {
			t.Errorf("train %s: cars=%d powered=%v, raw cars=%d locos=%d", c.TrainID, c.Cars, c.Powered, rt.cars, rt.locos)
		}
		if rt.locos == 0 && (cut == nil || c.Cars > cut.Cars) {
			cut = &cands[i]
		}
	}
	if cut == nil {
		t.Fatal("fixture has no unpowered cut")
	}
	rt := rawByID[cut.TrainID]
	var want []SectionRef
	for s := range rt.sections {
		want = append(want, s)
	}
	sortSections(want)
	if !reflect.DeepEqual(cut.Sections, want) {
		t.Fatalf("cut %s sections = %v, want %v", cut.TrainID, cut.Sections, want)
	}
	if len(cut.Samples) == 0 || len(cut.Samples) > 5 {
		t.Fatalf("cut samples = %v", cut.Samples)
	}
	t.Logf("sampled cut %s: %d cars, sections %v, tags %v", cut.TrainID, cut.Cars, cut.Sections, cut.Tags)

	// With an empty map every car is unmapped.
	if cov := m.Coverage(snap); cov.Cars != len(snap.Cars()) || cov.Unmapped != cov.Cars {
		t.Fatalf("coverage = %+v, cars = %d", cov, len(snap.Cars()))
	}

	// Labelling every candidate's sections leaves nothing to learn.
	for _, c := range cands {
		m.Apply(c, Label{Kind: KindIgnored})
	}
	if left := m.Candidates(snap, nil); len(left) != 0 {
		t.Fatalf("%d candidates remain after labelling all", len(left))
	}
	if cov := m.Coverage(snap); cov.Unmapped != 0 {
		t.Fatalf("coverage after labelling = %+v", cov)
	}
}
