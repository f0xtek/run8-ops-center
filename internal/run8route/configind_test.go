package run8route

import (
	"reflect"
	"testing"

	"run8-ops-center/internal/run8route/run8routetest"
)

func TestConfigIndRoundTrip(t *testing.T) {
	spec := run8routetest.Synthetic()
	got, err := DecodeConfigInd(run8routetest.EncodeConfigInd(spec.Industries))
	if err != nil {
		t.Fatal(err)
	}
	want := []Industry{
		{Name: "Synthetic Rail & Locomotive", Local: "AB45", Tag: "BRL",
			Tracks: []IndustryTrack{{280, 105, 0}, {280, 106, 1}}},
		{Name: "Padded Tag Works", Local: "AB10", Tag: " BBB",
			Tracks: []IndustryTrack{{280, 500, 0}}},
		{Name: "Unknown Industry", Local: "AB10", Tag: "ZZZ",
			Tracks: []IndustryTrack{{280, 510, 0}}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("decoded %+v\nwant %+v", got, want)
	}
	if got[1].Tag == NormTag(got[1].Tag) || NormTag(got[1].Tag) != "BBB" {
		t.Errorf("tag %q must stay raw and normalise to BBB", got[1].Tag)
	}
}

func TestConfigIndCarEntriesKeepRecordsAligned(t *testing.T) {
	// An industry with several car entries, tags and filters sits before
	// another industry; the second one only decodes if the first was consumed
	// exactly.
	inds := []run8routetest.Industry{
		{Name: "First", Local: "L", Tag: "AAA", Cars: []run8routetest.IndustryCar{
			{Version: 3, CarType: 2, ProducesEmpties: true, Hours: 48, Capacity: 9,
				Tags: []string{"X", " Y ", "Z"}, Filters: []string{"R8_Box_", "", "R8_Gon_"}},
			{Version: 3},
		}},
		{Name: "Second", Tag: "BBB", Tracks: []run8routetest.IndustryTrack{{Prefix: 280, Section: 7, Node: 1}}},
	}
	got, err := DecodeConfigInd(run8routetest.EncodeConfigInd(inds))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[1].Name != "Second" || len(got[1].Tracks) != 1 || got[1].Tracks[0] != (IndustryTrack{280, 7, 1}) {
		t.Errorf("decoded %+v", got)
	}
}

func TestConfigIndEmpty(t *testing.T) {
	got, err := DecodeConfigInd(run8routetest.EncodeConfigInd([]run8routetest.Industry{}))
	if err != nil || len(got) != 0 {
		t.Errorf("empty file: %v, %v", got, err)
	}
}

func TestNormTag(t *testing.T) {
	for in, want := range map[string]string{" BRR": "BRR", "AB10  BRR": "AB10 BRR", "  ": "", "NOR Hump": "NOR Hump"} {
		if got := NormTag(in); got != want {
			t.Errorf("NormTag(%q) = %q, want %q", in, got, want)
		}
	}
}
