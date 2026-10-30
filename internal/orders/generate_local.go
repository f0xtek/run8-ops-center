package orders

import (
	"fmt"

	"run8-ops-center/internal/carstatus"
	"run8-ops-center/internal/profile"
)

// localOrder builds the available order for one local job: an industry line
// per eligible industry, then a yard line per yard line with work. It returns
// false when the job has no work.
func (g *genCtx) localOrder(job *profile.LocalJob) (Order, bool) {
	var lines []Line
	claimed := map[string]bool{} // set-outs already placed on a line of this order
	for _, tag := range job.Industries {
		tag = profile.NormTag(tag)
		w := g.industryWork(tag, g.homeYardBound(job.HomeYard, tag))
		if w.empty() {
			continue
		}
		for _, c := range w.setOuts {
			claimed[c.Key] = true
		}
		lines = append(lines, genLine(LineIndustry, tag, g.industryName(tag), w))
	}
	for _, yl := range job.YardLines {
		w := g.yardLineWork(job, yl, claimed)
		if w.empty() {
			continue
		}
		for _, c := range w.setOuts {
			claimed[c.Key] = true
		}
		lines = append(lines, genLine(LineYard, yl.Yard, g.yardName(yl.Yard), w))
	}
	if len(lines) == 0 {
		return Order{}, false
	}
	o := Order{
		ID:      "avail:" + job.ID,
		Kind:    KindLocal,
		JobID:   job.ID,
		Symbol:  job.Symbol,
		Profile: g.p.ID,
		State:   StateAvailable,
		Lines:   lines,
	}
	g.localTotals(&o, job.Limit)
	return o, true
}

// homeYardBound returns the pool cars in the yard that are industry-bound for
// the industry tag, oldest first.
func (g *genCtx) homeYardBound(yard, tag string) []*carstatus.Car {
	var out []*carstatus.Car
	for _, c := range g.pool {
		if genInYard(c, yard) && genIndustryBound(c, tag) {
			out = append(out, c)
		}
	}
	return out
}

// localTotals sums the outbound train (every set-out, once) and flags the
// order when it exceeds the job's limits.
func (g *genCtx) localTotals(o *Order, lim profile.Limit) {
	seen := map[string]bool{}
	var ft, tons float64
	for _, l := range o.Lines {
		for _, r := range l.SetOuts {
			if seen[r.Key] {
				continue
			}
			seen[r.Key] = true
			ft += r.LengthFt
			tons += r.GrossTons
		}
	}
	o.Totals = Totals{
		LengthFt:    ft,
		GrossTons:   tons,
		RequiredHP:  tons * lim.MinHPT,
		MaxLengthFt: lim.MaxLengthFt,
		MaxTons:     lim.MaxTons,
	}
	if lim.MaxLengthFt > 0 && ft > float64(lim.MaxLengthFt) {
		o.Flags = append(o.Flags, Flag{
			Code:    "over-length",
			Message: fmt.Sprintf("Set-outs total %.0f ft against a %d ft limit", ft, lim.MaxLengthFt),
		})
	}
	if lim.MaxTons > 0 && tons > float64(lim.MaxTons) {
		o.Flags = append(o.Flags, Flag{
			Code:    "over-tons",
			Message: fmt.Sprintf("Set-outs total %.0f tons against a %d ton limit", tons, lim.MaxTons),
		})
	}
}

// yardName is the display name of a yard code.
func (g *genCtx) yardName(code string) string {
	if y := g.p.YardByCode(code); y != nil && y.Name != "" {
		return y.Name
	}
	return code
}
