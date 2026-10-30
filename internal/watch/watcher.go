package watch

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"

	"run8-ops-center/internal/run8save"
)

// Defaults applied by New to zero Config fields.
const (
	DefaultSettle     = time.Second
	DefaultPoll       = 5 * time.Second
	DefaultMinOverlap = 0.90
)

// autoSaveName matches the file names Run8 gives world auto-saves.
var autoSaveName = regexp.MustCompile(`^Auto Save World \d{4}\.\d{2}\.\d{2} \d{2}\.\d{2}\.\d{2}\.xml$`)

// Watcher follows one named world save and the auto-save folder.
type Watcher struct {
	cfg      Config
	savePath string

	emit      func(Event)
	last      time.Time // WriteTime of the last emitted snapshot
	baseline  *run8save.Snapshot
	baseKeys  map[string]struct{}
	seen      map[string]stamp // file state already handled (accepted, skipped or reported)
	autoWarn  bool             // the missing auto-save folder warning is out
	autoWatch bool             // the auto-save folder is registered with fsnotify
}

// New validates cfg, applies defaults and resolves the named save. It fails
// when the save folder is missing or the name does not select a world save, so
// nothing is ever emitted for a bad configuration.
func New(cfg Config) (*Watcher, error) {
	if cfg.Settle <= 0 {
		cfg.Settle = DefaultSettle
	}
	if cfg.Poll <= 0 {
		cfg.Poll = DefaultPoll
	}
	if cfg.MinOverlap <= 0 {
		cfg.MinOverlap = DefaultMinOverlap
	}
	path, err := Resolve(cfg.SaveDir, cfg.SaveName)
	if err != nil {
		return nil, err
	}
	return &Watcher{cfg: cfg, savePath: path, seen: map[string]stamp{}}, nil
}

// candidate is a file that may become the next snapshot.
type candidate struct {
	path  string
	auto  bool
	stamp stamp
	wt    time.Time
}

// Run ingests the newest qualifying save, calls emit for it, then follows the
// named save and the auto-save folder until ctx is done. A missing save folder
// is an error before anything is emitted. emit is only called from the calling
// goroutine. Run returns nil when ctx ends.
func (w *Watcher) Run(ctx context.Context, emit func(Event)) error {
	if fi, err := os.Stat(w.cfg.SaveDir); err != nil {
		return fmt.Errorf("save folder %s: %w", w.cfg.SaveDir, err)
	} else if !fi.IsDir() {
		return fmt.Errorf("save folder %s is not a directory", w.cfg.SaveDir)
	}
	w.emit = emit
	w.last, w.baseline, w.baseKeys = time.Time{}, nil, nil
	w.seen = map[string]stamp{}
	w.autoWarn, w.autoWatch = false, false

	fw, err := fsnotify.NewBufferedWatcher(256)
	if err != nil {
		emit(Event{Warning: fmt.Sprintf("file notifications unavailable, polling every %s: %v", w.cfg.Poll, err)})
		fw = nil
	} else {
		defer fw.Close()
		if err := fw.Add(w.cfg.SaveDir); err != nil {
			emit(Event{Warning: fmt.Sprintf("cannot watch %s, polling every %s: %v", w.cfg.SaveDir, w.cfg.Poll, err)})
		}
	}

	autoOK := w.checkAutoDir(fw)
	if err := w.startup(ctx, autoOK); err != nil {
		return w.finish(ctx, err)
	}

	var events <-chan fsnotify.Event
	var errs <-chan error
	if fw != nil {
		events, errs = fw.Events, fw.Errors
	}
	ticker := time.NewTicker(w.cfg.Poll)
	defer ticker.Stop()
	for {
		rescan := false
		select {
		case <-ctx.Done():
			return nil
		case ev, ok := <-events:
			if !ok {
				events, errs = nil, nil
				break
			}
			rescan = w.relevant(ev.Name)
		case err, ok := <-errs:
			if !ok {
				events, errs = nil, nil
				break
			}
			emit(Event{Warning: fmt.Sprintf("file notification error, relying on polling: %v", err)})
		case <-ticker.C:
			rescan = true
		}
		if !rescan {
			continue
		}
		w.checkAutoDir(fw)
		if err := w.scan(ctx); err != nil {
			return w.finish(ctx, err)
		}
	}
}

