package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// dataDirName is the folder created under the user config directory
// (%APPDATA% on Windows).
const dataDirName = "Run8OpsCenter"

// ResolveDataDir picks the data directory, checks it against the guard and
// creates it. flagValue is the --data flag; when empty the directory is
// os.UserConfigDir()/Run8OpsCenter. The guard check runs before anything is
// created, so a refused directory leaves the disk untouched. On success the
// directory is registered with the guard so that a later SetInstallDir cannot
// swallow it.
//
// Startup order: ResolveDataDir, LoadSettings, then g.SetInstallDir with the
// saved install folder.
func ResolveDataDir(flagValue string, g *Guard) (string, error) {
	dir := flagValue
	if dir == "" {
		base, err := os.UserConfigDir()
		if err != nil {
			return "", fmt.Errorf("locate user config directory (use --data): %w", err)
		}
		dir = filepath.Join(base, dataDirName)
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("data directory %q: %w", dir, err)
	}
	if err := g.check("use as data directory", abs, false); err != nil {
		return "", err
	}
	if err := g.MkdirAll(abs, 0o755); err != nil {
		return "", fmt.Errorf("create data directory: %w", err)
	}
	g.SetDataDir(abs)
	return abs, nil
}

// ValidateProfilePath refuses a route profile path inside a protected Run8
// directory. It does not require the path to exist.
func ValidateProfilePath(path string, g *Guard) error {
	if path == "" {
		return errors.New("profile path is empty")
	}
	return g.check("use as profile path", path, false)
}
