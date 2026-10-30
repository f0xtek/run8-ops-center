// Package store owns everything the application writes to disk: the
// protected-path guard (guard.go), atomic file replacement (atomic.go), data
// directory resolution (datadir.go), settings.json (settings.go) and
// state.json (state.go).
//
// The guard is the single choke point that keeps the application out of the
// Run8 install directory. Every write, rename and remove in the program goes
// through a *Guard.
package store

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
)

// DefaultInstallDir is the built-in protected Run8 install directory.
const DefaultInstallDir = `C:\Run8Studios\Run8 Train Simulator V3`

// ErrProtectedPath is wrapped by every error that refuses a path because it
// lies inside (or, for removals, contains) a protected Run8 directory.
var ErrProtectedPath = errors.New("protected Run8 path")

// ErrInstallConflict is wrapped by the error SetInstallDir returns when the
// new install folder contains the data directory or a profile path.
var ErrInstallConflict = errors.New("install folder conflicts with existing path")

// ProtectedPathError reports a refused path.
type ProtectedPathError struct {
	Op        string // what was attempted: "write", "remove", "check", ...
	Path      string // the path as given
	Resolved  string // the absolute, symlink-resolved form
	Protected string // the protected directory (as registered) that matched
	Covers    bool   // the path contains the protected directory rather than lying in it
}

func (e *ProtectedPathError) Error() string {
	rel := "is inside"
	if e.Covers {
		rel = "contains"
	}
	return fmt.Sprintf("refusing to %s %q: %q %s protected Run8 directory %q",
		e.Op, e.Path, e.Resolved, rel, e.Protected)
}

// Unwrap lets errors.Is(err, ErrProtectedPath) match.
func (e *ProtectedPathError) Unwrap() error { return ErrProtectedPath }

// Guard enforces the protected-path rules. The zero value is not usable; call
// NewGuard. A Guard is safe for concurrent use.
//
// It satisfies profile.FileWriter (MkdirAll, WriteFile) and the trackmap
// FileWriter (MkdirAll, WriteFile, Rename).
type Guard struct {
	fold bool   // compare case-insensitively
	seps string // characters that separate path components

	mu       sync.RWMutex
	fixed    []string            // built-in default plus extras given to NewGuard
	install  string              // configured Run8 install folder ("" = none)
	dataDir  string              // registered data directory ("" = none)
	profiles map[string]struct{} // registered profile paths
	logger   *slog.Logger

	// afterTempWrite, when set by an in-package test, runs after the temporary
	// file of AtomicWriteFile is fully written and before it is renamed.
	afterTempWrite func(tmp string) error
}

// NewGuard returns a Guard whose protected set is DefaultInstallDir plus the
// given extra directories. Comparison is case-insensitive when the host is
// Windows.
func NewGuard(extraProtected ...string) *Guard {
	return newGuard(foldCase(), string(filepath.Separator), extraProtected...)
}

func newGuard(fold bool, seps string, extra ...string) *Guard {
	g := &Guard{
		fold:     fold,
		seps:     seps,
		profiles: map[string]struct{}{},
		logger:   slog.Default(),
	}
	g.fixed = append(g.fixed, DefaultInstallDir)
	for _, e := range extra {
		if e != "" {
			g.fixed = append(g.fixed, e)
		}
	}
	return g
}

// foldCase reports whether paths on this host compare case-insensitively.
func foldCase() bool { return runtime.GOOS == "windows" }

// SetLogger sets where blocked operations are logged. nil restores the
// default logger.
func (g *Guard) SetLogger(l *slog.Logger) {
	if l == nil {
		l = slog.Default()
	}
	g.mu.Lock()
	g.logger = l
	g.mu.Unlock()
}

// SetDataDir registers the data directory so that SetInstallDir can reject an
// install folder that contains it. It does not validate dir; use Check first.
func (g *Guard) SetDataDir(dir string) {
	g.mu.Lock()
	g.dataDir = dir
	g.mu.Unlock()
}

// RegisterProfilePath records a profile path in use. Call it only after
// ValidateProfilePath accepted the path.
func (g *Guard) RegisterProfilePath(path string) {
	g.mu.Lock()
	g.profiles[path] = struct{}{}
	g.mu.Unlock()
}

