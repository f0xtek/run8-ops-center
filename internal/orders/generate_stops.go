package orders

import (
	"run8-ops-center/internal/carstatus"
	"run8-ops-center/internal/profile"
)

// stopRun tracks the train as it works its stops.
type stopRun struct {
	onBoard []*carstatus.Car // origin consist cars not yet set out
	ft      float64          // current train length
	tons    float64          // current train gross tons
	picked  []*carstatus.Car // every car picked up so far
	taken   map[string]bool  // cars already on the order
}

// runStops works the train's stops in order and returns the lines that have
// work, plus every pick-up. consist is the origin consist (nil for a through
// train). Stops with no work are omitted.
func (g *genCtx) runStops(t *profile.RoadTrain, consist []*carstatus.Car) ([]Line, []*carstatus.Car) {
	run := &stopRun{onBoard: append([]*carstatus.Car(nil), consist...), taken: genKeys(consist)}
	run.ft, run.tons = genSum(consist)

	var lines []Line
	for _, s := range t.Stops {
		var (
			w    genWork
			name string
		)
		switch s.Kind {
		case "yard":
			w, name = g.yardStopWork(s, t.Limit, run), g.yardName(s.Target)
		case "industry":
			w, name = g.industryStopWork(t, s, run), g.industryName(profile.NormTag(s.Target))
		default:
			continue
		}
		if w.empty() {
			continue
		}
		run.apply(w)
		lines = append(lines, genLine(LineStop, s.Target, name, w))
	}
	return lines, run.picked
}

// apply updates the train after a stop.
func (r *stopRun) apply(w genWork) {
	if len(w.setOuts) > 0 {
		out := genKeys(w.setOuts)
		kept := r.onBoard[:0:0]
		for _, c := range r.onBoard {
			if !out[c.Key] {
				kept = append(kept, c)
			}
		}
		r.onBoard = kept
		ft, tons := genSum(w.setOuts)
		r.ft -= ft
		r.tons -= tons
	}
	ft, tons := genSum(w.pickUps)
	r.ft += ft
	r.tons += tons
	r.picked = append(r.picked, w.pickUps...)
	for _, c := range w.pickUps {
		r.taken[c.Key] = true
	}
}

// yardStopWork finds a yard stop's work. Set-outs are consist cars matching
// the stop's set-out tags. Pick-ups are cars in that yard matching the pick-up
// tags and car types, oldest first, added only while the train after the stop
// stays within its limits.
func (g *genCtx) yardStopWork(s profile.Stop, lim profile.Limit, run *stopRun) genWork {
	var w genWork
	for _, c := range run.onBoard {
		if g.matches(c, s.SetOut) {
			w.setOuts = append(w.setOuts, c)
		}
	}
	ft, tons := run.ft, run.tons
	for _, c := range w.setOuts {
		ft -= c.LengthFt
		tons -= c.GrossTons
	}
	for _, c := range g.pool {
		if run.taken[c.Key] || !genInYard(c, s.Target) || !g.stopPicksUp(s, c) {
			continue
		}
		if !genFits(lim, ft+c.LengthFt, tons+c.GrossTons) {
			break
		}
		ft += c.LengthFt
		tons += c.GrossTons
		w.pickUps = append(w.pickUps, c)
	}
	return w
}

// industryStopWork finds an industry stop's work: the readiness gate, set-outs
// of industry-bound consist cars under capacity, and the ready pick-ups. The
// spec places no length limit on industry-stop pick-ups, so none is applied.
func (g *genCtx) industryStopWork(t *profile.RoadTrain, s profile.Stop, run *stopRun) genWork {
	tag := profile.NormTag(s.Target)
	var cands []*carstatus.Car
	for _, c := range run.onBoard {
		if genStopBound(c, t, tag) {
			cands = append(cands, c)
		}
	}
	w := g.industryWork(tag, cands)
	w.pickUps = g.dropTaken(w.pickUps, run)
	return w
}

// dropTaken removes cars already on the order.
func (g *genCtx) dropTaken(cars []*carstatus.Car, run *stopRun) []*carstatus.Car {
	var out []*carstatus.Car
	for _, c := range cars {
		if !run.taken[c.Key] {
			out = append(out, c)
		}
	}
	return out
}
