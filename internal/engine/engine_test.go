package engine

import (
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"run8-ops-center/internal/orders"
	"run8-ops-center/internal/run8save"
	"run8-ops-center/internal/store"
	"run8-ops-center/internal/trackmap"
)

var _ Backend = (*Engine)(nil)

func findOrder(list []orders.Order, id string) *orders.Order {
	for i := range list {
		if list[i].ID == id {
			return &list[i]
		}
	}
	return nil
}

func orderKeys(os ...orders.Order) map[string]bool {
	out := map[string]bool{}
	for _, o := range os {
		for _, k := range o.CarKeys() {
			out[k] = true
		}
	}
	return out
}

// TestPipelineTwoSnapshotsThenStart is the task 8.3 check: two snapshots go
// through derive, reconcile, generate, persist and publish, and a start
// command then changes the View.
func TestPipelineTwoSnapshotsThenStart(t *testing.T) {
	h := start(t)

	v0 := h.eng.View()
	if v0.HasSnapshot || v0.Status != nil || len(v0.Available) != 0 || v0.Profile.ID != DefaultProfileID {
		t.Fatalf("first view: HasSnapshot=%v Status=%v Available=%d profile=%s", v0.HasSnapshot, v0.Status, len(v0.Available), v0.Profile.ID)
	}
	if !slices.Equal(v0.Profiles, []ProfileInfo{{ID: DefaultProfileID, Name: v0.Profile.Name}}) {
		t.Errorf("Profiles = %v", v0.Profiles)
	}

	// First snapshot: the world save, nothing labelled yet.
	world := parse(t, worldPath)
	h.clock.Advance(time.Minute)
	v1 := h.ingest(snapshotEvent(world, "/saves/WorldSave.xml", false))
	if !v1.HasSnapshot || v1.Status == nil || len(v1.Status.Cars) != 458 {
		t.Fatalf("after first ingest: HasSnapshot=%v cars=%d", v1.HasSnapshot, len(v1.Status.Cars))
	}
	if v1.Source.ActiveFile != "WorldSave.xml" || v1.Source.ActiveAuto || !v1.Source.LastIngest.Equal(h.clock.Now()) ||
		!v1.Source.GameDate.Equal(world.GameDate) || v1.Source.SaveName != "WorldSave" || v1.Source.SaveDir != "saves" {
		t.Errorf("Source = %+v", v1.Source)
	}
	if v1.Tiles.OnRoute != 458 || v1.Tiles.Unmapped != 458 || v1.Coverage.Unmapped != 458 || len(v1.Candidates) == 0 || len(v1.Available) != 0 {
		t.Errorf("unlabelled view: tiles=%+v coverage=%+v candidates=%d available=%d", v1.Tiles, v1.Coverage, len(v1.Candidates), len(v1.Available))
	}
	if len(v1.Activity) != 1 || v1.Activity[0].Kind != "ingest" {
		t.Errorf("first ingest activity = %+v", v1.Activity)
	}

	// Label every section the snapshot uses, so orders exist.
	v2 := h.labelAllYard()
	if len(v2.Candidates) != 0 || v2.Coverage.Unmapped != 0 || v2.Tiles.Unmapped != 0 {
		t.Fatalf("labelled view: candidates=%d coverage=%+v", len(v2.Candidates), v2.Coverage)
	}
	if v2.Tiles.AwaitingDelivery != 233 || v2.Tiles.AwaitingTrain != 97 || v2.Tiles.AtIndustries != 0 || v2.Tiles.Unknown != 1 {
		t.Errorf("tiles = %+v", v2.Tiles)
	}
	for _, id := range []string{"avail:ab10", "avail:a55", "avail:108", "avail:131", "avail:339"} {
		if findOrder(v2.Available, id) == nil {
			t.Errorf("%s missing from available orders", id)
		}
	}
	if !slices.ContainsFunc(v2.Building, func(b orders.RoadBuild) bool { return b.Symbol == "108" && b.Ready }) {
		t.Errorf("108 should be ready in Building: %+v", v2.Building)
	}

	// Second snapshot: the later auto-save moves four trains.
	auto := parse(t, autoPath)
	h.clock.Advance(time.Minute)
	v3 := h.ingest(snapshotEvent(auto, "/autosaves/Auto Save World 2026.10.28 17.28.02.xml", true))
	if !v3.Source.ActiveAuto || v3.Source.ActiveFile != "Auto Save World 2026.10.28 17.28.02.xml" || !v3.Source.GameDate.Equal(auto.GameDate) {
		t.Errorf("Source after auto-save = %+v", v3.Source)
	}
	var moved []string
	for _, a := range v3.Activity {
		if a.Kind == "moved" {
			moved = append(moved, a.Message)
		}
	}
	if len(moved) != 4 {
		t.Fatalf("moved entries = %d, want one per moved train: %q", len(moved), moved)
	}
	if !slices.ContainsFunc(moved, func(m string) bool { return strings.Contains(m, "999949") }) {
		t.Errorf("no entry for train 999949: %q", moved)
	}
	// The auto-save has cars on sections the first labelling did not cover.
	v3 = h.labelAllYard()
	if v3.Coverage.Unmapped != 0 || findOrder(v3.Available, "avail:131") == nil || findOrder(v3.Available, "avail:339") == nil {
		t.Fatalf("after relabelling: coverage=%+v", v3.Coverage)
	}
	r131, r339 := findOrder(v3.Available, "avail:131"), findOrder(v3.Available, "avail:339")
	if len(r131.Shared) == 0 || len(r339.Shared) == 0 {
		t.Fatalf("131 and 339 should share cars: %d, %d", len(r131.Shared), len(r339.Shared))
	}

	// Start 131: its cars leave 339 and every other available order.
	h.clock.Advance(time.Minute)
	started, err := h.eng.StartOrder("avail:131", "")
	if err != nil {
		t.Fatal(err)
	}
	if started.ID != "ord-1" || started.State != orders.StateInProgress || !started.StartedAt.Equal(h.clock.Now()) || !started.GameStarted.Equal(auto.GameDate) {
		t.Errorf("started = %s %s %v game %v", started.ID, started.State, started.StartedAt, started.GameStarted)
	}
	v4 := h.eng.View() // the command returned, so this view includes it
	if len(v4.Active) != 1 || v4.Active[0].ID != "ord-1" {
		t.Fatalf("Active = %+v", v4.Active)
	}
	if findOrder(v4.Available, "avail:131") != nil {
		t.Error("131 is still available after starting it")
	}
	reserved := orderKeys(v4.Active[0])
	if len(reserved) != 68 {
		t.Errorf("reserved cars = %d, want 68", len(reserved))
	}
	for _, o := range v4.Available {
		for _, k := range o.CarKeys() {
			if reserved[k] {
				t.Errorf("available %s still lists reserved car %s", o.ID, k)
			}
		}
		if len(o.Shared) != 0 {
			t.Errorf("%s still has shared cars after the start", o.ID)
		}
	}
	if len(v4.Activity) == 0 || v4.Activity[0].Kind != "order" || !strings.Contains(v4.Activity[0].Message, "ord-1") || !strings.Contains(v4.Activity[0].Message, "131") {
		t.Errorf("newest activity = %+v", v4.Activity[0])
	}

	// The previous views are untouched.
	if findOrder(v3.Available, "avail:131") == nil || len(v3.Active) != 0 || len(v3.Activity) >= len(v4.Activity) {
		t.Error("a published View changed after a later command")
	}

	// state.json has the order, the seq and the dwell records.
	st, err := store.LoadState(h.dir)
	if err != nil {
		t.Fatal(err)
	}
	saved := st.ProfileOrders[DefaultProfileID]
	if len(saved) != 1 || saved[0].ID != "ord-1" || saved[0].State != orders.StateInProgress || st.Seq != 1 || len(st.Dwell[DefaultProfileID]) != 458 {
		t.Errorf("state.json: orders=%d seq=%d dwell=%d", len(saved), st.Seq, len(st.Dwell[DefaultProfileID]))
	}
	if len(st.Activity) != len(v4.Activity) {
		t.Errorf("state.json activity = %d entries, view has %d", len(st.Activity), len(v4.Activity))
	}
}

