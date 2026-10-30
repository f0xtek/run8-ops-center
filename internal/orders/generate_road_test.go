package orders

import (
	"math"
	"reflect"
	"testing"

	"run8-ops-center/internal/carstatus"
	"run8-ops-center/internal/profile"
)

func TestGenerateRoadBelowThresholdIsBuilding(t *testing.T) {
	// 3,000 ft for 131 against 8,000 ft at a 50% minimum.
	cars := gtMany("n", 5, gtBox, gtTrain("NOL1", "131"), "NOR", 1, 600)
	res := gtGen(gtProfile(), gtStatus(cars))
	if o := gtOrder(res, "avail:131"); o != nil {
		t.Fatalf("131 order available below threshold: %+v", o)
	}
	b := gtBuild(res, "131")
	if b == nil || b.Ready || b.WaitingFt != 3000 || b.MaxLengthFt != 8000 || b.ThresholdPct != 50 || b.Origin != "NOR" {
		t.Fatalf("131 build = %+v", b)
	}
	if math.Round(b.Pct) != 38 || b.Pct != 37.5 {
		t.Errorf("Pct = %v, want 37.5 (38%%)", b.Pct)
	}
	if b.Destination != "Atlanta" {
		t.Errorf("Destination = %q", b.Destination)
	}
}

func TestGenerateRoadBuildingListsEveryOriginTrain(t *testing.T) {
	res := gtGen(gtProfile(), gtStatus())
	var syms []string
	for _, b := range res.Building {
		syms = append(syms, b.Symbol)
	}
	if want := []string{"131", "339", "108", "341", "A28"}; !reflect.DeepEqual(syms, want) {
		t.Errorf("Building symbols = %v, want %v (through trains excluded)", syms, want)
	}
}

func TestGenerateRoadThresholdReachedAndCapped(t *testing.T) {
	// 18 x 500 ft = 9,000 ft for 131 against an 8,000 ft maximum: 16 cars.
	cars := gtMany("n", 18, gtBox, gtTrain("NOL1", "131"), "NOR", 1, 500)
	res := gtGen(gtProfile(), gtStatus(cars))
	o := gtOrder(res, "avail:131")
	if o == nil {
		t.Fatal("no 131 order")
	}
	if len(o.Consist) != 16 || o.Totals.LengthFt != 8000 || o.Totals.MaxLengthFt != 8000 {
		t.Errorf("consist %d cars, totals %+v, want 16 cars / 8000 ft", len(o.Consist), o.Totals)
	}
	b := gtBuild(res, "131")
	if !b.Ready || b.WaitingFt != 9000 {
		t.Errorf("build = %+v, want ready with all 9000 ft waiting", b)
	}
	if o.Kind != KindRoad || o.JobID != "131" || o.Symbol != "131" {
		t.Errorf("identity = %v %q %q", o.Kind, o.JobID, o.Symbol)
	}
}

func TestGenerateRoadCapTakesOldestAndStopsAtTons(t *testing.T) {
	p := gtProfile()
	p.RoadTrains[0].Limit.MaxTons = 350
	cars := []carstatus.Car{
		gtWait("young", gtBox, gtTrain("NOL1", "131"), "NOR", 1, 1500),
		gtWait("old", gtBox, gtTrain("NOL1", "131"), "NOR", 9, 1500),
		gtWait("mid", gtBox, gtTrain("NOL1", "131"), "NOR", 5, 1500),
		gtWait("mid2", gtBox, gtTrain("NOL1", "131"), "NOR", 4, 1500),
	}
	o := gtOrder(gtGen(p, gtStatus(cars)), "avail:131")
	if o == nil || !reflect.DeepEqual(gtKeys(o.Consist), []string{"old", "mid", "mid2"}) {
		t.Fatalf("consist = %+v, want the three oldest within 350 t", o)
	}
}

