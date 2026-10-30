package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"run8-ops-center/internal/carstatus"
	"run8-ops-center/internal/orders"
	"run8-ops-center/internal/profile"
)

func sampleSettings() Settings {
	return Settings{
		ProfileID:     "ags-south-p1",
		RunInstallDir: `C:\Games\Run8`,
		Port:          9090,
		AllInterfaces: true,
		NoBrowser:     true,
		Thresholds: map[string]Threshold{
			"108": {MinLengthPct: new(55.5)},
			"386": {MinLengthPct: new(0.0), MaxCarAgeDays: new(14.0)},
			"199": {},
		},
	}
}

func TestLoadSettingsMissingFileGivesDefaults(t *testing.T) {
	got, err := LoadSettings(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if want := DefaultSettings(); !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
	if got.Port != 8080 {
		t.Errorf("default port = %d", got.Port)
	}
}

func TestSettingsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	g := NewGuard()
	want := sampleSettings()
	if err := SaveSettings(g, dir, want); err != nil {
		t.Fatal(err)
	}
	got, err := LoadSettings(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("reloaded settings differ:\n got %+v\nwant %+v", got, want)
	}
	// A nil override survives as nil (inherit), distinct from an explicit 0.
	if got.Thresholds["108"].MaxCarAgeDays != nil {
		t.Error("unset MaxCarAgeDays came back non-nil")
	}
	if p := got.Thresholds["386"].MinLengthPct; p == nil || *p != 0 {
		t.Errorf("explicit zero override lost: %v", p)
	}
}

func TestLoadSettingsKeepsDefaultsForAbsentFields(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, SettingsFile), []byte(`{"profileId":"x"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := LoadSettings(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got.ProfileID != "x" || got.Port != DefaultPort {
		t.Errorf("got %+v", got)
	}
}

func TestLoadCorruptFilesError(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{SettingsFile, StateFile} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(`{"port": `), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := LoadSettings(dir); err == nil || !strings.Contains(err.Error(), SettingsFile) {
		t.Errorf("LoadSettings err = %v", err)
	}
	if _, err := LoadState(dir); err == nil || !strings.Contains(err.Error(), StateFile) {
		t.Errorf("LoadState err = %v", err)
	}
}

func sampleState() State {
	game := time.Date(2026, 10, 28, 17, 28, 2, 0, time.UTC)
	real := time.Date(2026, 10, 30, 9, 15, 0, 0, time.UTC)
	st := NewState()
	st.Seq = 7
	st.ProfileOrders["ags-south-p1"] = []orders.Order{
		{
			ID: "ord-6", Kind: orders.KindLocal, JobID: "108", Symbol: "108", Profile: "ags-south-p1",
			State: orders.StateInProgress,
			Lines: []orders.Line{{
				Kind: orders.LineIndustry, Target: "HTI", Name: "Holcim Trading Inc",
				SetOuts: []orders.CarRef{{Key: "k1", Number: "ARMN 194887", Type: "hopper", Loaded: true, Tag: "108 HTI", Location: "NOR / Bowl 14", LengthFt: 59.5, GrossTons: 110}},
				PickUps: []orders.CarRef{{Key: "k2", Number: "TILX 1", Done: true}},
			}},
			Totals:      orders.Totals{LengthFt: 119, GrossTons: 220, RequiredHP: 440, MaxLengthFt: 4000, MaxTons: 6000},
			Flags:       []orders.Flag{{Code: "shared-cars", Message: "k1 also on 109"}},
			Shared:      []string{"k1"},
			CreatedAt:   real,
			StartedAt:   real.Add(time.Minute),
			GameStarted: game,
		},
		{
			ID: "ord-7", Kind: orders.KindRoad, JobID: "386", Symbol: "386", Profile: "ags-south-p1",
			State:        orders.StateCompleted,
			Consist:      []orders.CarRef{{Key: "k3", Number: "BNSF 5", Type: "tank", Missing: true}},
			CreatedAt:    real,
			FinishedAt:   real.Add(time.Hour),
			GameFinished: game.Add(3 * time.Hour),
		},
	}
	st.ProfileOrders["other"] = []orders.Order{{ID: "ord-1", Profile: "other", State: orders.StateCancelled}}
	st.Dwell["ags-south-p1"] = carstatus.DwellState{
		"k1": {Loc: "yard:NOR:Bowl 14", Since: game},
		"k2": {Loc: "industry:HTI", Since: game.Add(-48 * time.Hour)},
	}
	st.AppendActivity(
		ActivityEntry{Time: real, Game: game, Kind: "retagged", Message: "ARMN 194887 108 HTI -> NOR"},
		ActivityEntry{Time: real.Add(time.Second), Game: game.Add(time.Minute), Kind: "order", Message: "ord-6 started"},
	)
	return st
}

func TestStateRoundTrip(t *testing.T) {
	dir := t.TempDir()
	g := NewGuard()
	want := sampleState()
	if err := SaveState(g, dir, want); err != nil {
		t.Fatal(err)
	}
	got, err := LoadState(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("reloaded state differs:\n got %+v\nwant %+v", got, want)
	}

	// Saving the reloaded value reproduces the same bytes.
	first, _ := os.ReadFile(filepath.Join(dir, StateFile))
	if err := SaveState(g, dir, got); err != nil {
		t.Fatal(err)
	}
	second, _ := os.ReadFile(filepath.Join(dir, StateFile))
	if string(first) != string(second) {
		t.Error("state.json bytes changed across a reload and save")
	}
}

func TestLoadStateMissingFileIsEmpty(t *testing.T) {
	got, err := LoadState(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, NewState()) {
		t.Errorf("got %+v", got)
	}
	if got.ProfileOrders == nil || got.Dwell == nil {
		t.Error("maps are nil; callers would panic on write")
	}
}

func TestLoadStateFillsNilMapsAndRejectsNewerVersion(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, StateFile)
	if err := os.WriteFile(path, []byte(`{"version":1,"seq":3}`), 0o644); err != nil {
		t.Fatal(err)
	}
	st, err := LoadState(dir)
	if err != nil {
		t.Fatal(err)
	}
	if st.Seq != 3 || st.ProfileOrders == nil || st.Dwell == nil {
		t.Errorf("got %+v", st)
	}
	if err := os.WriteFile(path, []byte(`{"version":99}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadState(dir); err == nil {
		t.Error("newer state version accepted")
	}
}

func TestActivityCap(t *testing.T) {
	st := NewState()
	base := time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)
	for i := range MaxActivity + 500 {
		st.AppendActivity(ActivityEntry{Game: base.Add(time.Duration(i) * time.Minute), Kind: "n", Message: fmt.Sprint(i)})
	}
	if len(st.Activity) != 2000 || MaxActivity != 2000 {
		t.Fatalf("len = %d, cap const = %d", len(st.Activity), MaxActivity)
	}
	if st.Activity[0].Message != "2499" {
		t.Errorf("newest = %q, want 2499", st.Activity[0].Message)
	}
	if last := st.Activity[len(st.Activity)-1].Message; last != "500" {
		t.Errorf("oldest kept = %q, want 500", last)
	}
}

