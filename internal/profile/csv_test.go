package profile

import (
	"os"
	"reflect"
	"strings"
	"testing"
)

func loadFixtureCSV(t *testing.T) []Industry {
	t.Helper()
	f, err := os.Open("../../testdata/IndustryConfiguration.csv")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	inds, err := LoadIndustryCSV(f)
	if err != nil {
		t.Fatalf("LoadIndustryCSV: %v", err)
	}
	return inds
}

func findIndustry(inds []Industry, tag string) *Industry {
	for i := range inds {
		if inds[i].Tag == tag {
			return &inds[i]
		}
	}
	return nil
}

func TestLoadIndustryCSVFixture(t *testing.T) {
	inds := loadFixtureCSV(t)
	entries := 0
	for _, in := range inds {
		entries += len(in.Cars)
	}
	if len(inds) != 46 || entries != 68 {
		t.Fatalf("got %d industries and %d car-type entries, want 46 and 68", len(inds), entries)
	}

	cmc := findIndustry(inds, "CMC")
	if cmc == nil {
		t.Fatal("CMC missing")
	}
	var types []CarType
	for _, c := range cmc.Cars {
		types = append(types, c.CarType)
	}
	if want := []CarType{"Gondola", "Covered Hopper", "Bulkhead_Flat"}; !reflect.DeepEqual(types, want) {
		t.Errorf("CMC car types = %v, want %v", types, want)
	}
	if cmc.Name != "CMC Steel" || cmc.LocalSymbol != "AB10" {
		t.Errorf("CMC = %q / %q", cmc.Name, cmc.LocalSymbol)
	}
	if c := cmc.Cars[0]; c.TimeHours != 48 || c.Capacity != 16 || c.Produces != "Loads" {
		t.Errorf("CMC gondola = %+v", c)
	}
	if n, ok := cmc.Capacity("Covered Hopper"); !ok || n != 3 {
		t.Errorf("CMC covered hopper capacity = %d, %v", n, ok)
	}
	if _, ok := cmc.Capacity("Tank Car"); ok {
		t.Error("CMC takes no tank cars")
	}
	if cmc.TotalCapacity() != 23 {
		t.Errorf("CMC total capacity = %d, want 23", cmc.TotalCapacity())
	}
}

func TestLoadIndustryCSVPaddedIncomingTag(t *testing.T) {
	inds := loadFixtureCSV(t)
	brr := findIndustry(inds, "BRR")
	if brr == nil {
		t.Fatal("BRR missing: padded incoming tag not normalised")
	}
	if brr.Tag != "BRR" || NormTag("AB10  BRR") != "AB10 BRR" {
		t.Errorf("tag %q", brr.Tag)
	}
	p := &Profile{Industries: inds}
	if p.IndustryByTag("BRR") == nil || p.IndustryByTag(" BRR ") == nil {
		t.Error("IndustryByTag does not match BRR")
	}
}

func TestLoadIndustryCSVKeepsBadAndDuplicateOutgoingTags(t *testing.T) {
	inds := loadFixtureCSV(t)

	dab := findIndustry(inds, "DAB")
	want := []string{"ATL NOL3", "CSX", "BNSF", "MERCP", "CHA"}
	if !reflect.DeepEqual(dab.Cars[0].OutgoingTags, want) {
		t.Errorf("DAB tags = %q, want %q", dab.Cars[0].OutgoingTags, want)
	}

	ons := findIndustry(inds, "ONS")
	n := 0
	for _, tag := range ons.Cars[0].OutgoingTags {
		if tag == "CSX" {
			n++
		}
	}
	if n != 2 {
		t.Errorf("ONS lists CSX %d times, want 2 (duplicate kept for the validator)", n)
	}
}

func TestLoadIndustryCSVUnquotedCommaInName(t *testing.T) {
	inds := loadFixtureCSV(t)
	sti := findIndustry(inds, "STI")
	if sti == nil || sti.Name != "Schnitzer Steel Industries, Inc" {
		t.Fatalf("STI = %+v", sti)
	}
	if sti.LocalSymbol != "AB45" || sti.Cars[0].CarType != "Gondola" || sti.Cars[0].Capacity != 10 {
		t.Errorf("STI fields shifted: %+v", sti)
	}
	if len(sti.Cars[0].OutgoingTags) != 10 {
		t.Errorf("STI tags = %q", sti.Cars[0].OutgoingTags)
	}
}

func TestLoadIndustryCSVQuotedAndMultiline(t *testing.T) {
	in := "\ufeffTitle\r\n" +
		"Industry Name,Incoming Tag,Local Symbol,Car Type,Time,Capacity,Produces,Outgoing Tags\r\n" +
		"\"Quoted, Name\",QN,L1,Box Car,12,3,Loads,\" AAA ,B  B,,CCC\"\r\n" +
		",,,Gondola,,2,Empties,\r\n" +
		",,,,,,,\r\n"
	inds, err := LoadIndustryCSV(strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	if len(inds) != 1 || inds[0].Name != "Quoted, Name" || len(inds[0].Cars) != 2 {
		t.Fatalf("got %+v", inds)
	}
	if want := []string{"AAA", "B B", "CCC"}; !reflect.DeepEqual(inds[0].Cars[0].OutgoingTags, want) {
		t.Errorf("tags = %q, want %q", inds[0].Cars[0].OutgoingTags, want)
	}
	g := inds[0].Cars[1]
	if g.TimeHours != 0 || g.Capacity != 2 || g.Produces != "Empties" || len(g.OutgoingTags) != 0 {
		t.Errorf("blank cells: %+v", g)
	}
}

func TestLoadIndustryCSVErrors(t *testing.T) {
	const header = "Industry Name,Incoming Tag,Local Symbol,Car Type,Time,Capacity,Produces,Outgoing Tags\n"
	cases := map[string]string{
		"no header":            "just a title\nA,B,C,Box Car,1,1,Loads,X\n",
		"continuation first":   header + ",,,Box Car,1,1,Loads,X\n",
		"short row":            header + "A,B,C\n",
		"non-numeric capacity": header + "A,AA,L,Box Car,1,many,Loads,X\n",
		"non-numeric time":     header + "A,AA,L,Box Car,soon,1,Loads,X\n",
	}
	for name, in := range cases {
		if _, err := LoadIndustryCSV(strings.NewReader(in)); err == nil {
			t.Errorf("%s: want error", name)
		}
	}
}
