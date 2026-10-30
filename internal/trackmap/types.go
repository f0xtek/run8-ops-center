// Package trackmap links Run8 track sections (route prefix + section index) to
// named yard tracks and industry tracks, and supports the learn mode.
package trackmap

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"run8-ops-center/internal/run8route"
)

// Kind is the type of a labelled section.
type Kind string

const (
	KindYard     Kind = "yard"
	KindIndustry Kind = "industry"
	KindIgnored  Kind = "ignored" // mainline or other track that is not a location
)

// SourceRun8 is the Label.Source of a label an import from Run8 route data
// added. The empty source is a manual label: one the user made, and every
// label read from a track map that predates label sources.
const SourceRun8 = "run8"

// Label names one track section.
type Label struct {
	Kind       Kind     `json:"kind"`
	Yard       string   `json:"yard,omitempty"`       // yard code, for KindYard
	Track      string   `json:"track,omitempty"`      // track name, for KindYard (e.g. "Bowl 14")
	Industries []string `json:"industries,omitempty"` // industry tags, for KindIndustry
	Source     string   `json:"source,omitempty"`     // "" for a manual label, SourceRun8 for an imported one
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

// ImportMeta records which Run8 route files the last import was built from.
type ImportMeta struct {
	At    time.Time
	Files []run8route.FileFingerprint
}

// importMetaJSON is the stored form of ImportMeta. run8route's fingerprint
// carries no JSON tags, so the stored names are set here.
type importMetaJSON struct {
	At    time.Time         `json:"at"`
	Files []fileFingerprint `json:"files"`
}

type fileFingerprint struct {
	Path    string    `json:"path"`
	Size    int64     `json:"size"`
	ModTime time.Time `json:"modTime"`
}

// MarshalJSON stores the import time and the files as {at, files:[{path,
// size, modTime}]}.
func (im ImportMeta) MarshalJSON() ([]byte, error) {
	w := importMetaJSON{At: im.At, Files: make([]fileFingerprint, len(im.Files))}
	for i, f := range im.Files {
		w.Files[i] = fileFingerprint{f.Path, f.Size, f.ModTime}
	}
	return json.Marshal(w)
}

// UnmarshalJSON reads what MarshalJSON wrote.
func (im *ImportMeta) UnmarshalJSON(data []byte) error {
	var w importMetaJSON
	if err := json.Unmarshal(data, &w); err != nil {
		return err
	}
	im.At = w.At
	im.Files = make([]run8route.FileFingerprint, len(w.Files))
	for i, f := range w.Files {
		im.Files[i] = run8route.FileFingerprint{Path: f.Path, Size: f.Size, ModTime: f.ModTime}
	}
	return nil
}

// Map is the set of labels for a route profile. The zero value is usable for
// reads; use New for writes.
type Map struct {
	Labels map[string]Label `json:"labels"`
	// Run8Import is nil until the first import.
	Run8Import *ImportMeta `json:"run8Import,omitempty"`
}

// New returns an empty map.
func New() *Map { return &Map{Labels: map[string]Label{}} }

// Clone returns a deep copy of the map, so a caller can change it without
// touching a published one.
func (m *Map) Clone() *Map {
	out := New()
	if m == nil {
		return out
	}
	for k, l := range m.Labels {
		out.Labels[k] = cloneLabel(l)
	}
	if m.Run8Import != nil {
		out.Run8Import = &ImportMeta{At: m.Run8Import.At, Files: append([]run8route.FileFingerprint(nil), m.Run8Import.Files...)}
	}
	return out
}

// Revision is a stable hash of every label and of the import metadata. It
// changes whenever either does, so a preview built from one revision can be
// told apart from the map it was applied to.
func (m *Map) Revision() string {
	var snap Map
	if m != nil {
		snap = *m
	}
	if snap.Labels == nil {
		snap.Labels = map[string]Label{}
	}
	data, err := json.Marshal(snap) // map keys are sorted, so the bytes are stable
	if err != nil {
		return "" // unreachable: every field is a plain value
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// Lookup returns the label for a section.
func (m *Map) Lookup(prefix, section int) (Label, bool) {
	if m == nil {
		return Label{}, false
	}
	l, ok := m.Labels[Key(prefix, section)]
	return l, ok
}

// API implemented in this package, beyond the above:
//
//	func Load(path string) (*Map, error)               // missing file -> empty map
//	func (m *Map) Save(w FileWriter, path string) error // atomic; FileWriter as in profile
//	func (m *Map) Set(prefix, section int, l Label)     // always a manual label
//	func (m *Map) Remove(prefix, section int)
//	func (m *Map) Candidates(snap *run8save.Snapshot, p *profile.Profile, route *run8route.Data) []Candidate
//	func (m *Map) Apply(c Candidate, l Label)          // labels every unmapped section of c.Track, as manual labels
//	func BuildImport(d run8route.Data, p *profile.Profile) ImportPlan
//	func (m *Map) Diff(plan ImportPlan) ImportDiff
//	func (m *Map) ApplyImport(diff ImportDiff, fp []run8route.FileFingerprint, at time.Time)
