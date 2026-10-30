package web

import (
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"run8-ops-center/internal/store"
	"run8-ops-center/internal/trackmap"
)

var vwInputRE = regexp.MustCompile(`<(?:input|select|textarea)\b[^>]*>`)

func TestSetupShowsSaveSourceReadOnly(t *testing.T) {
	b := vwNewBackend(t, trackmap.New())
	h := vwServer(t, b)
	body := vwGet(t, h, "/setup").Body.String()
	text := vwText(vwBetween(t, body, `id="source"`, `</section>`))

	for _, want := range []string{"ForRealThisTime", `C:\Run8\Trains`, `C:\Run8\AutoSaves`, "--save", "--save-dir", "--autosave-dir"} {
		if !strings.Contains(text, want) {
			t.Errorf("save source block lacks %q:\n%s", want, text)
		}
	}
	// No control carries the save name or either folder, and no form posts to a save setting.
	for _, in := range vwInputRE.FindAllString(body, -1) {
		for _, v := range []string{"ForRealThisTime", `C:\Run8\Trains`, `C:\Run8\AutoSaves`} {
			if strings.Contains(in, v) {
				t.Errorf("control holds the CLI value %q: %s", v, in)
			}
		}
	}
	if strings.Contains(vwBetween(t, body, `id="source"`, `</section>`), "<form") {
		t.Error("the save source block contains a form")
	}
	for _, m := range regexp.MustCompile(`action="([^"]+)"`).FindAllStringSubmatch(body, -1) {
		if strings.Contains(m[1], "save") {
			t.Errorf("a form posts to %s", m[1])
		}
	}
}

func TestSetupListsEveryRoadTrainWithItsLimits(t *testing.T) {
	b := vwNewBackend(t, trackmap.New())
	h := vwServer(t, b)
	body := vwGet(t, h, "/setup").Body.String()
	table := vwBetween(t, body, `id="threshold"`, `</section>`)
	if got, want := strings.Count(table, `name="min_length_pct"`), len(b.base.RoadTrains); got != want || want == 0 {
		t.Errorf("%d minimum-length inputs for %d road trains", got, want)
	}
	if got, want := strings.Count(table, `name="max_car_age_days"`), len(b.base.RoadTrains); got != want {
		t.Errorf("%d car-age inputs for %d road trains", got, want)
	}
	for _, rt := range b.base.RoadTrains {
		if !strings.Contains(table, `aria-label="`+rt.Symbol+` minimum length percent"`) {
			t.Errorf("no row for %s", rt.Symbol)
		}
	}
	if !strings.Contains(table, `aria-label="108 minimum length percent"`) || !regexp.MustCompile(`name="min_length_pct" value="50"[^>]*aria-label="108 `).MatchString(table) {
		t.Errorf("108 does not show its 50%% minimum")
	}
}

func TestSetupThresholdPostsTheCommand(t *testing.T) {
	b := vwNewBackend(t, trackmap.New())
	h := vwServer(t, b)

	rec := vwPost(t, h, "/settings/threshold", url.Values{"symbol": {"108"}, "min_length_pct": {"30"}, "max_car_age_days": {""}})
	if rec.Code != http.StatusSeeOther || !strings.HasPrefix(rec.Header().Get("Location"), "/setup") {
		t.Fatalf("POST = %d to %q", rec.Code, rec.Header().Get("Location"))
	}
	if len(b.thCalls) != 1 {
		t.Fatalf("threshold calls = %+v", b.thCalls)
	}
	c := b.thCalls[0]
	if c.Symbol != "108" || c.Min == nil || *c.Min != 30 || c.Age != nil {
		t.Errorf("command = %+v (min %v), want 108 with min 30 and no age", c, c.Min)
	}

	// The follow-up page shows the new effective value, marked as changed, and says it applied.
	page := vwGet(t, h, vwNext(rec)).Body.String()
	if !regexp.MustCompile(`name="min_length_pct" value="30"[^>]*aria-label="108 `).MatchString(page) {
		t.Error("108's minimum is not shown as 30 after saving")
	}
	if !strings.Contains(vwText(page), "Train limits saved. Available orders were recomputed.") {
		t.Errorf("no saved notice:\n%s", vwText(page))
	}

	// A max age alone; a value left at the profile's own is not stored as an override.
	vwPost(t, h, "/settings/threshold", url.Values{"symbol": {"131"}, "min_length_pct": {"50"}, "max_car_age_days": {"12.5"}})
	c = b.thCalls[1]
	if c.Symbol != "131" || c.Min != nil || c.Age == nil || *c.Age != 12.5 {
		t.Errorf("131 command = %+v", c)
	}
	// Clearing a field and the reset button clear overrides.
	vwPost(t, h, "/settings/threshold", url.Values{"symbol": {"108"}, "min_length_pct": {""}, "max_car_age_days": {""}})
	if c = b.thCalls[2]; c.Min != nil || c.Age != nil {
		t.Errorf("blank fields should clear the override: %+v", c)
	}
	vwPost(t, h, "/settings/threshold", url.Values{"symbol": {"131"}, "min_length_pct": {"50"}, "max_car_age_days": {"12.5"}, "reset": {"1"}})
	if c = b.thCalls[3]; c.Symbol != "131" || c.Min != nil || c.Age != nil {
		t.Errorf("reset = %+v", c)
	}
	if got := len(b.settings.Thresholds); got != 0 {
		t.Errorf("settings still hold %d overrides", got)
	}
}

