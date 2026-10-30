package run8save

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const autoSaveName = "Auto Save World 2026.10.28 17.28.02"

func mustParseFile(t *testing.T, name string) *Snapshot {
	t.Helper()
	snap, err := ParseFile(testdata + name)
	if err != nil {
		t.Fatalf("ParseFile(%s): %v", name, err)
	}
	return snap
}

func countVehicles(s *Snapshot) (vehicles, locos, cars int) {
	for _, tr := range s.Trains {
		for _, v := range tr.Vehicles {
			vehicles++
			if v.IsLoco() {
				locos++
			} else {
				cars++
			}
		}
	}
	return
}

func trainByID(t *testing.T, s *Snapshot, id string) Train {
	t.Helper()
	for _, tr := range s.Trains {
		if tr.ID == id {
			return tr
		}
	}
	t.Fatalf("train %s not found", id)
	return Train{}
}

// TestParseWorldSave covers task 2.2 on the reference world save.
func TestParseWorldSave(t *testing.T) {
	snap := mustParseFile(t, "WorldSave.xml")
	vehicles, locos, cars := countVehicles(snap)
	if len(snap.Trains) != 31 || vehicles != 498 || locos != 40 || cars != 458 {
		t.Errorf("trains=%d vehicles=%d locos=%d cars=%d, want 31/498/40/458",
			len(snap.Trains), vehicles, locos, cars)
	}
	if got := len(snap.Cars()); got != 458 {
		t.Errorf("Cars() = %d, want 458", got)
	}
	if got := len(snap.CarKeys()); got != 458 {
		t.Errorf("CarKeys() = %d, want 458 distinct keys", got)
	}
	want := time.Date(2026, 10, 28, 17, 22, 23, 743395700, time.UTC)
	if !snap.GameDate.Equal(want) || snap.GameDate.Location() != time.UTC {
		t.Errorf("GameDate = %v, want %v UTC", snap.GameDate, want)
	}
	if snap.SaveType != SaveWorld {
		t.Errorf("SaveType = %q, want World", snap.SaveType)
	}
	if snap.Description != "World Save-As" {
		t.Errorf("Description = %q", snap.Description)
	}
}

// TestParseAutoSaveNoDescription covers the second half of task 2.2.
func TestParseAutoSaveNoDescription(t *testing.T) {
	snap := mustParseFile(t, autoSaveName+".xml")
	vehicles, locos, cars := countVehicles(snap)
	if len(snap.Trains) != 31 || vehicles != 498 || locos != 40 || cars != 458 {
		t.Errorf("trains=%d vehicles=%d locos=%d cars=%d, want 31/498/40/458",
			len(snap.Trains), vehicles, locos, cars)
	}
	want := time.Date(2026, 10, 28, 17, 12, 47, 127564900, time.UTC)
	if !snap.GameDate.Equal(want) {
		t.Errorf("GameDate = %v, want %v", snap.GameDate, want)
	}
	if snap.Description != "" {
		t.Errorf("Description = %q, want empty", snap.Description)
	}
	if snap.SaveType != SaveWorld {
		t.Errorf("SaveType = %q, want World", snap.SaveType)
	}
}

