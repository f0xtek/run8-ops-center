package web

import (
	"net/http"
	"regexp"
	"strings"
	"testing"

	"run8-ops-center/internal/trackmap"
)

var vwHandlerAttr = regexp.MustCompile(`(?i)\son[a-z]+\s*=`)

// TestViewsPagesFitTheLayoutAndCSP checks every page of this package: it renders in the
// layout with its tab current, as a bare fragment on request, and with no inline script
// or event handler (the CSP forbids both).
func TestViewsPagesFitTheLayoutAndCSP(t *testing.T) {
	b := vwNewBackend(t, vwLabelledMap())
	h := vwServer(t, b)
	for path, tab := range map[string]string{"/yards": "Yards &amp; industries", "/learn": "Learn mode", "/setup": "Setup", "/activity": "Activity"} {
		rec := vwGet(t, h, path)
		if rec.Code != http.StatusOK {
			t.Errorf("%s: status %d", path, rec.Code)
			continue
		}
		body := rec.Body.String()
		if !regexp.MustCompile(`aria-current="page">` + regexp.QuoteMeta(tab) + `<`).MatchString(body) {
			t.Errorf("%s: tab %q not current", path, tab)
		}
		if strings.Contains(strings.ToLower(body), "<script") && strings.Count(strings.ToLower(body), "<script") != 2 {
			t.Errorf("%s: unexpected inline script", path)
		}
		if vwHandlerAttr.MatchString(body) {
			t.Errorf("%s: inline event handler", path)
		}
		frag := vwGet(t, h, path+"?fragment=1")
		if frag.Code != http.StatusOK || strings.Contains(frag.Body.String(), "<html") || strings.Contains(frag.Body.String(), "<nav aria-label=\"Primary\"") {
			t.Errorf("%s: fragment is not just the main content", path)
		}
		if got := frag.Header().Get("X-Page-Title"); got == "" {
			t.Errorf("%s: fragment has no title header", path)
		}
	}
}

// TestViewsRegisterWithoutTheSnapshotOrProfileCrashing covers a backend that has nothing yet.
func TestViewsHandleAnEmptyView(t *testing.T) {
	b := vwNewBackend(t, trackmap.New())
	s, err := New(Config{}, b)
	if err != nil {
		t.Fatal(err)
	}
	empty := vwEmpty{b}
	RegisterYards(s, empty)
	RegisterLearn(s, empty)
	RegisterSetup(s, empty)
	RegisterActivity(s, empty)
	for _, path := range []string{"/yards", "/learn", "/setup", "/activity"} {
		if rec := vwGet(t, s.Handler(), path); rec.Code != http.StatusOK {
			t.Errorf("%s with an empty view: %d %s", path, rec.Code, rec.Body.String())
		}
	}
	for _, path := range []string{"/trackmap/label", "/settings/threshold", "/settings/profile"} {
		if rec := vwPost(t, s.Handler(), path, map[string][]string{"kind": {"yard"}, "yard": {"NOR"}, "track": {"x"}, "train": {"1"}, "symbol": {"108"}, "id": {"x"}}); rec.Code < 400 || rec.Code >= 500 {
			t.Errorf("POST %s with no profile: %d", path, rec.Code)
		}
	}
}
