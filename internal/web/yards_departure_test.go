package web

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"run8-ops-center/internal/trackmap"
)

func TestYardsStateEachYardsDepartureRule(t *testing.T) {
	b := vwNewBackend(t, vwLabelledMap())
	body := vwGet(t, vwServer(t, b), "/yards").Body.String()

	nor := vwText(vwBetween(t, body, `id="yard-NOR"`, `id="yard-TUS"`))
	if !strings.Contains(nor, "Orders take cars from its 30 departure tracks only.") {
		t.Errorf("Norris does not state its rule:\n%.400s", nor)
	}
	tus := vwText(vwBetween(t, body, `id="yard-TUS"`, `id="yard-MCC"`))
	if !strings.Contains(tus, "Orders take cars from every yard track.") {
		t.Errorf("Tuscaloosa does not state its rule:\n%.400s", tus)
	}
}

func TestYardsRuleFollowsTheMarks(t *testing.T) {
	b := vwNewBackend(t, vwLabelledMap())
	h := vwServer(t, b)
	if err := b.SetDeparture("NOR", "R07", true); err != nil {
		t.Fatal(err)
	}
	nor := vwText(vwBetween(t, vwGet(t, h, "/yards").Body.String(), `id="yard-NOR"`, `id="yard-TUS"`))
	if !strings.Contains(nor, "Orders take cars from its 31 departure tracks only.") {
		t.Errorf("the rule did not count the marked track:\n%.400s", nor)
	}

	// Unmarking every default switches Norris to every yard track.
	if err := b.SetDeparture("NOR", "R07", false); err != nil {
		t.Fatal(err)
	}
	for _, y := range b.base.Yards {
		if y.Code != "NOR" {
			continue
		}
		for _, tr := range y.Tracks {
			if tr.Departure {
				if err := b.SetDeparture("NOR", tr.Name, false); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	nor = vwText(vwBetween(t, vwGet(t, h, "/yards").Body.String(), `id="yard-NOR"`, `id="yard-TUS"`))
	if !strings.Contains(nor, "Orders take cars from every yard track.") {
		t.Errorf("unmarking every departure track did not change the rule:\n%.400s", nor)
	}
}

func TestYardsPostingADepartureMarkCallsTheBackendAndRedirects(t *testing.T) {
	b := vwNewBackend(t, vwLabelledMap())
	h := vwServer(t, b)
	rec := vwPost(t, h, "/yards/departure", url.Values{"yard": {"NOR"}, "track": {" R07 "}, "on": {"true"}})
	if rec.Code != http.StatusSeeOther || !strings.HasPrefix(vwNext(rec), "/yards?") {
		t.Fatalf("POST = %d to %q", rec.Code, rec.Header().Get("Location"))
	}
	if got := b.Calls(); len(got) != 1 || got[0] != "departure NOR R07 true" {
		t.Fatalf("calls = %v", got)
	}
	page := vwText(vwGet(t, h, rec.Header().Get("Location")).Body.String())
	if !strings.Contains(page, "NOR / R07 is now a departure track. Available orders were recomputed.") {
		t.Errorf("no flash message after marking:\n%.600s", page)
	}

	rec = vwPost(t, h, "/yards/departure", url.Values{"yard": {"NOR"}, "track": {"WF01"}, "on": {"false"}})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("unmark POST = %d", rec.Code)
	}
	page = vwText(vwGet(t, h, rec.Header().Get("Location")).Body.String())
	if !strings.Contains(page, "NOR / WF01 is no longer a departure track.") {
		t.Errorf("no flash message after unmarking:\n%.600s", page)
	}
}

func TestYardsDepartureFragmentGetsThePageAndAToast(t *testing.T) {
	b := vwNewBackend(t, vwLabelledMap())
	h := vwServer(t, b)
	rec := vwPost(t, h, "/yards/departure", url.Values{"yard": {"NOR"}, "track": {"R07"}, "on": {"true"}}, "X-Fragment", "1")
	if rec.Code != http.StatusOK || !strings.Contains(vwText(rec.Body.String()), "31 departure tracks only") {
		t.Fatalf("fragment = %d:\n%.300s", rec.Code, vwText(rec.Body.String()))
	}
	toast, _ := url.PathUnescape(rec.Header().Get("X-Toast"))
	if !strings.Contains(toast, "NOR / R07 is now a departure track") {
		t.Errorf("toast = %q", toast)
	}
}

func TestYardsDepartureRejections(t *testing.T) {
	b := vwNewBackend(t, vwLabelledMap())
	h := vwServer(t, b)
	for name, tc := range map[string]struct {
		form url.Values
		want int
	}{
		"no yard":   {url.Values{"track": {"R07"}, "on": {"true"}}, http.StatusBadRequest},
		"no track":  {url.Values{"yard": {"NOR"}, "track": {" "}, "on": {"true"}}, http.StatusBadRequest},
		"bad state": {url.Values{"yard": {"NOR"}, "track": {"R07"}, "on": {"maybe"}}, http.StatusBadRequest},
		"no state":  {url.Values{"yard": {"NOR"}, "track": {"R07"}}, http.StatusBadRequest},
		"a siding":  {url.Values{"yard": {"TUS"}, "track": {"Pass"}, "on": {"true"}}, http.StatusUnprocessableEntity},
		"no yard X": {url.Values{"yard": {"ZZZ"}, "track": {"R07"}, "on": {"true"}}, http.StatusUnprocessableEntity},
	} {
		rec := vwPost(t, h, "/yards/departure", tc.form)
		if rec.Code != tc.want {
			t.Errorf("%s: status %d, want %d", name, rec.Code, tc.want)
		}
	}
	if got := b.Calls(); len(got) != 2 {
		t.Errorf("calls = %v, want only the two forwarded to the backend", got)
	}

	b.setErr = errors.New("disk full")
	rec := vwPost(t, h, "/yards/departure", url.Values{"yard": {"NOR"}, "track": {"R07"}, "on": {"true"}})
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(vwText(rec.Body.String()), "disk full") {
		t.Errorf("backend failure: %d %.200s", rec.Code, vwText(rec.Body.String()))
	}
}

func TestYardsSidingAndMainRowsHaveNoToggle(t *testing.T) {
	b := vwNewBackend(t, vwLabelledMap())
	body := vwGet(t, vwServer(t, b), "/yards").Body.String()
	for _, name := range []string{"Old AGS Main", "East Thoroughfare", "Pass"} {
		if strings.Contains(body, `name="track" value="`+name+`"`) {
			t.Errorf("%q has a departure toggle", name)
		}
	}
	for _, name := range []string{"WF01", "East Pullback"} {
		if !strings.Contains(body, `name="track" value="`+name+`"`) {
			t.Errorf("yard track %q has no departure toggle", name)
		}
	}
	if !strings.Contains(body, `action="/yards/departure"`) {
		t.Error("no toggle form posts to /yards/departure")
	}
}

func TestYardsMarkedReceivingTrackShowsAsChanged(t *testing.T) {
	b := vwNewBackend(t, vwLabelledMap())
	b.tm.Set(280, 99998, trackmap.Label{Kind: trackmap.KindYard, Yard: "NOR", Track: "R07"})
	h := vwServer(t, b)

	norText := func() string {
		return vwText(vwBetween(t, vwGet(t, h, "/yards").Body.String(), `id="yard-NOR"`, `id="yard-TUS"`))
	}
	if text := norText(); !strings.Contains(text, "R07 0 cars not a departure track Mark as departure track Empty.") {
		t.Fatalf("an unmarked receiving track:\n%s", text)
	}
	if err := b.SetDeparture("NOR", "R07", true); err != nil {
		t.Fatal(err)
	}
	if text := norText(); !strings.Contains(text, "R07 0 cars departure track changed from profile Unmark Empty.") {
		t.Errorf("a marked receiving track is not shown as changed:\n%s", text)
	}

	// A listed default that the user unmarked is changed too, and a default is not.
	if err := b.SetDeparture("NOR", "WF01", false); err != nil {
		t.Fatal(err)
	}
	listed := vwBetween(t, vwGet(t, h, "/yards").Body.String(), `class="sl yards-listed-table"`, `</table>`)
	row := func(name string) string {
		return vwText(vwBetween(t, listed, "<td>"+name+"</td>", "</tr>"))
	}
	if r := row("WF01"); !strings.Contains(r, "not a departure track") || !strings.Contains(r, "changed from profile") {
		t.Errorf("unmarked WF01 row = %q", r)
	}
	if r := row("WF02"); !strings.Contains(r, "departure track") || strings.Contains(r, "changed from profile") {
		t.Errorf("default WF02 row = %q", r)
	}
	if r := row("East Pullback"); !strings.Contains(r, "not a departure track") || strings.Contains(r, "changed from profile") {
		t.Errorf("default East Pullback row = %q", r)
	}

	// Marking it back to the profile default clears the tag.
	if err := b.SetDeparture("NOR", "WF01", true); err != nil {
		t.Fatal(err)
	}
	listed = vwBetween(t, vwGet(t, h, "/yards").Body.String(), `class="sl yards-listed-table"`, `</table>`)
	if r := row("WF01"); strings.Contains(r, "changed from profile") {
		t.Errorf("WF01 still changed after going back to the default: %q", r)
	}
}
