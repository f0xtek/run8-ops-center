// Package trackmap links Run8 track sections (route prefix + section index) to
// named yard tracks and industry tracks, and supports the learn mode.
package trackmap

import (
	"fmt"
	"strconv"
	"strings"
)

// Kind is the type of a labelled section.
type Kind string

const (
	KindYard     Kind = "yard"
	KindIndustry Kind = "industry"
	KindIgnored  Kind = "ignored" // mainline or other track that is not a location
)

// Label names one track section.
type Label struct {
	Kind       Kind     `json:"kind"`
	Yard       string   `json:"yard,omitempty"`       // yard code, for KindYard
	Track      string   `json:"track,omitempty"`      // track name, for KindYard (e.g. "Bowl 14")
	Industries []string `json:"industries,omitempty"` // industry tags, for KindIndustry
}

// Key renders a section as the "<prefix>/<section>" key used in trackmap.json.
func Key(prefix, section int) string { return fmt.Sprintf("%d/%d", prefix, section) }

// ParseKey is the inverse of Key.
func ParseKey(k string) (prefix, section int, err error) {
	a, b, ok := strings.Cut(k, "/")
	if !ok {
		return 0, 0, fmt.Errorf("bad section key %q", k)
	}
	if prefix, err = strconv.Atoi(a); err != nil {
		return 0, 0, err
	}
	section, err = strconv.Atoi(b)
	return prefix, section, err
}

// Map is the set of labels for a route profile. The zero value is usable for
// reads; use New for writes.
type Map struct {
	Labels map[string]Label `json:"labels"`
}

// New returns an empty map.
func New() *Map { return &Map{Labels: map[string]Label{}} }

// Lookup returns the label for a section.
func (m *Map) Lookup(prefix, section int) (Label, bool) {
	if m == nil {
		return Label{}, false
	}
	l, ok := m.Labels[Key(prefix, section)]
	return l, ok
}

// API implemented in this package (tasks 4.1 to 4.3), beyond the above:
//
//	func Load(path string) (*Map, error)               // missing file -> empty map
//	func (m *Map) Save(w FileWriter, path string) error // atomic; FileWriter as in profile
//	func (m *Map) Set(prefix, section int, l Label)
//	func (m *Map) Remove(prefix, section int)
//	func (m *Map) Candidates(snap *run8save.Snapshot, p *profile.Profile) []Candidate
//	func (m *Map) Apply(c Candidate, l Label)          // labels every unmapped section of c