func TestStartOneLineLeavesTheOtherLines(t *testing.T) {
	h := start(t)
	h.ingest(snapshotEvent(parse(t, worldPath), "WorldSave.xml", false))
	v := h.labelAllYard()
	ab10 := findOrder(v.Available, "avail:ab10")
	if ab10 == nil || len(ab10.Lines) != 6 {
		t.Fatalf("ab10 = %+v", ab10)
	}
	var fin orders.Line
	for _, l := range ab10.Lines {
		if l.Key() == "industry:FIN" {
			fin = l
		}
	}
	if len(fin.SetOuts) == 0 {
		t.Fatal("no FIN line")
	}

	o, err := h.eng.StartOrder("avail:ab10", "industry:FIN")
	if err != nil {
		t.Fatal(err)
	}
	if len(o.Lines) != 1 || o.Lines[0].Key() != "industry:FIN" || len(o.CarKeys()) != len(fin.SetOuts) {
		t.Fatalf("started line order: lines=%d cars=%d", len(o.Lines), len(o.CarKeys()))
	}
	v = h.eng.View()
	rest := findOrder(v.Available, "avail:ab10")
	if rest == nil {
		t.Fatal("ab10 vanished after starting one line")
	}
	reserved := orderKeys(o)
	for _, l := range rest.Lines {
		for _, c := range append(slices.Clone(l.SetOuts), l.PickUps...) {
			if reserved[c.Key] {
				t.Errorf("line %s lists the reserved car %s", l.Key(), c.Key)
			}
		}
	}
	for _, key := range []string{"industry:BSI", "industry:WWT", "industry:BRR", "industry:CMC", "industry:WRG"} {
		if !slices.ContainsFunc(rest.Lines, func(l orders.Line) bool { return l.Key() == key }) {
			t.Errorf("line %s is no longer available", key)
		}
	}
	if len(v.Active) != 1 || v.Active[0].ID != "ord-1" {
		t.Errorf("Active = %+v", v.Active)
	}
	if !strings.Contains(v.Activity[0].Message, "AB10/FIN") {
		t.Errorf("activity = %q", v.Activity[0].Message)
	}

	// A second start takes the next id; starting a road order by line is refused.
	o2, err := h.eng.StartOrder("avail:ab10", "industry:BSI")
	if err != nil || o2.ID != "ord-2" {
		t.Fatalf("second start: %v %v", o2.ID, err)
	}
	if v = h.eng.View(); len(v.Active) != 2 || v.Active[0].ID != "ord-2" {
		t.Errorf("Active should list the newest first: %+v", v.Active)
	}
	before := h.eng.View().Seq
	if _, err := h.eng.StartOrder("avail:131", "stop:MCC"); err == nil {
		t.Error("starting one line of a road order should fail")
	}
	if _, err := h.eng.StartOrder("avail:nope", ""); err == nil {
		t.Error("starting an unknown order should fail")
	}
	if _, err := h.eng.StartOrder("avail:ab10", "industry:NOPE"); err == nil {
		t.Error("starting an unknown line should fail")
	}
	if h.eng.View().Seq != before {
		t.Error("a failed command published a new view")
	}
}

