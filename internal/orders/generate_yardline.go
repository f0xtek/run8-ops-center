package orders

import (
	"run8-ops-center/internal/carstatus"
	"run8-ops-center/internal/profile"
)

// yardLineWork finds the work of one yard line.
//
// Set-outs are cars in the job's home yard whose local symbol, industry tag or
// class code is carried by the line (no capacity limit), skipping cars already
// placed on an industry line of the same order. Pick-ups are the train-bound
// or industry-bound cars in the satellite yard that do not belong there.
func (g *genCtx) yardLineWork(job *profile.LocalJob, yl profile.YardLine, claimed map[string]bool) genWork {
	var w genWork
	for _, c := range g.pool {
		switch {
		case genInYard(c, job.HomeYard) && !claimed[c.Key] && g.matches(c, yl.Carries):
			w.setOuts = append(w.setOuts, c)
		case genInYard(c, yl.Yard) && g.isBound(c) && !g.belongsAt(yl, c):
			w.pickUps = append(w.pickUps, c)
		}
	}
	return w
}

// isBound reports whether the car is industry-bound or train-bound.
func (g *genCtx) isBound(c *carstatus.Car) bool {
	return c.Dest.Kind == carstatus.DestIndustry || c.Dest.Kind == carstatus.DestTrain
}

// belongsAt reports whether a car in the line's satellite yard belongs there:
// its tags are carried by the yard line, it is industry-bound for a job homed
// at that yard, or a road train stop at that yard picks it up.
func (g *genCtx) belongsAt(yl profile.YardLine, c *carstatus.Car) bool {
	if g.matches(c, yl.Carries) {
		return true
	}
	if c.Dest.Kind == carstatus.DestIndustry && g.yardHomed[yl.Yard][profile.NormTag(c.Dest.Industry)] {
		return true
	}
	for _, s := range g.yardStops[yl.Yard] {
		if g.stopPicksUp(s, c) {
			return true
		}
	}
	return false
}

// stopPicksUp reports whether a stop's pick-up tags and car types claim the
// car.
func (g *genCtx) stopPicksUp(s profile.Stop, c *carstatus.Car) bool {
	return g.matches(c, s.PickUp) && genTypeAllowed(s.PickUpCarTypes, c.Type)
}
