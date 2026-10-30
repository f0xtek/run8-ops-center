package trackmap

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"run8-ops-center/internal/profile"
	"run8-ops-center/internal/run8route"
	"run8-ops-center/internal/run8save"
)

// maxSamples is how many unit numbers a Candidate lists.
const maxSamples = 5

// anchorRadiusM is how far from a yard anchor a cut can stand and still get
// the anchor's yard as a suggestion.
const anchorRadiusM = 3000.0

// Suggestion sources. They describe a Suggestion, not a Label: a label's own
// source is "" or SourceRun8.
const (
	SourceTags      = "tags"      // every car tagged for one industry
	SourceConnected = "connected" // the cut's track connects to a labelled section
	SourceAnchor    = "anchor"    // a yard anchor lies within reach
)

// Suggestion is a proposed label for a candidate. It is never applied
// automatically.
type Suggestion struct {
	Label  Label
	Reason string // human-readable
	Source string // SourceTags, SourceConnected or SourceAnchor
	// NeedsTrackName is set when Label is a yard with no track name. It can
	// only pre-fill the label form, because it is not a complete label.
	NeedsTrackName bool
}

// TagCount is a destination tag and how many cars carry it.
type TagCount struct {
	Tag   string
	Count int
}

// Candidate is one train (or cut) that occupies at least one unmapped section.
type Candidate struct {
	TrainID  string
	Powered  bool         // the train has a locomotive
	Symbol   string       // first locomotive's destination tag, "" for a cut
	Sections []SectionRef // unmapped sections it occupies, sorted
	// Track is every section Apply labels, sorted: the unmapped sections of
	// each switch-bounded track the train occupies, plus the switch sections
	// it occupies. It includes Sections. Without route data it equals
	// Sections.
	Track      []SectionRef
	Cars       int         // non-locomotive vehicles
	Samples    []string    // up to 5 unit numbers (cars; locomotives when it has no cars)
	Tags       []TagCount  // distinct non-blank car tags, most common first
	Suggestion *Suggestion // nil when there is nothing to suggest
}

// routeCtx is the route data a candidate search can use: the track graph of
// the profile's first route, and that route's section prefix.
type routeCtx struct {
	topo   *run8route.Topology
	prefix int
	data   *run8route.Data
	p      *profile.Profile

	anchors []anchor
}

// anchor is a point that marks where a yard is.
type anchor struct {
	yard, name string
	x, z       float64
}

// newRouteCtx returns nil when there is no usable track database or the
// profile names no route prefix to apply it to.
func newRouteCtx(p *profile.Profile, route *run8route.Data) *routeCtx {
	if route == nil || route.Topology == nil || p == nil {
		return nil
	}
	prefix, ok := routePrefix(p)
	if !ok {
		return nil
	}
	rc := &routeCtx{topo: route.Topology, prefix: prefix, data: route, p: p}
	for _, y := range p.Yards {
		for _, name := range y.AnchorMileposts {
			want := run8route.NormTag(name)
			for _, mp := range route.Mileposts {
				if run8route.NormTag(mp.Station) == want {
					rc.anchors = append(rc.anchors, anchor{y.Code, "milepost " + want, mp.X, mp.Z})
				}
			}
		}
		for _, tag := range y.AnchorIndustries {
			want := run8route.NormTag(tag)
			for _, ind := range route.Industries {
				if run8route.NormTag(ind.Tag) != want {
					continue
				}
				for _, t := range ind.Tracks {
					if sec, ok := rc.topo.Section(t.Section); ok && t.Prefix == prefix {
						rc.anchors = append(rc.anchors, anchor{y.Code, "industry " + want, sec.X, sec.Z})
					}
				}
			}
		}
	}
	return rc
}

