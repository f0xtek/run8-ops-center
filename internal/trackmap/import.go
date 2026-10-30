package trackmap

import (
	"cmp"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	"run8-ops-center/internal/profile"
	"run8-ops-center/internal/run8route"
)

// maxReceivingPath is the longest path, in sections, searched between the two
// ends of a receiving track.
const maxReceivingPath = 60

// Skip is something the import left out, and why.
type Skip struct {
	What   string // "industry Padded Works (XYZ)", "NOR / Bowl 3", "section 280/123"
	Reason string
}

// ImportPlan is the set of labels an import from Run8 route data proposes.
// Nothing in it has been applied.
type ImportPlan struct {
	// Labels maps a section key to its proposed label. Every label has
	// Source SourceRun8.
	Labels      map[string]Label
	Skipped     []Skip
	Unavailable []string // features that could not be built, each with the missing data

	// keepIndustries and keepYards say that the plan has no say over the
	// existing run8 industry or yard labels, because the data they come from
	// was not read. Diff leaves those labels in place instead of removing them.
	keepIndustries, keepYards bool
}

// claim is one thing that wants to label a section.
type claim struct {
	who   string // for the skip reason
	label Label
}

// builder collects claims by section key.
type builder struct {
	claims  map[string][]claim
	skipped []Skip
}

func (b *builder) skip(what, reason string) { b.skipped = append(b.skipped, Skip{what, reason}) }

func (b *builder) claim(prefix, section int, who string, l Label) {
	k := Key(prefix, section)
	b.claims[k] = append(b.claims[k], claim{who, l})
}

// routePrefix is the track section prefix of the route whose files Load
// decodes: the first route of the profile's run8 block.
func routePrefix(p *profile.Profile) (int, bool) {
	if p == nil || p.Run8 == nil || len(p.Run8.Routes) == 0 {
		return 0, false
	}
	return p.Run8.Routes[0].Prefix, true
}

// BuildImport proposes labels from Run8 route data: industry tracks from the
// industry configuration, bowl tracks from the hump controllers and receiving
// tracks from the named end points. A feature whose data is missing is listed
// in Unavailable and the rest is built. A section that two different
// locations claim is left out and listed in Skipped.
func BuildImport(d run8route.Data, p *profile.Profile) ImportPlan {
	plan := ImportPlan{Labels: map[string]Label{}}
	if p == nil {
		plan.Unavailable = []string{"all track labels: no route profile is loaded"}
		plan.keepIndustries, plan.keepYards = true, true
		return plan
	}
	b := &builder{claims: map[string][]claim{}}

	if len(d.Industries) == 0 {
		plan.Unavailable = append(plan.Unavailable, "industry labels: "+run8route.ConfigFile+" could not be read")
		plan.keepIndustries = true
	} else {
		b.industries(d.Industries, p)
	}

	prefix, havePrefix := routePrefix(p)
	hasHumps := havePrefix && len(p.Run8.Humps) > 0
	hasEnds := havePrefix && len(p.Run8.TrackEnds) > 0
	switch {
	case !havePrefix:
		plan.Unavailable = append(plan.Unavailable, "bowl and receiving track labels: the profile names no Run8 route")
		plan.keepYards = true
	case d.Topology == nil:
		if hasHumps || hasEnds {
			plan.Unavailable = append(plan.Unavailable, "bowl and receiving track labels: "+run8route.TrackDBFile+" could not be read")
			plan.keepYards = true
		}
	default:
		if hasHumps {
			if len(d.Hump) == 0 {
				plan.Unavailable = append(plan.Unavailable, "bowl track labels: "+run8route.HumpFile+" could not be read")
				plan.keepYards = true
			} else {
				b.bowls(d, p, prefix)
			}
		}
		if hasEnds {
			if len(d.Locations) == 0 {
				plan.Unavailable = append(plan.Unavailable, "receiving track labels: "+run8route.LocationsFile+" could not be read")
				plan.keepYards = true
			} else {
				b.receiving(d, p, prefix)
			}
		}
	}

	keys := make([]string, 0, len(b.claims))
	for k := range b.claims {
		keys = append(keys, k)
	}
	slices.SortFunc(keys, keyCmp)
	plan.Skipped = b.skipped
	for _, k := range keys {
		cl := b.claims[k]
		var who []string
		seen := map[string]bool{}
		for _, c := range cl {
			if lk := c.label.Location().Key(); !seen[lk] {
				seen[lk] = true
				who = append(who, c.who)
			}
		}
		if len(who) > 1 {
			plan.Skipped = append(plan.Skipped, Skip{"section " + k, "claimed by " + strings.Join(who, " and ") + ", left out"})
			continue
		}
		l := cloneLabel(cl[0].label)
		l.Source = SourceRun8
		plan.Labels[k] = l
	}
	return plan
}

