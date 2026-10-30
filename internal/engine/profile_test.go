package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"run8-ops-center/internal/orders"
	"run8-ops-center/internal/profile"
	"run8-ops-center/internal/store"
	"run8-ops-center/internal/trackmap"
)

const otherID = "ags-copy"

// installCopy writes a second profile, a copy of the bundled one with another
// id and name, through the guard.
func (h *harness) installCopy() {
	h.t.Helper()
	src := h.profileDir()
	dst := filepath.Join(h.dir, profilesDirName, otherID)
	if err := h.guard.MkdirAll(dst, 0o755); err != nil {
		h.t.Fatal(err)
	}
	for _, name := range []string{profile.IndustryFile, profile.LimitsFile, profile.ProfileFile} {
		data := readFile(h.t, filepath.Join(src, name))
		if name == profile.ProfileFile {
			text := strings.Replace(string(data), "id: "+DefaultProfileID, "id: "+otherID, 1)
			text = strings.Replace(text, "name: AGS South Phase 1 (NS Birmingham)", "name: AGS copy", 1)
			if !strings.Contains(text, "id: "+otherID) || !strings.Contains(text, "name: AGS copy") {
				h.t.Fatal("could not rewrite the profile id and name")
			}
			data = []byte(text)
		}
		if err := h.guard.WriteFile(filepath.Join(dst, name), data, 0o644); err != nil {
			h.t.Fatal(err)
		}
	}
}

