package web

import (
	"errors"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"testing"

	"run8-ops-center/internal/engine"
	"run8-ops-center/internal/orders"
)

// dashEdit changes a started order in place, as a snapshot reconcile would.
func (f *dashSim) dashEdit(t *testing.T, id string, fn func(o *orders.Order)) {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range f.all {
		if f.all[i].ID == id {
			fn(&f.all[i])
			return
		}
	}
	t.Fatalf("no started order %s", id)
}

// dashStart starts an order and fails the test on error.
func dashStart(t *testing.T, f *dashSim, id, lineKey string) orders.Order {
	t.Helper()
	o, err := f.StartOrder(id, lineKey)
	if err != nil {
		t.Fatal(err)
	}
	return o
}

func dashTotalLines(o orders.Order) int { return len(o.Lines) }

func TestOrdersActionsByState(t *testing.T) {
	w := dashLoadWorld(t)
	sim := dashNewSim(w)
	v := sim.View()

	prog := dashStart(t, sim, "avail:ab10", "")
	det := dashStart(t, sim, "avail:a44", "")
	sim.markDetected(det.ID)
	done := dashStart(t, sim, "avail:a55", "")
	if _, err := sim.ApplyOrder(done.ID, orders.ActComplete); err != nil {
		t.Fatal(err)
	}
	cancelled := dashStart(t, sim, "avail:ab45", "")
	if _, err := sim.ApplyOrder(cancelled.ID, orders.ActCancel); err != nil {
		t.Fatal(err)
	}
	h := dashNewServer(t, sim)

	actions := func(id string) []string {
		rec := dashReq(t, h, "GET", ordHref(id), nil)
		if rec.Code != 200 {
			t.Fatalf("%s: status %d", id, rec.Code)
		}
		got := dashActions(rec.Body.String())
		sort.Strings(got)
		return got
	}
	same := func(label string, got, want []string) {
		t.Helper()
		sort.Strings(want)
		if strings.Join(got, "\n") != strings.Join(want, "\n") {
			t.Errorf("%s: actions\n got %v\nwant %v", label, got, want)
		}
	}

	same("in progress", actions(prog.ID), []string{ordHref(prog.ID) + "/complete", ordHref(prog.ID) + "/cancel"})
	same("completion detected", actions(det.ID), []string{ordHref(det.ID) + "/confirm", ordHref(det.ID) + "/continue"})
	same("completed", actions(done.ID), nil)
	same("cancelled", actions(cancelled.ID), nil)

	// Available: start whole, plus start line for a local order with several lines, none for a road order.
	cur := sim.View()
	checked := map[orders.Kind]bool{}
	for _, o := range cur.Available {
		want := []string{ordHref(o.ID) + "/start"}
		if o.Kind == orders.KindLocal && len(o.Lines) > 1 {
			for _, l := range o.Lines {
				want = append(want, ordHref(o.ID)+"/lines/"+url.PathEscape(l.Key())+"/start")
			}
		}
		same("available "+o.ID, actions(o.ID), want)
		checked[o.Kind] = true
	}
	if !checked[orders.KindLocal] || !checked[orders.KindRoad] {
		t.Errorf("fixture should offer local and road orders, got %v from %v", checked, dashIDs(v.Available))
	}
	// A single-line local order offers only the whole-order start.
	single := orders.Order{ID: "avail:one", Kind: orders.KindLocal, State: orders.StateAvailable,
		Lines: []orders.Line{{Kind: orders.LineIndustry, Target: "HTI"}}}
	if got := ordActionsFor(&single, "/"); len(got) != 1 || got[0].Name != "start" {
		t.Errorf("single-line available order actions = %+v", got)
	}
	if got := dashTotalLines(single); got != 1 {
		t.Fatal("fixture broken")
	}
}

