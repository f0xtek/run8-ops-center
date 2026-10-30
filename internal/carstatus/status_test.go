package carstatus

import (
	"testing"

	"run8-ops-center/internal/run8save"
)

func TestLocation(t *testing.T) {
	tm := testMap()
	tests := []struct {
		name     string
		sections []int
		kind     LocKind
		section  int
		str      string
	}{
		{"leading truck wins", []int{100, 101}, LocYard, 100, "NOR / Bowl 14"},
		{"trailing truck when leading unmapped", []int{999, 101}, LocYard, 101, "NOR / Bowl 15"},
		{"industry", []int{200, 200}, LocIndustry, 200, "industry HTI"},
		{"ignored is a label, trailing truck not consulted", []int{300, 100}, LocIgnored, 300, "on line"},
		{"both unmapped reports the leading section", []int{998, 999}, LocUnmapped, 998, "unmapped"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := snap(day(1), train("1", car("R8_Gon_X_ABCD01.xml", "1", "None", 0, tc.sections...)))
			st, _ := Derive(s, testProfile(), tm, nil)
			c := st.Cars[0]
			if c.Loc.Kind != tc.kind || c.Loc.Section != tc.section || c.Loc.String() != tc.str {
				t.Errorf("Loc = %+v (%s)", c.Loc, c.Loc)
			}
			if wantUnmapped := tc.kind == LocUnmapped; (st.Unmapped == 1) != wantUnmapped {
				t.Errorf("Unmapped = %d", st.Unmapped)
			}
		})
	}
}

func TestServiceStates(t *testing.T) {
	loco := loco("1", "AB10", 300)
	tests := []struct {
		name string
		tag  string
		sec  int
		with *run8save.Vehicle // locomotive in the train, nil for a cut
		want Service
	}{
		{"awaiting delivery", "AB45 HTI", 100, nil, ServiceAwaitingDelivery},
		{"working", "AB45 HTI", 200, nil, ServiceWorking},
		{"ready to pull, retagged", "NOL1", 200, nil, ServiceReadyToPull},
		{"ready to pull, other industry", "AB10 GES", 200, nil, ServiceReadyToPull},
		{"awaiting train", "CHA", 100, nil, ServiceAwaitingTrain},
		{"en route", "CHA", 300, &loco, ServiceEnRoute},
		{"other: cut on line", "CHA", 300, nil, ServiceOther},
		{"other: unassigned in yard", "None", 100, nil, ServiceOther},
		{"other: unmapped", "AB45 HTI", 999, &loco, ServiceOther},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			vs := []run8save.Vehicle{car("R8_Gon_X_ABCD01.xml", "7", tc.tag, 0, tc.sec, tc.sec)}
			if tc.with != nil {
				vs = append([]run8save.Vehicle{*tc.with}, vs...)
			}
			st, _ := Derive(snap(day(1), train("T1", vs...)), testProfile(), testMap(), nil)
			if len(st.Cars) != 1 {
				t.Fatalf("locomotives must not appear in Cars, got %d cars", len(st.Cars))
			}
			if got := st.Cars[0].Service; got != tc.want {
				t.Errorf("Service = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestTrainFields(t *testing.T) {
	s := snap(day(1),
		train("T1", loco("1", "AB10", 300), car("R8_Gon_X_ABCD01.xml", "1", "None", 0, 300, 300)),
		train("T2", car("R8_Gon_X_ABCD01.xml", "2", "None", 0, 100, 100)),
	)
	st, _ := Derive(s, testProfile(), testMap(), nil)
	a, b := st.ByKey["R8_Gon_X_ABCD01.xml#1"], st.ByKey["R8_Gon_X_ABCD01.xml#2"]
	if a == nil || b == nil {
		t.Fatal("ByKey missing a car")
	}
	if a.TrainID != "T1" || a.TrainSymbol != "AB10" {
		t.Errorf("powered train: %q %q", a.TrainID, a.TrainSymbol)
	}
	if b.TrainID != "T2" || b.TrainSymbol != "" {
		t.Errorf("cut: %q %q", b.TrainID, b.TrainSymbol)
	}
	if a != &st.Cars[0] {
		t.Error("ByKey must point into Cars")
	}
}
