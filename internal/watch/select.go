package watch

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"run8-ops-center/internal/run8save"
)

// Resolve finds the world save called name in saveDir. The match is
// case-insensitive and works with or without the .xml extension. It returns an
// error naming the problem and listing the world saves found when name is
// empty, matches no file, or matches a save that is not a world save. A missing
// or unreadable saveDir is an error naming the path; no other folder is tried.
func Resolve(saveDir, name string) (string, error) {
	entries, err := readXMLFiles(saveDir)
	if err != nil {
		return "", err
	}
	worlds := worldNames(saveDir, entries)

	want := strings.TrimSpace(name)
	if want == "" {
		return "", fmt.Errorf("no save named: pass --save with one of the world saves in %s: %s", saveDir, listOrNone(worlds))
	}
	if strings.EqualFold(filepath.Ext(want), ".xml") {
		want = want[:len(want)-len(".xml")]
	}

	match := ""
	for _, e := range entries {
		base := strings.TrimSuffix(e, filepath.Ext(e))
		if base == want {
			match = e
			break
		}
		if match == "" && strings.EqualFold(base, want) {
			match = e
		}
	}
	if match == "" {
		return "", fmt.Errorf("no save named %q in %s; world saves found: %s", name, saveDir, listOrNone(worlds))
	}
	path := filepath.Join(saveDir, match)
	if !isWorldSave(path) {
		return "", fmt.Errorf("%q is not a world save; world saves found in %s: %s", name, saveDir, listOrNone(worlds))
	}
	return path, nil
}

// ListWorldSaves returns the names (without .xml) of the world saves in
// saveDir, sorted case-insensitively. A save counts as a world save when its
// .info sidecar declares SaveType World, or, with no usable sidecar, when the
// XML's saveGameType is World.
func ListWorldSaves(saveDir string) ([]string, error) {
	entries, err := readXMLFiles(saveDir)
	if err != nil {
		return nil, err
	}
	return worldNames(saveDir, entries), nil
}

// readXMLFiles lists the regular .xml files directly inside dir.
func readXMLFiles(dir string) ([]string, error) {
	des, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("save folder %s: %w", dir, err)
	}
	var out []string
	for _, de := range des {
		if !de.Type().IsRegular() && de.Type()&os.ModeSymlink == 0 {
			continue
		}
		if de.IsDir() || !strings.EqualFold(filepath.Ext(de.Name()), ".xml") {
			continue
		}
		out = append(out, de.Name())
	}
	return out, nil
}

func worldNames(dir string, files []string) []string {
	var out []string
	for _, f := range files {
		if isWorldSave(filepath.Join(dir, f)) {
			out = append(out, strings.TrimSuffix(f, filepath.Ext(f)))
		}
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := strings.ToLower(out[i]), strings.ToLower(out[j])
		if a != b {
			return a < b
		}
		return out[i] < out[j]
	})
	return out
}

func listOrNone(names []string) string {
	if len(names) == 0 {
		return "(none)"
	}
	return strings.Join(names, ", ")
}

// isWorldSave reports whether the save at xmlPath is a world save. The
// sidecar decides when it decodes; otherwise the XML header is scanned.
func isWorldSave(xmlPath string) bool {
	if info, err := run8save.ReadInfo(run8save.InfoPath(xmlPath)); err == nil && info.SaveType != "" {
		return info.SaveType == run8save.SaveWorld
	}
	t, err := scanSaveGameType(xmlPath)
	return err == nil && t == run8save.SaveWorld
}

// scanSaveGameType token-scans the file for <saveGameType> without decoding
// the train list.
func scanSaveGameType(path string) (run8save.SaveType, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	dec := xml.NewDecoder(f)
	for {
		tok, err := dec.Token()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return "", fmt.Errorf("%s: no saveGameType element", path)
			}
			return "", fmt.Errorf("%s: %w", path, err)
		}
		se, ok := tok.(xml.StartElement)
		if !ok || se.Name.Local != "saveGameType" {
			continue
		}
		var v string
		if err := dec.DecodeElement(&v, &se); err != nil {
			return "", fmt.Errorf("%s: %w", path, err)
		}
		return run8save.SaveType(strings.TrimSpace(v)), nil
	}
}
