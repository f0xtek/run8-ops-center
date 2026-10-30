package web

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"run8-ops-center/internal/engine"
	"run8-ops-center/internal/run8route/run8routetest"
	"run8-ops-center/internal/run8save"
	"run8-ops-center/internal/trackmap"
)

var importTokenRE = regexp.MustCompile(`name="token" value="([0-9a-f]+)"`)

// importBackend is a fake backend over the synthetic install, with an empty track
// map and a cut of BRL cars on the siding, and the server around it.
func importBackend(t *testing.T) (*vwBackend, http.Handler) {
	t.Helper()
	b := vwNewBackend(t, trackmap.New())
	b.useInstall(t, run8routetest.Synthetic(), vwCutSnapshot("T1", "AB45 BRL", run8routetest.SecSiding1, run8routetest.SecSiding1, run8routetest.SecSiding2))
	return b, vwServer(t, b)
}

// importToken posts the preview form and returns the confirm token on the page.
func importToken(t *testing.T, h http.Handler) string {
	t.Helper()
	rec := vwPost(t, h, "/trackmap/import/preview", url.Values{})
	if rec.Code != http.StatusOK {
		t.Fatalf("preview status %d: %s", rec.Code, vwText(rec.Body.String()))
	}
	m := importTokenRE.FindStringSubmatch(rec.Body.String())
	if m == nil {
		t.Fatalf("no confirm token on the preview page:\n%s", vwText(rec.Body.String()))
	}
	return m[1]
}

func TestLearnImportCardOffersPreview(t *testing.T) {
	b, h := importBackend(t)
	body := vwGet(t, h, "/learn").Body.String()
	card := vwBetween(t, body, `id="import"`, `</section>`)
	text := vwText(card)
	for _, want := range []string{"Import from Run8", "An import is available", "changes nothing until you confirm"} {
		if !strings.Contains(text, want) {
			t.Errorf("import card lacks %q:\n%s", want, text)
		}
	}
	if !strings.Contains(card, `action="/trackmap/import/preview"`) || strings.Contains(card, "disabled") {
		t.Errorf("preview form missing or disabled:\n%s", card)
	}
	if len(b.tm.Labels) != 0 {
		t.Error("opening learn mode labelled something")
	}

	// Without a Run8 block nothing is offered.
	plain := vwNewBackend(t, trackmap.New())
	text = vwText(vwBetween(t, vwGet(t, vwServer(t, plain), "/learn").Body.String(), `id="import"`, `</section>`))
	if !strings.Contains(text, "names no Run8 route folders") || strings.Contains(text, "Preview the import") {
		t.Errorf("card without route data:\n%s", text)
	}
}

func TestLearnImportCardStates(t *testing.T) {
	b, h := importBackend(t)
	tok := importToken(t, h)
	if rec := vwPost(t, h, "/trackmap/import/confirm", url.Values{"token": {tok}}); rec.Code != http.StatusSeeOther {
		t.Fatalf("confirm status %d", rec.Code)
	}
	text := vwText(vwBetween(t, vwGet(t, h, "/learn").Body.String(), `id="import"`, `</section>`))
	if !strings.Contains(text, "Last imported") || strings.Contains(text, "An import is available") || strings.Contains(text, "changed since the last import") {
		t.Errorf("card after an import:\n%s", text)
	}

	// Swap Config.ind: the card says the data changed.
	spec := run8routetest.Synthetic()
	spec.Industries = spec.Industries[:1]
	run8routetest.WriteInstall(t, b.imp.root, spec)
	text = vwText(vwBetween(t, vwGet(t, h, "/learn").Body.String(), `id="import"`, `</section>`))
	if !strings.Contains(text, "The Run8 track data changed since the last import") {
		t.Errorf("card after a file change:\n%s", text)
	}

	// A file that cannot be read is named with its path.
	spec = run8routetest.Synthetic()
	spec.Hump = nil
	b2 := vwNewBackend(t, trackmap.New())
	b2.useInstall(t, spec, nil)
	text = vwText(vwBetween(t, vwGet(t, vwServer(t, b2), "/learn").Body.String(), `id="import"`, `</section>`))
	if !strings.Contains(text, "Files that could not be used") || !strings.Contains(text, "HumpControllerList.r8") {
		t.Errorf("card with a missing file:\n%s", text)
	}
}

