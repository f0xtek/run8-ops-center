package orders

import (
	"run8-ops-center/internal/carstatus"
	"run8-ops-center/internal/profile"
)

// genWork is the set-outs and pick-ups of one line, before conversion to
// CarRefs.
type genWork struct {
	setOuts []*carstatus.Car
	pickUps []*carstatus.Car
}

func (w genWork) empty() bool { return len(w.setOuts) == 0 && len(w.pickUps) == 0 }

// genRef captures a car for an order.
func genRef(c *carstatus.Car) CarRef {
	return CarRef{
		Key:       c.Key,
		Number:    c.Number,
		Type:      c.Type,
		Loaded:    c.Loaded,
		Tag:       c.Dest.Tag,
		Location:  c.Loc.String(),
		LengthFt:  c.LengthFt,
		GrossTons: c.GrossTons,
	}
}

func genRefs(cars []*carstatus.Car) []CarRef {
	if len(cars) == 0 {
		return nil
	}
	out := make([]CarRef, len(cars))
	for i, c := range cars {
		out[i] = genRef(c)
	}
	return out
}

// genLine builds a line from its work.
func genLine(kind LineKind, target, name string, w genWork) Line {
	return Line{Kind: kind, Target: target, Name: name, SetOuts: genRefs(w.setOuts), PickUps: genRefs(w.pickUps)}
}

// genSum totals length and gross tons.
func genSum(cars []*carstatus.Car) (ft, tons float64) {
	for _, c := range cars {
		ft += c.LengthFt
		tons += c.GrossTons
	}
	return ft, tons
}

// genInYard reports whether the car sits in the yard.
func genInYard(c *carstatus.Car, yard string) bool {
	return c.Loc.Kind == carstatus.LocYard && c.Loc.Yard == yard
}

// genAtIndustry reports whether the car sits at the industry.
func genAtIndustry(c *carstatus.Car, tag string) bool {
	if c.Loc.Kind != carstatus.LocIndustry {
		return false
	}
	for _, t := range c.Loc.Industries {
		if profile.NormTag(t) == tag {
			return true
		}
	}
	return false
}

// genIndustryBound reports whether the car is industry-bound for tag.
func genIndustryBound(c *carstatus.Car, tag string) bool {
	return c.Dest.Kind == carstatus.DestIndustry && profile.NormTag(c.Dest.Industry) == tag
}

// genTags lists the normalised tags a car can be matched by in a tag list
// (yard-line carries, stop set-out and pick-up tags): for an industry-bound
// car its local symbol, industry tag and full tag; for a train-bound car its
// tag, the alias class code of that tag and, for off-map industries, the
// industry tag. Other cars have none, so they never match a tag list.
func (g *genCtx) genTags(c *carstatus.Car) []string {
	d := c.Dest
	var out []string
	add := func(s string) {
		if s = profile.NormTag(s); s != "" {
			out = append(out, s)
		}
	}
	switch d.Kind {
	case carstatus.DestIndustry:
		add(d.Local)
		add(d.Industry)
		add(d.Tag)
	case carstatus.DestTrain:
		add(d.Tag)
		if a, ok := g.p.Aliases[profile.NormTag(d.Tag)]; ok {
			add(a)
		}
		add(d.Industry)
	}
	return out
}

// matches reports whether any car tag equals any wanted tag.
func (g *genCtx) matches(c *carstatus.Car, wanted []string) bool {
	if len(wanted) == 0 {
		return false
	}
	for _, t := range g.genTags(c) {
		for _, w := range wanted {
			if profile.NormTag(w) == t {
				return true
			}
		}
	}
	return false
}

// genTypeAllowed reports whether the type passes a stop's car-type filter; an
// empty filter allows every type.
func genTypeAllowed(types []profile.CarType, t profile.CarType) bool {
	if len(types) == 0 {
		return true
	}
	for _, x := range types {
		if x == t {
			return true
		}
	}
	return false
}

// genKeys returns the set of car keys.
func genKeys(cars []*carstatus.Car) map[string]bool {
	out := make(map[string]bool, len(cars))
	for _, c := range cars {
		out[c.Key] = true
	}
	return out
}

// genStopBound reports whether the car is a set-out for an industry stop of
// train t: industry-bound for the industry, or train-bound for t itself with
// that industry (the `<road train symbol> <industry>` tag form, e.g. `A28 PHS`,
// which classifies as train-bound). Local industry lines use genIndustryBound.
func genStopBound(c *carstatus.Car, t *profile.RoadTrain, tag string) bool {
	if genIndustryBound(c, tag) {
		return true
	}
	return c.Dest.Kind == carstatus.DestTrain &&
		profile.NormTag(c.Dest.Industry) == tag &&
		profile.NormTag(c.Dest.Train) == profile.NormTag(t.Symbol)
}
