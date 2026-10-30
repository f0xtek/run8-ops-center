package watch

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"run8-ops-center/internal/run8save"
)

const (
	fixtureDir = "../../testdata"
	autoFixed  = "Auto Save World 2026.10.28 17.28.02"
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(fixtureDir, name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return b
}

// writeInfo writes a sidecar for xmlPath declaring saveType and ts.
func writeInfo(t *testing.T, xmlPath string, saveType run8save.SaveType, ts time.Time) {
	t.Helper()
	body := fmt.Sprintf(`<?xml version="1.0"?>
<ScnInfo xmlns:xsd="http://www.w3.org/2001/XMLSchema" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <SaveType>%s</SaveType>
  <Filename>%s</Filename>
  <DisplayFilename>x</DisplayFilename>
  <FileTimestamp>%s</FileTimestamp>
</ScnInfo>`, saveType, xmlPath, ts.UTC().Format(time.RFC3339Nano))
	if err := os.WriteFile(run8save.InfoPath(xmlPath), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// putSave writes data to path, then its sidecar (World, ts) the way Run8 does:
// the save first, the sidecar after.
func putSave(t *testing.T, path string, data []byte, ts time.Time) {
	t.Helper()
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	writeInfo(t, path, run8save.SaveWorld, ts)
}

// env is a pair of temp folders standing in for the Run8 Trains and
// AutoSaves folders. base is the write time of the named save, an hour before
// the real clock so later saves can be dated with plain offsets.
type env struct {
	save, auto string
	base       time.Time
	world      []byte // WorldSave.xml
	autoData   []byte // the reference auto-save
}

func newEnv(t *testing.T) *env {
	t.Helper()
	root := t.TempDir()
	e := &env{
		save:     filepath.Join(root, "Trains"),
		auto:     filepath.Join(root, "AutoSaves"),
		base:     time.Now().UTC().Add(-time.Hour).Truncate(time.Second),
		world:    fixture(t, "WorldSave.xml"),
		autoData: fixture(t, autoFixed+".xml"),
	}
	for _, d := range []string{e.save, e.auto} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	putSave(t, filepath.Join(e.save, "WorldSave.xml"), e.world, e.base)
	return e
}

func (e *env) config() Config {
	return Config{
		SaveDir:     e.save,
		AutoSaveDir: e.auto,
		SaveName:    "WorldSave",
		Settle:      40 * time.Millisecond,
		Poll:        25 * time.Millisecond,
	}
}

// autoPath is a matching auto-save path for the given clock text.
func (e *env) autoPath(stamp string) string {
	return filepath.Join(e.auto, "Auto Save World "+stamp+".xml")
}

// mutateCars rewrites the unit number of every freight car for which change
// returns true, so that car no longer matches the same car in WorldSave.
func mutateCars(data []byte, change func(carIndex int) bool) []byte {
	const open = "<RailVehicleStateClass>"
	parts := strings.Split(string(data), open)
	n := 0
	for i := 1; i < len(parts); i++ {
		if !strings.Contains(parts[i], "<unitType>US_Freightcar</unitType>") {
			continue
		}
		if change(n) {
			parts[i] = unitNumberRE.ReplaceAllString(parts[i], "<unitNumber>${1}-X</unitNumber>")
		}
		n++
	}
	return []byte(strings.Join(parts, open))
}

var unitNumberRE = regexp.MustCompile(`<unitNumber>([^<]*)</unitNumber>`)

// harness runs a Watcher and collects its events.
type harness struct {
	t      *testing.T
	events chan Event
	cancel func()
	done   chan error
}

func start(t *testing.T, cfg Config) *harness {
	t.Helper()
	w, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return run(t, w)
}

func run(t *testing.T, w *Watcher) *harness {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	h := &harness{t: t, events: make(chan Event, 256), cancel: cancel, done: make(chan error, 1)}
	go func() {
		h.done <- w.Run(ctx, func(ev Event) { h.events <- ev })
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-h.done:
			if err != nil {
				t.Errorf("Run returned %v after cancel", err)
			}
		case <-time.After(10 * time.Second):
			t.Error("Run did not return after cancel")
		}
	})
	return h
}

func (h *harness) next() Event {
	h.t.Helper()
	select {
	case ev := <-h.events:
		return ev
	case <-time.After(15 * time.Second):
		h.t.Fatal("timed out waiting for an event")
		return Event{}
	}
}

// quiet fails if any event arrives within d.
func (h *harness) quiet(d time.Duration) {
	h.t.Helper()
	select {
	case ev := <-h.events:
		h.t.Fatalf("unexpected event: %+v", describe(ev))
	case <-time.After(d):
	}
}

func describe(ev Event) string {
	return fmt.Sprintf("path=%q auto=%v overlap=%.3f warning=%q err=%v snapshot=%v",
		ev.Path, ev.Auto, ev.Overlap, ev.Warning, ev.Err, ev.Snapshot != nil)
}
