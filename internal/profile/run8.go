package profile

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

// buildRun8 turns the run8 block of profile.yaml into a Run8Config. It
// reports every invalid entry and leaves it out. An invalid region drops the
// whole config, because no route data can be located without it. It returns
// nil when the profile has no run8 block.
func buildRun8(p *Profile, s *run8Schema) (*Run8Config, []Problem) {
	if s == nil {
		return nil, nil
	}
	v := &validator{p: p, seen: map[string]bool{}}

	region := strings.TrimSpace(s.Region)
	if reason := folderNameProblem(region); reason != "" {
		v.add(CodeInvalidRun8, s.Region, "run8 region %q is not usable: %s", s.Region, reason)
		return nil, v.out
	}
	cfg := &Run8Config{Region: region}

	for _, r := range s.Routes {
		folder := strings.TrimSpace(r.Folder)
		if reason := folderNameProblem(folder); reason != "" {
			v.add(CodeInvalidRun8, r.Folder, "run8 route folder %q is not usable: %s", r.Folder, reason)
			continue
		}
		if !hasPrefix(p.RoutePrefixes, r.Prefix) {
			v.add(CodeInvalidRun8, folder, "run8 route %q has prefix %d, which is not in routePrefixes", folder, r.Prefix)
			continue
		}
		cfg.Routes = append(cfg.Routes, Run8Route{Folder: folder, Prefix: r.Prefix})
	}

	for _, h := range s.Humps {
		controller := strings.TrimSpace(h.Controller)
		yard := strings.TrimSpace(h.Yard)
		if controller == "" {
			v.add(CodeInvalidRun8, "", "run8 hump for yard %q has no controller name", yard)
			continue
		}
		if p.YardByCode(yard) == nil {
			v.add(CodeUnknownRef, yard, "run8 hump %q names unknown yard %q", controller, yard)
			continue
		}
		cfg.Humps = append(cfg.Humps, Run8Hump{Controller: controller, Yard: yard, TrackPrefix: strings.TrimSpace(h.TrackPrefix)})
	}

	for _, e := range s.TrackEnds {
		yard := strings.TrimSpace(e.Yard)
		re, err := regexp.Compile(e.Pattern)
		if err != nil {
			v.add(CodeInvalidRun8, e.Pattern, "run8 trackEnds pattern %q does not compile: %v", e.Pattern, err)
			continue
		}
		if !contains(re.SubexpNames(), "n") || !contains(re.SubexpNames(), "end") {
			v.add(CodeInvalidRun8, e.Pattern, "run8 trackEnds pattern %q needs the named groups n and end", e.Pattern)
			continue
		}
		if p.YardByCode(yard) == nil {
			v.add(CodeUnknownRef, yard, "run8 trackEnds pattern %q names unknown yard %q", e.Pattern, yard)
			continue
		}
		cfg.TrackEnds = append(cfg.TrackEnds, Run8TrackEnds{Pattern: re, Yard: yard, TrackPrefix: strings.TrimSpace(e.TrackPrefix)})
	}
	return cfg, v.out
}

// folderNameProblem returns why name cannot be used as a single Run8 folder
// name, or "" when it can. A name must be non-empty, relative, free of path
// separators and drive colons, and must not contain "..".
func folderNameProblem(name string) string {
	switch {
	case name == "":
		return "the name is empty"
	case filepath.IsAbs(name):
		return "the name is an absolute path"
	case strings.ContainsAny(name, `/\:`+"\x00"):
		return "the name contains a path separator or drive colon"
	case strings.Contains(name, ".."):
		return `the name contains ".."`
	case name == ".":
		return `the name is "."`
	}
	return ""
}

func hasPrefix(list []int, n int) bool {
	for _, x := range list {
		if x == n {
			return true
		}
	}
	return false
}

// anchorIndustries reports yard anchor industries that are not profile
// industries.
func (v *validator) anchorIndustries() {
	for _, y := range v.p.Yards {
		for _, tag := range y.AnchorIndustries {
			if v.p.IndustryByTag(tag) == nil {
				v.add(CodeUnknownRef, tag, "yard %s anchor industry %q is not a profile industry", y.Code, tag)
			}
		}
	}
}

// ValidateAnchors returns one "unmatched-anchor" problem for each yard
// anchor milepost that matches none of the given Run8 milepost station names.
// Matching is exact after whitespace normalisation. The engine calls it once
// route data has been read.
func (p *Profile) ValidateAnchors(mileposts []string) []Problem {
	known := make(map[string]bool, len(mileposts))
	for _, m := range mileposts {
		known[NormTag(m)] = true
	}
	var out []Problem
	for _, y := range p.Yards {
		for _, name := range y.AnchorMileposts {
			if !known[NormTag(name)] {
				out = append(out, Problem{
					Code:    CodeUnmatchedAnchor,
					Subject: name,
					Message: fmt.Sprintf("yard %s anchor milepost %q matches no milepost station name", y.Code, name),
				})
			}
		}
	}
	return out
}
