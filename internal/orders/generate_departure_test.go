package orders

import (
	"math"
	"reflect"
	"testing"

	"run8-ops-center/internal/carstatus"
	"run8-ops-center/internal/profile"
)

// gtDepartureProfile is gtProfile with Norris marking departure tracks, like
// the bundled AGS profile. Every other yard lists none.
func gtDepartureProfile() *profile.Profile {
	p := gtProfile()
	nor := p.YardByCode("NOR")
	for _, n := range []string{"EF05", "WF03", "L08"} {
		nor.Tracks = append(nor.Tracks, profile.YardTrack{Name: n, LengthFt: 5000, Kind: profile.TrackYard, Departure: true})
	}
	nor.Tracks = append(nor.Tracks, profile.YardTrack{Name: "Pass", Kind: profile.TrackSiding})
	return p
}

// gtOn builds a car waiting on a named track of a yard.
func gtOn(key string, typ profile.CarType, d carstatus.Dest, yard, track string, age int, ft float64) carstatus.Car {
	c := gtWait(key, typ, d, yard, age, ft)
	c.Loc.Track = track
	return c
}

func TestDepartureBowlCarIsNotASetOutAndWF03Is(t *testing.T) {
	bowl := gtOne(gtOn("b", gtBox, gtInd("AB45", "JPI"), "NOR", "Bowl 14", 1, 60))
	res := gtGen(gtDepartureProfile(), gtStatus(bowl))
	if o := gtOrder(res, "avail:ab45"); o != nil {
		t.Fatalf("AB45 order for a bowl car: %+v", o)
	}

	wf := gtOne(gtOn("b", gtBox, gtInd("AB45", "JPI"), "NOR", "WF03", 1, 60))
	l := gtLine(gtOrder(gtGen(gtDepartureProfile(), gtStatus(wf)), "avail:ab45"), "industry:JPI")
	if l == nil || !reflect.DeepEqual(gtKeys(l.SetOuts), []string{"b"}) {
		t.Fatalf("JPI line = %+v, want the WF03 car", l)
	}
}

func TestDepartureTrackNameIgnoresCaseAndSpacing(t *testing.T) {
	cars := gtOne(gtOn("b", gtBox, gtInd("AB45", "JPI"), "NOR", " wf03 ", 1, 60))
	if o := gtOrder(gtGen(gtDepartureProfile(), gtStatus(cars)), "avail:ab45"); o == nil {
		t.Error("a car on ' wf03 ' is not a set-out")
	}
}

func TestDepartureReceivingCarsDoNotBuildARoadTrain(t *testing.T) {
	// 3,000 ft on R07 and 1,200 ft on EF05 for 108, against 8,000 ft at 50%.
	cars := append(
		gtMany("r", 3, gtBox, gtTrain("CHA", "108"), "NOR", 1, 1000),
		gtMany("e", 2, gtBox, gtTrain("CHA", "108"), "NOR", 1, 600)...)
	for i := range cars {
		cars[i].Loc.Track = "R07"
		if cars[i].Key[0] == 'e' {
			cars[i].Loc.Track = "EF05"
		}
	}

	res := gtGen(gtDepartureProfile(), gtStatus(cars))
	if o := gtOrder(res, "avail:108"); o != nil {
		t.Fatalf("108 order available: %+v", o)
	}
	b := gtBuild(res, "108")
	if b == nil || b.Ready || b.WaitingFt != 1200 || math.Round(b.Pct) != 15 {
		t.Fatalf("108 build = %+v, want 1,200 ft at 15%%", b)
	}

	p := gtDepartureProfile()
	p.YardByCode("NOR").Marks = map[string]bool{"R07": true}
	res = gtGen(p, gtStatus(cars))
	b = gtBuild(res, "108")
	if b == nil || !b.Ready || b.WaitingFt != 4200 || math.Round(b.Pct) != 53 {
		t.Fatalf("108 build with R07 marked = %+v, want 4,200 ft at 53%%", b)
	}
	if o := gtOrder(res, "avail:108"); o == nil {
		t.Fatal("no 108 order with R07 marked")
	}
}

func TestDepartureUnmarkedTrackLeavesTheRoadTrainBuild(t *testing.T) {
	cars := gtMany("e", 5, gtBox, gtTrain("NOL1", "131"), "NOR", 1, 1000)
	for i := range cars {
		cars[i].Loc.Track = "EF05"
	}
	p := gtDepartureProfile()
	if o := gtOrder(gtGen(p, gtStatus(cars)), "avail:131"); o == nil {
		t.Fatal("no 131 order on EF05")
	}
	p.YardByCode("NOR").Marks = map[string]bool{"EF05": false}
	// With EF05 unmarked, WF03 and L08 still count, so NOR keeps its rule and
	// the cars no longer count.
	res := gtGen(p, gtStatus(cars))
	if o := gtOrder(res, "avail:131"); o != nil {
		t.Errorf("131 order with EF05 unmarked: %+v", o)
	}
	if b := gtBuild(res, "131"); b == nil || b.WaitingFt != 0 {
		t.Errorf("131 build = %+v, want nothing waiting", b)
	}
}

