package profile

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// loadMini loads the mini profile with extra yaml appended.
func loadMini(t *testing.T, extra string) *Profile {
	t.Helper()
	p, err := Load(writeProfile(t, miniYAML+extra, miniCSV, miniTSV))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return p
}

// problemsOf returns the report problems with the given code.
func problemsOf(p *Profile, code string) []Problem {
	var out []Problem
	for _, pr := range p.Report.Problems {
		if pr.Code == code {
			out = append(out, pr)
		}
	}
	return out
}

func TestRun8AbsentIsNil(t *testing.T) {
	p := loadMini(t, "")
	if p.Run8 != nil {
		t.Errorf("Run8 = %+v, want nil", p.Run8)
	}
	if p.SchemaVersion != 1 {
		t.Errorf("SchemaVersion = %d, want 1 when absent", p.SchemaVersion)
	}
	if len(problemsOf(p, CodeInvalidRun8)) != 0 {
		t.Errorf("unexpected problems: %+v", p.Report.Problems)
	}
}

func TestRun8ValidBlock(t *testing.T) {
	p := loadMini(t, `
run8:
  region: Coast
  routes:
    - {folder: Route_A, prefix: 10}
    - {folder: Route_B, prefix: 11}
  humps:
    - {controller: Hump Controller, yard: HOM, trackPrefix: Bowl}
  trackEnds:
    - pattern: '(?i)^RCV (?P<n>\d+) (?P<end>North|South) end$'
      yard: SAT
      trackPrefix: Receiving
`)
	if got := problemsOf(p, CodeInvalidRun8); len(got) != 0 {
		t.Fatalf("problems: %+v", got)
	}
	r := p.Run8
	if r == nil || r.Region != "Coast" {
		t.Fatalf("Run8 = %+v", r)
	}
	if want := []Run8Route{{"Route_A", 10}, {"Route_B", 11}}; !reflect.DeepEqual(r.Routes, want) {
		t.Errorf("Routes = %+v", r.Routes)
	}
	if want := []Run8Hump{{"Hump Controller", "HOM", "Bowl"}}; !reflect.DeepEqual(r.Humps, want) {
		t.Errorf("Humps = %+v", r.Humps)
	}
	if len(r.TrackEnds) != 1 {
		t.Fatalf("TrackEnds = %+v", r.TrackEnds)
	}
	te := r.TrackEnds[0]
	if te.Yard != "SAT" || te.TrackPrefix != "Receiving" {
		t.Errorf("TrackEnds[0] = %+v", te)
	}
	m := te.Pattern.FindStringSubmatch("RCV 3 South end")
	if m == nil || m[te.Pattern.SubexpIndex("n")] != "3" || m[te.Pattern.SubexpIndex("end")] != "South" {
		t.Errorf("pattern match = %v", m)
	}
}

func TestRun8EscapingRegionIsDroppedAndFlagged(t *testing.T) {
	for _, region := range []string{`..\..\Windows`, "../etc", "", `C:\Windows`, "/abs", "a/b", "..", "Coast:", "."} {
		p := loadMini(t, "\nrun8:\n  region: '"+region+"'\n  routes:\n    - {folder: Route_A, prefix: 10}\n")
		if p.Run8 != nil {
			t.Errorf("region %q: Run8 = %+v, want nil", region, p.Run8)
		}
		got := problemsOf(p, CodeInvalidRun8)
		if len(got) != 1 || !strings.Contains(got[0].Message, "region") {
			t.Errorf("region %q: problems = %+v", region, p.Report.Problems)
		}
	}
}

func TestRun8BadRouteEntriesAreDropped(t *testing.T) {
	p := loadMini(t, `
run8:
  region: Coast
  routes:
    - {folder: Good, prefix: 10}
    - {folder: '..\Evil', prefix: 10}
    - {folder: '', prefix: 10}
    - {folder: 'a/b', prefix: 10}
    - {folder: 'C:\Run8', prefix: 10}
    - {folder: 'a..b', prefix: 10}
    - {folder: NoPrefix, prefix: 99}
`)
	if p.Run8 == nil {
		t.Fatal("Run8 = nil")
	}
	if want := []Run8Route{{"Good", 10}}; !reflect.DeepEqual(p.Run8.Routes, want) {
		t.Errorf("Routes = %+v", p.Run8.Routes)
	}
	if got := problemsOf(p, CodeInvalidRun8); len(got) != 6 {
		t.Errorf("%d problems, want 6: %+v", len(got), got)
	}
	var sawPrefix bool
	for _, pr := range problemsOf(p, CodeInvalidRun8) {
		if strings.Contains(pr.Message, "routePrefixes") && strings.Contains(pr.Message, "99") {
			sawPrefix = true
		}
	}
	if !sawPrefix {
		t.Errorf("no problem for prefix 99 missing from routePrefixes: %+v", p.Report.Problems)
	}
}

