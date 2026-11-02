package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"run8-ops-center/internal/profile"
	"run8-ops-center/internal/store"
)

// departureHarness is a harness whose cars all stand on NOR / R07, a receiving
// track the bundled profile does not mark as a departure track.
func departureHarness(t *testing.T) *harness {
	t.Helper()
	h := start(t)
	h.ingest(snapshotEvent(parse(t, worldPath), "WorldSave.xml", false))
	h.labelAllOn("R07")
	return h
}

func buildOf(v *View, sym string) (waitingFt float64, ready, found bool) {
	for _, b := range v.Building {
		if b.Symbol == sym {
			return b.WaitingFt, b.Ready, true
		}
	}
	return 0, false, false
}

func savedSettings(t *testing.T, h *harness) store.Settings {
	t.Helper()
	s, err := store.LoadSettings(h.dir)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestSetDepartureMarkingReceivingTrackCountsItsCars(t *testing.T) {
	h := departureHarness(t)
	v := h.eng.View()
	if ft, _, ok := buildOf(v, "108"); !ok || ft != 0 || findOrder(v.Available, "avail:108") != nil {
		t.Fatalf("108 with every car on R07: waiting %v found %v order %v", ft, ok, findOrder(v.Available, "avail:108"))
	}

	if err := h.eng.SetDeparture("NOR", "R07", true); err != nil {
		t.Fatal(err)
	}
	v = h.eng.View()
	if ft, ready, _ := buildOf(v, "108"); ft == 0 || !ready || findOrder(v.Available, "avail:108") == nil {
		t.Fatalf("108 after marking R07: waiting %v ready %v", ft, ready)
	}
	if !v.Profile.YardByCode("NOR").DepartureSet()["R07"] {
		t.Error("the view's profile does not list R07 as a departure track")
	}
	if got := v.Settings.Departure[DefaultProfileID]["NOR"]; len(got) != 1 || !got["R07"] {
		t.Errorf("view settings = %v", v.Settings.Departure)
	}
	if got := savedSettings(t, h).Departure[DefaultProfileID]["NOR"]; len(got) != 1 || !got["R07"] {
		t.Errorf("settings.json departure = %v", got)
	}
}

func TestSetDepartureBackToProfileDefaultRemovesTheEntry(t *testing.T) {
	h := departureHarness(t)
	if err := h.eng.SetDeparture("NOR", "R07", true); err != nil {
		t.Fatal(err)
	}
	if err := h.eng.SetDeparture("NOR", " r07 ", false); err != nil {
		t.Fatal(err)
	}
	if s := savedSettings(t, h); s.Departure != nil {
		t.Errorf("settings.json departure = %v, want none", s.Departure)
	}
	raw, err := os.ReadFile(filepath.Join(h.dir, store.SettingsFile))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "departure") {
		t.Errorf("settings.json still has a departure key:\n%s", raw)
	}
	if findOrder(h.eng.View().Available, "avail:108") != nil {
		t.Error("108 order stayed after R07 was unmarked")
	}

	// A listed default unmarked is stored, and marking it again removes it.
	if err := h.eng.SetDeparture("NOR", "wf03", false); err != nil {
		t.Fatal(err)
	}
	if got := savedSettings(t, h).Departure[DefaultProfileID]["NOR"]; len(got) != 1 || got["WF03"] {
		t.Errorf("unmarked WF03 stored as %v, want {WF03: false}", got)
	}
	if h.eng.View().Profile.YardByCode("NOR").DepartureSet()["WF03"] {
		t.Error("WF03 is still a departure track")
	}
	if err := h.eng.SetDeparture("NOR", "WF03", true); err != nil {
		t.Fatal(err)
	}
	if s := savedSettings(t, h); s.Departure != nil {
		t.Errorf("settings.json departure = %v, want none", s.Departure)
	}
}

