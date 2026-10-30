package watch

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"run8-ops-center/internal/run8save"
)

const quietFor = 300 * time.Millisecond // a dozen polls at the injected 25 ms

func mustSnapshot(t *testing.T, ev Event) *run8save.Snapshot {
	t.Helper()
	if ev.Err != nil || ev.Warning != "" || ev.Snapshot == nil {
		t.Fatalf("want a snapshot event, got %s", describe(ev))
	}
	return ev.Snapshot
}

func TestStartupIngestsNamedSave(t *testing.T) {
	e := newEnv(t)
	h := start(t, e.config())
	ev := h.next()
	snap := mustSnapshot(t, ev)
	if ev.Auto || ev.Overlap != 0 {
		t.Errorf("named save event: auto=%v overlap=%v", ev.Auto, ev.Overlap)
	}
	if want := filepath.Join(e.save, "WorldSave.xml"); ev.Path != want {
		t.Errorf("Path = %q, want %q", ev.Path, want)
	}
	if snap.FileName != "WorldSave" || len(snap.Cars()) != 458 {
		t.Errorf("snapshot %q with %d cars", snap.FileName, len(snap.Cars()))
	}
	if !snap.WriteTime.Equal(e.base) {
		t.Errorf("WriteTime = %v, want sidecar timestamp %v", snap.WriteTime, e.base)
	}
	h.quiet(quietFor)
}

func TestWatcherNeverWritesSaveFolders(t *testing.T) {
	e := newEnv(t)
	putSave(t, e.autoPath("2026.10.30 19.15.00"), e.autoData, e.base.Add(time.Minute))
	listing := func() map[string]string {
		out := map[string]string{}
		for _, d := range []string{e.save, e.auto} {
			des, err := os.ReadDir(d)
			if err != nil {
				t.Fatal(err)
			}
			for _, de := range des {
				fi, err := de.Info()
				if err != nil {
					t.Fatal(err)
				}
				out[filepath.Join(d, de.Name())] = fmt.Sprint(fi.Size(), fi.ModTime().UnixNano())
			}
		}
		return out
	}
	before := listing()
	h := start(t, e.config())
	mustSnapshot(t, h.next())
	h.quiet(quietFor)
	h.cancel()
	if after := listing(); !reflect.DeepEqual(before, after) {
		t.Errorf("save folders changed:\nbefore %v\nafter  %v", before, after)
	}
}

func TestRewriteOfNamedSaveIsIngested(t *testing.T) {
	e := newEnv(t)
	h := start(t, e.config())
	first := mustSnapshot(t, h.next())

	// Run8 rewrites the world with one car's unit number changed.
	rewritten := mutateCars(e.world, func(i int) bool { return i == 0 })
	putSave(t, filepath.Join(e.save, "WorldSave.xml"), rewritten, e.base.Add(time.Minute))

	ev := h.next()
	snap := mustSnapshot(t, ev)
	if ev.Auto || ev.Path != filepath.Join(e.save, "WorldSave.xml") {
		t.Errorf("event: %s", describe(ev))
	}
	if !snap.WriteTime.Equal(e.base.Add(time.Minute)) {
		t.Errorf("WriteTime = %v", snap.WriteTime)
	}
	if reflect.DeepEqual(first.CarKeys(), snap.CarKeys()) {
		t.Error("rewritten content was not parsed: car keys identical")
	}
	h.quiet(quietFor)
}

func TestRewriteWithoutSidecarUsesModTime(t *testing.T) {
	e := newEnv(t)
	named := filepath.Join(e.save, "WorldSave.xml")
	if err := os.Remove(run8save.InfoPath(named)); err != nil {
		t.Fatal(err)
	}
	old := e.base
	if err := os.Chtimes(named, old, old); err != nil {
		t.Fatal(err)
	}
	h := start(t, e.config())
	if snap := mustSnapshot(t, h.next()); !snap.WriteTime.Equal(old) {
		t.Fatalf("WriteTime = %v, want mtime %v", snap.WriteTime, old)
	}
	if err := os.WriteFile(named, e.world, 0o644); err != nil { // mtime is now, an hour later
		t.Fatal(err)
	}
	if snap := mustSnapshot(t, h.next()); !snap.WriteTime.After(old) {
		t.Errorf("WriteTime = %v, want later than %v", snap.WriteTime, old)
	}
}