func TestOrdersDetailLocal(t *testing.T) {
	w := dashLoadWorld(t)
	sim := dashNewSim(w)
	h := dashNewServer(t, sim)
	o := dashOrder(t, sim.View(), "avail:ab45")
	body := dashReq(t, h, "GET", ordHref(o.ID), nil).Body.String()

	if !strings.Contains(body, "<title>AB45 order - Run8 Operations Centre</title>") {
		t.Error("document title should name the order")
	}
	if !strings.Contains(body, `<a href="/orders" aria-current="page">Orders</a>`) {
		t.Error("orders tab should be current")
	}
	// One card per line, in order, each with set-out and pick-up tables.
	last := -1
	for _, l := range o.Lines {
		label := l.Target
		if l.Name != "" && l.Name != l.Target {
			label = l.Name + " (" + l.Target + ")"
		}
		i := strings.Index(body, "<h3>"+html.EscapeString(label)+"</h3>")
		if i < 0 {
			t.Errorf("line %s not shown", label)
			continue
		}
		if i < last {
			t.Errorf("line %s out of order", label)
		}
		last = i
	}
	if n := strings.Count(body, "<h3 class=\"jobhead\">Set out</h3>"); n != len(o.Lines) {
		t.Errorf("set-out tables = %d, want %d", n, len(o.Lines))
	}
	if n := strings.Count(body, "<h3 class=\"jobhead\">Pick up</h3>"); n != len(o.Lines) {
		t.Errorf("pick-up tables = %d, want %d", n, len(o.Lines))
	}
	if n := strings.Count(body, "Pick-ups return to Norris (NOR)."); n != len(o.Lines) {
		t.Errorf("home yard note shown %d times, want once per line (%d)", n, len(o.Lines))
	}
	if !strings.Contains(body, "Industry line") || !strings.Contains(body, "Yard line") {
		t.Error("industry and yard lines should be labelled")
	}

	// Every car: number, current location from the Status, lading pill.
	var checkedCars int
	for _, l := range o.Lines {
		for _, ref := range append(append([]orders.CarRef{}, l.SetOuts...), l.PickUps...) {
			cur := w.Status.ByKey[ref.Key]
			if cur == nil {
				t.Fatalf("order car %s is not in the status", ref.Key)
			}
			row := dashCarRow(t, body, ref.Number)
			if !strings.Contains(row, html.EscapeString(cur.Loc.String())) {
				t.Errorf("%s: row lacks location %q:\n%s", ref.Number, cur.Loc.String(), row)
			}
			if pill := map[bool]string{true: "pill L\">LOAD", false: "pill E\">MTY"}[cur.Loaded]; !strings.Contains(row, pill) {
				t.Errorf("%s: row lacks %q", ref.Number, pill)
			}
			if !strings.Contains(row, "To do") {
				t.Errorf("%s: an available order's car should read To do", ref.Number)
			}
			checkedCars++
		}
	}
	if checkedCars == 0 {
		t.Fatal("order has no cars")
	}
	if !strings.Contains(body, `<table class="sl"`) {
		t.Error("tables should use the monospace switchlist style")
	}

	// Totals with limits and required HP.
	for _, want := range []string{
		"<b>" + ptFt(o.Totals.LengthFt) + "</b><span>Length</span>", "limit " + ptFt(float64(o.Totals.MaxLengthFt)),
		"<b>" + ptTons(o.Totals.GrossTons) + "</b><span>Gross weight</span>", "limit " + ptTons(float64(o.Totals.MaxTons)),
		"<b>" + ptGroup(int(o.Totals.RequiredHP+0.5)) + "</b><span>Required HP</span>",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("totals lack %q", want)
		}
	}
	if strings.Contains(body, "shared</span>") {
		t.Error("AB45 shares no cars, so no shared mark should appear")
	}
}

// dashCarRow returns the table row that starts with the car number.
func dashCarRow(t *testing.T, body, number string) string {
	t.Helper()
	re := regexp.MustCompile(`(?s)<tr>\s*<td[^>]*>` + regexp.QuoteMeta(html.EscapeString(number)) + `[^<]*(?:<span.*?</span>)?<div.*?</tr>`)
	m := re.FindString(body)
	if m == "" {
		t.Fatalf("no table row for car %q", number)
	}
	return m
}

