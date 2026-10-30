package profile

// This file defines the profile.yaml schema. Every field a user can write is
// a field of one of these structs. docs/route-profiles.md documents each one,
// and a test keeps the two in step.

// Names of the files that make up a profile directory.
const (
	// ProfileFile is the profile definition inside a profile directory.
	ProfileFile = "profile.yaml"
	// IndustryFile is the Run8 industry configuration CSV.
	IndustryFile = "IndustryConfiguration.csv"
	// LimitsFile is the tab-separated train limits table.
	LimitsFile = "train_limits.tsv"
)

// fileSchema is the root of profile.yaml.
type fileSchema struct {
	SchemaVersion      int                          `yaml:"schemaVersion"`
	ID                 string                       `yaml:"id"`
	Name               string                       `yaml:"name"`
	DefaultSaveDir     string                       `yaml:"defaultSaveDir"`
	DefaultAutoSaveDir string                       `yaml:"defaultAutoSaveDir"`
	Yards              []yardSchema                 `yaml:"yards"`
	LocalJobs          []localJobSchema             `yaml:"localJobs"`
	RoadTrains         []roadTrainSchema            `yaml:"roadTrains"`
	CarTypes           []carTypeSchema              `yaml:"carTypes"`
	CarDefaults        map[string]carDefaultsSchema `yaml:"carDefaults"`
	UnknownDefaults    carDefaultsSchema            `yaml:"unknownDefaults"`
	Aliases            map[string]string            `yaml:"aliases"`
	YardInternalTags   []string                     `yaml:"yardInternalTags"`
	RoutePrefixes      []int                        `yaml:"routePrefixes"`
	Run8               *run8Schema                  `yaml:"run8"`
}

type run8Schema struct {
	Region    string            `yaml:"region"`
	Routes    []run8RouteSchema `yaml:"routes"`
	Humps     []run8HumpSchema  `yaml:"humps"`
	TrackEnds []run8EndsSchema  `yaml:"trackEnds"`
}

type run8RouteSchema struct {
	Folder string `yaml:"folder"`
	Prefix int    `yaml:"prefix"`
}

type run8HumpSchema struct {
	Controller  string `yaml:"controller"`
	Yard        string `yaml:"yard"`
	TrackPrefix string `yaml:"trackPrefix"`
}

type run8EndsSchema struct {
	Pattern     string `yaml:"pattern"`
	Yard        string `yaml:"yard"`
	TrackPrefix string `yaml:"trackPrefix"`
}

type yardSchema struct {
	Code             string   `yaml:"code"`
	Name             string   `yaml:"name"`
	AnchorMileposts  []string `yaml:"anchorMileposts"`
	AnchorIndustries []string `yaml:"anchorIndustries"`
}

// limitSchema overrides limits from the limits file. A nil field keeps the
// file value. An explicit 0 means no limit.
type limitSchema struct {
	MaxLengthFt *int     `yaml:"maxLengthFt"`
	MaxTons     *int     `yaml:"maxTons"`
	MinHPT      *float64 `yaml:"minHpt"`
}

type localJobSchema struct {
	ID         string           `yaml:"id"`
	Symbol     string           `yaml:"symbol"`
	HomeYard   string           `yaml:"homeYard"`
	Industries []string         `yaml:"industries"`
	YardLines  []yardLineSchema `yaml:"yardLines"`
	Limit      limitSchema      `yaml:"limit"`
}

type yardLineSchema struct {
	Yard    string   `yaml:"yard"`
	Carries []string `yaml:"carries"`
}

type roadTrainSchema struct {
	Symbol           string       `yaml:"symbol"`
	Origin           string       `yaml:"origin"`
	Destination      string       `yaml:"destination"`
	ClassCodes       []string     `yaml:"classCodes"`
	OffMapIndustries []string     `yaml:"offMapIndustries"`
	Stops            []stopSchema `yaml:"stops"`
	Limit            limitSchema  `yaml:"limit"`
	MinLengthPct     *float64     `yaml:"minLengthPct"`
	MaxCarAgeDays    float64      `yaml:"maxCarAgeDays"`
}

type stopSchema struct {
	Kind           string   `yaml:"kind"`
	Target         string   `yaml:"target"`
	SetOut         []string `yaml:"setOut"`
	PickUp         []string `yaml:"pickUp"`
	PickUpCarTypes []string `yaml:"pickUpCarTypes"`
}

type carTypeSchema struct {
	Prefix string `yaml:"prefix"`
	Type   string `yaml:"type"`
}

type carDefaultsSchema struct {
	LengthFt float64 `yaml:"lengthFt"`
	TareTons float64 `yaml:"tareTons"`
}
