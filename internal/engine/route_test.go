package engine

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"run8-ops-center/internal/carstatus"
	"run8-ops-center/internal/profile"
	"run8-ops-center/internal/run8route"
	"run8-ops-center/internal/run8route/run8routetest"
	"run8-ops-center/internal/run8save"
	"run8-ops-center/internal/store"
	"run8-ops-center/internal/trackmap"
)

// startWithInstall writes a synthetic Run8 install, names it as the install
// folder in settings.json and starts an engine over a fresh data directory.
func startWithInstall(t *testing.T, spec run8routetest.InstallSpec) (*harness, string) {
	t.Helper()
	root := t.TempDir()
	run8routetest.WriteInstall(t, root, spec)
	h := &harness{t: t, dir: t.TempDir(), clock: newClock(), guard: store.NewGuard()}
	data, err := json.Marshal(map[string]string{"run8InstallDir": root})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(h.dir, store.SettingsFile), data, 0o644); err != nil {
		t.Fatal(err)
	}
	h.open()
	return h, root
}

// cutSnapshot is a snapshot with one unpowered train and one car on each
// given section of route prefix 280.
func cutSnapshot(trainID, tag string, sections ...int) *run8save.Snapshot {
	var vs []run8save.Vehicle
	for i, s := range sections {
		vs = append(vs, run8save.Vehicle{
			Model: "R8_Boxcar_X.xml", UnitType: "US_Freightcar", UnitNumber: string(rune('A' + i)), Tag: tag,
			Trucks: []run8save.Truck{{RoutePrefix: run8routetest.Prefix, Section: s}, {RoutePrefix: run8routetest.Prefix, Section: s}},
		})
	}
	return &run8save.Snapshot{Trains: []run8save.Train{{ID: trainID, Vehicles: vs}}}
}

func run8Warnings(v *View) []Warning {
	var out []Warning
	for _, w := range v.Warnings {
		if w.Kind == warnRun8 {
			out = append(out, w)
		}
	}
	return out
}

func problemFor(p *profile.Profile, code, subject string) bool {
	for _, pr := range p.Report.Problems {
		if pr.Code == code && pr.Subject == subject {
			return true
		}
	}
	return false
}

func fileStatus(v *View, name string) (run8route.FileStatus, bool) {
	for _, f := range v.Run8.Unavailable {
		if f.Name == name {
			return f, true
		}
	}
	return run8route.FileStatus{}, false
}

// touchLater moves a file's modification time forward so a rewrite is seen
// even on a file system with coarse timestamps.
func touchLater(t *testing.T, path string) {
	t.Helper()
	later := time.Now().Add(2 * time.Hour)
	if err := os.Chtimes(path, later, later); err != nil {
		t.Fatal(err)
	}
}

func TestRouteDataLoadedAtStart(t *testing.T) {
	h, _ := startWithInstall(t, run8routetest.Synthetic())
	v := h.eng.View()
	if !v.Run8.HasRun8Config || v.Run8.Status != Run8Ready || len(v.Run8.Unavailable) != 0 || v.Run8.Reason != "" {
		t.Fatalf("Run8 = %+v", v.Run8)
	}
	if w := run8Warnings(v); len(w) != 0 {
		t.Fatalf("warnings = %+v", w)
	}
	// The loaded topology reaches learn mode: a cut on the middle of a
	// receiving track labels the whole track.
	v = h.ingest(snapshotEvent(cutSnapshot("T1", "NOR", run8routetest.SecRcv1Mid), "WorldSave.xml", false))
	if len(v.Candidates) != 1 || len(v.Candidates[0].Track) != 3 {
		t.Fatalf("candidates = %+v", v.Candidates)
	}
}

