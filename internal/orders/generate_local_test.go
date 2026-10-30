package orders

import (
	"reflect"
	"testing"

	"run8-ops-center/internal/carstatus"
	"run8-ops-center/internal/profile"
)

func TestGenerateLocalCapacityLimit(t *testing.T) {
	// 8 Box Cars tagged AB45 JPI wait in Norris; JPI takes 6 Box Cars.
	// Car "bN" has dwelt N days, so b8 is the oldest.
	var cars []carstatus.Car
	for i := 1; i <= 8; i++ {
		cars = append(cars, gtWait("b"+string(rune('0'+i)), gtBox, gtInd("AB45", "JPI"), "NOR", i, 60))
	}
	res := gtGen(gtProfile(), gtStatus(cars))
	o := gtOrder(res, "avail:ab45")
	if o == nil {
		t.Fatal("no AB45 order")
	}
	l := gtLine(o, "industry:JPI")
	if l == nil {
		t.Fatal("no JPI line")
	}
	want := []string{"b8", "b7", "b6", "b5", "b4", "b3"}
	if got := gtKeys(l.SetOuts); !reflect.DeepEqual(got, want) {
		t.Errorf("JPI set-outs = %v, want the six oldest %v", got, want)
	}
	if l.Name != "Jasper Pipe Inc" || o.Kind != KindLocal || o.State != StateAvailable || o.Symbol != "AB45" {
		t.Errorf("unexpected line/order identity: %q %v %v %q", l.Name, o.Kind, o.State, o.Symbol)
	}
}

func TestGenerateLocalCapacityPerCarType(t *testing.T) {
	// JPI takes 6 Box Cars and 2 Gondolas; coil cars are not accepted at all.
	var cars []carstatus.Car
	cars = append(cars, gtMany("g", 4, gtGon, gtInd("AB45", "JPI"), "NOR", 5, 60)...)
	cars = append(cars, gtMany("x", 3, gtCoil, gtInd("AB45", "JPI"), "NOR", 9, 60)...)
	cars = append(cars, gtMany("b", 6, gtBox, gtInd("AB45", "JPI"), "NOR", 1, 60)...)
	l := gtLine(gtOrder(gtGen(gtProfile(), gtStatus(cars)), "avail:ab45"), "industry:JPI")
	if l == nil {
		t.Fatal("no JPI line")
	}
	var gon, box, coil int
	for _, r := range l.SetOuts {
		switch r.Type {
		case gtGon:
			gon++
		case gtBox:
			box++
		case gtCoil:
			coil++
		}
	}
	if gon != 2 || box != 6 || coil != 0 {
		t.Errorf("set-outs gon=%d box=%d coil=%d, want 2/6/0", gon, box, coil)
	}
}

func TestGenerateLocalUnknownTypeCountsAgainstTotalCapacity(t *testing.T) {
	// JPI total capacity is 8 (6 + 2). Ten unknown-type cars fit only 8.
	cars := gtMany("u", 10, profile.Unknown, gtInd("AB45", "JPI"), "NOR", 3, 60)
	l := gtLine(gtOrder(gtGen(gtProfile(), gtStatus(cars)), "avail:ab45"), "industry:JPI")
	if l == nil || len(l.SetOuts) != 8 {
		t.Fatalf("unknown cars set out = %v, want 8", l)
	}
	// Unknown cars also use up room for typed cars: 5 unknown (older) + 6 box.
	mixed := append(gtMany("u", 5, profile.Unknown, gtInd("AB45", "JPI"), "NOR", 9, 60),
		gtMany("b", 6, gtBox, gtInd("AB45", "JPI"), "NOR", 1, 60)...)
	l = gtLine(gtOrder(gtGen(gtProfile(), gtStatus(mixed)), "avail:ab45"), "industry:JPI")
	if l == nil || len(l.SetOuts) != 8 {
		t.Fatalf("mixed set-outs = %v, want 8 in total", l)
	}
}

func TestGenerateLocalDwellTieBreaksByKey(t *testing.T) {
	cars := []carstatus.Car{
		gtWait("k3", gtBox, gtInd("AB45", "STI"), "NOR", 2, 60),
		gtWait("k1", gtBox, gtInd("AB45", "STI"), "NOR", 2, 60),
		gtWait("k2", gtBox, gtInd("AB45", "STI"), "NOR", 2, 60),
	}
	l := gtLine(gtOrder(gtGen(gtProfile(), gtStatus(cars)), "avail:ab45"), "industry:STI")
	if got := gtKeys(l.SetOuts); !reflect.DeepEqual(got, []string{"k1", "k2", "k3"}) {
		t.Errorf("set-outs = %v", got)
	}
}