func TestOrdersDetailProgressAndMissing(t *testing.T) {
	w := dashLoadWorld(t)
	sim := dashNewSim(w)
	h := dashNewServer(t, sim)
	o := dashStart(t, sim, "avail:ab45", "")
	var doneNum, missingNum, doneLine string
	sim.dashEdit(t, o.ID, func(o *orders.Order) {
		// First line fully done, one pick-up on the second line missing from the save.
		o.Lines[0].Done = true
		for i := range o.Lines[0].SetOuts {
			o.Lines[0].SetOuts[i].Done = true
		}
		doneNum, doneLine = o.Lines[0].SetOuts[0].Number, html.EscapeString(o.Lines[0].Name+" ("+o.Lines[0].Target+")")
		l := &o.Lines[1]
		if len(l.SetOuts) == 0 {
			t.Fatal("second line has no set-outs")
		}
		l.SetOuts[0].Key = "gone.xml#1"
		l.SetOuts[0].Missing = true
		l.SetOuts[0].Location = "NOR / Bowl 3"
		missingNum = l.SetOuts[0].Number
	})
	body := dashReq(t, h, "GET", ordHref(o.ID), nil).Body.String()

	if row := dashCarRow(t, body, doneNum); !strings.Contains(row, `chip ready">Done`) {
		t.Errorf("done car row:\n%s", row)
	}
	row := dashCarRow(t, body, missingNum)
	for _, want := range []string{`chip off">Missing`, `chip off">not in save`, "last seen NOR / Bowl 3"} {
		if !strings.Contains(row, want) {
			t.Errorf("missing car row lacks %q:\n%s", want, row)
		}
	}
	if !strings.Contains(body, `<h3>`) || !strings.Contains(body, `chip ready">Done</span>`) {
		t.Error("line Done chip missing")
	}
	i := strings.Index(body, doneLine+"</h3>")
	if i < 0 || !strings.Contains(body[i:i+300], `chip ready">Done`) {
		t.Errorf("line %s should carry a Done chip", doneLine)
	}
	if !strings.Contains(body, "cars done") {
		t.Error("progress summary missing")
	}
	if !strings.Contains(body, "Started ") || !strings.Contains(body, "game 2026-10-30 12:00") {
		t.Error("start times missing")
	}
}

func TestOrdersDetailRoad(t *testing.T) {
	w := dashLoadWorld(t)
	sim := dashNewSim(w)
	h := dashNewServer(t, sim)
	o := dashOrder(t, sim.View(), "avail:131")
	body := dashReq(t, h, "GET", ordHref(o.ID), nil).Body.String()

	ci := strings.Index(body, "Departing consist")
	if ci < 0 {
		t.Fatal("no departing consist")
	}
	if n := len(o.Consist); !strings.Contains(body, fmt.Sprintf("%d cars", n)) {
		t.Errorf("consist size %d not shown", n)
	}
	prev := ci
	for i, l := range o.Lines {
		at := strings.Index(body, fmt.Sprintf(`<span class="muted">Stop %d</span>`, i+1))
		if at < prev {
			t.Errorf("stop %d (%s) missing or before the previous section", i+1, l.Target)
		}
		prev = at
		label := l.Name + " (" + l.Target + ")"
		if !strings.Contains(body, "<h3>"+html.EscapeString(label)+"</h3>") {
			t.Errorf("stop label %q missing", label)
		}
	}
	if len(o.Lines) == 0 {
		t.Fatal("131 has no stops")
	}
	if strings.Contains(body, "Pick-ups return to") {
		t.Error("road orders have no home yard note")
	}
	for _, c := range o.Consist {
		dashCarRow(t, body, c.Number)
	}

	// Shared cars: flagged on the order and marked on each car.
	if len(o.Shared) == 0 {
		t.Fatal("131 should share cars with 339")
	}
	if !strings.Contains(body, fmt.Sprintf("%d cars are also on another available order", len(o.Shared))) || strings.Contains(body, "also listed on another available order") {
		t.Error("shared note missing, or the shared-cars flag repeats it")
	}
	for _, key := range o.Shared {
		ref := dashRef(o, key)
		if !strings.Contains(dashCarRow(t, body, ref.Number), `>shared</span>`) {
			t.Errorf("shared car %s lacks the shared mark", ref.Number)
		}
	}
	// A car that is not shared carries no mark.
	shared := map[string]bool{}
	for _, k := range o.Shared {
		shared[k] = true
	}
	for _, c := range o.Consist {
		if !shared[c.Key] && strings.Contains(dashCarRow(t, body, c.Number), `>shared</span>`) {
			t.Errorf("%s is not shared but is marked", c.Number)
		}
	}
	// Starting it releases the mark: the started order no longer lists shared cars.
	started := dashStart(t, sim, o.ID, "")
	after := dashReq(t, h, "GET", ordHref(started.ID), nil).Body.String()
	if strings.Contains(after, `>shared</span>`) {
		t.Error("a started order should not show shared marks")
	}
}

