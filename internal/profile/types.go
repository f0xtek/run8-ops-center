// Package profile loads a route profile: industries, yards, jobs, road
// trains, limits and car-type rules. All route knowledge lives here.
package profile

import (
	"os"
	"strings"
)

// CarType is an industry car type as written in IndustryConfiguration.csv,
// for example "Box Car", "Gondola", "CoilCar", "Bulkhead_Flat".
type CarType string

// Unknown is the type of a car whose model matches no pattern.
const Unknown CarType = "Unknown"

// Yard is a named yard.
type Yard struct {
	Code string // "NOR"
	Name string // "Norris"
}

// IndustryCar is one car-type row of an industry (CSV continuation rows
// belong to the industry above).
type IndustryCar struct {
	CarType      CarType
	TimeHours    int
	Capacity     int
	Produces     string   // "Loads" or "Empties"
	OutgoingTags []string // trimmed, in file order; may include bad or duplicate entries
}

// Industry is one CSV industry.
type Industry struct {
	Name        string
	Tag         string // incoming tag, whitespace-normalised, e.g. "BRR"
	LocalSymbol string // CSV local symbol, e.g. "AB10"
	Cars        []IndustryCar
}

// Capacity is the industry's capacity for a car type; ok is false when the
// industry takes no cars of that type.
func (i *Industry) Capacity(t CarType) (n int, ok bool) {
	for _, c := range i.Cars {
		if c.CarType == t {
			return c.Capacity, true
		}
	}
	return 0, false
}

// TotalCapacity sums capacity over all car types.
func (i *Industry) TotalCapacity() int {
	n := 0
	for _, c := range i.Cars {
		n += c.Capacity
	}
	return n
}

// Limit holds train limits. A zero value means "no limit".
type Limit struct {
	MaxLengthFt int
	MaxTons     int
	MinHPT      float64
}

// YardLine is a satellite-yard customer of a local job.
type YardLine struct {
	Yard    string   // yard code
	Carries []string // local symbols, industry tags or class codes
}

// LocalJob is a local (switcher / turn) job.
type LocalJob struct {
	ID         string   // "a55"
	Symbol     string   // "A55"
	HomeYard   string   // yard code
	Industries []string // industry tags served (defaults to CSV rows whose LocalSymbol equals Symbol)
	YardLines  []YardLine
	Limit      Limit
}

// Stop is an en-route stop of a road train.
type Stop struct {
	Kind           string // "yard" or "industry"
	Target         string // yard code or industry tag
	SetOut         []string
	PickUp         []string
	PickUpCarTypes []CarType // empty means any type
}

// RoadTrain is an outbound or through train.
type RoadTrain struct {
	Symbol           string
	Origin           string   // yard code; "" means a through train
	Destination      string   // free text
	ClassCodes       []string // outgoing tags carried
	OffMapIndustries []string // industry tags whose cars are carried off-map (e.g. "A28")
	Stops            []Stop
	Limit            Limit
	MinLengthPct     float64 // percent of MaxLengthFt; 0 means any length
	MaxCarAgeDays    float64 // 0 means unset
}

// CarTypeRule maps a model file-name prefix to a car type. Rules are ordered;
// the first match wins.
type CarTypeRule struct {
	Prefix string
	Type   CarType
}

// CarDefaults are per-type default dimensions.
type CarDefaults struct {
	LengthFt float64
	TareTons float64
}

// Problem is one validation finding.
type Problem struct {
	Code    string // e.g. "orphan-tag", "malformed-tag", "unserved-industry", "unknown-ref", "duplicate-tag", "unmatched-model"
	Message string
	Subject string // tag, industry or model concerned
}

// Report lists validation problems; loading still succeeds with problems.
type Report struct {
	Problems []Problem
}

// Profile is a loaded route profile.
type Profile struct {
	ID                 string
	Name               string
	Dir                string
	DefaultSaveDir     string
	DefaultAutoSaveDir string
	Yards              []Yard
	Industries         []Industry
	LocalJobs          []LocalJob
	RoadTrains         []RoadTrain
	CarTypes           []CarTypeRule
	CarDefaults        map[CarType]CarDefaults
	UnknownDefaults    CarDefaults
	Aliases            map[string]string // outgoing tag -> class code
	YardInternalTags   []string          // "NOR", "NOR Hump", ...
	RoutePrefixes      []int
	Report             Report
}

