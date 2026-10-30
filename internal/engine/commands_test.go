package engine

import (
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"run8-ops-center/internal/carstatus"
	"run8-ops-center/internal/profile"
	"run8-ops-center/internal/store"
	"run8-ops-center/internal/trackmap"
)

func yardLabel(track string) trackmap.Label {
	return trackmap.Label{Kind: trackmap.KindYard, Yard: "NOR", Track: track}
}

func TestLabelCommandsRecomputeAndPersist(t *testing.T) {
	h := start(t)
	h.ingest(snapshotEvent(parse(t, worldPath), "WorldSave.xml", false))
	v := h.eng.View()
	if len(v.Candidates) == 0 {
		t.Fatal("no candidates")
	}
	c := v.Candidates[0]
	for _, x := range v.Candidates { // the cut with the most cars
		if x.Cars > c.Cars {
			c = x
		}
	}

	// Bad input is refused and leaves the view alone.
	seq := v.Seq
	for name, err := range map[string]error{
		"unknown train":     h.eng.LabelCandidate("no-such-train", yardLabel("A")),
		"yard no track":     h.eng.LabelCandidate(c.TrainID, trackmap.Label{Kind: trackmap.KindYard, Yard: "NOR"}),
		"industry no tag":   h.eng.LabelCandidate(c.TrainID, trackmap.Label{Kind: trackmap.KindIndustry}),
		"bad kind":          h.eng.LabelSection(1, 2, trackmap.Label{Kind: "road"}),
		"remove unlabelled": h.eng.RemoveLabel(1, 2),
	} {
		if err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
	if h.eng.View().Seq != seq {
		t.Error("failed commands published a view")
	}

	// Labelling one candidate maps its cars and drops it from the list.
	if err := h.eng.LabelCandidate(c.TrainID, yardLabel("Bowl 1")); err != nil {
		t.Fatal(err)
	}
	v = h.eng.View()
	if slices.ContainsFunc(v.Candidates, func(x trackmap.Candidate) bool { return x.TrainID == c.TrainID }) {
		t.Error("the labelled candidate is still listed")
	}
	if v.Coverage.Unmapped >= 458 || len(v.Locations) != 1 || v.Locations[0].Location.Track != "Bowl 1" {
		t.Errorf("coverage=%+v locations=%+v", v.Coverage, v.Locations)
	}
	// It is on disk, in the profile folder.
	saved, err := trackmap.Load(filepath.Join(h.profileDir(), trackMapFile))
	if err != nil || len(saved.Labels) != len(v.TrackMap.Labels) || len(saved.Labels) == 0 {
		t.Fatalf("saved map: %v, %d labels", err, len(saved.Labels))
	}

	// Rename the location, then relabel one section as an industry.
	n, err := h.eng.RenameLocation(trackmap.Label(yardLabel("Bowl 1")).Location(), trackmap.Label(yardLabel("Bowl 2")).Location())
	if err != nil || n != len(c.Sections) {
		t.Fatalf("rename: n=%d (want %d) err=%v", n, len(c.Sections), err)
	}
	if v = h.eng.View(); v.Locations[0].Location.Track != "Bowl 2" {
		t.Errorf("locations after rename = %+v", v.Locations)
	}
	if n, err = h.eng.RenameLocation(trackmap.Label(yardLabel("Nowhere")).Location(), trackmap.Label(yardLabel("X")).Location()); n != 0 || err != nil {
		t.Errorf("renaming an unknown location: %d, %v", n, err)
	}

	s := c.Sections[0]
	if err := h.eng.LabelSection(s.Prefix, s.Section, trackmap.Label{Kind: trackmap.KindIndustry, Industries: []string{" GES ", "GES"}}); err != nil {
		t.Fatal(err)
	}
	v = h.eng.View()
	atGES := 0
	for _, car := range v.Status.Cars {
		if car.Loc.Kind == carstatus.LocIndustry && slices.Equal(car.Loc.Industries, []string{"GES"}) {
			atGES++
		}
	}
	if atGES == 0 || v.Tiles.AtIndustries != atGES {
		t.Errorf("cars at GES = %d, tile = %d", atGES, v.Tiles.AtIndustries)
	}

	// Removing a label makes its cars unmapped again.
	if err := h.eng.RemoveLabel(s.Prefix, s.Section); err != nil {
		t.Fatal(err)
	}
	v = h.eng.View()
	if v.Tiles.AtIndustries != 0 || !slices.ContainsFunc(v.Candidates, func(x trackmap.Candidate) bool { return x.TrainID == c.TrainID }) {
		t.Errorf("after remove: at industries %d, candidates %d", v.Tiles.AtIndustries, len(v.Candidates))
	}
}

func TestLabelBeforeFirstSnapshot(t *testing.T) {
	h := start(t)
	if err := h.eng.LabelSection(280, 4535, trackmap.Label{Kind: trackmap.KindIgnored}); err != nil {
		t.Fatal(err)
	}
	v := h.eng.View()
	if v.HasSnapshot || len(v.Locations) != 1 || v.Locations[0].Location.Kind != trackmap.KindIgnored {
		t.Errorf("HasSnapshot=%v locations=%+v", v.HasSnapshot, v.Locations)
	}
}

func TestThresholdOverridesApplyAndClear(t *testing.T) {
	h := start(t)
	h.ingest(snapshotEvent(parse(t, worldPath), "WorldSave.xml", false))
	h.labelAllYard()
	build := func(v *View, sym string) (b struct {
		Pct   float64
		Ready bool
	}) {
		for _, x := range v.Building {
			if x.Symbol == sym {
				b.Pct, b.Ready = x.ThresholdPct, x.Ready
			}
		}
		return
	}
	v := h.eng.View()
	if b := build(v, "108"); b.Pct != 50 || !b.Ready || findOrder(v.Available, "avail:108") == nil {
		t.Fatalf("108 before: %+v", b)
	}

	if err := h.eng.SetThreshold("108", new(70.0), new(5.0)); err != nil {
		t.Fatal(err)
	}
	v = h.eng.View()
	rt := v.Profile.RoadTrainBySymbol("108")
	if b := build(v, "108"); b.Pct != 70 || b.Ready || findOrder(v.Available, "avail:108") != nil || rt.MinLengthPct != 70 || rt.MaxCarAgeDays != 5 {
		t.Errorf("108 at 70%%: %+v min=%v age=%v", b, rt.MinLengthPct, rt.MaxCarAgeDays)
	}
	if th := v.Settings.Thresholds["108"]; th.MinLengthPct == nil || *th.MinLengthPct != 70 || *th.MaxCarAgeDays != 5 {
		t.Errorf("settings = %+v", v.Settings.Thresholds)
	}

	// Lowering it brings the order back; a nil pointer clears that override.
	if err := h.eng.SetThreshold("108", new(30.0), nil); err != nil {
		t.Fatal(err)
	}
	v = h.eng.View()
	if v.Profile.RoadTrainBySymbol("108").MaxCarAgeDays != 0 || findOrder(v.Available, "avail:108") == nil || build(v, "108").Pct != 30 {
		t.Error("30% with no age limit did not apply")
	}

	h.open() // the override survives a restart
	if v = h.eng.View(); v.Profile.RoadTrainBySymbol("108").MinLengthPct != 30 {
		t.Errorf("after restart: min = %v", v.Profile.RoadTrainBySymbol("108").MinLengthPct)
	}
	if err := h.eng.SetThreshold("108", nil, nil); err != nil {
		t.Fatal(err)
	}
	v = h.eng.View()
	if v.Profile.RoadTrainBySymbol("108").MinLengthPct != profile.DefaultMinLengthPct || len(v.Settings.Thresholds) != 0 {
		t.Errorf("cleared: min=%v thresholds=%v", v.Profile.RoadTrainBySymbol("108").MinLengthPct, v.Settings.Thresholds)
	}

	for name, err := range map[string]error{
		"unknown train": h.eng.SetThreshold("ZZZ", new(10.0), nil),
		"over 100":      h.eng.SetThreshold("108", new(101.0), nil),
		"negative":      h.eng.SetThreshold("108", new(-1.0), nil),
		"negative age":  h.eng.SetThreshold("108", nil, new(-2.0)),
	} {
		if err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}

func TestSetInstallDirAndServer(t *testing.T) {
	h := start(t)
	install := filepath.Join(t.TempDir(), "Run8")
	if err := h.eng.SetInstallDir(install); err != nil {
		t.Fatal(err)
	}
	if h.guard.InstallDir() != install || h.eng.View().Settings.RunInstallDir != install {
		t.Error("install folder not applied")
	}
	// From now on writes inside it are refused, so labelling still works but
	// an install folder holding the data directory is rejected.
	if err := h.eng.SetInstallDir(filepath.Dir(h.dir)); !errors.Is(err, store.ErrInstallConflict) {
		t.Errorf("an install folder containing the profile or data: err = %v", err)
	}
	if h.guard.InstallDir() != install || h.eng.View().Settings.RunInstallDir != install {
		t.Error("a rejected install folder changed the setting")
	}
	if err := h.eng.SetServer(9090, true); err != nil {
		t.Fatal(err)
	}
	if s := h.eng.View().Settings; s.Port != 9090 || !s.AllInterfaces {
		t.Errorf("settings = %+v", s)
	}
	for _, p := range []int{0, -1, 70000} {
		if err := h.eng.SetServer(p, false); err == nil {
			t.Errorf("port %d accepted", p)
		}
	}

	saved, err := store.LoadSettings(h.dir)
	if err != nil || saved.RunInstallDir != install || saved.Port != 9090 || !saved.AllInterfaces {
		t.Fatalf("settings.json = %+v, %v", saved, err)
	}

	// A restart re-applies the saved install folder to the guard.
	h.guard = store.NewGuard()
	h.open()
	if h.guard.InstallDir() != install {
		t.Errorf("guard install dir after restart = %q", h.guard.InstallDir())
	}

	// Clearing it works too.
	if err := h.eng.SetInstallDir(""); err != nil || h.guard.InstallDir() != "" {
		t.Errorf("clear: %v %q", err, h.guard.InstallDir())
	}
}

func TestNewRefusesProtectedDataDir(t *testing.T) {
	protected := t.TempDir()
	g := store.NewGuard(protected)
	g.SetLogger(slog.New(slog.NewTextHandler(io.Discard, nil)))
	_, err := New(Config{DataDir: filepath.Join(protected, "data"), Guard: g})
	if !errors.Is(err, store.ErrProtectedPath) {
		t.Fatalf("err = %v", err)
	}
	if entries, _ := os.ReadDir(protected); len(entries) != 0 {
		t.Errorf("New created %d entries inside the protected folder", len(entries))
	}
	// The saved install folder counts too: a data directory inside it is refused.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, store.SettingsFile), []byte(`{"run8InstallDir": `+quote(dir)+`}`), 0o644); err != nil {
		t.Fatal(err)
	}
	g2 := store.NewGuard()
	g2.SetLogger(slog.New(slog.NewTextHandler(io.Discard, nil)))
	if _, err := New(Config{DataDir: dir, Guard: g2}); err == nil {
		t.Error("an install folder holding the data directory was accepted")
	}
	if _, err := New(Config{DataDir: t.TempDir()}); err == nil {
		t.Error("New without a guard succeeded")
	}
}

func quote(s string) string { return `"` + filepath.ToSlash(s) + `"` }

func TestSubscribeCoalescesAndCancels(t *testing.T) {
	h := start(t)
	events, cancel := h.eng.Subscribe()
	// Several publishes with nobody reading never block the engine.
	for i := range 3 {
		if err := h.eng.SetServer(8000+i, false); err != nil {
			t.Fatal(err)
		}
	}
	if len(events) != 1 {
		t.Fatalf("buffered notifications = %d, want 1 (coalesced)", len(events))
	}
	<-events
	if err := h.eng.SetServer(8100, false); err != nil {
		t.Fatal(err)
	}
	select {
	case <-events:
	default:
		t.Error("no notification after a publish")
	}
	cancel()
	cancel() // idempotent
	if err := h.eng.SetServer(8101, false); err != nil {
		t.Fatal(err)
	}
	if len(events) != 0 {
		t.Error("a cancelled subscription still receives")
	}
}

func TestCommandsAfterStop(t *testing.T) {
	h := start(t)
	h.close()
	if err := h.eng.SetServer(8000, false); !errors.Is(err, ErrStopped) {
		t.Errorf("err = %v, want ErrStopped", err)
	}
	if _, err := h.eng.StartOrder("avail:131", ""); !errors.Is(err, ErrStopped) {
		t.Errorf("err = %v, want ErrStopped", err)
	}
	h.eng.Ingest(snapshotEvent(parse(t, worldPath), "WorldSave.xml", false)) // must not block
	if h.eng.View() == nil {
		t.Error("View is nil after stop")
	}
}

func TestConcurrentReadersAndCommands(t *testing.T) {
	h := start(t)
	h.ingest(snapshotEvent(parse(t, worldPath), "WorldSave.xml", false))
	h.labelAllYard()

	stop := make(chan struct{})
	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			events, cancel := h.eng.Subscribe()
			defer cancel()
			for {
				v := h.eng.View()
				n := len(v.Available) + len(v.Active) + len(v.History) + len(v.Activity) + len(v.Warnings) + len(v.Candidates) + len(v.TrackMap.Labels)
				if v.Status != nil {
					n += len(v.Status.Cars)
				}
				_ = n
				select {
				case <-stop:
					return
				case <-events:
				case <-time.After(time.Millisecond):
				}
			}
		}()
	}
	for i := range 3 {
		o, err := h.eng.StartOrder("avail:ab10", "industry:BSI")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := h.eng.ApplyOrder(o.ID, "cancel"); err != nil {
			t.Fatal(err)
		}
		snap := parse(t, worldPath)
		snap.GameDate = snap.GameDate.Add(time.Duration(i+1) * time.Hour)
		h.ingest(snapshotEvent(snap, "WorldSave.xml", false))
	}
	close(stop)
	wg.Wait()
	if v := h.eng.View(); len(v.History) != 3 {
		t.Errorf("history = %d", len(v.History))
	}
}
