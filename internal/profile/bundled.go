package profile

import (
	"embed"
	"fmt"
	"os"
	"path"
	"path/filepath"
)

// BundledID is the ID and folder name of the profile embedded in the binary.
const BundledID = "ags-south-p1"

//go:embed bundled/ags-south-p1/profile.yaml bundled/ags-south-p1/IndustryConfiguration.csv bundled/ags-south-p1/train_limits.tsv
var bundledFS embed.FS

// bundledFiles lists the bundled profile files in write order. profile.yaml
// comes last, so an interrupted install leaves no loadable half profile.
var bundledFiles = []string{IndustryFile, LimitsFile, ProfileFile}

// InstallBundled writes the bundled AGS South Phase 1 profile to
// <profilesDir>/ags-south-p1 through w. It does nothing when that folder
// already exists, so edits to an installed profile are never overwritten.
func InstallBundled(w FileWriter, profilesDir string) error {
	dst := filepath.Join(profilesDir, BundledID)
	if _, err := os.Stat(dst); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("profile: check %s: %w", dst, err)
	}
	if err := w.MkdirAll(dst, 0o755); err != nil {
		return fmt.Errorf("profile: create %s: %w", dst, err)
	}
	for _, name := range bundledFiles {
		data, err := bundledFS.ReadFile(path.Join("bundled", BundledID, name))
		if err != nil {
			return fmt.Errorf("profile: read bundled %s: %w", name, err)
		}
		if err := w.WriteFile(filepath.Join(dst, name), data, 0o644); err != nil {
			return fmt.Errorf("profile: write %s: %w", name, err)
		}
	}
	return nil
}
