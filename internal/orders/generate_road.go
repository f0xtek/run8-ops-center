package orders

import (
	"time"

	"run8-ops-center/internal/carstatus"
	"run8-ops-center/internal/profile"
)

// thresholdEpsilon absorbs float rounding when comparing lengths to a
// percentage of the maximum.
const thresholdEpsilon = 1e-9

// roadOrders builds the road train orders: trains with an origin first, then
// through trains. It also returns a RoadBuild entry for every origin train.
func (g *genCtx) roadOrders() ([]Order, []RoadBuild) {
	var orders []Order
	var building []RoadBuild
	for i := range g.p.RoadTrains {
		t := &g.p.RoadTrains[i]
		if t.Origin == "" {
			continue
		}
		o, b, ok := g.originTrain(t)
		building = append(building, b)
		if ok {
			orders = append(orders, o)
		}
	}
	for i := range g.p.RoadTrains {
		t := &g.p.RoadTrains[i]
		if t.Origin != "" {
			continue
		}
		if o, ok := g.throughTrain(t); ok {
			orders = append(orders, o)
		}
	}
	return orders, building
}

// originTrain gathers the cars waiting at the train's origin and builds the
// order when the length threshold or the age trigger is met.
func (g *genCtx) originTrain(t *profile.RoadTrain) (Order, RoadBuild, bool) {
	waiting := g.waitingFor(t)
	waitFt, _ := genSum(waiting)
	ready := g.triggered(t, waitFt, waiting)
	b := RoadBuild{
		Symbol:       t.Symbol,
		Destination:  t.Destination,
		Origin:       t.Origin,
		WaitingFt:    waitFt,
		MaxLengthFt:  t.Limit.MaxLengthFt,
		Pct:          genPct(waitFt, t.Limit.MaxLengthFt),
		ThresholdPct: t.MinLengthPct,
		Ready:        ready,
	}
	if !ready {
		return Order{}, b, false
	}
	consist := genCapConsist(waiting, t.Limit)
	if len(consist) == 0 {
		return Order{}, b, false
	}
	lines, _ := g.runStops(t, consist)
	o := g.newRoadOrder(t, lines)
	o.Consist = genRefs(consist)
	o.Totals = genRoadTotals(t, consist)
	return o, b, true
}

// throughTrain builds the order of a train with no origin: it only lists stop
// pick-ups and is available once their length meets the threshold.
func (g *genCtx) throughTrain(t *profile.RoadTrain) (Order, bool) {
	lines, picks := g.runStops(t, nil)
	if len(picks) == 0 {
		return Order{}, false
	}
	ft, _ := genSum(picks)
	if !g.triggered(t, ft, picks) {
		return Order{}, false
	}
	o := g.newRoadOrder(t, lines)
	o.Totals = genRoadTotals(t, picks)
	return o, true
}

func (g *genCtx) newRoadOrder(t *profile.RoadTrain, lines []Line) Order {
	return Order{
		ID:      "avail:" + t.Symbol,
		Kind:    KindRoad,
		JobID:   t.Symbol,
		Symbol:  t.Symbol,
		Profile: g.p.ID,
		State:   StateAvailable,
		Lines:   lines,
	}
}

// genRoadTotals totals the cars the order's train carries. For a train with an
// origin this is the consist leaving the origin yard; for a through train it
// is the cars it picks up. RequiredHP is gross tons times the train's minimum
// HP per ton.
func genRoadTotals(t *profile.RoadTrain, cars []*carstatus.Car) Totals {
	ft, tons := genSum(cars)
	return Totals{
		LengthFt:    ft,
		GrossTons:   tons,
		RequiredHP:  tons * t.Limit.MinHPT,
		MaxLengthFt: t.Limit.MaxLengthFt,
		MaxTons:     t.Limit.MaxTons,
	}
}

