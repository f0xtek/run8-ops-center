package web

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"run8-ops-center/internal/run8route/run8routetest"
	"run8-ops-center/internal/trackmap"
)

// vwSecTUS717 is a lone section, 717 ft long, on the synthetic route. It stands for
// the track in the track-mapping scenario of a cut on a 717 ft run in Tuscaloosa.
const vwSecTUS717 = 900

// vwSpec717 is the synthetic install plus a lone 717 ft section.
func vwSpec717() run8routetest.InstallSpec {
	spec := run8routetest.Synthetic()
	const metres = 717 / 3.28084
	spec.Tracks = append(spec.Tracks, run8routetest.Place(vwSecTUS717, []int{}, false,
		3000, -100, 3000, -100-metres, run8routetest.TileW, run8routetest.TileH))
	return spec
}

// vwHintBackend shows one cut on the 717 ft section.
func vwHintBackend(t *testing.T) (*vwBackend, http.Handler) {
	t.Helper()
	b := vwNewBackend(t, trackmap.New())
	b.useInstall(t, vwSpec717(), vwCutSnapshot("T717", "AB45 BRL", vwSecTUS717))
	return b, vwServer(t, b)
}

// vwHintOptions reads the options of one datalist in a candidate card: name, label and kind, in order.
func vwHintOptions(t *testing.T, card, id string) [][3]string {
	t.Helper()
	list := vwBetween(t, card, `<datalist id="`+id+`">`, `</datalist>`)
	var out [][3]string
	re := regexp.MustCompile(`<option value="([^"]*)" label="([^"]*)" data-kind="([^"]*)">`)
	for _, m := range re.FindAllStringSubmatch(list, -1) {
		out = append(out, [3]string{m[1], m[2], m[3]})
	}
	if len(out) == 0 {
		t.Fatalf("datalist %s has no options:\n%s", id, list)
	}
	return out
}

func TestLearnHintsRankedWithBothLengths(t *testing.T) {
	b, h := vwHintBackend(t)
	body := vwGet(t, h, "/learn").Body.String()
	card := vwBetween(t, body, `id="train-T717"`, `class="learn-cand"`)

	if !strings.Contains(vwText(card), "Measured length of the track it stands on: 717 ft") {
		t.Errorf("the card does not show the measured length:\n%s", vwText(card))
	}
	var tus, nor bool
	for _, y := range b.base.Yards {
		if len(y.Tracks) == 0 {
			continue
		}
		if !strings.Contains(card, `<datalist id="hints-T717-`+y.Code+`">`) {
			t.Errorf("no datalist for listed yard %s", y.Code)
		}
		tus = tus || y.Code == "TUS"
		nor = nor || y.Code == "NOR"
	}
	if !tus || !nor {
		t.Fatal("fixture: the bundled profile lists no TUS or NOR tracks")
	}

	got := vwHintOptions(t, card, "hints-T717-TUS")
	want := [][3]string{
		{"TY08", "710 ft listed, 717 ft measured", "yard"},
		{"Pass", "752 ft listed, 717 ft measured (siding, ignored)", "siding"},
	}
	if len(got) != 12 || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("TUS hints = %v, want %v first, 12 in all", got, want)
	}
	// TY06 (543 ft) comes after both and TY01 (1,379 ft) last of those with lengths.
	var order []string
	for _, o := range got {
		order = append(order, o[0])
	}
	pos := func(n string) int {
		for i, x := range order {
			if x == n {
				return i
			}
		}
		return -1
	}
	if !(pos("TY06") > pos("Pass") && pos("TY01") > pos("TY06") && pos("TAGS") > pos("TY01")) {
		t.Errorf("order = %v", order)
	}

	// No yard is chosen yet, so no list is attached to the track name field.
	if strings.Contains(card, ` list="hints-T717-`) {
		t.Error("the track name field names a hint list before a yard is chosen")
	}
	if !strings.Contains(card, `data-hints="hints-T717"`) {
		t.Error("the label form does not carry data-hints for app.js")
	}
}

func TestLearnHintsDropUsedNames(t *testing.T) {
	b, h := vwHintBackend(t)
	b.tm.Set(280, run8routetest.SecRcv1Mid, trackmap.Label{Kind: trackmap.KindYard, Yard: "TUS", Track: "TY08"})
	b.tm.Set(280, run8routetest.SecRcv2North, trackmap.Label{Kind: trackmap.KindYard, Yard: "NOR", Track: "Outbound 3"})
	card := vwBetween(t, vwGet(t, h, "/learn").Body.String(), `id="train-T717"`, `class="learn-cand"`)

	tus := vwHintOptions(t, card, "hints-T717-TUS")
	if len(tus) != 11 || tus[0][0] != "Pass" {
		t.Errorf("TUS hints after labelling TY08 = %v", tus)
	}
	for _, o := range tus {
		if o[0] == "TY08" {
			t.Error("TY08 is still offered for TUS")
		}
	}
	// A name used in one yard is still offered in the others.
	norNames := map[string]bool{}
	for _, o := range vwHintOptions(t, card, "hints-T717-NOR") {
		norNames[o[0]] = true
	}
	if norNames["Outbound 3"] || !norNames["Outbound 4"] {
		t.Errorf("NOR hints: Outbound 3 offered %v, Outbound 4 offered %v", norNames["Outbound 3"], norNames["Outbound 4"])
	}
}

