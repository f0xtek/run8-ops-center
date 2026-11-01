package trackmap

import (
	"math"
	"sort"
	"strings"

	"run8-ops-center/internal/profile"
	"run8-ops-center/internal/run8route"
)

// NameHint is one listed yard track offered as a track name. DiffFt is the
// distance between its listed length and the measured one, and is only
// meaningful when HasDiff is set.
type NameHint struct {
	Track   profile.YardTrack
	DiffFt  int
	HasDiff bool
}

// TrackNamesInYard returns the track names that labels give to the yard with
// the given code, ignoring other yards and labels of other kinds.
func (m *Map) TrackNamesInYard(code string) map[string]bool {
	out := map[string]bool{}
	for _, l := range m.labels() {
		if l.Kind == KindYard && l.Track != "" && strings.EqualFold(l.Yard, code) {
			out[l.Track] = true
		}
	}
	return out
}

// NameHints lists the yard's listed tracks whose names are not in used, which
// holds the track names already labelled in that yard. Names match ignoring
// case and extra whitespace. With measuredFt above 0, tracks with a listed
// length come first, nearest to measuredFt first and in profile order among
// equals, and tracks without a length follow in profile order. Otherwise the
// profile's order is kept.
func NameHints(y profile.Yard, used map[string]bool, measuredFt int) []NameHint {
	taken := make(map[string]bool, len(used))
	for name, ok := range used {
		if ok {
			taken[strings.ToLower(profile.NormTag(name))] = true
		}
	}
	var withLen, without []NameHint
	for _, t := range y.Tracks {
		if taken[strings.ToLower(profile.NormTag(t.Name))] {
			continue
		}
		h := NameHint{Track: t}
		if measuredFt > 0 && t.LengthFt > 0 {
			h.DiffFt = t.LengthFt - measuredFt
			if h.DiffFt < 0 {
				h.DiffFt = -h.DiffFt
			}
			h.HasDiff = true
			withLen = append(withLen, h)
			continue
		}
		without = append(without, h)
	}
	sort.SliceStable(withLen, func(i, j int) bool { return withLen[i].DiffFt < withLen[j].DiffFt })
	return append(withLen, without...)
}

// LocationLengthsFt measures the yard locations in groups: the length in feet
// of each location's labelled sections, from the track database, keyed by
// Location.Key. A section of another route prefix or one the database does not
// know adds nothing, and a location that measures nothing is left out. It
// returns nil without usable route data.
func LocationLengthsFt(groups []LocationGroup, p *profile.Profile, route *run8route.Data) map[string]int {
	rc := newRouteCtx(p, route)
	if rc == nil {
		return nil
	}
	out := map[string]int{}
	for _, g := range groups {
		if g.Location.Kind != KindYard {
			continue
		}
		metres := 0.0
		for _, s := range g.Sections {
			if s.Prefix != rc.prefix {
				continue
			}
			if sec, ok := rc.topo.Section(s.Section); ok {
				metres += sec.LengthM
			}
		}
		if ft := int(math.Round(metres * metresToFeet)); ft > 0 {
			out[g.Location.Key()] = ft
		}
	}
	return out
}
