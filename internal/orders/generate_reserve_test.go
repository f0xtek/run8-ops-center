package orders

import (
	"reflect"
	"testing"

	"run8-ops-center/internal/carstatus"
)

func gtActive(id string, kind Kind, state State, consist []CarRef, lines ...Line) Order {
	return Order{ID: id, Kind: kind, State: state, Consist: consist, Lines: lines}
}

func TestGenerateReserveExcludesCarsOfReservedOrders(t *testing.T) {
	cars := gtMany("j", 4, gtBox, gtInd("AB45", "JPI"), "NOR", 1, 60)
	st := gtStatus(cars)
	started := gtActive("ord-1", KindLocal, StateInProgress, nil,
		Line{Kind: LineIndustry, Target: "JPI", SetOuts: []CarRef{{Key: "ja"}, {Key: "jb"}}})
	l := gtLine(gtOrder(gtGen(gtProfile(), st, started), "avail:ab45"), "industry:JPI")
	if l == nil || !reflect.DeepEqual(gtKeys(l.SetOuts), []string{"jc", "jd"}) {
		t.Fatalf("JPI line = %+v, want only the unreserved cars", l)
	}
	started.State = StateCompletionDetected
	l = gtLine(gtOrder(gtGen(gtProfile(), st, started), "avail:ab45"), "industry:JPI")
	if l == nil || len(l.SetOuts) != 2 {
		t.Fatalf("completion-detected orders must also reserve: %+v", l)
	}
}

func TestGenerateReserveIgnoresNonReservedStates(t *testing.T) {
	cars := gtMany("j", 2, gtBox, gtInd("AB45", "JPI"), "NOR", 1, 60)
	for _, s := range []State{StateAvailable, StateCompleted, StateCancelled} {
		o := gtActive("ord-1", KindLocal, s, nil,
			Line{Kind: LineIndustry, Target: "JPI", SetOuts: []CarRef{{Key: "ja"}, {Key: "jb"}}})
		l := gtLine(gtOrder(gtGen(gtProfile(), gtStatus(cars), o), "avail:ab45"), "industry:JPI")
		if l == nil || len(l.SetOuts) != 2 {
			t.Errorf("state %v reserved cars: %+v", s, l)
		}
	}
}

func TestGenerateReserveExcludesPickUpsAndConsistCars(t *testing.T) {
	cars := append(gtMany("n", 9, gtBox, gtTrain("NOL1", "131"), "NOR", 1, 500),
		gtAt("p1", gtBox, gtTrain("CHA", "108"), "WRG", carstatus.ServiceReadyToPull, 1, 60),
		gtAt("p2", gtBox, gtTrain("CHA", "108"), "WRG", carstatus.ServiceReadyToPull, 1, 60),
	)
	started := gtActive("ord-7", KindRoad, StateInProgress, []CarRef{{Key: "na"}},
		Line{Kind: LineIndustry, Target: "WRG", PickUps: []CarRef{{Key: "p1"}}})
	res := gtGen(gtProfile(), gtStatus(cars), started)
	if o := gtOrder(res, "avail:131"); o == nil || len(o.Consist) != 8 || o.Consist[0].Key == "na" {
		t.Errorf("131 consist = %+v, want 8 cars without na", o)
	}
	if l := gtLine(gtOrder(res, "avail:ab10"), "industry:WRG"); l == nil || !reflect.DeepEqual(gtKeys(l.PickUps), []string{"p2"}) {
		t.Errorf("WRG line = %+v", l)
	}
}

func TestGenerateReserveStillGatesIndustryOnReservedWorkingCars(t *testing.T) {
	// An in-progress order has just spotted a car at GES; it is working, so
	// no second delivery line appears even though that car is reserved.
	cars := []carstatus.Car{
		gtAt("w", gtBox, gtInd("A39", "GES"), "GES", carstatus.ServiceWorking, 1, 60),
		gtWait("n", gtBox, gtInd("A39", "GES"), "MCC", 1, 60),
	}
	started := gtActive("ord-1", KindLocal, StateInProgress, nil,
		Line{Kind: LineIndustry, Target: "GES", SetOuts: []CarRef{{Key: "w"}}})
	if o := gtOrder(gtGen(gtProfile(), gtStatus(cars), started), "avail:a39"); o != nil {
		t.Errorf("GES line while a spotted car is working: %+v", o.Lines)
	}
}