// Candidates returns every train in the snapshot that occupies at least one
// unmapped section, in snapshot order. A section carrying any label, including
// ignored, is mapped. p may be nil, which disables tag suggestions. route may
// be nil, which disables everything that needs the track database: whole-track
// labelling and the connected and anchor suggestions.
func (m *Map) Candidates(snap *run8save.Snapshot, p *profile.Profile, route *run8route.Data) []Candidate {
	if snap == nil {
		return nil
	}
	rc := newRouteCtx(p, route)
	industries := map[string][]SectionRef{}
	for k, l := range m.labels() {
		if l.Kind != KindIndustry {
			continue
		}
		pre, sec, err := ParseKey(k)
		if err != nil {
			continue
		}
		for _, tag := range l.Industries {
			industries[tag] = append(industries[tag], SectionRef{pre, sec})
		}
	}
	var out []Candidate
	for _, t := range snap.Trains {
		seen := map[SectionRef]struct{}{}
		var secs []SectionRef
		for _, v := range t.Vehicles {
			for _, tr := range v.Trucks {
				ref := SectionRef{tr.RoutePrefix, tr.Section}
				if _, dup := seen[ref]; dup {
					continue
				}
				seen[ref] = struct{}{}
				if _, ok := m.Lookup(ref.Prefix, ref.Section); !ok {
					secs = append(secs, ref)
				}
			}
		}
		if len(secs) == 0 {
			continue
		}
		sortSections(secs)
		c := Candidate{
			TrainID:  t.ID,
			Powered:  t.Powered(),
			Symbol:   t.Symbol(),
			Sections: secs,
			Track:    m.wholeTrack(secs, rc),
		}
		var cars, locos []run8save.Vehicle
		for _, v := range t.Vehicles {
			if v.IsLoco() {
				locos = append(locos, v)
			} else {
				cars = append(cars, v)
			}
		}
		c.Cars = len(cars)
		sample := cars
		if len(sample) == 0 {
			sample = locos
		}
		for _, v := range sample {
			if len(c.Samples) == maxSamples {
				break
			}
			c.Samples = append(c.Samples, v.UnitNumber)
		}
		c.Tags = tagCounts(cars)
		c.Suggestion = m.suggest(c, cars, p, rc, industries)
		out = append(out, c)
	}
	return out
}

// labels is the label map, empty for a nil map.
func (m *Map) labels() map[string]Label {
	if m == nil {
		return nil
	}
	return m.Labels
}

// wholeTrack extends the occupied unmapped sections to the unmapped sections
// of every switch-bounded track they stand on. Without route data it returns
// the occupied sections.
func (m *Map) wholeTrack(occupied []SectionRef, rc *routeCtx) []SectionRef {
	if rc == nil {
		return append([]SectionRef(nil), occupied...)
	}
	have := make(map[SectionRef]struct{}, len(occupied))
	out := append([]SectionRef(nil), occupied...)
	for _, s := range occupied {
		have[s] = struct{}{}
	}
	for _, s := range occupied {
		if s.Prefix != rc.prefix {
			continue
		}
		for _, n := range rc.topo.TrackOf(s.Section) {
			ref := SectionRef{rc.prefix, n}
			if _, dup := have[ref]; dup {
				continue
			}
			have[ref] = struct{}{}
			if _, ok := m.Lookup(ref.Prefix, ref.Section); !ok {
				out = append(out, ref)
			}
		}
	}
	sortSections(out)
	return out
}

func blankTag(t string) bool { return t == "" || strings.EqualFold(t, "None") }

func tagCounts(cars []run8save.Vehicle) []TagCount {
	counts := map[string]int{}
	for _, v := range cars {
		if t := profile.NormTag(v.Tag); !blankTag(t) {
			counts[t]++
		}
	}
	out := make([]TagCount, 0, len(counts))
	for t, n := range counts {
		out = append(out, TagCount{t, n})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Tag < out[j].Tag
	})
	return out
}

// suggest tries the tag rule, then a connected label, then the nearest
// anchor. The last two need the track database.
func (m *Map) suggest(c Candidate, cars []run8save.Vehicle, p *profile.Profile, rc *routeCtx, industries map[string][]SectionRef) *Suggestion {
	if s := tagSuggestion(c, cars, p, rc, industries); s != nil {
		return s
	}
	if rc == nil {
		return nil
	}
	if s := m.connectedSuggestion(c, rc); s != nil {
		return s
	}
	return anchorSuggestion(c, rc)
}

// tagSuggestion fires when every car carries a tag "<local> <industryTag>" (or
// the bare industry tag) for the same profile industry, and either that
// industry has no labelled section or one of its labelled sections is
// adjacent to the candidate's track. Without route data the second condition
// cannot be told, so only an industry with no labelled section qualifies.
func tagSuggestion(c Candidate, cars []run8save.Vehicle, p *profile.Profile, rc *routeCtx, industries map[string][]SectionRef) *Suggestion {
	if p == nil || len(cars) == 0 {
		return nil
	}
	var found *profile.Industry
	for _, v := range cars {
		ind := industryOfTag(p, v.Tag)
		if ind == nil || (found != nil && found.Tag != ind.Tag) {
			return nil
		}
		found = ind
	}
	reason := fmt.Sprintf("all %d cars are tagged for %s (%s)", len(cars), found.Tag, found.Name)
	if labelled := industries[found.Tag]; len(labelled) > 0 {
		if rc == nil {
			return nil
		}
		adj := map[int]bool{}
		for _, s := range rc.topo.Adjacent(trackInts(c.Track, rc.prefix)) {
			adj[s] = true
		}
		near := -1
		for _, l := range labelled {
			if l.Prefix == rc.prefix && adj[l.Section] {
				near = l.Section
				break
			}
		}
		if near < 0 {
			return nil
		}
		reason += fmt.Sprintf("; its track connects to %s, which is labelled %s", Key(rc.prefix, near), found.Tag)
	}
	return &Suggestion{
		Label:  Label{Kind: KindIndustry, Industries: []string{found.Tag}},
		Reason: reason,
		Source: SourceTags,
	}
}

