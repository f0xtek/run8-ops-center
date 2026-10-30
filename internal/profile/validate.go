package profile

import (
	"fmt"
	"sort"
	"strings"
)

// Problem codes reported by validation.
const (
	CodeOrphanTag        = "orphan-tag"
	CodeMalformedTag     = "malformed-tag"
	CodeUnservedIndustry = "unserved-industry"
	CodeUnknownRef       = "unknown-ref"
	CodeDuplicateTag     = "duplicate-tag"
	CodeUnmatchedModel   = "unmatched-model"
	CodeUnmappedCarType  = "unmapped-car-type"
	CodeInvalidRun8      = "invalid-run8"
	CodeUnmatchedAnchor  = "unmatched-anchor"
	stopKindYard         = "yard"
	stopKindIndustry     = "industry"
)

// validate checks a built profile and returns every problem it finds. It
// never stops at the first problem.
func validate(p *Profile, rows []limitRow) Report {
	v := &validator{p: p, seen: map[string]bool{}}
	v.duplicates()
	v.refs()
	v.malformedAndOrphanTags(rows)
	v.unservedIndustries()
	v.unmappedCarTypes()
	v.anchorIndustries()
	return Report{Problems: v.out}
}

type validator struct {
	p    *Profile
	out  []Problem
	seen map[string]bool
}

// add records a problem once per code, subject and message.
func (v *validator) add(code, subject, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	key := code + "\x00" + subject + "\x00" + msg
	if v.seen[key] {
		return
	}
	v.seen[key] = true
	v.out = append(v.out, Problem{Code: code, Subject: subject, Message: msg})
}

// duplicates reports repeated incoming tags, job IDs, train symbols, class
// codes and repeated outgoing tags within one car-type row.
func (v *validator) duplicates() {
	p := v.p
	incoming := map[string]string{}
	for _, in := range p.Industries {
		if first, ok := incoming[in.Tag]; ok {
			v.add(CodeDuplicateTag, in.Tag, "incoming tag %s is used by both %s and %s", in.Tag, first, in.Name)
		} else {
			incoming[in.Tag] = in.Name
		}
		for _, c := range in.Cars {
			count := map[string]int{}
			for _, t := range c.OutgoingTags {
				count[t]++
			}
			for _, t := range c.OutgoingTags {
				if count[t] > 1 {
					v.add(CodeDuplicateTag, t, "outgoing tag %s is listed more than once for %s (%s)", t, in.Name, c.CarType)
					count[t] = 0
				}
			}
		}
	}

	ids := map[string]bool{}
	for _, j := range p.LocalJobs {
		if ids[j.ID] {
			v.add(CodeDuplicateTag, j.ID, "local job id %s is defined more than once", j.ID)
		}
		ids[j.ID] = true
	}

	symbols := map[string]bool{}
	codes := map[string]string{}
	for _, t := range p.RoadTrains {
		if symbols[t.Symbol] {
			v.add(CodeDuplicateTag, t.Symbol, "road train %s is defined more than once", t.Symbol)
		}
		symbols[t.Symbol] = true
		for _, c := range t.ClassCodes {
			if other, ok := codes[c]; ok && other != t.Symbol {
				v.add(CodeDuplicateTag, c, "class code %s is carried by both road train %s and %s", c, other, t.Symbol)
			}
			codes[c] = t.Symbol
		}
	}
}

// refs reports yards and industries named by jobs, trains and stops that do
// not exist.
func (v *validator) refs() {
	p := v.p
	yard := func(code, where string) {
		if p.YardByCode(code) == nil {
			v.add(CodeUnknownRef, code, "%s names unknown yard %q", where, code)
		}
	}
	industry := func(tag, where string) {
		if p.IndustryByTag(tag) == nil {
			v.add(CodeUnknownRef, tag, "%s names unknown industry %q", where, tag)
		}
	}

	for _, j := range p.LocalJobs {
		where := "local job " + j.Symbol
		yard(j.HomeYard, where+" home yard")
		for _, t := range j.Industries {
			industry(t, where)
		}
		for _, yl := range j.YardLines {
			yard(yl.Yard, where+" yard line")
		}
	}
	for _, t := range p.RoadTrains {
		where := "road train " + t.Symbol
		if t.Origin != "" {
			yard(t.Origin, where+" origin")
		}
		for _, tag := range t.OffMapIndustries {
			industry(tag, where+" off-map industries")
		}
		for _, s := range t.Stops {
			switch s.Kind {
			case stopKindYard:
				yard(s.Target, where+" stop")
			case stopKindIndustry:
				industry(s.Target, where+" stop")
			default:
				v.add(CodeUnknownRef, s.Target, "%s stop %q is neither a known yard nor a known industry", where, s.Target)
			}
		}
	}
}

