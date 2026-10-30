// Package run8save decodes Run8 world save files (ScnLoader XML) and their
// .info sidecars (ScnInfo) into a Snapshot. It has no route knowledge.
package run8save

import (
	"strings"
	"time"
)

// SaveType is the SaveType value in a .info sidecar.
type SaveType string

const (
	SaveWorld      SaveType = "World"
	SaveTrain      SaveType = "Train"
	SaveExtraBoard SaveType = "ExtraBoard"
)

// Info is a decoded .info sidecar (ScnInfo).
type Info struct {
	SaveType        SaveType
	Filename        string
	DisplayFilename string
	FileTimestamp   time.Time // zero when the element is absent
}

// Truck is one entry of the two-element per-vehicle position arrays.
type Truck struct {
	RoutePrefix int
	Section     int // currentTrackSectionIndex
	StartNode   int
	DistanceM   float64
	Reverse     bool
}

// Vehicle is one RailVehicleStateClass.
type Vehicle struct {
	Model      string // rvXMLfilename, e.g. "R8_Reefer_PCF_57_rbld_ARMN.xml"
	UnitType   string // "US_Freightcar" or "US_DieselEngine"
	UnitNumber string
	LoadTons   float64
	Tag        string  // raw destinationTag, untouched
	Hazmat     int     // hazmatPlacardIndex
	Trucks     []Truck // index 0 is the leading truck
}

// Key identifies a vehicle across snapshots.
func (v Vehicle) Key() string { return v.Model + "#" + v.UnitNumber }

// IsLoco reports whether the vehicle is a locomotive.
func (v Vehicle) IsLoco() bool { return v.UnitType == "US_DieselEngine" }

// Train is one TrainLoader. A train with no locomotive is a cut of cars.
type Train struct {
	ID                string
	AI                bool
	DispatchDirection int
	Vehicles          []Vehicle
}

// Powered reports whether the train has at least one locomotive.
func (t Train) Powered() bool {
	for _, v := range t.Vehicles {
		if v.IsLoco() {
			return true
		}
	}
	return false
}

// Symbol is the first locomotive's destination tag, or "" when the train is a
// cut or that tag is "None" or blank.
func (t Train) Symbol() string {
	for _, v := range t.Vehicles {
		if v.IsLoco() {
			s := strings.Join(strings.Fields(v.Tag), " ")
			if s == "" || strings.EqualFold(s, "None") {
				return ""
			}
			return s
		}
	}
	return ""
}

// Snapshot is one parsed save.
type Snapshot struct {
	FileName    string    // base name without .xml, "" when parsed from a reader
	WriteTime   time.Time // sidecar FileTimestamp when present, else file mtime
	GameDate    time.Time // <date>
	SaveType    SaveType  // <saveGameType>
	Description string    // optional
	Trains      []Train
}

// Cars returns every non-locomotive vehicle.
func (s *Snapshot) Cars() []Vehicle {
	var out []Vehicle
	for _, t := range s.Trains {
		for _, v := range t.Vehicles {
			if !v.IsLoco() {
				out = append(out, v)
			}
		}
	}
	return out
}

// CarKeys is the set of freight-car keys (locomotives excluded).
func (s *Snapshot) CarKeys() map[string]struct{} {
	out := map[string]struct{}{}
	for _, v := range s.Cars() {
		out[v.Key()] = struct{}{}
	}
	return out
}

// API implemented in this package (see tasks 2.1 to 2.4):
//
//	func InfoPath(xmlPath string) string                 // "x.xml" -> "x.info"
//	func ReadInfo(path string) (Info, error)             // path is the .info file
//	func Parse(r io.Reader) (*Snapshot, error)           // streaming; any XML error -> nil, err
//	func ParseFile(path string) (*Snapshot, error)       // sets FileName and WriteTime