func TestParseVehicleFields(t *testing.T) {
	snap := mustParseFile(t, "WorldSave.xml")

	// A single reefer, uncoupled, with its two trucks on different sections.
	cut := trainByID(t, snap, "999935")
	if len(cut.Vehicles) != 1 {
		t.Fatalf("train 999935 has %d vehicles, want 1", len(cut.Vehicles))
	}
	v := cut.Vehicles[0]
	if v.Model != "R8_Reefer_PCF_57_rbld_ARMN.xml" || v.UnitType != "US_Freightcar" ||
		v.UnitNumber != "194887" || v.Tag != "WC ." || v.LoadTons != 0 || v.Hazmat != 0 {
		t.Errorf("vehicle = %+v", v)
	}
	if v.Key() != "R8_Reefer_PCF_57_rbld_ARMN.xml#194887" {
		t.Errorf("Key() = %q", v.Key())
	}
	want := []Truck{
		{RoutePrefix: 280, Section: 4535, StartNode: 1, DistanceM: 0.5486534, Reverse: false},
		{RoutePrefix: 280, Section: 4537, StartNode: 1, DistanceM: 248.0489, Reverse: false},
	}
	if len(v.Trucks) != 2 || v.Trucks[0] != want[0] || v.Trucks[1] != want[1] {
		t.Errorf("Trucks = %+v, want %+v", v.Trucks, want)
	}

	// The lead locomotive of the first train: both trucks reversed.
	first := snap.Trains[0]
	if first.ID != "999933" || first.AI || first.DispatchDirection != 0 {
		t.Errorf("first train = %s ai=%v dir=%d", first.ID, first.AI, first.DispatchDirection)
	}
	lead := first.Vehicles[0]
	if lead.Model != "Run8_ES44DC_UP01.xml" || lead.UnitNumber != "5320" || lead.Tag != "344-16" || !lead.IsLoco() {
		t.Errorf("lead = %+v", lead)
	}
	if lead.LoadTons != 12.2231 {
		t.Errorf("lead LoadTons = %v", lead.LoadTons)
	}
	if len(lead.Trucks) != 2 || !lead.Trucks[0].Reverse || !lead.Trucks[1].Reverse ||
		lead.Trucks[0].Section != 8964 || lead.Trucks[1].DistanceM != 117.024544 {
		t.Errorf("lead trucks = %+v", lead.Trucks)
	}
	// Vehicle order within a train is preserved.
	if first.Vehicles[1].UnitNumber != "1119" {
		t.Errorf("second vehicle = %s, want 1119", first.Vehicles[1].UnitNumber)
	}
}

// TestPoweredTrainSymbolAndCuts covers task 2.3.
func TestPoweredTrainSymbolAndCuts(t *testing.T) {
	t.Run("29K fixture", func(t *testing.T) {
		snap := mustParseFile(t, "29K.xml")
		if len(snap.Trains) != 1 {
			t.Fatalf("trains = %d, want 1", len(snap.Trains))
		}
		tr := snap.Trains[0]
		if !tr.Powered() || tr.Symbol() != "29K" {
			t.Errorf("Powered=%v Symbol=%q, want true/29K", tr.Powered(), tr.Symbol())
		}
		if snap.SaveType != SaveTrain {
			t.Errorf("SaveType = %q, want Train", snap.SaveType)
		}
		if !strings.HasPrefix(snap.Description, "Intermodal from Inman Yard") {
			t.Errorf("Description = %.40q", snap.Description)
		}
		wantDate := time.Date(2023, 10, 16, 16, 21, 33, 211129200, time.UTC)
		if !snap.GameDate.Equal(wantDate) {
			t.Errorf("GameDate = %v, want %v", snap.GameDate, wantDate)
		}
		vehicles, locos, cars := countVehicles(snap)
		if vehicles != 90 || locos != 2 || cars != 88 {
			t.Errorf("vehicles=%d locos=%d cars=%d, want 90/2/88", vehicles, locos, cars)
		}
	})

	t.Run("WorldSave trains", func(t *testing.T) {
		snap := mustParseFile(t, "WorldSave.xml")
		powered, cuts := 0, 0
		for _, tr := range snap.Trains {
			hasLoco := false
			for _, v := range tr.Vehicles {
				hasLoco = hasLoco || v.IsLoco()
			}
			if tr.Powered() != hasLoco {
				t.Errorf("train %s: Powered() = %v but hasLoco = %v", tr.ID, tr.Powered(), hasLoco)
			}
			if tr.Powered() {
				powered++
				continue
			}
			cuts++
			if tr.Symbol() != "" {
				t.Errorf("cut %s has symbol %q", tr.ID, tr.Symbol())
			}
		}
		if powered != 19 || cuts != 12 {
			t.Errorf("powered=%d cuts=%d, want 19/12", powered, cuts)
		}
		if got := trainByID(t, snap, "999941").Symbol(); got != "AB45" {
			t.Errorf("train 999941 symbol = %q, want AB45", got)
		}
		// Lead loco tag "None": powered, but no symbol.
		none := trainByID(t, snap, "999944")
		if !none.Powered() || none.Symbol() != "" {
			t.Errorf("train 999944: Powered=%v Symbol=%q, want true/empty", none.Powered(), none.Symbol())
		}
	})
}

