package run8save

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

const testdata = "../../testdata/"

func TestInfoPath(t *testing.T) {
	cases := map[string]string{
		"x.xml":                       "x.info",
		filepath.Join("a", "b c.xml"): filepath.Join("a", "b c.info"),
		"UPPER.XML":                   "UPPER.info",
		"no-extension":                "no-extension.info",
		"dotted.name.xml":             "dotted.name.info",
	}
	for in, want := range cases {
		if got := InfoPath(in); got != want {
			t.Errorf("InfoPath(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestReadInfoSaveTypes covers task 2.1: the three fixture sidecars decode to
// World, Train and ExtraBoard.
func TestReadInfoSaveTypes(t *testing.T) {
	cases := []struct {
		file    string
		want    SaveType
		display string
		stamp   time.Time
	}{
		{"WorldSave.info", SaveWorld, "ForRealThisTime", time.Date(2026, 10, 28, 17, 37, 39, 27703100, time.UTC)},
		{"29K.info", SaveTrain, "29K", time.Date(2024, 12, 17, 1, 21, 48, 0, time.UTC)},
		{"Bessemer Yard.info", SaveExtraBoard, "Bessemer Yard", time.Date(2024, 12, 17, 2, 35, 10, 0, time.UTC)},
	}
	for _, c := range cases {
		t.Run(c.file, func(t *testing.T) {
			info, err := ReadInfo(testdata + c.file)
			if err != nil {
				t.Fatal(err)
			}
			if info.SaveType != c.want {
				t.Errorf("SaveType = %q, want %q", info.SaveType, c.want)
			}
			if info.DisplayFilename != c.display {
				t.Errorf("DisplayFilename = %q, want %q", info.DisplayFilename, c.display)
			}
			if info.Filename == "" {
				t.Error("Filename is empty")
			}
			if !info.FileTimestamp.Equal(c.stamp) {
				t.Errorf("FileTimestamp = %v, want %v", info.FileTimestamp, c.stamp)
			}
		})
	}
}

func TestReadInfoAutoSaveTimestamp(t *testing.T) {
	info, err := ReadInfo(testdata + "Auto Save World 2026.10.28 17.28.02.info")
	if err != nil {
		t.Fatal(err)
	}
	if info.SaveType != SaveWorld {
		t.Errorf("SaveType = %q, want World", info.SaveType)
	}
	want := time.Date(2026, 10, 28, 17, 28, 2, 396337000, time.UTC)
	if !info.FileTimestamp.Equal(want) {
		t.Errorf("FileTimestamp = %v, want %v", info.FileTimestamp, want)
	}
}

func writeTemp(t *testing.T, name, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestReadInfoMissingTimestamp(t *testing.T) {
	p := writeTemp(t, "x.info", `<?xml version="1.0"?>
<ScnInfo>
  <SaveType>World</SaveType>
  <Filename>C:\x\y.xml</Filename>
  <DisplayFilename>y</DisplayFilename>
</ScnInfo>`)
	info, err := ReadInfo(p)
	if err != nil {
		t.Fatal(err)
	}
	if !info.FileTimestamp.IsZero() {
		t.Errorf("FileTimestamp = %v, want zero", info.FileTimestamp)
	}
	if info.SaveType != SaveWorld || info.DisplayFilename != "y" {
		t.Errorf("got %+v", info)
	}
}

func TestReadInfoErrors(t *testing.T) {
	cases := map[string]string{
		"bad timestamp": `<ScnInfo><SaveType>World</SaveType><FileTimestamp>yesterday</FileTimestamp></ScnInfo>`,
		"wrong root":    `<ScnLoader><SaveType>World</SaveType></ScnLoader>`,
		"truncated":     `<ScnInfo><SaveType>World</SaveType>`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := ReadInfo(writeTemp(t, "x.info", body)); err == nil {
				t.Error("expected an error")
			}
		})
	}
	if _, err := ReadInfo(filepath.Join(t.TempDir(), "absent.info")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("missing file: err = %v, want not-exist", err)
	}
}