// trackInts returns the section numbers of refs with the given prefix.
func trackInts(refs []SectionRef, prefix int) []int {
	var out []int
	for _, r := range refs {
		if r.Prefix == prefix {
			out = append(out, r.Section)
		}
	}
	return out
}

// industryOfTag resolves a car's destination tag to an industry: the second
// token of "<local> <tag>", or a one-token tag that is itself an industry tag.
func industryOfTag(p *profile.Profile, raw string) *profile.Industry {
	fields := strings.Fields(raw)
	switch len(fields) {
	case 0:
		return nil
	case 1:
		return p.IndustryByTag(fields[0])
	}
	if ind := p.IndustryByTag(fields[1]); ind != nil {
		return ind
	}
	return p.IndustryByTag(strings.Join(fields[1:], " "))
}

// connectedSuggestion proposes the most common label among the sections
// linked to the candidate's track, directly or across one switch. An industry
// label is offered whole. A yard label is offered as its yard alone.
func (m *Map) connectedSuggestion(c Candidate, rc *routeCtx) *Suggestion {
	type vote struct {
		label Label
		n     int
		near  SectionRef
		from  Label
	}
	votes := map[string]*vote{}
	var order []string
	for _, s := range rc.topo.Adjacent(trackInts(c.Track, rc.prefix)) {
		l, ok := m.Lookup(rc.prefix, s)
		if !ok {
			continue
		}
		var key string
		var offer Label
		switch l.Kind {
		case KindYard:
			key, offer = "yard|"+l.Yard, Label{Kind: KindYard, Yard: l.Yard}
		case KindIndustry:
			key, offer = l.Location().Key(), Label{Kind: KindIndustry, Industries: append([]string(nil), l.Industries...)}
		default:
			continue
		}
		v := votes[key]
		if v == nil {
			v = &vote{label: offer, near: SectionRef{rc.prefix, s}, from: l}
			votes[key] = v
			order = append(order, key)
		}
		v.n++
	}
	var best *vote
	for _, k := range order {
		if v := votes[k]; best == nil || v.n > best.n {
			best = v
		}
	}
	if best == nil {
		return nil
	}
	s := &Suggestion{
		Label:  best.label,
		Source: SourceConnected,
	}
	if best.label.Kind == KindYard {
		s.NeedsTrackName = true
		s.Reason = fmt.Sprintf("its track connects to %s, which is labelled %s; enter the track name", best.near.Key(), best.from.Location())
	} else {
		s.Reason = fmt.Sprintf("its track connects to %s, which is labelled %s", best.near.Key(), best.from.Location())
	}
	return s
}

// anchorSuggestion pre-fills the yard of the anchor nearest to the
// candidate's track, when one lies within anchorRadiusM.
func anchorSuggestion(c Candidate, rc *routeCtx) *Suggestion {
	best, bestD := -1, math.Inf(1)
	for _, s := range trackInts(c.Track, rc.prefix) {
		sec, ok := rc.topo.Section(s)
		if !ok {
			continue
		}
		for i, a := range rc.anchors {
			if d := math.Hypot(sec.X-a.x, sec.Z-a.z); d < bestD {
				best, bestD = i, d
			}
		}
	}
	if best < 0 || bestD > anchorRadiusM {
		return nil
	}
	a := rc.anchors[best]
	return &Suggestion{
		Label:          Label{Kind: KindYard, Yard: a.yard},
		Reason:         fmt.Sprintf("the nearest anchor is %s of yard %s, %.0f m away; enter the track name", a.name, a.yard, bestD),
		Source:         SourceAnchor,
		NeedsTrackName: true,
	}
}

// Apply labels every still-unmapped section of the candidate's track with l,
// as a manual label. Sections that have gained a label since the candidate
// was computed are left alone.
func (m *Map) Apply(c Candidate, l Label) {
	secs := c.Track
	if len(secs) == 0 {
		secs = c.Sections
	}
	for _, s := range secs {
		if _, ok := m.Lookup(s.Prefix, s.Section); ok {
			continue
		}
		m.Set(s.Prefix, s.Section, l)
	}
}
