package trackmap

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"run8-ops-center/internal/run8route"
)

func sampleMeta() *ImportMeta {
	return &ImportMeta{
		At: time.Date(2026, 10, 30, 14, 0, 0, 0, time.UTC),
		Files: []run8route.FileFingerprint{
			{Path: `C:\Run8\Regions\GulfCoast\Config.ind`, Size: 41094, ModTime: time.Date(2026, 9, 1, 8, 30, 15, 123456789, time.UTC)},
			{Path: `C:\Run8\NS_AGS_Phase01\TrackDatabase.r8`, Size: 2274422, ModTime: time.Date(2026, 9, 2, 9, 0, 0, 0, time.UTC)},
		},
	}
}

func TestSourceAndImportMetaRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "trackmap.json")
	m := New()
	m.put("280/548", Label{Kind: KindIndustry, Industries: []string{"BRL"}, Source: SourceRun8})
	m.put("280/5101", Label{Kind: KindYard, Yard: "NOR", Track: "Bowl 1", Source: SourceRun8})
	m.Set(280, 8346, Label{Kind: KindYard, Yard: "NOR", Track: "Track 9"})
	m.Run8Import = sampleMeta()
	if err := m.Save(&osWriter{}, path); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Labels     map[string]map[string]any `json:"labels"`
		Run8Import map[string]any            `json:"run8Import"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Labels["280/548"]["source"] != "run8" {
		t.Errorf("imported label = %v", doc.Labels["280/548"])
	}
	if _, has := doc.Labels["280/8346"]["source"]; has {
		t.Errorf("manual label carries a source: %v", doc.Labels["280/8346"])
	}
	files, _ := doc.Run8Import["files"].([]any)
	if doc.Run8Import["at"] == nil || len(files) != 2 {
		t.Fatalf("run8Import = %v", doc.Run8Import)
	}
	for _, key := range []string{"path", "size", "modTime"} {
		if _, ok := files[0].(map[string]any)[key]; !ok {
			t.Errorf("stored file lacks %q: %v", key, files[0])
		}
	}

	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Labels, m.Labels) {
		t.Fatalf("labels:\n got %+v\nwant %+v", got.Labels, m.Labels)
	}
	if got.Run8Import == nil || !got.Run8Import.At.Equal(m.Run8Import.At) ||
		!run8route.SameFingerprint(got.Run8Import.Files, m.Run8Import.Files) {
		t.Fatalf("import meta = %+v, want %+v", got.Run8Import, m.Run8Import)
	}
	if got.Revision() != m.Revision() {
		t.Error("a reloaded map has another revision")
	}
}

func TestLoadWithoutSourcesIsManual(t *testing.T) {
	path := filepath.Join(t.TempDir(), "trackmap.json")
	old := `{"labels":{"280/1":{"kind":"industry","industries":["GES"]},"280/2":{"kind":"yard","yard":"NOR","track":"Bowl 1"},"280/3":{"kind":"ignored"}}}`
	if err := os.WriteFile(path, []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Labels) != 3 || m.Run8Import != nil {
		t.Fatalf("map = %+v", m)
	}
	for k, l := range m.Labels {
		if l.Source != "" {
			t.Errorf("%s loaded with source %q", k, l.Source)
		}
	}
	// A later import leaves all of them alone.
	plan := ImportPlan{Labels: map[string]Label{"280/1": {Kind: KindIndustry, Industries: []string{"BRL"}, Source: SourceRun8}}}
	d := m.Diff(plan)
	if len(d.KeptManual) != 1 || len(d.New)+len(d.Changed)+len(d.Removed) != 0 {
		t.Fatalf("diff = %+v", d)
	}
}

func TestLoadRejectsUnknownSource(t *testing.T) {
	path := filepath.Join(t.TempDir(), "trackmap.json")
	body := `{"labels":{"280/1":{"kind":"industry","industries":["GES"],"source":"guess"}}}`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(path)
	if err == nil || !strings.Contains(err.Error(), "source") || !strings.Contains(err.Error(), "guess") {
		t.Fatalf("error = %v", err)
	}
	// A broken import block is a load error as well.
	if err := os.WriteFile(path, []byte(`{"labels":{},"run8Import":{"at":"yesterday"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("bad import time accepted")
	}
}

// Every user-driven write makes a manual label, whatever it replaces.
func TestUserEditsMakeLabelsManual(t *testing.T) {
	imported := Label{Kind: KindYard, Yard: "NOR", Track: "Bowl 1", Source: SourceRun8}
	m := New()
	m.put("280/1", imported)
	m.put("280/2", imported)
	m.put("280/3", imported)
	m.put("280/4", imported)

	// Set ignores a source in its argument.
	m.Set(280, 1, Label{Kind: KindIndustry, Industries: []string{"COP"}, Source: SourceRun8})
	// Rename.
	m.Rename(Location{Kind: KindYard, Yard: "NOR", Track: "Bowl 1"}, Location{Kind: KindYard, Yard: "NOR", Track: "Bowl 9"})
	// Apply.
	m.put("280/5", Label{Kind: KindIgnored, Source: SourceRun8})
	m.Remove(280, 5)
	m.Apply(Candidate{Sections: []SectionRef{{280, 5}}}, Label{Kind: KindIgnored, Source: SourceRun8})

	for k, l := range m.Labels {
		if l.Source != "" {
			t.Errorf("%s kept source %q after a user edit: %+v", k, l.Source, l)
		}
	}
	if l := m.Labels["280/2"]; l.Track != "Bowl 9" {
		t.Errorf("rename lost: %+v", l)
	}
}

func TestRevisionAndClone(t *testing.T) {
	m := New()
	m.Set(280, 1, Label{Kind: KindIgnored})
	m.Set(280, 2, Label{Kind: KindIndustry, Industries: []string{"GES"}})
	base := m.Revision()
	if base == "" || base != m.Clone().Revision() || base != m.Revision() {
		t.Fatalf("revision unstable: %q", base)
	}
	var nilMap *Map
	if nilMap.Revision() != New().Revision() {
		t.Error("nil and empty maps differ")
	}

	c := m.Clone()
	c.Set(280, 2, Label{Kind: KindIndustry, Industries: []string{"BRL"}})
	if c.Revision() == base || m.Revision() != base {
		t.Error("clone shares state, or the revision ignores a label change")
	}
	if l, _ := m.Lookup(280, 2); l.Industries[0] != "GES" {
		t.Errorf("clone changed the original: %+v", l)
	}

	d := m.Clone()
	d.put("280/1", Label{Kind: KindIgnored, Source: SourceRun8})
	if d.Revision() == base {
		t.Error("the revision ignores a label source")
	}
	e := m.Clone()
	e.Run8Import = sampleMeta()
	if e.Revision() == base {
		t.Error("the revision ignores the import metadata")
	}
	f := e.Clone()
	f.Run8Import.Files[0].Size++
	if f.Revision() == e.Revision() || e.Run8Import.Files[0].Size != sampleMeta().Files[0].Size {
		t.Error("clone shares the file list, or the revision ignores a size")
	}
}
