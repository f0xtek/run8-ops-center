// Package watch follows the named world save and the auto-save folder,
// delivering parsed snapshots. It never writes any file.
package watch

import (
	"time"

	"run8-ops-center/internal/run8save"
)

// Event is one ingest result delivered to the engine.
type Event struct {
	Snapshot *run8save.Snapshot // nil when Err or Warning-only
	Path     string             // file the event is about
	Auto     bool               // from the auto-save folder
	Overlap  float64            // car-key overlap with the previous snapshot, 0..1, auto-saves only
	Warning  string             // non-fatal: file skipped, folder missing, world check failed (includes the overlap percentage)
	Err      error              // parse failure; the previous snapshot stays in use
}

// Config configures a Watcher.
type Config struct {
	SaveDir     string
	AutoSaveDir string
	SaveName    string        // from --save
	Settle      time.Duration // default 1s
	Poll        time.Duration // default 5s
	MinOverlap  float64       // default 0.90
}

// API implemented in this package (tasks 9.1 and 9.3):
//
//	func Resolve(saveDir, name string) (path string, err error)   // case-insensitive, with or without .xml, must be a world save; error lists the world saves found
//	func ListWorldSaves(saveDir string) ([]string, error)
//	func New(cfg Config) (*Watcher, error)
//	func (w *Watcher) Run(ctx context.Context, emit func(Event)) error // startup ingest first, then follow