func TestConfirmAndCancelGoToHistory(t *testing.T) {
	h := start(t)
	world := parse(t, worldPath)
	h.ingest(snapshotEvent(world, "WorldSave.xml", false))
	h.labelAllYard()

	a, err := h.eng.StartOrder("avail:131", "")
	if err != nil {
		t.Fatal(err)
	}
	b, err := h.eng.StartOrder("avail:ab10", "industry:FIN")
	if err != nil {
		t.Fatal(err)
	}

	// Cancelling releases the cars: the FIN line is available again.
	h.clock.Advance(time.Hour)
	cancelled, err := h.eng.ApplyOrder(b.ID, orders.ActCancel)
	if err != nil {
		t.Fatal(err)
	}
	if cancelled.State != orders.StateCancelled || !cancelled.FinishedAt.Equal(h.clock.Now()) || !cancelled.GameFinished.Equal(world.GameDate) {
		t.Errorf("cancelled = %s finished %v game %v", cancelled.State, cancelled.FinishedAt, cancelled.GameFinished)
	}
	v := h.eng.View()
	if len(v.Active) != 1 || v.Active[0].ID != a.ID || len(v.History) != 1 || v.History[0].ID != b.ID {
		t.Fatalf("after cancel: Active=%d History=%d", len(v.Active), len(v.History))
	}
	ab10 := findOrder(v.Available, "avail:ab10")
	if ab10 == nil || !slices.ContainsFunc(ab10.Lines, func(l orders.Line) bool { return l.Key() == "industry:FIN" }) {
		t.Error("the cancelled order's cars were not released")
	}

	// A manual completion is allowed from In Progress; confirm is not.
	if _, err := h.eng.ApplyOrder(a.ID, orders.ActConfirm); !errors.Is(err, orders.ErrInvalidTransition) {
		t.Errorf("confirm on In Progress: err = %v", err)
	}
	h.clock.Advance(time.Hour)
	done, err := h.eng.ApplyOrder(a.ID, orders.ActComplete)
	if err != nil || done.State != orders.StateCompleted {
		t.Fatalf("complete: %v %v", done.State, err)
	}
	v = h.eng.View()
	if len(v.Active) != 0 || len(v.History) != 2 || v.History[0].ID != a.ID || v.History[1].ID != b.ID {
		t.Fatalf("history should list the newest finish first: %+v", v.History)
	}
	if findOrder(v.Available, "avail:131") == nil {
		t.Error("131 should be available again once its order is finished")
	}
	if _, err := h.eng.ApplyOrder("ord-99", orders.ActCancel); err == nil {
		t.Error("applying to an unknown order should fail")
	}
	if _, err := h.eng.ApplyOrder(a.ID, orders.ActCancel); !errors.Is(err, orders.ErrInvalidTransition) {
		t.Errorf("cancel on a completed order: err = %v", err)
	}
}

