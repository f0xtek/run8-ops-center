package engine

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"run8-ops-center/internal/run8save"
	"run8-ops-center/internal/store"
	"run8-ops-center/internal/trackmap"
	"run8-ops-center/internal/watch"
)

const (
	worldPath = "../../testdata/WorldSave.xml"
	autoPath  = "../../testdata/Auto Save World 2026.10.28 17.28.02.xml"
)

// fakeClock is the engine's Now. It moves only when the test advances it.
type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func newClock() *fakeClock {
	return &fakeClock{t: time.Date(2026, 10, 30, 9, 0, 0, 0, time.UTC)}
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

// harness is an engine running over a temp data directory.
type harness struct {
	t     *testing.T
	dir   string
	clock *fakeClock
	guard *store.Guard
	eng   *Engine
	stop  context.CancelFunc
	done  chan struct{}
}

// start creates a data directory and an engine over it.
func start(t *testing.T) *harness {
	t.Helper()
	h := &harness{t: t, dir: t.TempDir(), clock: newClock(), guard: store.NewGuard()}
	h.open()
	return h
}

// open starts an engine over h.dir. It stops a running one first, so calling
// it again models a restart.
func (h *harness) open() {
	h.t.Helper()
	h.close()
	eng, err := New(Config{
		DataDir:     h.dir,
		Guard:       h.guard,
		SaveName:    "WorldSave",
		SaveDir:     "saves",
		AutoSaveDir: "autosaves",
		Now:         h.clock.Now,
	})
	if err != nil {
		h.t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		if err := eng.Run(ctx); err != nil {
			h.t.Errorf("Run: %v", err)
		}
	}()
	h.eng, h.stop, h.done = eng, cancel, done
	h.t.Cleanup(h.close)
}

func (h *harness) close() {
	if h.stop == nil {
		return
	}
	h.stop()
	<-h.done
	h.stop = nil
}

// ingest sends an event and waits until the engine has published for it.
func (h *harness) ingest(ev watch.Event) *View {
	h.t.Helper()
	before := h.eng.View().Seq
	events, cancel := h.eng.Subscribe()
	defer cancel()
	h.eng.Ingest(ev)
	deadline := time.After(10 * time.Second)
	for h.eng.View().Seq == before {
		select {
		case <-events:
		case <-deadline:
			h.t.Fatal("engine did not publish after the event")
		}
	}
	return h.eng.View()
}

// snapshotEvent wraps a snapshot as the watcher would deliver it.
func snapshotEvent(snap *run8save.Snapshot, path string, auto bool) watch.Event {
	return watch.Event{Snapshot: snap, Path: path, Auto: auto}
}

func parse(t *testing.T, path string) *run8save.Snapshot {
	t.Helper()
	snap, err := run8save.ParseFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return snap
}

// labelAllYard labels every unmapped section of every candidate as the yard
// track NOR / All, so every car sits in the Norris yard.
func (h *harness) labelAllYard() *View {
	h.t.Helper()
	for _, c := range h.eng.View().Candidates {
		if err := h.eng.LabelCandidate(c.TrainID, trackmap.Label{Kind: trackmap.KindYard, Yard: "NOR", Track: "All"}); err != nil {
			h.t.Fatal(err)
		}
	}
	return h.eng.View()
}

func (h *harness) profileDir() string {
	return filepath.Join(h.dir, profilesDirName, DefaultProfileID)
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func watchParse(path string) (*run8save.Snapshot, error) { return run8save.ParseFile(path) }

// ingestWarnings drops the profile-report warning, which the bundled profile
// always has, and the route file warnings, which the tests have because the
// default Run8 install folder does not exist on the test machine.
func ingestWarnings(v *View) []Warning {
	var out []Warning
	for _, w := range v.Warnings {
		if w.Kind != "profile" && w.Kind != warnRun8 {
			out = append(out, w)
		}
	}
	return out
}
