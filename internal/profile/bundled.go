package profile

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// BundledID is the ID and folder name of the profile embedded in the binary.
const BundledID = "ags-south-p1"

//go:embed bundled/ags-south-p1/profile.yaml bundled/ags-south-p1/IndustryConfiguration.csv bundled/ags-south-p1/train_limits.tsv
var bundledFS embed.FS

// bundledFiles lists the bundled profile files in write order. profile.yaml
// comes last, so an interrupted install or upgrade leaves no loadable half
// profile and the old version number, and the next start repeats the work.
var bundledFiles = []string{IndustryFile, LimitsFile, ProfileFile}

// InstallBundled writes the bundled AGS South Phase 1 profile to
// <profilesDir>/ags-south-p1 through w when that folder is absent.
//
// When the folder exists, it reads only schemaVersion from the installed
// profile.yaml. An older version is upgraded: each bundled file replaces the
// installed one, after the installed file is saved as <name>.v<old>.bak. An
// equal or newer version, a missing profile.yaml or one that does not parse
// leaves every file alone, and Load reports the problem. No other file in
// the folder, such as trackmap.json, is touched.
func InstallBundled(w FileWriter, profilesDir string) error {
	dst := filepath.Join(profilesDir, BundledID)
	if _, err := os.Stat(dst); err == nil {
		return upgradeBundled(w, dst)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("profile: check %s: %w", dst, err)
	}
	if err := w.MkdirAll(dst, 0o755); err != nil {
		return fmt.Errorf("profile: create %s: %w", dst, err)
	}
	for _, name := range bundledFiles {
		data, err := readBundled(name)
		if err != nil {
			return err
		}
		if err := w.WriteFile(filepath.Join(dst, name), data, 0o644); err != nil {
			return fmt.Errorf("profile: write %s: %w", name, err)
		}
	}
	return nil
}

func readBundled(name string) ([]byte, error) {
	data, err := bundledFS.ReadFile(path.Join("bundled", BundledID, name))
	if err != nil {
		return nil, fmt.Errorf("profile: read bundled %s: %w", name, err)
	}
	return data, nil
}

// upgradeBundled replaces the installed bundled profile in dir when its
// schema version is older than BundledSchemaVersion.
func upgradeBundled(w FileWriter, dst string) error {
	installed, err := os.ReadFile(filepath.Join(dst, ProfileFile))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("profile: read installed %s: %w", ProfileFile, err)
	}
	// A tolerant parse: unknown keys are fine, because only the version
	// matters here. Load reports every other problem.
	var head struct {
		SchemaVersion int `yaml:"schemaVersion"`
	}
	if err := yaml.Unmarshal(installed, &head); err != nil {
		return nil
	}
	old := schemaVersionOf(head.SchemaVersion)
	if old >= BundledSchemaVersion {
		return nil
	}

	for _, name := range bundledFiles {
		fresh, err := readBundled(name)
		if err != nil {
			return err
		}
		file := filepath.Join(dst, name)
		current, err := os.ReadFile(file)
		switch {
		case err == nil:
			// A file that already equals the bundled one was replaced by an
			// earlier, interrupted upgrade. Saving it would overwrite the
			// user's original backup.
			if bytes.Equal(current, fresh) {
				continue
			}
			backup := fmt.Sprintf("%s.v%d.bak", file, old)
			if err := w.WriteFile(backup, current, 0o644); err != nil {
				return fmt.Errorf("profile: back up %s: %w", name, err)
			}
		case errors.Is(err, os.ErrNotExist):
			// Nothing to back up, for example when the optional limits file
			// was never installed.
		default:
			return fmt.Errorf("profile: read installed %s: %w", name, err)
		}
		if err := w.WriteFile(file, fresh, 0o644); err != nil {
			return fmt.Errorf("profile: upgrade %s: %w", name, err)
		}
	}
	return nil
}
