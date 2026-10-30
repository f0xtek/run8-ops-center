package run8route

import (
	"crypto/sha256"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"run8-ops-center/internal/run8route/run8routetest"
)

var synthRoutes = []Route{{Folder: run8routetest.Route}}

func writeSynthetic(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	run8routetest.WriteInstall(t, root, run8routetest.Synthetic())
	return root
}

func statusOf(res Result, name string) FileStatus {
	for _, f := range res.Files {
		if f.Name == name {
			return f
		}
	}
	return FileStatus{}
}

func TestLoadSyntheticInstall(t *testing.T) {
	root := writeSynthetic(t)
	res := Load(root, run8routetest.Region, synthRoutes)
	wantNames := []string{ConfigFile, TrackDBFile, HumpFile, LocationsFile, MilepostsFile}
	if len(res.Files) != len(wantNames) {
		t.Fatalf("files = %+v", res.Files)
	}
	for i, f := range res.Files {
		if f.Name != wantNames[i] || !f.OK || f.Err != nil || f.Path == "" {
			t.Errorf("file %d = %+v", i, f)
		}
	}
	if statusOf(res, ConfigFile).Path != run8routetest.ConfigPath(root, run8routetest.Region) {
		t.Errorf("Config.ind path = %q", statusOf(res, ConfigFile).Path)
	}
	d := res.Data
	if len(d.Industries) != 3 || d.Topology == nil || len(d.Hump) != 1 || len(d.Locations) != 8 || len(d.Mileposts) != 2 {
		t.Errorf("data = %d industries, topology %v, %d hump, %d locations, %d mileposts",
			len(d.Industries), d.Topology != nil, len(d.Hump), len(d.Locations), len(d.Mileposts))
	}
	// Mileposts use the tile size from the track database.
	if math.Abs(d.Mileposts[1].X-6000) > 1e-3 || math.Abs(d.Mileposts[1].Z+1200) > 1e-3 {
		t.Errorf("milepost = %+v", d.Mileposts[1])
	}
	if len(res.Fingerprint) != 5 {
		t.Errorf("fingerprint = %+v", res.Fingerprint)
	}
	if !SameFingerprint(res.Fingerprint, Stat(root, run8routetest.Region, synthRoutes)) {
		t.Error("Stat and Load fingerprints differ")
	}
}

func TestLoadFailsEachFileOnItsOwn(t *testing.T) {
	root := writeSynthetic(t)
	if err := os.Remove(run8routetest.RoutePath(root, run8routetest.Route, TrackDBFile)); err != nil {
		t.Fatal(err)
	}
	res := Load(root, run8routetest.Region, synthRoutes)
	if f := statusOf(res, TrackDBFile); f.OK || f.Err == nil || f.Path == "" {
		t.Errorf("TrackDatabase.r8 = %+v", f)
	}
	if res.Data.Topology != nil {
		t.Error("topology present without its file")
	}
	for _, name := range []string{ConfigFile, HumpFile, LocationsFile, MilepostsFile} {
		if f := statusOf(res, name); !f.OK {
			t.Errorf("%s = %+v", name, f)
		}
	}
	if len(res.Data.Industries) != 3 || len(res.Data.Hump) != 1 {
		t.Errorf("the other files lost their data: %+v", res.Data)
	}
	if len(res.Fingerprint) != 4 {
		t.Errorf("fingerprint has %d entries, want 4 (the missing file is left out)", len(res.Fingerprint))
	}
}

func TestLoadReportsCorruptFile(t *testing.T) {
	root := writeSynthetic(t)
	path := run8routetest.ConfigPath(root, run8routetest.Region)
	b, _ := os.ReadFile(path)
	if err := os.WriteFile(path, b[:len(b)-3], 0o644); err != nil {
		t.Fatal(err)
	}
	res := Load(root, run8routetest.Region, synthRoutes)
	if f := statusOf(res, ConfigFile); f.OK || f.Err == nil {
		t.Errorf("truncated Config.ind = %+v", f)
	}
	if res.Data.Industries != nil {
		t.Error("industries from a rejected file")
	}
	if !statusOf(res, TrackDBFile).OK {
		t.Error("an unrelated file failed")
	}
	// A bad file still has a fingerprint, so a later fix is noticed.
	if len(res.Fingerprint) != 5 {
		t.Errorf("fingerprint = %d entries", len(res.Fingerprint))
	}
}

type fileState struct {
	sum   [32]byte
	mtime time.Time
}

