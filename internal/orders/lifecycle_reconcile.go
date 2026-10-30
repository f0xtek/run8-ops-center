package orders

import (
	"fmt"
	"sort"
	"strings"

	"run8-ops-center/internal/carstatus"
	"run8-ops-center/internal/profile"
)

// LocOffMap is the CarRef.Location of a car that is absent from the snapshot.
const LocOffMap = "off map"

// Reconcile refreshes per-car and per-line progress of an In Progress order
// against a new status and detects completion. Orders in any other state, and
// calls without a status or profile, return the order unchanged.
//
// Local orders: industry set-outs are done at their industry, yard-line
// set-outs once delivered to the satellite yard (in it, or at an industry
// served by a local homed there), pick-ups in the job's home yard. A local car
// absent from the snapshot is missing. Road orders: stop set-outs are done
// once delivered to their stop; every other car (consist, stop pick-ups) is
// done once absent. A road set-out that is absent before reaching its stop is
// missing; one that was delivered earlier stays done.
//
// The order moves to Completion Detected, with DetectedAt from the clock, only
// when every car is done and none is missing, and (after a "continue") the
// order's Fingerprint differs from SuppressFP. SuppressFP is cleared once the
// fingerprint differs. The input is not modified.
func Reconcile(o Order, st *carstatus.Status, p *profile.Profile, c Clock) Order {
	if o.State != StateInProgress || st == nil || p == nil {
		return o
	}
	o = lifecycleClone(o)
	r := reconciler{st: st, p: p}
	switch o.Kind {
	case KindRoad:
		r.road(&o)
	default:
		r.local(&o)
	}

	missing := r.missingNumbers(&o)
	o.Flags = filterFlags(o.Flags, func(f Flag) bool { return f.Code != FlagMissingCar })
	if len(missing) > 0 {
		o.Flags = append(o.Flags, Flag{
			Code:    FlagMissingCar,
			Message: fmt.Sprintf("%d car(s) missing from the latest save: %s", len(missing), strings.Join(missing, ", ")),
		})
	}

	fp := Fingerprint(o)
	if o.SuppressFP != "" && fp != o.SuppressFP {
		o.SuppressFP = ""
	}
	if o.SuppressFP == "" && len(missing) == 0 && orderDone(&o) {
		o.State = StateCompletionDetected
		o.DetectedAt = c.Real
	}
	return o
}

// orderDone reports whether the order has cars and all of them are done.
func orderDone(o *Order) bool {
	n := 0
	for _, r := range o.Consist {
		if !r.Done {
			return false
		}
		n++
	}
	for _, l := range o.Lines {
		if !l.Done {
			return false
		}
		n += len(l.SetOuts) + len(l.PickUps)
	}
	return n > 0
}

type reconciler struct {
	st *carstatus.Status
	p  *profile.Profile
}

// rule says how one car's progress is judged.
type rule struct {
	delivered  func(carstatus.Location) bool // nil: never done while on the map
	absentDone bool                          // done once absent from the snapshot
	sticky     bool                          // an absent car that was already done stays done
}

func (r *reconciler) car(key string) *carstatus.Car {
	if r.st.ByKey != nil {
		return r.st.ByKey[key]
	}
	for i := range r.st.Cars {
		if r.st.Cars[i].Key == key {
			return &r.st.Cars[i]
		}
	}
	return nil
}

// refresh updates one car ref from the status under the rule.
func (r *reconciler) refresh(ref *CarRef, ru rule) {
	car := r.car(ref.Key)
	if car == nil {
		ref.Location = LocOffMap
		switch {
		case ru.absentDone:
			ref.Done, ref.Missing = true, false
		case ru.sticky && ref.Done:
			ref.Missing = false
		default:
			ref.Done, ref.Missing = false, true
		}
		return
	}
	ref.Location = car.Loc.String()
	ref.Missing = false
	ref.Done = ru.delivered != nil && ru.delivered(car.Loc)
}

func atIndustry(tag string) func(carstatus.Location) bool {
	return func(l carstatus.Location) bool {
		if l.Kind != carstatus.LocIndustry {
			return false
		}
		for _, t := range l.Industries {
			if t == tag {
				return true
			}
		}
		return false
	}
}