func TestLearnHintsWithoutRouteDataKeepProfileOrder(t *testing.T) {
	b := vwNewBackend(t, trackmap.New())
	h := vwServer(t, b)
	body := vwGet(t, h, "/learn").Body.String()
	card := vwBetween(t, body, `id="train-999958"`, `class="learn-cand"`)
	got := vwHintOptions(t, card, "hints-999958-TUS")
	if got[0][0] != "TY01" || got[0][1] != "1,379 ft listed" || got[1][0] != "TY02" {
		t.Errorf("TUS hints = %v", got[:2])
	}
	if strings.Contains(vwText(card), "Measured length") || strings.Contains(card, "ft measured") {
		t.Errorf("a measured length without route data:\n%s", vwText(card))
	}
}

func TestLearnPostingASidingStoresAnIgnoredLabel(t *testing.T) {
	b, h := vwHintBackend(t)
	rec := vwPost(t, h, "/trackmap/label", url.Values{
		"train": {"T717"}, "kind": {"yard"}, "yard": {"TUS"}, "track": {" pass "},
	}, "X-Fragment", "1")
	if rec.Code != http.StatusOK {
		t.Fatalf("POST = %d: %s", rec.Code, rec.Body.String())
	}
	if l, ok := b.tm.Lookup(280, vwSecTUS717); !ok || l.Kind != trackmap.KindIgnored || l.Yard != "" || l.Track != "" {
		t.Errorf("label = %+v, %v; want ignored", l, ok)
	}
	if got := b.Calls(); len(got) != 1 || !strings.HasPrefix(got[0], "label-candidate T717 ") {
		t.Errorf("calls = %v", got)
	}
	if toast, _ := url.PathUnescape(rec.Header().Get("X-Toast")); !strings.Contains(toast, "ignored") || !strings.Contains(toast, "Pass is a siding") {
		t.Errorf("toast = %q", toast)
	}

	// A main line of another yard is ignored too, and per section as well.
	rec = vwPost(t, h, "/trackmap/label", url.Values{
		"section": {"280/100"}, "kind": {"yard"}, "yard": {"MCC"}, "track": {"Main"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("POST = %d: %s", rec.Code, rec.Body.String())
	}
	if l, _ := b.tm.Lookup(280, 100); l.Kind != trackmap.KindIgnored {
		t.Errorf("MCC / Main = %+v, want ignored", l)
	}
}

func TestLearnPostingAYardTrackOrFreeTextKeepsTheName(t *testing.T) {
	b, h := vwHintBackend(t)
	// A listed yard track is stored as typed.
	if rec := vwPost(t, h, "/trackmap/label", url.Values{
		"section": {"280/100"}, "kind": {"yard"}, "yard": {"TUS"}, "track": {"TY08"},
	}); rec.Code != http.StatusSeeOther {
		t.Fatalf("POST = %d: %s", rec.Code, rec.Body.String())
	}
	if l, _ := b.tm.Lookup(280, 100); l.Kind != trackmap.KindYard || l.Yard != "TUS" || l.Track != "TY08" {
		t.Errorf("TY08 = %+v", l)
	}
	// A name the list does not have is kept as free text.
	if rec := vwPost(t, h, "/trackmap/label", url.Values{
		"train": {"T717"}, "kind": {"yard"}, "yard": {"TUS"}, "track": {"Mystery  9"},
	}); rec.Code != http.StatusSeeOther {
		t.Fatalf("POST = %d: %s", rec.Code, rec.Body.String())
	}
	if l, ok := b.tm.Lookup(280, vwSecTUS717); !ok || l.Kind != trackmap.KindYard || l.Yard != "TUS" || l.Track != "Mystery 9" {
		t.Errorf("free text = %+v, %v", l, ok)
	}
	// The same name in a yard that does not list it as a siding is not ignored: Pass is TUS's.
	if rec := vwPost(t, h, "/trackmap/label", url.Values{
		"section": {"280/101"}, "kind": {"yard"}, "yard": {"NOR"}, "track": {"Pass"},
	}); rec.Code != http.StatusSeeOther {
		t.Fatalf("POST = %d: %s", rec.Code, rec.Body.String())
	}
	if l, _ := b.tm.Lookup(280, 101); l.Kind != trackmap.KindYard || l.Yard != "NOR" || l.Track != "Pass" {
		t.Errorf("NOR / Pass = %+v", l)
	}
	// A rename is not a label: it keeps the name it was given.
	if rec := vwPost(t, h, "/trackmap/rename", url.Values{
		"old_kind": {"yard"}, "old_yard": {"TUS"}, "old_track": {"Mystery 9"},
		"kind": {"yard"}, "yard": {"TUS"}, "track": {"Pass"},
	}); rec.Code != http.StatusSeeOther {
		t.Fatalf("rename = %d: %s", rec.Code, rec.Body.String())
	}
	if l, _ := b.tm.Lookup(280, vwSecTUS717); l.Kind != trackmap.KindYard || l.Track != "Pass" {
		t.Errorf("renamed = %+v", l)
	}
}

func TestLearnYardChoiceNeedsAppJS(t *testing.T) {
	// The script swaps the datalist and sets the kind; check the contract with the markup.
	js := vwGet(t, vwServerFor(t), "/static/app.js").Body.String()
	for _, want := range []string{"data-hints", "data-kind", `"ignored"`, "setAttribute(\"list\""} {
		if !strings.Contains(js, want) {
			t.Errorf("app.js lacks %q", want)
		}
	}
}

func vwServerFor(t *testing.T) http.Handler {
	t.Helper()
	return vwServer(t, vwNewBackend(t, trackmap.New()))
}