// FileWriter is how this package writes files (first-run install). The store
// package supplies an implementation that enforces the protected-path guard.
type FileWriter interface {
	MkdirAll(path string, perm os.FileMode) error
	WriteFile(path string, data []byte, perm os.FileMode) error
}

// API implemented in this package (tasks 3.1 to 3.5):
//
//	func LoadIndustryCSV(r io.Reader) ([]Industry, error)
//	func LoadLimits(r io.Reader) (map[string]Limit, map[string][]string, error) // by base symbol; class codes by base symbol
//	func Load(dir string) (*Profile, error)                                      // reads profile.yaml, CSV, TSV from dir
//	func InstallBundled(w FileWriter, profilesDir string) error                  // installs ags-south-p1 when absent

// NormTag trims a tag and collapses internal whitespace.
func NormTag(s string) string { return strings.Join(strings.Fields(s), " ") }

// IndustryByTag finds an industry by incoming tag.
func (p *Profile) IndustryByTag(tag string) *Industry {
	tag = NormTag(tag)
	for i := range p.Industries {
		if p.Industries[i].Tag == tag {
			return &p.Industries[i]
		}
	}
	return nil
}

// LocalJobByID finds a local job.
func (p *Profile) LocalJobByID(id string) *LocalJob {
	for i := range p.LocalJobs {
		if p.LocalJobs[i].ID == id {
			return &p.LocalJobs[i]
		}
	}
	return nil
}

// LocalJobsServing returns every local job that serves the industry tag.
func (p *Profile) LocalJobsServing(tag string) []*LocalJob {
	var out []*LocalJob
	for i := range p.LocalJobs {
		for _, t := range p.LocalJobs[i].Industries {
			if t == tag {
				out = append(out, &p.LocalJobs[i])
				break
			}
		}
	}
	return out
}

// RoadTrainBySymbol finds a road train.
func (p *Profile) RoadTrainBySymbol(sym string) *RoadTrain {
	for i := range p.RoadTrains {
		if p.RoadTrains[i].Symbol == sym {
			return &p.RoadTrains[i]
		}
	}
	return nil
}

// RoadTrainForTag returns the road train that carries an outgoing tag, after
// applying Aliases. Trains with an origin are preferred over through trains.
func (p *Profile) RoadTrainForTag(tag string) *RoadTrain {
	tag = NormTag(tag)
	if a, ok := p.Aliases[tag]; ok {
		tag = a
	}
	var through *RoadTrain
	for i := range p.RoadTrains {
		for _, c := range p.RoadTrains[i].ClassCodes {
			if c == tag {
				if p.RoadTrains[i].Origin != "" {
					return &p.RoadTrains[i]
				}
				through = &p.RoadTrains[i]
			}
		}
	}
	return through
}

// RoadTrainForOffMapIndustry returns the road train that carries an industry
// off-map, or nil.
func (p *Profile) RoadTrainForOffMapIndustry(tag string) *RoadTrain {
	for i := range p.RoadTrains {
		for _, t := range p.RoadTrains[i].OffMapIndustries {
			if t == tag {
				return &p.RoadTrains[i]
			}
		}
	}
	return nil
}

// YardByCode finds a yard.
func (p *Profile) YardByCode(code string) *Yard {
	for i := range p.Yards {
		if p.Yards[i].Code == code {
			return &p.Yards[i]
		}
	}
	return nil
}

// IsYardInternalTag reports whether a tag is a yard-internal tag.
func (p *Profile) IsYardInternalTag(tag string) bool {
	tag = NormTag(tag)
	for _, t := range p.YardInternalTags {
		if NormTag(t) == tag {
			return true
		}
	}
	return false
}

// CarTypeOf maps a model file name to a car type using the ordered rules.
func (p *Profile) CarTypeOf(model string) CarType {
	for _, r := range p.CarTypes {
		if strings.HasPrefix(model, r.Prefix) {
			return r.Type
		}
	}
	return Unknown
}

// DefaultsFor returns default dimensions for a car type.
func (p *Profile) DefaultsFor(t CarType) CarDefaults {
	if d, ok := p.CarDefaults[t]; ok {
		return d
	}
	return p.UnknownDefaults
}
