package engine

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"run8-ops-center/internal/carstatus"
	"run8-ops-center/internal/orders"
	"run8-ops-center/internal/profile"
	"run8-ops-center/internal/run8save"
	"run8-ops-center/internal/store"
	"run8-ops-center/internal/trackmap"
	"run8-ops-center/internal/watch"
)

// DefaultProfileID is the profile used when neither settings nor Config name one.
const DefaultProfileID = profile.BundledID

const (
	profilesDirName = "profiles"
	trackMapFile    = "trackmap.json"
	ingestQueue     = 32
	maxWarnings     = 20
)

// ErrStopped is returned by commands once Run has returned.
var ErrStopped = errors.New("engine is not running")

var _ Backend = (*Engine)(nil)

// Engine owns all mutable state. Run is the only goroutine that touches the
// fields below "loop-owned"; everything else goes through channels or the
// atomic View pointer.
type Engine struct {
	cfg         Config
	guard       *store.Guard
	now         func() time.Time
	dataDir     string
	profilesDir string

	view     atomic.Pointer[View]
	ingestCh chan watch.Event
	cmdCh    chan func()
	done     chan struct{} // closed when Run returns
	running  atomic.Bool

	subMu   sync.Mutex
	subs    map[int]chan struct{}
	nextSub int

	// loop-owned. Maps, slices and pointers published in a View are replaced,
	// never mutated in place.
	settings   store.Settings
	state      store.State
	base       *profile.Profile // as loaded from disk
	eff        *profile.Profile // base with threshold overrides applied
	tm         *trackmap.Map
	installed  []ProfileInfo
	warnings   []Warning
	snap       *run8save.Snapshot
	source     Source
	status     *carstatus.Status
	avail      []orders.Order
	building   []orders.RoadBuild
	candidates []trackmap.Candidate
	locations  []trackmap.LocationGroup
	coverage   trackmap.Coverage
	viewSeq    uint64
	pending    []store.ActivityEntry // activity of the current step, oldest first
	stateDirty bool
}

// New loads settings, state, the active profile and its track map, installing
// the bundled profile on first run, and publishes the first View. Persisted
// in-progress orders are visible in it before any snapshot arrives. Nothing
// runs until Run is called.
func New(cfg Config) (*Engine, error) {
	if cfg.Guard == nil {
		return nil, errors.New("engine: a store guard is required")
	}
	if cfg.DataDir == "" {
		return nil, errors.New("engine: data directory is empty")
	}
	if err := cfg.Guard.Check(cfg.DataDir); err != nil {
		return nil, fmt.Errorf("engine: data directory: %w", err)
	}
	cfg.Guard.SetDataDir(cfg.DataDir)
	e := &Engine{
		cfg:         cfg,
		guard:       cfg.Guard,
		now:         cfg.Now,
		dataDir:     cfg.DataDir,
		profilesDir: filepath.Join(cfg.DataDir, profilesDirName),
		ingestCh:    make(chan watch.Event, ingestQueue),
		cmdCh:       make(chan func()),
		done:        make(chan struct{}),
		subs:        map[int]chan struct{}{},
	}
	if e.now == nil {
		e.now = time.Now
	}

	var err error
	if e.settings, err = store.LoadSettings(e.dataDir); err != nil {
		return nil, fmt.Errorf("engine: %w", err)
	}
	if e.state, err = store.LoadState(e.dataDir); err != nil {
		return nil, fmt.Errorf("engine: %w", err)
	}
	if e.settings.RunInstallDir != "" {
		if err := e.guard.SetInstallDir(e.settings.RunInstallDir); err != nil {
			return nil, fmt.Errorf("engine: install folder from settings: %w", err)
		}
	}
	if err := e.guard.MkdirAll(e.dataDir, 0o755); err != nil {
		return nil, fmt.Errorf("engine: %w", err)
	}
	if err := profile.InstallBundled(e.guard, e.profilesDir); err != nil {
		return nil, fmt.Errorf("engine: %w", err)
	}

	found, problems := scanProfiles(e.profilesDir)
	p, fellBack := pickProfile(found, e.settings.ProfileID, cfg.ProfileID)
	if p == nil {
		return nil, fmt.Errorf("engine: no usable profile in %s (wanted %q)", e.profilesDir, firstNonEmpty(e.settings.ProfileID, cfg.ProfileID, DefaultProfileID))
	}
	if err := e.adoptProfile(p); err != nil {
		return nil, fmt.Errorf("engine: %w", err)
	}
	e.installed = infoOf(found)
	for _, m := range problems {
		e.addWarning("profile", m)
	}
	if fellBack {
		e.addWarning("profile", fmt.Sprintf("profile %q from settings is not installed; using %q", e.settings.ProfileID, p.ID))
	}
	e.profileWarnings()

	e.source = Source{SaveName: cfg.SaveName, SaveDir: cfg.SaveDir, AutoSaveDir: cfg.AutoSaveDir}
	e.recompute(false)
	e.publish()
	return e, nil
}

// Run is the single-writer loop. It returns nil when ctx ends.
func (e *Engine) Run(ctx context.Context) error {
	if !e.running.CompareAndSwap(false, true) {
		return errors.New("engine: Run called twice")
	}
	defer close(e.done)
	for {
		select {
		case <-ctx.Done():
			return nil
		case ev := <-e.ingestCh:
			e.safely(func() { e.ingest(ev) }, func(err error) {
				e.addWarning("ingest", fmt.Sprintf("internal error while ingesting %s: %v", filepath.Base(ev.Path), err))
				e.publish()
			})
		case c := <-e.cmdCh:
			c()
		}
	}
}

// Ingest queues a watcher event for the engine loop. It blocks only when the
// queue is full, and returns at once when the engine has stopped.
func (e *Engine) Ingest(ev watch.Event) {
	select {
	case e.ingestCh <- ev:
	case <-e.done:
	}
}

// View returns the latest published View.
func (e *Engine) View() *View { return e.view.Load() }

// Subscribe returns a coalescing change channel. The channel is never closed.
func (e *Engine) Subscribe() (<-chan struct{}, func()) {
	ch := make(chan struct{}, 1)
	e.subMu.Lock()
	id := e.nextSub
	e.nextSub++
	e.subs[id] = ch
	e.subMu.Unlock()
	return ch, func() {
		e.subMu.Lock()
		delete(e.subs, id)
		e.subMu.Unlock()
	}
}

func (e *Engine) notify() {
	e.subMu.Lock()
	defer e.subMu.Unlock()
	for _, ch := range e.subs {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

// do runs fn on the engine goroutine and returns its error after fn (and the
// publish it triggers) has finished. A panic inside fn becomes an error.
func (e *Engine) do(fn func() error) error {
	reply := make(chan error, 1)
	c := func() {
		var err error
		e.safely(func() { err = fn() }, func(p error) { err = p })
		reply <- err
	}
	select {
	case e.cmdCh <- c:
	case <-e.done:
		return ErrStopped
	}
	return <-reply
}

// safely runs fn and hands a recovered panic to onPanic.
func (e *Engine) safely(fn func(), onPanic func(error)) {
	defer func() {
		if r := recover(); r != nil {
			onPanic(fmt.Errorf("internal error: %v", r))
		}
	}()
	fn()
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if s != "" {
			return s
		}
	}
	return ""
}
