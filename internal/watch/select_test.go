package watch

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"run8-ops-center/internal/run8save"
)

// selectionEnv adds a Train save, a sidecar-less world save and a sidecar-less
// train save to the standard environment.
func selectionEnv(t *testing.T) *env {
	t.Helper()
	e := newEnv(t)
	for _, ext := range []string{".xml", ".info"} {
		if err := os.WriteFile(filepath.Join(e.save, "29K"+ext), fixture(t, "29K"+ext), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// No sidecars: the XML saveGameType decides.
	if err := os.WriteFile(filepath.Join(e.save, "NoSidecarWorld.xml"), e.world, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(e.save, "NoSidecarTrain.xml"), fixture(t, "29K.xml"), 0o644); err != nil {
		t.Fatal(err)
	}
	return e
}

func TestListWorldSaves(t *testing.T) {
	e := selectionEnv(t)
	got, err := ListWorldSaves(e.save)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"NoSidecarWorld", "WorldSave"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ListWorldSaves = %v, want %v (Train saves, with or without sidecar, excluded)", got, want)
	}
}

func TestResolveMatchesCaseInsensitivelyWithOrWithoutExtension(t *testing.T) {
	e := selectionEnv(t)
	want := filepath.Join(e.save, "WorldSave.xml")
	for _, name := range []string{"WorldSave", "worldsave", "WORLDSAVE.XML", "WorldSave.xml", "worldsave.Xml"} {
		got, err := Resolve(e.save, name)
		if err != nil {
			t.Errorf("Resolve(%q): %v", name, err)
			continue
		}
		if got != want {
			t.Errorf("Resolve(%q) = %q, want %q", name, got, want)
		}
	}
	if got, err := Resolve(e.save, "nosidecarworld"); err != nil || filepath.Base(got) != "NoSidecarWorld.xml" {
		t.Errorf("sidecar-less world save by XML header: %q, %v", got, err)
	}
}

func TestResolveErrorsListWorldSaves(t *testing.T) {
	e := selectionEnv(t)
	cases := []struct {
		name, label string
		wantInErr   []string
	}{
		{"", "missing flag", []string{"--save"}},
		{"Nope", "unknown name", []string{`"Nope"`}},
		{"29K", "Train save with sidecar", []string{"29K", "not a world save"}},
		{"NoSidecarTrain", "Train save without sidecar", []string{"NoSidecarTrain", "not a world save"}},
		{"Bessemer Yard", "no such file", []string{"Bessemer Yard"}},
	}
	for _, tc := range cases {
		t.Run(tc.label, func(t *testing.T) {
			path, err := Resolve(e.save, tc.name)
			if err == nil {
				t.Fatalf("Resolve(%q) = %q, want error", tc.name, path)
			}
			msg := err.Error()
			for _, s := range append(tc.wantInErr, "WorldSave", "NoSidecarWorld") {
				if !strings.Contains(msg, s) {
					t.Errorf("error %q lacks %q", msg, s)
				}
			}
			listing := msg[strings.LastIndex(msg, ": ")+2:]
			if strings.Contains(listing, "29K") || strings.Contains(listing, "NoSidecarTrain") {
				t.Errorf("listing %q includes a Train save", listing)
			}
			if strings.Contains(listing, ".xml") {
				t.Errorf("listing %q should give names without .xml", listing)
			}
		})
	}
}

func TestResolveMissingSaveDirNamesPathWithoutFallback(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "Trains")
	_, err := Resolve(missing, "WorldSave")
	if err == nil {
		t.Fatal("expected an error for a missing save folder")
	}
	if !strings.Contains(err.Error(), missing) {
		t.Errorf("error %q does not name %s", err, missing)
	}
	// WorldSave exists in testdata; a fallback would have found it.
	if _, err := New(Config{SaveDir: missing, SaveName: "WorldSave"}); err == nil || !strings.Contains(err.Error(), missing) {
		t.Errorf("New with a missing save folder: %v", err)
	}
}