// industries claims every listed section for its industry's tag. A section
// that two industries list carries both tags.
func (b *builder) industries(inds []run8route.Industry, p *profile.Profile) {
	tags := map[SectionRef][]string{}
	for _, ind := range inds {
		tag := run8route.NormTag(ind.Tag)
		if p.IndustryByTag(tag) == nil {
			b.skip(fmt.Sprintf("industry %s (%s)", run8route.NormTag(ind.Name), tag),
				fmt.Sprintf("tag %q is not an industry of the profile", tag))
			continue
		}
		for _, t := range ind.Tracks {
			ref := SectionRef{t.Prefix, t.Section}
			if !slices.Contains(tags[ref], tag) {
				tags[ref] = append(tags[ref], tag)
			}
		}
	}
	for ref, ts := range tags {
		sort.Strings(ts)
		b.claim(ref.Prefix, ref.Section, "industry "+strings.Join(ts, ", "),
			Label{Kind: KindIndustry, Industries: ts})
	}
}

func sameName(a, b string) bool {
	return strings.EqualFold(run8route.NormTag(a), run8route.NormTag(b))
}

func trackName(prefix, name string) string {
	return strings.TrimSpace(prefix + " " + strings.TrimSpace(name))
}

// bowls claims each hump controller track's sections. A track runs from the
// last switch of its path, along the leg the path selects, to the next switch
// or the end of the track.
func (b *builder) bowls(d run8route.Data, p *profile.Profile, prefix int) {
	topo := d.Topology
	matched := make([]bool, len(d.Hump))
	for _, cfg := range p.Run8.Humps {
		found := false
		for i, hc := range d.Hump {
			if !sameName(hc.Name, cfg.Controller) {
				continue
			}
			matched[i], found = true, true
			for _, tr := range hc.Tracks {
				name := trackName(cfg.TrackPrefix, tr.Name)
				who := cfg.Yard + " / " + name
				if len(tr.Switches) == 0 {
					b.skip(who, "its path throws no switch")
					continue
				}
				last := tr.Switches[len(tr.Switches)-1]
				leg, ok := topo.Leg(last.Section, last.Reversed)
				if !ok {
					b.skip(who, fmt.Sprintf("section %d is not a switch with a %s leg in the track database", last.Section, legName(last.Reversed)))
					continue
				}
				if s, _ := topo.Section(leg); s.Switch {
					b.skip(who, fmt.Sprintf("the %s leg of switch %d leads straight into switch %d", legName(last.Reversed), last.Section, leg))
					continue
				}
				secs := topo.TrackOf(leg)
				if len(secs) == 0 {
					b.skip(who, fmt.Sprintf("the %s leg of switch %d leads to section %d, which is not in the track database", legName(last.Reversed), last.Section, leg))
					continue
				}
				l := Label{Kind: KindYard, Yard: cfg.Yard, Track: name}
				for _, s := range secs {
					b.claim(prefix, s, who, l)
				}
			}
		}
		if !found {
			b.skip("hump controller "+cfg.Controller, "not in "+run8route.HumpFile)
		}
	}
	for i, hc := range d.Hump {
		if !matched[i] {
			b.skip("hump controller "+hc.Name, "the profile maps it to no yard")
		}
	}
}

func legName(reversed bool) string {
	if reversed {
		return "reverse"
	}
	return "normal"
}

// ends holds the special locations that mark the ends of one numbered track.
type ends struct{ north, south []run8route.SpecialLocation }