// moveCars returns a fresh copy of the world save in which the given cars sit
// on one section, one day later.
func moveCars(t *testing.T, keys []string, prefix, section int) *run8save.Snapshot {
	t.Helper()
	snap := parse(t, worldPath)
	snap.GameDate = snap.GameDate.Add(24 * time.Hour)
	n := 0
	for ti := range snap.Trains {
		for vi := range snap.Trains[ti].Vehicles {
			v := &snap.Trains[ti].Vehicles[vi]
			if slices.Contains(keys, v.Key()) {
				for k := range v.Trucks {
					v.Trucks[k].RoutePrefix, v.Trucks[k].Section = prefix, section
				}
				n++
			}
		}
	}
	if n != len(keys) {
		t.Fatalf("moved %d of %d cars", n, len(keys))
	}
	return snap
}

func TestCompletionDetectedFromLaterSnapshot(t *testing.T) {
	h := start(t)
	h.ingest(snapshotEvent(parse(t, worldPath), "WorldSave.xml", false))
	v := h.labelAllYard()
	o, err := h.eng.StartOrder("avail:ab10", "industry:BRR")
	if err != nil {
		t.Fatal(err)
	}
	keys := o.CarKeys()
	if len(keys) == 0 {
		t.Fatal("BRR line has no cars")
	}
	_ = v

	// Nothing has moved: the same world again leaves the order In Progress.
	same := parse(t, worldPath)
	same.GameDate = same.GameDate.Add(time.Hour)
	if v = h.ingest(snapshotEvent(same, "WorldSave.xml", false)); v.Active[0].State != orders.StateInProgress {
		t.Fatalf("state = %s", v.Active[0].State)
	}

	// The set-outs reach BRR.
	if err := h.eng.LabelSection(990, 7777, trackmap.Label{Kind: trackmap.KindIndustry, Industries: []string{"BRR"}}); err != nil {
		t.Fatal(err)
	}
	h.clock.Advance(time.Minute)
	v = h.ingest(snapshotEvent(moveCars(t, keys, 990, 7777), "WorldSave.xml", false))
	if len(v.Active) != 1 || v.Active[0].State != orders.StateCompletionDetected || !v.Active[0].DetectedAt.Equal(h.clock.Now()) {
		t.Fatalf("Active = %+v", v.Active)
	}
	if !slices.ContainsFunc(v.Activity, func(a store.ActivityEntry) bool {
		return a.Kind == "order" && strings.Contains(a.Message, "completion detected") && strings.Contains(a.Message, "AB10/BRR")
	}) {
		t.Errorf("no completion entry in the activity log: %+v", v.Activity[:3])
	}
	if len(v.History) != 0 {
		t.Error("a detected order must wait for the user to confirm")
	}

	// Continue: the same snapshot does not detect it again.
	if o, err = h.eng.ApplyOrder(o.ID, orders.ActContinue); err != nil || o.State != orders.StateInProgress {
		t.Fatalf("continue: %v %v", o.State, err)
	}
	if v = h.eng.View(); v.Active[0].State != orders.StateInProgress {
		t.Fatal("continue did not take effect")
	}
	repeat := moveCars(t, keys, 990, 7777)
	repeat.GameDate = repeat.GameDate.Add(time.Hour)
	if v = h.ingest(snapshotEvent(repeat, "WorldSave.xml", false)); v.Active[0].State != orders.StateInProgress {
		t.Fatalf("an identical snapshot re-detected completion: %s", v.Active[0].State)
	}

	// A car changes location (back to the yard), which lifts the suppression;
	// when the set-outs reach BRR again the order is detected.
	back := parse(t, worldPath)
	back.GameDate = back.GameDate.Add(2 * time.Hour)
	if v = h.ingest(snapshotEvent(back, "WorldSave.xml", false)); v.Active[0].State != orders.StateInProgress {
		t.Fatalf("state with the cars back in the yard = %s", v.Active[0].State)
	}
	again := moveCars(t, keys, 990, 7777)
	again.GameDate = again.GameDate.Add(3 * time.Hour)
	if v = h.ingest(snapshotEvent(again, "WorldSave.xml", false)); v.Active[0].State != orders.StateCompletionDetected {
		t.Fatalf("state after a car moved = %s", v.Active[0].State)
	}
	if _, err := h.eng.ApplyOrder(o.ID, orders.ActConfirm); err != nil {
		t.Fatal(err)
	}
	if v = h.eng.View(); len(v.Active) != 0 || len(v.History) != 1 || v.History[0].State != orders.StateCompleted {
		t.Fatalf("after confirm: Active=%d History=%d", len(v.Active), len(v.History))
	}
}