func TestRouteDataReloadedAfterSetInstallDir(t *testing.T) {
	h := start(t) // the default install folder does not exist here
	v := h.eng.View()
	if v.Run8.Status != Run8Unavailable || len(v.Run8.Unavailable) != 5 || v.Run8.ImportAvailable || !v.Run8.HasRun8Config {
		t.Fatalf("Run8 before = %+v", v.Run8)
	}
	warns := run8Warnings(v)
	if len(warns) != 5 {
		t.Fatalf("warnings = %+v", warns)
	}
	want := run8routetest.RoutePath(store.DefaultInstallDir, run8routetest.Route, run8route.TrackDBFile)
	f, ok := fileStatus(v, run8route.TrackDBFile)
	if !ok || !errors.Is(f.Err, fs.ErrNotExist) {
		t.Fatalf("track database status = %+v, want a not-exist error", f)
	}
	found := false
	for _, w := range warns {
		if strings.Contains(w.Message, want) && strings.Contains(w.Message, reason(f.Err)) {
			found = true
		}
	}
	if !found {
		t.Errorf("no warning names %s and the reason %q: %+v", want, reason(f.Err), warns)
	}

	root := t.TempDir()
	run8routetest.WriteInstall(t, root, run8routetest.Synthetic())
	if err := h.eng.SetInstallDir(root); err != nil {
		t.Fatal(err)
	}
	v = h.eng.View()
	if v.Run8.Status != Run8Ready || !v.Run8.ImportAvailable || len(run8Warnings(v)) != 0 {
		t.Fatalf("Run8 after = %+v, warnings %+v", v.Run8, run8Warnings(v))
	}
}

func TestMissingRouteFileIsAWarning(t *testing.T) {
	h, root := startWithInstall(t, run8routetest.Synthetic())
	track := run8routetest.RoutePath(root, run8routetest.Route, run8route.TrackDBFile)
	if err := os.Remove(track); err != nil {
		t.Fatal(err)
	}
	if err := h.eng.SetInstallDir(root); err != nil {
		t.Fatal(err)
	}
	v := h.eng.View()
	if v.Run8.Status != Run8Partial || !v.Run8.ImportAvailable {
		t.Fatalf("Run8 = %+v", v.Run8)
	}
	f, ok := fileStatus(v, run8route.TrackDBFile)
	if !ok || f.Path != track || len(v.Run8.Unavailable) != 1 || !errors.Is(f.Err, fs.ErrNotExist) {
		t.Fatalf("unavailable = %+v", v.Run8.Unavailable)
	}
	w := run8Warnings(v)
	if len(w) != 1 || !strings.Contains(w[0].Message, track) || !strings.Contains(w[0].Message, reason(f.Err)) {
		t.Fatalf("warnings = %+v", w)
	}
	// The other files still work: the import offers the industry labels.
	p, err := h.eng.PreviewImport()
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Diff.New) == 0 || len(p.Unusable) != 1 || p.Unusable[0].Name != run8route.TrackDBFile {
		t.Fatalf("preview: %d new, unusable %+v", len(p.Diff.New), p.Unusable)
	}
}

func TestMisspelledAnchorMilepostIsInTheReport(t *testing.T) {
	good, _ := startWithInstall(t, run8routetest.Synthetic())
	if problemFor(good.eng.View().Profile, profile.CodeUnmatchedAnchor, "Receiving Yard") {
		t.Fatalf("the matching anchor is reported: %+v", good.eng.View().Profile.Report.Problems)
	}

	spec := run8routetest.Synthetic()
	spec.Mileposts[0] = run8routetest.MilepostAt("MP 1.0", "Recieving Yard", 1000, -1250, run8routetest.TileW, run8routetest.TileH)
	h, _ := startWithInstall(t, spec)
	v := h.eng.View()
	if !problemFor(v.Profile, profile.CodeUnmatchedAnchor, "Receiving Yard") {
		t.Fatalf("misspelled anchor not in the report: %+v", v.Profile.Report.Problems)
	}
	// The loaded profile's own report is not touched.
	base := len(h.eng.base.Report.Problems)
	if len(v.Profile.Report.Problems) <= base {
		t.Errorf("effective report has %d problems, base %d", len(v.Profile.Report.Problems), base)
	}
}

