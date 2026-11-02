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
	"run8-ops-center/internal/run8route"
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

// Run8Status values of Run8State.Status.
const (
	Run8Ready       = "ready"       // every Run8 route file was read
	Run8Partial     = "partial"     // some files were read and some were not
	Run8Unavailable = "unavailable" // no route data: see Run8State.Reason
)

// Run8State describes the Run8 route data the engine holds and how it stands
// against the last import. It is refreshed on every ingest by statting the
// files, without decoding them.
type Run8State struct {
	// Status is Run8Ready, Run8Partial or Run8Unavailable.
	Status string
	// Reason says why Status is Run8Unavailable ("profile names no Run8
	// folders", or that no file could be read); empty otherwise.
	Reason string
	// Unavailable lists the files that could not be used, with path and error.
	Unavailable []run8route.FileStatus
	// LastImport is the time of the last import, zero when none was made.
	LastImport time.Time
	// Changed is true when an import was made and the route files differ from
	// the ones it was built from.
	Changed bool
	// ImportAvailable is true when route data is readable and no import has
	// ever been made for this profile.
	ImportAvailable bool
	// HasRun8Config is true when the profile names Run8 region and route folders.
	HasRun8Config bool
}

// ImportPreview is what PreviewImport returns: the diff an import would
// apply, the route files that could not be used, and the token that
// ConfirmImport needs.
type ImportPreview struct {
	Diff     trackmap.ImportDiff
	Unusable []run8route.FileStatus
	// Token identifies the route files and the track map revision the diff was
	// built from. ConfirmImport refuses a token that no longer matches.
	Token string
	// Locations holds what the sections each location covers in the diff
	// (new, changed and removed entries) measure and carry, keyed by
	// trackmap.Location.Key. A location with no length and no cars has no entry.
	Locations map[string]ImportLocation
	// CarsKnown is true when a save was loaded, so an absent car count means
	// "no cars" rather than "not known".
	CarsKnown bool
}

// ImportLocation is the length and the cars of one location's listed sections.
type ImportLocation struct {
	// LengthFt is the summed length of the sections, in feet. It is 0 without
	// route data.
	LengthFt int
	// Cars counts the freight cars standing on the sections in the current
	// save. A car counts once, at its first truck whose section is listed.
	Cars int
	// CarNumbers are up to 5 of those cars as car status shows them.
	CarNumbers []string
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

	Run8 Run8State

	TrackMap   *trackmap.Map
	Candidates []trackmap.Candidate     // learn mode: unmapped cuts and trains with suggestions
	Locations  []trackmap.LocationGroup // labelled sections grouped by location
	// LocationFt is the measured length in feet of each yard location's
	// labelled sections, keyed by trackmap.Location.Key. It is nil without
	// route data, and holds no entry for a location that measures nothing.
	LocationFt map[string]int
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

	// PreviewImport re-reads the Run8 route files and returns what an import
	// would change, without changing anything.
	PreviewImport() (ImportPreview, error)
	// ConfirmImport applies the import a preview described. It returns
	// ErrStaleImport when the route files or the track map changed since.
	ConfirmImport(token string) error

	// Settings commands. All are persisted; thresholds, departure marks and
	// the profile apply immediately, port/bind changes need a restart.
	SetThreshold(symbol string, minLengthPct, maxCarAgeDays *float64) error // nil pointer clears that override
	// SetDeparture marks (on) or unmarks a yard track as a departure track.
	// Setting a track to its profile default removes the saved mark.
	SetDeparture(yard, track string, on bool) error
	SetInstallDir(dir string) error // validated by the store guard
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
//	func New(cfg Config) (*Engine, error)           // installs or upgrades the bundled profile, loads settings/state/trackmap and the Run8 route data
//	func (e *Engine) Run(ctx context.Context) error // the single-writer loop; returns when ctx ends
//	func (e *Engine) Ingest(ev watch.Event)         // called from the watcher goroutine; queued, never blocks the caller for long
//	// plus every Backend method.
