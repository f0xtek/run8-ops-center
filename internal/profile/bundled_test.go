package profile

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// recordingWriter is a FileWriter that writes to the real file system and
// records every path it was asked to touch.
type recordingWriter struct {
	paths []string
}

func (w *recordingWriter) MkdirAll(path string, perm os.FileMode) error {
	w.paths = append(w.paths, path)
	return os.MkdirAll(path, perm)
}

func (w *recordingWriter) WriteFile(path string, data []byte, perm os.FileMode) error {
	w.paths = append(w.paths, path)
	return os.WriteFile(path, data, perm)
}

// installBundled installs the bundled profile into a fresh data directory and
// returns the loaded profile.
func installBundled(t *testing.T) (*Profile, string) {
	t.Helper()
	profiles := filepath.Join(t.TempDir(), "profiles")
	w := &recordingWriter{}
	if err := InstallBundled(w, profiles); err != nil {
		t.Fatalf("InstallBundled: %v", err)
	}
	dir := filepath.Join(profiles, BundledID)
	p, err := Load(dir)
	if err != nil {
		t.Fatalf("Load installed profile: %v", err)
	}
	return p, dir
}

func TestInstallBundledOnEmptyDataDir(t *testing.T) {
	profiles := filepath.Join(t.TempDir(), "profiles")
	w := &recordingWriter{}
	if err := InstallBundled(w, profiles); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(profiles, "ags-south-p1")
	for _, name := range []string{"profile.yaml", "IndustryConfiguration.csv", "train_limits.tsv"} {
		if st, err := os.Stat(filepath.Join(dir, name)); err != nil || st.Size() == 0 {
			t.Errorf("%s not installed: %v", name, err)
		}
	}
	for _, p := range w.paths {
		if !strings.HasPrefix(p, dir) {
			t.Errorf("wrote outside the profile dir: %s", p)
		}
	}

	p, err := Load(dir)
	if err != nil {
		t.Fatalf("installed profile does not load: %v", err)
	}
	if p.ID != "ags-south-p1" {
		t.Errorf("ID = %q", p.ID)
	}
}

