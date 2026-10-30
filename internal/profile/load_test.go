package profile

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const miniCSV = "Title\r\n" +
	"Industry Name,Incoming Tag,Local Symbol,Car Type,Time,Capacity,Produces,Outgoing Tags\r\n" +
	"Alpha Works,ALP,L1,Box Car,24,4,Loads,\"EAST,WEST\"\r\n" +
	",,,Gondola,24,2,Empties,\"EAST\"\r\n" +
	"Beta Mill,BET,L1,Gondola,24,3,Empties,\"WEST,EAST\"\r\n" +
	"Gamma Yard Customer,GAM,L2,Box Car,24,1,Loads,\"EAST\"\r\n"

const miniTSV = "Base Train Symbol\tClass Code(s)\tDest\tMin HP/T\tMax Length\tMax Tons\tMax Trailing Tons\n" +
	"L1\tL1\tLocalville\t1.0\t3,000\t4,000\t\n" +
	"EB\tEAST, E2\tEastport, ST\t1.1\t8,000\t9,000\t\n" +
	"WB\tWEST\tWestport\t1.0\t5,000\t6,000\t\n"

const miniYAML = `
id: mini
name: Mini Route
defaultSaveDir: 'C:\Saves'
defaultAutoSaveDir: 'C:\Saves\Auto'
yards:
  - {code: HOM, name: Home}
  - {code: SAT, name: Satellite}
localJobs:
  - symbol: L1
    homeYard: HOM
    yardLines:
      - yard: SAT
        carries: [L2, GAM]
  - id: l2-job
    symbol: L2
    homeYard: SAT
    industries: [GAM]
    limit: {maxLengthFt: 1500}
roadTrains:
  - symbol: EB
    origin: HOM
    stops:
      - kind: yard
        target: SAT
        setOut: [WEST]
        pickUp: [EXTRA]
        pickUpCarTypes: [Box Car]
  - symbol: WB
    destination: Somewhere Else
    minLengthPct: 25
    maxCarAgeDays: 2.5
    limit: {maxTons: 0}
  - symbol: TH
    stops:
      - target: SAT
        pickUp: [E2]
carTypes:
  - {prefix: Box_, type: Box Car}
carDefaults:
  Box Car: {lengthFt: 50, tareTons: 30}
aliases:
  " LEGACY ": EAST
yardInternalTags: [HOM, HOM  Hump]
routePrefixes: [10, 11]
`

