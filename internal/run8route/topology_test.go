package run8route

import (
	"math"
	"reflect"
	"testing"

	"run8-ops-center/internal/run8route/run8routetest"
)

func syntheticTopology(t *testing.T) *Topology {
	t.Helper()
	top, err := DecodeTrackDatabase(run8routetest.EncodeTrackDB(run8routetest.Synthetic().Tracks))
	if err != nil {
		t.Fatal(err)
	}
	return top
}

func TestTopologyCounts(t *testing.T) {
	top := syntheticTopology(t)
	if top.Len() != len(run8routetest.Synthetic().Tracks) {
		t.Errorf("Len = %d", top.Len())
	}
	if top.SwitchCount() != 7 {
		t.Errorf("SwitchCount = %d, want 7", top.SwitchCount())
	}
	sw, _ := top.Section(102)
	if !sw.Switch || !reflect.DeepEqual(sw.Next, []int{101, 103, 105}) {
		t.Errorf("section 102 = %+v", sw)
	}
	if s, _ := top.Section(101); s.Switch || s.LengthM != 450 {
		t.Errorf("section 101 = %+v", s)
	}
	if _, ok := top.Section(9999); ok {
		t.Error("unknown section found")
	}
}

func TestTileSizeIsRecovered(t *testing.T) {
	w, h := syntheticTopology(t).TileSize()
	if math.Abs(w-run8routetest.TileW) > 1e-3 || math.Abs(h-run8routetest.TileH) > 1e-3 {
		t.Errorf("TileSize = %v, %v; want %v, %v", w, h, run8routetest.TileW, run8routetest.TileH)
	}
}

func TestTileSizeWithoutSamples(t *testing.T) {
	// Sections that stay inside one tile give no estimate.
	secs := []run8routetest.TrackSection{
		run8routetest.Place(1, []int{2}, false, 10, -10, 50, -10, 800, -1000),
		run8routetest.Place(2, []int{1}, false, 50, -10, 90, -10, 800, -1000),
	}
	top, err := DecodeTrackDatabase(run8routetest.EncodeTrackDB(secs))
	if err != nil {
		t.Fatal(err)
	}
	if w, h := top.TileSize(); w != 0 || h != 0 {
		t.Errorf("TileSize = %v, %v; want 0, 0", w, h)
	}
}

func TestTileSizeIgnoresCurvesAndSwitches(t *testing.T) {
	secs := run8routetest.Synthetic().Tracks
	// A curved section across a tile edge, with wrong geometry for a straight
	// one, must not skew the estimate.
	bad := run8routetest.Place(900, nil, false, 700, -300, 1000, -300, 800, -1000)
	bad.Curve = 5
	bad.Length = 5000
	secs = append(secs, bad)
	top, err := DecodeTrackDatabase(run8routetest.EncodeTrackDB(secs))
	if err != nil {
		t.Fatal(err)
	}
	if w, _ := top.TileSize(); math.Abs(w-800) > 1e-3 {
		t.Errorf("width = %v", w)
	}
}

func TestSectionPositions(t *testing.T) {
	top := syntheticTopology(t)
	for _, c := range []struct {
		idx  int
		x, z float64
	}{
		{101, 625, -500},  // crosses the x tile edge
		{200, 900, -1000}, // crosses the z tile edge
		{206, 900, -1315},
	} {
		s, _ := top.Section(c.idx)
		if math.Abs(s.X-c.x) > 1e-3 || math.Abs(s.Z-c.z) > 1e-3 {
			t.Errorf("section %d midpoint = %v, %v; want %v, %v", c.idx, s.X, s.Z, c.x, c.z)
		}
	}
}

func TestTrackOf(t *testing.T) {
	top := syntheticTopology(t)
	for _, c := range []struct {
		from int
		want []int
	}{
		{100, []int{100, 101}}, // dead end, then a switch
		{101, []int{100, 101}},
		{103, []int{103, 104}}, // switch, then a dead end
		{105, []int{105, 106}},
		{207, []int{207, 210}},
		{210, []int{207, 210}},
		{302, []int{300, 301, 302}},
		{301, []int{300, 301, 302}},
		{320, []int{320}}, // no links at all
		{102, []int{102}}, // a switch is its own track
		{9999, nil},
	} {
		if got := top.TrackOf(c.from); !reflect.DeepEqual(got, c.want) {
			t.Errorf("TrackOf(%d) = %v, want %v", c.from, got, c.want)
		}
	}
}

