package profile

import (
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestLoadLimitsFixture(t *testing.T) {
	f, err := os.Open("../../testdata/yard_and_train_limits.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	limits, codes, err := LoadLimits(f)
	if err != nil {
		t.Fatal(err)
	}

	// NOL1 belongs to train 131, which allows 8,000 ft and 8,000 tons.
	var owner string
	for sym, cs := range codes {
		for _, c := range cs {
			if c == "NOL1" {
				owner = sym
			}
		}
	}
	if owner != "131" {
		t.Fatalf("NOL1 maps to %q, want 131", owner)
	}
	if l := limits["131"]; l.MaxLengthFt != 8000 || l.MaxTons != 8000 || l.MinHPT != 1.0 {
		t.Errorf("131 limit = %+v", l)
	}
	if want := []string{"BRI", "NOL1"}; !reflect.DeepEqual(codes["131"], want) {
		t.Errorf("131 codes = %q", codes["131"])
	}
	if want := []string{"A55", "A37", "A56", "AGR", "ABS"}; !reflect.DeepEqual(codes["A55"], want) {
		t.Errorf("A55 codes = %q", codes["A55"])
	}
	if l := limits["AB10"]; l.MaxLengthFt != 4000 || l.MaxTons != 6000 {
		t.Errorf("AB10 limit = %+v (thousands separator)", l)
	}
	if l := limits["341"]; l.MaxLengthFt != 6000 {
		t.Errorf("341 limit = %+v", l)
	}

	// The 21 train rows are the whole table: the subdivision table after the
	// blank line is ignored.
	if len(limits) != 21 || len(codes) != 21 {
		t.Errorf("got %d limit rows, want 21", len(limits))
	}
	for _, k := range []string{"3B North", "3B", "AGS North", "AGS South", "East End", "Subdivision", "N/A West End", "AGS Subdivision Limits:"} {
		if _, ok := limits[k]; ok {
			t.Errorf("subdivision row %q leaked into limits", k)
		}
	}
}

func TestLoadLimitsBlankCellsAndStop(t *testing.T) {
	in := "Base\tCodes\tDest\tHPT\tLen\tTons\tTrailing\r\n" +
		"T1\tAA, BB ,\tPlace, ST\t\t\t\t\r\n" +
		"T2\tCC\tOther\t0.9\t12,500\t\t7,000\r\n" +
		"  \r\n" +
		"T3\tDD\tNever\t1\t1\t1\t1\r\n"
	limits, codes, err := LoadLimits(strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	if len(limits) != 2 {
		t.Fatalf("rows = %v, want T1 and T2 only (stop at first blank line)", limits)
	}
	if limits["T1"] != (Limit{}) {
		t.Errorf("T1 = %+v, blank cells mean no limit", limits["T1"])
	}
	if want := []string{"AA", "BB"}; !reflect.DeepEqual(codes["T1"], want) {
		t.Errorf("T1 codes = %q", codes["T1"])
	}
	if l := limits["T2"]; l.MinHPT != 0.9 || l.MaxLengthFt != 12500 || l.MaxTons != 0 {
		t.Errorf("T2 = %+v", l)
	}
}

func TestLoadLimitsErrors(t *testing.T) {
	cases := map[string]string{
		"bad length": "h\nT1\tAA\tD\t1.0\tlong\t1\t\n",
		"bad hpt":    "h\nT1\tAA\tD\tfast\t1\t1\t\n",
		"no symbol":  "h\n\tAA\tD\t1.0\t1\t1\t\n",
	}
	for name, in := range cases {
		if _, _, err := LoadLimits(strings.NewReader(in)); err == nil {
			t.Errorf("%s: want error", name)
		}
	}
}