func TestPreviewLeavesTheTrackMapAlone(t *testing.T) {
	h, _ := startWithInstall(t, run8routetest.Synthetic())
	path := filepath.Join(h.profileDir(), trackMapFile)
	before := h.eng.View()
	p, err := h.eng.PreviewImport()
	if err != nil {
		t.Fatal(err)
	}
	if p.Token == "" || len(p.Unusable) != 0 {
		t.Fatalf("preview = %+v", p)
	}
	byKey := map[string]trackmap.DiffEntry{}
	for _, e := range p.Diff.New {
		byKey[e.Key] = e
	}
	if e, ok := byKey[trackmap.Key(run8routetest.Prefix, run8routetest.SecSiding1)]; !ok || e.New.Kind != trackmap.KindIndustry || e.New.Industries[0] != "BRL" {
		t.Errorf("siding entry = %+v, %v", e, ok)
	}
	if e, ok := byKey[trackmap.Key(run8routetest.Prefix, run8routetest.SecBowl1)]; !ok || e.New.Yard != "NOR" || e.New.Track != "Bowl 1" {
		t.Errorf("bowl entry = %+v, %v", e, ok)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("trackmap.json exists after a preview: %v", err)
	}
	after := h.eng.View()
	if len(after.TrackMap.Labels) != 0 || after.TrackMap != before.TrackMap || after.Run8.LastImport != (time.Time{}) || !after.Run8.ImportAvailable {
		t.Errorf("preview changed the map: %d labels, Run8 %+v", len(after.TrackMap.Labels), after.Run8)
	}
}

func TestConfirmImportWritesAndRecomputes(t *testing.T) {
	h, _ := startWithInstall(t, run8routetest.Synthetic())
	// One car on the BRL siding and one on a section no label will cover.
	snap := cutSnapshot("T1", "BRL", run8routetest.SecSiding1, 99999)
	v := h.ingest(snapshotEvent(snap, "WorldSave.xml", false))
	if v.Coverage.Cars != 2 || v.Coverage.Unmapped != 2 {
		t.Fatalf("coverage before = %+v", v.Coverage)
	}
	p, err := h.eng.PreviewImport()
	if err != nil {
		t.Fatal(err)
	}
	if err := h.eng.ConfirmImport(p.Token); err != nil {
		t.Fatal(err)
	}
	v = h.eng.View()
	if v.Coverage.Unmapped != 1 {
		t.Errorf("coverage after = %+v", v.Coverage)
	}
	if len(v.TrackMap.Labels) != len(p.Diff.New) || v.TrackMap.Run8Import == nil {
		t.Errorf("%d labels, %d new, meta %+v", len(v.TrackMap.Labels), len(p.Diff.New), v.TrackMap.Run8Import)
	}
	if l, _ := v.TrackMap.Lookup(run8routetest.Prefix, run8routetest.SecSiding1); l.Source != trackmap.SourceRun8 {
		t.Errorf("imported label = %+v", l)
	}
	if !v.Run8.LastImport.Equal(h.clock.Now()) || v.Run8.ImportAvailable || v.Run8.Changed {
		t.Errorf("Run8 = %+v", v.Run8)
	}
	saved, err := trackmap.Load(filepath.Join(h.profileDir(), trackMapFile))
	if err != nil {
		t.Fatal(err)
	}
	if len(saved.Labels) != len(v.TrackMap.Labels) || saved.Run8Import == nil || saved.Revision() != v.TrackMap.Revision() {
		t.Errorf("saved map differs: %d labels, meta %+v", len(saved.Labels), saved.Run8Import)
	}
	var logged bool
	for _, a := range v.Activity {
		if a.Kind == "import" {
			logged = true
		}
	}
	if !logged {
		t.Errorf("no activity entry for the import: %+v", v.Activity)
	}
	// The old token is stale now: the map revision moved.
	if err := h.eng.ConfirmImport(p.Token); !errors.Is(err, ErrStaleImport) {
		t.Errorf("second confirm with the old token = %v", err)
	}
}