func TestParseExtraBoardNoTrains(t *testing.T) {
	snap := mustParseFile(t, "Bessemer Yard.xml")
	if len(snap.Trains) != 0 {
		t.Errorf("trains = %d, want 0", len(snap.Trains))
	}
	if snap.SaveType != SaveExtraBoard || !strings.HasPrefix(snap.Description, "In addition to") {
		t.Errorf("SaveType=%q Description=%.20q", snap.SaveType, snap.Description)
	}
	if want := time.Date(2024, 4, 2, 13, 51, 31, 793323800, time.UTC); !snap.GameDate.Equal(want) {
		t.Errorf("GameDate = %v, want %v", snap.GameDate, want)
	}
}

// TestParseRejectsBadInput covers task 2.4: a bad document gives an error and
// never a partial snapshot.
func TestParseRejectsBadInput(t *testing.T) {
	world, err := os.ReadFile(testdata + "WorldSave.xml")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("truncated at midpoint", func(t *testing.T) {
		snap, err := Parse(bytes.NewReader(world[:len(world)/2]))
		if err == nil || snap != nil {
			t.Fatalf("Parse = (%v, %v), want (nil, error)", snap, err)
		}
	})

	t.Run("truncated at other points", func(t *testing.T) {
		marker := []byte("</trainList>")
		afterTrains := bytes.Index(world, marker) + len(marker)
		afterTrainLoader := bytes.Index(world, []byte("</TrainLoader>")) + len("</TrainLoader>")
		beforeClose := bytes.LastIndex(world, []byte("</ScnLoader>"))
		for name, n := range map[string]int{
			"one byte":            1,
			"after first train":   afterTrainLoader,
			"after train list":    afterTrains,
			"just before closing": beforeClose,
			"one byte short":      len(world) - 1,
		} {
			snap, err := Parse(bytes.NewReader(world[:n]))
			if err == nil || snap != nil {
				t.Errorf("%s (len %d): Parse = (%v, %v), want (nil, error)", name, n, snap != nil, err)
			}
		}
	})

	t.Run("complete document still parses", func(t *testing.T) {
		if snap, err := Parse(bytes.NewReader(world)); err != nil || snap == nil {
			t.Fatalf("Parse = (%v, %v)", snap, err)
		}
	})

	vehicle := func(body string) string {
		return `<ScnLoader><trainList><TrainLoader><trainID>1</trainID><unitLoaderList>` +
			`<RailVehicleStateClass>` + body + `</RailVehicleStateClass>` +
			`</unitLoaderList></TrainLoader></trainList></ScnLoader>`
	}
	cases := map[string]string{
		"empty":                "",
		"not xml":              "this is not xml",
		"wrong root":           `<ScnInfo><SaveType>World</SaveType></ScnInfo>`,
		"root only start":      `<ScnLoader>`,
		"mismatched tags":      `<ScnLoader><trainList></ScnLoader>`,
		"bad game date":        `<ScnLoader><date>not a date</date></ScnLoader>`,
		"bad float":            vehicle(`<loadWeightUSTons>heavy</loadWeightUSTons>`),
		"bad truck int":        vehicle(`<currentRoutePrefix><int>x</int><int>1</int></currentRoutePrefix>`),
		"bad truck float":      vehicle(`<distanceTravelledInMeters><float>1..2</float></distanceTravelledInMeters>`),
		"bad truck bool":       vehicle(`<reverseDirection><boolean>maybe</boolean></reverseDirection>`),
		"bad hazmat":           vehicle(`<hazmatPlacardIndex>1.5</hazmatPlacardIndex>`),
		"bad train bool":       `<ScnLoader><trainList><TrainLoader><TrainWasAI>perhaps</TrainWasAI></TrainLoader></trainList></ScnLoader>`,
		"bad direction":        `<ScnLoader><trainList><TrainLoader><DispatchTrainDirection>up</DispatchTrainDirection></TrainLoader></trainList></ScnLoader>`,
		"truck arrays differ":  vehicle(`<currentRoutePrefix><int>1</int><int>1</int></currentRoutePrefix><currentTrackSectionIndex><int>5</int></currentTrackSectionIndex>`),
		"error after sections": `<ScnLoader><signals><a></signals></ScnLoader>`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			snap, err := Parse(strings.NewReader(body))
			if err == nil || snap != nil {
				t.Errorf("Parse = (%v, %v), want (nil, error)", snap, err)
			}
		})
	}
}