func dashRef(o orders.Order, key string) orders.CarRef {
	for _, c := range o.Consist {
		if c.Key == key {
			return c
		}
	}
	for _, l := range o.Lines {
		for _, c := range append(append([]orders.CarRef{}, l.SetOuts...), l.PickUps...) {
			if c.Key == key {
				return c
			}
		}
	}
	return orders.CarRef{}
}

func TestOrdersList(t *testing.T) {
	w := dashLoadWorld(t)
	sim := dashNewSim(w)
	h := dashNewServer(t, sim)
	prog := dashStart(t, sim, "avail:ab10", "")
	det := dashStart(t, sim, "avail:a44", "")
	sim.markDetected(det.ID)
	done := dashStart(t, sim, "avail:a55", "")
	cancelled := dashStart(t, sim, "avail:ab45", "")
	sim.dashEdit(t, done.ID, func(o *orders.Order) {})
	if _, err := sim.ApplyOrder(done.ID, orders.ActComplete); err != nil {
		t.Fatal(err)
	}
	if _, err := sim.ApplyOrder(cancelled.ID, orders.ActCancel); err != nil {
		t.Fatal(err)
	}

	body := dashReq(t, h, "GET", "/orders", nil).Body.String()
	for _, id := range []string{"orders-detected", "orders-progress", "orders-available", "orders-history"} {
		if !strings.Contains(body, `id="`+id+`"`) {
			t.Errorf("section %s missing", id)
		}
	}
	if !strings.Contains(dashSection(t, body, "orders-progress"), ordHref(prog.ID)) ||
		!strings.Contains(dashSection(t, body, "orders-detected"), ordHref(det.ID)) {
		t.Error("started orders are in the wrong section")
	}
	avail := dashRows(dashSection(t, body, "orders-available"))
	if len(avail) != len(sim.View().Available) {
		t.Errorf("available rows = %d, want %d", len(avail), len(sim.View().Available))
	}

	hist := dashSection(t, body, "orders-history")
	for _, o := range []orders.Order{done, cancelled} {
		i := strings.Index(hist, `href="`+ordHref(o.ID)+`"`)
		if i < 0 {
			t.Errorf("history lacks %s", o.ID)
			continue
		}
		row := hist[i:]
		if j := strings.Index(row, "</tr>"); j >= 0 {
			row = row[:j]
		}
		for _, want := range []string{
			dashRealNow.Format("2006-01-02 15:04"), "game 2026-10-30 12:00",
			html.EscapeString(ordCarNumbers(&o)[0]),
		} {
			if !strings.Contains(row, want) {
				t.Errorf("history row for %s lacks %q:\n%s", o.ID, want, row)
			}
		}
	}
	if !strings.Contains(hist, "Completed") || !strings.Contains(hist, "Cancelled") {
		t.Error("final states missing from history")
	}
}

func TestOrdersHistoryLimit(t *testing.T) {
	w := dashLoadWorld(t)
	sim := dashNewSim(w)
	h := dashNewServer(t, sim)
	o := dashStart(t, sim, "avail:ab10", "")
	if _, err := sim.ApplyOrder(o.ID, orders.ActCancel); err != nil {
		t.Fatal(err)
	}
	sim.mu.Lock()
	tpl := sim.all[0]
	sim.all = nil
	for i := range ordHistoryLimit + 5 {
		c := tpl
		c.ID = fmt.Sprintf("ord-%d", i+1)
		sim.all = append(sim.all, c)
	}
	sim.mu.Unlock()
	hist := dashSection(t, dashReq(t, h, "GET", "/orders", nil).Body.String(), "orders-history")
	if n := strings.Count(hist, "<tr>") - 1; n != ordHistoryLimit {
		t.Errorf("history rows = %d, want %d", n, ordHistoryLimit)
	}
	if want := fmt.Sprintf("Showing the newest %d of %d finished orders.", ordHistoryLimit, ordHistoryLimit+5); !strings.Contains(hist, want) {
		t.Errorf("history cap note missing: want %q", want)
	}
}