func TestSetupThresholdValidation(t *testing.T) {
	b := vwNewBackend(t, trackmap.New())
	h := vwServer(t, b)
	for name, form := range map[string]url.Values{
		"over 100":       {"symbol": {"108"}, "min_length_pct": {"150"}},
		"negative":       {"symbol": {"108"}, "min_length_pct": {"-1"}},
		"not a number":   {"symbol": {"108"}, "min_length_pct": {"half"}},
		"negative age":   {"symbol": {"108"}, "max_car_age_days": {"-3"}},
		"unknown symbol": {"symbol": {"ZZZ"}, "min_length_pct": {"30"}},
		"no symbol":      {"min_length_pct": {"30"}},
	} {
		rec := vwPost(t, h, "/settings/threshold", form)
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), `role="alert"`) {
			t.Errorf("%s: status %d, alert shown %v", name, rec.Code, strings.Contains(rec.Body.String(), `role="alert"`))
		}
	}
	if len(b.thCalls) != 0 {
		t.Errorf("invalid forms reached the backend: %+v", b.thCalls)
	}
	// A backend refusal is shown, with status 422.
	b.setErr = http.ErrAbortHandler
	rec := vwPost(t, h, "/settings/threshold", url.Values{"symbol": {"108"}, "min_length_pct": {"30"}})
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(vwText(rec.Body.String()), http.ErrAbortHandler.Error()) {
		t.Errorf("backend error: %d %s", rec.Code, vwText(rec.Body.String()))
	}
}

func TestSetupInstallDirErrorIsShown(t *testing.T) {
	b := vwNewBackend(t, trackmap.New())
	h := vwServer(t, b)

	// A folder that contains the app's data directory: the guard refuses it.
	bad := filepath.Dir(b.dataDir)
	rec := vwPost(t, h, "/settings/install-dir", url.Values{"dir": {bad}})
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status %d, want 422", rec.Code)
	}
	text := vwText(rec.Body.String())
	if !strings.Contains(text, "The Run8 install folder was not changed") || !strings.Contains(text, "install folder") || !strings.Contains(text, "data") {
		t.Errorf("guard error not shown:\n%s", text)
	}
	if !strings.Contains(rec.Body.String(), `name="dir" value="`+bad+`"`) {
		t.Error("the rejected folder was not kept in the field")
	}
	if b.settings.RunInstallDir != "" {
		t.Errorf("a refused folder was stored: %q", b.settings.RunInstallDir)
	}
	frag := vwPost(t, h, "/settings/install-dir", url.Values{"dir": {bad}}, "X-Fragment", "1")
	if frag.Code != http.StatusUnprocessableEntity || !strings.Contains(vwText(frag.Body.String()), "data") || strings.Contains(frag.Body.String(), "<section") {
		t.Errorf("fragment error: %d %q", frag.Code, frag.Body.String())
	}

	// A folder elsewhere is accepted and shown.
	ok := filepath.Join(t.TempDir(), "Run8")
	rec = vwPost(t, h, "/settings/install-dir", url.Values{"dir": {ok}})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status %d: %s", rec.Code, vwText(rec.Body.String()))
	}
	page := vwGet(t, h, "/setup").Body.String()
	if !strings.Contains(page, `name="dir" value="`+ok+`"`) {
		t.Error("the saved folder is not shown")
	}
}