func TestInstallBundledNeverOverwrites(t *testing.T) {
	profiles := t.TempDir()
	if err := InstallBundled(&recordingWriter{}, profiles); err != nil {
		t.Fatal(err)
	}
	yamlPath := filepath.Join(profiles, BundledID, ProfileFile)
	if err := os.WriteFile(yamlPath, []byte("# user edit\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	w := &recordingWriter{}
	if err := InstallBundled(w, profiles); err != nil {
		t.Fatal(err)
	}
	if len(w.paths) != 0 {
		t.Errorf("second install wrote %v", w.paths)
	}
	got, _ := os.ReadFile(yamlPath)
	if string(got) != "# user edit\n" {
		t.Errorf("user edit overwritten: %q", got)
	}
}

func TestBundledDataFilesAreByteExactFixtures(t *testing.T) {
	for embedded, fixture := range map[string]string{
		IndustryFile: "../../testdata/IndustryConfiguration.csv",
		LimitsFile:   "../../testdata/yard_and_train_limits.txt",
	} {
		want, err := os.ReadFile(fixture)
		if err != nil {
			t.Fatal(err)
		}
		got, err := bundledFS.ReadFile("bundled/ags-south-p1/" + embedded)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("%s differs from %s", embedded, fixture)
		}
	}
}

func TestBundledAGSDefaults(t *testing.T) {
	p, _ := installBundled(t)
	if p.DefaultSaveDir != `C:\Run8Studios\Run8 Train Simulator V3\Content\V3Routes\Regions\GulfCoast\Trains` {
		t.Errorf("DefaultSaveDir = %q", p.DefaultSaveDir)
	}
	if p.DefaultAutoSaveDir != `C:\Run8Studios\Run8 Train Simulator V3\Content\V3Routes\Regions\GulfCoast\AutoSaves` {
		t.Errorf("DefaultAutoSaveDir = %q", p.DefaultAutoSaveDir)
	}
	want := map[string]string{"NOR": "Norris", "TUS": "Tuscaloosa", "MCC": "McCalla", "BES": "Bessemer", "32ND": "32nd Street"}
	if len(p.Yards) != len(want) {
		t.Errorf("yards = %+v", p.Yards)
	}
	for code, name := range want {
		if y := p.YardByCode(code); y == nil || y.Name != name {
			t.Errorf("yard %s = %+v", code, y)
		}
	}
	if !reflect.DeepEqual(p.RoutePrefixes, []int{280}) {
		t.Errorf("RoutePrefixes = %v", p.RoutePrefixes)
	}
	for _, tag := range []string{"NOR", "NOR Hump", "NOR Trim W", "NOR Trim E"} {
		if !p.IsYardInternalTag(tag) {
			t.Errorf("%q should be yard-internal", tag)
		}
	}
	if len(p.Aliases) != 0 {
		t.Errorf("AGS ships without aliases, got %v", p.Aliases)
	}
}

func TestBundledLocalJobs(t *testing.T) {
	p, _ := installBundled(t)

	a55 := p.LocalJobByID("a55")
	if a55 == nil {
		t.Fatal("A55 missing")
	}
	if a55.Symbol != "A55" || a55.HomeYard != "NOR" {
		t.Errorf("A55 = %+v", a55)
	}
	if !reflect.DeepEqual(a55.Industries, []string{"KYK", "ABS"}) {
		t.Errorf("A55 industries = %v", a55.Industries)
	}
	if want := []YardLine{{Yard: "TUS", Carries: []string{"A37", "A56", "AGR"}}}; !reflect.DeepEqual(a55.YardLines, want) {
		t.Errorf("A55 yard lines = %+v", a55.YardLines)
	}
	if a55.Limit.MaxLengthFt != 5000 || a55.Limit.MaxTons != 6000 {
		t.Errorf("A55 limit from TSV = %+v", a55.Limit)
	}

	if j := p.LocalJobByID("a39"); j == nil || j.HomeYard != "MCC" {
		t.Errorf("A39 = %+v", j)
	}
	a44 := p.LocalJobByID("a44")
	if a44 == nil || !reflect.DeepEqual(a44.YardLines, []YardLine{{Yard: "MCC", Carries: []string{"A39"}}}) {
		t.Errorf("A44 = %+v", a44)
	}
	if j := p.LocalJobByID("ab11"); j == nil || !reflect.DeepEqual(j.YardLines, []YardLine{{Yard: "32ND", Carries: []string{"BNSF"}}}) {
		t.Errorf("AB11 = %+v", j)
	}
	if j := p.LocalJobByID("ab45"); j == nil || !reflect.DeepEqual(j.YardLines, []YardLine{{Yard: "BES", Carries: []string{"BRL", "WWL"}}}) {
		t.Errorf("AB45 = %+v", j)
	}
	if j := p.LocalJobByID("csx-bes"); j == nil || j.Symbol != "CSXB" || j.HomeYard != "BES" {
		t.Errorf("CSXB = %+v", j)
	}
	if j := p.LocalJobByID("ab10"); j == nil || len(j.Industries) != 7 || j.Limit.MaxLengthFt != 4000 {
		t.Errorf("AB10 = %+v", j)
	}
	if n := len(p.LocalJobs); n != 10 {
		t.Errorf("%d local jobs, want 10", n)
	}
	if got := p.LocalJobsServing("BRL"); len(got) != 1 || got[0].ID != "csx-bes" {
		t.Errorf("BRL served by %v", got)
	}
	if j := p.LocalJobByID("a39"); j.Limit != (Limit{}) {
		t.Errorf("A39 has no limits row, got %+v", j.Limit)
	}
}

func TestBundledRoadTrains(t *testing.T) {
	p, _ := installBundled(t)

	t131 := p.RoadTrainBySymbol("131")
	if t131 == nil || t131.Origin != "NOR" {
		t.Fatalf("131 = %+v", t131)
	}
	if !reflect.DeepEqual(t131.ClassCodes, []string{"BRI", "NOL1"}) {
		t.Errorf("131 class codes = %v", t131.ClassCodes)
	}
	if t131.Limit.MaxLengthFt != 8000 || t131.Limit.MaxTons != 8000 || t131.MinLengthPct != 50 {
		t.Errorf("131 limit/threshold = %+v / %v", t131.Limit, t131.MinLengthPct)
	}
	wantStop := Stop{Kind: "yard", Target: "MCC", SetOut: []string{"BRI"}, PickUp: []string{"NOL", "MER"}, PickUpCarTypes: []CarType{"Autorack Car"}}
	if len(t131.Stops) != 1 || !reflect.DeepEqual(t131.Stops[0], wantStop) {
		t.Errorf("131 stops = %+v", t131.Stops)
	}
	if t339 := p.RoadTrainBySymbol("339"); t339 == nil || !reflect.DeepEqual(t339.Stops, t131.Stops) ||
		!reflect.DeepEqual(t339.ClassCodes, []string{"MER", "NOL3"}) {
		t.Errorf("339 = %+v", t339)
	}

	// 346, 28J and 29K are through trains that pick up at McCalla only.
	for sym, want := range map[string]Stop{
		"346": {Kind: "yard", Target: "MCC", PickUp: []string{"ATL"}, PickUpCarTypes: []CarType{"Autorack Car"}},
		"28J": {Kind: "yard", Target: "MCC", PickUp: []string{"ATL", "JAX"}, PickUpCarTypes: []CarType{"Well Car"}},
		"29K": {Kind: "yard", Target: "MCC", PickUp: []string{"MER"}, PickUpCarTypes: []CarType{"Well Car"}},
	} {
		rt := p.RoadTrainBySymbol(sym)
		if rt == nil {
			t.Fatalf("%s missing", sym)
		}
		if rt.Origin != "" || rt.MinLengthPct != 0 || len(rt.ClassCodes) != 0 {
			t.Errorf("%s must be a through train: %+v", sym, rt)
		}
		if len(rt.Stops) != 1 || !reflect.DeepEqual(rt.Stops[0], want) {
			t.Errorf("%s stops = %+v", sym, rt.Stops)
		}
		if rt.Limit != (Limit{}) {
			t.Errorf("%s has no limits row, got %+v", sym, rt.Limit)
		}
	}

	a28 := p.RoadTrainBySymbol("A28")
	if a28 == nil || a28.Origin != "NOR" || !reflect.DeepEqual(a28.OffMapIndustries, []string{"A28"}) ||
		!reflect.DeepEqual(a28.ClassCodes, []string{"A28"}) {
		t.Fatalf("A28 = %+v", a28)
	}
	if len(a28.Stops) != 1 || a28.Stops[0].Kind != "industry" || a28.Stops[0].Target != "PHS" {
		t.Errorf("A28 stops = %+v", a28.Stops)
	}
	if got := p.RoadTrainForOffMapIndustry("A28"); got != a28 && (got == nil || got.Symbol != "A28") {
		t.Errorf("RoadTrainForOffMapIndustry(A28) = %v", got)
	}

	// Orphan-free routing for the plain class codes.
	for tag, sym := range map[string]string{"CSX": "Y130", "NOLUP": "345", "NOL": "355", "CHA": "108", "KNX": "16T", "BEO": "178",
		"LIN": "154", "ATL": "156", "MAC": "378", "SEL": "341", "CVT": "183", "MOB": "385", "SHF": "A79", "NOL1": "131", "MER": "339"} {
		if got := p.RoadTrainForTag(tag); got == nil || got.Symbol != sym {
			t.Errorf("RoadTrainForTag(%s) = %v, want %s", tag, got, sym)
		}
	}
	if n := len(p.RoadTrains); n != 19 {
		t.Errorf("%d road trains, want 19", n)
	}
	for _, rt := range p.RoadTrains {
		if rt.MaxCarAgeDays != 0 {
			t.Errorf("%s: maxCarAgeDays should be unset", rt.Symbol)
		}
		if rt.Origin != "" && rt.MinLengthPct != 50 {
			t.Errorf("%s MinLengthPct = %v, want 50", rt.Symbol, rt.MinLengthPct)
		}
	}
}

func TestBundledValidationReport(t *testing.T) {
	p, _ := installBundled(t)

	bySubject := map[string]string{}
	for _, pr := range p.Report.Problems {
		bySubject[pr.Code+"/"+pr.Subject] = pr.Message
	}
	for _, want := range []string{
		CodeMalformedTag + "/ATL NOL3",
		CodeOrphanTag + "/CSX1",
		CodeOrphanTag + "/MERCP",
		CodeOrphanTag + "/MERUP",
		CodeOrphanTag + "/ROS",
		CodeUnservedIndustry + "/MB",
		CodeDuplicateTag + "/CSX",
	} {
		if _, ok := bySubject[want]; !ok {
			t.Errorf("report lacks %s; has %v", want, bySubject)
		}
	}
	if len(p.Report.Problems) != 7 {
		t.Errorf("%d problems, want exactly 7:\n%+v", len(p.Report.Problems), p.Report.Problems)
	}
	for k := range bySubject {
		if strings.HasSuffix(k, "/JAX") {
			t.Errorf("JAX is picked up by 28J and must not be reported: %s", k)
		}
	}
	if msg := bySubject[CodeOrphanTag+"/ROS"]; !strings.Contains(msg, "MB") {
		t.Errorf("ROS message must list its producer MB: %q", msg)
	}
	if msg := bySubject[CodeUnservedIndustry+"/MB"]; !strings.Contains(msg, "Mercedes Benz") || !strings.Contains(msg, "395") {
		t.Errorf("MB message = %q", msg)
	}
}

var modelRE = regexp.MustCompile(`<rvXMLfilename>([^<]*)</rvXMLfilename>`)

func fixtureModels(t *testing.T, name string) []string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("../../testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, m := range modelRE.FindAllSubmatch(data, -1) {
		out = append(out, strings.TrimSpace(string(m[1])))
	}
	return out
}

