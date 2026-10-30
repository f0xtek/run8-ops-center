package carstatus

import (
	"reflect"
	"testing"

	"run8-ops-center/internal/run8save"
)

const gon = "R8_Gon_THR2743_DJJX01.xml"

func TestDiffRetagAndMove(t *testing.T) {
	prev := snap(day(1), train("T1",
		loco("1", "AB45", 100),
		car(gon, "1", "AB45 HTI", 40, 100, 100), // retagged
		car(gon, "2", "AB10 BRR", 0, 100, 100),  // moved
		car(gon, "3", "AB10 BRR", 0, 100, 100),  // spacing only: unchanged
		car(gon, "4", "CHA", 0, 100, 100),       // disappears
	))
	cur := snap(day(2), train("T1",
		loco("1", "AB45", 100),
		car(gon, "1", "NOL1", 0, 100, 100),
		car(gon, "2", "AB10 BRR", 0, 101, 101),
		car(gon, "3", "AB10  BRR", 0, 100, 100),
		car(gon, "5", "None", 0, 100, 100), // appears
	))
	got := Diff(prev, cur)
	want := []Change{
		{Kind: ChangeRetagged, Key: gon + "#1", Number: "DJJX 1", TrainID: "T1", OldTag: "AB45 HTI", NewTag: "NOL1"},
		{Kind: ChangeMoved, Key: gon + "#2", Number: "DJJX 2", TrainID: "T1", NewTag: "AB10 BRR"},
		{Kind: ChangeAppeared, Key: gon + "#5", Number: "DJJX 5", TrainID: "T1", NewTag: "None"},
		{Kind: ChangeDisappeared, Key: gon + "#4", Number: "DJJX 4", TrainID: "T1", OldTag: "CHA"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Diff =\n%+v\nwant\n%+v", got, want)
	}
}

func TestDiffLocomotiveAndTrainChange(t *testing.T) {
	prev := snap(day(1), train("T1", loco("1", "AB45", 100), car(gon, "1", "None", 0, 100, 100)))
	// The loco moves; the car stays but is now in another train (not a change).
	cur := snap(day(2), train("T1", loco("1", "AB45", 101)), train("T2", car(gon, "1", "None", 0, 100, 100)))
	got := Diff(prev, cur)
	if len(got) != 1 || got[0].Kind != ChangeMoved || got[0].Number != "NS 1" || got[0].TrainID != "T1" {
		t.Errorf("Diff = %+v, want one loco move", got)
	}
}

func TestDiffMoveEpsilon(t *testing.T) {
	a := car(gon, "1", "None", 0, 100, 100)
	b := a
	b.Trucks = append([]run8save.Truck(nil), a.Trucks...)
	b.Trucks[0].DistanceM += 0.01 // float noise
	if got := Diff(snap(day(1), train("T", a)), snap(day(2), train("T", b))); len(got) != 0 {
		t.Errorf("sub-metre noise reported: %+v", got)
	}
	b.Trucks[1].DistanceM += 5
	if got := Diff(snap(day(1), train("T", a)), snap(day(2), train("T", b))); len(got) != 1 || got[0].Kind != ChangeMoved {
		t.Errorf("5 m shift: %+v", got)
	}
}

func TestDiffMovedAndRetaggedSameCar(t *testing.T) {
	prev := snap(day(1), train("T", car(gon, "1", "AB45 HTI", 0, 100, 100)))
	cur := snap(day(2), train("T", car(gon, "1", "NOL1", 0, 200, 200)))
	got := Diff(prev, cur)
	if len(got) != 2 || got[0].Kind != ChangeMoved || got[1].Kind != ChangeRetagged {
		t.Errorf("Diff = %+v", got)
	}
}
