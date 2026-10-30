package web

import (
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"run8-ops-center/internal/run8save"
	"run8-ops-center/internal/trackmap"
)

// vwTrainsOnUnmapped counts trains with a vehicle on an unlabelled section, from the raw snapshot.
func vwTrainsOnUnmapped(snap *run8save.Snapshot, tm *trackmap.Map) int {
	n := 0
	for _, tr := range snap.Trains {
		found := false
		for _, v := range tr.Vehicles {
			for _, k := range v.Trucks {
				if _, ok := tm.Lookup(k.RoutePrefix, k.Section); !ok {
					found = true
				}
			}
		}
		if found {
			n++
		}
	}
	return n
}

func vwUnmappedFigure(t *testing.T, body string) (n int, share string) {
	t.Helper()
	m := regexp.MustCompile(`(\d+) unmapped cars (\S+) of (\d+) cars`).FindStringSubmatch(vwText(body))
	if m == nil {
		t.Fatalf("no coverage indicator in:\n%s", vwText(body))
	}
	n, _ = strconv.Atoi(m[1])
	return n, m[2]
}

func TestLearnCoverageAndCandidates(t *testing.T) {
	b := vwNewBackend(t, trackmap.New())
	h := vwServer(t, b)
	body := vwGet(t, h, "/learn").Body.String()

	n, share := vwUnmappedFigure(t, body)
	if want := vwUnmappedCars(b.snap, b.tm); n != want || n != len(b.snap.Cars()) {
		t.Errorf("coverage says %d unmapped, want %d (all %d cars)", n, want, len(b.snap.Cars()))
	}
	if share != "100%" {
		t.Errorf("share = %q, want 100%%", share)
	}
	if got, want := strings.Count(body, `class="learn-cand"`), vwTrainsOnUnmapped(b.snap, b.tm); got != want {
		t.Errorf("%d candidates listed, snapshot has %d trains on unmapped sections", got, want)
	}
	// Cuts with cars come before locomotive-only trains, whatever the snapshot order.
	if cut, loco := strings.Index(body, `id="train-999935"`), strings.Index(body, `id="train-999933"`); cut < 0 || loco < 0 || cut > loco {
		t.Errorf("cut 999935 (at %d) should precede the locomotive-only train 999933 (at %d)", cut, loco)
	}
	// A cut shows its car count, sample numbers, tags and sections.
	card := vwText(vwBetween(t, body, `id="train-999958"`, `class="learn-cand"`))
	for _, want := range []string{"Cut", "7 cars", "2 sections", "AGR AGR ×7", "280/4557", "280/4558"} {
		if !strings.Contains(card, want) {
			t.Errorf("candidate 999958 lacks %q:\n%s", want, card)
		}
	}
	for _, c := range b.View().Candidates {
		if c.TrainID == "999958" {
			for _, unit := range c.Samples {
				if !strings.Contains(card, unit) {
					t.Errorf("candidate 999958 does not list sample car %s:\n%s", unit, card)
				}
			}
			if len(c.Samples) == 0 {
				t.Error("fixture: candidate has no samples")
			}
		}
	}
}

func TestLearnLabellingACutRemovesItAndUpdatesCoverage(t *testing.T) {
	b := vwNewBackend(t, trackmap.New())
	h := vwServer(t, b)
	before := vwGet(t, h, "/learn").Body.String()
	beforeN, _ := vwUnmappedFigure(t, before)
	beforeCands := strings.Count(before, `class="learn-cand"`)
	if !strings.Contains(before, `id="train-999950"`) {
		t.Fatal("fixture: train 999950 not listed")
	}

	rec := vwPost(t, h, "/trackmap/label", url.Values{"train": {"999950"}, "kind": {"industry"}, "industries": {"BRI"}})
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/learn" {
		t.Fatalf("POST = %d to %q, want 303 to /learn", rec.Code, rec.Header().Get("Location"))
	}
	if got := b.Calls(); len(got) != 1 || !strings.HasPrefix(got[0], "label-candidate 999950 ") {
		t.Fatalf("backend calls = %v", got)
	}

	after := vwGet(t, h, "/learn").Body.String()
	if strings.Contains(after, `id="train-999950"`) {
		t.Error("the labelled cut is still listed")
	}
	afterN, _ := vwUnmappedFigure(t, after)
	if want := vwUnmappedCars(b.snap, b.tm); afterN != want || afterN >= beforeN {
		t.Errorf("unmapped cars %d -> %d, independent count says %d", beforeN, afterN, want)
	}
	if got, want := strings.Count(after, `class="learn-cand"`), vwTrainsOnUnmapped(b.snap, b.tm); got != want || got >= beforeCands {
		t.Errorf("candidates %d -> %d, snapshot says %d", beforeCands, got, want)
	}
	labels := vwText(vwBetween(t, after, `id="labels"`, `</section>`))
	for _, sec := range []string{"280/11211", "280/11293", "280/11303", "280/11319"} {
		if !strings.Contains(labels, sec) {
			t.Errorf("labelled list lacks %s:\n%s", sec, labels)
		}
	}
	// The dashboard-facing count in the view moved too.
	if v := b.View(); v.Status.Unmapped != afterN {
		t.Errorf("status unmapped %d != page %d", v.Status.Unmapped, afterN)
	}
}

