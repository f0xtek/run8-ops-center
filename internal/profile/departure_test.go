package profile

import (
	"reflect"
	"testing"
)

func norYard(marks map[string]bool) *Yard {
	return &Yard{Code: "NOR", Marks: marks, Tracks: []YardTrack{
		{Name: "WF01", LengthFt: 100, Kind: TrackYard, Departure: true},
		{Name: "Southern Outbound", LengthFt: 100, Kind: TrackYard, Departure: true},
		{Name: "East Pullback", LengthFt: 100, Kind: TrackYard},
		{Name: "Pass", Kind: TrackSiding},
	}}
}

func TestDepartureSetListedDefaultsOnly(t *testing.T) {
	got := norYard(nil).DepartureSet()
	want := map[string]bool{"WF01": true, "SOUTHERN OUTBOUND": true}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("DepartureSet = %v, want %v", got, want)
	}
}

func TestDepartureSetTrueMarkAddsUnlistedName(t *testing.T) {
	got := norYard(map[string]bool{"R07": true, "East Pullback": true}).DepartureSet()
	want := map[string]bool{"WF01": true, "SOUTHERN OUTBOUND": true, "R07": true, "EAST PULLBACK": true}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("DepartureSet = %v, want %v", got, want)
	}
}

func TestDepartureSetFalseMarkRemovesListedDefault(t *testing.T) {
	got := norYard(map[string]bool{"WF01": false}).DepartureSet()
	want := map[string]bool{"SOUTHERN OUTBOUND": true}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("DepartureSet = %v, want %v", got, want)
	}
}

func TestDepartureSetIgnoresCaseAndSpacing(t *testing.T) {
	got := norYard(map[string]bool{"  wf01 ": false, " southern   outbound": false, "r  07": true}).DepartureSet()
	want := map[string]bool{"R 07": true}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("DepartureSet = %v, want %v", got, want)
	}
	if got := norYard(map[string]bool{"WF01": true}).DepartureSet(); !got["WF01"] || len(got) != 2 {
		t.Errorf("a true mark on a listed default changed the set: %v", got)
	}
}

func TestDepartureSetNilWhenEveryDefaultUnmarked(t *testing.T) {
	y := norYard(map[string]bool{"WF01": false, "Southern Outbound": false})
	if got := y.DepartureSet(); got != nil {
		t.Errorf("DepartureSet = %v, want nil", got)
	}
}

func TestDepartureSetNilWithoutDefaultsOrMarks(t *testing.T) {
	y := &Yard{Code: "TUS", Tracks: []YardTrack{{Name: "TY01", Kind: TrackYard}}}
	if got := y.DepartureSet(); got != nil {
		t.Errorf("DepartureSet = %v, want nil", got)
	}
	y.Marks = map[string]bool{"TY01": false}
	if got := y.DepartureSet(); got != nil {
		t.Errorf("DepartureSet with only a false mark = %v, want nil", got)
	}
	y.Marks = map[string]bool{"TY01": true}
	if got := y.DepartureSet(); !reflect.DeepEqual(got, map[string]bool{"TY01": true}) {
		t.Errorf("DepartureSet with a true mark = %v", got)
	}
}
