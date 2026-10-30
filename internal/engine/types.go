// Package engine owns all mutable application state. One goroutine runs the
// pipeline carstatus.Derive -> orders.Reconcile -> orders.Generate -> persist
// -> publish; everything else reads the latest immutable View and sends
// commands.
package engine

import (
	"time"

	"run8-ops-center/internal/carstatus"
	"run8-ops-center/internal/orders"
	"run8-ops-center/internal/profile"
	"run8-ops-center/internal/store"
	"run8-ops-center/internal/trackmap"
)

// Source describes where snapshots come from.
type Source struct {
	SaveName    string // --save
	SaveDir     string
	AutoSaveDir string
	ActiveFile  string // file of the snapshot in use, "" before the first ingest
	ActiveAuto  bool   // the active snapshot is an auto-save
	LastIngest  time.Time
	GameDate    time.Time
}

// Warning is an ingest or profile warning shown on the dashboard.
type Warning struct {
	Time    time.Time
	Kind    string // "ingest", "autosave", "parse", "profile", "settings"
	Message string
}

// Tiles are the dashboard summary counts.
type Tiles struct {
	OnRoute          int // cars in the snapshot
	AtIndustries     int
	ReadyToPull      int
	AwaitingDelivery int
	AwaitingTrain    int
	Unmapped         int
	Unknown          int // unknown destination or unknown car type
}

// ProfileInfo identifies an installed profile.
type ProfileInfo struct {
	ID   string
	Name string
}

// View is an immutable snapshot of everything the UI shows. The engine never
// mutates a published View; handlers must not either.
type View struct {
	Seq         uint64 // increments on every publish
	Source      Source
	HasSnapshot bool

	// Profile is the effective profile: the loaded profile with the user's
	// per-train threshold overrides applied.
	Profile  *profile.Profile
	Profiles []ProfileInfo // installed profiles

	Status *carstatus.Status // nil before the first ingest

	Available []orders.Order // regenerated every snapshot
	Active    []orders.Order // In Progress and Completion Detected, newest start first
	History   []orders.Order // Completed and Cancelled, newest finish first
	Building  []orders.RoadBuild

	Tiles    Tiles
	Warnings []Warning // newest first

	TrackMap   *trackmap.Map
	Candidates []trackmap.Candidate     // learn mode: unmapped cuts and trains with suggestions
	Locations  []trackmap.LocationGroup // labelled sections grouped by location
	Coverage   trackmap.Coverage

	Activity []store.ActivityEntry // newest first
	Settings store.Settings
}

// Backend is what the web layer needs from the engine. *Engine implements it.
// Every method is safe for concurrent use. Commands return after the engine
// has applied them and published a new View, so the caller can redirect and
// immediately read the result.
type Backend interface {
	View() *View
	// Subscribe returns a channel that receives a value after each publish
	// (coalesced; a slow reader never blocks the engine) and a cancel func.
	Subscribe() (events <-chan struct{}, cancel func())

	// StartOrder starts an available order (id "avail:...") whole when
	// lineKey is "", or one line of a local order (orders.Line.Key()).
	StartOrder(id, lineKey string) (orders.Order, error)
	// ApplyOrder confirms, continues, completes or cancels a started order.
	ApplyOrder(id string, a orders.Action) (orders.Order, error)

	// LabelCandidate labels every unmapped section of the learn-mode
	// candidate for trainID.
	LabelCandidate(trainID string, l trackmap.Label) error
	LabelSection(prefix, section int, l trackmap.Label) error
	RemoveLabel(prefix, section int) error
	// RenameLocation relabels every section of old as new; returns the count.
	RenameLocation(old, new trackmap.Location) (int, error)

	// Settings commands. All are persisted; thresholds and the profile apply
	// immediately, port/bind changes need a restart.
	SetThreshold(symbol string, minLengthPct, maxCarAgeDays *float64) error // nil pointer clears that override
	SetInstallDir(dir string) error                                         // validated by the store guard
	SetServer(port int, allInterfaces bool) error
	SwitchProfile(id string) error
}

// Config configures New.
type Config struct {
	DataDir     string
	Guard       *store.Guard
	SaveName    string
	SaveDir     string
	AutoSaveDir string
	ProfileID   string           // initial profile when settings name none; default "ags-south-p1"
	Now         func() time.Time // default time.Now
}

// API implemented in this package (tasks 8.3, 8.4, 9.2):
//
//	func New(cfg Config) (*Engine, error)           // installs the bundled profile on first run, loads settings/state/trackmap
//	func (e *Engine) Run(ctx context.Context) error // the single-writer loop; returns when ctx ends
//	func (e *Engine) Ingest(ev watch.Event)         // called from the watcher goroutine; queued, never blocks the caller for long
//	// plus every Backend method.
