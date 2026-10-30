package engine

import (
	"fmt"
	"path/filepath"
	"reflect"
	"slices"
	"time"

	"run8-ops-center/internal/carstatus"
	"run8-ops-center/internal/orders"
	"run8-ops-center/internal/run8save"
	"run8-ops-center/internal/store"
	"run8-ops-center/internal/watch"
)

// ingest handles one watcher event on the engine goroutine.
func (e *Engine) ingest(ev watch.Event) {
	if ev.Err != nil {
		// The previous snapshot, status and orders stay as they are.
		e.addWarning("parse", fmt.Sprintf("%s could not be read, keeping the previous save: %v", baseName(ev.Path), ev.Err))
		e.publish()
		return
	}
	if ev.Warning != "" {
		kind := "ingest"
		if ev.Auto {
			kind = "autosave"
		}
		e.addWarning(kind, ev.Warning)
	}
	if ev.Snapshot == nil {
		if ev.Warning != "" {
			e.publish()
		}
		return
	}

	prev := e.snap
	e.snap = ev.Snapshot
	e.source.ActiveFile = baseName(ev.Path)
	e.source.ActiveAuto = ev.Auto
	e.source.LastIngest = e.now()
	e.source.GameDate = ev.Snapshot.GameDate
	e.dropWarnings("parse") // the file that failed has been superseded by a good one

	if prev == nil {
		e.logf("ingest", "Loaded %s: %d cars", e.source.ActiveFile, len(ev.Snapshot.Cars()))
	} else {
		e.logSnapshotChanges(prev, ev.Snapshot)
	}
	e.recompute(true)
	e.persistAndPublish()
}

// recompute rebuilds everything derived from the last snapshot, the active
// profile and the track map: status, order progress (when reconcile is set),
// available orders, learn-mode candidates, locations and coverage. It does
// not persist or publish.
func (e *Engine) recompute(reconcile bool) {
	e.locations = e.tm.Locations()
	if e.snap == nil {
		e.status, e.avail, e.building, e.candidates = nil, nil, nil, nil
		e.coverage = e.tm.Coverage(nil)
		return
	}
	id := e.base.ID
	prevDwell := e.state.Dwell[id]
	st, dwell := carstatus.Derive(e.snap, e.eff, e.tm, prevDwell)
	if len(dwell) != len(prevDwell) || !reflect.DeepEqual(dwell, prevDwell) {
		e.state.Dwell[id] = dwell
		e.stateDirty = true
	}
	e.status = st

	if reconcile {
		list := e.orders()
		changed := false
		for i, o := range list {
			if o.State != orders.StateInProgress {
				continue
			}
			n := orders.Reconcile(o, st, e.eff, e.clock())
			if reflect.DeepEqual(o, n) {
				continue
			}
			if !changed {
				list = slices.Clone(list)
				changed = true
			}
			list[i] = n
			e.logReconcile(o, n)
		}
		if changed {
			e.setOrders(list)
		}
	}

	gen := orders.Generate(orders.GenInput{Profile: e.eff, Status: st, Active: e.orders()})
	e.avail, e.building = gen.Orders, gen.Building
	e.candidates = e.tm.Candidates(e.snap, e.eff)
	e.coverage = e.tm.Coverage(e.snap)
}

func (e *Engine) logReconcile(old, n orders.Order) {
	if old.State != n.State && n.State == orders.StateCompletionDetected {
		e.logf("order", "%s: completion detected, confirm or continue", orderLabel(n))
	}
	if hasFlag(n, orders.FlagMissingCar) && !hasFlag(old, orders.FlagMissingCar) {
		e.logf("order", "%s: a car is missing from the latest save", orderLabel(n))
	}
}

func hasFlag(o orders.Order, code string) bool {
	return slices.ContainsFunc(o.Flags, func(f orders.Flag) bool { return f.Code == code })
}

// orderLabel names an order for the activity log: "AB45/HTI" for a single
// line, otherwise the symbol.
func orderLabel(o orders.Order) string {
	name := o.Symbol
	if name == "" {
		name = o.JobID
	}
	if o.Kind == orders.KindLocal && len(o.Lines) == 1 {
		name += "/" + o.Lines[0].Target
	}
	return o.ID + " " + name
}

