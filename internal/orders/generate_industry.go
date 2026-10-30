package orders

import (
	"run8-ops-center/internal/carstatus"
	"run8-ops-center/internal/profile"
)

// industryReady is the readiness gate: an industry is ready when no car at it
// is in the working state. An industry with no cars counts as ready. Reserved
// cars still count: cars an in-progress order has just spotted are working and
// must block a second delivery.
func (g *genCtx) industryReady(tag string) bool {
	for i := range g.st.Cars {
		c := &g.st.Cars[i]
		if c.Service == carstatus.ServiceWorking && genAtIndustry(c, tag) {
			return false
		}
	}
	return true
}

// readyAt returns the cars at the industry in the ready-to-pull state.
func (g *genCtx) readyAt(tag string) []*carstatus.Car {
	var out []*carstatus.Car
	for _, c := range g.pool {
		if c.Service == carstatus.ServiceReadyToPull && genAtIndustry(c, tag) {
			out = append(out, c)
		}
	}
	return out
}

// capSetOuts picks the cars an industry can take from cands, which must be in
// oldest-dwell-first order.
//
// Capacity is the industry's per-type capacity, applied literally as the spec
// words it: cars already sitting at the industry are not subtracted, because
// the readiness gate guarantees none is still being worked and the ready ones
// are pulled by the same line. A type the industry does not take gets no
// set-outs. A car of unknown type counts against the industry's total
// capacity, together with the typed set-outs already chosen.
func (g *genCtx) capSetOuts(tag string, cands []*carstatus.Car) []*carstatus.Car {
	ind := g.p.IndustryByTag(tag)
	if ind == nil {
		return nil
	}
	total := ind.TotalCapacity()
	perType := map[profile.CarType]int{}
	var out []*carstatus.Car
	for _, c := range cands {
		if len(out) >= total {
			break
		}
		if c.Type == profile.Unknown || c.Type == "" {
			out = append(out, c)
			continue
		}
		if limit, ok := ind.Capacity(c.Type); ok && perType[c.Type] < limit {
			perType[c.Type]++
			out = append(out, c)
		}
	}
	return out
}

// industryName is the display name of an industry tag.
func (g *genCtx) industryName(tag string) string {
	if ind := g.p.IndustryByTag(tag); ind != nil && ind.Name != "" {
		return ind.Name
	}
	return tag
}

// industryWork applies the readiness gate, picks set-outs from cands under
// capacity and collects the ready pick-ups. A gated industry has no work.
func (g *genCtx) industryWork(tag string, cands []*carstatus.Car) genWork {
	if !g.industryReady(tag) {
		return genWork{}
	}
	return genWork{setOuts: g.capSetOuts(tag, cands), pickUps: g.readyAt(tag)}
}