// TestSwitchProfileHidesAndRestoresOrders is the task 8.4 check.
func TestSwitchProfileHidesAndRestoresOrders(t *testing.T) {
	h := start(t)
	h.installCopy()
	world := parse(t, worldPath)
	h.ingest(snapshotEvent(world, "WorldSave.xml", false))
	v := h.labelAllYard()
	labels := len(v.TrackMap.Labels)
	ord1, err := h.eng.StartOrder("avail:131", "")
	if err != nil {
		t.Fatal(err)
	}
	finished, err := h.eng.StartOrder("avail:ab10", "industry:BRR")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.eng.ApplyOrder(finished.ID, orders.ActCancel); err != nil {
		t.Fatal(err)
	}
	before := h.eng.View()
	if len(before.Active) != 1 || len(before.History) != 1 {
		t.Fatalf("setup: Active=%d History=%d", len(before.Active), len(before.History))
	}

	// Switch: the other profile has its own (empty) track map and no orders.
	if err := h.eng.SwitchProfile(otherID); err != nil {
		t.Fatal(err)
	}
	v = h.eng.View()
	if v.Profile.ID != otherID || v.Profile.Name != "AGS copy" || v.Settings.ProfileID != otherID {
		t.Fatalf("profile = %s %q, settings %q", v.Profile.ID, v.Profile.Name, v.Settings.ProfileID)
	}
	if len(v.Profiles) != 2 || v.Profiles[0].ID != otherID || v.Profiles[1].ID != DefaultProfileID {
		t.Errorf("Profiles = %+v", v.Profiles)
	}
	if len(v.Active) != 0 || len(v.History) != 0 || len(v.Available) != 0 {
		t.Errorf("orders of the other profile leaked: Active=%d History=%d Available=%d", len(v.Active), len(v.History), len(v.Available))
	}
	if len(v.TrackMap.Labels) != 0 || len(v.Locations) != 0 || v.Coverage.Unmapped != 458 || len(v.Candidates) == 0 {
		t.Errorf("track map is not per profile: labels=%d coverage=%+v candidates=%d", len(v.TrackMap.Labels), v.Coverage, len(v.Candidates))
	}
	if !v.HasSnapshot || v.Status == nil || v.Status.Unmapped != 458 || v.Tiles.Unmapped != 458 {
		t.Error("the last snapshot was not recomputed for the new profile")
	}
	if !v.Source.GameDate.Equal(world.GameDate) {
		t.Error("source lost its snapshot")
	}

	// The hidden orders are still in state.json, under their own profile.
	st, err := store.LoadState(h.dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := st.ProfileOrders[DefaultProfileID]; len(got) != 2 || got[0].ID != ord1.ID {
		t.Errorf("state.json lost the first profile's orders: %d", len(got))
	}
	if len(st.Dwell[DefaultProfileID]) != 458 || len(st.Dwell[otherID]) != 458 {
		t.Errorf("dwell by profile: %d, %d", len(st.Dwell[DefaultProfileID]), len(st.Dwell[otherID]))
	}

	// Work in the other profile: labels go to its own file, order ids continue.
	h.labelAllYard()
	if v = h.eng.View(); findOrder(v.Available, "avail:131") == nil {
		t.Fatal("the second profile has no 131 order: the first profile's reservation leaked")
	}
	other, err := h.eng.StartOrder("avail:131", "")
	if err != nil || other.ID != "ord-3" || other.Profile != otherID {
		t.Fatalf("order in the other profile: %q %q %v", other.ID, other.Profile, err)
	}
	if got, _ := trackmap.Load(filepath.Join(h.dir, profilesDirName, otherID, trackMapFile)); len(got.Labels) == 0 {
		t.Error("the other profile's track map was not saved in its folder")
	}
	if got, _ := trackmap.Load(filepath.Join(h.profileDir(), trackMapFile)); len(got.Labels) != labels {
		t.Errorf("the first profile's track map changed: %d labels, want %d", len(got.Labels), labels)
	}

	// Switch back: its orders and labels return.
	h.clock.Advance(time.Minute)
	if err := h.eng.SwitchProfile(DefaultProfileID); err != nil {
		t.Fatal(err)
	}
	v = h.eng.View()
	if v.Profile.ID != DefaultProfileID || len(v.Active) != 1 || v.Active[0].ID != ord1.ID || len(v.History) != 1 || v.History[0].ID != finished.ID {
		t.Fatalf("after switching back: profile %s, Active=%+v History=%d", v.Profile.ID, v.Active, len(v.History))
	}
	if len(v.TrackMap.Labels) != labels || v.Coverage.Unmapped != 0 || findOrder(v.Available, "avail:131") != nil {
		t.Errorf("after switching back: labels=%d coverage=%+v", len(v.TrackMap.Labels), v.Coverage)
	}

	// Unknown ids are refused and change nothing.
	seq := v.Seq
	if err := h.eng.SwitchProfile("no-such-profile"); err == nil {
		t.Error("switching to an unknown profile succeeded")
	}
	if v = h.eng.View(); v.Seq != seq || v.Profile.ID != DefaultProfileID || v.Settings.ProfileID != DefaultProfileID {
		t.Error("a failed switch changed the view")
	}

	// The choice survives a restart; the other profile's order is still there.
	if err := h.eng.SwitchProfile(otherID); err != nil {
		t.Fatal(err)
	}
	h.open()
	v = h.eng.View()
	if v.Profile.ID != otherID || len(v.Active) != 1 || v.Active[0].ID != other.ID {
		t.Errorf("after restart: profile %s Active=%+v", v.Profile.ID, v.Active)
	}
}

func TestSwitchToBrokenProfileKeepsCurrent(t *testing.T) {
	h := start(t)
	broken := filepath.Join(h.dir, profilesDirName, "broken")
	if err := h.guard.MkdirAll(broken, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := h.guard.WriteFile(filepath.Join(broken, profile.ProfileFile), []byte("id: [unterminated"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := h.eng.SwitchProfile("broken"); err == nil {
		t.Fatal("switching to a profile that does not load succeeded")
	}
	if v := h.eng.View(); v.Profile.ID != DefaultProfileID || len(v.Profiles) != 1 {
		t.Errorf("profile = %s, installed = %+v", v.Profile.ID, v.Profiles)
	}
	// A bad trackmap.json is an error, not a silent empty map.
	h.installCopy()
	tm := filepath.Join(h.dir, profilesDirName, otherID, trackMapFile)
	if err := h.guard.WriteFile(tm, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := h.eng.SwitchProfile(otherID); err == nil {
		t.Error("switching to a profile with a corrupt track map succeeded")
	}
	if h.eng.View().Profile.ID != DefaultProfileID {
		t.Error("the failed switch changed the profile")
	}
	if got, _ := os.ReadFile(tm); string(got) != "{not json" {
		t.Error("the corrupt track map was overwritten")
	}
	if st := h.eng.View().Settings.ProfileID; st != "" {
		t.Errorf("settings.profileId = %q after a failed switch", st)
	}
}

func TestSettingsProfileMissingFallsBack(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, store.SettingsFile), []byte(`{"profileId":"gone"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	h := &harness{t: t, dir: dir, clock: newClock(), guard: store.NewGuard()}
	h.open()
	v := h.eng.View()
	if v.Profile.ID != DefaultProfileID {
		t.Fatalf("profile = %s", v.Profile.ID)
	}
	found := false
	for _, w := range v.Warnings {
		if w.Kind == "profile" && strings.Contains(w.Message, `"gone"`) {
			found = true
		}
	}
	if !found {
		t.Errorf("no warning about the missing profile: %+v", v.Warnings)
	}
}
