package store

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// SettingsFile is the file name of the settings inside the data directory.
const SettingsFile = "settings.json"

// DefaultPort is the HTTP port used when settings.json does not set one.
const DefaultPort = 8080

// Threshold overrides the profile's limits for one train. A nil field means
// "use the profile value".
type Threshold struct {
	MinLengthPct  *float64 `json:"minLengthPct,omitempty"`
	MaxCarAgeDays *float64 `json:"maxCarAgeDays,omitempty"`
}

// Settings is the content of settings.json.
type Settings struct {
	ProfileID     string               `json:"profileId"`
	RunInstallDir string               `json:"run8InstallDir"`
	Port          int                  `json:"port"`
	AllInterfaces bool                 `json:"allInterfaces"`
	NoBrowser     bool                 `json:"noBrowser"`
	Thresholds    map[string]Threshold `json:"thresholds,omitempty"` // by train symbol
}

// DefaultSettings returns the settings used when no file exists.
func DefaultSettings() Settings { return Settings{Port: DefaultPort} }

// LoadSettings reads settings.json from dir. A missing file yields
// DefaultSettings. Fields absent from the file keep their defaults.
func LoadSettings(dir string) (Settings, error) {
	s := DefaultSettings()
	path := filepath.Join(dir, SettingsFile)
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return Settings{}, fmt.Errorf("read settings: %w", err)
	}
	if err := json.Unmarshal(data, &s); err != nil {
		return Settings{}, fmt.Errorf("parse %s: %w", path, err)
	}
	return s, nil
}

// SaveSettings writes settings.json into dir atomically through the guard.
func SaveSettings(g *Guard, dir string, s Settings) error {
	return saveJSON(g, filepath.Join(dir, SettingsFile), s)
}

// saveJSON marshals v as indented JSON and replaces path atomically.
func saveJSON(g *Guard, path string, v any) error {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return fmt.Errorf("encode %s: %w", filepath.Base(path), err)
	}
	return g.AtomicWriteFile(path, buf.Bytes(), 0o644)
}