// finish maps a context error to a clean return.
func (w *Watcher) finish(ctx context.Context, err error) error {
	if ctx.Err() != nil && errors.Is(err, ctx.Err()) {
		return nil
	}
	return err
}

// checkAutoDir reports whether the auto-save folder is readable. It warns once
// per outage and registers the folder with fsnotify when it appears.
func (w *Watcher) checkAutoDir(fw *fsnotify.Watcher) bool {
	dir := w.cfg.AutoSaveDir
	if dir == "" {
		return false
	}
	if _, err := os.ReadDir(dir); err != nil {
		w.autoWatch = false
		if !w.autoWarn {
			w.autoWarn = true
			w.emit(Event{Path: dir, Warning: fmt.Sprintf("auto-save folder %s is missing or unreadable (%v); following %s only", dir, err, filepath.Base(w.savePath))})
		}
		return false
	}
	w.autoWarn = false
	if fw != nil && !w.autoWatch {
		if err := fw.Add(dir); err == nil {
			w.autoWatch = true
		}
	}
	return true
}

// relevant reports whether a file event concerns the named save, its sidecar
// or an auto-save with its sidecar.
func (w *Watcher) relevant(name string) bool {
	dir, base := filepath.Dir(name), filepath.Base(name)
	if sameDir(dir, w.cfg.SaveDir) {
		saveBase := filepath.Base(w.savePath)
		if strings.EqualFold(base, saveBase) || strings.EqualFold(base, filepath.Base(run8save.InfoPath(saveBase))) {
			return true
		}
	}
	if w.cfg.AutoSaveDir != "" && sameDir(dir, w.cfg.AutoSaveDir) {
		if strings.EqualFold(filepath.Ext(base), ".info") {
			base = base[:len(base)-len(".info")] + ".xml"
		}
		return autoSaveName.MatchString(base)
	}
	return false
}

func sameDir(a, b string) bool { return filepath.Clean(a) == filepath.Clean(b) }

// candidates lists the named save and, when the auto-save folder is readable,
// its matching files, skipping any whose on-disk state was already handled.
func (w *Watcher) candidates(includeAuto bool) []candidate {
	var out []candidate
	add := func(path string, auto bool) {
		st, err := stampOf(path)
		if err != nil {
			return
		}
		if prev, ok := w.seen[path]; ok && prev == st {
			return
		}
		wt, err := writeTime(path)
		if err != nil {
			return
		}
		out = append(out, candidate{path: path, auto: auto, stamp: st, wt: wt})
	}
	add(w.savePath, false)
	if includeAuto && w.cfg.AutoSaveDir != "" {
		if des, err := os.ReadDir(w.cfg.AutoSaveDir); err == nil {
			for _, de := range des {
				if de.IsDir() || !autoSaveName.MatchString(de.Name()) {
					continue
				}
				add(filepath.Join(w.cfg.AutoSaveDir, de.Name()), true)
			}
		}
	}
	return out
}

// startup ingests the newest qualifying save: a matching auto-save newer than
// the named save, else the named save itself.
func (w *Watcher) startup(ctx context.Context, autoOK bool) error {
	named := candidate{path: w.savePath}
	st, err := waitStable(ctx, w.savePath, w.cfg.Settle)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		w.emit(Event{Path: w.savePath, Err: fmt.Errorf("named save %s: %w", w.savePath, err)})
		return nil
	}
	named.stamp = st
	snap, _, err := w.load(ctx, named)
	if err != nil {
		return err
	}
	if snap == nil {
		return nil // parse failure already reported; the follow loop retries on change
	}
	w.setBaseline(snap)

	if autoOK {
		var autos []candidate
		for _, c := range w.candidates(true) {
			if c.auto && c.wt.After(snap.WriteTime) {
				autos = append(autos, c)
			}
		}
		sort.Slice(autos, func(i, j int) bool { return newer(autos[i], autos[j]) })
		for _, c := range autos {
			s, ov, err := w.load(ctx, c)
			if err != nil {
				return err
			}
			if s != nil {
				w.accept(c, s, ov)
				return nil
			}
		}
	}
	w.accept(named, snap, 0)
	return nil
}