func TestStateSurvivesRestart(t *testing.T) {
	h := start(t)
	world := parse(t, worldPath)
	h.ingest(snapshotEvent(world, "WorldSave.xml", false))
	h.labelAllYard()
	a, _ := h.eng.StartOrder("avail:131", "")
	b, _ := h.eng.StartOrder("avail:ab10", "industry:FIN")
	if _, err := h.eng.ApplyOrder(b.ID, orders.ActCancel); err != nil {
		t.Fatal(err)
	}
	later := parse(t, worldPath)
	later.GameDate = later.GameDate.Add(48 * time.Hour)
	before := h.ingest(snapshotEvent(later, "WorldSave.xml", false))
	labels := len(before.TrackMap.Labels)
	if labels == 0 {
		t.Fatal("no labels were saved")
	}
	activity := len(before.Activity)

	h.open() // restart over the same data directory
	v := h.eng.View()
	if v.HasSnapshot || v.Status != nil {
		t.Fatal("a snapshot cannot survive a restart")
	}
	if len(v.Active) != 1 || v.Active[0].ID != a.ID || v.Active[0].State != orders.StateInProgress || len(v.Active[0].CarKeys()) != 68 {
		t.Fatalf("Active before first ingest = %+v", v.Active)
	}
	if len(v.History) != 1 || v.History[0].ID != b.ID || v.History[0].State != orders.StateCancelled {
		t.Fatalf("History = %+v", v.History)
	}
	if len(v.TrackMap.Labels) != labels || len(v.Locations) == 0 || len(v.Activity) != activity {
		t.Errorf("labels=%d locations=%d activity=%d", len(v.TrackMap.Labels), len(v.Locations), len(v.Activity))
	}

	// Dwell records came back: a car that has not moved keeps its first-seen date.
	third := parse(t, worldPath)
	third.GameDate = third.GameDate.Add(72 * time.Hour)
	v = h.ingest(snapshotEvent(third, "WorldSave.xml", false))
	for i := range v.Status.Cars {
		if c := &v.Status.Cars[i]; !c.DwellSince.Equal(world.GameDate) {
			t.Fatalf("car %s dwell since %v, want %v", c.Key, c.DwellSince, world.GameDate)
		}
	}
	if len(v.Active) != 1 || findOrder(v.Available, "avail:131") != nil {
		t.Error("the restored order should still reserve its cars")
	}
	next, err := h.eng.StartOrder("avail:ab10", "industry:FIN")
	if err != nil || next.ID != "ord-3" {
		t.Fatalf("next order id = %q (%v), want ord-3", next.ID, err)
	}
}

