package engine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"run8-ops-center/internal/run8route/run8routetest"
	"run8-ops-center/internal/store"
	"run8-ops-center/internal/trackmap"
)

// TestRealPreviewPlacesWorldSaveCarsOnEastThoroughfare reads the git-ignored
// Run8 files in references/AutoLabel and skips without them. The 70 cars that
// WorldSave.xml has on RCV 1 show in the import preview under the receiving
// track's name from the bundled profile.
func TestRealPreviewPlacesWorldSaveCarsOnEastThoroughfare(t *testing.T) {
	root := t.TempDir()
	run8routetest.CopyLocal(t, root)
	h := &harness{t: t, dir: t.TempDir(), clock: newClock(), guard: store.NewGuard()}
	data, err := json.Marshal(map[string]string{"run8InstallDir": root})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(h.dir, store.SettingsFile), data, 0o644); err != nil {
		t.Fatal(err)
	}
	h.open()
	h.ingest(snapshotEvent(parse(t, worldPath), "WorldSave.xml", false))

	p, err := h.eng.PreviewImport()
	if err != nil {
		t.Fatal(err)
	}
	if !p.CarsKnown {
		t.Fatal("the preview does not know the cars")
	}
	east := trackmap.Location{Kind: trackmap.KindYard, Yard: "NOR", Track: "Receiving East Thoroughfare"}
	loc, ok := p.Locations[east.Key()]
	if !ok || loc.Cars != 70 || len(loc.CarNumbers) != maxImportCarNumbers || loc.LengthFt == 0 {
		t.Errorf("%s = %+v (present %v), want 70 cars, %d numbers and a length", east.Key(), loc, ok, maxImportCarNumbers)
	}
	for _, name := range []string{"R12", "R01", "Receiving West Thoroughfare"} {
		if _, ok := p.Locations[trackmap.Location{Kind: trackmap.KindYard, Yard: "NOR", Track: name}.Key()]; !ok {
			t.Errorf("the preview does not measure NOR / %s", name)
		}
	}
}