// UnregisterProfilePath forgets a profile path, for example after the user
// switches to another profile.
func (g *Guard) UnregisterProfilePath(path string) {
	g.mu.Lock()
	delete(g.profiles, path)
	g.mu.Unlock()
}

// InstallDir returns the configured Run8 install folder, or "".
func (g *Guard) InstallDir() string {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.install
}

// Protected lists the protected directories as registered.
func (g *Guard) Protected() []string {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.protectedLocked()
}

func (g *Guard) protectedLocked() []string {
	out := append([]string(nil), g.fixed...)
	if g.install != "" {
		out = append(out, g.install)
	}
	return out
}

// SetInstallDir sets the configured Run8 install folder, which joins the
// protected set. It is rejected, leaving the previous folder in effect, when dir
// contains the registered data directory or any registered profile path. An
// empty dir clears the configured folder.
func (g *Guard) SetInstallDir(dir string) error {
	if dir == "" {
		g.mu.Lock()
		g.install = ""
		g.mu.Unlock()
		return nil
	}
	root, err := g.resolve(dir)
	if err != nil {
		return fmt.Errorf("resolve install folder %q: %w", dir, err)
	}

	g.mu.Lock()
	defer g.mu.Unlock()
	if g.dataDir != "" {
		if err := g.conflictLocked(dir, root, "data directory", g.dataDir); err != nil {
			return err
		}
	}
	for p := range g.profiles {
		if err := g.conflictLocked(dir, root, "profile path", p); err != nil {
			return err
		}
	}
	g.install = dir
	return nil
}

func (g *Guard) conflictLocked(install string, root resolved, what, path string) error {
	t, err := g.resolve(path)
	if err != nil {
		return fmt.Errorf("resolve %s %q: %w", what, path, err)
	}
	if g.inside(t, root) {
		return fmt.Errorf("%w: install folder %q contains the %s %q (resolved to %q); move the %s or pick another install folder",
			ErrInstallConflict, install, what, path, t.real, what)
	}
	return nil
}

// Check returns a *ProtectedPathError when path, once made absolute, cleaned
// and symlink-resolved, lies inside a protected directory.
func (g *Guard) Check(path string) error { return g.check("write to", path, false) }

// checkRemove is Check for destructive operations: it also refuses a path that
// contains a protected directory, since removing it would remove that too.
func (g *Guard) checkRemove(op, path string) error { return g.check(op, path, true) }

func (g *Guard) check(op, path string, covering bool) error {
	if path == "" {
		return fmt.Errorf("refusing to %s an empty path", op)
	}
	t, err := g.resolve(path)
	if err != nil {
		// Fail closed: a path that cannot be resolved cannot be proven safe.
		return fmt.Errorf("refusing to %s %q: resolve path: %w", op, path, err)
	}
	g.mu.RLock()
	roots := g.protectedLocked()
	logger := g.logger
	g.mu.RUnlock()
	for _, r := range roots {
		rr, err := g.resolve(r)
		if err != nil {
			rr = resolved{abs: r, real: r}
		}
		var perr *ProtectedPathError
		switch {
		case g.inside(t, rr):
			perr = &ProtectedPathError{Op: op, Path: path, Resolved: t.real, Protected: r}
		case covering && g.inside(rr, t):
			perr = &ProtectedPathError{Op: op, Path: path, Resolved: t.real, Protected: r, Covers: true}
		}
		if perr != nil {
			logger.Error("blocked operation on protected Run8 path",
				"op", op, "path", path, "resolved", t.real, "protected", r)
			return perr
		}
	}
	return nil
}

// resolved holds both forms of a path that the guard compares: the absolute
// lexical path and the symlink-resolved path. A target is refused when either
// form lies inside either form of a protected directory.
type resolved struct{ abs, real string }

func (g *Guard) resolve(path string) (resolved, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return resolved{}, err
	}
	real, err := resolvePath(abs, 0)
	if err != nil {
		return resolved{}, err
	}
	return resolved{abs: abs, real: real}, nil
}