func TestLearnSuggestionIsAHintNeverApplied(t *testing.T) {
	b := vwNewBackend(t, trackmap.New())
	h := vwServer(t, b)
	body := vwGet(t, h, "/learn").Body.String()

	sug := b.View().Candidates
	var withHint string
	for _, c := range sug {
		if c.TrainID == "999950" {
			if c.Suggestion == nil {
				t.Fatal("fixture: 999950 has no suggestion")
			}
			withHint = c.Suggestion.Reason
		}
	}
	card := vwBetween(t, body, `id="train-999950"`, `class="learn-cand"`)
	text := vwText(card)
	for _, want := range []string{"Hint:", "industry track BRI", withHint, "not applied until you accept"} {
		if !strings.Contains(text, want) {
			t.Errorf("hint block lacks %q:\n%s", want, text)
		}
	}
	// A cut with mixed tags gets no hint.
	if mixed := vwText(vwBetween(t, body, `id="train-999957"`, `class="learn-cand"`)); strings.Contains(mixed, "Hint:") {
		t.Errorf("mixed-tag cut shows a hint:\n%s", mixed)
	}
	// Viewing changed nothing.
	if len(b.Calls()) != 0 || len(b.tm.Labels) != 0 {
		t.Fatalf("viewing the page applied something: calls %v, labels %v", b.Calls(), b.tm.Labels)
	}
	// Accepting is one POST that carries the hinted label.
	form := vwBetween(t, card, `<form method="post" action="/trackmap/label" data-async>`, `</form>`)
	for _, want := range []string{`name="train" value="999950"`, `name="kind" value="industry"`, `name="industries" value="BRI"`} {
		if !strings.Contains(form, want) {
			t.Errorf("accept form lacks %s:\n%s", want, form)
		}
	}
	vwPost(t, h, "/trackmap/label", url.Values{"train": {"999950"}, "kind": {"industry"}, "industries": {"BRI"}})
	if l, ok := b.tm.Lookup(280, 11211); !ok || l.Kind != trackmap.KindIndustry || l.Industries[0] != "BRI" {
		t.Errorf("accepted hint did not label the section: %v %v", l, ok)
	}
}

func TestLearnLabelFormOffersProfileYardsAndIndustries(t *testing.T) {
	b := vwNewBackend(t, trackmap.New())
	body := vwGet(t, vwServer(t, b), "/learn").Body.String()
	for _, y := range b.base.Yards {
		if !strings.Contains(body, `<option value="`+y.Code+`"`) {
			t.Errorf("yard select lacks %s", y.Code)
		}
	}
	list := vwBetween(t, body, `<datalist id="learn-industry-tags">`, `</datalist>`)
	for _, in := range b.base.Industries {
		if !strings.Contains(list, `<option value="`+in.Tag+`"`) {
			t.Errorf("industry tag list lacks %s", in.Tag)
		}
	}
	for _, kind := range []string{"yard", "industry", "ignored"} {
		if !strings.Contains(body, `<option value="`+kind+`"`) {
			t.Errorf("kind select lacks %s", kind)
		}
	}
}