func TestImportPreviewShowsEveryCategory(t *testing.T) {
	b, h := importBackend(t)
	// A manual label on a section the import wants.
	b.tm.Set(run8routetest.Prefix, run8routetest.SecSiding2, trackmap.Label{Kind: trackmap.KindYard, Yard: "NOR", Track: "Track 9"})
	before := b.tm.Revision()

	rec := vwPost(t, h, "/trackmap/import/preview", url.Values{})
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, vwText(rec.Body.String()))
	}
	body := rec.Body.String()
	text := vwText(body)
	for _, want := range []string{
		"Import preview", "Nothing has been changed yet",
		"New labels", "Changed labels", "Removed labels", "Kept manual", "Skipped items",
		"NOR / Bowl 1", "NOR / Bowl 4", "NOR / Receiving 1", "NOR / Receiving 2", "BRL",
		"Kept manual labels", "280/106: stays NOR / Track 9, the import would say BRL",
		"Skipped items", // the heading
	} {
		if !strings.Contains(text, want) {
			t.Errorf("preview lacks %q:\n%s", want, text)
		}
	}
	// Industries the profile does not know, and receiving tracks that cannot be built, are
	// skipped with reasons.
	skipped := vwText(vwBetween(t, body, `id="import-skipped"`, `</section>`))
	for _, want := range []string{"ZZZ", "BBB", "Receiving 3", "Receiving 4"} {
		if !strings.Contains(skipped, want) {
			t.Errorf("skipped list lacks %q:\n%s", want, skipped)
		}
	}
	// Location groups are collapsible and use Groups().
	if n := strings.Count(body, `<details class="imp-group">`); n < 7 {
		t.Errorf("%d location groups, want at least the 4 bowl tracks, 2 receiving tracks and BRL", n)
	}
	// The confirm form carries the token and the cancel link goes back to learn mode.
	if !strings.Contains(body, `action="/trackmap/import/confirm"`) || importTokenRE.FindString(body) == "" {
		t.Error("confirm form or token missing")
	}
	if !strings.Contains(body, `<a class="btn" href="/learn#import">Cancel</a>`) {
		t.Error("cancel link missing")
	}
	if !strings.Contains(body, `aria-current="page">Learn mode`) {
		t.Error("the preview does not highlight the learn tab")
	}
	// Counts match the diff: BRL sections 105 and 106 are new but 106 is kept manual.
	var preview engine.ImportPreview
	preview, err := b.PreviewImport()
	if err != nil {
		t.Fatal(err)
	}
	if got := len(preview.Diff.KeptManual); got != 1 {
		t.Errorf("kept manual = %d, want 1", got)
	}
	// Preview changes nothing.
	if b.tm.Revision() != before || len(b.tm.Labels) != 1 {
		t.Errorf("preview changed the track map: %d labels", len(b.tm.Labels))
	}
}

func TestImportPreviewListsUnusableFiles(t *testing.T) {
	b := vwNewBackend(t, trackmap.New())
	spec := run8routetest.Synthetic()
	spec.Hump = nil
	b.useInstall(t, spec, nil)
	h := vwServer(t, b)
	rec := vwPost(t, h, "/trackmap/import/preview", url.Values{})
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, vwText(rec.Body.String()))
	}
	card := vwText(vwBetween(t, rec.Body.String(), `id="import-unusable"`, `</section>`))
	if !strings.Contains(card, "Files that could not be used") || !strings.Contains(card, "HumpControllerList.r8") {
		t.Errorf("unusable files:\n%s", card)
	}
	// The labels the rest of the data builds are still offered.
	if !strings.Contains(vwText(rec.Body.String()), "BRL") {
		t.Error("industry labels missing from a partial preview")
	}
}

func TestImportPreviewRefusals(t *testing.T) {
	b := vwNewBackend(t, trackmap.New())
	h := vwServer(t, b)
	rec := vwPost(t, h, "/trackmap/import/preview", url.Values{})
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(vwText(rec.Body.String()), "names no Run8 folders") {
		t.Errorf("no route data: %d %s", rec.Code, vwText(rec.Body.String()))
	}
	frag := vwPost(t, h, "/trackmap/import/preview", url.Values{}, "X-Fragment", "1")
	if frag.Code != http.StatusUnprocessableEntity || strings.Contains(frag.Body.String(), "<section") || !strings.Contains(frag.Body.String(), "names no Run8 folders") {
		t.Errorf("fragment refusal: %d %q", frag.Code, frag.Body.String())
	}
}