// malformedAndOrphanTags checks every tag for whitespace, then reports
// outgoing tags that no train, stop, local or alias picks up.
func (v *validator) malformedAndOrphanTags(rows []limitRow) {
	p := v.p

	// Malformed tags: a tag with an internal space can never match a car.
	checkTag := func(tag, where string) bool {
		if strings.ContainsAny(tag, " \t") {
			v.add(CodeMalformedTag, tag, "tag %q in %s contains a space", tag, where)
			return false
		}
		return true
	}
	for _, in := range p.Industries {
		checkTag(in.Tag, "incoming tag of "+in.Name)
		for _, c := range in.Cars {
			for _, t := range c.OutgoingTags {
				checkTag(t, fmt.Sprintf("outgoing tags of %s (%s)", in.Name, c.CarType))
			}
		}
	}
	for _, j := range p.LocalJobs {
		for _, yl := range j.YardLines {
			for _, t := range yl.Carries {
				checkTag(t, fmt.Sprintf("local job %s yard line %s", j.Symbol, yl.Yard))
			}
		}
	}
	for _, t := range p.RoadTrains {
		for _, s := range t.Stops {
			for _, tag := range s.SetOut {
				checkTag(tag, fmt.Sprintf("road train %s stop %s set-out", t.Symbol, s.Target))
			}
			for _, tag := range s.PickUp {
				checkTag(tag, fmt.Sprintf("road train %s stop %s pick-up", t.Symbol, s.Target))
			}
		}
	}
	for from, to := range p.Aliases {
		checkTag(from, "alias")
		checkTag(to, "alias")
	}

	resolved := v.resolvedTags(rows)

	// Alias targets must themselves resolve.
	aliasKeys := make([]string, 0, len(p.Aliases))
	for k := range p.Aliases {
		aliasKeys = append(aliasKeys, k)
	}
	sort.Strings(aliasKeys)
	for _, from := range aliasKeys {
		to := p.Aliases[from]
		if !resolved[to] {
			v.add(CodeUnknownRef, from, "alias %s points to %s, which no road train, stop or local picks up", from, to)
		}
	}

	// Orphan outgoing tags, in order of first appearance.
	var order []string
	producers := map[string][]string{}
	for _, in := range p.Industries {
		for _, c := range in.Cars {
			for _, t := range c.OutgoingTags {
				if strings.ContainsAny(t, " \t") {
					continue // already reported as malformed
				}
				if _, ok := producers[t]; !ok {
					order = append(order, t)
				}
				label := in.Tag + " (" + in.Name + ")"
				if !contains(producers[t], label) {
					producers[t] = append(producers[t], label)
				}
			}
		}
	}
	for _, t := range order {
		target := t
		if a, ok := p.Aliases[t]; ok {
			target = a
		}
		if resolved[target] {
			continue
		}
		v.add(CodeOrphanTag, t, "outgoing tag %s matches no road train, stop pick-up, local or alias; produced by %s",
			t, summarise(producers[t]))
	}
}

// resolvedTags returns every tag some job or train handles: road train class
// codes, stop pick-up tags, local symbols, yard-line carries, the class codes
// of locals in the limits file, and yard-internal tags.
func (v *validator) resolvedTags(rows []limitRow) map[string]bool {
	p := v.p
	set := map[string]bool{}
	for _, t := range p.RoadTrains {
		for _, c := range t.ClassCodes {
			set[c] = true
		}
		for _, s := range t.Stops {
			for _, c := range s.PickUp {
				set[c] = true
			}
		}
	}
	localSyms := map[string]bool{}
	for _, j := range p.LocalJobs {
		localSyms[j.Symbol] = true
		set[j.Symbol] = true
		for _, yl := range j.YardLines {
			for _, c := range yl.Carries {
				set[c] = true
			}
		}
	}
	for _, r := range rows {
		if localSyms[r.Symbol] {
			for _, c := range r.Codes {
				set[c] = true
			}
		}
	}
	for _, t := range p.YardInternalTags {
		set[t] = true
	}
	return set
}

// unservedIndustries reports industries that no local job lists, no train
// stops at and no train carries off-map.
func (v *validator) unservedIndustries() {
	p := v.p
	served := map[string]bool{}
	for _, j := range p.LocalJobs {
		for _, t := range j.Industries {
			served[t] = true
		}
	}
	for _, t := range p.RoadTrains {
		for _, s := range t.Stops {
			if s.Kind == stopKindIndustry {
				served[s.Target] = true
			}
		}
		for _, o := range t.OffMapIndustries {
			served[o] = true
		}
	}
	for _, in := range p.Industries {
		if served[in.Tag] {
			continue
		}
		v.add(CodeUnservedIndustry, in.Tag, "industry %s (%s, local symbol %s) is served by no local job, road train stop or off-map road train",
			in.Tag, in.Name, orNone(in.LocalSymbol))
	}
}

// unmappedCarTypes reports industry car types that no car-type rule produces,
// because no car could ever match them.
func (v *validator) unmappedCarTypes() {
	p := v.p
	mapped := map[CarType]bool{}
	for _, r := range p.CarTypes {
		mapped[r.Type] = true
	}
	for _, in := range p.Industries {
		for _, c := range in.Cars {
			if !mapped[c.CarType] {
				v.add(CodeUnmappedCarType, string(c.CarType), "industry %s takes car type %s, but no car-type rule produces it", in.Tag, c.CarType)
			}
		}
	}
}

// UnmatchedModels returns one "unmatched-model" problem for each distinct
// model file name that matches no car-type rule, sorted by name. Callers pass
// the model names found in a save; locomotive models belong in the result
// unless the caller filters them out first.
func (p *Profile) UnmatchedModels(models []string) []Problem {
	seen := map[string]bool{}
	var names []string
	for _, m := range models {
		if seen[m] || p.CarTypeOf(m) != Unknown {
			continue
		}
		seen[m] = true
		names = append(names, m)
	}
	sort.Strings(names)
	out := make([]Problem, 0, len(names))
	for _, m := range names {
		out = append(out, Problem{
			Code:    CodeUnmatchedModel,
			Subject: m,
			Message: fmt.Sprintf("model %s matches no car-type rule", m),
		})
	}
	return out
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func summarise(tags []string) string { return strings.Join(tags, ", ") }

func orNone(s string) string {
	if s == "" {
		return "none"
	}
	return s
}