func TestGenerateRoadStopSetOutsCountTowardThreshold(t *testing.T) {
	// 2,500 ft NOL1 plus 2,000 ft BRI BRI is 4,500 ft: 56% of 8,000.
	cars := append(gtMany("n", 5, gtBox, gtTrain("NOL1", "131"), "NOR", 1, 500),
		gtMany("b", 4, gtBox, gtInd("BRI", "BRI"), "NOR", 1, 500)...)
	res := gtGen(gtProfile(), gtStatus(cars))
	b := gtBuild(res, "131")
	if b == nil || !b.Ready || b.WaitingFt != 4500 || math.Round(b.Pct) != 56 {
		t.Fatalf("131 build = %+v", b)
	}
	if o := gtOrder(res, "avail:131"); o == nil || len(o.Consist) != 9 {
		t.Errorf("131 order = %+v, want 9 consist cars", o)
	}
	// 339 only sees the 2,000 ft of BRI BRI: 25%, still building.
	if b := gtBuild(res, "339"); b.Ready || b.Pct != 25 {
		t.Errorf("339 build = %+v", b)
	}
}

func TestGenerateRoadAgeTrigger(t *testing.T) {
	// 341 allows 2 days. 1,200 ft is far below its 50% threshold.
	old := gtMany("o", 2, gtBox, gtTrain("CRD", "341"), "NOR", 3, 600)
	res := gtGen(gtProfile(), gtStatus(old))
	if o := gtOrder(res, "avail:341"); o == nil || len(o.Consist) != 2 {
		t.Fatalf("341 order = %+v, want available by age", o)
	}
	if b := gtBuild(res, "341"); !b.Ready || math.Round(b.Pct) != 15 {
		t.Errorf("341 build = %+v, want ready at 15%%", b)
	}
	young := gtMany("y", 2, gtBox, gtTrain("CRD", "341"), "NOR", 1, 600)
	if o := gtOrder(gtGen(gtProfile(), gtStatus(young)), "avail:341"); o != nil {
		t.Errorf("341 order from 1-day-old cars: %+v", o)
	}
	exact := gtMany("e", 2, gtBox, gtTrain("CRD", "341"), "NOR", 2, 600)
	if o := gtOrder(gtGen(gtProfile(), gtStatus(exact)), "avail:341"); o != nil {
		t.Errorf("age trigger must be strictly greater than the maximum: %+v", o)
	}
}

func TestGenerateRoadRequiredHP(t *testing.T) {
	// 10 cars of 620 t total 6,200 gross tons at 1.0 HP/T.
	cars := gtMany("n", 10, gtBox, gtTrain("NOL1", "131"), "NOR", 1, 500)
	for i := range cars {
		cars[i].GrossTons = 620
	}
	o := gtOrder(gtGen(gtProfile(), gtStatus(cars)), "avail:131")
	if o == nil || o.Totals.GrossTons != 6200 || o.Totals.RequiredHP != 6200 || o.Totals.MaxTons != 10000 {
		t.Fatalf("totals = %+v", o)
	}
}

func TestGenerateRoadOnlyOriginYardCarsAreGathered(t *testing.T) {
	cars := append(gtMany("n", 9, gtBox, gtTrain("NOL1", "131"), "NOR", 1, 500),
		gtMany("m", 3, gtBox, gtTrain("NOL1", "131"), "MCC", 1, 500)...)
	o := gtOrder(gtGen(gtProfile(), gtStatus(cars)), "avail:131")
	if o == nil || len(o.Consist) != 9 {
		t.Fatalf("consist = %+v, want only the 9 Norris cars", o)
	}
}

