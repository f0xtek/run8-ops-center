package store

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestResolveDataDirInsideInstallCreatesNothing(t *testing.T) {
	g, tmp, dir := install(t)
	if err := os.Symlink(dir, filepath.Join(tmp, "link")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	t.Chdir(tmp)
	before := walk(t, tmp)

	// What `--data <path>` could look like at startup.
	flags := map[string]string{
		"inside":             filepath.Join(dir, "OpsData"),
		"deep and new":       filepath.Join(dir, "a", "b", "c"),
		"install itself":     dir,
		"relative":           "Run8 Train Simulator V3/OpsData",
		"relative dot-dot":   "x/../Run8 Train Simulator V3/OpsData",
		"through symlink":    filepath.Join(tmp, "link", "OpsData"),
		"existing subfolder": filepath.Join(dir, "Content", "ops"),
	}
	for name, flag := range flags {
		t.Run(name, func(t *testing.T) {
			got, err := ResolveDataDir(flag, g)
			if !errors.Is(err, ErrProtectedPath) {
				t.Fatalf("ResolveDataDir(%q) = %q, %v; want ErrProtectedPath", flag, got, err)
			}
			if got != "" {
				t.Errorf("returned dir %q with an error", got)
			}
			if after := walk(t, tmp); !equalStrings(after, before) {
				t.Fatalf("disk changed by a refused data dir:\n before %q\n after  %q", before, after)
			}
		})
	}
}

func TestResolveDataDirCreatesAndRegisters(t *testing.T) {
	g, tmp, dir := install(t)
	t.Chdir(tmp)

	got, err := ResolveDataDir("data/nested", g)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(tmp, "data", "nested")
	if got != want {
		// t.TempDir may sit behind a symlink (/var -> /private/var); compare resolved.
		a, _ := filepath.EvalSymlinks(got)
		b, _ := filepath.EvalSymlinks(want)
		if a != b {
			t.Errorf("ResolveDataDir = %q, want %q", got, want)
		}
	}
	if fi, err := os.Stat(got); err != nil || !fi.IsDir() {
		t.Fatalf("data dir not created: %v", err)
	}

	// The data dir is now registered: an install folder over it is refused.
	if err := g.SetInstallDir(tmp); !errors.Is(err, ErrInstallConflict) {
		t.Errorf("SetInstallDir over data dir = %v", err)
	}
	// A sibling-name data dir is allowed.
	if _, err := ResolveDataDir(dir+" Ops", g); err != nil {
		t.Errorf("sibling-name data dir: %v", err)
	}
}

func TestResolveDataDirDefault(t *testing.T) {
	cfg := t.TempDir()
	t.Setenv("HOME", cfg)
	t.Setenv("XDG_CONFIG_HOME", cfg)
	t.Setenv("APPDATA", cfg)
	base, err := os.UserConfigDir()
	if err != nil {
		t.Skipf("no user config dir: %v", err)
	}
	got, err := ResolveDataDir("", NewGuard())
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(base, "Run8OpsCenter"); got != want {
		t.Errorf("default data dir = %q, want %q", got, want)
	}
	if fi, err := os.Stat(got); err != nil || !fi.IsDir() {
		t.Errorf("default data dir not created: %v", err)
	}
}

func TestResolveDataDirDefaultInsideInstall(t *testing.T) {
	// A config dir that sits inside a protected install folder is refused too.
	g, tmp, dir := install(t)
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("APPDATA", dir)
	before := walk(t, tmp)
	if _, err := ResolveDataDir("", g); !errors.Is(err, ErrProtectedPath) {
		t.Fatalf("err = %v, want ErrProtectedPath", err)
	}
	if after := walk(t, tmp); !equalStrings(after, before) {
		t.Errorf("disk changed: %q -> %q", before, after)
	}
}

func TestValidateProfilePath(t *testing.T) {
	g, tmp, dir := install(t)
	t.Chdir(tmp)
	tests := []struct {
		name    string
		path    string
		blocked bool
	}{
		{"inside content", filepath.Join(dir, "Content", "profile.yaml"), true},
		{"relative inside", "Run8 Train Simulator V3/Content", true},
		{"not yet existing inside", filepath.Join(dir, "profiles", "ags"), true},
		{"sibling name allowed", filepath.Join(tmp, "Run8 Train Simulator V3 Ops", "profiles", "ags"), false},
		{"elsewhere allowed", filepath.Join(tmp, "profiles", "ags"), false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateProfilePath(tc.path, g)
			if tc.blocked != errors.Is(err, ErrProtectedPath) || (!tc.blocked && err != nil) {
				t.Errorf("ValidateProfilePath(%q) = %v, blocked want %v", tc.path, err, tc.blocked)
			}
		})
	}
	if err := ValidateProfilePath("", g); err == nil {
		t.Error("empty profile path accepted")
	}
}