func TestGenerateLocalTwoEligibleIndustries(t *testing.T) {
	cars := append(gtMany("s", 2, gtBox, gtInd("AB45", "STI"), "NOR", 1, 60),
		gtMany("h", 2, gtBox, gtInd("AB45", "HTI"), "NOR", 1, 60)...)
	o := gtOrder(gtGen(gtProfile(), gtStatus(cars)), "avail:ab45")
	if o == nil || gtLine(o, "industry:STI") == nil || gtLine(o, "industry:HTI") == nil || len(o.Lines) != 2 {
		t.Fatalf("AB45 lines = %+v, want exactly STI and HTI", o)
	}
}

func TestGenerateLocalIndustryStillProcessing(t *testing.T) {
	// One car at GES is still tagged A39 GES (working); another is ready.
	cars := []carstatus.Car{
		gtAt("w", gtBox, gtInd("A39", "GES"), "GES", carstatus.ServiceWorking, 1, 60),
		gtAt("r", gtBox, gtTrain("CHA", "108"), "GES", carstatus.ServiceReadyToPull, 1, 60),
		gtWait("n", gtBox, gtInd("A39", "GES"), "MCC", 1, 60),
	}
	res := gtGen(gtProfile(), gtStatus(cars))
	if o := gtOrder(res, "avail:a39"); o != nil {
		t.Errorf("A39 order exists while GES is processing: %+v", o.Lines)
	}
}

func TestGenerateLocalAllCarsRetagged(t *testing.T) {
	cars := []carstatus.Car{
		gtAt("r1", gtBox, gtTrain("CHA", "108"), "GES", carstatus.ServiceReadyToPull, 2, 60),
		gtAt("r2", gtBox, gtTrain("NOL1", "131"), "GES", carstatus.ServiceReadyToPull, 1, 60),
	}
	l := gtLine(gtOrder(gtGen(gtProfile(), gtStatus(cars)), "avail:a39"), "industry:GES")
	if l == nil || len(l.SetOuts) != 0 || !reflect.DeepEqual(gtKeys(l.PickUps), []string{"r1", "r2"}) {
		t.Fatalf("GES line = %+v, want pick-ups r1 r2 only", l)
	}
}

func TestGenerateLocalOutsideHomeYard(t *testing.T) {
	// A37 CYC in Norris: A37 is homed at Tuscaloosa, so only A55's
	// Tuscaloosa yard line moves it.
	res := gtGen(gtProfile(), gtStatus(gtOne(gtWait("c", gtBox, gtInd("A37", "CYC"), "NOR", 1, 60))))
	if o := gtOrder(res, "avail:a37"); o != nil {
		t.Errorf("A37 order lists the Norris car: %+v", o.Lines)
	}
	l := gtLine(gtOrder(res, "avail:a55"), "yard:TUS")
	if l == nil || !reflect.DeepEqual(gtKeys(l.SetOuts), []string{"c"}) {
		t.Fatalf("A55 Tuscaloosa line = %+v", l)
	}
}

func TestGenerateLocalPickUpOnly(t *testing.T) {
	cars := []carstatus.Car{
		gtAt("p1", gtBox, gtTrain("CHA", "108"), "WRG", carstatus.ServiceReadyToPull, 2, 60),
		gtAt("p2", gtBox, gtTrain("CHA", "108"), "WRG", carstatus.ServiceReadyToPull, 1, 60),
	}
	l := gtLine(gtOrder(gtGen(gtProfile(), gtStatus(cars)), "avail:ab10"), "industry:WRG")
	if l == nil || len(l.SetOuts) != 0 || len(l.PickUps) != 2 {
		t.Fatalf("WRG line = %+v, want two pick-ups and no set-outs", l)
	}
	if l.PickUps[0].Location != "industry WRG" || l.PickUps[0].Tag != "CHA" || l.PickUps[0].Number != "T p1" {
		t.Errorf("pick-up ref = %+v", l.PickUps[0])
	}
}

