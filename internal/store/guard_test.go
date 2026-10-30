package store

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"run8-ops-center/internal/profile"
	"run8-ops-center/internal/trackmap"
)

// Compile-time checks that the guard can stand in for the writers other
// packages ask for: profile.FileWriter, and trackmap's (MkdirAll, WriteFile,
// Rename).
var (
	_ profile.FileWriter  = (*Guard)(nil)
	_ trackmap.FileWriter = (*Guard)(nil)
)

const (
	unixSeps = "/"
	winSeps  = `\/`
)

func TestWithin(t *testing.T) {
	tests := []struct {
		name       string
		path, root string
		fold       bool
		seps       string
		want       bool
	}{
		{"equal", "/a/b", "/a/b", false, unixSeps, true},
		{"child", "/a/b/c/d", "/a/b", false, unixSeps, true},
		{"sibling prefix is not inside", "/a/b Ops", "/a/b", false, unixSeps, false},
		{"sibling prefix with child", "/a/b Ops/x", "/a/b", false, unixSeps, false},
		{"parent is not inside", "/a", "/a/b", false, unixSeps, false},
		{"case differs, case-sensitive", "/a/B/c", "/a/b", false, unixSeps, false},
		{"case differs, folded", "/a/B/c", "/a/b", true, unixSeps, true},
		{"trailing separator", "/a/b/", "/a/b", false, unixSeps, true},
		{"windows exact", `C:\Run8Studios\Run8 Train Simulator V3`, `C:\Run8Studios\Run8 Train Simulator V3`, true, winSeps, true},
		{"windows child", `C:\Run8Studios\Run8 Train Simulator V3\OpsData`, `C:\Run8Studios\Run8 Train Simulator V3`, true, winSeps, true},
		{"windows lower case variant", `c:\run8studios\RUN8 TRAIN SIMULATOR V3\x`, `C:\Run8Studios\Run8 Train Simulator V3`, true, winSeps, true},
		{"windows forward slashes", `C:/Run8Studios/Run8 Train Simulator V3/x`, `C:\Run8Studios\Run8 Train Simulator V3`, true, winSeps, true},
		{"windows sibling name allowed", `C:\Run8Studios\Run8 Train Simulator V3 Ops`, `C:\Run8Studios\Run8 Train Simulator V3`, true, winSeps, false},
		{"windows sibling name with child allowed", `C:\Run8Studios\Run8 Train Simulator V3 Ops\data`, `C:\Run8Studios\Run8 Train Simulator V3`, true, winSeps, false},
		{"windows other drive", `D:\Run8Studios\Run8 Train Simulator V3\x`, `C:\Run8Studios\Run8 Train Simulator V3`, true, winSeps, false},
		{"windows extended-length prefix", `\\?\C:\Run8Studios\Run8 Train Simulator V3\x`, `C:\Run8Studios\Run8 Train Simulator V3`, true, winSeps, true},
		{"windows parent", `C:\Run8Studios`, `C:\Run8Studios\Run8 Train Simulator V3`, true, winSeps, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := within(tc.path, tc.root, tc.fold, tc.seps); got != tc.want {
				t.Errorf("within(%q, %q, fold=%v) = %v, want %v", tc.path, tc.root, tc.fold, got, tc.want)
			}
		})
	}
}

func TestFoldCaseFollowsHost(t *testing.T) {
	if got, want := foldCase(), runtime.GOOS == "windows"; got != want {
		t.Errorf("foldCase() = %v on %s", got, runtime.GOOS)
	}
}

func TestDefaultProtectedDirAlwaysPresent(t *testing.T) {
	g := NewGuard("/extra")
	got := g.Protected()
	if len(got) != 2 || got[0] != DefaultInstallDir || got[1] != "/extra" {
		t.Errorf("Protected() = %q", got)
	}
}

// install makes <tmp>/Run8 Train Simulator V3 and returns a guard protecting it.
func install(t *testing.T) (*Guard, string, string) {
	t.Helper()
	tmp := t.TempDir()
	dir := filepath.Join(tmp, "Run8 Train Simulator V3")
	if err := os.MkdirAll(filepath.Join(dir, "Content"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "Content", "existing.txt"), []byte("run8"), 0o644); err != nil {
		t.Fatal(err)
	}
	return NewGuard(dir), tmp, dir
}