func TestOrdersCommandRouting(t *testing.T) {
	w := dashLoadWorld(t)
	line := dashOrder(t, dashNewSim(w).View(), "avail:ab45").Lines[0]
	lineKey := line.Key()

	type tc struct {
		name   string
		setup  func(f *dashSim) string // returns the order the request targets
		path   func(id string) string
		form   url.Values
		status int
		loc    string // expected Location; %s is the target order ID
		call   string // expected backend call; %s is the target order ID
	}
	inProgress := func(f *dashSim) string { return dashStart(t, f, "avail:ab10", "").ID }
	detected := func(f *dashSim) string {
		o := dashStart(t, f, "avail:ab10", "")
		f.markDetected(o.ID)
		return o.ID
	}
	post := func(suffix string) func(string) string {
		return func(id string) string { return ordHref(id) + suffix }
	}
	cases := []tc{
		{"start whole", func(*dashSim) string { return "avail:ab45" }, post("/start"), nil, 303, "/orders/ord-1", `start avail:ab45 ""`},
		{"start line", func(*dashSim) string { return "avail:ab45" },
			func(id string) string { return ordHref(id) + "/lines/" + url.PathEscape(lineKey) + "/start" },
			nil, 303, "/orders/ord-1", fmt.Sprintf("start avail:ab45 %q", lineKey)},
		{"confirm", detected, post("/confirm"), nil, 303, "/orders/%s", "apply %s confirm"},
		{"continue", detected, post("/continue"), nil, 303, "/orders/%s", "apply %s continue"},
		{"complete", inProgress, post("/complete"), nil, 303, "/orders/%s", "apply %s complete"},
		{"cancel", inProgress, post("/cancel"), nil, 303, "/orders/%s", "apply %s cancel"},
		{"cancel from the list", inProgress, post("/cancel"), url.Values{"next": {"/orders"}}, 303, "/orders", "apply %s cancel"},
		{"complete from the dashboard", inProgress, post("/complete"), url.Values{"next": {"/"}}, 303, "/", "apply %s complete"},
		{"next to another order", inProgress, post("/cancel"), url.Values{"next": {"/orders/ord-9"}}, 303, "/orders/ord-9", "apply %s cancel"},
		{"absolute next is ignored", inProgress, post("/cancel"), url.Values{"next": {"https://evil.example/x"}}, 303, "/orders/%s", "apply %s cancel"},
		{"protocol-relative next is ignored", inProgress, post("/cancel"), url.Values{"next": {"//evil.example"}}, 303, "/orders/%s", "apply %s cancel"},
		{"nested next is ignored", inProgress, post("/cancel"), url.Values{"next": {"/orders/ord-1/../../x"}}, 303, "/orders/%s", "apply %s cancel"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			sim := dashNewSim(w)
			h := dashNewServer(t, sim)
			id := c.setup(sim)
			before := len(sim.callLog())
			form := c.form
			if form == nil {
				form = url.Values{}
			}
			rec := dashReq(t, h, "POST", c.path(id), form)
			if rec.Code != c.status {
				t.Fatalf("status %d, want %d: %s", rec.Code, c.status, rec.Body)
			}
			if got, want := rec.Header().Get("Location"), strings.ReplaceAll(c.loc, "%s", id); got != want {
				t.Errorf("Location = %q, want %q", got, want)
			}
			calls := sim.callLog()[before:]
			if want := strings.ReplaceAll(c.call, "%s", id); len(calls) != 1 || calls[0] != want {
				t.Errorf("backend calls = %q, want [%q]", calls, want)
			}
		})
	}

	t.Run("GET on an action route is refused", func(t *testing.T) {
		sim := dashNewSim(w)
		h := dashNewServer(t, sim)
		id := dashStart(t, sim, "avail:ab10", "").ID
		rec := dashReq(t, h, "GET", ordHref(id)+"/cancel", nil)
		if rec.Code != 405 {
			t.Errorf("status %d, want 405", rec.Code)
		}
		if got := sim.callLog(); len(got) != 1 {
			t.Errorf("a GET reached the backend: %q", got)
		}
	})
}