func TestGenerateLocalOverLength(t *testing.T) {
	// 10 cars of 430 ft total 4,300 ft against AB45's 4,000 ft limit.
	cars := gtMany("l", 10, gtBox, gtInd("AB45", "STI"), "NOR", 1, 430)
	o := gtOrder(gtGen(gtProfile(), gtStatus(cars)), "avail:ab45")
	if o == nil || !gtFlag(o, "over-length") {
		t.Fatalf("AB45 order = %+v, want an over-length flag", o)
	}
	if o.Totals.LengthFt != 4300 || o.Totals.MaxLengthFt != 4000 || o.Totals.GrossTons != 1000 {
		t.Errorf("totals = %+v", o.Totals)
	}
	if gtFlag(o, "over-tons") {
		t.Error("over-tons flagged without a ton limit")
	}
	// Nine cars (3,870 ft) are within the limit.
	ok := gtOrder(gtGen(gtProfile(), gtStatus(cars[:9])), "avail:ab45")
	if gtFlag(ok, "over-length") {
		t.Error("within-limit order flagged over-length")
	}
}

func TestGenerateLocalOverTons(t *testing.T) {
	p := gtProfile()
	p.LocalJobs[0].Limit = profile.Limit{MaxTons: 500}
	cars := gtMany("t", 6, gtBox, gtInd("AB45", "STI"), "NOR", 1, 60) // 6 x 100 t
	if o := gtOrder(gtGen(p, gtStatus(cars)), "avail:ab45"); o == nil || !gtFlag(o, "over-tons") {
		t.Errorf("order = %+v, want over-tons flag", o)
	}
}

func TestGenerateLocalExcludesUnmappedForeignAndUnknown(t *testing.T) {
	unmapped := gtWait("u", gtBox, gtInd("AB45", "STI"), "NOR", 1, 60)
	unmapped.Loc = carstatus.Location{Kind: carstatus.LocUnmapped}
	foreign := gtWait("f", gtBox, carstatus.Dest{Kind: carstatus.DestForeign, Tag: "CPKC ."}, "NOR", 1, 60)
	unknown := gtWait("k", gtBox, carstatus.Dest{Kind: carstatus.DestUnknown, Tag: "STI"}, "NOR", 1, 60)
	atIndForeign := gtAt("g", gtBox, carstatus.Dest{Kind: carstatus.DestForeign, Tag: "CPKC ."}, "WRG", carstatus.ServiceReadyToPull, 1, 60)
	res := gtGen(gtProfile(), gtStatus([]carstatus.Car{unmapped, foreign, unknown, atIndForeign}))
	if len(res.Orders) != 0 {
		t.Errorf("orders generated from excluded cars: %+v", res.Orders)
	}
}

func TestGenerateYardLineTuscaloosaBlock(t *testing.T) {
	// Norris holds A37 CYC, A56 TBP and AGR AGR; A55's Tuscaloosa line
	// carries A37, A56 and AGR, alongside its KYK and ABS industry lines.
	cars := []carstatus.Car{
		gtWait("c", gtBox, gtInd("A37", "CYC"), "NOR", 3, 60),
		gtWait("t", gtBox, gtInd("A56", "TBP"), "NOR", 2, 60),
		gtWait("a", gtBox, gtInd("AGR", "AGR"), "NOR", 1, 60),
		gtWait("k", gtBox, gtInd("A55", "KYK"), "NOR", 1, 60),
		gtWait("s", gtBox, gtInd("A55", "ABS"), "NOR", 1, 60),
	}
	o := gtOrder(gtGen(gtProfile(), gtStatus(cars)), "avail:a55")
	if o == nil || len(o.Lines) != 3 {
		t.Fatalf("A55 order = %+v, want KYK, ABS and Tuscaloosa lines", o)
	}
	l := gtLine(o, "yard:TUS")
	if l == nil || l.Name != "Tuscaloosa" || !reflect.DeepEqual(gtKeys(l.SetOuts), []string{"c", "t", "a"}) {
		t.Fatalf("Tuscaloosa line = %+v", l)
	}
	if gtLine(o, "industry:KYK") == nil || gtLine(o, "industry:ABS") == nil {
		t.Error("missing KYK or ABS line")
	}
	if o.Totals.LengthFt != 300 {
		t.Errorf("outbound length = %v, want 300 (all five set-outs)", o.Totals.LengthFt)
	}
}

func TestGenerateYardLineBessemerBlock(t *testing.T) {
	cars := gtOne(gtWait("r", gtBox, gtInd("AB45", "BRL"), "NOR", 1, 60))
	o := gtOrder(gtGen(gtProfile(), gtStatus(cars)), "avail:ab45")
	l := gtLine(o, "yard:BES")
	if l == nil || l.Kind != LineYard || !reflect.DeepEqual(gtKeys(l.SetOuts), []string{"r"}) {
		t.Fatalf("Bessemer line = %+v", l)
	}
}