func TestConfirmRefusedWhenConfigIndChanges(t *testing.T) {
	h, root := startWithInstall(t, run8routetest.Synthetic())
	path := filepath.Join(h.profileDir(), trackMapFile)
	p, err := h.eng.PreviewImport()
	if err != nil {
		t.Fatal(err)
	}
	spec := run8routetest.Synthetic()
	cfg := run8routetest.ConfigPath(root, run8routetest.Region)
	if err := os.WriteFile(cfg, run8routetest.EncodeConfigInd(spec.Industries[1:]), 0o644); err != nil {
		t.Fatal(err)
	}
	touchLater(t, cfg)

	if err := h.eng.ConfirmImport(p.Token); !errors.Is(err, ErrStaleImport) {
		t.Fatalf("confirm after rewriting Config.ind = %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) || len(h.eng.View().TrackMap.Labels) != 0 {
		t.Fatalf("a refused confirm wrote labels: %v", err)
	}
	// A fresh preview of the new files confirms.
	p2, err := h.eng.PreviewImport()
	if err != nil {
		t.Fatal(err)
	}
	if p2.Token == p.Token {
		t.Fatal("the token did not change with the file")
	}
	if err := h.eng.ConfirmImport(p2.Token); err != nil {
		t.Fatal(err)
	}
	if len(h.eng.View().TrackMap.Labels) == 0 {
		t.Error("nothing imported")
	}
}

func TestConfirmRefusedAfterLabelEdit(t *testing.T) {
	h, _ := startWithInstall(t, run8routetest.Synthetic())
	p, err := h.eng.PreviewImport()
	if err != nil {
		t.Fatal(err)
	}
	if err := h.eng.LabelSection(run8routetest.Prefix, 99999, trackmap.Label{Kind: trackmap.KindIgnored}); err != nil {
		t.Fatal(err)
	}
	if err := h.eng.ConfirmImport(p.Token); !errors.Is(err, ErrStaleImport) {
		t.Fatalf("confirm after a label edit = %v", err)
	}
	if n := len(h.eng.View().TrackMap.Labels); n != 1 {
		t.Errorf("labels = %d, want only the manual one", n)
	}
	if err := h.eng.ConfirmImport(""); !errors.Is(err, ErrStaleImport) {
		t.Errorf("empty token = %v", err)
	}
}

func TestImportKeepsManualLabelsAndLabelEditsStayManual(t *testing.T) {
	h, _ := startWithInstall(t, run8routetest.Synthetic())
	manual := trackmap.Label{Kind: trackmap.KindYard, Yard: "NOR", Track: "Track 9"}
	if err := h.eng.LabelSection(run8routetest.Prefix, run8routetest.SecSiding1, manual); err != nil {
		t.Fatal(err)
	}
	p, err := h.eng.PreviewImport()
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Diff.KeptManual) != 1 {
		t.Fatalf("kept manual = %+v", p.Diff.KeptManual)
	}
	if err := h.eng.ConfirmImport(p.Token); err != nil {
		t.Fatal(err)
	}
	l, _ := h.eng.View().TrackMap.Lookup(run8routetest.Prefix, run8routetest.SecSiding1)
	if l.Track != "Track 9" || l.Source != "" {
		t.Errorf("manual label = %+v", l)
	}
}