// receiving pairs the end points each trackEnds pattern names and claims the
// sections between them.
func (b *builder) receiving(d run8route.Data, p *profile.Profile, prefix int) {
	topo := d.Topology
	for _, te := range p.Run8.TrackEnds {
		if te.Pattern == nil {
			continue
		}
		// The match is case-insensitive whatever the profile wrote.
		re, err := regexp.Compile("(?i)" + te.Pattern.String())
		if err != nil {
			re = te.Pattern
		}
		ni, ei := re.SubexpIndex("n"), re.SubexpIndex("end")
		if ni < 0 || ei < 0 {
			b.skip("end pattern "+te.Pattern.String(), `it lacks the named groups "n" and "end"`)
			continue
		}
		byN := map[string]*ends{}
		var order []string
		for _, loc := range d.Locations {
			m := re.FindStringSubmatch(run8route.NormTag(loc.Name))
			if m == nil {
				continue
			}
			if loc.Prefix != prefix {
				b.skip("end point "+loc.Name, fmt.Sprintf("it is on route prefix %d, not %d", loc.Prefix, prefix))
				continue
			}
			n, end := m[ni], strings.ToLower(m[ei])
			e := byN[n]
			if e == nil {
				e = &ends{}
				byN[n] = e
				order = append(order, n)
			}
			switch {
			case strings.HasPrefix(end, "n"):
				e.north = append(e.north, loc)
			case strings.HasPrefix(end, "s"):
				e.south = append(e.south, loc)
			default:
				b.skip("end point "+loc.Name, fmt.Sprintf("%q is neither a north nor a south end", m[ei]))
			}
		}
		for _, n := range order {
			e := byN[n]
			name := trackName(te.TrackPrefix, n)
			who := te.Yard + " / " + name
			switch {
			case len(e.north) == 0 && len(e.south) == 0:
				continue // only ends of an unknown kind, already reported
			case len(e.south) == 0:
				b.skip(who, fmt.Sprintf("it has a north end (%s) but no south end", e.north[0].Name))
				continue
			case len(e.north) == 0:
				b.skip(who, fmt.Sprintf("it has a south end (%s) but no north end", e.south[0].Name))
				continue
			case len(e.north) > 1 || len(e.south) > 1:
				b.skip(who, "it has more than one north or south end")
				continue
			}
			from, to := e.north[0].Section, e.south[0].Section
			path, ok := topo.Path(from, to, maxReceivingPath)
			if !ok {
				b.skip(who, fmt.Sprintf("no path of at most %d sections joins its ends, sections %d and %d", maxReceivingPath, from, to))
				continue
			}
			if sw := firstSwitch(topo, path); sw >= 0 {
				b.skip(who, fmt.Sprintf("the path between its ends, sections %d and %d, crosses switch %d", from, to, sw))
				continue
			}
			l := Label{Kind: KindYard, Yard: te.Yard, Track: name}
			for _, s := range path {
				b.claim(prefix, s, who, l)
			}
		}
	}
}

// firstSwitch returns the first switch in path, or -1.
func firstSwitch(t *run8route.Topology, path []int) int {
	for _, s := range path {
		if sec, ok := t.Section(s); ok && sec.Switch {
			return s
		}
	}
	return -1
}

// keyCmp orders section keys by prefix, then section.
func keyCmp(a, b string) int {
	pa, sa, ea := ParseKey(a)
	pb, sb, eb := ParseKey(b)
	if ea != nil || eb != nil {
		return strings.Compare(a, b)
	}
	if c := cmp.Compare(pa, pb); c != 0 {
		return c
	}
	return cmp.Compare(sa, sb)
}

// DiffEntry is one section in an import diff. Old is the label the map holds
// and New the label the import proposes; either is nil when it has none.
type DiffEntry struct {
	Key      string
	Old, New *Label
}

// ImportDiff sorts every section an import touches or skips into one of five
// groups. It describes what ApplyImport will do.
type ImportDiff struct {
	New        []DiffEntry // sections with no label that the import labels
	Changed    []DiffEntry // run8 labels the import replaces
	Removed    []DiffEntry // run8 labels the import no longer produces
	KeptManual []DiffEntry // sections the import would label but a manual label holds
	Unchanged  []DiffEntry // run8 labels the import produces again as they are
	Skipped    []Skip
	// Unavailable lists the features the plan could not build.
	Unavailable []string
}

// Diff compares what the import proposes with the map. Manual labels are
// never changed or removed. A run8 label of a feature whose data was not read
// is neither changed nor removed.
func (m *Map) Diff(plan ImportPlan) ImportDiff {
	d := ImportDiff{
		Skipped:     slices.Clone(plan.Skipped),
		Unavailable: slices.Clone(plan.Unavailable),
	}
	keys := make([]string, 0, len(plan.Labels))
	for k := range plan.Labels {
		keys = append(keys, k)
	}
	slices.SortFunc(keys, keyCmp)
	for _, k := range keys {
		nl := cloneLabel(plan.Labels[k])
		e := DiffEntry{Key: k, New: &nl}
		cur, ok := Label{}, false
		if m != nil {
			cur, ok = m.Labels[k]
		}
		switch {
		case !ok:
			d.New = append(d.New, e)
		case cur.Source != SourceRun8:
			old := cloneLabel(cur)
			e.Old = &old
			d.KeptManual = append(d.KeptManual, e)
		default:
			old := cloneLabel(cur)
			e.Old = &old
			if old.Location().Key() == nl.Location().Key() {
				d.Unchanged = append(d.Unchanged, e)
			} else {
				d.Changed = append(d.Changed, e)
			}
		}
	}
	if m != nil {
		for k, cur := range m.Labels {
			if cur.Source != SourceRun8 {
				continue
			}
			if _, ok := plan.Labels[k]; ok {
				continue
			}
			if (cur.Kind == KindIndustry && plan.keepIndustries) || (cur.Kind == KindYard && plan.keepYards) {
				continue
			}
			old := cloneLabel(cur)
			d.Removed = append(d.Removed, DiffEntry{Key: k, Old: &old})
		}
		slices.SortFunc(d.Removed, func(a, b DiffEntry) int { return keyCmp(a.Key, b.Key) })
	}
	return d
}