func TestLearnLabelKindsAndValidation(t *testing.T) {
	b := vwNewBackend(t, trackmap.New())
	h := vwServer(t, b)

	// Valid yard, industry (lower case, several tags) and ignored labels reach the backend as typed labels.
	for _, form := range []url.Values{
		{"train": {"999958"}, "kind": {"yard"}, "yard": {"TUS"}, "track": {"  Lead   3 "}},
		{"train": {"999955"}, "kind": {"industry"}, "industries": {"lpt, ges"}},
		{"train": {"999951"}, "kind": {"ignored"}},
	} {
		if rec := vwPost(t, h, "/trackmap/label", form); rec.Code != http.StatusSeeOther {
			t.Fatalf("%v: status %d: %s", form, rec.Code, vwText(rec.Body.String()))
		}
	}
	if l, ok := b.tm.Lookup(280, 4557); !ok || l.Kind != trackmap.KindYard || l.Yard != "TUS" || l.Track != "Lead 3" {
		t.Errorf("yard label = %+v", l)
	}
	if l, ok := b.tm.Lookup(280, 4604); !ok || l.Kind != trackmap.KindIndustry || strings.Join(l.Industries, ",") != "LPT,GES" {
		t.Errorf("industry label = %+v", l)
	}
	if l, ok := b.tm.Lookup(280, 3892); !ok || l.Kind != trackmap.KindIgnored {
		t.Errorf("ignored label = %+v", l)
	}

	before := len(b.Calls())
	for name, form := range map[string]url.Values{
		"unknown yard":     {"train": {"999953"}, "kind": {"yard"}, "yard": {"ZZZ"}, "track": {"T1"}},
		"no track name":    {"train": {"999953"}, "kind": {"yard"}, "yard": {"NOR"}},
		"unknown industry": {"train": {"999953"}, "kind": {"industry"}, "industries": {"NOPE"}},
		"no industry":      {"train": {"999953"}, "kind": {"industry"}},
		"no kind":          {"train": {"999953"}},
		"bad kind":         {"train": {"999953"}, "kind": {"hump"}},
		"no target":        {"kind": {"ignored"}},
		"bad section":      {"section": {"abc"}, "kind": {"ignored"}},
	} {
		rec := vwPost(t, h, "/trackmap/label", form)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status %d, want 400", name, rec.Code)
		}
		if !strings.Contains(rec.Body.String(), `role="alert"`) {
			t.Errorf("%s: the page shows no error message", name)
		}
	}
	if got := len(b.Calls()); got != before {
		t.Errorf("rejected forms still reached the backend (%d new calls)", got-before)
	}
}

func TestLearnBackendErrorIsShown(t *testing.T) {
	b := vwNewBackend(t, trackmap.New())
	h := vwServer(t, b)
	rec := vwPost(t, h, "/trackmap/label", url.Values{"train": {"000000"}, "kind": {"ignored"}})
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status %d, want 422", rec.Code)
	}
	if !strings.Contains(vwText(rec.Body.String()), "train 000000 has no unmapped sections") {
		t.Errorf("backend message not shown:\n%s", vwText(rec.Body.String()))
	}
	// The fragment body is plain text in a <p>: the page script shows it as a toast, so quotes stay literal.
	if q := vwPost(t, h, "/trackmap/label", url.Values{"train": {"1"}, "kind": {"industry"}, "industries": {"<NOPE>"}}, "X-Fragment", "1"); q.Code != http.StatusBadRequest ||
		!strings.Contains(q.Body.String(), `"&lt;NOPE&gt;" is not`) {
		t.Errorf("fragment escaping: %d %q", q.Code, q.Body.String())
	}
	frag := vwPost(t, h, "/trackmap/label", url.Values{"train": {"000000"}, "kind": {"ignored"}}, "X-Fragment", "1")
	if frag.Code != http.StatusUnprocessableEntity || strings.Contains(frag.Body.String(), "<main") ||
		vwText(frag.Body.String()) != "train 000000 has no unmapped sections" {
		t.Errorf("fragment error = %d %q", frag.Code, frag.Body.String())
	}
}

func TestLearnLabelOneSectionOfACandidate(t *testing.T) {
	b := vwNewBackend(t, trackmap.New())
	h := vwServer(t, b)
	body := vwGet(t, h, "/learn").Body.String()
	card := vwBetween(t, body, `id="train-999958"`, `class="learn-cand"`)
	if !strings.Contains(card, `<option value="280/4558">`) {
		t.Fatalf("section chooser lacks 280/4558:\n%s", card)
	}
	rec := vwPost(t, h, "/trackmap/label", url.Values{"train": {"999958"}, "section": {"280/4558"}, "kind": {"yard"}, "yard": {"NOR"}, "track": {"Bowl 1"}})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status %d", rec.Code)
	}
	if _, ok := b.tm.Lookup(280, 4557); ok {
		t.Error("section 4557 was labelled although only 4558 was chosen")
	}
	if l, ok := b.tm.Lookup(280, 4558); !ok || l.Track != "Bowl 1" {
		t.Errorf("section 4558 label = %+v %v", l, ok)
	}
	// The cut is still listed, now with one unmapped section.
	card = vwText(vwBetween(t, vwGet(t, h, "/learn").Body.String(), `id="train-999958"`, `class="learn-cand"`))
	if !strings.Contains(card, "1 section") || !strings.Contains(card, "280/4557") {
		t.Errorf("card after a single-section label:\n%s", card)
	}
}