func TestCheck(t *testing.T) {
	g, tmp, dir := install(t)
	if err := os.Symlink(dir, filepath.Join(tmp, "link")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := os.Symlink(filepath.Join(dir, "Content"), filepath.Join(tmp, "contentlink")); err != nil {
		t.Fatal(err)
	}
	// A dangling link: nothing exists at its target yet, but a write through
	// it would create the file inside the install directory.
	if err := os.Symlink(filepath.Join(dir, "new-from-dangling.txt"), filepath.Join(tmp, "dangling")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(tmp, "elsewhere"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(tmp)

	tests := []struct {
		name    string
		path    string
		blocked bool
	}{
		{"install dir itself", dir, true},
		{"existing file inside", filepath.Join(dir, "Content", "existing.txt"), true},
		{"not-yet-existing file inside", filepath.Join(dir, "OpsData", "state.json"), true},
		{"not-yet-existing deep dir inside", filepath.Join(dir, "a", "b", "c", "d"), true},
		{"dot-dot back into install", filepath.Join(tmp, "elsewhere", "..", "Run8 Train Simulator V3", "x"), true},
		{"relative path", "Run8 Train Simulator V3/x", true},
		{"relative path with dot-dot", "elsewhere/../Run8 Train Simulator V3/Content/y", true},
		{"symlink to install dir", filepath.Join(tmp, "link"), true},
		{"file below symlink to install dir", filepath.Join(tmp, "link", "OpsData", "x.json"), true},
		{"file below symlink to subfolder", filepath.Join(tmp, "contentlink", "x"), true},
		{"dangling symlink into install", filepath.Join(tmp, "dangling"), true},
		{"sibling name Ops allowed", filepath.Join(tmp, "Run8 Train Simulator V3 Ops"), false},
		{"file in sibling name Ops allowed", filepath.Join(tmp, "Run8 Train Simulator V3 Ops", "state.json"), false},
		{"parent of install allowed", tmp, false},
		{"other dir allowed", filepath.Join(tmp, "elsewhere", "settings.json"), false},
		{"relative other dir allowed", "elsewhere/settings.json", false},
		{"not-yet-existing path outside allowed", filepath.Join(tmp, "x", "y", "z"), false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := g.Check(tc.path)
			if tc.blocked {
				if !errors.Is(err, ErrProtectedPath) {
					t.Fatalf("Check(%q) = %v, want ErrProtectedPath", tc.path, err)
				}
				var pe *ProtectedPathError
				if !errors.As(err, &pe) || pe.Path != tc.path || pe.Protected != dir {
					t.Errorf("error does not name path and protected dir: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Check(%q) = %v, want nil", tc.path, err)
			}
		})
	}
}

func TestCheckCaseVariants(t *testing.T) {
	_, tmp, dir := install(t)
	// A guard in case-insensitive (Windows) mode, run over a real directory
	// tree: the variant spellings must be refused whether or not the host file
	// system also folds case.
	g := newGuard(true, string(filepath.Separator), dir)
	for _, p := range []string{
		filepath.Join(tmp, "RUN8 TRAIN SIMULATOR V3", "x"),
		filepath.Join(tmp, "run8 train simulator v3", "OpsData", "x"),
		filepath.Join(tmp, "Run8 Train Simulator v3"),
	} {
		if err := g.Check(p); !errors.Is(err, ErrProtectedPath) {
			t.Errorf("fold guard Check(%q) = %v, want ErrProtectedPath", p, err)
		}
	}
	// The sibling name stays allowed in either mode.
	if err := g.Check(filepath.Join(tmp, "RUN8 TRAIN SIMULATOR V3 OPS", "x")); err != nil {
		t.Errorf("sibling in fold mode: %v", err)
	}
	// A case-sensitive guard on a case-sensitive spelling difference does not
	// match: this proves the fold flag is what makes the difference.
	strict := newGuard(false, string(filepath.Separator), filepath.Join(tmp, "Nowhere", "Run8"))
	if err := strict.Check(filepath.Join(tmp, "Nowhere", "RUN8", "x")); err != nil {
		t.Errorf("case-sensitive guard refused a different spelling: %v", err)
	}
	fold := newGuard(true, string(filepath.Separator), filepath.Join(tmp, "Nowhere", "Run8"))
	if err := fold.Check(filepath.Join(tmp, "Nowhere", "RUN8", "x")); !errors.Is(err, ErrProtectedPath) {
		t.Errorf("fold guard accepted a different spelling: %v", err)
	}
}

func TestCheckEmptyPath(t *testing.T) {
	if err := NewGuard().Check(""); err == nil {
		t.Error("Check(\"\") = nil")
	}
}

func TestGuardOperationsBlockedInsideInstall(t *testing.T) {
	g, tmp, dir := install(t)
	existing := filepath.Join(dir, "Content", "existing.txt")
	outside := filepath.Join(tmp, "outside.txt")
	if err := os.WriteFile(outside, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	ops := []struct {
		name string
		run  func() error
	}{
		{"MkdirAll", func() error { return g.MkdirAll(filepath.Join(dir, "OpsData"), 0o755) }},
		{"WriteFile new", func() error { return g.WriteFile(filepath.Join(dir, "new.txt"), []byte("x"), 0o644) }},
		{"WriteFile existing", func() error { return g.WriteFile(existing, []byte("changed"), 0o644) }},
		{"AtomicWriteFile new", func() error { return g.AtomicWriteFile(filepath.Join(dir, "state.json"), []byte("x"), 0o644) }},
		{"AtomicWriteFile existing", func() error { return g.AtomicWriteFile(existing, []byte("changed"), 0o644) }},
		{"Rename into", func() error { return g.Rename(outside, filepath.Join(dir, "moved.txt")) }},
		{"Rename over", func() error { return g.Rename(outside, existing) }},
		{"Rename out of", func() error { return g.Rename(existing, filepath.Join(tmp, "stolen.txt")) }},
		{"Remove", func() error { return g.Remove(existing) }},
		{"RemoveAll inside", func() error { return g.RemoveAll(filepath.Join(dir, "Content")) }},
		{"RemoveAll install", func() error { return g.RemoveAll(dir) }},
		{"RemoveAll parent of install", func() error { return g.RemoveAll(tmp) }},
		{"Remove parent of install", func() error { return g.Remove(tmp) }},
	}
	for _, op := range ops {
		t.Run(op.name, func(t *testing.T) {
			if err := op.run(); !errors.Is(err, ErrProtectedPath) {
				t.Fatalf("got %v, want ErrProtectedPath", err)
			}
		})
	}

	// Nothing changed on disk.
	if b, err := os.ReadFile(existing); err != nil || string(b) != "run8" {
		t.Errorf("existing file = %q, %v", b, err)
	}
	if _, err := os.Stat(outside); err != nil {
		t.Errorf("outside file gone: %v", err)
	}
	entries := walk(t, tmp)
	want := []string{
		"Run8 Train Simulator V3",
		"Run8 Train Simulator V3/Content",
		"Run8 Train Simulator V3/Content/existing.txt",
		"outside.txt",
	}
	if !equalStrings(entries, want) {
		t.Errorf("disk changed:\n got %q\nwant %q", entries, want)
	}
}

func TestGuardOperationsAllowedOutsideInstall(t *testing.T) {
	g, tmp, _ := install(t)
	sib := filepath.Join(tmp, "Run8 Train Simulator V3 Ops")
	if err := g.MkdirAll(sib, 0o755); err != nil {
		t.Fatal(err)
	}
	f := filepath.Join(sib, "a.txt")
	if err := g.WriteFile(f, []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	f2 := filepath.Join(sib, "b.txt")
	if err := g.Rename(f, f2); err != nil {
		t.Fatal(err)
	}
	if err := g.AtomicWriteFile(f2, []byte("b"), 0o644); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(f2); string(b) != "b" {
		t.Errorf("content = %q", b)
	}
	if err := g.Remove(f2); err != nil {
		t.Fatal(err)
	}
	if err := g.RemoveAll(sib); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(sib); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("sibling dir still there: %v", err)
	}
}

func TestSetInstallDir(t *testing.T) {
	tmp := t.TempDir()
	data := filepath.Join(tmp, "Run8", "OpsData")
	profiles := filepath.Join(tmp, "Other", "profiles", "ags")
	for _, d := range []string{data, profiles} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("contains data dir is rejected", func(t *testing.T) {
		g := NewGuard()
		if _, err := ResolveDataDir(data, g); err != nil {
			t.Fatal(err)
		}
		err := g.SetInstallDir(filepath.Join(tmp, "Run8"))
		if !errors.Is(err, ErrInstallConflict) {
			t.Fatalf("SetInstallDir = %v, want ErrInstallConflict", err)
		}
		if g.InstallDir() != "" {
			t.Errorf("install dir changed to %q after rejection", g.InstallDir())
		}
		// The rejected folder must not have become protected.
		if err := g.Check(filepath.Join(data, "settings.json")); err != nil {
			t.Errorf("data dir blocked after rejected install change: %v", err)
		}
	})

	t.Run("equal to data dir is rejected", func(t *testing.T) {
		g := NewGuard()
		g.SetDataDir(data)
		if err := g.SetInstallDir(data); !errors.Is(err, ErrInstallConflict) {
			t.Fatalf("SetInstallDir = %v, want ErrInstallConflict", err)
		}
	})

	t.Run("contains profile path is rejected and previous folder stays", func(t *testing.T) {
		g := NewGuard()
		g.SetDataDir(data)
		prev := filepath.Join(tmp, "Previous")
		if err := g.SetInstallDir(prev); err != nil {
			t.Fatal(err)
		}
		g.RegisterProfilePath(profiles)
		if err := g.SetInstallDir(filepath.Join(tmp, "Other")); !errors.Is(err, ErrInstallConflict) {
			t.Fatalf("SetInstallDir = %v, want ErrInstallConflict", err)
		}
		if g.InstallDir() != prev {
			t.Errorf("install dir = %q, want previous %q", g.InstallDir(), prev)
		}
		g.UnregisterProfilePath(profiles)
		if err := g.SetInstallDir(filepath.Join(tmp, "Other")); err != nil {
			t.Errorf("after unregistering: %v", err)
		}
	})

	t.Run("unrelated folder is accepted and becomes protected", func(t *testing.T) {
		g := NewGuard()
		g.SetDataDir(data)
		g.RegisterProfilePath(profiles)
		unrelated := filepath.Join(tmp, "Games", "Run8")
		if err := g.SetInstallDir(unrelated); err != nil {
			t.Fatal(err)
		}
		if err := g.Check(filepath.Join(unrelated, "Content", "x")); !errors.Is(err, ErrProtectedPath) {
			t.Errorf("Check inside new install dir = %v", err)
		}
		if err := g.Check(filepath.Join(tmp, "Games", "Run8 Ops")); err != nil {
			t.Errorf("sibling of new install dir blocked: %v", err)
		}
	})

	t.Run("replacing the install folder drops the old one", func(t *testing.T) {
		g := NewGuard()
		a, b := filepath.Join(tmp, "A"), filepath.Join(tmp, "B")
		if err := g.SetInstallDir(a); err != nil {
			t.Fatal(err)
		}
		if err := g.SetInstallDir(b); err != nil {
			t.Fatal(err)
		}
		if err := g.Check(filepath.Join(a, "x")); err != nil {
			t.Errorf("old install dir still protected: %v", err)
		}
		if err := g.Check(filepath.Join(b, "x")); !errors.Is(err, ErrProtectedPath) {
			t.Errorf("new install dir not protected: %v", err)
		}
		if err := g.SetInstallDir(""); err != nil || g.InstallDir() != "" {
			t.Errorf("clear: %v, %q", err, g.InstallDir())
		}
	})

	t.Run("symlinked install folder resolves onto the data dir", func(t *testing.T) {
		link := filepath.Join(tmp, "runlink")
		if err := os.Symlink(filepath.Join(tmp, "Run8"), link); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
		g := NewGuard()
		g.SetDataDir(data)
		if err := g.SetInstallDir(link); !errors.Is(err, ErrInstallConflict) {
			t.Fatalf("SetInstallDir(symlink) = %v, want ErrInstallConflict", err)
		}
	})
}

// walk lists every path below root, relative and slash-separated, sorted.
func walk(t *testing.T, root string) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(root, func(p string, _ os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p == root {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		out = append(out, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
