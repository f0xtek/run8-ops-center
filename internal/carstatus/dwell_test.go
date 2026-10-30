package carstatus

import (
	"testing"

	"run8-ops-center/internal/trackmap"
)

func TestDwellFirstSeenAndReset(t *testing.T) {
	p, tm := testProfile(), testMap()
	const m = "R8_Gon_X_ABCD01.xml"
	key := m + "#1"

	// First sight: Since is the snapshot's game date.
	st1, d1 := Derive(snap(day(1), train("T", car(m, "1", "None", 0, 100, 100), car(m, "2", "None", 0, 100, 100))), p, tm, nil)
	if got := st1.ByKey[key].DwellSince; !got.Equal(day(1)) {
		t.Fatalf("first seen Since = %v, want %v", got, day(1))
	}

	// Same location, later date: Since is kept. Car 2 leaves the snapshot.
	// Car 3 is new.
	st2, d2 := Derive(snap(day(3), train("T", car(m, "1", "None", 0, 100, 100), car(m, "3", "None", 0, 100, 100))), p, tm, d1)
	if got := st2.ByKey[key].DwellSince; !got.Equal(day(1)) {
		t.Errorf("unmoved Since = %v, want %v", got, day(1))
	}
	if got := st2.ByKey[m+"#3"].DwellSince; !got.Equal(day(3)) {
		t.Errorf("new car Since = %v, want %v", got, day(3))
	}
	if _, ok := d2[m+"#2"]; ok {
		t.Error("car absent from the snapshot must be dropped")
	}
	if len(d1) != 2 {
		t.Errorf("prev state mutated: %v", d1)
	}

	// Yard track to another yard track, then to industry GES: each resets.
	st3, d3 := Derive(snap(day(5), train("T", car(m, "1", "None", 0, 101, 101))), p, tm, d2)
	if got := st3.ByKey[key].DwellSince; !got.Equal(day(5)) {
		t.Errorf("track change Since = %v, want %v", got, day(5))
	}
	st4, d4 := Derive(snap(day(6), train("T", car(m, "1", "None", 0, 201, 201))), p, tm, d3)
	if got := st4.ByKey[key].DwellSince; !got.Equal(day(6)) {
		t.Errorf("industry Since = %v, want %v", got, day(6))
	}
	if d4[key].Loc != "industry:GES" {
		t.Errorf("Loc key = %q", d4[key].Loc)
	}
	if d3[key].Loc != "yard:NOR/Bowl 15" {
		t.Errorf("prev Loc key = %q", d3[key].Loc)
	}
}

func TestDwellIgnoredAndUnmappedDoNotReset(t *testing.T) {
	p, tm := testProfile(), testMap()
	const m = "R8_Gon_X_ABCD01.xml"
	_, d := Derive(snap(day(1), train("T", car(m, "1", "None", 0, 300, 300))), p, tm, nil)
	// A second ignored section is a different section but the same dwell location.
	tm.Set(1, 301, trackmap.Label{Kind: trackmap.KindIgnored})
	st, d := Derive(snap(day(2), train("T", car(m, "1", "None", 0, 301, 301))), p, tm, d)
	if got := st.Cars[0].DwellSince; !got.Equal(day(1)) {
		t.Errorf("ignored->ignored Since = %v, want %v", got, day(1))
	}
	// Unmapped sections are likewise one location.
	_, d = Derive(snap(day(3), train("T", car(m, "1", "None", 0, 900, 900))), p, tm, d)
	st, _ = Derive(snap(day(4), train("T", car(m, "1", "None", 0, 901, 901))), p, tm, d)
	if got := st.Cars[0].DwellSince; !got.Equal(day(3)) {
		t.Errorf("unmapped->unmapped Since = %v, want %v", got, day(3))
	}
}