func TestAppendActivityBatchOrderAndCap(t *testing.T) {
	st := NewState()
	st.AppendActivity(ActivityEntry{Message: "a"}, ActivityEntry{Message: "b"}, ActivityEntry{Message: "c"})
	st.AppendActivity(ActivityEntry{Message: "d"})
	var got []string
	for _, e := range st.Activity {
		got = append(got, e.Message)
	}
	if !equalStrings(got, []string{"d", "c", "b", "a"}) {
		t.Errorf("order = %v, want newest first [d c b a]", got)
	}

	// One batch larger than the cap keeps the newest MaxActivity entries.
	big := make([]ActivityEntry, MaxActivity+10)
	for i := range big {
		big[i].Message = fmt.Sprint(i)
	}
	st.AppendActivity(big...)
	if len(st.Activity) != MaxActivity || st.Activity[0].Message != fmt.Sprint(MaxActivity+9) || st.Activity[MaxActivity-1].Message != "10" {
		t.Errorf("len=%d first=%q last=%q", len(st.Activity), st.Activity[0].Message, st.Activity[len(st.Activity)-1].Message)
	}
}

func TestSaveStateCapsOversizedActivity(t *testing.T) {
	dir := t.TempDir()
	st := NewState()
	for i := range MaxActivity + 50 {
		st.Activity = append(st.Activity, ActivityEntry{Message: fmt.Sprint(i)})
	}
	if err := SaveState(NewGuard(), dir, st); err != nil {
		t.Fatal(err)
	}
	got, err := LoadState(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Activity) != MaxActivity || got.Activity[0].Message != "0" {
		t.Errorf("len=%d first=%q", len(got.Activity), got.Activity[0].Message)
	}
}

