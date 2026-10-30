// Package orders generates available work (local job orders with industry and
// yard lines, road train orders with stops) and runs the order lifecycle.
//
// generate*.go owns generation (tasks 6.x); lifecycle*.go owns the lifecycle
// (tasks 7.x). Both share the types in this file.
package orders

import (
	"time"

	"run8-ops-center/internal/carstatus"
	"run8-ops-center/internal/profile"
)

// Kind is the order type.
type Kind string

const (
	KindLocal Kind = "local"
	KindRoad  Kind = "road"
)

// State is the lifecycle state.
type State string

const (
	StateAvailable          State = "available"
	StateInProgress         State = "in-progress"
	StateCompletionDetected State = "completion-detected"
	StateCompleted          State = "completed"
	StateCancelled          State = "cancelled"
)

// LineKind is the type of order line.
type LineKind string

const (
	LineIndustry LineKind = "industry" // local: an industry; Target is the industry tag
	LineYard     LineKind = "yard"     // local: a satellite yard line; Target is the yard code
	LineStop     LineKind = "stop"     // road: an en-route stop; Target is the yard code or industry tag
)

// CarRef is a car listed on an order, captured when the order is generated.
type CarRef struct {
	Key       string
	Number    string
	Type      profile.CarType
	Loaded    bool
	Tag       string // normalised destination tag at generation time
	Location  string // carstatus.Location.String() at generation time
	LengthFt  float64
	GrossTons float64
	Done      bool // set by Reconcile: this car's part of the order is finished
	Missing   bool // set by Reconcile: car absent from the snapshot but still expected on the map
}

// Line is one switchlist line.
type Line struct {
	Kind    LineKind
	Target  string
	Name    string // display name, e.g. "Holcim Trading Inc" or "Tuscaloosa"
	SetOuts []CarRef
	PickUps []CarRef
	Done    bool // set by Reconcile
}

// Key identifies a line within its order: "industry:HTI", "yard:TUS", "stop:MCC".
func (l Line) Key() string { return string(l.Kind) + ":" + l.Target }

// Flag is a warning shown on an order.
type Flag struct {
	Code    string // "over-length", "over-tons", "missing-car", "shared-cars", "unmapped"
	Message string
}

// Totals summarises the departing train.
type Totals struct {
	LengthFt    float64
	GrossTons   float64
	RequiredHP  float64 // GrossTons * MinHPT
	MaxLengthFt int     // 0 = no limit
	MaxTons     int
}

// Order is a unit of work.
type Order struct {
	ID      string // available: "avail:<jobID>"; started: "ord-<n>"
	Kind    Kind
	JobID   string // local job ID or road train symbol
	Symbol  string
	Profile string // profile ID the order was generated for
	State   State

	Consist []CarRef // road: cars departing the origin yard, in pick order
	Lines   []Line   // local: industry and yard lines; road: stops in order
	Totals  Totals
	Flags   []Flag
	Shared  []string // car keys also listed on another available order

	SuppressFP string // set by "continue": completion is not re-detected while the fingerprint is unchanged

	CreatedAt    time.Time // real time
	StartedAt    time.Time
	DetectedAt   time.Time
	FinishedAt   time.Time
	GameStarted  time.Time // in-game date
	GameFinished time.Time
}

// CarKeys lists every car on the order (Consist, set-outs and pick-ups), once.
func (o *Order) CarKeys() []string {
	seen := map[string]bool{}
	var out []string
	add := func(rs []CarRef) {
		for _, r := range rs {
			if !seen[r.Key] {
				seen[r.Key] = true
				out = append(out, r.Key)
			}
		}
	}
	add(o.Consist)
	for _, l := range o.Lines {
		add(l.SetOuts)
		add(l.PickUps)
	}
	return out
}

// Reserved reports whether the order's state reserves its cars.
func (o *Order) Reserved() bool {
	return o.State == StateInProgress || o.State == StateCompletionDetected
}

// RoadBuild is the build percentage of a road train that has not met its
// threshold (or has), shown on the dashboard.
type RoadBuild struct {
	Symbol       string
	Destination  string
	Origin       string
	WaitingFt    float64
	MaxLengthFt  int
	Pct          float64 // WaitingFt / MaxLengthFt * 100
	ThresholdPct float64
	Ready        bool
}

// GenInput is the input to Generate.
type GenInput struct {
	Profile *profile.Profile
	Status  *carstatus.Status
	Active  []Order // started orders; cars of Reserved ones are excluded from generation
}

// GenResult is the output of Generate.
type GenResult struct {
	Orders   []Order // all State == StateAvailable
	Building []RoadBuild
}

// Action is a user action on an order.
type Action string

const (
	ActStart    Action = "start"
	ActConfirm  Action = "confirm"
	ActContinue Action = "continue"
	ActComplete Action = "complete"
	ActCancel   Action = "cancel"
)

// Clock carries the real and in-game time of an action or snapshot.
type Clock struct {
	Real time.Time
	Game time.Time
}

// API implemented in this package:
//
// generate*.go (tasks 6.1 to 6.5):
//
//	func Generate(in GenInput) GenResult
//
// lifecycle*.go (tasks 7.1 to 7.3):
//
//	// Start turns an available order into a started order. lineKey == "" starts
//	// the whole order; otherwise only that line (Line.Key()), local orders only.
//	func Start(avail Order, lineKey string, newID string, c Clock) (Order, error)
//	// Apply performs confirm, continue, complete or cancel on a started order.
//	func Apply(o Order, a Action, c Clock) (Order, error)
//	// Reconcile refreshes per-car and per-line progress and detects completion
//	// for In Progress orders against a new status; other states pass through.
//	func Reconcile(o Order, st *carstatus.Status, p *profile.Profile, c Clock) Order
