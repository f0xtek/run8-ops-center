package web

import (
	"fmt"
	"time"
)

// Chrome is the data shown in the page header and footer on every page.
type Chrome struct {
	SaveName     string    // world save being followed (--save)
	ActiveFile   string    // file the current snapshot came from, for display
	GameDate     time.Time // in-game date of the current snapshot; zero before the first ingest
	LastIngest   time.Time // wall-clock time of the last accepted ingest; zero before the first
	SaveDir      string    // world save folder, shown read-only
	AutoSaveDir  string    // auto-save folder, shown read-only
	WarningCount int       // ingest warnings and profile problems currently outstanding
}

// Folders returns a one-line description of the source folders, used as a tooltip.
func (c Chrome) Folders() string {
	return fmt.Sprintf("World saves: %s | Auto-saves: %s", c.SaveDir, c.AutoSaveDir)
}

// Backend is the minimum the web infrastructure needs from the rest of the app.
// View packages define their own, larger interfaces and are handed a value that also
// satisfies this one.
type Backend interface {
	// Chrome returns the header data for the current state.
	Chrome() Chrome
	// Subscribe returns a channel that receives a value whenever anything a page
	// shows may have changed (new snapshot, order state change, settings change).
	// The caller must call cancel when done. Slow receivers may miss values; the
	// channel is expected to coalesce, so each value means "refresh now".
	Subscribe() (events <-chan struct{}, cancel func())
}