func TestParseMinimalDocument(t *testing.T) {
	snap, err := Parse(strings.NewReader(`<?xml version="1.0"?>
<ScnLoader>
  <trainList/>
  <date>2024-04-02T13:51:31Z</date>
  <saveGameType>Train</saveGameType>
  <signals><Signal><x>1</x></Signal></signals>
  <unknownFutureElement a="b"><c/></unknownFutureElement>
</ScnLoader>`))
	if err != nil {
		t.Fatal(err)
	}
	if snap.SaveType != SaveTrain || len(snap.Trains) != 0 || snap.Description != "" ||
		!snap.GameDate.Equal(time.Date(2024, 4, 2, 13, 51, 31, 0, time.UTC)) {
		t.Errorf("snapshot = %+v", snap)
	}
}

func copyTo(t *testing.T, dir, src, dst string) string {
	t.Helper()
	b, err := os.ReadFile(testdata + src)
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, dst)
	if err := os.WriteFile(p, b, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestParseFileNameAndWriteTime(t *testing.T) {
	t.Run("sidecar timestamp wins", func(t *testing.T) {
		snap := mustParseFile(t, autoSaveName+".xml")
		if snap.FileName != autoSaveName {
			t.Errorf("FileName = %q, want %q", snap.FileName, autoSaveName)
		}
		want := time.Date(2026, 10, 28, 17, 28, 2, 396337000, time.UTC)
		if !snap.WriteTime.Equal(want) {
			t.Errorf("WriteTime = %v, want %v", snap.WriteTime, want)
		}
	})

	t.Run("no sidecar uses mtime", func(t *testing.T) {
		dir := t.TempDir()
		p := copyTo(t, dir, "Bessemer Yard.xml", "Bessemer Yard.xml")
		mtime := time.Date(2025, 3, 4, 5, 6, 7, 0, time.UTC)
		if err := os.Chtimes(p, mtime, mtime); err != nil {
			t.Fatal(err)
		}
		snap, err := ParseFile(p)
		if err != nil {
			t.Fatal(err)
		}
		if snap.FileName != "Bessemer Yard" || !snap.WriteTime.Equal(mtime) {
			t.Errorf("FileName=%q WriteTime=%v, want Bessemer Yard / %v", snap.FileName, snap.WriteTime, mtime)
		}
	})

	t.Run("sidecar without timestamp uses mtime", func(t *testing.T) {
		dir := t.TempDir()
		p := copyTo(t, dir, "Bessemer Yard.xml", "y.xml")
		if err := os.WriteFile(InfoPath(p), []byte(`<ScnInfo><SaveType>ExtraBoard</SaveType></ScnInfo>`), 0o644); err != nil {
			t.Fatal(err)
		}
		mtime := time.Date(2025, 3, 4, 5, 6, 7, 0, time.UTC)
		if err := os.Chtimes(p, mtime, mtime); err != nil {
			t.Fatal(err)
		}
		snap, err := ParseFile(p)
		if err != nil {
			t.Fatal(err)
		}
		if !snap.WriteTime.Equal(mtime) {
			t.Errorf("WriteTime = %v, want %v", snap.WriteTime, mtime)
		}
	})

	t.Run("malformed sidecar does not block the save", func(t *testing.T) {
		dir := t.TempDir()
		p := copyTo(t, dir, "Bessemer Yard.xml", "z.xml")
		if err := os.WriteFile(InfoPath(p), []byte(`<ScnInfo>`), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := ParseFile(p); err != nil {
			t.Errorf("ParseFile: %v", err)
		}
	})

	t.Run("missing file", func(t *testing.T) {
		if snap, err := ParseFile(filepath.Join(t.TempDir(), "nope.xml")); err == nil || snap != nil {
			t.Errorf("ParseFile = (%v, %v), want (nil, error)", snap, err)
		}
	})

	t.Run("truncated file", func(t *testing.T) {
		dir := t.TempDir()
		b, err := os.ReadFile(testdata + "WorldSave.xml")
		if err != nil {
			t.Fatal(err)
		}
		p := filepath.Join(dir, "cut.xml")
		if err := os.WriteFile(p, b[:len(b)/2], 0o644); err != nil {
			t.Fatal(err)
		}
		if snap, err := ParseFile(p); err == nil || snap != nil {
			t.Errorf("ParseFile = (%v, %v), want (nil, error)", snap, err)
		}
	})
}