func TestImportConfirmApplies(t *testing.T) {
	b, h := importBackend(t)
	snap := b.snap
	if got := vwUnmappedCars(snap, b.tm); got != 3 {
		t.Fatalf("fixture: %d unmapped cars, want 3", got)
	}
	if n, _ := vwUnmappedFigure(t, vwGet(t, h, "/learn").Body.String()); n != 3 {
		t.Fatalf("learn page before: %d unmapped", n)
	}
	tok := importToken(t, h)

	rec := vwPost(t, h, "/trackmap/import/confirm", url.Values{"token": {tok}})
	if rec.Code != http.StatusSeeOther || vwNext(rec) != "/learn" {
		t.Fatalf("confirm: %d to %q: %s", rec.Code, rec.Header().Get("Location"), vwText(rec.Body.String()))
	}
	if l, _ := b.tm.Lookup(run8routetest.Prefix, run8routetest.SecSiding1); l.Source != trackmap.SourceRun8 || l.Industries[0] != "BRL" {
		t.Errorf("section 105 = %+v", l)
	}
	if b.tm.Run8Import == nil || !b.tm.Run8Import.At.Equal(vwImportTime) {
		t.Errorf("import metadata = %+v", b.tm.Run8Import)
	}
	body := vwGet(t, h, "/learn").Body.String()
	if n, _ := vwUnmappedFigure(t, body); n != 0 {
		t.Errorf("unmapped cars after the import = %d, want 0", n)
	}
	// The label list marks every imported section, and the manual ones stay unmarked.
	labels := vwBetween(t, body, `id="labels"`, `</section>`)
	imported := 0
	for _, l := range b.tm.Labels {
		if l.Source == trackmap.SourceRun8 {
			imported++
		}
	}
	if got := strings.Count(labels, `>imported</span>`); got != imported || imported == 0 {
		t.Errorf("%d imported pills, %d imported labels", got, imported)
	}
	b.tm.Set(run8routetest.Prefix, run8routetest.SecSiding1, trackmap.Label{Kind: trackmap.KindIndustry, Industries: []string{"BRL"}})
	labels = vwBetween(t, vwGet(t, h, "/learn").Body.String(), `id="labels"`, `</section>`)
	if got := strings.Count(labels, `>imported</span>`); got != imported-1 {
		t.Errorf("after a manual edit: %d pills, want %d", got, imported-1)
	}
}

func TestImportConfirmFragment(t *testing.T) {
	_, h := importBackend(t)
	tok := importToken(t, h)
	rec := vwPost(t, h, "/trackmap/import/confirm", url.Values{"token": {tok}}, "X-Fragment", "1")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, vwText(rec.Body.String()))
	}
	toast, _ := url.PathUnescape(rec.Header().Get("X-Toast"))
	if !strings.Contains(toast, "Run8 import applied") || !strings.Contains(toast, "0 unmapped cars left") {
		t.Errorf("toast = %q", toast)
	}
	if rec.Header().Get("X-Navigate") != "/learn" {
		t.Errorf("X-Navigate = %q", rec.Header().Get("X-Navigate"))
	}
	if !strings.Contains(rec.Body.String(), `id="import"`) || strings.Contains(rec.Body.String(), "<html") {
		t.Errorf("fragment is not the learn page's main content:\n%s", rec.Body.String())
	}
}

