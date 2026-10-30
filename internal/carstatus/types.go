// Package carstatus derives operational status for every car in a snapshot:
// type, load, destination, location, service state and dwell.
package carstatus

import (
	"time"

	"run8-ops-center/internal/profile"
	"run8-ops-center/internal/run8save"
)

// DestKind classifies a destination tag.
type DestKind string

const (
	DestIndustry   DestKind = "industry"   // "<local> <industry tag>" for an industry served by a local
	DestTrain      DestKind = "train"      // class code (via alias) or off-map industry: waits for a road train
	DestForeign    DestKind = "foreign"    // "<code> ."
	DestUnassigned DestKind = "unassigned" // "None", yard-internal tags
	DestUnknown    DestKind = "unknown"
)

// Dest is a classified destination tag.
type Dest struct {
	Kind     DestKind
	Raw      string
	Tag      string // whitespace-normalised
	Local    string // local symbol, for DestIndustry
	Industry string // industry tag, for DestIndustry and off-map DestTrain
	Train    string // road train symbol, for DestTrain
}

// LocKind classifies a car's location.
type LocKind string

const (
	LocYard     LocKind = "yard"
	LocIndustry LocKind = "industry"
	LocIgnored  LocKind = "ignored" // on line: labelled ignored
	LocUnmapped LocKind = "unmapped"
)

// Location is where a car sits.
type Location struct {
	Kind       LocKind
	Yard       string   // yard code, LocYard
	Track      string   // track name, LocYard
	Industries []string // industry tags, LocIndustry
	Prefix     int      // route prefix of the resolving section
	Section    int      // resolving section index
}

// String is a short human label such as "NOR / Bowl 14", "industry GES",
// "on line" or "unmapped 4535".
func (l Location) String() string { return locString(l) }

// Service is a car's service state.
type Service string

const (
	ServiceAwaitingDelivery Service = "awaiting-delivery" // industry-bound, in a yard
	ServiceWorking          Service = "working"           // at an industry, tagged for that same industry
	ServiceReadyToPull      Service = "ready-to-pull"     // at an industry, tagged for anything else
	ServiceAwaitingTrain    Service = "awaiting-train"    // train-bound, in a yard
	ServiceEnRoute          Service = "en-route"          // in a powered train on ignored track
	ServiceOther            Service = "other"
)

// Car is the derived status of one freight car.
type Car struct {
	Key         string // run8save.Vehicle.Key()
	Number      string // "ARMN 194887", or just the unit number when no mark
	Mark        string
	Model       string
	Type        profile.CarType
	Loaded      bool
	LoadTons    float64
	TareTons    float64
	GrossTons   float64
	LengthFt    float64
	Dest        Dest
	Loc         Location
	Service     Service
	TrainID     string
	TrainSymbol string    // powered train's symbol, "" for a cut
	DwellSince  time.Time // in-game date the car was first seen at its current location
}

// Status is the derived view of one snapshot.
type Status struct {
	Snapshot *run8save.Snapshot
	Cars     []Car
	ByKey    map[string]*Car // points into Cars
	Unmapped int             // cars with LocUnmapped
	Unknown  int             // cars with DestUnknown or type Unknown
}

// DwellRecord remembers where a car was last seen and since when.
type DwellRecord struct {
	Loc   string    `json:"loc"`   // Location key, stable across snapshots (kind + yard/track/industries or section)
	Since time.Time `json:"since"` // in-game date
}

// DwellState maps car key to its record. It is persisted in state.json.
type DwellState map[string]DwellRecord

// ChangeKind classifies a snapshot difference.
type ChangeKind string

const (
	ChangeAppeared    ChangeKind = "appeared"
	ChangeDisappeared ChangeKind = "disappeared"
	ChangeMoved       ChangeKind = "moved"
	ChangeRetagged    ChangeKind = "retagged"
)

// Change is one activity-log difference between two snapshots.
type Change struct {
	Kind    ChangeKind
	Key     string
	Number  string
	TrainID string
	OldTag  string // normalised
	NewTag  string // normalised
}

// API implemented in this package (tasks 5.1 to 5.4):
//
//	func Derive(snap *run8save.Snapshot, p *profile.Profile, tm *trackmap.Map, prev DwellState) (*Status, DwellState)
//	func Diff(prev, cur *run8save.Snapshot) []Change   // freight cars and locomotives; tags compared after profile.NormTag
//	func Mark(model string) string                     // reporting-mark derivation
//	func Classify(tag string, p *profile.Profile) Dest
//
// (Derive imports run8-ops-center/internal/trackmap.)

func locString(l Location) string {
	switch l.Kind {
	case LocYard:
		if l.Track != "" {
			return l.Yard + " / " + l.Track
		}
		return l.Yard
	case LocIndustry:
		s := "industry"
		for i, t := range l.Industries {
			if i == 0 {
				s += " "
			} else {
				s += "/"
			}
			s += t
		}
		return s
	case LocIgnored:
		return "on line"
	}
	return "unmapped"
}
