package orders

import (
	"time"

	"run8-ops-center/internal/carstatus"
	"run8-ops-center/internal/profile"
	"run8-ops-center/internal/run8save"
)

const (
	gtBox  profile.CarType = "Box Car"
	gtGon  profile.CarType = "Gondola"
	gtAuto profile.CarType = "Autorack Car"
	gtWell profile.CarType = "Well Car"
	gtCoil profile.CarType = "CoilCar"
)

// gtNow is the in-game time of every test snapshot.
var gtNow = time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)

func gtInd(local, ind string) carstatus.Dest {
	return carstatus.Dest{Kind: carstatus.DestIndustry, Local: local, Industry: ind, Tag: local + " " + ind, Raw: local + " " + ind}
}

func gtTrain(tag, train string) carstatus.Dest {
	return carstatus.Dest{Kind: carstatus.DestTrain, Tag: tag, Raw: tag, Train: train}
}

func gtYardLoc(code string) carstatus.Location {
	return carstatus.Location{Kind: carstatus.LocYard, Yard: code, Track: "T1"}
}

func gtIndLoc(tag string) carstatus.Location {
	return carstatus.Location{Kind: carstatus.LocIndustry, Industries: []string{tag}}
}

// gtCar builds a car; age is whole in-game days of dwell, ft its length.
func gtCar(key string, typ profile.CarType, d carstatus.Dest, loc carstatus.Location, svc carstatus.Service, age int, ft float64) carstatus.Car {
	return carstatus.Car{
		Key: key, Number: "T " + key, Type: typ, LengthFt: ft, GrossTons: 100,
		Dest: d, Loc: loc, Service: svc,
		DwellSince: gtNow.Add(-time.Duration(age) * 24 * time.Hour),
	}
}

// gtWait builds a car waiting in a yard (awaiting delivery or train).
func gtWait(key string, typ profile.CarType, d carstatus.Dest, yard string, age int, ft float64) carstatus.Car {
	svc := carstatus.ServiceAwaitingTrain
	if d.Kind == carstatus.DestIndustry {
		svc = carstatus.ServiceAwaitingDelivery
	}
	return gtCar(key, typ, d, gtYardLoc(yard), svc, age, ft)
}

// gtAt builds a car at an industry track.
func gtAt(key string, typ profile.CarType, d carstatus.Dest, ind string, svc carstatus.Service, age int, ft float64) carstatus.Car {
	return gtCar(key, typ, d, gtIndLoc(ind), svc, age, ft)
}

// gtMany builds n yard cars key0..key(n-1), all with the same age and length.
func gtMany(prefix string, n int, typ profile.CarType, d carstatus.Dest, yard string, age int, ft float64) []carstatus.Car {
	out := make([]carstatus.Car, n)
	for i := range out {
		out[i] = gtWait(prefix+string(rune('a'+i)), typ, d, yard, age, ft)
	}
	return out
}

func gtStatus(groups ...[]carstatus.Car) *carstatus.Status {
	st := &carstatus.Status{
		Snapshot: &run8save.Snapshot{GameDate: gtNow},
		ByKey:    map[string]*carstatus.Car{},
	}
	for _, g := range groups {
		st.Cars = append(st.Cars, g...)
	}
	for i := range st.Cars {
		st.ByKey[st.Cars[i].Key] = &st.Cars[i]
	}
	return st
}

func gtOne(c carstatus.Car) []carstatus.Car { return []carstatus.Car{c} }

func gtIndustries() []profile.Industry {
	box := func(n int) []profile.IndustryCar { return []profile.IndustryCar{{CarType: gtBox, Capacity: n}} }
	return []profile.Industry{
		{Name: "Sunbelt Trading Inc", Tag: "STI", LocalSymbol: "AB45", Cars: box(20)},
		{Name: "Holcim Trading Inc", Tag: "HTI", LocalSymbol: "AB45", Cars: box(20)},
		{Name: "Jasper Pipe Inc", Tag: "JPI", LocalSymbol: "AB45", Cars: []profile.IndustryCar{
			{CarType: gtBox, Capacity: 6}, {CarType: gtGon, Capacity: 2},
		}},
		{Name: "Kyker", Tag: "KYK", LocalSymbol: "A55", Cars: box(10)},
		{Name: "ABS", Tag: "ABS", LocalSymbol: "A55", Cars: box(10)},
		{Name: "Cyclone", Tag: "CYC", LocalSymbol: "A37", Cars: box(10)},
		{Name: "Sister", Tag: "SIS", LocalSymbol: "A37", Cars: box(10)},
		{Name: "TBP", Tag: "TBP", LocalSymbol: "A56", Cars: box(10)},
		{Name: "AGR", Tag: "AGR", LocalSymbol: "A56", Cars: box(10)},
		{Name: "GES", Tag: "GES", LocalSymbol: "A39", Cars: box(10)},
		{Name: "WRG", Tag: "WRG", LocalSymbol: "AB10", Cars: box(10)},
		{Name: "BRI", Tag: "BRI", LocalSymbol: "BRI", Cars: box(20)},
		{Name: "PHS", Tag: "PHS", LocalSymbol: "A28", Cars: box(10)},
	}
}