func TestRun8UnknownYardInMappings(t *testing.T) {
	p := loadMini(t, `
run8:
  region: Coast
  humps:
    - {controller: Hump A, yard: XYZ, trackPrefix: Bowl}
    - {controller: Hump B, yard: HOM, trackPrefix: Bowl}
  trackEnds:
    - {pattern: '^R (?P<n>\d+) (?P<end>N|S)$', yard: XYZ, trackPrefix: Rcv}
`)
	if want := []Run8Hump{{"Hump B", "HOM", "Bowl"}}; !reflect.DeepEqual(p.Run8.Humps, want) {
		t.Errorf("Humps = %+v", p.Run8.Humps)
	}
	if len(p.Run8.TrackEnds) != 0 {
		t.Errorf("TrackEnds = %+v", p.Run8.TrackEnds)
	}
	refs := problemsOf(p, CodeUnknownRef)
	if len(refs) != 2 {
		t.Fatalf("unknown-ref problems = %+v", refs)
	}
	for _, r := range refs {
		if r.Subject != "XYZ" {
			t.Errorf("subject = %q", r.Subject)
		}
	}
}

func TestRun8BadTrackEndsPatterns(t *testing.T) {
	p := loadMini(t, `
run8:
  region: Coast
  trackEnds:
    - {pattern: '([unclosed', yard: HOM, trackPrefix: A}
    - {pattern: '^R (\d+) (N|S)$', yard: HOM, trackPrefix: B}
    - {pattern: '^R (?P<n>\d+)$', yard: HOM, trackPrefix: C}
    - {pattern: '^R (?P<end>N|S)$', yard: HOM, trackPrefix: D}
    - {pattern: '^R (?P<n>\d+) (?P<end>N|S)$', yard: HOM, trackPrefix: E}
`)
	if len(p.Run8.TrackEnds) != 1 || p.Run8.TrackEnds[0].TrackPrefix != "E" {
		t.Errorf("TrackEnds = %+v", p.Run8.TrackEnds)
	}
	if got := problemsOf(p, CodeInvalidRun8); len(got) != 4 {
		t.Errorf("%d problems, want 4: %+v", len(got), got)
	}
}

func TestAnchorIndustriesMustBeProfileIndustries(t *testing.T) {
	p, err := Load(writeProfile(t, strings.Replace(miniYAML,
		"  - {code: HOM, name: Home}",
		"  - {code: HOM, name: Home, anchorIndustries: [ALP,  NOPE], anchorMileposts: ['Home   Yard']}", 1),
		miniCSV, miniTSV))
	if err != nil {
		t.Fatal(err)
	}
	y := p.YardByCode("HOM")
	if !reflect.DeepEqual(y.AnchorIndustries, []string{"ALP", "NOPE"}) {
		t.Errorf("AnchorIndustries = %v", y.AnchorIndustries)
	}
	if !reflect.DeepEqual(y.AnchorMileposts, []string{"Home Yard"}) {
		t.Errorf("AnchorMileposts = %v (must be whitespace normalised)", y.AnchorMileposts)
	}
	refs := problemsOf(p, CodeUnknownRef)
	var found bool
	for _, r := range refs {
		if r.Subject == "NOPE" && strings.Contains(r.Message, "anchor industry") {
			found = true
		}
		if r.Subject == "ALP" {
			t.Errorf("valid anchor industry flagged: %+v", r)
		}
	}
	if !found {
		t.Errorf("no problem for NOPE: %+v", p.Report.Problems)
	}
}

func TestValidateAnchors(t *testing.T) {
	p, err := Load(writeProfile(t, strings.Replace(miniYAML,
		"  - {code: HOM, name: Home}",
		"  - {code: HOM, name: Home, anchorMileposts: [Home Yard, Tuscalosa MP 199.1]}", 1),
		miniCSV, miniTSV))
	if err != nil {
		t.Fatal(err)
	}
	got := p.ValidateAnchors([]string{"Home  Yard", "Tuscaloosa MP 199.1"})
	if len(got) != 1 || got[0].Code != CodeUnmatchedAnchor || got[0].Subject != "Tuscalosa MP 199.1" || !strings.Contains(got[0].Message, "HOM") {
		t.Errorf("problems = %+v", got)
	}
	if got := p.ValidateAnchors([]string{"Home Yard", "Tuscalosa MP 199.1"}); len(got) != 0 {
		t.Errorf("all anchors match, got %+v", got)
	}
	if got := p.ValidateAnchors(nil); len(got) != 2 {
		t.Errorf("no mileposts: got %+v, want both anchors flagged", got)
	}
}

func TestSchemaVersion(t *testing.T) {
	for _, tc := range []struct {
		yaml string
		want int
	}{
		{"", 1},
		{"schemaVersion: 1\n", 1},
		{"schemaVersion: 2\n", 2},
	} {
		p, err := Load(writeProfile(t, tc.yaml+miniYAML, miniCSV, miniTSV))
		if err != nil {
			t.Fatalf("%q: %v", tc.yaml, err)
		}
		if p.SchemaVersion != tc.want {
			t.Errorf("%q: SchemaVersion = %d, want %d", tc.yaml, p.SchemaVersion, tc.want)
		}
	}
}

func TestSchemaVersionTooNewIsLoadError(t *testing.T) {
	_, err := Load(writeProfile(t, "schemaVersion: 7\n"+miniYAML, miniCSV, miniTSV))
	if err == nil {
		t.Fatal("want an error for a newer schemaVersion")
	}
	if !strings.Contains(err.Error(), "schemaVersion 7") || !strings.Contains(err.Error(), fmt.Sprintf("up to %d", BundledSchemaVersion)) {
		t.Errorf("error must name both numbers: %v", err)
	}
}
