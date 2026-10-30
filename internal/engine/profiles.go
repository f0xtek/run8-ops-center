package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"run8-ops-center/internal/profile"
	"run8-ops-center/internal/store"
	"run8-ops-center/internal/trackmap"
)

// scanProfiles loads every profile folder under dir. Folders that cannot be
// loaded are reported in problems and skipped. The result is sorted by ID; the
// first folder wins when two declare the same ID.
func scanProfiles(dir string) (found []*profile.Profile, problems []string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, []string{fmt.Sprintf("cannot read profiles folder %s: %v", dir, err)}
	}
	seen := map[string]bool{}
	for _, ent := range entries {
		if !ent.IsDir() {
			continue
		}
		pdir := filepath.Join(dir, ent.Name())
		if _, err := os.Stat(filepath.Join(pdir, profile.ProfileFile)); err != nil {
			continue
		}
		p, err := profile.Load(pdir)
		if err != nil {
			problems = append(problems, fmt.Sprintf("profile folder %s could not be loaded: %v", ent.Name(), err))
			continue
		}
		if p.ID == "" {
			problems = append(problems, fmt.Sprintf("profile folder %s has no id", ent.Name()))
			continue
		}
		if seen[p.ID] {
			problems = append(problems, fmt.Sprintf("profile folder %s repeats the id %q and is ignored", ent.Name(), p.ID))
			continue
		}
		seen[p.ID] = true
		found = append(found, p)
	}
	sort.Slice(found, func(i, j int) bool { return found[i].ID < found[j].ID })
	return found, problems
}

// pickProfile chooses the profile to start with: the one in settings, else the
// Config one, else the default. fellBack is true when settings named a profile
// that is not installed and another one was used.
func pickProfile(found []*profile.Profile, settingsID, cfgID string) (p *profile.Profile, fellBack bool) {
	byID := func(id string) *profile.Profile {
		for _, c := range found {
			if c.ID == id {
				return c
			}
		}
		return nil
	}
	if settingsID != "" {
		if p := byID(settingsID); p != nil {
			return p, false
		}
		fellBack = true
	}
	if cfgID != "" {
		if p := byID(cfgID); p != nil {
			return p, fellBack
		}
		return nil, fellBack
	}
	return byID(DefaultProfileID), fellBack
}

func infoOf(found []*profile.Profile) []ProfileInfo {
	out := make([]ProfileInfo, 0, len(found))
	for _, p := range found {
		out = append(out, ProfileInfo{ID: p.ID, Name: p.Name})
	}
	return out
}

// prepareProfile checks the profile folder against the guard and loads its
// track map. It changes nothing in the engine.
func (e *Engine) prepareProfile(p *profile.Profile) (*trackmap.Map, error) {
	if err := store.ValidateProfilePath(p.Dir, e.guard); err != nil {
		return nil, err
	}
	tm, err := trackmap.Load(filepath.Join(p.Dir, trackMapFile))
	if err != nil {
		return nil, fmt.Errorf("profile %s: %w", p.ID, err)
	}
	return tm, nil
}

// installProfile makes p the active profile.
func (e *Engine) installProfile(p *profile.Profile, tm *trackmap.Map) {
	if e.base != nil && e.base.Dir != p.Dir {
		e.guard.UnregisterProfilePath(e.base.Dir)
	}
	e.guard.RegisterProfilePath(p.Dir)
	e.base = p
	e.tm = tm
	e.anchorProblems = nil // the caller reloads route data for p
	e.eff = e.effectiveProfile()
}

func (e *Engine) adoptProfile(p *profile.Profile) error {
	tm, err := e.prepareProfile(p)
	if err != nil {
		return err
	}
	e.installProfile(p, tm)
	return nil
}

// profileWarnings sets the warning for the active profile's validation report
// (the profile's own problems and the anchor milepost findings), replacing an
// earlier one whose count may be out of date.
func (e *Engine) profileWarnings() {
	prefix := fmt.Sprintf("profile %s has ", e.base.ID)
	if slices.ContainsFunc(e.warnings, func(w Warning) bool { return w.Kind == "profile" && strings.HasPrefix(w.Message, prefix) }) {
		e.warnings = slices.DeleteFunc(slices.Clone(e.warnings), func(w Warning) bool {
			return w.Kind == "profile" && strings.HasPrefix(w.Message, prefix)
		})
	}
	if n := len(e.eff.Report.Problems); n > 0 {
		e.addWarning("profile", fmt.Sprintf("%s%d validation problem(s); see the setup page", prefix, n))
	}
}

// effectiveProfile returns the base profile with the threshold overrides
// applied and the anchor milepost findings added to its report. Neither the
// base profile nor its report is modified.
func (e *Engine) effectiveProfile() *profile.Profile {
	p := effective(e.base, e.settings.Thresholds)
	if len(e.anchorProblems) > 0 {
		p.Report.Problems = append(slices.Clone(e.base.Report.Problems), e.anchorProblems...)
	}
	return p
}

// effective returns a copy of base with the per-train threshold overrides
// applied to its road trains. base itself is never modified.
func effective(base *profile.Profile, th map[string]store.Threshold) *profile.Profile {
	p := *base
	p.RoadTrains = slices.Clone(base.RoadTrains)
	for i := range p.RoadTrains {
		t, ok := th[p.RoadTrains[i].Symbol]
		if !ok {
			continue
		}
		if t.MinLengthPct != nil {
			p.RoadTrains[i].MinLengthPct = *t.MinLengthPct
		}
		if t.MaxCarAgeDays != nil {
			p.RoadTrains[i].MaxCarAgeDays = *t.MaxCarAgeDays
		}
	}
	return &p
}

func cloneSettings(s store.Settings) store.Settings {
	if s.Thresholds != nil {
		m := make(map[string]store.Threshold, len(s.Thresholds))
		for k, t := range s.Thresholds {
			m[k] = cloneThreshold(t)
		}
		s.Thresholds = m
	}
	return s
}

func cloneThreshold(t store.Threshold) store.Threshold {
	if t.MinLengthPct != nil {
		v := *t.MinLengthPct
		t.MinLengthPct = &v
	}
	if t.MaxCarAgeDays != nil {
		v := *t.MaxCarAgeDays
		t.MaxCarAgeDays = &v
	}
	return t
}