func TestDepartureUnlistedNorrisTrackIsNotOnTheTuscaloosaLine(t *testing.T) {
	all := gtOne(gtOn("c", gtBox, gtInd("A37", "CYC"), "NOR", "All", 1, 60))
	if o := gtOrder(gtGen(gtDepartureProfile(), gtStatus(all)), "avail:a55"); o != nil {
		t.Fatalf("A55 order for a car on NOR / All: %+v", o)
	}
	wf := gtOne(gtOn("c", gtBox, gtInd("A37", "CYC"), "NOR", "WF03", 1, 60))
	l := gtLine(gtOrder(gtGen(gtDepartureProfile(), gtStatus(wf)), "avail:a55"), "yard:TUS")
	if l == nil || !reflect.DeepEqual(gtKeys(l.SetOuts), []string{"c"}) {
		t.Fatalf("A55 Tuscaloosa line = %+v, want the WF03 car", l)
	}
}

func TestDepartureLocalYardCarIsAnAB10SetOut(t *testing.T) {
	cars := gtOne(gtOn("w", gtBox, gtInd("AB10", "WRG"), "NOR", "L08", 1, 60))
	l := gtLine(gtOrder(gtGen(gtDepartureProfile(), gtStatus(cars)), "avail:ab10"), "industry:WRG")
	if l == nil || !reflect.DeepEqual(gtKeys(l.SetOuts), []string{"w"}) {
		t.Fatalf("AB10 WRG line = %+v, want the L08 car", l)
	}
}

func TestDepartureYardWithoutMarksStillYieldsPickUps(t *testing.T) {
	cars := []carstatus.Car{
		gtOn("sel", gtBox, gtTrain("SEL", "345"), "TUS", "TY04", 2, 60),
		gtOn("nol", gtBox, gtTrain("NOL3", "339"), "TUS", "Anything", 1, 60),
	}
	l := gtLine(gtOrder(gtGen(gtDepartureProfile(), gtStatus(cars)), "avail:a55"), "yard:TUS")
	if l == nil || !reflect.DeepEqual(gtKeys(l.PickUps), []string{"sel", "nol"}) {
		t.Fatalf("Tuscaloosa line = %+v, want both pick-ups", l)
	}
}

func TestDepartureYardLinePickUpsUseTheLineYardRule(t *testing.T) {
	p := gtDepartureProfile()
	p.YardByCode("TUS").Tracks = []profile.YardTrack{{Name: "TY04", Kind: profile.TrackYard, Departure: true}}
	cars := []carstatus.Car{
		gtOn("on", gtBox, gtTrain("SEL", "345"), "TUS", "TY04", 2, 60),
		gtOn("off", gtBox, gtTrain("SEL", "345"), "TUS", "TY05", 1, 60),
	}
	l := gtLine(gtOrder(gtGen(p, gtStatus(cars)), "avail:a55"), "yard:TUS")
	if l == nil || !reflect.DeepEqual(gtKeys(l.PickUps), []string{"on"}) {
		t.Fatalf("Tuscaloosa line = %+v, want the TY04 pick-up only", l)
	}
}

func TestDepartureYardStopPickUpsUseTheStopYardRule(t *testing.T) {
	p := gtDepartureProfile()
	p.YardByCode("MCC").Tracks = []profile.YardTrack{{Name: "MST1", Kind: profile.TrackYard, Departure: true}}
	cars := []carstatus.Car{
		gtOn("on", gtAuto, gtTrain("ATL", "346"), "MCC", "MST1", 1, 90),
		gtOn("off", gtAuto, gtTrain("ATL", "346"), "MCC", "MST3", 1, 90),
	}
	l := gtLine(gtOrder(gtGen(p, gtStatus(cars)), "avail:346"), "stop:MCC")
	if l == nil || !reflect.DeepEqual(gtKeys(l.PickUps), []string{"on"}) {
		t.Fatalf("346 MCC stop = %+v, want the MST1 rack only", l)
	}
}

func TestDepartureEveryTrackCountsWhenAllUnmarked(t *testing.T) {
	p := gtDepartureProfile()
	p.YardByCode("NOR").Marks = map[string]bool{"EF05": false, "WF03": false, "L08": false}
	cars := gtOne(gtOn("b", gtBox, gtInd("AB45", "JPI"), "NOR", "Bowl 14", 1, 60))
	if o := gtOrder(gtGen(p, gtStatus(cars)), "avail:ab45"); o == nil {
		t.Error("a bowl car does not count once every Norris departure track is unmarked")
	}
}

func TestDepartureUnmarkedProfileKeepsEveryTrack(t *testing.T) {
	cars := gtOne(gtOn("b", gtBox, gtInd("AB45", "JPI"), "NOR", "Bowl 14", 1, 60))
	if o := gtOrder(gtGen(gtProfile(), gtStatus(cars)), "avail:ab45"); o == nil {
		t.Error("a profile with no departure tracks dropped a bowl car")
	}
}