// scan ingests every unhandled candidate that is newer than the last
// snapshot, oldest first so each world check runs against the previous accept.
func (w *Watcher) scan(ctx context.Context) error {
	cands := w.candidates(w.baseline != nil)
	var todo []candidate
	for _, c := range cands {
		if w.baseline != nil && !c.wt.After(w.last) {
			w.seen[c.path] = c.stamp
			continue
		}
		todo = append(todo, c)
	}
	sort.Slice(todo, func(i, j int) bool { return newer(todo[j], todo[i]) })
	for _, c := range todo {
		s, ov, err := w.load(ctx, c)
		if err != nil {
			return err
		}
		if s != nil {
			w.accept(c, s, ov)
		}
	}
	return nil
}

// newer orders candidates by write time, newest first; the file name breaks
// ties, which for auto-saves is the embedded timestamp.
func newer(a, b candidate) bool {
	if !a.wt.Equal(b.wt) {
		return a.wt.After(b.wt)
	}
	return a.path > b.path
}

// load waits for c to settle, parses it and applies the newer-than and world
// checks. It returns the snapshot when c is acceptable. In every other case it
// returns nil and, when the file deserves a report, has already emitted it.
// The only error is the context ending.
func (w *Watcher) load(ctx context.Context, c candidate) (*run8save.Snapshot, float64, error) {
	st, err := waitStable(ctx, c.path, w.cfg.Settle)
	if err != nil {
		if ctx.Err() != nil {
			return nil, 0, ctx.Err()
		}
		return nil, 0, nil // vanished while settling; the next scan decides
	}
	snap, err := run8save.ParseFile(c.path)
	if err != nil {
		w.seen[c.path] = st
		w.emit(Event{Path: c.path, Auto: c.auto, Err: err})
		return nil, 0, nil
	}
	w.seen[c.path] = st
	if w.baseline != nil && !snap.WriteTime.After(w.last) {
		return nil, 0, nil
	}
	if !c.auto {
		return snap, 0, nil
	}
	present, total := overlapCounts(snap.CarKeys(), w.baseKeys)
	ov := overlapRatio(present, total)
	if ov < w.cfg.MinOverlap {
		w.emit(Event{Path: c.path, Auto: true, Overlap: ov, Warning: fmt.Sprintf(
			"auto-save %s ignored: only %d%% of its %d cars are in %s (need %d%%)",
			filepath.Base(c.path), percent(present, total), total, w.baseline.FileName, int(w.cfg.MinOverlap*100+0.5))})
		return nil, 0, nil
	}
	return snap, ov, nil
}

func (w *Watcher) setBaseline(s *run8save.Snapshot) {
	w.baseline, w.baseKeys, w.last = s, s.CarKeys(), s.WriteTime
}

// accept records snap as the last accepted snapshot and emits it.
func (w *Watcher) accept(c candidate, snap *run8save.Snapshot, ov float64) {
	w.setBaseline(snap)
	w.emit(Event{Snapshot: snap, Path: c.path, Auto: c.auto, Overlap: ov})
}

// overlapCounts returns how many keys of cand are in base, and len(cand).
func overlapCounts(cand, base map[string]struct{}) (present, total int) {
	for k := range cand {
		if _, ok := base[k]; ok {
			present++
		}
	}
	return present, len(cand)
}

func overlapRatio(present, total int) float64 {
	if total == 0 {
		return 0
	}
	return float64(present) / float64(total)
}

// percent is the whole-number floor of present/total, so a value just under a
// threshold never displays as the threshold.
func percent(present, total int) int {
	if total == 0 {
		return 0
	}
	return present * 100 / total
}

// Overlap returns |cand ∩ base| / |cand|, the share of cand's car keys that
// are also in base. It is 0 when cand is empty.
func Overlap(cand, base map[string]struct{}) float64 {
	return overlapRatio(overlapCounts(cand, base))
}