// A crash between writing the temporary file and renaming it must leave the
// previous file readable and unchanged.
func TestCrashMidWriteLeavesOldFileIntact(t *testing.T) {
	dir := t.TempDir()
	g := NewGuard()
	oldSettings, oldState := sampleSettings(), sampleState()
	if err := SaveSettings(g, dir, oldSettings); err != nil {
		t.Fatal(err)
	}
	if err := SaveState(g, dir, oldState); err != nil {
		t.Fatal(err)
	}
	beforeSettings, _ := os.ReadFile(filepath.Join(dir, SettingsFile))
	beforeState, _ := os.ReadFile(filepath.Join(dir, StateFile))

	crash := errors.New("simulated crash")
	var sawTemp string
	g.afterTempWrite = func(tmp string) error {
		sawTemp = tmp
		// The new content is complete on disk but not yet visible.
		if b, err := os.ReadFile(tmp); err != nil || len(b) == 0 {
			t.Errorf("temp file not fully written at crash point: %d bytes, %v", len(b), err)
		}
		return crash
	}

	newSettings := sampleSettings()
	newSettings.Port = 1234
	newState := sampleState()
	newState.Seq = 99
	if err := SaveSettings(g, dir, newSettings); !errors.Is(err, crash) {
		t.Fatalf("SaveSettings err = %v, want crash", err)
	}
	if err := SaveState(g, dir, newState); !errors.Is(err, crash) {
		t.Fatalf("SaveState err = %v, want crash", err)
	}
	if sawTemp == "" || filepath.Dir(sawTemp) != dir {
		t.Errorf("temp file %q not in the target directory", sawTemp)
	}

	afterSettings, _ := os.ReadFile(filepath.Join(dir, SettingsFile))
	afterState, _ := os.ReadFile(filepath.Join(dir, StateFile))
	if string(afterSettings) != string(beforeSettings) || string(afterState) != string(beforeState) {
		t.Error("old files changed by a failed write")
	}
	gotS, err := LoadSettings(dir)
	if err != nil || !reflect.DeepEqual(gotS, oldSettings) {
		t.Errorf("settings after crash: %+v, %v", gotS, err)
	}
	gotT, err := LoadState(dir)
	if err != nil || !reflect.DeepEqual(gotT, oldState) {
		t.Errorf("state after crash: seq=%d, %v", gotT.Seq, err)
	}
	if left := walk(t, dir); !equalStrings(left, []string{SettingsFile, StateFile}) {
		t.Errorf("temp files left behind: %q", left)
	}

	// A process killed for real leaves its temp file behind; that must not
	// disturb loading or the next save.
	stray := filepath.Join(dir, SettingsFile+".tmp-deadbeef")
	if err := os.WriteFile(stray, []byte(`{"port": 1`), 0o644); err != nil {
		t.Fatal(err)
	}
	g.afterTempWrite = nil
	if got, err := LoadSettings(dir); err != nil || !reflect.DeepEqual(got, oldSettings) {
		t.Errorf("load with stray temp: %+v, %v", got, err)
	}
	if err := SaveSettings(g, dir, newSettings); err != nil {
		t.Fatal(err)
	}
	if got, _ := LoadSettings(dir); got.Port != 1234 {
		t.Errorf("save after recovery: port = %d", got.Port)
	}
}

func TestAtomicWriteFileFailureToRenameCleansUp(t *testing.T) {
	dir := t.TempDir()
	g := NewGuard()
	// Renaming a file onto a non-empty directory fails on every platform.
	target := filepath.Join(dir, "target")
	if err := os.MkdirAll(filepath.Join(target, "child"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := g.AtomicWriteFile(target, []byte("x"), 0o644); err == nil {
		t.Fatal("expected rename failure")
	}
	if left := walk(t, dir); !equalStrings(left, []string{"target", "target/child"}) {
		t.Errorf("temp file left behind: %q", left)
	}
}

func TestAtomicWriteFileGuardChecksBothPaths(t *testing.T) {
	g, tmp, dir := install(t)
	// The final path is outside, so only a guard check of the temp path or
	// the final path can catch a write through a link into the install dir.
	link := filepath.Join(tmp, "dirlink")
	if err := os.Symlink(dir, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := g.AtomicWriteFile(filepath.Join(link, "state.json"), []byte("x"), 0o644); !errors.Is(err, ErrProtectedPath) {
		t.Fatalf("err = %v, want ErrProtectedPath", err)
	}
	if left := walk(t, dir); !equalStrings(left, []string{"Content", "Content/existing.txt"}) {
		t.Errorf("install dir modified: %q", left)
	}
}

func TestSaveThroughGuardRefusesProtectedDataDir(t *testing.T) {
	g, _, dir := install(t)
	if err := SaveSettings(g, filepath.Join(dir, "OpsData"), DefaultSettings()); !errors.Is(err, ErrProtectedPath) {
		t.Errorf("SaveSettings = %v", err)
	}
	if err := SaveState(g, filepath.Join(dir, "OpsData"), NewState()); !errors.Is(err, ErrProtectedPath) {
		t.Errorf("SaveState = %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "OpsData")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("OpsData exists: %v", err)
	}
}

// keep the profile import honest: CarRef.Type is a profile.CarType.
var _ profile.CarType = orders.CarRef{}.Type
