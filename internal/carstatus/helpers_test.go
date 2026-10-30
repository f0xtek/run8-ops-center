package carstatus

import (
	"time"

	"run8-ops-center/internal/profile"
	"run8-ops-center/internal/run8save"
	"run8-ops-center/internal/trackmap"
)

// testProfile is a small hand-written profile modelled on AGS South Phase 1.
func testProfile() *profile.Profile {
	return &profile.Profile{
		Yards: []profile.Yard{{Code: "NOR", Name: "Norris"}},
		Industries: []profile.Industry{
			{Name: "Holcim Trading Inc", Tag: "HTI", LocalSymbol: "AB45"},
			{Name: "Birmingham Rail", Tag: "BRR", LocalSymbol: "AB10"},
			{Name: "Gulf Electric Supply", Tag: "GES", LocalSymbol: "AB10"},
			{Name: "Physical Security", Tag: "PHS", LocalSymbol: "A28"},
			{Name: "BRL cars", Tag: "BRL", LocalSymbol: "AB45"},
			{Name: "ABS", Tag: "ABS", LocalSymbol: "ABS"},
			{Name: "AGR", Tag: "AGR", LocalSymbol: "AGR"},
			{Name: "Simulated for Local train", Tag: "A28", LocalSymbol: "A28"},
		},
		LocalJobs: []profile.LocalJob{
			{ID: "ab45", Symbol: "AB45", HomeYard: "NOR", Industries: []string{"HTI"}},
			{ID: "csx-bes", Symbol: "CSXBES", HomeYard: "NOR", Industries: []string{"BRL"}},
			{ID: "a55", Symbol: "A55", HomeYard: "NOR", Industries: []string{"ABS"}},
			{ID: "a56", Symbol: "A56", HomeYard: "NOR", Industries: []string{"AGR"}},
			{ID: "ab10", Symbol: "AB10", HomeYard: "NOR", Industries: []string{"BRR", "GES"}},
		},
		RoadTrains: []profile.RoadTrain{
			{Symbol: "108", Origin: "NOR", ClassCodes: []string{"CHA"}},
			{Symbol: "346", Origin: "NOR", ClassCodes: []string{"NOL1"}},
			{
				Symbol: "A28", Origin: "NOR", ClassCodes: []string{"A28"}, OffMapIndustries: []string{"A28"},
				Stops: []profile.Stop{{Kind: "industry", Target: "PHS"}},
			},
		},
		CarTypes: []profile.CarTypeRule{
			{Prefix: "R8_Gon_", Type: "Gondola"},
			{Prefix: "R8_Box", Type: "Box Car"},
		},
		CarDefaults:      map[profile.CarType]profile.CarDefaults{"Gondola": {LengthFt: 52, TareTons: 30}, "Box Car": {LengthFt: 60, TareTons: 35}},
		UnknownDefaults:  profile.CarDefaults{LengthFt: 55, TareTons: 32},
		Aliases:          map[string]string{"NOL": "NOL1"},
		YardInternalTags: []string{"NOR", "NOR Hump"},
	}
}

// Test sections: 100 yard NOR/Bowl 14, 101 yard NOR/Bowl 15, 200 industry HTI,
// 201 industry GES, 300 ignored. Anything else is unmapped.
func testMap() *trackmap.Map {
	m := trackmap.New()
	m.Set(1, 100, trackmap.Label{Kind: trackmap.KindYard, Yard: "NOR", Track: "Bowl 14"})
	m.Set(1, 101, trackmap.Label{Kind: trackmap.KindYard, Yard: "NOR", Track: "Bowl 15"})
	m.Set(1, 200, trackmap.Label{Kind: trackmap.KindIndustry, Industries: []string{"HTI"}})
	m.Set(1, 201, trackmap.Label{Kind: trackmap.KindIndustry, Industries: []string{"GES"}})
	m.Set(1, 300, trackmap.Label{Kind: trackmap.KindIgnored})
	return m
}

func car(model, unit, tag string, tons float64, sections ...int) run8save.Vehicle {
	v := run8save.Vehicle{Model: model, UnitType: "US_Freightcar", UnitNumber: unit, LoadTons: tons, Tag: tag}
	for _, s := range sections {
		v.Trucks = append(v.Trucks, run8save.Truck{RoutePrefix: 1, Section: s, DistanceM: 10})
	}
	return v
}

func loco(unit, tag string, section int) run8save.Vehicle {
	v := car("Run8_SD70ACE_NS01.xml", unit, tag, 0, section, section)
	v.UnitType = "US_DieselEngine"
	return v
}

func snap(date time.Time, trains ...run8save.Train) *run8save.Snapshot {
	return &run8save.Snapshot{GameDate: date, Trains: trains}
}

func train(id string, vs ...run8save.Vehicle) run8save.Train {
	return run8save.Train{ID: id, Vehicles: vs}
}

func day(n int) time.Time { return time.Date(2026, 1, n, 6, 0, 0, 0, time.UTC) }
