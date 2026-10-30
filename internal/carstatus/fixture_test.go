package carstatus

import (
	"sort"
	"testing"

	"run8-ops-center/internal/run8save"
)

func TestDiffAutoSaveAgainstWorldSave(t *testing.T) {
	world, err := run8save.ParseFile("../../testdata/WorldSave.xml")
	if err != nil {
		t.Fatal(err)
	}
	auto, err := run8save.ParseFile("../../testdata/Auto Save World 2026.10.28 17.28.02.xml")
	if err != nil {
		t.Fatal(err)
	}
	got := Diff(world, auto)
	counts := map[ChangeKind]int{}
	trains := map[string]int{}
	for _, c := range got {
		counts[c.Kind]++
		if c.Kind == ChangeMoved {
			trains[c.TrainID]++
		}
	}
	if counts[ChangeMoved] != 300 || counts[ChangeAppeared] != 0 || counts[ChangeDisappeared] != 0 || counts[ChangeRetagged] != 0 {
		t.Errorf("counts = %v, want 300 moved and nothing else", counts)
	}
	var ids []string
	for id := range trains {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	want := []string{"999949", "999962", "999965", "999967"}
	if len(ids) != len(want) {
		t.Fatalf("moved trains = %v, want %v", trains, want)
	}
	for i := range want {
		if ids[i] != want[i] {
			t.Fatalf("moved trains = %v, want %v", trains, want)
		}
	}
}

// TestMarksInFixtures checks Mark against model names that occur in the real
// fixture, so the derivation is tied to names Run8 actually ships.
func TestMarksInFixtures(t *testing.T) {
	snap, err := run8save.ParseFile("../../testdata/WorldSave.xml")
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, v := range snap.Cars() {
		seen[v.Model] = true
	}
	want := map[string]string{
		"R8_Reefer_PCF_57_rbld_ARMN.xml":    "ARMN",
		"R8_Bulkhead_Thrall_TTJX01.xml":     "TTJX",
		"Run8_Tank107GATX02.xml":            "GATX",
		"Run8_WellCar53AP_FEC14.xml":        "FEC",
		"R8_AutoRack_Tri_NS01.xml":          "NS",
		"Run8_52ftwell_53_40_Schneider.xml": "",
	}
	for model, mark := range want {
		if !seen[model] {
			t.Errorf("fixture no longer contains %s", model)
		}
		if got := Mark(model); got != mark {
			t.Errorf("Mark(%q) = %q, want %q", model, got, mark)
		}
	}
}