func TestViewsAreImmutable(t *testing.T) {
	h := start(t)
	h.ingest(snapshotEvent(parse(t, worldPath), "WorldSave.xml", false))
	h.labelAllYard()
	old := h.eng.View()
	snapshot := func(v *View) View {
		c := *v
		c.Available = slices.Clone(v.Available)
		c.Active = slices.Clone(v.Active)
		return c
	}
	want := snapshot(old)
	wantLabels := len(old.TrackMap.Labels)
	wantMin := old.Profile.RoadTrainBySymbol("108").MinLengthPct

	if _, err := h.eng.StartOrder("avail:108", ""); err != nil {
		t.Fatal(err)
	}
	if err := h.eng.SetThreshold("108", new(80.0), nil); err != nil {
		t.Fatal(err)
	}
	if err := h.eng.RemoveLabel(parseFirstLabel(t, old)); err != nil {
		t.Fatal(err)
	}
	if got := snapshot(old); !reflect.DeepEqual(got.Available, want.Available) || !reflect.DeepEqual(got.Active, want.Active) {
		t.Error("an old view's orders changed")
	}
	if len(old.TrackMap.Labels) != wantLabels {
		t.Error("an old view's track map changed")
	}
	if old.Profile.RoadTrainBySymbol("108").MinLengthPct != wantMin {
		t.Error("an old view's profile changed")
	}
	if len(old.Settings.Thresholds) != 0 {
		t.Error("an old view's settings changed")
	}
}

func parseFirstLabel(t *testing.T, v *View) (int, int) {
	t.Helper()
	for k := range v.TrackMap.Labels {
		p, s, err := trackmap.ParseKey(k)
		if err != nil {
			t.Fatal(err)
		}
		return p, s
	}
	t.Fatal("no labels")
	return 0, 0
}
