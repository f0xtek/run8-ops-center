// Package run8route reads the Run8 route files that name track sections:
// Config.ind (industries), TrackDatabase.r8 (the track graph),
// HumpControllerList.r8, AISpecialLocations.r8 and MilepostDatabase.r8.
//
// It is a leaf package: it imports nothing from this project. It only reads.
// Each decoder accepts a file only when it decodes exactly to its last byte,
// and checks every count against the bytes left before it allocates.
package run8route

import (
	"strings"
	"time"
)

// File names, as they appear in FileStatus.Name.
const (
	ConfigFile    = "Config.ind"
	TrackDBFile   = "TrackDatabase.r8"
	HumpFile      = "HumpControllerList.r8"
	LocationsFile = "AISpecialLocations.r8"
	MilepostsFile = "MilepostDatabase.r8"
)

// Industry is one Config.ind industry. Name, Local and Tag are raw: Run8 pads
// some tags (" BRR"), so callers normalise with NormTag before comparing.
type Industry struct {
	Name, Local, Tag string
	Tracks           []IndustryTrack
}

// IndustryTrack is one section an industry lists.
type IndustryTrack struct{ Prefix, Section, Node int }

// HumpSwitch is one switch a hump path throws, and the leg it takes.
type HumpSwitch struct {
	Section  int
	Reversed bool
}

// HumpTrack is one bowl track: its name and the switches that lead to it.
type HumpTrack struct {
	Name     string
	Switches []HumpSwitch
}

// HumpController is one hump controller and its bowl track paths.
type HumpController struct {
	Name   string
	Tracks []HumpTrack
}

// SpecialLocation is one AISpecialLocations.r8 entry.
type SpecialLocation struct {
	Name                  string
	Type, Prefix, Section int
}

// Milepost is one MilepostDatabase.r8 entry. Name is the first string in the
// record and Station the second; the real files put the label in Station.
// X and Z are world metres, from the same tile formula as Section.
// They are only meaningful when the track database loaded, because the tile
// size is estimated from it.
type Milepost struct {
	Name, Station string
	X, Z          float64
}

// Data is everything Load decoded. A field is nil or empty when its file
// failed.
type Data struct {
	Industries []Industry
	Topology   *Topology
	Hump       []HumpController
	Locations  []SpecialLocation
	Mileposts  []Milepost
}

// FileFingerprint identifies one file's state.
type FileFingerprint struct {
	Path    string
	Size    int64
	ModTime time.Time
}

// FileStatus is the outcome for one file. Name is one of the *File constants.
type FileStatus struct {
	Name, Path string
	OK         bool
	Err        error
}

// Result is the outcome of Load.
type Result struct {
	Data        Data
	Files       []FileStatus
	Fingerprint []FileFingerprint
}

// Route is one route folder under Content/V3Routes.
type Route struct{ Folder string }

// NormTag collapses runs of whitespace and trims the ends, so " BRR" and
// "AB10  BRR" compare with their profile spellings.
func NormTag(s string) string { return strings.Join(strings.Fields(s), " ") }