func gtMCCStop(setOut ...string) profile.Stop {
	return profile.Stop{Kind: "yard", Target: "MCC", SetOut: setOut,
		PickUp: []string{"NOL", "MER"}, PickUpCarTypes: []profile.CarType{gtAuto}}
}

// gtProfile is a small hand-written route profile modelled on AGS South.
func gtProfile() *profile.Profile {
	return &profile.Profile{
		ID: "test",
		Yards: []profile.Yard{
			{Code: "NOR", Name: "Norris"}, {Code: "TUS", Name: "Tuscaloosa"},
			{Code: "MCC", Name: "McCalla"}, {Code: "BES", Name: "Bessemer"},
		},
		Industries: gtIndustries(),
		LocalJobs: []profile.LocalJob{
			{ID: "ab45", Symbol: "AB45", HomeYard: "NOR", Industries: []string{"STI", "HTI", "JPI"},
				YardLines: []profile.YardLine{{Yard: "BES", Carries: []string{"BRL", "WWL"}}},
				Limit:     profile.Limit{MaxLengthFt: 4000}},
			{ID: "a55", Symbol: "A55", HomeYard: "NOR", Industries: []string{"KYK", "ABS"},
				YardLines: []profile.YardLine{{Yard: "TUS", Carries: []string{"A37", "A56", "AGR"}}}},
			{ID: "a37", Symbol: "A37", HomeYard: "TUS", Industries: []string{"CYC", "SIS"}},
			{ID: "a56", Symbol: "A56", HomeYard: "TUS", Industries: []string{"TBP", "AGR"}},
			{ID: "a44", Symbol: "A44", HomeYard: "NOR",
				YardLines: []profile.YardLine{{Yard: "MCC", Carries: []string{"A39"}}}},
			{ID: "a39", Symbol: "A39", HomeYard: "MCC", Industries: []string{"GES"}},
			{ID: "ab10", Symbol: "AB10", HomeYard: "NOR", Industries: []string{"WRG"}},
			{ID: "bri", Symbol: "BRI", HomeYard: "MCC", Industries: []string{"BRI"}},
		},
		RoadTrains: []profile.RoadTrain{
			{Symbol: "131", Origin: "NOR", Destination: "Atlanta", ClassCodes: []string{"NOL1"},
				Limit: profile.Limit{MaxLengthFt: 8000, MaxTons: 10000, MinHPT: 1.0}, MinLengthPct: 50,
				Stops: []profile.Stop{gtMCCStop("BRI")}},
			{Symbol: "339", Origin: "NOR", Destination: "Atlanta", ClassCodes: []string{"MER", "NOL3"},
				Limit: profile.Limit{MaxLengthFt: 8000, MaxTons: 10000, MinHPT: 1.0}, MinLengthPct: 50,
				Stops: []profile.Stop{gtMCCStop("BRI")}},
			{Symbol: "108", Origin: "NOR", Destination: "Chattanooga", ClassCodes: []string{"CHA"},
				Limit: profile.Limit{MaxLengthFt: 8000}, MinLengthPct: 50},
			{Symbol: "341", Origin: "NOR", Destination: "Cordele", ClassCodes: []string{"CRD"},
				Limit: profile.Limit{MaxLengthFt: 8000}, MinLengthPct: 50, MaxCarAgeDays: 2},
			{Symbol: "A28", Origin: "NOR", Destination: "Parrish", OffMapIndustries: []string{"A28"},
				Limit: profile.Limit{MaxLengthFt: 4000}, MinLengthPct: 10,
				Stops: []profile.Stop{{Kind: "industry", Target: "PHS"}}},
			{Symbol: "346", Stops: []profile.Stop{{Kind: "yard", Target: "MCC",
				PickUp: []string{"ATL"}, PickUpCarTypes: []profile.CarType{gtAuto}}}},
			{Symbol: "28J", Stops: []profile.Stop{{Kind: "yard", Target: "MCC",
				PickUp: []string{"ATL"}, PickUpCarTypes: []profile.CarType{gtWell}}}},
			{Symbol: "T50", Limit: profile.Limit{MaxLengthFt: 2000}, MinLengthPct: 50,
				Stops: []profile.Stop{{Kind: "yard", Target: "MCC", PickUp: []string{"TTT"}}}},
		},
	}
}

func gtGen(p *profile.Profile, st *carstatus.Status, active ...Order) GenResult {
	return Generate(GenInput{Profile: p, Status: st, Active: active})
}

func gtOrder(res GenResult, id string) *Order {
	for i := range res.Orders {
		if res.Orders[i].ID == id {
			return &res.Orders[i]
		}
	}
	return nil
}

func gtLine(o *Order, key string) *Line {
	if o == nil {
		return nil
	}
	for i := range o.Lines {
		if o.Lines[i].Key() == key {
			return &o.Lines[i]
		}
	}
	return nil
}

func gtKeys(rs []CarRef) []string {
	out := []string{}
	for _, r := range rs {
		out = append(out, r.Key)
	}
	return out
}

func gtBuild(res GenResult, sym string) *RoadBuild {
	for i := range res.Building {
		if res.Building[i].Symbol == sym {
			return &res.Building[i]
		}
	}
	return nil
}

func gtFlag(o *Order, code string) bool {
	for _, f := range o.Flags {
		if f.Code == code {
			return true
		}
	}
	return false
}