func TestImportConfirmRefusesStaleToken(t *testing.T) {
	b, h := importBackend(t)
	tok := importToken(t, h)

	// Config.ind is replaced between the preview and the confirm.
	spec := run8routetest.Synthetic()
	spec.Industries = spec.Industries[:1]
	run8routetest.WriteInstall(t, b.imp.root, spec)

	rec := vwPost(t, h, "/trackmap/import/confirm", url.Values{"token": {tok}})
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status %d, want 422", rec.Code)
	}
	if text := vwText(rec.Body.String()); !strings.Contains(text, "review a new preview") || !strings.Contains(text, "nothing was imported") {
		t.Errorf("stale message missing:\n%s", text)
	}
	if !strings.Contains(rec.Body.String(), `id="import"`) {
		t.Error("the refusal does not lead back to the learn page's import card")
	}
	if len(b.tm.Labels) != 0 || b.tm.Run8Import != nil {
		t.Errorf("a refused import changed the track map: %d labels", len(b.tm.Labels))
	}
	frag := vwPost(t, h, "/trackmap/import/confirm", url.Values{"token": {tok}}, "X-Fragment", "1")
	if frag.Code != http.StatusUnprocessableEntity || !strings.Contains(frag.Body.String(), "review a new preview") || strings.Contains(frag.Body.String(), "<section") {
		t.Errorf("fragment refusal: %d %q", frag.Code, frag.Body.String())
	}
	if frag.Header().Get("X-Navigate") != "" || frag.Header().Get("X-Toast") != "" {
		t.Error("a refusal must not navigate or toast success")
	}

	// A fresh preview works.
	fresh := importToken(t, h)
	if rec := vwPost(t, h, "/trackmap/import/confirm", url.Values{"token": {fresh}}); rec.Code != http.StatusSeeOther {
		t.Errorf("confirm after a new preview: %d", rec.Code)
	}
}

func TestImportConfirmRefusesAfterLabelEdit(t *testing.T) {
	b, h := importBackend(t)
	tok := importToken(t, h)
	b.tm.Set(run8routetest.Prefix, run8routetest.SecMainEast, trackmap.Label{Kind: trackmap.KindIgnored})
	rec := vwPost(t, h, "/trackmap/import/confirm", url.Values{"token": {tok}})
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(vwText(rec.Body.String()), "review a new preview") {
		t.Errorf("confirm after a label edit: %d %s", rec.Code, vwText(rec.Body.String()))
	}
	if len(b.tm.Labels) != 1 {
		t.Errorf("a refused import changed the track map: %d labels", len(b.tm.Labels))
	}
	// A token seen twice is stale the second time: the first confirm moved the map.
	_, h2 := importBackend(t)
	tok = importToken(t, h2)
	vwPost(t, h2, "/trackmap/import/confirm", url.Values{"token": {tok}})
	if rec := vwPost(t, h2, "/trackmap/import/confirm", url.Values{"token": {tok}}); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("second confirm with the same token: %d", rec.Code)
	}
}

func TestImportConfirmNeedsToken(t *testing.T) {
	b, h := importBackend(t)
	rec := vwPost(t, h, "/trackmap/import/confirm", url.Values{})
	if rec.Code != http.StatusBadRequest || !strings.Contains(vwText(rec.Body.String()), "review a new preview") {
		t.Errorf("no token: %d %s", rec.Code, vwText(rec.Body.String()))
	}
	for _, c := range b.Calls() {
		if c == "import-confirm" {
			t.Error("the backend was asked to confirm without a token")
		}
	}
}

func TestImportCancelChangesNothing(t *testing.T) {
	b, h := importBackend(t)
	before := b.tm.Revision()
	body := vwPost(t, h, "/trackmap/import/preview", url.Values{}).Body.String()
	m := regexp.MustCompile(`<a class="btn" href="([^"]+)">Cancel</a>`).FindStringSubmatch(body)
	if m == nil {
		t.Fatal("no cancel link on the preview")
	}
	rec := vwGet(t, h, strings.SplitN(m[1], "#", 2)[0])
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `id="import"`) {
		t.Fatalf("cancel leads to %q: %d", m[1], rec.Code)
	}
	if b.tm.Revision() != before || len(b.tm.Labels) != 0 || b.tm.Run8Import != nil {
		t.Error("cancelling changed the track map")
	}
	for _, c := range b.Calls() {
		if c == "import-confirm" {
			t.Error("cancelling confirmed the import")
		}
	}
	// The import is still on offer.
	if !strings.Contains(vwText(rec.Body.String()), "An import is available") {
		t.Error("the import is no longer offered after cancelling")
	}
}

