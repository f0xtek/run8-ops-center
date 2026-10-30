package trackmap

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"run8-ops-center/internal/run8save"
)

// FileWriter is how this package writes trackmap.json. The store package
// supplies an implementation that enforces the protected-path guard. Save
// needs Rename so the file is replaced atomically.
type FileWriter interface {
	MkdirAll(path string, perm os.FileMode) error
	WriteFile(path string, data []byte, perm os.FileMode) error
	Rename(oldpath, newpath string) error
}

// SectionRef identifies one track section.
type SectionRef struct {
	Prefix  int `json:"prefix"`
	Section int `json:"section"`
}

// Key renders the reference as "<prefix>/<section>".
func (s SectionRef) Key() string { return Key(s.Prefix, s.Section) }

func sortSections(s []SectionRef) {
	sort.Slice(s, func(i, j int) bool {
		if s[i].Prefix != s[j].Prefix {
			return s[i].Prefix < s[j].Prefix
		}
		return s[i].Section < s[j].Section
	})
}

// Load reads trackmap.json. A missing file yields an empty map. Malformed
// JSON, a bad section key, an unknown label kind or an unknown label source
// is an error. A label without a source is manual.
func Load(path string) (*Map, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return New(), nil
	}
	if err != nil {
		return nil, fmt.Errorf("read track map: %w", err)
	}
	m := New()
	if err := json.Unmarshal(data, m); err != nil {
		return nil, fmt.Errorf("parse track map %s: %w", path, err)
	}
	if m.Labels == nil {
		m.Labels = map[string]Label{}
	}
	for k, l := range m.Labels {
		if _, _, err := ParseKey(k); err != nil {
			return nil, fmt.Errorf("track map %s: %w", path, err)
		}
		switch l.Kind {
		case KindYard, KindIndustry, KindIgnored:
		default:
			return nil, fmt.Errorf("track map %s: section %s has unknown kind %q", path, k, l.Kind)
		}
		switch l.Source {
		case "", SourceRun8:
		default:
			return nil, fmt.Errorf("track map %s: section %s has unknown source %q", path, k, l.Source)
		}
	}
	return m, nil
}

// Save writes the map to path atomically: the JSON goes to a temp file in the
// same directory, which is then renamed over path.
func (m *Map) Save(w FileWriter, path string) error {
	labels := map[string]Label{}
	if m != nil && m.Labels != nil {
		labels = m.Labels
	}
	var meta *ImportMeta
	if m != nil {
		meta = m.Run8Import
	}
	data, err := json.MarshalIndent(Map{Labels: labels, Run8Import: meta}, "", "  ")
	if err != nil {
		return fmt.Errorf("encode track map: %w", err)
	}
	data = append(data, '\n')
	if err := w.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create track map dir: %w", err)
	}
	tmp := path + ".tmp"
	if err := w.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write track map: %w", err)
	}
	if err := w.Rename(tmp, path); err != nil {
		return fmt.Errorf("replace track map: %w", err)
	}
	return nil
}

func cloneLabel(l Label) Label {
	l.Industries = slices.Clone(l.Industries)
	return l
}

// Set labels a section, replacing any existing label. The label is stored as
// a manual label whatever l.Source says, because Set is the path user
// commands take. Imports go through ApplyImport.
func (m *Map) Set(prefix, section int, l Label) {
	l.Source = ""
	m.put(Key(prefix, section), l)
}

// put stores a label under key, keeping its source.
func (m *Map) put(key string, l Label) {
	if m.Labels == nil {
		m.Labels = map[string]Label{}
	}
	m.Labels[key] = cloneLabel(l)
}

// Remove deletes a section's label, making it unmapped again.
func (m *Map) Remove(prefix, section int) {
	delete(m.Labels, Key(prefix, section))
}

// Location is the place a label names: a yard track, a set of industry tags or
// the ignored group. It shares Label's fields; Key gives it identity.
type Location Label

// Location returns the location a label names. The location has no source:
// where a label came from is not part of where it puts a section.
func (l Label) Location() Location {
	l.Source = ""
	return Location(cloneLabel(l))
}

// Label returns the label that places a section at the location. It is a
// manual label, so renaming or relabelling a location makes the labels
// manual.
func (l Location) Label() Label {
	l.Source = ""
	return cloneLabel(Label(l))
}

// Key is a stable identity for the location. Industry tag order does not
// matter.
func (l Location) Key() string {
	switch l.Kind {
	case KindYard:
		return "yard|" + l.Yard + "|" + l.Track
	case KindIndustry:
		tags := slices.Clone(l.Industries)
		sort.Strings(tags)
		return "industry|" + strings.Join(tags, ",")
	default:
		return string(l.Kind)
	}
}

// String renders the location for display, e.g. "Norris / Bowl 14", "GES".
func (l Location) String() string {
	switch l.Kind {
	case KindYard:
		return l.Yard + " / " + l.Track
	case KindIndustry:
		return strings.Join(l.Industries, ", ")
	default:
		return "Ignored"
	}
}

// LocationGroup is one location with the sections labelled for it.
type LocationGroup struct {
	Location Location
	Sections []SectionRef // sorted by prefix, then section
}

func kindRank(k Kind) int {
	switch k {
	case KindYard:
		return 0
	case KindIndustry:
		return 1
	}
	return 2
}

// Locations groups labelled sections by location. Groups are ordered yards
// first, then industries, then ignored, each by name; sections are sorted.
func (m *Map) Locations() []LocationGroup {
	if m == nil {
		return nil
	}
	idx := map[string]int{}
	var out []LocationGroup
	for k, l := range m.Labels {
		p, s, err := ParseKey(k)
		if err != nil {
			continue
		}
		loc := l.Location()
		lk := loc.Key()
		i, ok := idx[lk]
		if !ok {
			i = len(out)
			idx[lk] = i
			out = append(out, LocationGroup{Location: loc})
		}
		out[i].Sections = append(out[i].Sections, SectionRef{p, s})
	}
	for i := range out {
		sortSections(out[i].Sections)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i].Location, out[j].Location
		if ra, rb := kindRank(a.Kind), kindRank(b.Kind); ra != rb {
			return ra < rb
		}
		return a.Key() < b.Key()
	})
	return out
}

// Rename relabels every section at location old to location new and returns
// how many sections changed.
func (m *Map) Rename(old, new Location) int {
	if m == nil {
		return 0
	}
	ok := old.Key()
	n := 0
	for k, l := range m.Labels {
		if l.Location().Key() == ok {
			m.Labels[k] = new.Label()
			n++
		}
	}
	return n
}

// Coverage counts snapshot cars by mapping state.
type Coverage struct {
	Cars     int // non-locomotive vehicles
	Unmapped int // cars whose location does not resolve to a label
}

// Coverage counts how many cars in the snapshot sit on unmapped sections. A
// car resolves through its first truck's section, then its second truck's;
// when neither is labelled (an ignored label counts as labelled) the car is
// unmapped.
func (m *Map) Coverage(snap *run8save.Snapshot) Coverage {
	var c Coverage
	if snap == nil {
		return c
	}
	for _, t := range snap.Trains {
		for _, v := range t.Vehicles {
			if v.IsLoco() {
				continue
			}
			c.Cars++
			if !m.carMapped(v) {
				c.Unmapped++
			}
		}
	}
	return c
}

func (m *Map) carMapped(v run8save.Vehicle) bool {
	for i := range min(len(v.Trucks), 2) {
		if _, ok := m.Lookup(v.Trucks[i].RoutePrefix, v.Trucks[i].Section); ok {
			return true
		}
	}
	return false
}