func TestGenerateStableIDsAcrossRegenerations(t *testing.T) {
	cars := append(gtMany("j", 3, gtBox, gtInd("AB45", "JPI"), "NOR", 1, 60),
		gtMany("n", 9, gtBox, gtTrain("NOL1", "131"), "NOR", 1, 500)...)
	st := gtStatus(cars)
	a, b := gtGen(gtProfile(), st), gtGen(gtProfile(), st)
	if !reflect.DeepEqual(a, b) {
		t.Fatal("two runs on the same input differ")
	}
	var ids []string
	for _, o := range a.Orders {
		ids = append(ids, o.ID)
	}
	if want := []string{"avail:ab45", "avail:131"}; !reflect.DeepEqual(ids, want) {
		t.Fatalf("IDs = %v, want %v", ids, want)
	}
	// Three more cars arrive: same order identity, updated line.
	more := append(append([]carstatus.Car(nil), cars...), gtMany("m", 3, gtBox, gtInd("AB45", "JPI"), "NOR", 1, 60)...)
	c := gtGen(gtProfile(), gtStatus(more))
	oa, oc := gtOrder(a, "avail:ab45"), gtOrder(c, "avail:ab45")
	if oc == nil || oc.ID != oa.ID || len(gtLine(oc, "industry:JPI").SetOuts) != 6 || len(gtLine(oa, "industry:JPI").SetOuts) != 3 {
		t.Errorf("order not updated in place: %+v", oc)
	}
}

func TestGenerateSharedBRICarsOn131And339(t *testing.T) {
	// 2,000 ft of BRI BRI for each train's 50% of 8,000 would be short, so
	// give them 4,000 ft: both trains qualify on the shared cars alone.
	bri := gtMany("b", 4, gtBox, gtInd("BRI", "BRI"), "NOR", 1, 1000)
	st := gtStatus(bri)
	res := gtGen(gtProfile(), st)
	o131, o339 := gtOrder(res, "avail:131"), gtOrder(res, "avail:339")
	if o131 == nil || o339 == nil {
		t.Fatalf("131=%v 339=%v, want both available", o131, o339)
	}
	want := []string{"ba", "bb", "bc", "bd"}
	for _, o := range []*Order{o131, o339} {
		if !reflect.DeepEqual(o.Shared, want) || !gtFlag(o, "shared-cars") {
			t.Errorf("%s: Shared=%v flags=%v", o.ID, o.Shared, o.Flags)
		}
	}
	// Starting 131 reserves the cars: 339 loses them and the overlap is gone.
	started := gtActive("ord-1", KindRoad, StateInProgress, gtRefsOf(bri))
	res = gtGen(gtProfile(), st, started)
	if len(res.Orders) != 0 {
		t.Errorf("orders after 131 started = %+v, want none", res.Orders)
	}
	// With extra NOL1 cars 131 stays available, without shared cars.
	nol := gtMany("n", 9, gtBox, gtTrain("NOL1", "131"), "NOR", 1, 500)
	res = gtGen(gtProfile(), gtStatus(bri, nol), started)
	o := gtOrder(res, "avail:131")
	if o == nil || len(o.Shared) != 0 || gtFlag(o, "shared-cars") || len(o.Consist) != 9 {
		t.Errorf("131 after start = %+v", o)
	}
	if gtOrder(res, "avail:339") != nil {
		t.Error("339 still available after its cars were reserved")
	}
}

func gtRefsOf(cars []carstatus.Car) []CarRef {
	var out []CarRef
	for _, c := range cars {
		out = append(out, CarRef{Key: c.Key})
	}
	return out
}

func TestGenerateNilInputs(t *testing.T) {
	if r := Generate(GenInput{}); len(r.Orders) != 0 || len(r.Building) != 0 {
		t.Errorf("empty input produced %+v", r)
	}
}
