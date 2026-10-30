package carstatus

import (
	"run8-ops-center/internal/profile"
	"run8-ops-center/internal/run8save"
	"run8-ops-center/internal/trackmap"
)

// Derive computes the status of every freight car in snap (locomotives are
// not included). prev is the dwell state from the previous snapshot; Derive
// never modifies it and returns a new state holding only cars present in snap.
// A car seen for the first time, or whose location key changed, gets
// Since = snap.GameDate.
func Derive(snap *run8save.Snapshot, p *profile.Profile, tm *trackmap.Map, prev DwellState) (*Status, DwellState) {
	st := &Status{Snapshot: snap, ByKey: map[string]*Car{}}
	next := DwellState{}
	for _, t := range snap.Trains {
		symbol := t.Symbol()
		powered := t.Powered()
		for _, v := range t.Vehicles {
			if v.IsLoco() {
				continue
			}
			c := deriveCar(v, p, tm)
			c.TrainID = t.ID
			c.TrainSymbol = symbol
			c.Service = serviceOf(c, powered)

			key := locKey(c.Loc)
			rec, ok := prev[c.Key]
			if !ok || rec.Loc != key {
				rec = DwellRecord{Loc: key, Since: snap.GameDate}
			}
			next[c.Key] = rec
			c.DwellSince = rec.Since

			if c.Loc.Kind == LocUnmapped {
				st.Unmapped++
			}
			if c.Dest.Kind == DestUnknown || c.Type == profile.Unknown {
				st.Unknown++
			}
			st.Cars = append(st.Cars, c)
		}
	}
	for i := range st.Cars {
		st.ByKey[st.Cars[i].Key] = &st.Cars[i]
	}
	return st, next
}

// deriveCar fills everything that depends only on the vehicle itself.
func deriveCar(v run8save.Vehicle, p *profile.Profile, tm *trackmap.Map) Car {
	typ := p.CarTypeOf(v.Model)
	def := p.DefaultsFor(typ)
	c := Car{
		Key:      v.Key(),
		Number:   Number(v.Model, v.UnitNumber),
		Mark:     Mark(v.Model),
		Model:    v.Model,
		Type:     typ,
		Loaded:   v.LoadTons > 0,
		LoadTons: v.LoadTons,
		TareTons: def.TareTons,
		LengthFt: def.LengthFt,
		Dest:     Classify(v.Tag, p),
		Loc:      locate(v, tm),
	}
	c.GrossTons = c.LoadTons + c.TareTons
	return c
}

// locate resolves the car's location from its leading truck, then its
// trailing truck when the leading section has no label.
func locate(v run8save.Vehicle, tm *trackmap.Map) Location {
	for _, t := range v.Trucks {
		l, ok := tm.Lookup(t.RoutePrefix, t.Section)
		if !ok {
			continue
		}
		loc := Location{Prefix: t.RoutePrefix, Section: t.Section}
		switch l.Kind {
		case trackmap.KindYard:
			loc.Kind, loc.Yard, loc.Track = LocYard, l.Yard, l.Track
		case trackmap.KindIndustry:
			loc.Kind, loc.Industries = LocIndustry, append([]string(nil), l.Industries...)
		default:
			loc.Kind = LocIgnored
		}
		return loc
	}
	loc := Location{Kind: LocUnmapped}
	if len(v.Trucks) > 0 {
		loc.Prefix, loc.Section = v.Trucks[0].RoutePrefix, v.Trucks[0].Section
	}
	return loc
}

// serviceOf assigns the single service state of a car.
func serviceOf(c Car, powered bool) Service {
	switch c.Loc.Kind {
	case LocIndustry:
		if c.Dest.Kind == DestIndustry && contains(c.Loc.Industries, c.Dest.Industry) {
			return ServiceWorking
		}
		return ServiceReadyToPull
	case LocYard:
		switch c.Dest.Kind {
		case DestIndustry:
			return ServiceAwaitingDelivery
		case DestTrain:
			return ServiceAwaitingTrain
		}
	case LocIgnored:
		if powered {
			return ServiceEnRoute
		}
	}
	return ServiceOther
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