func snapshotFiles(t *testing.T, root string) map[string]fileState {
	t.Helper()
	out := map[string]fileState{}
	err := filepath.Walk(root, func(p string, fi os.FileInfo, err error) error {
		if err != nil || fi.IsDir() {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		out[p] = fileState{sha256.Sum256(b), fi.ModTime()}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestLoadAndStatLeaveFilesAlone(t *testing.T) {
	root := writeSynthetic(t)
	// Age the files so a rewrite would show.
	old := time.Now().Add(-48 * time.Hour).Truncate(time.Second)
	filepath.Walk(root, func(p string, fi os.FileInfo, err error) error {
		if err == nil && !fi.IsDir() {
			os.Chtimes(p, old, old)
		}
		return nil
	})
	before := snapshotFiles(t, root)
	Load(root, run8routetest.Region, synthRoutes)
	Stat(root, run8routetest.Region, synthRoutes)
	after := snapshotFiles(t, root)
	if !reflect.DeepEqual(before, after) {
		t.Errorf("files changed: before %v after %v", before, after)
	}
	if len(before) != 5 {
		t.Errorf("%d files in the install", len(before))
	}
}

func TestFingerprintChangesWhenAFileIsRewritten(t *testing.T) {
	root := writeSynthetic(t)
	first := Load(root, run8routetest.Region, synthRoutes).Fingerprint

	// Same bytes, new modification time.
	path := run8routetest.ConfigPath(root, run8routetest.Region)
	later := time.Now().Add(time.Hour)
	if err := os.Chtimes(path, later, later); err != nil {
		t.Fatal(err)
	}
	if SameFingerprint(first, Stat(root, run8routetest.Region, synthRoutes)) {
		t.Error("a new modification time kept the fingerprint")
	}

	// New bytes.
	spec := run8routetest.Synthetic()
	spec.Industries = spec.Industries[:1]
	run8routetest.WriteInstall(t, root, spec)
	second := Load(root, run8routetest.Region, synthRoutes).Fingerprint
	if SameFingerprint(first, second) {
		t.Error("rewriting Config.ind kept the fingerprint")
	}
	if !SameFingerprint(second, Stat(root, run8routetest.Region, synthRoutes)) {
		t.Error("Stat disagrees with Load")
	}
}

func TestFingerprintSeesAFileAppearing(t *testing.T) {
	root := writeSynthetic(t)
	path := run8routetest.RoutePath(root, run8routetest.Route, MilepostsFile)
	b, _ := os.ReadFile(path)
	os.Remove(path)
	without := Stat(root, run8routetest.Region, synthRoutes)
	if len(without) != 4 {
		t.Fatalf("Stat = %d entries", len(without))
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
	if SameFingerprint(without, Stat(root, run8routetest.Region, synthRoutes)) {
		t.Error("a new file kept the fingerprint")
	}
}

func TestLoadRefusesUnsafeNames(t *testing.T) {
	root := writeSynthetic(t)
	for _, region := range []string{"", "..", "../GulfCoast", `..\..\Windows`, "Gulf/Coast"} {
		res := Load(root, region, nil)
		f := statusOf(res, ConfigFile)
		if f.OK || f.Err == nil {
			t.Errorf("region %q = %+v", region, f)
		}
		if len(Stat(root, region, nil)) != 0 {
			t.Errorf("Stat followed region %q", region)
		}
	}
	res := Load(root, run8routetest.Region, []Route{{Folder: "../" + run8routetest.Route}})
	if f := statusOf(res, TrackDBFile); f.OK || f.Err == nil {
		t.Errorf("route with a separator = %+v", f)
	}
	if res := Load("", run8routetest.Region, synthRoutes); statusOf(res, ConfigFile).Err == nil {
		t.Error("an empty install root was accepted")
	}
}

func TestLoadWithoutRoutesReadsConfigOnly(t *testing.T) {
	root := writeSynthetic(t)
	res := Load(root, run8routetest.Region, nil)
	if len(res.Files) != 1 || !res.Files[0].OK || len(res.Data.Industries) != 3 || res.Data.Topology != nil {
		t.Errorf("result = %+v", res)
	}
}

func TestExtraRoutesAreFingerprintedNotDecoded(t *testing.T) {
	root := writeSynthetic(t)
	spec := run8routetest.Synthetic()
	spec.Route = "Other"
	spec.Industries = nil
	run8routetest.WriteInstall(t, root, spec)
	routes := []Route{{Folder: run8routetest.Route}, {Folder: "Other"}}
	res := Load(root, run8routetest.Region, routes)
	if len(res.Files) != 5 || len(res.Fingerprint) != 9 {
		t.Errorf("%d files, %d fingerprints", len(res.Files), len(res.Fingerprint))
	}
	if !SameFingerprint(res.Fingerprint, Stat(root, run8routetest.Region, routes)) {
		t.Error("Stat and Load fingerprints differ")
	}
}

func TestCopyLocalSkipsOrCopies(t *testing.T) {
	root := t.TempDir()
	run8routetest.CopyLocal(t, root) // skips when references/AutoLabel is absent
	if _, err := os.Stat(run8routetest.ConfigPath(root, run8routetest.Region)); err != nil {
		t.Error(err)
	}
}