func TestGenerateRoadStop131WorksMcCalla(t *testing.T) {
	// 131 carries BRI BRI; McCalla holds empty autoracks tagged NOL and MER.
	cars := append(gtMany("n", 4, gtBox, gtTrain("NOL1", "131"), "NOR", 1, 1000),
		gtMany("b", 2, gtBox, gtInd("BRI", "BRI"), "NOR", 2, 1000)...)
	cars = append(cars,
		gtWait("nol", gtAuto, gtTrain("NOL", "355"), "MCC", 4, 90),
		gtWait("mer", gtAuto, gtTrain("MER", "339"), "MCC", 3, 90),
		gtWait("merwell", gtWell, gtTrain("MER", "29K"), "MCC", 5, 90), // wrong type
		gtWait("ges", gtBox, gtInd("A39", "GES"), "MCC", 9, 60),        // wrong tag
	)
	o := gtOrder(gtGen(gtProfile(), gtStatus(cars)), "avail:131")
	if o == nil {
		t.Fatal("no 131 order")
	}
	l := gtLine(o, "stop:MCC")
	if l == nil || l.Name != "McCalla" || len(o.Lines) != 1 {
		t.Fatalf("131 lines = %+v", o.Lines)
	}
	if got := gtKeys(l.SetOuts); !reflect.DeepEqual(got, []string{"ba", "bb"}) {
		t.Errorf("set-outs = %v, want the BRI BRI cars", got)
	}
	if got := gtKeys(l.PickUps); !reflect.DeepEqual(got, []string{"nol", "mer"}) {
		t.Errorf("pick-ups = %v, want NOL then MER autoracks", got)
	}
}

func TestGenerateRoadStopPickUpsLimitedByLength(t *testing.T) {
	// 131 leaves with 7,400 ft: 600 ft spare. 900 ft of NOL racks wait.
	cars := append(gtMany("n", 4, gtBox, gtTrain("NOL1", "131"), "NOR", 1, 1850),
		gtWait("r1", gtAuto, gtTrain("NOL", "355"), "MCC", 3, 300),
		gtWait("r2", gtAuto, gtTrain("NOL", "355"), "MCC", 2, 300),
		gtWait("r3", gtAuto, gtTrain("NOL", "355"), "MCC", 1, 300),
	)
	l := gtLine(gtOrder(gtGen(gtProfile(), gtStatus(cars)), "avail:131"), "stop:MCC")
	if l == nil || !reflect.DeepEqual(gtKeys(l.PickUps), []string{"r1", "r2"}) {
		t.Fatalf("stop = %+v, want the two oldest racks", l)
	}
}

func TestGenerateRoadStopSetOutFreesRoomForPickUps(t *testing.T) {
	// 8,000 ft train; 1,000 ft of BRI BRI set out at McCalla leaves room for
	// a 900 ft pick-up even though the consist was full.
	cars := append(gtMany("n", 7, gtBox, gtTrain("NOL1", "131"), "NOR", 1, 1000),
		gtWait("b", gtBox, gtInd("BRI", "BRI"), "NOR", 1, 1000),
		gtWait("r", gtAuto, gtTrain("NOL", "355"), "MCC", 1, 900),
	)
	l := gtLine(gtOrder(gtGen(gtProfile(), gtStatus(cars)), "avail:131"), "stop:MCC")
	if l == nil || len(l.SetOuts) != 1 || !reflect.DeepEqual(gtKeys(l.PickUps), []string{"r"}) {
		t.Fatalf("stop = %+v", l)
	}
}

func TestGenerateRoadStopWithoutWorkIsOmitted(t *testing.T) {
	cars := gtMany("n", 9, gtBox, gtTrain("NOL1", "131"), "NOR", 1, 500)
	o := gtOrder(gtGen(gtProfile(), gtStatus(cars)), "avail:131")
	if o == nil || len(o.Lines) != 0 {
		t.Fatalf("131 order = %+v, want no stop lines", o)
	}
}