// inside reports whether t lies in or equals root, comparing every pairing of
// the two forms.
func (g *Guard) inside(t, root resolved) bool {
	for _, tp := range [2]string{t.abs, t.real} {
		for _, rp := range [2]string{root.abs, root.real} {
			if within(tp, rp, g.fold, g.seps) {
				return true
			}
		}
	}
	return false
}

const maxSymlinkDepth = 40

// resolvePath resolves symlinks in abs (already absolute and clean). When abs
// does not exist, the nearest existing ancestor is resolved and the missing
// tail re-appended; a dangling symlink met on the way is followed so that a
// write through it is judged by where it would land.
func resolvePath(abs string, depth int) (string, error) {
	if depth > maxSymlinkDepth {
		return "", errors.New("too many levels of symbolic links")
	}
	cur := abs
	var rest []string
	for {
		real, err := filepath.EvalSymlinks(cur)
		if err == nil {
			return joinRest(real, rest), nil
		}
		if !errors.Is(err, fs.ErrNotExist) && !errors.Is(err, syscall.ENOTDIR) {
			return "", err
		}
		if fi, lerr := os.Lstat(cur); lerr == nil && fi.Mode()&os.ModeSymlink != 0 {
			target, rerr := os.Readlink(cur)
			if rerr != nil {
				return "", rerr
			}
			if !filepath.IsAbs(target) {
				target = filepath.Join(filepath.Dir(cur), target)
			}
			real, rerr := resolvePath(filepath.Clean(target), depth+1)
			if rerr != nil {
				return "", rerr
			}
			return joinRest(real, rest), nil
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return abs, nil
		}
		rest = append(rest, filepath.Base(cur))
		cur = parent
	}
}

func joinRest(base string, rest []string) string {
	for i := len(rest) - 1; i >= 0; i-- {
		base = filepath.Join(base, rest[i])
	}
	return base
}

// within reports whether path equals root or lies below it, comparing whole
// components split on any character in seps, and ignoring case when fold is
// set. "Run8 Train Simulator V3 Ops" is therefore not inside "Run8 Train
// Simulator V3".
func within(path, root string, fold bool, seps string) bool {
	pc, rc := components(path, seps), components(root, seps)
	if len(rc) > len(pc) {
		return false
	}
	for i, r := range rc {
		if fold {
			if !strings.EqualFold(pc[i], r) {
				return false
			}
		} else if pc[i] != r {
			return false
		}
	}
	return true
}

// components splits p into non-empty path components. A Windows extended-length
// prefix (\\?\) is dropped when backslash is a separator.
func components(p, seps string) []string {
	if strings.Contains(seps, `\`) {
		p = strings.TrimPrefix(p, `\\?\`)
	}
	return strings.FieldsFunc(p, func(r rune) bool { return strings.ContainsRune(seps, r) })
}

// MkdirAll checks path, then creates it and any missing parents.
func (g *Guard) MkdirAll(path string, perm os.FileMode) error {
	if err := g.check("create directory", path, false); err != nil {
		return err
	}
	return os.MkdirAll(path, perm)
}

// WriteFile checks path, then writes data to it. Use AtomicWriteFile for files
// that must never be left half-written.
func (g *Guard) WriteFile(path string, data []byte, perm os.FileMode) error {
	if err := g.check("write", path, false); err != nil {
		return err
	}
	return os.WriteFile(path, data, perm)
}

// Rename checks both paths, then renames oldpath to newpath.
func (g *Guard) Rename(oldpath, newpath string) error {
	if err := g.checkRemove("rename", oldpath); err != nil {
		return err
	}
	if err := g.check("rename to", newpath, false); err != nil {
		return err
	}
	return os.Rename(oldpath, newpath)
}

// Remove checks path, then removes the file or empty directory.
func (g *Guard) Remove(path string) error {
	if err := g.checkRemove("remove", path); err != nil {
		return err
	}
	return os.Remove(path)
}

// RemoveAll checks path, then removes it and everything below it.
func (g *Guard) RemoveAll(path string) error {
	if err := g.checkRemove("remove", path); err != nil {
		return err
	}
	return os.RemoveAll(path)
}
