package web

import (
	"bufio"
	"fmt"
	"html"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"run8-ops-center/internal/carstatus"
	"run8-ops-center/internal/engine"
	"run8-ops-center/internal/orders"
	"run8-ops-center/internal/profile"
)

// dashTileValue returns the number shown on the tile with the given label.
func dashTileValue(t *testing.T, body, label string) string {
	t.Helper()
	m := regexp.MustCompile(`<b>(\d+)</b><span>` + regexp.QuoteMeta(label) + `</span>`).FindStringSubmatch(body)
	if m == nil {
		t.Fatalf("no tile %q in page", label)
	}
	return m[1]
}

// dashSection returns the markup of the element with the given id up to the next
// <section, which is enough to scope an assertion to one card.
func dashSection(t *testing.T, body, id string) string {
	t.Helper()
	i := strings.Index(body, `id="`+id+`"`)
	if i < 0 {
		t.Fatalf("no element id=%q in page", id)
	}
	rest := body[i:]
	if j := strings.Index(rest[1:], "<section"); j >= 0 {
		rest = rest[:j+1]
	}
	return rest
}

// dashRows splits a card into order rows keyed by order href.
func dashRows(card string) map[string]string {
	out := map[string]string{}
	parts := strings.Split(card, `<div class="line">`)
	for _, p := range parts[1:] {
		m := regexp.MustCompile(`href="(/orders/[^"]+)"`).FindStringSubmatch(p)
		if m != nil {
			out[m[1]] = p
		}
	}
	return out
}

var dashActionRe = regexp.MustCompile(`<form[^>]*action="([^"]+)"`)

func dashActions(markup string) []string {
	var out []string
	for _, m := range dashActionRe.FindAllStringSubmatch(markup, -1) {
		out = append(out, html.UnescapeString(m[1]))
	}
	return out
}

func TestDashTilesMatchStatus(t *testing.T) {
	w := dashLoadWorld(t)
	h := dashNewServer(t, dashNewSim(w))
	rec := dashReq(t, h, "GET", "/", nil)
	if rec.Code != 200 {
		t.Fatalf("status %d", rec.Code)
	}
	body := rec.Body.String()

	// Expected counts come straight from the derived Status, one car at a time.
	var atInd, ready, delivery, train int
	for _, c := range w.Status.Cars {
		if c.Loc.Kind == carstatus.LocIndustry {
			atInd++
		}
		switch c.Service {
		case carstatus.ServiceReadyToPull:
			ready++
		case carstatus.ServiceAwaitingDelivery:
			delivery++
		case carstatus.ServiceAwaitingTrain:
			train++
		}
	}
	want := map[string]int{
		"Cars on route":        len(w.Status.Cars),
		"At industries":        atInd,
		"Ready to pull":        ready,
		"Awaiting delivery":    delivery,
		"Awaiting road trains": train,
		"Unmapped or unknown":  w.Status.Unmapped + w.Status.Unknown,
	}
	for label, n := range want {
		if n == 0 {
			t.Errorf("fixture has no %q cars, so the check proves nothing", label)
		}
		if got := dashTileValue(t, body, label); got != fmt.Sprint(n) {
			t.Errorf("tile %q = %s, want %d", label, got, n)
		}
	}
	// The reference save holds 458 cars (see carstatus tests).
	if want["Cars on route"] != 458 {
		t.Errorf("reference save has %d cars, want 458", want["Cars on route"])
	}
	if !strings.Contains(body, fmt.Sprintf("%d unmapped, %d unknown", w.Status.Unmapped, w.Status.Unknown)) {
		t.Errorf("unmapped/unknown breakdown missing")
	}
}