func TestGenerateStopAutorackVersusWellSplitBetween346And28J(t *testing.T) {
	// McCalla holds an ATL autorack and an ATL well car. 346 takes only the
	// autorack, 28J only the well car.
	cars := []carstatus.Car{
		gtWait("rack", gtAuto, gtTrain("ATL", "346"), "MCC", 2, 90),
		gtWait("well", gtWell, gtTrain("ATL", "28J"), "MCC", 2, 90),
	}
	res := gtGen(gtProfile(), gtStatus(cars))
	o346, o28j := gtOrder(res, "avail:346"), gtOrder(res, "avail:28J")
	if o346 == nil || o28j == nil {
		t.Fatalf("orders 346=%v 28J=%v", o346, o28j)
	}
	if got := gtKeys(gtLine(o346, "stop:MCC").PickUps); !reflect.DeepEqual(got, []string{"rack"}) {
		t.Errorf("346 pick-ups = %v", got)
	}
	if got := gtKeys(gtLine(o28j, "stop:MCC").PickUps); !reflect.DeepEqual(got, []string{"well"}) {
		t.Errorf("28J pick-ups = %v", got)
	}
}

func TestGenerateStopA28WorksPHS(t *testing.T) {
	// A28 carries A28 PHS cars; two cars at PHS are tagged ATL.
	cars := append(gtMany("p", 5, gtBox, gtInd("A28", "PHS"), "NOR", 1, 100),
		gtAt("x1", gtAuto, gtTrain("ATL", "346"), "PHS", carstatus.ServiceReadyToPull, 2, 90),
		gtAt("x2", gtAuto, gtTrain("ATL", "346"), "PHS", carstatus.ServiceReadyToPull, 1, 90),
	)
	o := gtOrder(gtGen(gtProfile(), gtStatus(cars)), "avail:A28")
	if o == nil {
		t.Fatal("no A28 order")
	}
	l := gtLine(o, "stop:PHS")
	if l == nil || l.Name != "PHS" || len(l.SetOuts) != 5 || !reflect.DeepEqual(gtKeys(l.PickUps), []string{"x1", "x2"}) {
		t.Fatalf("PHS stop = %+v", l)
	}
}

func TestGenerateStopIndustryReadinessGateAndCapacity(t *testing.T) {
	cars := append(gtMany("p", 12, gtBox, gtInd("A28", "PHS"), "NOR", 1, 100),
		gtAt("w", gtBox, gtInd("A28", "PHS"), "PHS", carstatus.ServiceWorking, 1, 100),
		gtAt("x", gtAuto, gtTrain("ATL", "346"), "PHS", carstatus.ServiceReadyToPull, 1, 90),
	)
	o := gtOrder(gtGen(gtProfile(), gtStatus(cars)), "avail:A28")
	if o == nil || gtLine(o, "stop:PHS") != nil {
		t.Fatalf("A28 order = %+v, want no PHS stop while a car is working", o)
	}
	ready := append([]carstatus.Car(nil), cars[:12]...)
	l := gtLine(gtOrder(gtGen(gtProfile(), gtStatus(ready)), "avail:A28"), "stop:PHS")
	if l == nil || len(l.SetOuts) != 10 {
		t.Fatalf("PHS set-outs = %+v, want 10 (Box Car capacity)", l)
	}
}

func TestGenerateThroughTrain346CollectsATLRacks(t *testing.T) {
	cars := gtMany("r", 3, gtAuto, gtTrain("ATL", "346"), "MCC", 1, 90)
	res := gtGen(gtProfile(), gtStatus(cars))
	o := gtOrder(res, "avail:346")
	if o == nil {
		t.Fatal("no 346 order")
	}
	if len(o.Consist) != 0 {
		t.Errorf("through train has a consist: %+v", o.Consist)
	}
	l := gtLine(o, "stop:MCC")
	if l == nil || len(l.PickUps) != 3 || len(l.SetOuts) != 0 {
		t.Fatalf("stop = %+v", l)
	}
	if o.Totals.LengthFt != 270 || o.Totals.GrossTons != 300 {
		t.Errorf("totals = %+v", o.Totals)
	}
	if gtBuild(res, "346") != nil {
		t.Error("through train appears in Building")
	}
	if gtOrder(res, "avail:28J") != nil {
		t.Error("28J order without well cars")
	}
}

