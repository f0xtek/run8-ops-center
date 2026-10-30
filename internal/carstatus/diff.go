package carstatus

import (
	"math"

	"run8-ops-center/internal/profile"
	"run8-ops-center/internal/run8save"
)

// moveEpsilonM is the distance tolerance, in metres, below which a change of
// a truck's distance along its section is treated as float noise rather than
// movement. A section change or a different route prefix always counts.
const moveEpsilonM = 0.5

// Diff lists the differences between two snapshots for the activity log. It
// compares every vehicle, freight cars and locomotives, by key:
//
//   - appeared: in cur only (NewTag set);
//   - disappeared: in prev only (OldTag set);
//   - moved: either truck changed route prefix or section, or its distance by
//     more than moveEpsilonM (NewTag set);
//   - retagged: the destination tags differ after profile.NormTag (OldTag and
//     NewTag set), so "AB10 BRR" versus "AB10  BRR" is not a change.
//
// A vehicle that was both moved and retagged yields two changes. Appeared,
// moved and retagged changes follow cur's order, then disappeared changes
// follow prev's order. Change.TrainID is the train in cur, or in prev for a
// disappeared vehicle.
func Diff(prev, cur *run8save.Snapshot) []Change {
	type entry struct {
		v     run8save.Vehicle
		train string
	}
	old := map[string]entry{}
	for _, t := range prev.Trains {
		for _, v := range t.Vehicles {
			old[v.Key()] = entry{v, t.ID}
		}
	}

	var out []Change
	seen := map[string]bool{}
	for _, t := range cur.Trains {
		for _, v := range t.Vehicles {
			k := v.Key()
			if seen[k] {
				continue
			}
			seen[k] = true
			num := Number(v.Model, v.UnitNumber)
			tag := profile.NormTag(v.Tag)
			o, ok := old[k]
			if !ok {
				out = append(out, Change{Kind: ChangeAppeared, Key: k, Number: num, TrainID: t.ID, NewTag: tag})
				continue
			}
			if moved(o.v.Trucks, v.Trucks) {
				out = append(out, Change{Kind: ChangeMoved, Key: k, Number: num, TrainID: t.ID, NewTag: tag})
			}
			if oldTag := profile.NormTag(o.v.Tag); oldTag != tag {
				out = append(out, Change{Kind: ChangeRetagged, Key: k, Number: num, TrainID: t.ID, OldTag: oldTag, NewTag: tag})
			}
		}
	}

	gone := map[string]bool{}
	for _, t := range prev.Trains {
		for _, v := range t.Vehicles {
			k := v.Key()
			if seen[k] || gone[k] {
				continue
			}
			gone[k] = true
			out = append(out, Change{
				Kind: ChangeDisappeared, Key: k, Number: Number(v.Model, v.UnitNumber),
				TrainID: t.ID, OldTag: profile.NormTag(v.Tag),
			})
		}
	}
	return out
}

// moved reports whether two truck lists place a vehicle differently.
func moved(a, b []run8save.Truck) bool {
	if len(a) != len(b) {
		return true
	}
	for i := range a {
		if a[i].RoutePrefix != b[i].RoutePrefix || a[i].Section != b[i].Section ||
			math.Abs(a[i].DistanceM-b[i].DistanceM) > moveEpsilonM {
			return true
		}
	}
	return false
}