func TestOrdersNextAllowlist(t *testing.T) {
	for next, want := range map[string]string{
		"/":                 "/",
		"/orders":           "/orders",
		"/orders/ord-3":     "/orders/ord-3",
		"/orders/avail:ab1": "/orders/avail:ab1",
		"":                  "/fallback",
		"/learn":            "/fallback",
		"/orders/":          "/fallback",
		"/orders/a/b":       "/fallback",
		"/orders/a?x=1":     "/fallback",
		"//evil.example":    "/fallback",
		"http://evil/":      "/fallback",
		`/orders/a\b`:       "/fallback",
	} {
		r, _ := http.NewRequest("POST", "/orders/x/cancel", strings.NewReader(url.Values{"next": {next}}.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		if got := ordNext(r, "/fallback"); got != want {
			t.Errorf("ordNext(%q) = %q, want %q", next, got, want)
		}
	}
}

func TestOrdersFragmentMode(t *testing.T) {
	w := dashLoadWorld(t)
	sim := dashNewSim(w)
	h := dashNewServer(t, sim)
	o := dashStart(t, sim, "avail:ab10", "")
	sim.markDetected(o.ID)
	frag := []string{"X-Fragment", "1"}

	// Confirm from the order page: the updated page comes back directly, with a toast.
	rec := dashReq(t, h, "POST", ordHref(o.ID)+"/confirm", url.Values{"next": {ordHref(o.ID)}}, frag...)
	if rec.Code != 200 {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	body := rec.Body.String()
	if strings.Contains(body, "<!DOCTYPE") || !strings.Contains(body, `id="order"`) || !strings.Contains(body, "Completed") {
		t.Errorf("expected the completed order as a fragment:\n%.400s", body)
	}
	if len(dashActions(body)) != 0 {
		t.Errorf("a completed order offers actions: %v", dashActions(body))
	}
	if got, _ := url.PathUnescape(rec.Header().Get("X-Toast")); got != "Order confirmed." {
		t.Errorf("X-Toast = %q", got)
	}

	// Cancel from the dashboard: the dashboard fragment comes back, not the order page.
	o2 := dashStart(t, sim, "avail:a44", "")
	rec = dashReq(t, h, "POST", ordHref(o2.ID)+"/cancel", url.Values{"next": {"/"}}, frag...)
	body = rec.Body.String()
	if rec.Code != 200 || !strings.Contains(body, `id="attention"`) || strings.Contains(body, `id="order"`) {
		t.Errorf("expected the dashboard fragment, got %d:\n%.300s", rec.Code, body)
	}
	if strings.Contains(dashSection(t, body, "orders-active"), ordHref(o2.ID)) {
		t.Error("the cancelled order is still listed as active")
	}
	if got, _ := url.PathUnescape(rec.Header().Get("X-Toast")); got != "Order cancelled." {
		t.Errorf("X-Toast = %q", got)
	}
	if got := rec.Header().Get("X-Page-Title"); !strings.HasPrefix(got, "Dashboard") {
		t.Errorf("X-Page-Title = %q, want the dashboard's", got)
	}

	// Starting redirects so the script moves to the new order.
	rec = dashReq(t, h, "POST", "/orders/avail:ab45/start", url.Values{}, frag...)
	if rec.Code != 303 || !strings.HasPrefix(rec.Header().Get("Location"), "/orders/ord-") {
		t.Errorf("start in fragment mode: %d %q", rec.Code, rec.Header().Get("Location"))
	}
	// The redirect target renders as a fragment when the header is kept.
	rec = dashReq(t, h, "GET", rec.Header().Get("Location"), nil, frag...)
	if rec.Code != 200 || strings.Contains(rec.Body.String(), "<!DOCTYPE") {
		t.Errorf("order fragment: %d", rec.Code)
	}
	for _, p := range []string{"/orders", "/orders?fragment=1"} {
		rec = dashReq(t, h, "GET", p, nil, frag...)
		if strings.Contains(rec.Body.String(), "<!DOCTYPE") || !strings.Contains(rec.Body.String(), `id="orders-history"`) {
			t.Errorf("%s: not a fragment of the list", p)
		}
	}
}

func TestOrdersErrors(t *testing.T) {
	w := dashLoadWorld(t)
	frag := []string{"X-Fragment", "1"}

	t.Run("backend failure on start, plain post", func(t *testing.T) {
		sim := dashNewSim(w)
		sim.startErr = errors.New("could not save state.json: disk full")
		h := dashNewServer(t, sim)
		rec := dashReq(t, h, "POST", "/orders/avail:ab10/start", url.Values{"next": {"/orders"}})
		if rec.Code != 422 {
			t.Fatalf("status %d", rec.Code)
		}
		body := rec.Body.String()
		if !strings.Contains(body, `role="alert"`) || !strings.Contains(body, "could not save state.json: disk full") {
			t.Errorf("inline message missing:\n%.500s", body)
		}
		if !strings.Contains(body, `id="orders-available"`) {
			t.Error("the list page should still render under the message")
		}
	})
	t.Run("backend failure, fragment", func(t *testing.T) {
		sim := dashNewSim(w)
		sim.startErr = errors.New("could not save state.json: disk full")
		h := dashNewServer(t, sim)
		rec := dashReq(t, h, "POST", "/orders/avail:ab10/start", url.Values{}, frag...)
		if rec.Code != 422 || strings.TrimSpace(rec.Body.String()) != "could not save state.json: disk full" {
			t.Errorf("got %d %q; the script shows this body as a toast", rec.Code, rec.Body)
		}
		if rec.Header().Get("Location") != "" {
			t.Error("an error must not redirect")
		}
	})
	t.Run("invalid transition", func(t *testing.T) {
		sim := dashNewSim(w)
		h := dashNewServer(t, sim)
		id := dashStart(t, sim, "avail:ab10", "").ID
		// The real lifecycle refuses to confirm an order that is only In Progress.
		rec := dashReq(t, h, "POST", ordHref(id)+"/confirm", url.Values{"next": {ordHref(id)}})
		if rec.Code != 409 {
			t.Fatalf("status %d, want 409", rec.Code)
		}
		body := rec.Body.String()
		if !strings.Contains(body, "cannot confirm an order that is in-progress") || !strings.Contains(body, `id="order"`) {
			t.Errorf("message or order page missing:\n%.500s", body)
		}
		rec = dashReq(t, h, "POST", ordHref(id)+"/confirm", url.Values{}, frag...)
		if rec.Code != 409 || !strings.Contains(rec.Body.String(), "cannot confirm") {
			t.Errorf("fragment: %d %q", rec.Code, rec.Body)
		}
	})
	t.Run("wrapped transition error", func(t *testing.T) {
		sim := dashNewSim(w)
		sim.applyErr = fmt.Errorf("engine: %w", orders.ErrInvalidTransition)
		h := dashNewServer(t, sim)
		id := dashStart(t, sim, "avail:ab10", "").ID
		if rec := dashReq(t, h, "POST", ordHref(id)+"/cancel", url.Values{}); rec.Code != 409 {
			t.Errorf("status %d, want 409", rec.Code)
		}
	})
	t.Run("unknown order", func(t *testing.T) {
		sim := dashNewSim(w)
		h := dashNewServer(t, sim)
		for _, p := range []string{"/orders/nope/cancel", "/orders/avail:nope/start", "/orders/nope/lines/industry%3AHTI/start"} {
			rec := dashReq(t, h, "POST", p, url.Values{})
			if rec.Code != 404 {
				t.Errorf("%s: status %d, want 404", p, rec.Code)
			}
		}
		if got := sim.callLog(); len(got) != 0 {
			t.Errorf("unknown orders reached the backend: %q", got)
		}
		rec := dashReq(t, h, "GET", "/orders/nope", nil)
		if rec.Code != 404 || !strings.Contains(rec.Body.String(), "Order not found") || !strings.Contains(rec.Body.String(), `href="/orders"`) {
			t.Errorf("not-found page: %d\n%.400s", rec.Code, rec.Body)
		}
		if !strings.Contains(rec.Body.String(), "<title>Order not found - ") {
			t.Error("not-found title")
		}
	})
}

// TestOrdersStartedLifecycle walks one job through the pages: start a single line, the
// rest stays available, then complete it.
func TestOrdersStartedLifecycle(t *testing.T) {
	w := dashLoadWorld(t)
	sim := dashNewSim(w)
	h := dashNewServer(t, sim)
	full := dashOrder(t, sim.View(), "avail:ab45")
	line := full.Lines[0]

	rec := dashReq(t, h, "POST", ordHref(full.ID)+"/lines/"+url.PathEscape(line.Key())+"/start", url.Values{})
	if rec.Code != 303 {
		t.Fatalf("status %d", rec.Code)
	}
	page := dashReq(t, h, "GET", rec.Header().Get("Location"), nil).Body.String()
	if !strings.Contains(page, "AB45 / "+line.Target) || !strings.Contains(page, "In progress") {
		t.Errorf("started line order page:\n%.600s", page)
	}
	if n := strings.Count(page, "<h3 class=\"jobhead\">Set out</h3>"); n != 1 {
		t.Errorf("a started line order has %d lines, want 1", n)
	}
	rest := dashOrder(t, sim.View(), "avail:ab45")
	if len(rest.Lines) != len(full.Lines)-1 {
		t.Errorf("remaining AB45 lines = %d, want %d", len(rest.Lines), len(full.Lines)-1)
	}
	_ = engine.View{}
}