func TestSetDepartureRejectsBadInput(t *testing.T) {
	h := start(t)
	for name, err := range map[string]error{
		"unknown yard":  h.eng.SetDeparture("ZZZ", "R07", true),
		"empty track":   h.eng.SetDeparture("NOR", "  ", true),
		"listed main":   h.eng.SetDeparture("NOR", "Old AGS Main", true),
		"listed siding": h.eng.SetDeparture("TUS", "pass", true),
	} {
		if err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
	if s := savedSettings(t, h); s.Departure != nil {
		t.Errorf("a rejected mark was saved: %v", s.Departure)
	}
}

func TestDepartureMarksSurviveRestart(t *testing.T) {
	h := departureHarness(t)
	if err := h.eng.SetDeparture("NOR", "R07", true); err != nil {
		t.Fatal(err)
	}
	h.open()
	v := h.eng.View()
	if !v.Profile.YardByCode("NOR").DepartureSet()["R07"] || v.Settings.Departure[DefaultProfileID]["NOR"]["R07"] != true {
		t.Errorf("after restart: set %v settings %v", v.Profile.YardByCode("NOR").DepartureSet(), v.Settings.Departure)
	}
}

func TestDepartureMarksSurviveAProfileUpgrade(t *testing.T) {
	h := departureHarness(t)
	if err := h.eng.SetDeparture("NOR", "R07", true); err != nil {
		t.Fatal(err)
	}
	h.close()

	path := filepath.Join(h.profileDir(), profile.ProfileFile)
	current := fmt.Sprintf("schemaVersion: %d\n", profile.BundledSchemaVersion)
	body := string(readFile(t, path))
	if !strings.Contains(body, current) {
		t.Fatalf("the bundled profile lacks %q", current)
	}
	v4 := strings.ReplaceAll(strings.Replace(body, current, "schemaVersion: 4\n", 1), ", departure: true", "")
	if err := os.WriteFile(path, []byte(v4), 0o644); err != nil {
		t.Fatal(err)
	}

	h.open()
	v := h.eng.View()
	if v.Profile.SchemaVersion != profile.BundledSchemaVersion {
		t.Fatalf("profile version %d, want the upgrade to %d", v.Profile.SchemaVersion, profile.BundledSchemaVersion)
	}
	if set := v.Profile.YardByCode("NOR").DepartureSet(); len(set) != 31 || !set["R07"] {
		t.Errorf("after the upgrade NOR departure set = %v, want the 30 defaults and R07", set)
	}
	if got := savedSettings(t, h).Departure[DefaultProfileID]["NOR"]; !got["R07"] {
		t.Errorf("settings.json lost the mark: %v", got)
	}
}

func TestDepartureMarksUnderAnotherProfileHaveNoEffect(t *testing.T) {
	h := departureHarness(t)
	h.close()
	s := savedSettings(t, h)
	s.Departure = map[string]map[string]map[string]bool{"other-route": {"NOR": {"R07": true, "WF01": false}}}
	if err := store.SaveSettings(h.guard, h.dir, s); err != nil {
		t.Fatal(err)
	}
	h.open()
	v := h.eng.View()
	if set := v.Profile.YardByCode("NOR").DepartureSet(); len(set) != 30 || set["R07"] || !set["WF01"] {
		t.Errorf("NOR departure set = %v, want the 30 profile defaults", set)
	}
	if v.Profile.YardByCode("NOR").Marks != nil {
		t.Errorf("marks = %v, want none", v.Profile.YardByCode("NOR").Marks)
	}
}

func TestDepartureMarkForAnUnknownYardIsIgnored(t *testing.T) {
	h := start(t)
	h.close()
	s := savedSettings(t, h)
	s.ProfileID = DefaultProfileID
	s.Departure = map[string]map[string]map[string]bool{DefaultProfileID: {"ZZZ": {"R07": true}}}
	if err := store.SaveSettings(h.guard, h.dir, s); err != nil {
		t.Fatal(err)
	}
	h.open()
	for _, y := range h.eng.View().Profile.Yards {
		if y.Marks != nil {
			t.Errorf("yard %s marks = %v", y.Code, y.Marks)
		}
	}
}