func TestLearnShowsWholeTrackItWillLabel(t *testing.T) {
	b := vwNewBackend(t, trackmap.New())
	// A cut on 103 stands on the track {103, 104}.
	b.useInstall(t, run8routetest.Synthetic(), vwCutSnapshot("T1", "AB45 BRL", run8routetest.SecMainEast))
	h := vwServer(t, b)
	body := vwGet(t, h, "/learn").Body.String()
	card := vwBetween(t, body, `id="train-T1"`, `</section>`)
	text := vwText(card)
	if !strings.Contains(text, "also labels the other 1 unmapped section of the track it stands on, 2 in all") ||
		!strings.Contains(card, "<span>280/104</span>") {
		t.Errorf("whole track not shown:\n%s", text)
	}
	if !strings.Contains(card, `<option value="">All 2 sections of the track</option>`) {
		t.Error("the apply-to choice does not name the whole track")
	}
	// Labelling the cut labels the whole track.
	rec := vwPost(t, h, "/trackmap/label", url.Values{"train": {"T1"}, "kind": {"ignored"}})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("label: %d %s", rec.Code, vwText(rec.Body.String()))
	}
	for _, s := range []int{103, 104} {
		if _, ok := b.tm.Lookup(run8routetest.Prefix, s); !ok {
			t.Errorf("section %d not labelled", s)
		}
	}
}

// mixedCut is a cut whose cars carry different tags, so no tag suggestion applies.
func mixedCut(trainID string, section int) *run8save.Snapshot {
	snap := vwCutSnapshot(trainID, "AB45 BRL", section, section)
	snap.Trains[0].Vehicles[1].Tag = "AB10 BBB"
	return snap
}

func TestSuggestionNeedingTrackNamePrefillsForm(t *testing.T) {
	b := vwNewBackend(t, trackmap.New())
	// Lead 346 joins the receiving ladder; its neighbours are labelled by the import.
	b.useInstall(t, run8routetest.Synthetic(), mixedCut("T1", run8routetest.SecLadderS+5))
	h := vwServer(t, b)
	tok := importToken(t, h)
	if rec := vwPost(t, h, "/trackmap/import/confirm", url.Values{"token": {tok}}); rec.Code != http.StatusSeeOther {
		t.Fatalf("confirm: %d", rec.Code)
	}
	body := vwGet(t, h, "/learn").Body.String()
	card := vwBetween(t, body, `id="train-T1"`, `</section>`)
	text := vwText(card)
	if !strings.Contains(text, "yard NOR") || !strings.Contains(text, "track name") {
		t.Fatalf("no yard suggestion:\n%s", text)
	}
	if strings.Contains(card, "Accept this hint") {
		t.Error("a suggestion with no track name offers a one-click accept")
	}
	if !strings.Contains(card, "<details open>") {
		t.Error("the label form is not open")
	}
	if !strings.Contains(card, `<option value="NOR" selected>`) || !strings.Contains(card, `name="track" value=""`) {
		t.Errorf("form not pre-filled with yard NOR and a blank track:\n%s", card)
	}
	// Submitting without a track name is refused.
	rec := vwPost(t, h, "/trackmap/label", url.Values{"train": {"T1"}, "kind": {"yard"}, "yard": {"NOR"}, "track": {""}})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("blank track name: %d", rec.Code)
	}
	// A complete label from the form labels the cut's track.
	rec = vwPost(t, h, "/trackmap/label", url.Values{"train": {"T1"}, "kind": {"yard"}, "yard": {"NOR"}, "track": {"Lead 1"}})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("label: %d %s", rec.Code, vwText(rec.Body.String()))
	}
	if l, ok := b.tm.Lookup(run8routetest.Prefix, run8routetest.SecLadderS+5); !ok || l.Track != "Lead 1" || l.Source != "" {
		t.Errorf("label = %+v, %v", l, ok)
	}
}

func TestSuggestionWithTrackNameStaysOneClick(t *testing.T) {
	b := vwNewBackend(t, trackmap.New())
	b.useInstall(t, run8routetest.Synthetic(), vwCutSnapshot("T1", "AB45 BRL", run8routetest.SecSiding1))
	h := vwServer(t, b)
	card := vwBetween(t, vwGet(t, h, "/learn").Body.String(), `id="train-T1"`, `</section>`)
	if !strings.Contains(card, "Accept this hint") || strings.Contains(card, "<details open>") {
		t.Errorf("a complete suggestion lost its accept button:\n%s", vwText(card))
	}
}

func TestSetupInstallFolderHelp(t *testing.T) {
	h := vwServer(t, vwNewBackend(t, trackmap.New()))
	text := vwText(vwBetween(t, vwGet(t, h, "/setup").Body.String(), `id="install-dir"`, `</section>`))
	if !strings.Contains(text, "Used to read car files and Run8 track data.") {
		t.Errorf("install folder help:\n%s", text)
	}
}