func TestAdjacent(t *testing.T) {
	top := syntheticTopology(t)
	for _, c := range []struct {
		run  []int
		want []int
	}{
		// The siding meets switch 102; across it lie 101 and 103.
		{[]int{105, 106}, []int{101, 102, 103}},
		// The receiving track meets 340 and 341; across them lie 345, 310,
		// 346 and 311. Nothing is taken across a second switch.
		{[]int{300, 301, 302}, []int{310, 311, 340, 341, 345, 346}},
		{[]int{100, 101}, []int{102, 103, 105}},
		// A dead end links to nothing.
		{[]int{320}, nil},
		{[]int{500}, nil},
	} {
		got := top.Adjacent(c.run)
		if len(got) == 0 && len(c.want) == 0 {
			continue
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("Adjacent(%v) = %v, want %v", c.run, got, c.want)
		}
	}
}

func TestAdjacentCrossesOnlyOneSwitch(t *testing.T) {
	top := syntheticTopology(t)
	// 202 touches 201 and 204, both switches. 200 lies beyond 201, and 206
	// and 207 beyond 204. Nothing farther.
	got := top.Adjacent([]int{202})
	want := []int{200, 201, 203, 204, 206, 207}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Adjacent(202) = %v, want %v", got, want)
	}
}

func TestLeg(t *testing.T) {
	top := syntheticTopology(t)
	for _, c := range []struct {
		sw       int
		reversed bool
		want     int
		ok       bool
	}{
		{201, false, 202, true},
		{201, true, 203, true},
		{204, false, 206, true},
		{204, true, 207, true},
		{205, true, 209, true},
		{101, false, 0, false}, // not a switch
		{9999, false, 0, false},
	} {
		got, ok := top.Leg(c.sw, c.reversed)
		if got != c.want || ok != c.ok {
			t.Errorf("Leg(%d, %v) = %d, %v; want %d, %v", c.sw, c.reversed, got, ok, c.want, c.ok)
		}
	}
}

func TestPath(t *testing.T) {
	top := syntheticTopology(t)
	if p, ok := top.Path(300, 302, 60); !ok || !reflect.DeepEqual(p, []int{300, 301, 302}) {
		t.Errorf("Path(300, 302) = %v, %v", p, ok)
	}
	if p, ok := top.Path(302, 300, 60); !ok || !reflect.DeepEqual(p, []int{302, 301, 300}) {
		t.Errorf("Path(302, 300) = %v, %v", p, ok)
	}
	if p, ok := top.Path(330, 332, 60); !ok || !reflect.DeepEqual(p, []int{330, 331, 332}) {
		t.Errorf("Path(330, 332) = %v, %v", p, ok)
	}
	if p, ok := top.Path(300, 311, 60); !ok || !reflect.DeepEqual(p, []int{300, 340, 310, 311}) {
		t.Errorf("Path(300, 311) = %v, %v", p, ok)
	}
	if p, ok := top.Path(300, 300, 1); !ok || !reflect.DeepEqual(p, []int{300}) {
		t.Errorf("Path(300, 300) = %v, %v", p, ok)
	}
	// The limit counts sections, both ends included.
	if _, ok := top.Path(300, 302, 3); !ok {
		t.Error("a 3-section path must fit a limit of 3")
	}
	if _, ok := top.Path(300, 302, 2); ok {
		t.Error("a 3-section path must not fit a limit of 2")
	}
	if _, ok := top.Path(300, 302, 0); ok {
		t.Error("limit 0 finds nothing")
	}
	// Unreachable and unknown.
	if _, ok := top.Path(300, 320, 60); ok {
		t.Error("320 is not linked to anything")
	}
	if _, ok := top.Path(300, 9999, 60); ok {
		t.Error("unknown target")
	}
}

func TestTrackDatabaseRejectsBadSections(t *testing.T) {
	dup := []run8routetest.TrackSection{
		run8routetest.Place(1, nil, false, 0, 0, 10, 0, 800, -1000),
		run8routetest.Place(1, nil, false, 0, 0, 10, 0, 800, -1000),
	}
	if _, err := DecodeTrackDatabase(run8routetest.EncodeTrackDB(dup)); err == nil {
		t.Error("a repeated section index was accepted")
	}
}
