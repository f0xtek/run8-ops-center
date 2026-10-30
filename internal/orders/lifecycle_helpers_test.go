package orders

import (
	"time"

	"run8-ops-center/internal/carstatus"
	"run8-ops-center/internal/profile"
)

var (
	lcT0   = time.Date(2026, 10, 30, 9, 0, 0, 0, time.UTC)
	lcG0   = time.Date(2026, 10, 28, 17, 0, 0, 0, time.UTC)
	lcClk0 = Clock{Real: lcT0, Game: lcG0}
)

// lcClock returns the clock n minutes after lcClk0 (game time runs 10x).
func lcClock(n int) Clock {
	return Clock{
		Real: lcT0.Add(time.Duration(n) * time.Minute),
		Game: lcG0.Add(time.Duration(n*10) * time.Minute),
	}
}

// lcProfile is a hand-made profile: NOR home yard of AB45 (HTI) and A55
// (Tuscaloosa yard line); TUS home yard of A37 (GES industry); MCC home yard of
// A44. Road train 131 sets out at MCC (yard stop) and picks up at TUS, and
// 339 has an industry stop.
func lcProfile() *profile.Profile {
	return &profile.Profile{
		ID: "test",
		Yards: []profile.Yard{
			{Code: "NOR", Name: "Norris"},
			{Code: "TUS", Name: "Tuscaloosa"},
			{Code: "MCC", Name: "McCalla"},
		},
		LocalJobs: []profile.LocalJob{
			{ID: "ab45", Symbol: "AB45", HomeYard: "NOR", Industries: []string{"HTI"}},
			{ID: "a55", Symbol: "A55", HomeYard: "NOR", YardLines: []profile.YardLine{{Yard: "TUS"}}},
			{ID: "a37", Symbol: "A37", HomeYard: "TUS", Industries: []string{"GES"}},
			{ID: "ab10", Symbol: "AB10", HomeYard: "NOR", Industries: []string{"BRR"}},
		},
		RoadTrains: []profile.RoadTrain{
			{Symbol: "131", Origin: "NOR", Stops: []profile.Stop{
				{Kind: "yard", Target: "MCC", SetOut: []string{"BRI BRI"}},
				{Kind: "yard", Target: "TUS", PickUp: []string{"NOL1"}},
			}},
			{Symbol: "339", Origin: "NOR", Stops: []profile.Stop{
				{Kind: "industry", Target: "GES", SetOut: []string{"GES"}},
			}},
		},
	}
}

func lcRef(key string) CarRef {
	return CarRef{Key: key, Number: "CAR " + key, Location: "NOR / Bowl 1", LengthFt: 50, GrossTons: 100}
}

func lcRefs(keys ...string) []CarRef {
	var out []CarRef
	for _, k := range keys {
		out = append(out, lcRef(k))
	}
	return out
}

func lcYard(code, track string) carstatus.Location {
	return carstatus.Location{Kind: carstatus.LocYard, Yard: code, Track: track}
}

func lcInd(tags ...string) carstatus.Location {
	return carstatus.Location{Kind: carstatus.LocIndustry, Industries: tags}
}

var lcOnLine = carstatus.Location{Kind: carstatus.LocIgnored}

// lcStatus builds a status holding exactly the given car locations; cars not
// listed are absent from the snapshot.
func lcStatus(locs map[string]carstatus.Location) *carstatus.Status {
	st := &carstatus.Status{ByKey: map[string]*carstatus.Car{}}
	for k, l := range locs {
		st.Cars = append(st.Cars, carstatus.Car{Key: k, Loc: l})
	}
	for i := range st.Cars {
		st.ByKey[st.Cars[i].Key] = &st.Cars[i]
	}
	return st
}

// lcAvailAB45 is an available AB45 order: an HTI line (2 set-outs, 1 pick-up)
// and a BRL line (1 set-out), with an over-length and a shared-cars flag.
func lcAvailAB45() Order {
	return Order{
		ID: "avail:ab45", Kind: KindLocal, JobID: "ab45", Symbol: "AB45", Profile: "test",
		State: StateAvailable, CreatedAt: lcT0,
		Lines: []Line{
			{Kind: LineIndustry, Target: "HTI", Name: "Holcim", SetOuts: lcRefs("h1", "h2"), PickUps: lcRefs("hp")},
			{Kind: LineIndustry, Target: "BRL", Name: "Birmingham Rail", SetOuts: lcRefs("b1")},
		},
		Totals: Totals{LengthFt: 150, GrossTons: 300, RequiredHP: 600, MaxLengthFt: 5000, MaxTons: 6000},
		Flags: []Flag{
			{Code: "over-length", Message: "too long"},
			{Code: "shared-cars", Message: "shared"},
		},
		Shared: []string{"h1"},
	}
}

// lcAvailA55 is an available A55 order with a single Tuscaloosa yard line.
func lcAvailA55() Order {
	return Order{
		ID: "avail:a55", Kind: KindLocal, JobID: "a55", Symbol: "A55", Profile: "test",
		State: StateAvailable,
		Lines: []Line{
			{Kind: LineYard, Target: "TUS", Name: "Tuscaloosa", SetOuts: lcRefs("t1", "t2"), PickUps: lcRefs("tp")},
		},
		Totals: Totals{LengthFt: 100, GrossTons: 200, RequiredHP: 400},
	}
}

// lcAvail131 is an available road order: consist of three BRI cars and two
// other cars, set-outs at MCC, one pick-up at TUS.
func lcAvail131() Order {
	return Order{
		ID: "avail:131", Kind: KindRoad, JobID: "131", Symbol: "131", Profile: "test",
		State:   StateAvailable,
		Consist: lcRefs("bri1", "bri2", "n1", "n2"),
		Lines: []Line{
			{Kind: LineStop, Target: "MCC", Name: "McCalla", SetOuts: lcRefs("bri1", "bri2")},
			{Kind: LineStop, Target: "TUS", Name: "Tuscaloosa", PickUps: lcRefs("tp1")},
		},
		Totals: Totals{LengthFt: 200, GrossTons: 400},
	}
}

func lcMustStart(o Order, lineKey, id string) Order {
	s, err := Start(o, lineKey, id, lcClk0)
	if err != nil {
		panic(err)
	}
	return s
}