// waitingFor gathers, oldest first, the pool cars in the train's origin yard
// that wait for it: cars train-bound for it plus cars matching its stops'
// set-outs.
func (g *genCtx) waitingFor(t *profile.RoadTrain) []*carstatus.Car {
	var out []*carstatus.Car
	for _, c := range g.pool {
		if !genInYard(c, t.Origin) {
			continue
		}
		if g.trainBound(c, t) || g.stopSetsOut(t, c) {
			out = append(out, c)
		}
	}
	return out
}

// trainBound reports whether the car is train-bound for the train itself.
func (g *genCtx) trainBound(c *carstatus.Car, t *profile.RoadTrain) bool {
	return c.Dest.Kind == carstatus.DestTrain && profile.NormTag(c.Dest.Train) == profile.NormTag(t.Symbol)
}

// stopSetsOut reports whether any stop of the train sets the car out: a yard
// stop by its set-out tags, an industry stop for industry-bound cars.
func (g *genCtx) stopSetsOut(t *profile.RoadTrain, c *carstatus.Car) bool {
	for _, s := range t.Stops {
		if g.setsOutAt(t, s, c) {
			return true
		}
	}
	return false
}

// setsOutAt reports whether the car is a set-out at the stop.
func (g *genCtx) setsOutAt(t *profile.RoadTrain, s profile.Stop, c *carstatus.Car) bool {
	if s.Kind == "industry" {
		return genStopBound(c, t, profile.NormTag(s.Target))
	}
	return g.matches(c, s.SetOut)
}

// triggered reports whether an order is available: the cars' total length
// reaches the train's minimum percentage of its maximum length, or the oldest
// car has waited longer than the train's maximum age. With no cars there is
// never an order.
func (g *genCtx) triggered(t *profile.RoadTrain, ft float64, cars []*carstatus.Car) bool {
	if len(cars) == 0 || ft <= 0 {
		return false
	}
	if genThresholdMet(t, ft) {
		return true
	}
	return genAgeExceeded(t.MaxCarAgeDays, g.now, cars)
}

// genThresholdMet applies the minimum length percentage. A zero percentage or
// a train without a maximum length accepts any length above zero.
func genThresholdMet(t *profile.RoadTrain, ft float64) bool {
	if t.MinLengthPct <= 0 || t.Limit.MaxLengthFt <= 0 {
		return ft > 0
	}
	need := t.MinLengthPct / 100 * float64(t.Limit.MaxLengthFt)
	return ft+thresholdEpsilon >= need
}

// genAgeExceeded reports whether the oldest car has dwelt longer than
// maxDays in-game days. Cars without a dwell date are ignored.
func genAgeExceeded(maxDays float64, now time.Time, cars []*carstatus.Car) bool {
	if maxDays <= 0 || now.IsZero() {
		return false
	}
	for _, c := range cars {
		if c.DwellSince.IsZero() {
			continue
		}
		if now.Sub(c.DwellSince).Hours()/24 > maxDays {
			return true
		}
	}
	return false
}

// genPct is length as a percentage of the maximum; 0 when there is no maximum.
func genPct(ft float64, maxFt int) float64 {
	if maxFt <= 0 {
		return 0
	}
	return ft / float64(maxFt) * 100
}

// genCapConsist takes cars oldest first, stopping before the next car would
// exceed the maximum length or tons. A zero limit means unlimited.
func genCapConsist(cars []*carstatus.Car, lim profile.Limit) []*carstatus.Car {
	var out []*carstatus.Car
	var ft, tons float64
	for _, c := range cars {
		if !genFits(lim, ft+c.LengthFt, tons+c.GrossTons) {
			break
		}
		ft += c.LengthFt
		tons += c.GrossTons
		out = append(out, c)
	}
	return out
}

// genFits reports whether a train of the given length and tons is within lim.
func genFits(lim profile.Limit, ft, tons float64) bool {
	if lim.MaxLengthFt > 0 && ft > float64(lim.MaxLengthFt)+thresholdEpsilon {
		return false
	}
	if lim.MaxTons > 0 && tons > float64(lim.MaxTons)+thresholdEpsilon {
		return false
	}
	return true
}
