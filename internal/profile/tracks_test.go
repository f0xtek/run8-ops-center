package profile

import (
	"reflect"
	"strings"
	"testing"
)

// tracksProfile loads miniYAML with its yards replaced, so each test controls
// the yard track lists.
func tracksProfile(t *testing.T, yards string) *Profile {
	t.Helper()
	yaml := strings.Replace(miniYAML, "yards:\n  - {code: HOM, name: Home}\n  - {code: SAT, name: Satellite}\n", "yards:\n"+yards, 1)
	if yaml == miniYAML {
		t.Fatal("miniYAML yards block not found")
	}
	p, err := Load(writeProfile(t, yaml, miniCSV, miniTSV))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestYardTracksLoadInOrderWithDefaultKind(t *testing.T) {
	p := tracksProfile(t, `
  - code: HOM
    name: Home
    tracks:
      - {name: TY01, lengthFt: 1379}
      - {name: Pass, lengthFt: 752, kind: siding}
      - {name: "  Main   1 ", kind: main}
      - {name: Mystery, kind: Yard}
  - {code: SAT, name: Satellite}
`)
	want := []YardTrack{
		{Name: "TY01", LengthFt: 1379, Kind: TrackYard},
		{Name: "Pass", LengthFt: 752, Kind: TrackSiding},
		{Name: "Main 1", LengthFt: 0, Kind: TrackMain},
		{Name: "Mystery", LengthFt: 0, Kind: TrackYard},
	}
	if got := p.YardByCode("HOM").Tracks; !reflect.DeepEqual(got, want) {
		t.Errorf("Tracks = %+v, want %+v", got, want)
	}
	if got := p.YardByCode("SAT").Tracks; len(got) != 0 {
		t.Errorf("SAT Tracks = %+v, want none", got)
	}
	for _, code := range []string{CodeInvalidTrack, CodeDuplicateTrack} {
		if got := problemsOf(p, code); len(got) != 0 {
			t.Errorf("%s problems: %+v", code, got)
		}
	}
}

func TestYardTracksDropBadEntriesAndKeepTheRest(t *testing.T) {
	p := tracksProfile(t, `
  - code: HOM
    name: Home
    tracks:
      - {name: Shop 1, lengthFt: 585}
      - {name: "", lengthFt: 100}
      - {name: Neg, lengthFt: -5}
      - {name: Odd, lengthFt: 100, kind: spur}
      - {name: "Shop  1", lengthFt: 999}
      - {name: shop 1, lengthFt: 1}
      - {name: Last, lengthFt: 50}
  - code: SAT
    name: Satellite
    tracks:
      - {name: Shop 1, lengthFt: 20}
`)
	wantHom := []YardTrack{
		{Name: "Shop 1", LengthFt: 585, Kind: TrackYard},
		{Name: "Last", LengthFt: 50, Kind: TrackYard},
	}
	if got := p.YardByCode("HOM").Tracks; !reflect.DeepEqual(got, wantHom) {
		t.Errorf("HOM Tracks = %+v, want %+v", got, wantHom)
	}
	// The same name in another yard is fine.
	if got := p.YardByCode("SAT").Tracks; len(got) != 1 || got[0].Name != "Shop 1" {
		t.Errorf("SAT Tracks = %+v, want Shop 1", got)
	}

	invalid := problemsOf(p, CodeInvalidTrack)
	if len(invalid) != 3 {
		t.Fatalf("invalid-track problems = %+v, want 3 (empty name, negative length, unknown kind)", invalid)
	}
	for i, sub := range []string{"HOM", "Neg", "Odd"} {
		if invalid[i].Subject != sub {
			t.Errorf("invalid-track[%d].Subject = %q, want %q", i, invalid[i].Subject, sub)
		}
	}
	dup := problemsOf(p, CodeDuplicateTrack)
	if len(dup) != 2 || dup[0].Subject != "Shop 1" || dup[1].Subject != "shop 1" {
		t.Errorf("duplicate-track problems = %+v, want the second and third Shop 1 entries", dup)
	}
}

func TestYardTrackDepartureFlagLoadsOnYardTracks(t *testing.T) {
	p := tracksProfile(t, `
  - code: HOM
    name: Home
    tracks:
      - {name: WF01, lengthFt: 100, departure: true}
      - {name: WF02, lengthFt: 100, kind: yard, departure: true}
      - {name: WF03, lengthFt: 100, departure: false}
      - {name: WF04, lengthFt: 100}
  - {code: SAT, name: Satellite}
`)
	var got []bool
	for _, tr := range p.YardByCode("HOM").Tracks {
		got = append(got, tr.Departure)
	}
	if want := []bool{true, true, false, false}; !reflect.DeepEqual(got, want) {
		t.Errorf("Departure flags = %v, want %v", got, want)
	}
	if bad := problemsOf(p, CodeInvalidTrack); len(bad) != 0 {
		t.Errorf("invalid-track problems = %+v, want none", bad)
	}
}

func TestYardTrackDepartureFlagDroppedOnSidingAndMain(t *testing.T) {
	p := tracksProfile(t, `
  - code: HOM
    name: Home
    tracks:
      - {name: Pass, lengthFt: 752, kind: siding, departure: true}
      - {name: Main 1, kind: main, departure: true}
      - {name: Shop 1, lengthFt: 585, departure: true}
  - {code: SAT, name: Satellite}
`)
	want := []YardTrack{
		{Name: "Pass", LengthFt: 752, Kind: TrackSiding},
		{Name: "Main 1", Kind: TrackMain},
		{Name: "Shop 1", LengthFt: 585, Kind: TrackYard, Departure: true},
	}
	if got := p.YardByCode("HOM").Tracks; !reflect.DeepEqual(got, want) {
		t.Errorf("Tracks = %+v, want %+v", got, want)
	}
	invalid := problemsOf(p, CodeInvalidTrack)
	if len(invalid) != 2 || invalid[0].Subject != "Pass" || invalid[1].Subject != "Main 1" {
		t.Fatalf("invalid-track problems = %+v, want Pass and Main 1", invalid)
	}
}

func TestTrackByName(t *testing.T) {
	y := &Yard{Tracks: []YardTrack{
		{Name: "TY08", LengthFt: 710, Kind: TrackYard},
		{Name: "Pass", LengthFt: 752, Kind: TrackSiding},
	}}
	got, ok := y.TrackByName("  pass ")
	if !ok || got.Name != "Pass" || got.Kind != TrackSiding {
		t.Errorf("TrackByName(pass) = %+v, %v", got, ok)
	}
	if got, ok := y.TrackByName("ty08"); !ok || got.LengthFt != 710 {
		t.Errorf("TrackByName(ty08) = %+v, %v", got, ok)
	}
	if got, ok := y.TrackByName("TY09"); ok || got != (YardTrack{}) {
		t.Errorf("TrackByName(TY09) = %+v, %v, want zero and false", got, ok)
	}
	if _, ok := (&Yard{}).TrackByName("Pass"); ok {
		t.Error("a yard without a list found a track")
	}
}

// TestBundledYardTracks checks the bundled AGS lists against design D5 of the
// add-yard-track-hints change.
func TestBundledYardTracks(t *testing.T) {
	p, _ := installBundled(t)
	for _, code := range []string{CodeInvalidTrack, CodeDuplicateTrack} {
		if got := problemsOf(p, code); len(got) != 0 {
			t.Errorf("%s problems: %+v", code, got)
		}
	}

	wantCount := map[string]int{"NOR": 70, "32ND": 8, "TUS": 12, "BES": 7, "MCC": 8}
	for code, n := range wantCount {
		y := p.YardByCode(code)
		if y == nil {
			t.Errorf("yard %s missing", code)
			continue
		}
		if len(y.Tracks) != n {
			t.Errorf("%s lists %d tracks, want %d", code, len(y.Tracks), n)
		}
	}

	tus := p.YardByCode("TUS")
	wantTUS := []YardTrack{
		{Name: "TY01", LengthFt: 1379, Kind: TrackYard},
		{Name: "TY02", LengthFt: 877, Kind: TrackYard},
		{Name: "TY04", LengthFt: 334, Kind: TrackYard},
		{Name: "TY05", LengthFt: 376, Kind: TrackYard},
		{Name: "TY06", LengthFt: 543, Kind: TrackYard},
		{Name: "TY07", LengthFt: 500, Kind: TrackYard},
		{Name: "TY08", LengthFt: 710, Kind: TrackYard},
		{Name: "TM01", LengthFt: 1081, Kind: TrackYard},
		{Name: "TS01", LengthFt: 1044, Kind: TrackYard},
		{Name: "TAGS", LengthFt: 1500, Kind: TrackYard},
		{Name: "Pass", LengthFt: 752, Kind: TrackSiding},
		{Name: "Chevron", LengthFt: 167, Kind: TrackYard},
	}
	if !reflect.DeepEqual(tus.Tracks, wantTUS) {
		t.Errorf("TUS tracks = %+v\nwant %+v", tus.Tracks, wantTUS)
	}

	nor := p.YardByCode("NOR")
	for _, tr := range nor.Tracks {
		name := strings.ToUpper(tr.Name)
		if (len(name) == 3 && name[0] == 'R' && name[1] >= '0' && name[1] <= '9' && name[2] >= '0' && name[2] <= '9') ||
			strings.Contains(name, "RECEIVING") {
			t.Errorf("NOR lists receiving track %q", tr.Name)
		}
	}
	for name, kind := range map[string]TrackKind{
		"East Thoroughfare":      TrackMain,
		"Old AGS Main":           TrackMain,
		"Engine Thoroughfare 01": TrackMain,
		"Engine Thoroughfare 02": TrackMain,
		"Shop Thoroughfare West": TrackMain,
		"Shop Thoroughfare East": TrackMain,
		"WF01":                   TrackYard,
		"Engine Shop 6":          TrackYard,
		"L14":                    TrackYard,
	} {
		if tr, ok := nor.TrackByName(name); !ok || tr.Kind != kind {
			t.Errorf("NOR %q = %+v, %v, want kind %q", name, tr, ok, kind)
		}
	}
	if tr, _ := nor.TrackByName("WF01"); tr.LengthFt != 11448 {
		t.Errorf("WF01 length = %d", tr.LengthFt)
	}

	for code, kinds := range map[string]map[TrackKind]int{
		"32ND": {TrackYard: 5, TrackMain: 3},
		"TUS":  {TrackYard: 11, TrackSiding: 1},
		"BES":  {TrackYard: 4, TrackSiding: 1, TrackMain: 2},
		"MCC":  {TrackYard: 5, TrackSiding: 2, TrackMain: 1},
	} {
		got := map[TrackKind]int{}
		for _, tr := range p.YardByCode(code).Tracks {
			got[tr.Kind]++
		}
		if !reflect.DeepEqual(got, kinds) {
			t.Errorf("%s kinds = %v, want %v", code, got, kinds)
		}
	}

	for _, y := range p.Yards {
		seen := map[string]bool{}
		for _, tr := range y.Tracks {
			if tr.Kind == TrackYard && tr.LengthFt == 0 {
				t.Errorf("%s yard track %q has no length", y.Code, tr.Name)
			}
			if k := strings.ToLower(tr.Name); seen[k] {
				t.Errorf("%s lists %q twice", y.Code, tr.Name)
			} else {
				seen[k] = true
			}
		}
	}

	var departures []string
	for _, y := range p.Yards {
		for _, tr := range y.Tracks {
			if !tr.Departure {
				continue
			}
			if y.Code != "NOR" {
				t.Errorf("%s marks departure track %q, only NOR may", y.Code, tr.Name)
			}
			departures = append(departures, tr.Name)
		}
	}
	wantDeparture := []string{
		"EF01", "EF02", "EF03", "EF04", "EF05", "EF06", "EF07", "Southern Outbound",
		"WF01", "WF02", "WF03", "WF04", "WF05", "WF06", "WF07", "CAB 01", "CAB 02",
		"L02", "L03", "L04", "L05", "L06", "L07", "L08", "L09", "L10", "L11", "L12", "L13", "L14",
	}
	if len(wantDeparture) != 30 || !reflect.DeepEqual(departures, wantDeparture) {
		t.Errorf("departure tracks = %v (%d), want the 30 in %v", departures, len(departures), wantDeparture)
	}
}
