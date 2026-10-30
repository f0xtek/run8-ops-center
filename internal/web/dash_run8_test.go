package web

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"run8-ops-center/internal/engine"
	"run8-ops-center/internal/run8route"
	"run8-ops-center/internal/run8route/run8routetest"
)

// attentionItem returns the text of the attention item that mentions what, "" if none.
func attentionItem(att, what string) string {
	for _, part := range strings.Split(att, `<div class="ind">`) {
		if strings.Contains(part, what) {
			return part
		}
	}
	return ""
}

func TestDashAttentionImportAvailable(t *testing.T) {
	w := dashLoadWorld(t)
	sim := dashNewSim(w)
	sim.imp = vwNewImporter(t, w.Profile, run8routetest.Synthetic())
	h := dashNewServer(t, sim)

	body := dashReq(t, h, "GET", "/", nil).Body.String()
	att := dashSection(t, body, "attention")
	item := attentionItem(att, "Run8 track data can be imported")
	if item == "" || !strings.Contains(item, `href="/learn#import"`) || !strings.Contains(item, "Review the import") {
		t.Errorf("import-available item missing or unlinked:\n%s", att)
	}
	if strings.Contains(att, "changed since the last import") || strings.Contains(att, "could not be used") {
		t.Errorf("unexpected Run8 items:\n%s", att)
	}
}

func TestDashAttentionTrackDataChanged(t *testing.T) {
	w := dashLoadWorld(t)
	sim := dashNewSim(w)
	sim.imp = vwNewImporter(t, w.Profile, run8routetest.Synthetic())
	h := dashNewServer(t, sim)

	// Import, and the notice goes away.
	p, err := sim.PreviewImport()
	if err != nil {
		t.Fatal(err)
	}
	if err := sim.ConfirmImport(p.Token); err != nil {
		t.Fatal(err)
	}
	att := dashSection(t, dashReq(t, h, "GET", "/", nil).Body.String(), "attention")
	if strings.Contains(att, "Run8 track data") {
		t.Errorf("a notice remains after the import:\n%s", att)
	}

	// Swapping Config.ind brings it back with a link to the import.
	spec := run8routetest.Synthetic()
	spec.Industries = spec.Industries[:1]
	run8routetest.WriteInstall(t, sim.imp.root, spec)
	body := dashReq(t, h, "GET", "/", nil).Body.String()
	att = dashSection(t, body, "attention")
	item := attentionItem(att, "The Run8 track data changed since the last import")
	if item == "" || !strings.Contains(item, `href="/learn#import"`) || !strings.Contains(item, "Review the import") {
		t.Errorf("changed item missing or unlinked:\n%s", att)
	}
	if strings.Contains(att, "can be imported") {
		t.Error("both the changed and the available notices are shown")
	}
	items := strings.Count(att, `<div class="ind">`)
	if !strings.Contains(body, fmt.Sprintf(`Warnings <b>%d</b>`, items)) {
		t.Errorf("header badge does not count the notice (%d items)", items)
	}
}

func TestDashAttentionUnusableFiles(t *testing.T) {
	w := dashLoadWorld(t)
	sim := dashNewSim(w)
	spec := run8routetest.Synthetic()
	spec.Hump, spec.Mileposts = nil, nil
	sim.imp = vwNewImporter(t, w.Profile, spec)
	// The engine also warns about each file; the dashboard lists the file once.
	for _, f := range sim.imp.load().Files {
		if !f.OK {
			sim.base.Warnings = append(sim.base.Warnings, engine.Warning{
				Kind: "run8", Message: fmt.Sprintf("Run8 route file %s could not be used: %s", f.Path, dashFileReason(f)), Time: time.Now(),
			})
		}
	}
	if len(sim.base.Warnings) != 2 {
		t.Fatalf("fixture: %d unusable files, want 2", len(sim.base.Warnings))
	}
	h := dashNewServer(t, sim)
	att := dashSection(t, dashReq(t, h, "GET", "/", nil).Body.String(), "attention")

	for _, name := range []string{run8route.HumpFile, run8route.MilepostsFile} {
		item := attentionItem(att, name)
		if item == "" || !strings.Contains(item, "could not be used") || !strings.Contains(item, `href="/learn#import"`) {
			t.Errorf("item for %s missing or unlinked:\n%s", name, att)
		}
		if strings.Count(att, name) != 1 {
			t.Errorf("%s is listed %d times", name, strings.Count(att, name))
		}
	}
}

func TestDashNoRun8NoticeWithoutRouteData(t *testing.T) {
	w := dashLoadWorld(t)
	att := dashSection(t, dashReq(t, dashNewServer(t, dashNewSim(w)), "GET", "/", nil).Body.String(), "attention")
	if strings.Contains(att, "Run8 track data") || strings.Contains(att, "/learn#import") {
		t.Errorf("a profile with no Run8 data shows a Run8 item:\n%s", att)
	}
}