func inYard(code string) func(carstatus.Location) bool {
	return func(l carstatus.Location) bool { return l.Kind == carstatus.LocYard && l.Yard == code }
}

// atYardOrServed is "delivered to a yard": in that yard, or at an industry
// served by a local job homed there.
func (r *reconciler) atYardOrServed(code string) func(carstatus.Location) bool {
	return func(l carstatus.Location) bool {
		if inYard(code)(l) {
			return true
		}
		if l.Kind != carstatus.LocIndustry {
			return false
		}
		for _, tag := range l.Industries {
			for _, j := range r.p.LocalJobsServing(tag) {
				if j.HomeYard == code {
					return true
				}
			}
		}
		return false
	}
}

func (r *reconciler) local(o *Order) {
	var home func(carstatus.Location) bool
	if job := r.p.LocalJobByID(o.JobID); job != nil {
		home = inYard(job.HomeYard)
	}
	for i := range o.Lines {
		l := &o.Lines[i]
		var setOut func(carstatus.Location) bool
		if l.Kind == LineYard {
			setOut = r.atYardOrServed(l.Target)
		} else {
			setOut = atIndustry(l.Target)
		}
		for j := range l.SetOuts {
			r.refresh(&l.SetOuts[j], rule{delivered: setOut})
		}
		for j := range l.PickUps {
			r.refresh(&l.PickUps[j], rule{delivered: home})
		}
		l.Done = allDone(l.SetOuts) && allDone(l.PickUps)
	}
}

// stopDelivered is the delivered test for a road stop line.
func (r *reconciler) stopDelivered(train *profile.RoadTrain, l *Line) func(carstatus.Location) bool {
	kind := ""
	if train != nil {
		for _, s := range train.Stops {
			if s.Target == l.Target {
				kind = s.Kind
				break
			}
		}
	}
	if kind == "" {
		if r.p.YardByCode(l.Target) != nil {
			kind = "yard"
		} else {
			kind = "industry"
		}
	}
	if kind == "yard" {
		return r.atYardOrServed(l.Target)
	}
	return atIndustry(l.Target)
}

func (r *reconciler) road(o *Order) {
	train := r.p.RoadTrainBySymbol(o.JobID)
	if train == nil {
		train = r.p.RoadTrainBySymbol(o.Symbol)
	}
	setOutRule := map[string]rule{}
	for i := range o.Lines {
		l := &o.Lines[i]
		ru := rule{delivered: r.stopDelivered(train, l), sticky: true}
		for _, ref := range l.SetOuts {
			if _, ok := setOutRule[ref.Key]; !ok {
				setOutRule[ref.Key] = ru
			}
		}
	}
	other := rule{absentDone: true}
	judge := func(ref *CarRef) {
		if ru, ok := setOutRule[ref.Key]; ok {
			r.refresh(ref, ru)
		} else {
			r.refresh(ref, other)
		}
	}
	for j := range o.Consist {
		judge(&o.Consist[j])
	}
	for i := range o.Lines {
		l := &o.Lines[i]
		for j := range l.SetOuts {
			judge(&l.SetOuts[j])
		}
		for j := range l.PickUps {
			r.refresh(&l.PickUps[j], other)
		}
		l.Done = allDone(l.SetOuts) && allDone(l.PickUps)
	}
}

func allDone(rs []CarRef) bool {
	for _, r := range rs {
		if !r.Done {
			return false
		}
	}
	return true
}

// missingNumbers lists the numbers of missing cars, once each, sorted.
func (r *reconciler) missingNumbers(o *Order) []string {
	seen := map[string]bool{}
	var out []string
	add := func(rs []CarRef) {
		for _, ref := range rs {
			if ref.Missing && !seen[ref.Key] {
				seen[ref.Key] = true
				n := ref.Number
				if n == "" {
					n = ref.Key
				}
				out = append(out, n)
			}
		}
	}
	add(o.Consist)
	for _, l := range o.Lines {
		add(l.SetOuts)
		add(l.PickUps)
	}
	sort.Strings(out)
	return out
}