func TestGenerateThroughTrainThreshold(t *testing.T) {
	// T50 needs 50% of 2,000 ft = 1,000 ft of pick-ups.
	low := gtMany("t", 2, gtBox, gtTrain("TTT", "T50"), "MCC", 1, 400)
	if o := gtOrder(gtGen(gtProfile(), gtStatus(low)), "avail:T50"); o != nil {
		t.Errorf("T50 order at 800 ft: %+v", o)
	}
	enough := gtMany("t", 3, gtBox, gtTrain("TTT", "T50"), "MCC", 1, 400)
	if o := gtOrder(gtGen(gtProfile(), gtStatus(enough)), "avail:T50"); o == nil {
		t.Error("no T50 order at 1,200 ft")
	}
}

func TestGenerateThroughTrainPickUpsRespectLimit(t *testing.T) {
	p := gtProfile()
	p.RoadTrains[5].Limit = profile.Limit{MaxLengthFt: 200}
	cars := gtMany("r", 4, gtAuto, gtTrain("ATL", "346"), "MCC", 1, 90)
	l := gtLine(gtOrder(gtGen(p, gtStatus(cars)), "avail:346"), "stop:MCC")
	if l == nil || len(l.PickUps) != 2 {
		t.Fatalf("stop = %+v, want 2 racks within 200 ft", l)
	}
}

func gtA28Cars(dest func() carstatus.Dest) []carstatus.Car {
	cars := []carstatus.Car{}
	for _, k := range []string{"p1", "p2", "p3", "p4", "p5"} {
		cars = append(cars, gtWait(k, gtBox, dest(), "NOR", 1, 100))
	}
	return append(cars,
		gtAt("x1", gtAuto, gtTrain("ATL", "346"), "PHS", carstatus.ServiceReadyToPull, 2, 90))
}

func checkA28Order(t *testing.T, res GenResult) {
	t.Helper()
	o := gtOrder(res, "avail:A28")
	if o == nil {
		t.Fatal("no A28 order")
	}
	if len(o.Consist) != 5 || o.Totals.LengthFt != 500 || o.Totals.GrossTons != 500 {
		t.Errorf("consist %d cars, totals %+v; cars must be counted once", len(o.Consist), o.Totals)
	}
	l := gtLine(o, "stop:PHS")
	if l == nil || len(l.SetOuts) != 5 || !reflect.DeepEqual(gtKeys(l.PickUps), []string{"x1"}) {
		t.Fatalf("PHS stop = %+v", l)
	}
}

func TestGenerateStopA28TrainBoundShape(t *testing.T) {
	// `A28 PHS` classifies as train-bound for A28 with Industry PHS.
	shape := func() carstatus.Dest {
		return carstatus.Dest{Kind: carstatus.DestTrain, Train: "A28", Industry: "PHS", Tag: "A28 PHS"}
	}
	checkA28Order(t, gtGen(gtProfile(), gtStatus(gtA28Cars(shape))))
	// Train-bound for another train with the same industry is not A28's.
	other := func() carstatus.Dest {
		return carstatus.Dest{Kind: carstatus.DestTrain, Train: "131", Industry: "PHS", Tag: "131 PHS"}
	}
	if gtLine(gtOrder(gtGen(gtProfile(), gtStatus(gtA28Cars(other))), "avail:A28"), "stop:PHS") != nil {
		t.Error("another train's car set out at A28's PHS stop")
	}
}

func TestGenerateStopA28ViaClassify(t *testing.T) {
	p := gtProfile()
	classify := func() carstatus.Dest { return carstatus.Classify("A28  PHS", p) }
	if d := classify(); d.Kind != carstatus.DestTrain && d.Kind != carstatus.DestIndustry {
		t.Fatalf("Classify(A28 PHS) = %+v", d)
	}
	checkA28Order(t, gtGen(p, gtStatus(gtA28Cars(classify))))
}