func TestOtherSavesAndNonMatchingAutoNamesAreIgnored(t *testing.T) {
	e := newEnv(t)
	h := start(t, e.config())
	mustSnapshot(t, h.next())

	later := e.base.Add(5 * time.Minute)
	// Save folder: another world save and a Train save, both newer.
	putSave(t, filepath.Join(e.save, "OtherWorld.xml"), e.world, later)
	if err := os.WriteFile(filepath.Join(e.save, "29K.xml"), fixture(t, "29K.xml"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeInfo(t, filepath.Join(e.save, "29K.xml"), run8save.SaveTrain, later)
	// Auto-save folder: names that do not match the pattern.
	for _, name := range []string{
		"Auto Save Train 2026.10.30 19.15.00.xml",
		"Auto Save World 2026.10.30 19.15.00.xml.bak",
		"auto save world 2026.10.30 19.16.00.xml",
		"Auto Save World 2026.10.30 19.15.xml",
		"notes.xml",
	} {
		putSave(t, filepath.Join(e.auto, name), e.autoData, later)
	}
	h.quiet(quietFor)

	// The watcher is alive: a matching auto-save is taken.
	path := e.autoPath("2026.10.30 19.15.00")
	putSave(t, path, e.autoData, later)
	ev := h.next()
	mustSnapshot(t, ev)
	if !ev.Auto || ev.Path != path {
		t.Errorf("event: %s", describe(ev))
	}
	h.quiet(quietFor)
}

func TestNewerAutoSaveIngestedOlderSkipped(t *testing.T) {
	e := newEnv(t)
	h := start(t, e.config())
	mustSnapshot(t, h.next())

	// Older than the named save: skipped, silently.
	putSave(t, e.autoPath("2026.10.30 17.00.00"), e.autoData, e.base.Add(-time.Minute))
	h.quiet(quietFor)

	newest := e.autoPath("2026.10.30 19.15.00")
	putSave(t, newest, e.autoData, e.base.Add(10*time.Minute))
	ev := h.next()
	snap := mustSnapshot(t, ev)
	if !ev.Auto || ev.Path != newest || snap.FileName != "Auto Save World 2026.10.30 19.15.00" {
		t.Errorf("event: %s", describe(ev))
	}
	if ev.Overlap < 0.99 {
		t.Errorf("Overlap = %v, want ~1 for the reference auto-save against the reference world", ev.Overlap)
	}

	// Now older than the last accepted snapshot, though newer than the named save.
	putSave(t, e.autoPath("2026.10.30 18.00.00"), e.autoData, e.base.Add(5*time.Minute))
	h.quiet(quietFor)

	// Equal write time is not strictly newer.
	putSave(t, e.autoPath("2026.10.30 19.15.01"), e.autoData, e.base.Add(10*time.Minute))
	h.quiet(quietFor)
}

func TestFileStillGrowingIsNotParsedUntilStable(t *testing.T) {
	e := newEnv(t)
	cfg := e.config()
	cfg.Settle = 250 * time.Millisecond
	h := start(t, cfg)
	mustSnapshot(t, h.next())

	// No sidecar: the modification time (now) orders it after the named save.
	path := e.autoPath("2026.10.30 19.15.00")
	const chunks = 12
	size := len(e.autoData) / chunks
	var finished atomic.Bool
	go func() {
		f, err := os.Create(path)
		if err != nil {
			return
		}
		defer f.Close()
		for i := range chunks {
			end := (i + 1) * size
			if i == chunks-1 {
				end = len(e.autoData)
			}
			f.Write(e.autoData[i*size : end])
			if i == chunks-1 {
				finished.Store(true)
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
	}()

	ev := h.next()
	if !finished.Load() {
		t.Fatalf("event before the writer finished: %s", describe(ev))
	}
	if ev.Err != nil {
		t.Fatalf("partial file was parsed: %v", ev.Err)
	}
	snap := mustSnapshot(t, ev)
	if !ev.Auto || len(snap.Cars()) != 458 {
		t.Errorf("event: %s with %d cars", describe(ev), len(snap.Cars()))
	}
	h.quiet(quietFor)
}

func TestMissingAutoSaveFolderOnlyWarns(t *testing.T) {
	e := newEnv(t)
	if err := os.RemoveAll(e.auto); err != nil {
		t.Fatal(err)
	}
	h := start(t, e.config())
	ev := h.next()
	if ev.Warning == "" || ev.Snapshot != nil || ev.Err != nil || !strings.Contains(ev.Warning, e.auto) {
		t.Fatalf("want a warning naming %s, got %s", e.auto, describe(ev))
	}
	snap := mustSnapshot(t, h.next())
	if snap.FileName != "WorldSave" {
		t.Errorf("snapshot %q", snap.FileName)
	}
	h.quiet(quietFor) // warned once, not on every poll

	// The named save is still followed.
	putSave(t, filepath.Join(e.save, "WorldSave.xml"), e.world, e.base.Add(time.Minute))
	if ev := h.next(); !mustSnapshot(t, ev).WriteTime.Equal(e.base.Add(time.Minute)) || ev.Auto {
		t.Errorf("rewrite not followed: %s", describe(ev))
	}

	// When Run8 creates the folder, auto-saves are followed too.
	if err := os.MkdirAll(e.auto, 0o755); err != nil {
		t.Fatal(err)
	}
	path := e.autoPath("2026.10.30 19.15.00")
	putSave(t, path, e.autoData, e.base.Add(10*time.Minute))
	if ev := h.next(); !ev.Auto || ev.Path != path || ev.Snapshot == nil {
		t.Errorf("auto-save after folder appeared: %s", describe(ev))
	}
}

func TestEmptyAutoSaveDirDisablesAutoSaves(t *testing.T) {
	e := newEnv(t)
	cfg := e.config()
	cfg.AutoSaveDir = ""
	h := start(t, cfg)
	mustSnapshot(t, h.next())
	h.quiet(quietFor)
}

func TestRunWithSaveFolderGoneIsAnErrorBeforeAnyEvent(t *testing.T) {
	e := newEnv(t)
	w, err := New(e.config())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(e.save); err != nil {
		t.Fatal(err)
	}
	var got []Event
	err = w.Run(t.Context(), func(ev Event) { got = append(got, ev) })
	if err == nil || !strings.Contains(err.Error(), e.save) {
		t.Fatalf("Run error = %v, want one naming %s", err, e.save)
	}
	if len(got) != 0 {
		t.Errorf("events emitted before the error: %v", got)
	}
}

func TestAutoSaveOfAnotherWorldIsRejectedWithPercentage(t *testing.T) {
	e := newEnv(t)
	h := start(t, e.config())
	base := mustSnapshot(t, h.next())

	other := mutateCars(e.autoData, func(i int) bool { return i%8 != 0 }) // ~13% of cars kept
	path := e.autoPath("2026.10.30 19.15.00")
	putSave(t, path, other, e.base.Add(10*time.Minute))

	cand, err := run8save.ParseFile(path)
	if err != nil {
		t.Fatal(err)
	}
	ov := Overlap(cand.CarKeys(), base.CarKeys())
	if ov < 0.05 || ov > 0.25 {
		t.Fatalf("synthetic other world overlaps %.2f, want about 13%%", ov)
	}
	pct := fmt.Sprintf("%d%%", int(ov*100))

	ev := h.next()
	if ev.Snapshot != nil || ev.Err != nil || ev.Path != path || !ev.Auto {
		t.Fatalf("want a warning event for the auto-save, got %s", describe(ev))
	}
	if !strings.Contains(ev.Warning, filepath.Base(path)) || !strings.Contains(ev.Warning, pct) {
		t.Errorf("warning %q should name the file and show %s", ev.Warning, pct)
	}
	if math.Abs(ev.Overlap-ov) > 1e-9 {
		t.Errorf("Overlap = %v, want %v", ev.Overlap, ov)
	}
	h.quiet(quietFor) // once per file, not on every poll

	// The rejected file did not become the baseline: a matching auto-save still passes.
	good := e.autoPath("2026.10.30 19.20.00")
	putSave(t, good, e.autoData, e.base.Add(11*time.Minute))
	if ev := h.next(); ev.Snapshot == nil || ev.Path != good {
		t.Errorf("matching auto-save after a rejected one: %s", describe(ev))
	}
}

func TestAutoSaveWith440Of458CarsIsAccepted(t *testing.T) {
	e := newEnv(t)
	h := start(t, e.config())
	mustSnapshot(t, h.next())

	churn := mutateCars(e.autoData, func(i int) bool { return i < 18 })
	path := e.autoPath("2026.10.30 19.15.00")
	putSave(t, path, churn, e.base.Add(10*time.Minute))

	ev := h.next()
	snap := mustSnapshot(t, ev)
	if len(snap.Cars()) != 458 {
		t.Fatalf("cars = %d", len(snap.Cars()))
	}
	if want := 440.0 / 458.0; math.Abs(ev.Overlap-want) > 1e-9 {
		t.Errorf("Overlap = %v, want %v (96%%)", ev.Overlap, want)
	}
}

func TestOverlapBaselineIsLastAcceptedSnapshot(t *testing.T) {
	e := newEnv(t)
	h := start(t, e.config())
	mustSnapshot(t, h.next())

	// 30 cars churn (93.4% against the world): accepted, and becomes the baseline.
	first := mutateCars(e.autoData, func(i int) bool { return i < 30 })
	putSave(t, e.autoPath("2026.10.30 19.15.00"), first, e.base.Add(10*time.Minute))
	mustSnapshot(t, h.next())

	// Another 30 cars churn on top of the first 30 (86.9% against the world,
	// 93.4% against the first auto-save): accepted only because the baseline moved.
	second := mutateCars(e.autoData, func(i int) bool { return i < 60 })
	putSave(t, e.autoPath("2026.10.30 19.30.00"), second, e.base.Add(20*time.Minute))
	ev := h.next()
	mustSnapshot(t, ev)
	world, err := run8save.ParseFile(filepath.Join(e.save, "WorldSave.xml"))
	if err != nil {
		t.Fatal(err)
	}
	if Overlap(ev.Snapshot.CarKeys(), world.CarKeys()) >= DefaultMinOverlap {
		t.Fatal("test setup: second auto-save should fail against the named save")
	}
}

func TestStartupOlderAutoSaveSkippedWithReferenceSidecars(t *testing.T) {
	e := newEnv(t)
	for _, f := range []string{"WorldSave.xml", "WorldSave.info"} {
		if err := os.WriteFile(filepath.Join(e.save, f), fixture(t, f), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, f := range []string{autoFixed + ".xml", autoFixed + ".info"} {
		if err := os.WriteFile(filepath.Join(e.auto, f), fixture(t, f), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	h := start(t, e.config())
	ev := h.next()
	snap := mustSnapshot(t, ev)
	if ev.Auto || snap.FileName != "WorldSave" {
		t.Errorf("startup ingested %s, want the named save", describe(ev))
	}
	if want := time.Date(2026, 10, 28, 17, 37, 39, 27703100, time.UTC); !snap.WriteTime.Equal(want) {
		t.Errorf("WriteTime = %v, want %v", snap.WriteTime, want)
	}
	h.quiet(quietFor)
}

func TestStartupNewerMatchingAutoSaveWins(t *testing.T) {
	e := newEnv(t)
	for _, f := range []string{"WorldSave.xml", "WorldSave.info"} {
		if err := os.WriteFile(filepath.Join(e.save, f), fixture(t, f), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// The reference auto-save, renamed and dated after the world save.
	path := e.autoPath("2026.10.30 19.15.00")
	putSave(t, path, e.autoData, time.Date(2026, 10, 28, 18, 37, 39, 0, time.UTC))
	// A still older matching auto-save loses to it.
	putSave(t, e.autoPath("2026.10.30 18.00.00"), e.autoData, time.Date(2026, 10, 28, 17, 50, 0, 0, time.UTC))

	h := start(t, e.config())
	ev := h.next()
	snap := mustSnapshot(t, ev)
	if !ev.Auto || ev.Path != path || snap.FileName != "Auto Save World 2026.10.30 19.15.00" {
		t.Fatalf("startup ingested %s, want the newer auto-save", describe(ev))
	}
	if ev.Overlap < 0.99 {
		t.Errorf("Overlap = %v", ev.Overlap)
	}
	h.quiet(quietFor) // the named save and the older auto-save are not emitted afterwards
}

func TestStartupRejectsOtherWorldThenTakesNewestMatch(t *testing.T) {
	e := newEnv(t)
	bad := e.autoPath("2026.10.30 20.00.00")
	good := e.autoPath("2026.10.30 19.00.00")
	putSave(t, bad, mutateCars(e.autoData, func(i int) bool { return i%8 != 0 }), e.base.Add(20*time.Minute))
	putSave(t, good, e.autoData, e.base.Add(10*time.Minute))

	h := start(t, e.config())
	warn := h.next()
	if warn.Snapshot != nil || !strings.Contains(warn.Warning, filepath.Base(bad)) || !strings.Contains(warn.Warning, "%") {
		t.Fatalf("want a warning for the other world's auto-save, got %s", describe(warn))
	}
	ev := h.next()
	mustSnapshot(t, ev)
	if !ev.Auto || ev.Path != good {
		t.Errorf("startup ingested %s, want %s", describe(ev), good)
	}
	h.quiet(quietFor)
}

func TestParseFailureAtStartupIsReportedAndRetried(t *testing.T) {
	e := newEnv(t)
	named := filepath.Join(e.save, "WorldSave.xml")
	putSave(t, named, e.world[:len(e.world)/2], e.base)

	h := start(t, e.config())
	ev := h.next()
	if ev.Err == nil || ev.Snapshot != nil || ev.Path != named {
		t.Fatalf("want a parse error for %s, got %s", named, describe(ev))
	}
	h.quiet(quietFor) // reported once, not on every poll

	putSave(t, named, e.world, e.base.Add(time.Minute))
	if snap := mustSnapshot(t, h.next()); len(snap.Cars()) != 458 {
		t.Errorf("cars = %d", len(snap.Cars()))
	}
}

func TestParseFailureWhileFollowingKeepsFollowing(t *testing.T) {
	e := newEnv(t)
	h := start(t, e.config())
	mustSnapshot(t, h.next())

	named := filepath.Join(e.save, "WorldSave.xml")
	putSave(t, named, e.world[:len(e.world)/2], e.base.Add(time.Minute))
	ev := h.next()
	if ev.Err == nil || ev.Snapshot != nil || ev.Path != named {
		t.Fatalf("want a parse error, got %s", describe(ev))
	}
	h.quiet(quietFor)

	// A truncated auto-save is reported too, as an auto-save.
	bad := e.autoPath("2026.10.30 19.15.00")
	putSave(t, bad, e.autoData[:len(e.autoData)/3], e.base.Add(2*time.Minute))
	if ev := h.next(); ev.Err == nil || !ev.Auto || ev.Path != bad {
		t.Fatalf("want an auto-save parse error, got %s", describe(ev))
	}
	h.quiet(quietFor)

	putSave(t, named, e.world, e.base.Add(3*time.Minute))
	if snap := mustSnapshot(t, h.next()); !snap.WriteTime.Equal(e.base.Add(3 * time.Minute)) {
		t.Errorf("WriteTime = %v", snap.WriteTime)
	}
}