// TestBundledCarTypesCoverFixtureModels checks the car-type table against
// every model in the world save. Only locomotives and one maintenance car may
// stay Unknown.
func TestBundledCarTypesCoverFixtureModels(t *testing.T) {
	p, _ := installBundled(t)
	var models []string
	for _, f := range []string{"WorldSave.xml", "Auto Save World 2026.10.28 17.28.02.xml", "29K.xml"} {
		models = append(models, fixtureModels(t, f)...)
	}
	if len(models) == 0 {
		t.Fatal("no models found in fixture")
	}

	unknown := map[string]bool{}
	for _, m := range models {
		if p.CarTypeOf(m) == Unknown {
			unknown[m] = true
		}
	}
	var got []string
	for m := range unknown {
		got = append(got, m)
	}
	sort.Strings(got)
	want := []string{
		"R8_FCA_AutoFlood_III_NS.xml", "R8_GP40-2_CON01.xml", "R8_MP15_NS01.xml", "R8_SD40-2_NS01.xml", "R8_SD40T-2_UP01.xml",
		"Run8_ES44DC_BNSF02.xml", "Run8_ES44DC_NS.xml", "Run8_ES44DC_UP.xml", "Run8_ES44DC_UP01.xml",
		"Run8_SD70ACE_KCS01.xml", "Run8_SD70ACE_NS01.xml", "Run8_SD70ACE_UP02.xml",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Unknown models:\n got %q\nwant %q", got, want)
	}

	// Known families.
	for model, typ := range map[string]CarType{
		"R8_Gon_THR2743_CSX01.xml":                  "Gondola",
		"R8_Autorack_NW_ETTX1.xml":                  "Autorack Car",
		"R8_AutoRack_Tri_SP01.xml":                  "Autorack Car",
		"R8_Box_60Ft_HC_UP01.xml":                   "Box Car",
		"R8_Boxcar_FMC5347_CAGY01.xml":              "Box Car",
		"R8_Bulkhead_Thrall_TTJX01.xml":             "Bulkhead_Flat",
		"R8_Centerbeam_BC01.xml":                    "Centerbeam Flat Car",
		"R8_Coil_Evans_Cov_CR01.xml":                "CoilCar",
		"R8_C14Hopper_UP01.xml":                     "Two Bay Covered Hopper",
		"R8_CoveredHopper_Trin3281_NS.xml":          "Covered Hopper",
		"R8_PlasticPelletHopper_ACF5250_ACFX01.xml": "PlasticPellet_Hopper",
		"R8_ChipHopper_GSC7000_CSXT01.xml":          "Woodchip_Hopper",
		"R8_Hopper_H9_CSX02.xml":                    "Open Hopper",
		"Run8_Tank105_UTLX.xml":                     "Tank Car",
		"Run8_Tank107FRS01.xml":                     "Tank Car",
		"R8_T104_ADMX.xml":                          "Tank Car",
		"R8_T389_ACF_LPG_ACFX.xml":                  "Tank Car",
		"Run8_52ftwell_1JBHunt.xml":                 "Well Car",
		"Run8_WellCar53_DTTX01.xml":                 "Well Car",
		"Run8_WellCar53AP_FEC02.xml":                "Well Car",
	} {
		if got := p.CarTypeOf(model); got != typ {
			t.Errorf("CarTypeOf(%s) = %q, want %q", model, got, typ)
		}
	}

	// Every car type an industry takes can be produced by some rule, and
	// every rule type has defaults.
	for _, pr := range p.Report.Problems {
		if pr.Code == CodeUnmappedCarType {
			t.Errorf("industry car type without a rule: %+v", pr)
		}
	}
	for _, r := range p.CarTypes {
		d := p.DefaultsFor(r.Type)
		if _, ok := p.CarDefaults[r.Type]; !ok || d.LengthFt <= 0 || d.TareTons <= 0 {
			t.Errorf("no defaults for %s: %+v", r.Type, d)
		}
	}
	if p.UnknownDefaults.LengthFt <= 0 || p.UnknownDefaults.TareTons <= 0 {
		t.Errorf("UnknownDefaults = %+v", p.UnknownDefaults)
	}

	// The profile reports unmatched models on request.
	probs := p.UnmatchedModels(models)
	if len(probs) != len(want) {
		t.Errorf("UnmatchedModels = %d problems, want %d", len(probs), len(want))
	}
}
