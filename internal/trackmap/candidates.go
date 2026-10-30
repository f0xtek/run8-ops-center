package trackmap

import (
	"fmt"
	"sort"
	"strings"

	"run8-ops-center/internal/profile"
	"run8-ops-center/internal/run8save"
)

// maxSamples is how many unit numbers a Candidate lists.
const maxSamples = 5

// Suggestion sources.
const (
	SourceTags      = "tags"      // every car tagged for one industry
	SourceNeighbour = "neighbour" // an adjacent section index is labelled
)

// Suggestion is a proposed label for a candidate. It is never applied
// automatically.
type Suggestion struct {
	Label  Label
	Reason string // human-readable; neighbour suggestions say they are only a hint
	Source string // SourceTags or SourceNeighbour
}

// TagCount is a destination tag and how many cars carry it.
type TagCount struct {
	Tag   string
	Count int
}

// Candidate is one train (or cut) that occupies at least one unmapped section.
type Candidate struct {
	TrainID    string
	Powered    bool         // the train has a locomotive
	Symbol     string       // first locomotive's destination tag, "" for a cut
	Sections   []SectionRef // unmapped sections it occupies, sorted
	Cars       int          // non-locomotive vehicles
	Samples    []string     // up to 5 unit numbers (cars; locomotives when it has no cars)
	Tags       []TagCount   // distinct non-blank car tags, most common first
	Suggestion *Suggestion  // nil when there is nothing to suggest
}

// Candidates returns every train in the snapshot that occupies at least one
// unmapped section, in snapshot order. A section carrying any label, including
// ignored, is mapped. p may be nil, which disables tag suggestions.
func (m *Map) Candidates(snap *run8save.Snapshot, p *profile.Profile) []Candidate {
	if snap == nil {
		return nil
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
		c.Suggestion = m.suggest(c, cars, p)
		out = append(out, c)
	}
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

// suggest prefers the tag-majority suggestion over the neighbour hint.
func (m *Map) suggest(c Candidate, cars []run8save.Vehicle, p *profile.Profile) *Suggestion {
	if s := tagSuggestion(cars, p); s != nil {
		return s
	}
	return m.neighbourSuggestion(c.Sections)
}

// tagSuggestion fires when every car carries a tag "<local> <industryTag>" (or
// the bare industry tag) for the same profile industry.
func tagSuggestion(cars []run8save.Vehicle, p *profile.Profile) *Suggestion {
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
	return &Suggestion{
		Label:  Label{Kind: KindIndustry, Industries: []string{found.Tag}},
		Reason: fmt.Sprintf("all %d cars are tagged for %s (%s)", len(cars), found.Tag, found.Name),
		Source: SourceTags,
	}
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

// neighbourSuggestion proposes the most common label on sections adjacent
// (index +/-1, same prefix) to the candidate's unmapped sections.
func (m *Map) neighbourSuggestion(secs []SectionRef) *Suggestion {
	type vote struct {
		label Label
		n     int
		from  SectionRef
		near  SectionRef
	}
	votes := map[string]*vote{}
	var order []string
	for _, s := range secs {
		for _, d := range [2]int{-1, 1} {
			l, ok := m.Lookup(s.Prefix, s.Section+d)
			if !ok {
				continue
			}
			k := l.Location().Key()
			v := votes[k]
			if v == nil {
				v = &vote{label: l, from: s, near: SectionRef{s.Prefix, s.Section + d}}
				votes[k] = v
				order = append(order, k)
			}
			v.n++
		}
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
	return &Suggestion{
		Label: cloneLabel(best.label),
		Reason: fmt.Sprintf("hint only: section %s is next to %s, which is %s; section numbers are not guaranteed to be adjacent on the ground",
			best.from.Key(), best.near.Key(), best.label.Location()),
		Source: SourceNeighbour,
	}
}

// Apply labels every still-unmapped section of the candidate with l. Sections
// that have gained a label since the candidate was computed are left alone.
func (m *Map) Apply(c Candidate, l Label) {
	for _, s := range c.Sections {
		if _, ok := m.Lookup(s.Prefix, s.Section); ok {
			continue
		}
		m.Set(s.Prefix, s.Section, l)
	}
}
