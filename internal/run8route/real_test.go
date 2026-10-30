package run8route

import (
	"math"
	"reflect"
	"testing"

	"run8-ops-center/internal/run8route/run8routetest"
)

// The tests here read the git-ignored Run8 files in references/AutoLabel and
// skip when they are absent. They check numbers measured on the AGS route.

func loadReal(t *testing.T) (string, Result) {
	t.Helper()
	root := t.TempDir()
	run8routetest.CopyLocal(t, root)
	res := Load(root, run8routetest.Region, synthRoutes)
	for _, f := range res.Files {
		if !f.OK {
			t.Errorf("%s: %v", f.Name, f.Err)
		}
	}
	return root, res
}

func industryByTag(inds []Industry, tag string) []Industry {
	var out []Industry
	for _, in := range inds {
		if NormTag(in.Tag) == tag {
			out = append(out, in)
		}
	}
	return out
}

func TestRealConfigInd(t *testing.T) {
	_, res := loadReal(t)
	inds := res.Data.Industries
	if len(inds) != 46 {
		t.Fatalf("%d industries, want 46", len(inds))
	}
	brl := industryByTag(inds, "BRL")
	if len(brl) != 1 || len(brl[0].Tracks) != 8 {
		t.Fatalf("BRL = %+v", brl)
	}
	have := map[int]bool{}
	for _, tr := range brl[0].Tracks {
		if tr.Prefix != 280 {
			t.Errorf("BRL track prefix %d", tr.Prefix)
		}
		have[tr.Section] = true
	}
	for _, s := range []int{548, 559, 11776} {
		if !have[s] {
			t.Errorf("BRL lacks 280/%d", s)
		}
	}
	brr := industryByTag(inds, "BRR")
	if len(brr) != 1 || brr[0].Tag != " BRR" {
		t.Fatalf("BRR = %+v", brr)
	}
	found := false
	for _, tr := range brr[0].Tracks {
		found = found || (tr.Prefix == 280 && tr.Section == 8346)
	}
	if !found {
		t.Errorf("BRR tracks = %+v, want 280/8346", brr[0].Tracks)
	}
}

func TestRealTrackDatabase(t *testing.T) {
	_, res := loadReal(t)
	top := res.Data.Topology
	if top == nil {
		t.Fatal("no topology")
	}
	if top.Len() != 10630 || top.SwitchCount() != 990 {
		t.Errorf("%d sections, %d switches; want 10630, 990", top.Len(), top.SwitchCount())
	}
	want := []int{11174, 11199, 11211, 11293, 11303, 11319}
	if got := top.TrackOf(11319); !reflect.DeepEqual(got, want) {
		t.Errorf("TrackOf(11319) = %v, want %v", got, want)
	}
	w, h := top.TileSize()
	if math.Abs(w-844.3)/844.3 > 0.01 || math.Abs(h+1025.8)/1025.8 > 0.01 {
		t.Errorf("tile size = %v by %v, want 844.3 by -1025.8", w, h)
	}
}

// Linked sections must have midpoints within their combined half-lengths, or
// the tile size and the coordinate formula are wrong.
func TestRealCoordinatesAreConsistent(t *testing.T) {
	_, res := loadReal(t)
	top := res.Data.Topology
	if top == nil {
		t.Fatal("no topology")
	}
	checked, far := 0, 0
	for i := range top.secs {
		a := top.secs[i]
		if a.Switch {
			continue
		}
		for _, n := range a.Next {
			b, _ := top.Section(n)
			if b.Switch {
				continue
			}
			checked++
			// Arcs are longer than chords, so allow no more than the sum.
			if math.Hypot(a.X-b.X, a.Z-b.Z) > (a.LengthM+b.LengthM)/2+1 {
				far++
			}
		}
	}
	if checked == 0 || far > 0 {
		t.Errorf("%d of %d linked pairs are farther apart than their lengths allow", far, checked)
	}
}

func TestRealHumpAndLocationsAndMileposts(t *testing.T) {
	_, res := loadReal(t)
	d := res.Data

	if len(d.Hump) != 1 || d.Hump[0].Name != "Norris Yard Hump Controller" || len(d.Hump[0].Tracks) != 56 {
		t.Fatalf("hump = %d controllers", len(d.Hump))
	}
	var one *HumpTrack
	for i := range d.Hump[0].Tracks {
		if d.Hump[0].Tracks[i].Name == "1" {
			one = &d.Hump[0].Tracks[i]
		}
	}
	if one == nil || len(one.Switches) == 0 || one.Switches[len(one.Switches)-1].Section != 11201 {
		t.Errorf("track 1 = %+v, want a path ending at switch 11201", one)
	}
	for _, ht := range d.Hump[0].Tracks {
		for _, s := range ht.Switches {
			if sec, ok := d.Topology.Section(s.Section); !ok || !sec.Switch {
				t.Errorf("track %s: section %d is not a switch", ht.Name, s.Section)
			}
		}
	}

	if len(d.Locations) != 50 {
		t.Errorf("%d locations, want 50", len(d.Locations))
	}
	at := map[string]int{}
	for _, l := range d.Locations {
		if l.Prefix != 280 {
			t.Errorf("location %q prefix %d", l.Name, l.Prefix)
		}
		at[l.Name] = l.Section
	}
	if at["Norris RCV 3 South End"] != 8905 || at["Norris RCV 3 North end"] != 9695 {
		t.Errorf("RCV 3 ends = %d, %d", at["Norris RCV 3 South End"], at["Norris RCV 3 North end"])
	}

	if len(d.Mileposts) != 319 {
		t.Errorf("%d mileposts, want 319", len(d.Mileposts))
	}
	n := 0
	for _, m := range d.Mileposts {
		if NormTag(m.Station) == "Receiving Yard" {
			n++
		}
	}
	if n != 1 {
		t.Errorf("%d stations named Receiving Yard, want 1", n)
	}
}

func TestRealFilesLoadAndStayUntouched(t *testing.T) {
	root, res := loadReal(t)
	if len(res.Files) != 5 || len(res.Fingerprint) != 5 {
		t.Fatalf("%d files, %d fingerprints", len(res.Files), len(res.Fingerprint))
	}
	before := snapshotFiles(t, root)
	Load(root, run8routetest.Region, synthRoutes)
	if after := snapshotFiles(t, root); !reflect.DeepEqual(before, after) {
		t.Error("loading changed a file")
	}
	if !SameFingerprint(res.Fingerprint, Stat(root, run8routetest.Region, synthRoutes)) {
		t.Error("Stat and Load fingerprints differ")
	}
}