// logSnapshotChanges records appeared, disappeared and retagged cars one by
// one and moved cars once per train.
func (e *Engine) logSnapshotChanges(prev, cur *run8save.Snapshot) {
	changes := carstatus.Diff(prev, cur)
	if len(changes) == 0 {
		return
	}
	loco := map[string]bool{}
	symbol := map[string]string{}
	for _, t := range cur.Trains {
		symbol[t.ID] = t.Symbol()
		for _, v := range t.Vehicles {
			if v.IsLoco() {
				loco[v.Key()] = true
			}
		}
	}
	tag := func(s string) string {
		if s == "" {
			return "no tag"
		}
		return s
	}
	var movedOrder []string
	moved := map[string]int{}
	for _, c := range changes {
		switch c.Kind {
		case carstatus.ChangeAppeared:
			e.logf("appeared", "%s appeared on train %s (%s)", c.Number, c.TrainID, tag(c.NewTag))
		case carstatus.ChangeDisappeared:
			e.logf("disappeared", "%s left the map (was %s)", c.Number, tag(c.OldTag))
		case carstatus.ChangeRetagged:
			e.logf("retagged", "%s retagged from %s to %s", c.Number, tag(c.OldTag), tag(c.NewTag))
		case carstatus.ChangeMoved:
			if loco[c.Key] {
				continue
			}
			if _, ok := moved[c.TrainID]; !ok {
				movedOrder = append(movedOrder, c.TrainID)
			}
			moved[c.TrainID]++
		}
	}
	for _, id := range movedOrder {
		who := "train " + id
		if s := symbol[id]; s != "" {
			who += " (" + s + ")"
		}
		n := moved[id]
		e.logf("moved", "%d %s moved with %s", n, plural(n, "car", "cars"), who)
	}
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// logf appends an activity entry for the current step.
func (e *Engine) logf(kind, format string, args ...any) {
	e.pending = append(e.pending, store.ActivityEntry{
		Time:    e.now(),
		Game:    e.gameDate(),
		Kind:    kind,
		Message: fmt.Sprintf(format, args...),
	})
}

func (e *Engine) gameDate() time.Time {
	if e.snap == nil {
		return time.Time{}
	}
	return e.snap.GameDate
}

func (e *Engine) clock() orders.Clock {
	return orders.Clock{Real: e.now(), Game: e.gameDate()}
}

// orders returns the active profile's non-available orders, oldest first.
// Orders of other profiles stay in the state but are not visible here.
func (e *Engine) orders() []orders.Order { return e.state.ProfileOrders[e.base.ID] }

func (e *Engine) setOrders(list []orders.Order) {
	e.state.ProfileOrders[e.base.ID] = list
	e.stateDirty = true
}

// addWarning puts a warning at the front, replacing an identical older one,
// and keeps at most maxWarnings.
func (e *Engine) addWarning(kind, msg string) {
	out := make([]Warning, 0, min(len(e.warnings)+1, maxWarnings))
	out = append(out, Warning{Time: e.now(), Kind: kind, Message: msg})
	for _, w := range e.warnings {
		if len(out) == maxWarnings {
			break
		}
		if w.Kind == kind && w.Message == msg {
			continue
		}
		out = append(out, w)
	}
	e.warnings = out
}

func (e *Engine) dropWarnings(kind string) {
	if !slices.ContainsFunc(e.warnings, func(w Warning) bool { return w.Kind == kind }) {
		return
	}
	e.warnings = slices.DeleteFunc(slices.Clone(e.warnings), func(w Warning) bool { return w.Kind == kind })
}

// persist writes state.json when something persistent changed in this step.
func (e *Engine) persist() error {
	if len(e.pending) > 0 {
		e.state.AppendActivity(e.pending...)
		e.pending = nil
		e.stateDirty = true
	}
	if !e.stateDirty {
		return nil
	}
	if err := store.SaveState(e.guard, e.dataDir, e.state); err != nil {
		return fmt.Errorf("save state: %w", err)
	}
	e.stateDirty = false
	return nil
}

// persistAndPublish ends an ingest step. A failed save becomes a warning.
func (e *Engine) persistAndPublish() {
	if err := e.persist(); err != nil {
		e.addWarning("ingest", err.Error())
	}
	e.publish()
}

func baseName(path string) string {
	if path == "" {
		return ""
	}
	return filepath.Base(path)
}

// tiles counts the dashboard tiles from a status.
func tiles(st *carstatus.Status) Tiles {
	if st == nil {
		return Tiles{}
	}
	t := Tiles{OnRoute: len(st.Cars), Unmapped: st.Unmapped, Unknown: st.Unknown}
	for i := range st.Cars {
		c := &st.Cars[i]
		if c.Loc.Kind == carstatus.LocIndustry {
			t.AtIndustries++
		}
		switch c.Service {
		case carstatus.ServiceReadyToPull:
			t.ReadyToPull++
		case carstatus.ServiceAwaitingDelivery:
			t.AwaitingDelivery++
		case carstatus.ServiceAwaitingTrain:
			t.AwaitingTrain++
		}
	}
	return t
}