func TestLearnRenameRelabelAndRemove(t *testing.T) {
	b := vwNewBackend(t, vwLabelledMap())
	h := vwServer(t, b)

	body := vwGet(t, h, "/learn").Body.String()
	labels := vwBetween(t, body, `id="labels"`, `</section>`)
	for _, want := range []string{`/learn?relabel=280%2f3451`, `action="/trackmap/remove"`, `/learn?rename=`} {
		if !strings.Contains(labels, want) {
			t.Errorf("label list lacks %s", want)
		}
	}
	// Grouped by location: HTI's three sections sit under one heading.
	if !strings.Contains(vwText(vwBetween(t, labels, `HTI`, `Rename`)), "3 sections") {
		t.Errorf("HTI group does not show 3 sections:\n%s", vwText(labels))
	}

	// Relabel one section: the edit card is prefilled.
	edit := vwGet(t, h, "/learn?relabel=280/3451").Body.String()
	card := vwBetween(t, edit, `id="learn-edit"`, `</section>`)
	for _, want := range []string{`name="section" value="280/3451"`, `value="Bowl 14"`, `<option value="NOR" selected>`} {
		if !strings.Contains(card, want) {
			t.Errorf("relabel card lacks %s:\n%s", want, card)
		}
	}
	vwPost(t, h, "/trackmap/label", url.Values{"section": {"280/3451"}, "kind": {"yard"}, "yard": {"NOR"}, "track": {"Bowl 15"}})
	if l, _ := b.tm.Lookup(280, 3451); l.Track != "Bowl 15" {
		t.Errorf("relabel: %+v", l)
	}

	// Rename a location: every section moves.
	var key string
	for _, g := range b.View().Locations {
		if g.Location.Track == "Bowl 2" {
			key = g.Location.Key()
		}
	}
	redit := vwGet(t, h, "/learn?rename="+url.QueryEscape(key)).Body.String()
	rcard := vwBetween(t, redit, `id="learn-edit"`, `</section>`)
	for _, want := range []string{`action="/trackmap/rename"`, `name="old_track" value="Bowl 2"`, `name="old_yard" value="NOR"`} {
		if !strings.Contains(rcard, want) {
			t.Errorf("rename card lacks %s:\n%s", want, rcard)
		}
	}
	rec := vwPost(t, h, "/trackmap/rename", url.Values{
		"old_kind": {"yard"}, "old_yard": {"NOR"}, "old_track": {"Bowl 2"},
		"kind": {"yard"}, "yard": {"TUS"}, "track": {"Lead 1"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("rename status %d: %s", rec.Code, vwText(rec.Body.String()))
	}
	if l, _ := b.tm.Lookup(280, 3452); l.Yard != "TUS" || l.Track != "Lead 1" {
		t.Errorf("rename: %+v", l)
	}
	// Renaming a location that has no sections is the backend's refusal, shown to the user.
	rec = vwPost(t, h, "/trackmap/rename", url.Values{
		"old_kind": {"yard"}, "old_yard": {"NOR"}, "old_track": {"Nowhere"}, "kind": {"ignored"},
	})
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(vwText(rec.Body.String()), "no section is labelled") {
		t.Errorf("rename of a missing location: %d %s", rec.Code, vwText(rec.Body.String()))
	}
	// The ignored group cannot be renamed.
	if rec = vwPost(t, h, "/trackmap/rename", url.Values{"old_kind": {"ignored"}, "kind": {"ignored"}}); rec.Code != http.StatusBadRequest {
		t.Errorf("rename of ignored: %d", rec.Code)
	}

	// Remove makes the section unmapped again.
	if rec = vwPost(t, h, "/trackmap/remove", url.Values{"section": {"280/3451"}}); rec.Code != http.StatusSeeOther {
		t.Fatalf("remove status %d", rec.Code)
	}
	if _, ok := b.tm.Lookup(280, 3451); ok {
		t.Error("label still present after remove")
	}
	if rec = vwPost(t, h, "/trackmap/remove", url.Values{"section": {"x"}}); rec.Code != http.StatusBadRequest {
		t.Errorf("remove of a bad key: %d", rec.Code)
	}
}

func TestLearnFragmentPostRendersTheListDirectly(t *testing.T) {
	b := vwNewBackend(t, trackmap.New())
	h := vwServer(t, b)
	rec := vwPost(t, h, "/trackmap/label", url.Values{"train": {"999950"}, "kind": {"industry"}, "industries": {"BRI"}}, "X-Fragment", "1")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	body := rec.Body.String()
	if strings.Contains(body, "<html") || strings.Contains(body, `id="train-999950"`) || !strings.Contains(body, `id="candidates"`) {
		t.Errorf("fragment should be the refreshed main without the labelled cut:\n%s", vwText(body))
	}
	if got, _ := url.PathUnescape(rec.Header().Get("X-Toast")); !strings.Contains(got, "BRI") {
		t.Errorf("toast = %q", got)
	}
}