func TestSetupServerSettingsNeedARestart(t *testing.T) {
	b := vwNewBackend(t, trackmap.New())
	h := vwServer(t, b)
	page := vwGet(t, h, "/setup").Body.String()
	server := vwBetween(t, page, `id="server"`, `</section>`)
	if !strings.Contains(vwText(server), "Restart needed") || !strings.Contains(server, fmt.Sprintf(`name="port" value="%d"`, store.DefaultSettings().Port)) {
		t.Errorf("server block:\n%s", vwText(server))
	}
	if strings.Contains(server, "checked") {
		t.Error("all-interfaces is on by default")
	}

	rec := vwPost(t, h, "/settings/server", url.Values{"port": {"9000"}, "all_interfaces": {"1"}})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status %d", rec.Code)
	}
	if got := b.Calls(); len(got) != 1 || got[0] != "server 9000 true" {
		t.Fatalf("calls = %v", got)
	}
	after := vwGet(t, h, vwNext(rec)).Body.String()
	if !strings.Contains(vwText(after), "Restart the app for them to take effect") || !strings.Contains(after, `name="port" value="9000"`) || !strings.Contains(after, "checked") {
		t.Errorf("after saving:\n%s", vwText(vwBetween(t, after, `id="server"`, `</section>`)))
	}

	for _, port := range []string{"0", "70000", "abc", ""} {
		rec := vwPost(t, h, "/settings/server", url.Values{"port": {port}})
		if rec.Code != http.StatusBadRequest {
			t.Errorf("port %q: status %d", port, rec.Code)
		}
		if !strings.Contains(rec.Body.String(), `name="port" value="`+port+`"`) {
			t.Errorf("port %q: the typed value is not kept", port)
		}
	}
	if len(b.Calls()) != 1 {
		t.Errorf("invalid ports reached the backend: %v", b.Calls())
	}
}

func TestSetupProfileSelection(t *testing.T) {
	b := vwNewBackend(t, trackmap.New())
	h := vwServer(t, b)
	page := vwGet(t, h, "/setup").Body.String()
	sel := vwBetween(t, page, `id="profile"`, `</section>`)
	if !strings.Contains(sel, `<option value="ags-south-p1" selected>`) || !strings.Contains(sel, `<option value="other-route">`) {
		t.Errorf("profile select:\n%s", sel)
	}
	if rec := vwPost(t, h, "/settings/profile", url.Values{"id": {"other-route"}}); rec.Code != http.StatusSeeOther {
		t.Fatalf("status %d", rec.Code)
	}
	if got := b.Calls(); len(got) != 1 || got[0] != "profile other-route" {
		t.Errorf("calls = %v", got)
	}
	if rec := vwPost(t, h, "/settings/profile", url.Values{"id": {"nope"}}); rec.Code != http.StatusBadRequest {
		t.Errorf("unknown profile: %d", rec.Code)
	}
	if len(b.Calls()) != 1 {
		t.Errorf("an unknown profile reached the backend")
	}
}

func TestSetupShowsTheValidationReport(t *testing.T) {
	b := vwNewBackend(t, trackmap.New())
	h := vwServer(t, b)
	report := vwText(vwBetween(t, vwGet(t, h, "/setup").Body.String(), `id="profile-report"`, `</section>`))
	problems := b.base.Report.Problems
	if len(problems) == 0 {
		t.Fatal("fixture: the bundled profile has no problems to show")
	}
	for _, p := range problems {
		if !strings.Contains(report, p.Message) {
			t.Errorf("report lacks %q", p.Message)
		}
	}
	if !strings.Contains(report, "Outgoing tags that reach no train") {
		t.Errorf("problems are not grouped under titles:\n%s", report)
	}
}

func TestSetupFragmentPostRendersDirectly(t *testing.T) {
	b := vwNewBackend(t, trackmap.New())
	h := vwServer(t, b)
	rec := vwPost(t, h, "/settings/threshold", url.Values{"symbol": {"108"}, "min_length_pct": {"30"}}, "X-Fragment", "1")
	if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), "<html") || !strings.Contains(rec.Body.String(), `id="threshold"`) {
		t.Fatalf("fragment post: %d", rec.Code)
	}
	if got, _ := url.PathUnescape(rec.Header().Get("X-Toast")); !strings.Contains(got, "Train limits saved") {
		t.Errorf("toast = %q", got)
	}
}
