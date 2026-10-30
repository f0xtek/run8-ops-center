package engine

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"strconv"
	"strings"

	"run8-ops-center/internal/profile"
	"run8-ops-center/internal/run8route"
	"run8-ops-center/internal/store"
	"run8-ops-center/internal/trackmap"
)

// ErrStaleImport is returned by ConfirmImport when the Run8 route files or
// the track map changed since the preview was built. The user needs a new
// preview.
var ErrStaleImport = errors.New("the Run8 track data or the track map changed since the preview; review a new preview")

// warnRun8 is the Warning.Kind of an unusable Run8 route file.
const warnRun8 = "run8"

// noRun8Reason is Run8State.Reason for a profile without a run8 block.
const noRun8Reason = "profile names no Run8 folders"

// installRoot is the Run8 install folder route data is read from.
func (e *Engine) installRoot() string {
	return firstNonEmpty(e.settings.RunInstallDir, store.DefaultInstallDir)
}

// routeFolders lists the route folders of the active profile's run8 block.
func routeFolders(cfg *profile.Run8Config) []run8route.Route {
	out := make([]run8route.Route, len(cfg.Routes))
	for i, r := range cfg.Routes {
		out[i] = run8route.Route{Folder: r.Folder}
	}
	return out
}

// routeData is what learn mode and the import build from: nil when the
// profile names no Run8 folders.
func (e *Engine) routeData() *run8route.Data {
	if e.base.Run8 == nil {
		return nil
	}
	return &e.route.Data
}

// loadRoute reads the Run8 route files for the active profile and replaces
// the data the engine holds. Each unusable file becomes a warning naming its
// path and reason. The anchor milepost findings go to the effective
// profile's report. It does not recompute or publish.
func (e *Engine) loadRoute() {
	e.dropWarnings(warnRun8)
	e.route = run8route.Result{}
	e.anchorProblems = nil
	if cfg := e.base.Run8; cfg != nil {
		e.route = run8route.Load(e.installRoot(), cfg.Region, routeFolders(cfg))
		for _, st := range e.route.Files {
			if !st.OK {
				e.addWarning(warnRun8, fmt.Sprintf("Run8 route file %s could not be used: %s", st.Path, reason(st.Err)))
			}
		}
		if e.fileOK(run8route.MilepostsFile) {
			names := make([]string, len(e.route.Data.Mileposts))
			for i, m := range e.route.Data.Mileposts {
				names[i] = run8route.NormTag(m.Station)
			}
			e.anchorProblems = e.base.ValidateAnchors(names)
		}
	}
	e.eff = e.effectiveProfile()
	e.profileWarnings()
}

// fileOK reports whether the named route file was decoded.
func (e *Engine) fileOK(name string) bool {
	for _, st := range e.route.Files {
		if st.Name == name {
			return st.OK
		}
	}
	return false
}

// reason is the error text without the "open <path>:" prefix, since the
// warning names the path itself.
func reason(err error) string {
	if err == nil {
		return "unknown error"
	}
	var pe *fs.PathError
	if errors.As(err, &pe) {
		return pe.Err.Error()
	}
	return err.Error()
}

// refreshRun8 rebuilds the View's Run8 state. It stats the route files and
// compares them with the last import, without decoding anything.
func (e *Engine) refreshRun8() {
	st := Run8State{}
	imp := e.tm.Run8Import
	if imp != nil {
		st.LastImport = imp.At
	}
	cfg := e.base.Run8
	if cfg == nil {
		st.Status, st.Reason = Run8Unavailable, noRun8Reason
		e.run8 = st
		return
	}
	st.HasRun8Config = true
	usable := 0
	for _, f := range e.route.Files {
		if f.OK {
			usable++
		} else {
			st.Unavailable = append(st.Unavailable, f)
		}
	}
	switch {
	case usable == 0:
		st.Status, st.Reason = Run8Unavailable, "no Run8 route file could be read"
	case len(st.Unavailable) > 0:
		st.Status = Run8Partial
	default:
		st.Status = Run8Ready
	}
	if imp != nil {
		cur := run8route.Stat(e.installRoot(), cfg.Region, routeFolders(cfg))
		st.Changed = !run8route.SameFingerprint(cur, imp.Files)
	}
	st.ImportAvailable = usable > 0 && imp == nil
	e.run8 = st
}

// importBuild is a diff built from freshly read route files.
type importBuild struct {
	diff     trackmap.ImportDiff
	unusable []run8route.FileStatus
	token    string
}

// buildImport re-reads the route files and diffs the import against the
// current track map. The engine adopts the fresh data, so the caller must
// recompute and publish afterwards, also on error.
func (e *Engine) buildImport() (importBuild, error) {
	var b importBuild
	if e.base.Run8 == nil {
		return b, errors.New(noRun8Reason)
	}
	e.loadRoute()
	usable := 0
	for _, f := range e.route.Files {
		if f.OK {
			usable++
		} else {
			b.unusable = append(b.unusable, f)
		}
	}
	if usable == 0 {
		var parts []string
		for _, f := range b.unusable {
			parts = append(parts, f.Path+": "+reason(f.Err))
		}
		return b, fmt.Errorf("no Run8 route file could be read: %s", strings.Join(parts, "; "))
	}
	b.diff = e.tm.Diff(trackmap.BuildImport(e.route.Data, e.eff))
	b.token = importToken(e.route.Fingerprint, e.tm)
	return b, nil
}

// importToken hashes the route file fingerprints with the track map revision.
func importToken(fp []run8route.FileFingerprint, tm *trackmap.Map) string {
	h := sha256.New()
	for _, f := range fp {
		fmt.Fprintf(h, "%s\x00%d\x00%s\n", f.Path, f.Size, strconv.FormatInt(f.ModTime.UnixNano(), 10))
	}
	fmt.Fprintf(h, "map\x00%s", tm.Revision())
	return hex.EncodeToString(h.Sum(nil))
}

// PreviewImport re-reads the Run8 route files and diffs the import against
// the track map. It changes no label.
func (e *Engine) PreviewImport() (ImportPreview, error) {
	var out ImportPreview
	err := e.do(func() error {
		b, err := e.buildImport()
		out = ImportPreview{Diff: b.diff, Unusable: b.unusable, Token: b.token}
		e.recompute(false)
		if ferr := e.finish(); err == nil {
			err = ferr
		}
		return err
	})
	if err != nil {
		return ImportPreview{}, err
	}
	return out, nil
}

// ConfirmImport applies the import again from freshly read files and saves it,
// unless the route files or the track map differ from what the token names.
func (e *Engine) ConfirmImport(token string) error {
	return e.do(func() error {
		b, err := e.buildImport()
		if err == nil && (token == "" || b.token != token) {
			err = ErrStaleImport
		}
		if err != nil {
			e.recompute(false)
			if ferr := e.finish(); ferr != nil {
				return errors.Join(err, ferr)
			}
			return err
		}
		tm := e.tm.Clone()
		tm.ApplyImport(b.diff, e.route.Fingerprint, e.now())
		if err := tm.Save(e.guard, e.mapPath()); err != nil {
			return err
		}
		e.tm = tm
		e.logf("import", "Imported Run8 track labels: %d new, %d changed, %d removed, %d kept manual",
			len(b.diff.New), len(b.diff.Changed), len(b.diff.Removed), len(b.diff.KeptManual))
		e.recompute(true)
		return e.finish()
	})
}
