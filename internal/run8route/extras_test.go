package run8route

import (
	"math"
	"reflect"
	"testing"

	"run8-ops-center/internal/run8route/run8routetest"
)

func TestHumpControllersRoundTrip(t *testing.T) {
	got, err := DecodeHumpControllers(run8routetest.EncodeHumpControllers(run8routetest.Synthetic().Hump))
	if err != nil {
		t.Fatal(err)
	}
	want := []HumpController{{
		Name: "Norris Yard Hump Controller",
		Tracks: []HumpTrack{
			{Name: "1", Switches: []HumpSwitch{{201, false}, {204, false}}},
			{Name: "2", Switches: []HumpSwitch{{201, false}, {204, true}}},
			{Name: "3", Switches: []HumpSwitch{{201, true}, {205, false}}},
			{Name: "4", Switches: []HumpSwitch{{201, true}, {205, true}}},
		},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("decoded %+v", got)
	}
}

func TestHumpPathsResolveToBowlTracks(t *testing.T) {
	// The design's rule: the last switch's leg opens the bowl track.
	top := syntheticTopology(t)
	hcs, err := DecodeHumpControllers(run8routetest.EncodeHumpControllers(run8routetest.Synthetic().Hump))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string][]int{"1": {206}, "2": {207, 210}, "3": {208}, "4": {209}}
	for _, ht := range hcs[0].Tracks {
		last := ht.Switches[len(ht.Switches)-1]
		leg, ok := top.Leg(last.Section, last.Reversed)
		if !ok {
			t.Fatalf("track %s: no leg", ht.Name)
		}
		if got := top.TrackOf(leg); !reflect.DeepEqual(got, want[ht.Name]) {
			t.Errorf("track %s = %v, want %v", ht.Name, got, want[ht.Name])
		}
	}
}

func TestSpecialLocationsRoundTrip(t *testing.T) {
	spec := run8routetest.Synthetic()
	got, err := DecodeSpecialLocations(run8routetest.EncodeSpecialLocations(spec.Locations))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(spec.Locations) {
		t.Fatalf("got %d locations", len(got))
	}
	for i, l := range spec.Locations {
		if got[i] != (SpecialLocation{Name: l.Name, Type: l.Type, Prefix: l.Prefix, Section: l.Section}) {
			t.Errorf("location %d = %+v, want %+v", i, got[i], l)
		}
	}
}

func TestMilepostsRoundTrip(t *testing.T) {
	spec := run8routetest.Synthetic()
	got, err := DecodeMileposts(run8routetest.EncodeMileposts(spec.Mileposts), run8routetest.TileW, run8routetest.TileH)
	if err != nil {
		t.Fatal(err)
	}
	want := []Milepost{
		{Name: "MP 1.0", Station: "Receiving  Yard", X: 1000, Z: -1250},
		{Name: "MP 40.0", Station: "Far Junction", X: 6000, Z: -1200},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d mileposts", len(got))
	}
	for i, w := range want {
		g := got[i]
		if g.Name != w.Name || g.Station != w.Station || math.Abs(g.X-w.X) > 1e-3 || math.Abs(g.Z-w.Z) > 1e-3 {
			t.Errorf("milepost %d = %+v, want %+v", i, g, w)
		}
	}
	if NormTag(got[0].Station) != "Receiving Yard" {
		t.Errorf("station %q does not normalise to Receiving Yard", got[0].Station)
	}
}