// writeProfile writes a profile directory under t.TempDir().
func writeProfile(t *testing.T, yaml, csv, tsv string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range map[string]string{ProfileFile: yaml, IndustryFile: csv, LimitsFile: tsv} {
		if content == "" {
			continue
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestLoadMinimalProfile(t *testing.T) {
	dir := writeProfile(t, miniYAML, miniCSV, miniTSV)
	p, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if p.ID != "mini" || p.Name != "Mini Route" || p.Dir != dir {
		t.Errorf("identity: %q %q %q", p.ID, p.Name, p.Dir)
	}
	if p.DefaultSaveDir != `C:\Saves` || p.DefaultAutoSaveDir != `C:\Saves\Auto` {
		t.Errorf("dirs: %q %q", p.DefaultSaveDir, p.DefaultAutoSaveDir)
	}
	if len(p.Yards) != 2 || p.YardByCode("SAT").Name != "Satellite" {
		t.Errorf("yards = %+v", p.Yards)
	}
	if len(p.Industries) != 3 {
		t.Fatalf("industries = %d", len(p.Industries))
	}

	// Local with a yard line. Industries default from the CSV local symbol,
	// the ID defaults to the lower-cased symbol and limits come from the TSV.
	l1 := p.LocalJobByID("l1")
	if l1 == nil {
		t.Fatal("local l1 missing")
	}
	if l1.Symbol != "L1" || l1.HomeYard != "HOM" {
		t.Errorf("l1 = %+v", l1)
	}
	if want := []string{"ALP", "BET"}; !reflect.DeepEqual(l1.Industries, want) {
		t.Errorf("l1 industries = %v, want %v (default from CSV local symbol)", l1.Industries, want)
	}
	if want := []YardLine{{Yard: "SAT", Carries: []string{"L2", "GAM"}}}; !reflect.DeepEqual(l1.YardLines, want) {
		t.Errorf("l1 yard lines = %+v", l1.YardLines)
	}
	if l1.Limit != (Limit{MinHPT: 1.0, MaxLengthFt: 3000, MaxTons: 4000}) {
		t.Errorf("l1 limit = %+v", l1.Limit)
	}
	if got := p.LocalJobsServing("BET"); len(got) != 1 || got[0].ID != "l1" {
		t.Errorf("LocalJobsServing(BET) = %v", got)
	}

	// An explicit list replaces the default. A limit override replaces one
	// field and leaves the others (there is no TSV row for L2).
	l2 := p.LocalJobByID("l2-job")
	if l2 == nil || !reflect.DeepEqual(l2.Industries, []string{"GAM"}) {
		t.Fatalf("l2 = %+v", l2)
	}
	if l2.Limit != (Limit{MaxLengthFt: 1500}) {
		t.Errorf("l2 limit = %+v", l2.Limit)
	}

	// Road train with a stop. Class codes, destination and limits default
	// from the TSV; a train with an origin defaults to 50%.
	eb := p.RoadTrainBySymbol("EB")
	if eb == nil || eb.Origin != "HOM" {
		t.Fatalf("EB = %+v", eb)
	}
	if want := []string{"EAST", "E2"}; !reflect.DeepEqual(eb.ClassCodes, want) {
		t.Errorf("EB class codes = %v", eb.ClassCodes)
	}
	if eb.Destination != "Eastport, ST" || eb.Limit != (Limit{MinHPT: 1.1, MaxLengthFt: 8000, MaxTons: 9000}) {
		t.Errorf("EB dest/limit = %q %+v", eb.Destination, eb.Limit)
	}
	if eb.MinLengthPct != 50 {
		t.Errorf("EB MinLengthPct = %v, want 50", eb.MinLengthPct)
	}
	wantStop := Stop{Kind: "yard", Target: "SAT", SetOut: []string{"WEST"}, PickUp: []string{"EXTRA"}, PickUpCarTypes: []CarType{"Box Car"}}
	if len(eb.Stops) != 1 || !reflect.DeepEqual(eb.Stops[0], wantStop) {
		t.Errorf("EB stops = %+v", eb.Stops)
	}

	// Overrides: destination, thresholds and an explicit 0 limit ("no limit").
	wb := p.RoadTrainBySymbol("WB")
	if wb.Destination != "Somewhere Else" || wb.MinLengthPct != 25 || wb.MaxCarAgeDays != 2.5 {
		t.Errorf("WB = %+v", wb)
	}
	if wb.Limit != (Limit{MinHPT: 1.0, MaxLengthFt: 5000}) {
		t.Errorf("WB limit = %+v, want maxTons overridden to 0 and the rest from the TSV", wb.Limit)
	}

	// Through train: no origin, 0% default, stop kind inferred.
	th := p.RoadTrainBySymbol("TH")
	if th.Origin != "" || th.MinLengthPct != 0 || len(th.ClassCodes) != 0 {
		t.Errorf("TH = %+v", th)
	}
	if th.Stops[0].Kind != "yard" {
		t.Errorf("TH stop kind = %q, want inferred yard", th.Stops[0].Kind)
	}
	if got := p.RoadTrainForTag("E2"); got == nil || got.Symbol != "EB" {
		t.Errorf("RoadTrainForTag(E2) = %v", got)
	}

	// Car types, defaults, aliases, internal tags, prefixes.
	if p.CarTypeOf("Box_x.xml") != "Box Car" || p.CarTypeOf("Other.xml") != Unknown {
		t.Error("CarTypeOf")
	}
	if d := p.DefaultsFor("Box Car"); d != (CarDefaults{LengthFt: 50, TareTons: 30}) {
		t.Errorf("Box Car defaults = %+v", d)
	}
	if d := p.DefaultsFor("Gondola"); d != (CarDefaults{LengthFt: 60, TareTons: 30}) {
		t.Errorf("fallback defaults = %+v", d)
	}
	if p.Aliases["LEGACY"] != "EAST" {
		t.Errorf("aliases = %v", p.Aliases)
	}
	if !p.IsYardInternalTag("HOM Hump") {
		t.Errorf("internal tags = %v", p.YardInternalTags)
	}
	if !reflect.DeepEqual(p.RoutePrefixes, []int{10, 11}) {
		t.Errorf("prefixes = %v", p.RoutePrefixes)
	}
}

func TestLoadMinimalProfileReportIsClean(t *testing.T) {
	p, err := Load(writeProfile(t, miniYAML, miniCSV, miniTSV))
	if err != nil {
		t.Fatal(err)
	}
	// Gondola has no car-type rule, so only that is reported.
	for _, pr := range p.Report.Problems {
		if pr.Code != CodeUnmappedCarType {
			t.Errorf("unexpected problem %+v", pr)
		}
	}
}

func TestAliasRoutesOrphanTag(t *testing.T) {
	csv := strings.Replace(miniCSV, `"EAST,WEST"`, `"EAST,WESTCP"`, 1)

	without := strings.Replace(miniYAML, `  " LEGACY ": EAST`, `  WESTCP: NOPE`, 1)
	p, err := Load(writeProfile(t, without, csv, miniTSV))
	if err != nil {
		t.Fatal(err)
	}
	if !hasProblem(p, CodeUnknownRef, "WESTCP") {
		t.Errorf("alias to a tag nobody handles must be reported: %+v", p.Report.Problems)
	}

	with := strings.Replace(miniYAML, `  " LEGACY ": EAST`, `  WESTCP: WEST`, 1)
	p, err = Load(writeProfile(t, with, csv, miniTSV))
	if err != nil {
		t.Fatal(err)
	}
	if got := p.RoadTrainForTag("WESTCP"); got == nil || got.Symbol != "WB" {
		t.Errorf("RoadTrainForTag(WESTCP) = %v, want WB", got)
	}
	if hasProblem(p, CodeOrphanTag, "WESTCP") {
		t.Error("aliased tag still reported as orphan")
	}
}

func hasProblem(p *Profile, code, subject string) bool {
	for _, pr := range p.Report.Problems {
		if pr.Code == code && pr.Subject == subject {
			return true
		}
	}
	return false
}

func TestLoadReportsProblemsWithoutFailing(t *testing.T) {
	yaml := `
id: broken
yards:
  - {code: HOM, name: Home}
localJobs:
  - symbol: L1
    homeYard: NOWHERE
    industries: [ALP, MISSING]
    yardLines:
      - yard: GHOST
        carries: [X]
  - id: l1
    symbol: L1B
    homeYard: HOM
    industries: [ALP]
roadTrains:
  - symbol: EB
    origin: HOM
    offMapIndustries: [NOPE]
    stops:
      - {kind: yard, target: SAT}
      - {kind: industry, target: ZZZ}
      - {kind: depot, target: HOM}
`
	csv := miniCSV + "Dupe,ALP,L9,Box Car,1,1,Loads,\"A B,EAST,EAST\"\r\n"
	p, err := Load(writeProfile(t, yaml, csv, miniTSV))
	if err != nil {
		t.Fatalf("Load must not fail on validation problems: %v", err)
	}
	for _, w := range []struct{ code, subject string }{
		{CodeUnknownRef, "NOWHERE"},
		{CodeUnknownRef, "MISSING"},
		{CodeUnknownRef, "GHOST"},
		{CodeUnknownRef, "SAT"},
		{CodeUnknownRef, "ZZZ"},
		{CodeUnknownRef, "NOPE"},
		{CodeUnknownRef, "HOM"}, // stop kind "depot"
		{CodeDuplicateTag, "ALP"},
		{CodeDuplicateTag, "l1"},
		{CodeDuplicateTag, "EAST"},
		{CodeMalformedTag, "A B"},
		{CodeUnservedIndustry, "BET"},
		{CodeUnservedIndustry, "GAM"},
		{CodeOrphanTag, "WEST"}, // WB train is not defined in this profile
	} {
		if !hasProblem(p, w.code, w.subject) {
			t.Errorf("missing problem %s %q in %+v", w.code, w.subject, p.Report.Problems)
		}
	}
	if hasProblem(p, CodeUnservedIndustry, "ALP") {
		t.Error("ALP is served by a local job")
	}
}

func TestLoadStopPickUpResolvesOrphanTag(t *testing.T) {
	yaml := `
id: pick
yards: [{code: HOM, name: Home}]
localJobs:
  - {symbol: L1, homeYard: HOM}
roadTrains:
  - symbol: TH
    stops:
      - {kind: yard, target: HOM, pickUp: [WEST]}
`
	p, err := Load(writeProfile(t, yaml, miniCSV, ""))
	if err != nil {
		t.Fatal(err)
	}
	if hasProblem(p, CodeOrphanTag, "WEST") {
		t.Error("stop pick-up tag WEST must count as resolved")
	}
	if !hasProblem(p, CodeOrphanTag, "EAST") {
		t.Error("EAST has no train and must be an orphan")
	}
}

func TestLoadErrors(t *testing.T) {
	if _, err := Load(t.TempDir()); err == nil {
		t.Error("missing profile.yaml must fail")
	}
	if _, err := Load(writeProfile(t, "id: x\nbogusField: 1\n", miniCSV, miniTSV)); err == nil {
		t.Error("unknown yaml field must fail")
	}
	if _, err := Load(writeProfile(t, "id: [unclosed", miniCSV, miniTSV)); err == nil {
		t.Error("bad yaml must fail")
	}
	if _, err := Load(writeProfile(t, "id: x\n", "", miniTSV)); err == nil {
		t.Error("missing csv must fail")
	}
	if _, err := Load(writeProfile(t, "# nothing\n", miniCSV, miniTSV)); err == nil {
		t.Error("empty yaml must fail")
	}
	p, err := Load(writeProfile(t, "id: x\n", miniCSV, ""))
	if err != nil || len(p.Industries) != 3 {
		t.Errorf("tsv is optional: %v", err)
	}
}

func TestUnmatchedModels(t *testing.T) {
	p, err := Load(writeProfile(t, miniYAML, miniCSV, miniTSV))
	if err != nil {
		t.Fatal(err)
	}
	got := p.UnmatchedModels([]string{"Box_a.xml", "Zed.xml", "Alt.xml", "Zed.xml"})
	if len(got) != 2 || got[0].Subject != "Alt.xml" || got[1].Subject != "Zed.xml" || got[0].Code != CodeUnmatchedModel {
		t.Errorf("got %+v", got)
	}
}
