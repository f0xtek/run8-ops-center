package orders

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrInvalidTransition is returned (wrapped) when an action is not allowed in
// the order's current state, or when Start is given an order that is not
// Available.
var ErrInvalidTransition = errors.New("invalid order transition")

// FlagMissingCar is the Flag.Code that Reconcile sets while a car is missing.
const FlagMissingCar = "missing-car"

// transitions is the lifecycle transition table for Apply. Start is not in it:
// it turns an Available order into a new started order and is handled by
// Start. Every state/action pair absent from the table is rejected.
var transitions = map[State]map[Action]State{
	StateInProgress: {
		ActComplete: StateCompleted,
		ActCancel:   StateCancelled,
	},
	StateCompletionDetected: {
		ActConfirm:  StateCompleted,
		ActContinue: StateInProgress,
	},
}

func invalidTransition(s State, a Action) error {
	return fmt.Errorf("%w: cannot %s an order that is %s", ErrInvalidTransition, a, s)
}

// Apply performs confirm, continue, complete or cancel on a started order and
// returns the updated copy. Finishing (Completed or Cancelled) stamps
// FinishedAt and GameFinished from the clock and keeps the order: a finished
// order is its own history record. Continue records the current Fingerprint in
// SuppressFP so Reconcile does not detect completion again until a car moves.
// Any other state/action pair returns an error wrapping ErrInvalidTransition.
func Apply(o Order, a Action, c Clock) (Order, error) {
	next, ok := transitions[o.State][a]
	if !ok {
		return Order{}, invalidTransition(o.State, a)
	}
	o = lifecycleClone(o)
	o.State = next
	switch next {
	case StateCompleted, StateCancelled:
		o.FinishedAt = c.Real
		o.GameFinished = c.Game
	case StateInProgress: // continue
		o.SuppressFP = Fingerprint(o)
		o.DetectedAt = time.Time{}
	}
	return o, nil
}

// Start turns an Available order into a started (In Progress) order with the
// ID newID and StartedAt/GameStarted taken from the clock. lineKey == ""
// starts the whole order. Otherwise lineKey is a Line.Key() of a local order
// and only that industry or yard line is started, as a new order that keeps
// the job and symbol and whose Totals cover that line's set-out cars. Road
// orders can only be started whole. The input is not modified.
//
// A started order reserves its cars, so Shared and the shared-cars flag are
// cleared; Flags other than over-length, over-tons and unmapped are dropped
// from a single-line order when they no longer apply to its cars.
func Start(avail Order, lineKey string, newID string, c Clock) (Order, error) {
	if avail.State != StateAvailable {
		return Order{}, invalidTransition(avail.State, ActStart)
	}
	if newID == "" {
		return Order{}, errors.New("start order: empty order ID")
	}
	o := lifecycleClone(avail)
	if lineKey != "" {
		if o.Kind != KindLocal {
			return Order{}, fmt.Errorf("start order %s: %s orders can only be started whole", avail.ID, o.Kind)
		}
		idx := -1
		for i := range o.Lines {
			if o.Lines[i].Key() == lineKey {
				idx = i
				break
			}
		}
		if idx < 0 {
			return Order{}, fmt.Errorf("start order %s: unknown line %q", avail.ID, lineKey)
		}
		narrowToLine(&o, idx)
	}
	o.Shared = nil
	o.Flags = filterFlags(o.Flags, func(f Flag) bool {
		return f.Code != "shared-cars" && f.Code != FlagMissingCar
	})
	resetProgress(&o)
	o.ID = newID
	o.State = StateInProgress
	o.SuppressFP = ""
	o.StartedAt = c.Real
	o.GameStarted = c.Game
	o.DetectedAt = time.Time{}
	o.FinishedAt = time.Time{}
	o.GameFinished = time.Time{}
	return o, nil
}

// narrowToLine reduces a local order to its idx-th line, recomputing totals
// from that line's set-out cars and re-evaluating the totals-based flags.
func narrowToLine(o *Order, idx int) {
	line := o.Lines[idx]
	old := o.Totals
	var t Totals
	t.MaxLengthFt, t.MaxTons = old.MaxLengthFt, old.MaxTons
	for _, r := range line.SetOuts {
		t.LengthFt += r.LengthFt
		t.GrossTons += r.GrossTons
	}
	if old.GrossTons > 0 {
		t.RequiredHP = t.GrossTons * old.RequiredHP / old.GrossTons
	}
	over := func(f Flag) bool {
		switch f.Code {
		case "over-length":
			return t.MaxLengthFt > 0 && t.LengthFt > float64(t.MaxLengthFt)
		case "over-tons":
			return t.MaxTons > 0 && t.GrossTons > float64(t.MaxTons)
		case "unmapped":
			for _, r := range append(append([]CarRef(nil), line.SetOuts...), line.PickUps...) {
				if strings.HasPrefix(r.Location, "unmapped") {
					return true
				}
			}
			return false
		}
		return false
	}
	o.Flags = filterFlags(o.Flags, over)
	o.Consist = nil
	o.Lines = []Line{line}
	o.Totals = t
}

func filterFlags(in []Flag, keep func(Flag) bool) []Flag {
	var out []Flag
	for _, f := range in {
		if keep(f) {
			out = append(out, f)
		}
	}
	return out
}

// resetProgress clears per-car and per-line progress on a freshly started order.
func resetProgress(o *Order) {
	zero := func(rs []CarRef) {
		for i := range rs {
			rs[i].Done, rs[i].Missing = false, false
		}
	}
	zero(o.Consist)
	for i := range o.Lines {
		zero(o.Lines[i].SetOuts)
		zero(o.Lines[i].PickUps)
		o.Lines[i].Done = false
	}
}

// Fingerprint hashes every car's key and location text, in CarKeys order. It
// changes exactly when a car of the order changes location (or leaves the map),
// which is what lets "continue" suppress re-detection on an unchanged world.
func Fingerprint(o Order) string {
	loc := map[string]string{}
	note := func(rs []CarRef) {
		for _, r := range rs {
			if _, ok := loc[r.Key]; !ok {
				loc[r.Key] = r.Location
			}
		}
	}
	note(o.Consist)
	for _, l := range o.Lines {
		note(l.SetOuts)
		note(l.PickUps)
	}
	h := sha256.New()
	for _, k := range o.CarKeys() {
		fmt.Fprintf(h, "%d:%s\x00%d:%s\x00", len(k), k, len(loc[k]), loc[k])
	}
	return hex.EncodeToString(h.Sum(nil)[:16])
}

// lifecycleClone deep-copies the slices of an order so a returned order never
// aliases the caller's.
func lifecycleClone(o Order) Order {
	o.Consist = append([]CarRef(nil), o.Consist...)
	if o.Lines != nil {
		lines := make([]Line, len(o.Lines))
		for i, l := range o.Lines {
			l.SetOuts = append([]CarRef(nil), l.SetOuts...)
			l.PickUps = append([]CarRef(nil), l.PickUps...)
			lines[i] = l
		}
		o.Lines = lines
	}
	o.Flags = append([]Flag(nil), o.Flags...)
	o.Shared = append([]string(nil), o.Shared...)
	return o
}