// DiffGroup is the diff entries of one location.
type DiffGroup struct {
	Location                                     Location
	New, Changed, Removed, KeptManual, Unchanged []DiffEntry
}

// location is the place an entry puts its section: where the import would
// label it, or where it is now when the import drops it.
func (e DiffEntry) location() Location {
	if e.New != nil {
		return e.New.Location()
	}
	return e.Old.Location()
}

// Groups returns the entries grouped by location, yard tracks first, then
// industries. Tracks sort in natural order, so Bowl 2 comes before Bowl 10.
// Entries keep their section order.
func (d ImportDiff) Groups() []DiffGroup {
	idx := map[string]int{}
	var out []DiffGroup
	add := func(entries []DiffEntry, dst func(*DiffGroup) *[]DiffEntry) {
		for _, e := range entries {
			loc := e.location()
			k := loc.Key()
			i, ok := idx[k]
			if !ok {
				i = len(out)
				idx[k] = i
				out = append(out, DiffGroup{Location: loc})
			}
			s := dst(&out[i])
			*s = append(*s, e)
		}
	}
	add(d.New, func(g *DiffGroup) *[]DiffEntry { return &g.New })
	add(d.Changed, func(g *DiffGroup) *[]DiffEntry { return &g.Changed })
	add(d.Removed, func(g *DiffGroup) *[]DiffEntry { return &g.Removed })
	add(d.KeptManual, func(g *DiffGroup) *[]DiffEntry { return &g.KeptManual })
	add(d.Unchanged, func(g *DiffGroup) *[]DiffEntry { return &g.Unchanged })
	slices.SortStableFunc(out, func(a, b DiffGroup) int {
		la, lb := a.Location, b.Location
		if c := cmp.Compare(kindRank(la.Kind), kindRank(lb.Kind)); c != 0 {
			return c
		}
		if la.Kind == KindYard {
			if c := strings.Compare(la.Yard, lb.Yard); c != 0 {
				return c
			}
			return naturalCmp(la.Track, lb.Track)
		}
		return naturalCmp(la.Key(), lb.Key())
	})
	for i := range out {
		g := &out[i]
		for _, s := range []*[]DiffEntry{&g.New, &g.Changed, &g.Removed, &g.KeptManual, &g.Unchanged} {
			slices.SortFunc(*s, func(a, b DiffEntry) int { return keyCmp(a.Key, b.Key) })
		}
	}
	return out
}

// naturalCmp compares strings so that runs of digits compare as numbers.
func naturalCmp(a, b string) int {
	for a != "" && b != "" {
		if isDigit(a[0]) && isDigit(b[0]) {
			na, nb := digitRun(a), digitRun(b)
			ta, tb := strings.TrimLeft(a[:na], "0"), strings.TrimLeft(b[:nb], "0")
			if c := cmp.Compare(len(ta), len(tb)); c != 0 {
				return c
			}
			if c := strings.Compare(ta, tb); c != 0 {
				return c
			}
			a, b = a[na:], b[nb:]
			continue
		}
		if a[0] != b[0] {
			return cmp.Compare(a[0], b[0])
		}
		a, b = a[1:], b[1:]
	}
	return cmp.Compare(len(a), len(b))
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

func digitRun(s string) int {
	n := 0
	for n < len(s) && isDigit(s[n]) {
		n++
	}
	return n
}

// ApplyImport applies a diff: it adds the new labels, replaces the changed
// run8 labels and removes the run8 labels the import dropped, then records the
// route files and the time of the import. It never touches a manual label,
// even one made since the diff was built. Callers apply it to a copy and save
// that.
func (m *Map) ApplyImport(diff ImportDiff, fp []run8route.FileFingerprint, at time.Time) {
	for _, e := range diff.New {
		if _, taken := m.Labels[e.Key]; taken || e.New == nil {
			continue
		}
		l := *e.New
		l.Source = SourceRun8
		m.put(e.Key, l)
	}
	for _, e := range diff.Changed {
		if cur, ok := m.Labels[e.Key]; !ok || cur.Source != SourceRun8 || e.New == nil {
			continue
		}
		l := *e.New
		l.Source = SourceRun8
		m.put(e.Key, l)
	}
	for _, e := range diff.Removed {
		if cur, ok := m.Labels[e.Key]; ok && cur.Source == SourceRun8 {
			delete(m.Labels, e.Key)
		}
	}
	m.Run8Import = &ImportMeta{At: at, Files: slices.Clone(fp)}
}