func TestPreviewWithoutRun8Folders(t *testing.T) {
	h := start(t)
	h.installCopy()
	// A copy of the profile without its run8 block.
	path := filepath.Join(h.dir, profilesDirName, otherID, profile.ProfileFile)
	text := string(readFile(t, path))
	cut := strings.Index(text, "# Run8 route data the app can import")
	if cut < 0 {
		t.Fatal("cannot find the run8 block in the bundled profile")
	}
	if err := h.guard.WriteFile(path, []byte(text[:cut]), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := h.eng.SwitchProfile(otherID); err != nil {
		t.Fatal(err)
	}
	v := h.eng.View()
	if v.Run8.HasRun8Config || v.Run8.Status != Run8Unavailable || v.Run8.Reason != "profile names no Run8 folders" ||
		v.Run8.ImportAvailable || v.Run8.Changed || len(run8Warnings(v)) != 0 {
		t.Fatalf("Run8 = %+v", v.Run8)
	}
	if _, err := h.eng.PreviewImport(); err == nil || !strings.Contains(err.Error(), "profile names no Run8 folders") {
		t.Errorf("PreviewImport = %v", err)
	}
	if err := h.eng.ConfirmImport("x"); err == nil {
		t.Error("ConfirmImport succeeded without Run8 folders")
	}
}

func TestRun8StateFollowsTheFiles(t *testing.T) {
	h, root := startWithInstall(t, run8routetest.Synthetic())
	v := h.ingest(snapshotEvent(cutSnapshot("T1", "BRL", 99999), "WorldSave.xml", false))
	if !v.Run8.ImportAvailable || v.Run8.Changed || !v.Run8.LastImport.IsZero() {
		t.Fatalf("before any import: %+v", v.Run8)
	}

	p, err := h.eng.PreviewImport()
	if err != nil {
		t.Fatal(err)
	}
	if err := h.eng.ConfirmImport(p.Token); err != nil {
		t.Fatal(err)
	}
	v = h.ingest(snapshotEvent(cutSnapshot("T1", "BRL", 99998), "WorldSave.xml", false))
	if v.Run8.Changed || v.Run8.ImportAvailable || v.Run8.Status != Run8Ready {
		t.Fatalf("after the import: %+v", v.Run8)
	}

	// Run8 or the user replaces Config.ind: the next ingest notices without a reload.
	spec := run8routetest.Synthetic()
	cfg := run8routetest.ConfigPath(root, run8routetest.Region)
	if err := os.WriteFile(cfg, run8routetest.EncodeConfigInd(spec.Industries[1:]), 0o644); err != nil {
		t.Fatal(err)
	}
	touchLater(t, cfg)
	v = h.ingest(snapshotEvent(cutSnapshot("T1", "BRL", 99997), "WorldSave.xml", false))
	if !v.Run8.Changed || v.Run8.ImportAvailable || v.Run8.LastImport.IsZero() {
		t.Fatalf("after rewriting Config.ind: %+v", v.Run8)
	}

	// A new import brings the flag down.
	p, err = h.eng.PreviewImport()
	if err != nil {
		t.Fatal(err)
	}
	if err := h.eng.ConfirmImport(p.Token); err != nil {
		t.Fatal(err)
	}
	if v := h.eng.View(); v.Run8.Changed {
		t.Errorf("still changed after re-import: %+v", v.Run8)
	}

	// The record survives a restart.
	h.open()
	if v := h.eng.View(); v.Run8.Changed || v.Run8.ImportAvailable || v.Run8.LastImport.IsZero() {
		t.Errorf("after restart: %+v", v.Run8)
	}
}

// yardTracksRE matches the `tracks:` block of a bundled yard, which a version-2
// profile does not have. receivingNamesRE matches the `names:` line of the
// receiving rule, which a profile before version 4 does not have.
var (
	yardTracksRE     = regexp.MustCompile(`(?m)^    tracks:\n(?:      .*\n)+`)
	receivingNamesRE = regexp.MustCompile(`(?m)^      names: .*\n`)
)

// TestStartupUpgradesAnOldProfile is the task 5.4 check: a data directory with
// a version-2 or version-3 AGS profile and a track map starts with the current
// profile active, the old files kept as .v<old>.bak and the labels intact.
func TestStartupUpgradesAnOldProfile(t *testing.T) {
	for _, old := range []int{2, 3} {
		t.Run(fmt.Sprintf("v%d", old), func(t *testing.T) { upgradesAnOldProfile(t, old) })
	}
}

func upgradesAnOldProfile(t *testing.T, version int) {
	dir := t.TempDir()
	guard := store.NewGuard()
	profiles := filepath.Join(dir, profilesDirName)
	if err := profile.InstallBundled(guard, profiles); err != nil {
		t.Fatal(err)
	}
	pdir := filepath.Join(profiles, DefaultProfileID)
	bundled := string(readFile(t, filepath.Join(pdir, profile.ProfileFile)))
	body := receivingNamesRE.ReplaceAllString(bundled, "")
	if body == bundled {
		t.Fatal("cannot build a version 3 profile from the bundled one")
	}
	if version == 2 {
		body = yardTracksRE.ReplaceAllString(body, "")
		if strings.Contains(body, "tracks:\n") {
			t.Fatal("cannot build a version 2 profile from the bundled one")
		}
	}
	current := fmt.Sprintf("schemaVersion: %d\n", profile.BundledSchemaVersion)
	if !strings.Contains(body, current) {
		t.Fatalf("the bundled profile lacks %q", current)
	}
	old := map[string]string{
		profile.ProfileFile:  strings.Replace(body, current, fmt.Sprintf("schemaVersion: %d\n", version), 1),
		profile.IndustryFile: string(readFile(t, filepath.Join(pdir, profile.IndustryFile))) + "# user edit\n",
		profile.LimitsFile:   string(readFile(t, filepath.Join(pdir, profile.LimitsFile))) + "# user edit\n",
	}
	for name, content := range old {
		if err := os.WriteFile(filepath.Join(pdir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	tm := trackmap.New()
	tm.Set(280, 4535, trackmap.Label{Kind: trackmap.KindIndustry, Industries: []string{"GES"}})
	tm.Set(280, 5101, trackmap.Label{Kind: trackmap.KindYard, Yard: "NOR", Track: "Bowl 14"})
	mapPath := filepath.Join(pdir, trackMapFile)
	if err := tm.Save(guard, mapPath); err != nil {
		t.Fatal(err)
	}
	mapBytes := readFile(t, mapPath)

	h := &harness{t: t, dir: dir, clock: newClock(), guard: guard}
	h.open()
	v := h.eng.View()
	if v.Profile.ID != DefaultProfileID || v.Profile.SchemaVersion != profile.BundledSchemaVersion || v.Profile.Run8 == nil {
		t.Fatalf("profile = %s version %d run8 %v", v.Profile.ID, v.Profile.SchemaVersion, v.Profile.Run8)
	}
	if names := v.Profile.Run8.TrackEnds[0].Names; len(names) != 14 {
		t.Errorf("the upgraded profile has %d receiving names, want 14", len(names))
	}
	bak := fmt.Sprintf(".v%d.bak", version)
	for name, content := range old {
		if got := string(readFile(t, filepath.Join(pdir, name+bak))); got != content {
			t.Errorf("%s%s = %.60q, want the old file", name, bak, got)
		}
		if got := string(readFile(t, filepath.Join(pdir, name))); got == content {
			t.Errorf("%s was not replaced", name)
		}
	}
	if string(readFile(t, mapPath)) != string(mapBytes) {
		t.Error("trackmap.json changed")
	}
	if len(v.TrackMap.Labels) != 2 {
		t.Errorf("labels = %+v", v.TrackMap.Labels)
	}
	if l, _ := v.TrackMap.Lookup(280, 5101); l.Track != "Bowl 14" || l.Source != "" {
		t.Errorf("280/5101 = %+v", l)
	}
}

func TestViewMeasuresYardLocationsFromRouteData(t *testing.T) {
	h, _ := startWithInstall(t, run8routetest.Synthetic())
	if v := h.eng.View(); len(v.LocationFt) != 0 {
		t.Fatalf("no labels yet, but LocationFt = %v", v.LocationFt)
	}
	p, err := h.eng.PreviewImport()
	if err != nil {
		t.Fatal(err)
	}
	if err := h.eng.ConfirmImport(p.Token); err != nil {
		t.Fatal(err)
	}
	topo := h.eng.routeData().Topology
	feet := func(sections ...int) int {
		m := 0.0
		for _, s := range sections {
			sec, ok := topo.Section(s)
			if !ok {
				t.Fatalf("no section %d", s)
			}
			m += sec.LengthM
		}
		return int(math.Round(m * 3.28084))
	}
	key := func(track string) string {
		return trackmap.Location{Kind: trackmap.KindYard, Yard: "NOR", Track: track}.Key()
	}
	v := h.eng.View()
	// RCV 1 is sections 300, 301 and 302; the bundled profile names it "Receiving East Thoroughfare".
	if got, want := v.LocationFt[key("Receiving East Thoroughfare")], feet(300, 301, 302); got != want || want < 500 {
		t.Errorf("Receiving East Thoroughfare measures %d ft, want %d", got, want)
	}
	for k := range v.LocationFt {
		if !strings.HasPrefix(k, "yard|") {
			t.Errorf("LocationFt holds %q, which is not a yard location", k)
		}
	}

	// A manual label on one section measures that section alone, and the view follows at once.
	manual := trackmap.Label{Kind: trackmap.KindYard, Yard: "NOR", Track: "Track 9"}
	if err := h.eng.LabelSection(run8routetest.Prefix, run8routetest.SecSiding1, manual); err != nil {
		t.Fatal(err)
	}
	if got, want := h.eng.View().LocationFt[key("Track 9")], feet(run8routetest.SecSiding1); got != want || want == 0 {
		t.Errorf("Track 9 measures %d ft, want %d", got, want)
	}

	// Without Run8 folders there is no route data and so no measured length.
	bare := start(t)
	if err := bare.eng.LabelSection(run8routetest.Prefix, run8routetest.SecSiding1, manual); err != nil {
		t.Fatal(err)
	}
	if v := bare.eng.View(); v.LocationFt != nil {
		t.Errorf("no install: LocationFt = %v", v.LocationFt)
	}
}

func TestPreviewMeasuresLocationsAndFindsCars(t *testing.T) {
	h, _ := startWithInstall(t, run8routetest.Synthetic())
	const prefix = run8routetest.Prefix
	truck := func(section int) run8save.Truck { return run8save.Truck{RoutePrefix: prefix, Section: section} }
	boxcar := func(unit string, sections ...int) run8save.Vehicle {
		v := run8save.Vehicle{Model: "R8_Boxcar_X.xml", UnitType: "US_Freightcar", UnitNumber: unit}
		for _, s := range sections {
			v.Trucks = append(v.Trucks, truck(s))
		}
		return v
	}
	number := func(unit string) string { return carstatus.Number("R8_Boxcar_X.xml", unit) }

	// A PreviewImport before any save knows no cars.
	p, err := h.eng.PreviewImport()
	if err != nil {
		t.Fatal(err)
	}
	if p.CarsKnown {
		t.Error("CarsKnown before a save was loaded")
	}

	vs := []run8save.Vehicle{
		boxcar("B1", run8routetest.SecBowl1, run8routetest.SecBowl1),
		boxcar("B2", 99999, run8routetest.SecBowl1), // lead truck off the listed sections, trailing truck on Bowl 1
		boxcar("S1", run8routetest.SecSiding1, run8routetest.SecSiding2), // one car across two sections of BRL
	}
	loco := boxcar("L1", run8routetest.SecBowl1, run8routetest.SecBowl1)
	loco.UnitType = "US_DieselEngine"
	vs = append(vs, loco)
	for i := range 6 { // more cars on the siding than the sample holds
		vs = append(vs, boxcar(string(rune('c'+i)), run8routetest.SecSiding2))
	}
	h.ingest(snapshotEvent(&run8save.Snapshot{Trains: []run8save.Train{{ID: "T1", Vehicles: vs}}}, "WorldSave.xml", false))

	p, err = h.eng.PreviewImport()
	if err != nil {
		t.Fatal(err)
	}
	if !p.CarsKnown {
		t.Error("CarsKnown with a save loaded")
	}
	feet := func(sections ...int) int {
		m := 0.0
		for _, s := range sections {
			sec, ok := h.eng.route.Data.Topology.Section(s)
			if !ok {
				t.Fatalf("section %d unknown", s)
			}
			m += sec.LengthM
		}
		return int(math.Round(m * 3.28084))
	}

	bowl := p.Locations[trackmap.Location{Kind: trackmap.KindYard, Yard: "NOR", Track: "Bowl 1"}.Key()]
	if want := feet(run8routetest.SecBowl1); bowl.LengthFt != want || want == 0 {
		t.Errorf("Bowl 1 measures %d ft, want %d", bowl.LengthFt, want)
	}
	if bowl.Cars != 2 || len(bowl.CarNumbers) != 2 || bowl.CarNumbers[0] != number("B1") || bowl.CarNumbers[1] != number("B2") {
		t.Errorf("Bowl 1 cars = %d %v, want the two cars and no locomotive", bowl.Cars, bowl.CarNumbers)
	}

	brl := p.Locations[trackmap.Location{Kind: trackmap.KindIndustry, Industries: []string{"BRL"}}.Key()]
	if want := feet(run8routetest.SecSiding1, run8routetest.SecSiding2); brl.LengthFt != want {
		t.Errorf("BRL measures %d ft, want %d", brl.LengthFt, want)
	}
	// S1 counts once, at its lead truck, plus the six cars.
	if brl.Cars != 7 || len(brl.CarNumbers) != 5 || brl.CarNumbers[0] != number("S1") {
		t.Errorf("BRL cars = %d %v, want 7 with 5 numbers", brl.Cars, brl.CarNumbers)
	}

	// A location nothing stands on has a length and no cars.
	empty := p.Locations[trackmap.Location{Kind: trackmap.KindYard, Yard: "NOR", Track: "Bowl 4"}.Key()]
	if empty.Cars != 0 || len(empty.CarNumbers) != 0 || empty.LengthFt == 0 {
		t.Errorf("Bowl 4 = %+v", empty)
	}

	// Without route data nothing is measured, but the cars are still found.
	got := ImportLocations(p.Diff, h.eng.snap, h.eng.eff, nil)
	for k, l := range got {
		if l.LengthFt != 0 {
			t.Errorf("%s measures %d ft without route data", k, l.LengthFt)
		}
	}
	if got[trackmap.Location{Kind: trackmap.KindYard, Yard: "NOR", Track: "Bowl 1"}.Key()].Cars != 2 {
		t.Error("no route data: the cars on Bowl 1 are lost")
	}
}