func TestGenerateYardLineCarriesClassCodeViaAlias(t *testing.T) {
	p := gtProfile()
	p.Aliases = map[string]string{"BN1": "BNSF"}
	p.LocalJobs[0].YardLines = append(p.LocalJobs[0].YardLines, profile.YardLine{Yard: "TUS", Carries: []string{"BNSF"}})
	cars := gtOne(gtWait("x", gtBox, gtTrain("BN1", "AB11"), "NOR", 1, 60))
	l := gtLine(gtOrder(gtGen(p, gtStatus(cars)), "avail:ab45"), "yard:TUS")
	if l == nil || len(l.SetOuts) != 1 {
		t.Fatalf("yard line = %+v, want the aliased class-code car", l)
	}
}

func TestGenerateYardLinePullsReturnedFromTuscaloosa(t *testing.T) {
	// A56 pulls tagged SEL and NOL3 sit in Tuscaloosa next to A37 SIS cars.
	cars := []carstatus.Car{
		gtWait("sel", gtBox, gtTrain("SEL", "345"), "TUS", 2, 60),
		gtWait("nol", gtBox, gtTrain("NOL3", "339"), "TUS", 1, 60),
		gtWait("sis", gtBox, gtInd("A37", "SIS"), "TUS", 1, 60),
	}
	l := gtLine(gtOrder(gtGen(gtProfile(), gtStatus(cars)), "avail:a55"), "yard:TUS")
	if l == nil || len(l.SetOuts) != 0 || !reflect.DeepEqual(gtKeys(l.PickUps), []string{"sel", "nol"}) {
		t.Fatalf("Tuscaloosa line = %+v, want pick-ups sel and nol only", l)
	}
}

func TestGenerateYardLineCarriedTagsStayInSatelliteYard(t *testing.T) {
	// A carried-tag car already in Tuscaloosa belongs there.
	cars := gtOne(gtWait("c", gtBox, gtInd("A56", "TBP"), "TUS", 1, 60))
	if o := gtOrder(gtGen(gtProfile(), gtStatus(cars)), "avail:a55"); o != nil {
		t.Errorf("A55 order = %+v, want none", o)
	}
}

func TestGenerateYardLineAutorackEmptiesLeftForRoadTrain(t *testing.T) {
	// McCalla holds A39 pulls tagged CHA and autoracks tagged NOL; 131's
	// McCalla stop picks up NOL, so A44's McCalla line takes only CHA.
	cars := []carstatus.Car{
		gtWait("cha", gtBox, gtTrain("CHA", "108"), "MCC", 2, 60),
		gtWait("nol", gtAuto, gtTrain("NOL", "355"), "MCC", 2, 60),
		gtWait("mer", gtAuto, gtTrain("MER", "339"), "MCC", 2, 60),
		gtWait("ges", gtBox, gtInd("A39", "GES"), "MCC", 2, 60), // belongs: carried and homed
	}
	l := gtLine(gtOrder(gtGen(gtProfile(), gtStatus(cars)), "avail:a44"), "yard:MCC")
	if l == nil || !reflect.DeepEqual(gtKeys(l.PickUps), []string{"cha"}) {
		t.Fatalf("McCalla line = %+v, want pick-up cha only", l)
	}
}

func TestGenerateYardLineStopTypeFilterDoesNotClaimOtherTypes(t *testing.T) {
	// NOL on a Box Car is not claimed by the autorack-only stop.
	cars := gtOne(gtWait("nolbox", gtBox, gtTrain("NOL", "355"), "MCC", 1, 60))
	l := gtLine(gtOrder(gtGen(gtProfile(), gtStatus(cars)), "avail:a44"), "yard:MCC")
	if l == nil || len(l.PickUps) != 1 {
		t.Fatalf("McCalla line = %+v, want the box car picked up", l)
	}
}

func TestGenerateLocalOutputIsSorted(t *testing.T) {
	cars := append(gtMany("w", 2, gtBox, gtInd("A55", "KYK"), "NOR", 1, 60),
		gtMany("s", 2, gtBox, gtInd("AB45", "STI"), "NOR", 1, 60)...)
	cars = append(cars, gtMany("r", 9, gtBox, gtTrain("NOL1", "131"), "NOR", 1, 500)...)
	res := gtGen(gtProfile(), gtStatus(cars))
	var ids []string
	for _, o := range res.Orders {
		ids = append(ids, o.ID)
	}
	if want := []string{"avail:a55", "avail:ab45", "avail:131"}; !reflect.DeepEqual(ids, want) {
		t.Errorf("order IDs = %v, want %v", ids, want)
	}
}
