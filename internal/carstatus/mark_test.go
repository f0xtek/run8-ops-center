package carstatus

import (
	"testing"

	"run8-ops-center/internal/profile"
)

func TestMark(t *testing.T) {
	tests := []struct{ model, mark string }{
		{"R8_Reefer_PCF_57_rbld_ARMN.xml", "ARMN"},
		{"R8_Bulkhead_Thrall_TTJX01.xml", "TTJX"},
		{"Run8_Tank107GATX02.xml", "GATX"},
		{"Run8_Tank105_BNSF.xml", "BNSF"},
		{"R8_AutoRack_Tri_NS01.xml", "NS"},
		{"R8_Autorack_GTW01.xml", "GTW"},
		{"Run8_WellCar53AP_FEC14.xml", "FEC"},
		{"Run8_52ftwell_53_40_SchneiderJBHunt.xml", ""},
		{"R8_Boxcar_50ft_PlateF.xml", ""},
		{"Mystery.xml", ""},
		{"", ""},
	}
	for _, tc := range tests {
		if got := Mark(tc.model); got != tc.mark {
			t.Errorf("Mark(%q) = %q, want %q", tc.model, got, tc.mark)
		}
	}
}

func TestCarIdentity(t *testing.T) {
	p := testProfile()
	tm := testMap()
	s := snap(day(1), train("1",
		car("R8_Reefer_PCF_57_rbld_ARMN.xml", "194887", "None", 0, 100, 100),
		car("R8_Bulkhead_Thrall_TTJX01.xml", "5", "None", 0, 100, 100),
		car("Run8_Tank107GATX02.xml", "77", "None", 0, 100, 100),
		car("Run8_52ftwell_53_40_SchneiderJBHunt.xml", "9", "None", 0, 100, 100),
	))
	st, _ := Derive(s, p, tm, nil)
	want := []struct{ key, number, mark string }{
		{"R8_Reefer_PCF_57_rbld_ARMN.xml#194887", "ARMN 194887", "ARMN"},
		{"R8_Bulkhead_Thrall_TTJX01.xml#5", "TTJX 5", "TTJX"},
		{"Run8_Tank107GATX02.xml#77", "GATX 77", "GATX"},
		{"Run8_52ftwell_53_40_SchneiderJBHunt.xml#9", "9", ""},
	}
	if len(st.Cars) != len(want) {
		t.Fatalf("got %d cars", len(st.Cars))
	}
	for i, w := range want {
		c := st.Cars[i]
		if c.Key != w.key || c.Number != w.number || c.Mark != w.mark {
			t.Errorf("car %d = %q %q %q, want %v", i, c.Key, c.Number, c.Mark, w)
		}
	}
}

func TestLoadAndWeight(t *testing.T) {
	p := testProfile()
	s := snap(day(1), train("1",
		car("R8_Gon_THR2743_DJJX01.xml", "1", "None", 0, 100, 100),
		car("R8_Gon_THR2743_DJJX01.xml", "2", "None", 70.5, 100, 100),
		car("Run8_Tank105_BNSF.xml", "3", "None", 0, 100, 100),
		car("R8_Boxcar_50ft_PlateF_CN01.xml", "4", "None", 0, 100, 100),
	))
	st, _ := Derive(s, p, testMap(), nil)
	type want struct {
		typ                 profile.CarType
		loaded              bool
		tare, gross, length float64
	}
	cases := []want{
		{"Gondola", false, 30, 30, 52},       // empty: gross is tare
		{"Gondola", true, 30, 100.5, 52},     // loaded: load plus tare
		{profile.Unknown, false, 32, 32, 55}, // unknown type uses Unknown defaults
		{"Box Car", false, 35, 35, 60},       // length fallback from the type default
	}
	for i, w := range cases {
		c := st.Cars[i]
		if c.Type != w.typ || c.Loaded != w.loaded || c.TareTons != w.tare || c.GrossTons != w.gross || c.LengthFt != w.length {
			t.Errorf("car %d = %+v, want %+v", i, c, w)
		}
	}
	if st.Unknown != 1 {
		t.Errorf("Unknown = %d, want 1", st.Unknown)
	}
}