func TestNewAppliesDefaults(t *testing.T) {
	e := newEnv(t)
	w, err := New(Config{SaveDir: e.save, SaveName: "worldsave"})
	if err != nil {
		t.Fatal(err)
	}
	if w.cfg.Settle != time.Second || w.cfg.Poll != 5*time.Second || w.cfg.MinOverlap != 0.90 {
		t.Errorf("defaults: %+v", w.cfg)
	}
}

func TestNewRejectsBadSelection(t *testing.T) {
	e := selectionEnv(t)
	for _, name := range []string{"", "Nope", "29K"} {
		if w, err := New(Config{SaveDir: e.save, SaveName: name}); err == nil || w != nil {
			t.Errorf("New(%q) = %v, %v; want an error", name, w, err)
		}
	}
}

func TestReferenceAutoSaveSidecarDecodesAsWorld(t *testing.T) {
	auto, err := run8save.ReadInfo(filepath.Join(fixtureDir, autoFixed+".info"))
	if err != nil {
		t.Fatal(err)
	}
	if auto.SaveType != run8save.SaveWorld {
		t.Errorf("SaveType = %q, want World", auto.SaveType)
	}
	wantTS := time.Date(2026, 10, 28, 17, 28, 2, 396337000, time.UTC)
	if !auto.FileTimestamp.Equal(wantTS) {
		t.Errorf("FileTimestamp = %v, want %v", auto.FileTimestamp, wantTS)
	}
	world, err := run8save.ReadInfo(filepath.Join(fixtureDir, "WorldSave.info"))
	if err != nil {
		t.Fatal(err)
	}
	if !auto.FileTimestamp.Before(world.FileTimestamp) {
		t.Errorf("auto-save %v should be older than WorldSave %v", auto.FileTimestamp, world.FileTimestamp)
	}
	if !world.FileTimestamp.Equal(time.Date(2026, 10, 28, 17, 37, 39, 27703100, time.UTC)) {
		t.Errorf("WorldSave FileTimestamp = %v", world.FileTimestamp)
	}
}

func TestOverlap(t *testing.T) {
	keys := func(prefix string, n int) map[string]struct{} {
		m := map[string]struct{}{}
		for i := range n {
			m[prefix+string(rune('A'+i/100))+string(rune('a'+i%100/10))+string(rune('0'+i%10))] = struct{}{}
		}
		return m
	}
	// cand has 458 keys, 440 of them also in base.
	base := keys("k", 440)
	cand := keys("k", 440)
	for k := range keys("new", 18) {
		cand[k] = struct{}{}
	}
	if len(cand) != 458 {
		t.Fatalf("setup: %d", len(cand))
	}
	present, total := overlapCounts(cand, base)
	if present != 440 || total != 458 {
		t.Fatalf("counts = %d/%d", present, total)
	}
	if ov := Overlap(cand, base); ov < DefaultMinOverlap || percent(present, total) != 96 {
		t.Errorf("440/458: overlap %.4f (%d%%) should be accepted at 96%%", ov, percent(present, total))
	}

	// 300 candidate keys, 40 in base.
	cand2 := keys("x", 260)
	base2 := map[string]struct{}{}
	for k := range keys("y", 40) {
		cand2[k] = struct{}{}
		base2[k] = struct{}{}
	}
	present, total = overlapCounts(cand2, base2)
	if present != 40 || total != 300 {
		t.Fatalf("counts = %d/%d", present, total)
	}
	if ov := Overlap(cand2, base2); ov >= DefaultMinOverlap || percent(present, total) != 13 {
		t.Errorf("40/300: overlap %.4f (%d%%) should be rejected at 13%%", ov, percent(present, total))
	}

	// Boundary and degenerate cases.
	if got := Overlap(keys("b", 100), keys("b", 90)); got != 0.90 {
		t.Errorf("90 of 100 = %v, want exactly the 0.90 threshold", got)
	}
	if got := percent(899, 1000); got != 89 {
		t.Errorf("percent floors: %d", got)
	}
	if got := Overlap(map[string]struct{}{}, base); got != 0 {
		t.Errorf("empty candidate overlap = %v", got)
	}
}
