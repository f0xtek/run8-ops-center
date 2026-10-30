package orders

import (
	"sort"
	"time"

	"run8-ops-center/internal/carstatus"
	"run8-ops-center/internal/profile"
)

// genReservedKeys returns the keys of every car on a reserved order.
func genReservedKeys(active []Order) map[string]bool {
	out := map[string]bool{}
	for i := range active {
		if !active[i].Reserved() {
			continue
		}
		for _, k := range active[i].CarKeys() {
			out[k] = true
		}
	}
	return out
}

// genCtx is the per-call state shared by every generation step.
type genCtx struct {
	p        *profile.Profile
	st       *carstatus.Status
	reserved map[string]bool
	// pool holds the cars eligible for any order, oldest dwell first: not
	// reserved, not on an unmapped section, with a destination that is not
	// foreign or unknown.
	pool []*carstatus.Car
	now  time.Time // in-game time of the snapshot; zero when unknown
	// yardHomed maps a yard code to the industry tags served by local jobs
	// homed there.
	yardHomed map[string]map[string]bool
	// yardStops maps a yard code to the yard stops of every road train there.
	yardStops map[string][]profile.Stop
}

func newGenCtx(in GenInput) *genCtx {
	g := &genCtx{
		p:         in.Profile,
		st:        in.Status,
		reserved:  genReservedKeys(in.Active),
		yardHomed: map[string]map[string]bool{},
		yardStops: map[string][]profile.Stop{},
	}
	if in.Status.Snapshot != nil {
		g.now = in.Status.Snapshot.GameDate
	}
	for i := range in.Status.Cars {
		c := &in.Status.Cars[i]
		if g.eligible(c) {
			g.pool = append(g.pool, c)
		}
	}
	sort.SliceStable(g.pool, func(i, j int) bool { return genOlder(g.pool[i], g.pool[j]) })

	for i := range in.Profile.LocalJobs {
		j := &in.Profile.LocalJobs[i]
		set := g.yardHomed[j.HomeYard]
		if set == nil {
			set = map[string]bool{}
			g.yardHomed[j.HomeYard] = set
		}
		for _, t := range j.Industries {
			set[profile.NormTag(t)] = true
		}
	}
	for i := range in.Profile.RoadTrains {
		for _, s := range in.Profile.RoadTrains[i].Stops {
			if s.Kind == "yard" {
				g.yardStops[s.Target] = append(g.yardStops[s.Target], s)
			}
		}
	}
	return g
}

// eligible reports whether a car may appear on any order.
func (g *genCtx) eligible(c *carstatus.Car) bool {
	if g.reserved[c.Key] || c.Loc.Kind == carstatus.LocUnmapped {
		return false
	}
	return c.Dest.Kind != carstatus.DestForeign && c.Dest.Kind != carstatus.DestUnknown
}

// genOlder orders cars oldest dwell first, ties broken by key.
func genOlder(a, b *carstatus.Car) bool {
	if !a.DwellSince.Equal(b.DwellSince) {
		return a.DwellSince.Before(b.DwellSince)
	}
	return a.Key < b.Key
}