func TestDashEmptyState(t *testing.T) {
	sim := dashSimFrom(&dashWorld{Profile: &profile.Profile{}}, &engine.View{
		Source: engine.Source{SaveName: "ForRealThisTime", SaveDir: `C:\Run8\Trains`, AutoSaveDir: `C:\Run8\AutoSaves`},
		Warnings: []engine.Warning{
			{Kind: "parse", Message: "ForRealThisTime.xml is not a Run8 world save", Time: dashRealNow},
		},
	})
	rec := dashReq(t, dashNewServer(t, sim), "GET", "/", nil)
	if rec.Code != 200 {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"Waiting for the first save", "ForRealThisTime", `C:\Run8\Trains`, `C:\Run8\AutoSaves`,
		"is not a Run8 world save", `id="attention"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("empty dashboard lacks %q", want)
		}
	}
	for _, bad := range []string{`class="tiles"`, "Available orders", "Road train build"} {
		if strings.Contains(body, bad) {
			t.Errorf("empty dashboard should not show %q", bad)
		}
	}
	if !strings.Contains(body, `Warnings <b>1</b>`) {
		t.Errorf("header warning count should be 1")
	}
}

func TestDashAttention(t *testing.T) {
	w := dashLoadWorld(t)
	sim := dashNewSim(w)
	// The spec scenario: 12 cars on unmapped sections and an orphan tag in the profile.
	sim.base.Tiles.Unmapped = 12
	sim.base.Warnings = []engine.Warning{{Kind: "autosave", Message: "Auto Save World 2026.10.30 19.15.00.xml does not match WorldSave", Time: dashRealNow}}
	h := dashNewServer(t, sim)
	body := dashReq(t, h, "GET", "/", nil).Body.String()
	att := dashSection(t, body, "attention")

	if !strings.Contains(att, "12 cars are on track sections that have no label") || !strings.Contains(att, `href="/learn"`) {
		t.Errorf("unmapped item or learn link missing:\n%s", att)
	}
	if !strings.Contains(att, "does not match WorldSave") {
		t.Error("ingest warning missing")
	}
	orphan := 0
	for _, p := range w.Profile.Report.Problems {
		if p.Code == profile.CodeOrphanTag {
			orphan++
		}
		if !strings.Contains(att, html.EscapeString(p.Message)) {
			t.Errorf("profile problem missing: %s", p.Message)
		}
	}
	if orphan == 0 {
		t.Fatal("bundled profile no longer reports an orphan tag; the check proves nothing")
	}
	if !strings.Contains(att, `href="/setup#profile-report"`) {
		t.Error("link to the profile report missing")
	}

	// Cars with an unknown tag or type, counted from the Status.
	var tagCars, typeCars int
	for _, c := range w.Status.Cars {
		if c.Dest.Kind == carstatus.DestUnknown {
			tagCars++
		}
		if c.Type == profile.Unknown {
			typeCars++
		}
	}
	if tagCars+typeCars == 0 {
		t.Fatal("reference save has no unknown car; the check proves nothing")
	}
	if tagCars > 0 && !strings.Contains(att, "a destination tag the profile does not know") {
		t.Error("unknown-tag item missing")
	}
	if typeCars > 0 && !strings.Contains(att, "a model that matches no car type") {
		t.Error("unknown-type item missing")
	}

	// The header badge counts exactly the items in the list.
	items := strings.Count(att, `<div class="ind">`)
	if want := len(w.Profile.Report.Problems) + 1 + 1 + btoi(tagCars > 0) + btoi(typeCars > 0); items != want {
		t.Errorf("attention list has %d items, want %d", items, want)
	}
	if !strings.Contains(body, fmt.Sprintf(`Warnings <b>%d</b>`, items)) {
		t.Errorf("header badge does not show %d", items)
	}
}

func btoi(b bool) int {
	if b {
		return 1
	}
	return 0
}

func TestDashNothingToReport(t *testing.T) {
	w := dashLoadWorld(t)
	sim := dashNewSim(w)
	sim.base.Profile = &profile.Profile{Name: "clean"}
	sim.base.Status = &carstatus.Status{}
	sim.base.Tiles = engine.Tiles{}
	sim.base.Available, sim.base.Building = nil, nil
	sim.w = &dashWorld{Profile: sim.base.Profile, Status: sim.base.Status}
	body := dashReq(t, dashNewServer(t, sim), "GET", "/", nil).Body.String()
	if !strings.Contains(dashSection(t, body, "attention"), "Nothing needs attention.") {
		t.Error("clean dashboard should say nothing needs attention")
	}
	if !strings.Contains(body, `Warnings <b>0</b>`) {
		t.Error("badge should read 0")
	}
}

func TestDashOrderSections(t *testing.T) {
	w := dashLoadWorld(t)
	sim := dashNewSim(w)
	v := sim.View()
	dashOrder(t, v, "avail:ab10") // the job exists before it is started
	// Three started orders: one stays In Progress, one is detected, one is not touched.
	prog, err := sim.StartOrder("avail:ab10", "")
	if err != nil {
		t.Fatal(err)
	}
	det, err := sim.StartOrder("avail:a44", "")
	if err != nil {
		t.Fatal(err)
	}
	sim.markDetected(det.ID)
	h := dashNewServer(t, sim)
	body := dashReq(t, h, "GET", "/", nil).Body.String()

	active := dashRows(dashSection(t, body, "orders-active"))
	if len(active) != 2 {
		t.Fatalf("active rows = %d, want 2: %v", len(active), active)
	}
	if got := dashActions(active[ordHref(prog.ID)]); len(got) != 2 ||
		got[0] != ordHref(prog.ID)+"/complete" || got[1] != ordHref(prog.ID)+"/cancel" {
		t.Errorf("in-progress buttons = %v", got)
	}
	if got := dashActions(active[ordHref(det.ID)]); len(got) != 2 ||
		got[0] != ordHref(det.ID)+"/confirm" || got[1] != ordHref(det.ID)+"/continue" {
		t.Errorf("completion-detected buttons = %v", got)
	}
	if !strings.Contains(active[ordHref(det.ID)], "Completion detected") || !strings.Contains(active[ordHref(prog.ID)], "In progress") {
		t.Error("state chips missing")
	}
	if !strings.Contains(active[ordHref(prog.ID)], "cars done") {
		t.Error("progress for the started order missing")
	}
	// Detected orders come first: they wait on the user.
	card := dashSection(t, body, "orders-active")
	if strings.Index(card, "Completion detected</h3>") > strings.Index(card, "In progress</h3>") {
		t.Error("completion-detected group should come before in-progress")
	}

	avail := dashSection(t, body, "orders-available")
	rows := dashRows(avail)
	cur := sim.View()
	if len(rows) != len(cur.Available) {
		t.Errorf("available rows = %d, want %d", len(rows), len(cur.Available))
	}
	for _, o := range cur.Available {
		row, ok := rows[ordHref(o.ID)]
		if !ok {
			t.Errorf("available order %s missing", o.ID)
			continue
		}
		if got := dashActions(row); len(got) != 1 || got[0] != ordHref(o.ID)+"/start" {
			t.Errorf("%s buttons = %v, want just start", o.ID, got)
		}
		if !regexp.MustCompile(`class="btn small"[^>]*href="` + regexp.QuoteMeta(ordHref(o.ID)) + `">Open`).MatchString(row) {
			t.Errorf("%s has no Open link", o.ID)
		}
	}
	// Grouped by kind: local jobs first, then road trains.
	li, ri := strings.Index(avail, "Local jobs"), strings.Index(avail, "Road trains")
	if li < 0 || ri < li {
		t.Errorf("kind groups out of order: local at %d, road at %d", li, ri)
	}
	for _, o := range cur.Available {
		pos := strings.Index(avail, `href="`+ordHref(o.ID)+`"><b>`)
		if (o.Kind == orders.KindRoad) != (pos > ri) {
			t.Errorf("%s is under the wrong heading", o.ID)
		}
	}
}

func TestDashRoadBuild(t *testing.T) {
	w := dashLoadWorld(t)
	sim := dashNewSim(w)
	v := sim.View()
	body := dashReq(t, dashNewServer(t, sim), "GET", "/", nil).Body.String()
	card := dashSection(t, body, "road-builds")

	var shown, idle int
	for _, rb := range v.Building {
		if rb.WaitingFt <= 0 && !rb.Ready {
			idle++
			continue
		}
		shown++
		i := strings.Index(card, `<b class="mono">`+rb.Symbol+`</b>`)
		if i < 0 {
			t.Errorf("train %s missing from the build list", rb.Symbol)
			continue
		}
		block := card[i:]
		if j := strings.Index(block[1:], `<div class="ind">`); j >= 0 {
			block = block[:j+1]
		}
		if want := fmt.Sprintf(`style="width:%.1f%%"`, rb.Pct); !strings.Contains(block, want) {
			t.Errorf("%s: bar fill %q missing in %s", rb.Symbol, want, block)
		}
		if want := fmt.Sprintf(`left:%.1f%%`, rb.ThresholdPct); !strings.Contains(block, want) {
			t.Errorf("%s: threshold marker %q missing", rb.Symbol, want)
		}
		if want := fmt.Sprintf("%.0f%%", rb.Pct); !strings.Contains(block, want) {
			t.Errorf("%s: percentage %q missing", rb.Symbol, want)
		}
		if rb.Ready != strings.Contains(block, `chip ready">Ready`) || rb.Ready == strings.Contains(block, `chip wait">Building`) {
			t.Errorf("%s: ready chip does not match Ready=%v", rb.Symbol, rb.Ready)
		}
	}
	if shown == 0 || idle == 0 {
		t.Fatalf("fixture should have both building and idle trains, got %d and %d", shown, idle)
	}
	if !strings.Contains(card, fmt.Sprintf("%d trains have no cars waiting", idle)) {
		t.Errorf("idle train summary missing")
	}
}

func TestDashFragmentAndLiveUpdate(t *testing.T) {
	w := dashLoadWorld(t)
	sim := dashNewSim(w)
	h := dashNewServer(t, sim)

	full := dashReq(t, h, "GET", "/", nil).Body.String()
	frag := dashReq(t, h, "GET", "/?fragment=1", nil)
	if strings.Contains(frag.Body.String(), "<!DOCTYPE") || !strings.Contains(frag.Body.String(), `id="attention"`) {
		t.Errorf("fragment should be the main content only:\n%.300s", frag.Body)
	}
	if !strings.Contains(full, strings.TrimSpace(frag.Body.String())) {
		t.Error("fragment is not part of the full page")
	}
	if got := frag.Header().Get("X-Page-Title"); got != "Dashboard%20-%20Run8%20Operations%20Centre" {
		t.Errorf("X-Page-Title = %q", got)
	}

	// A change in the backend shows up in the next fragment without a reload.
	if strings.Contains(frag.Body.String(), "ord-1") {
		t.Fatal("no order should be active yet")
	}
	if _, err := sim.StartOrder("avail:ab10", ""); err != nil {
		t.Fatal(err)
	}
	again := dashReq(t, h, "GET", "/", nil, "X-Fragment", "1").Body.String()
	if !strings.Contains(dashSection(t, again, "orders-active"), ordHref("ord-1")) {
		t.Error("started order missing from the refreshed dashboard")
	}
}

func TestDashAdapter(t *testing.T) {
	w := dashLoadWorld(t)
	sim := dashNewSim(w)
	sim.base.Warnings = []engine.Warning{{Kind: "ingest", Message: "x"}}
	a := NewBackendAdapter(sim)
	c := a.Chrome()
	src := sim.base.Source
	if c.SaveName != src.SaveName || c.ActiveFile != src.ActiveFile || !c.GameDate.Equal(src.GameDate) ||
		!c.LastIngest.Equal(src.LastIngest) || c.SaveDir != src.SaveDir || c.AutoSaveDir != src.AutoSaveDir {
		t.Errorf("chrome %+v does not match source %+v", c, src)
	}
	if want := len(dashAttention(sim.View())); c.WarningCount != want || want < 1+len(w.Profile.Report.Problems) {
		t.Errorf("WarningCount = %d, attention items = %d", c.WarningCount, want)
	}

	ch, cancel := a.Subscribe()
	defer cancel()
	if _, err := sim.StartOrder("avail:ab10", ""); err != nil {
		t.Fatal(err)
	}
	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatal("no event after a command")
	}

	if got := NewBackendAdapter(&dashNilView{}).Chrome(); got != (Chrome{}) {
		t.Errorf("chrome for a nil View = %+v", got)
	}
}

type dashNilView struct{ dashSim }

func (*dashNilView) View() *engine.View { return nil }

// TestDashEventStream checks the whole path: a command on the backend reaches an open
// /events stream through the adapter.
func TestDashEventStream(t *testing.T) {
	w := dashLoadWorld(t)
	sim := dashNewSim(w)
	s, err := New(Config{}, NewBackendAdapter(sim))
	if err != nil {
		t.Fatal(err)
	}
	RegisterDashboard(s, sim)
	RegisterOrders(s, sim)
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/events")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	got := make(chan string, 1)
	go func() {
		sc := bufio.NewScanner(resp.Body)
		for sc.Scan() {
			if strings.HasPrefix(sc.Text(), "event: ") {
				got <- sc.Text()
				return
			}
		}
	}()
	// Wait for the subscription, then change state.
	deadline := time.Now().Add(2 * time.Second)
	for {
		sim.mu.Lock()
		n := len(sim.subs)
		sim.mu.Unlock()
		if n > 0 || time.Now().After(deadline) {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if _, err := sim.StartOrder("avail:ab10", ""); err != nil {
		t.Fatal(err)
	}
	select {
	case line := <-got:
		if line != "event: view-changed" {
			t.Errorf("event line %q", line)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no view-changed event after the order started")
	}
}
