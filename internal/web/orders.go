package web

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"run8-ops-center/internal/engine"
	"run8-ops-center/internal/orders"
	"run8-ops-center/internal/profile"
)

// ordHistoryLimit caps the history table so a long session does not make the page huge.
const ordHistoryLimit = 100

// ordAction is one button: a POST form to URL.
type ordAction struct {
	Name    string // "start", "start-line", "confirm", "continue", "complete", "cancel"
	Label   string
	URL     string
	Class   string // "primary" or "danger"
	Confirm string // browser confirmation text, "" for none
	Next    string // page to come back to, sent as the hidden field "next"
}

// ordRow is the one-line summary of an order on the dashboard and the orders list.
type ordRow struct {
	ID        string
	Href      string
	Title     string
	Summary   string
	LinesText string
	State     orders.State
	Kind      orders.Kind
	Flags     []orders.Flag
	Actions   []ordAction
	ShowOpen  bool
	Bar       *ptBar // progress of a started order
	Progress  string
}

// ordLine is one switchlist line on the detail page.
type ordLine struct {
	Key       string
	Label     string
	Kind      string // "Industry line", "Yard line", "Stop 2"
	Done      bool
	Progress  string
	SetOuts   ptCarTable
	PickUps   ptCarTable
	Start     []ordAction // start just this line
	HomeNote  string      // local lines: "Pick-ups return to Norris (NOR)."
	ShowState bool        // started orders show per-line progress
}

type ordTime struct{ Label, Real, Game string }

// ordPage is the order detail page.
type ordPage struct {
	Notice     string
	NotFound   bool
	ID         string
	Row        ordRow
	Road       bool
	Flags      []orders.Flag
	SharedNote string
	Tiles      []ptTile
	Times      []ordTime
	Consist    ptCarTable
	ConsistFt  string
	Lines      []ordLine
}

// PageTitle names the order in the document title.
func (p *ordPage) PageTitle() string {
	if p.NotFound {
		return "Order not found"
	}
	return p.Row.Title + " order"
}

// ordHistoryRow is one finished order in the history table.
type ordHistoryRow struct {
	ID, Href, Title string
	State           orders.State
	Started         ordTime
	Finished        ordTime
	Cars            []string
}

// ordListPage is the /orders page.
type ordListPage struct {
	Notice       string
	Detected     []ordRow
	InProgress   []ordRow
	Local        []ordRow
	Road         []ordRow
	History      []ordHistoryRow
	HistoryTotal int
}

func ordHref(id string) string { return "/orders/" + url.PathEscape(id) }

func ordStartedState(s orders.State) bool {
	return s == orders.StateInProgress || s == orders.StateCompletionDetected
}

// ordFind looks an order up by ID in the available, active and history lists.
func ordFind(v *engine.View, id string) *orders.Order {
	for _, list := range [][]orders.Order{v.Active, v.Available, v.History} {
		for i := range list {
			if list[i].ID == id {
				return &list[i]
			}
		}
	}
	return nil
}

// ordTitle is the short name of an order: the symbol, plus the line when the
// order has exactly one.
func ordTitle(o *orders.Order) string {
	name := o.Symbol
	if name == "" {
		name = strings.ToUpper(o.JobID)
	}
	if o.Kind == orders.KindLocal && len(o.Lines) == 1 {
		name += " / " + o.Lines[0].Target
	}
	return name
}

func ordLineLabel(l orders.Line) string {
	switch l.Kind {
	case orders.LineYard:
		return l.Target + " yard"
	}
	return l.Target
}

// ordActionsFor lists the buttons valid for the order's state and nothing else.
// next is the page to return to after the action.
func ordActionsFor(o *orders.Order, next string) []ordAction {
	base := ordHref(o.ID)
	switch o.State {
	case orders.StateAvailable:
		return []ordAction{{Name: "start", Label: "Start", URL: base + "/start", Class: "primary"}}
	case orders.StateInProgress:
		return []ordAction{
			{Name: "complete", Label: "Mark complete", URL: base + "/complete", Next: next,
				Confirm: "Mark this order complete? Its cars are released."},
			{Name: "cancel", Label: "Cancel order", URL: base + "/cancel", Class: "danger", Next: next,
				Confirm: "Cancel this order? Its cars go back to the available orders."},
		}
	case orders.StateCompletionDetected:
		return []ordAction{
			{Name: "confirm", Label: "Confirm complete", URL: base + "/confirm", Class: "primary", Next: next},
			{Name: "continue", Label: "Continue working", URL: base + "/continue", Next: next},
		}
	}
	return nil
}

// ordCarProgress counts the distinct cars on an order and how many are done.
func ordCarProgress(o *orders.Order) (done, total int) {
	state := map[string]bool{} // key -> every listing done
	var order []string
	add := func(rs []orders.CarRef) {
		for _, r := range rs {
			d, seen := state[r.Key]
			if !seen {
				order = append(order, r.Key)
				d = true
			}
			state[r.Key] = d && r.Done
		}
	}
	add(o.Consist)
	for _, l := range o.Lines {
		add(l.SetOuts)
		add(l.PickUps)
	}
	for _, k := range order {
		if state[k] {
			done++
		}
	}
	return done, len(order)
}

func ordRowOf(v *engine.View, o *orders.Order, next string) ordRow {
	r := ordRow{
		ID: o.ID, Href: ordHref(o.ID), Title: ordTitle(o), State: o.State, Kind: o.Kind,
		Flags: o.Flags, Actions: ordActionsFor(o, next), ShowOpen: next != ordHref(o.ID),
	}
	var set, pick int
	var labels []string
	for _, l := range o.Lines {
		set += len(l.SetOuts)
		pick += len(l.PickUps)
		labels = append(labels, ordLineLabel(l))
	}
	r.LinesText = strings.Join(labels, " \u00b7 ")
	switch o.Kind {
	case orders.KindRoad:
		var parts []string
		if rt := ordTrain(v, o); rt != nil && rt.Origin != "" && rt.Destination != "" {
			parts = append(parts, rt.Origin+" to "+rt.Destination)
		}
		parts = append(parts, ptPlural(len(o.Consist), "car", "cars")+" leaving")
		if len(o.Lines) > 0 {
			parts = append(parts, ptPlural(len(o.Lines), "stop", "stops"))
		}
		r.Summary = strings.Join(parts, ", ")
		if len(o.Lines) > 0 {
			r.LinesText = "Stops: " + r.LinesText
		}
	default:
		r.Summary = fmt.Sprintf("%s, %s", ptPlural(set, "set-out", "set-outs"), ptPlural(pick, "pick-up", "pick-ups"))
	}
	if o.Totals.LengthFt > 0 {
		r.Summary += ", " + ptFt(o.Totals.LengthFt)
		if o.Totals.MaxLengthFt > 0 {
			r.Summary += " of " + ptFt(float64(o.Totals.MaxLengthFt))
		}
	}
	if ordStartedState(o.State) {
		done, total := ordCarProgress(o)
		if total > 0 {
			r.Progress = fmt.Sprintf("%d of %d cars done", done, total)
			r.Bar = &ptBar{Fill: ptCSSPct(float64(done) / float64(total) * 100), Label: r.Progress}
		}
	}
	return r
}

func ordTrain(v *engine.View, o *orders.Order) *profile.RoadTrain {
	if v.Profile == nil {
		return nil
	}
	sym := o.Symbol
	if sym == "" {
		sym = o.JobID
	}
	return v.Profile.RoadTrainBySymbol(sym)
}

// ordRows builds rows for every order in list whose state is in want.
func ordRows(v *engine.View, list []orders.Order, next string, want ...orders.State) []ordRow {
	var out []ordRow
	for i := range list {
		for _, s := range want {
			if list[i].State == s {
				out = append(out, ordRowOf(v, &list[i], next))
				break
			}
		}
	}
	return out
}

func ordListData(v *engine.View, notice string) *ordListPage {
	const next = "/orders"
	p := &ordListPage{
		Notice:     notice,
		Detected:   ordRows(v, v.Active, next, orders.StateCompletionDetected),
		InProgress: ordRows(v, v.Active, next, orders.StateInProgress),
	}
	for i := range v.Available {
		if v.Available[i].State != orders.StateAvailable {
			continue
		}
		row := ordRowOf(v, &v.Available[i], next)
		if v.Available[i].Kind == orders.KindRoad {
			p.Road = append(p.Road, row)
		} else {
			p.Local = append(p.Local, row)
		}
	}
	p.HistoryTotal = len(v.History)
	for i := range v.History {
		if i == ordHistoryLimit {
			break
		}
		o := &v.History[i]
		h := ordHistoryRow{
			ID: o.ID, Href: ordHref(o.ID), Title: ordTitle(o), State: o.State,
			Started:  ordTime{Real: ptWhen(o.StartedAt), Game: ptGame(o.GameStarted)},
			Finished: ordTime{Real: ptWhen(o.FinishedAt), Game: ptGame(o.GameFinished)},
		}
		h.Cars = ordCarNumbers(o)
		p.History = append(p.History, h)
	}
	return p
}

// ordCarNumbers lists the car numbers on an order once each, in order.
func ordCarNumbers(o *orders.Order) []string {
	numbers := map[string]string{}
	note := func(rs []orders.CarRef) {
		for _, r := range rs {
			if _, ok := numbers[r.Key]; !ok {
				numbers[r.Key] = r.Number
			}
		}
	}
	note(o.Consist)
	for _, l := range o.Lines {
		note(l.SetOuts)
		note(l.PickUps)
	}
	var out []string
	for _, k := range o.CarKeys() {
		out = append(out, numbers[k])
	}
	return out
}

// ordCarTable builds the rows for one list of cars, using each car's current
// state from the snapshot where there is one.
func ordCarTable(v *engine.View, refs []orders.CarRef, shared map[string]bool, empty string) ptCarTable {
	t := ptCarTable{Empty: empty}
	for _, r := range refs {
		c := ptCar{
			Number: r.Number, Type: string(r.Type), Loaded: r.Loaded, Tag: r.Tag, Where: r.Location,
			Missing: r.Missing, Done: r.Done, Shared: shared[r.Key],
		}
		if v.Status != nil {
			if cur := v.Status.ByKey[r.Key]; cur != nil {
				c.Where, c.Loaded = cur.Loc.String(), cur.Loaded
			} else {
				c.Gone = true
			}
		}
		t.Rows = append(t.Rows, c)
	}
	return t
}

func ordHomeYard(v *engine.View, o *orders.Order) string {
	if v.Profile == nil || o.Kind != orders.KindLocal {
		return ""
	}
	job := v.Profile.LocalJobByID(o.JobID)
	if job == nil || job.HomeYard == "" {
		return ""
	}
	if y := v.Profile.YardByCode(job.HomeYard); y != nil && y.Name != "" {
		return fmt.Sprintf("%s (%s)", y.Name, y.Code)
	}
	return job.HomeYard
}

func ordDetailData(v *engine.View, o *orders.Order, notice string) *ordPage {
	next := ordHref(o.ID)
	p := &ordPage{Notice: notice, ID: o.ID, Row: ordRowOf(v, o, next), Road: o.Kind == orders.KindRoad, Flags: o.Flags}
	shared := map[string]bool{}
	if o.State == orders.StateAvailable {
		for _, k := range o.Shared {
			shared[k] = true
		}
		if len(shared) > 0 {
			p.SharedNote = fmt.Sprintf("%s also on another available order. They are marked below. Starting this order removes them from the other.",
				ptPlural(len(shared), "car is", "cars are"))
		}
	}

	if p.SharedNote != "" { // the note says it already; the flag would repeat it
		p.Flags = nil
		for _, f := range o.Flags {
			if f.Code != "shared-cars" {
				p.Flags = append(p.Flags, f)
			}
		}
	}
	p.Tiles = ordTotals(o)
	if t := o.StartedAt; !t.IsZero() {
		p.Times = append(p.Times, ordTime{"Started", ptWhen(t), ptGame(o.GameStarted)})
	}
	if t := o.DetectedAt; !t.IsZero() {
		p.Times = append(p.Times, ordTime{Label: "Completion detected", Real: ptWhen(t)})
	}
	if t := o.FinishedAt; !t.IsZero() {
		p.Times = append(p.Times, ordTime{"Finished", ptWhen(t), ptGame(o.GameFinished)})
	}

	if p.Road {
		p.Consist = ordCarTable(v, o.Consist, shared, "No cars leave the origin yard on this train.")
		var ft float64
		for _, c := range o.Consist {
			ft += c.LengthFt
		}
		p.ConsistFt = fmt.Sprintf("%s, %s", ptPlural(len(o.Consist), "car", "cars"), ptFt(ft))
	}
	home := ordHomeYard(v, o)
	started := ordStartedState(o.State) || o.State == orders.StateCompleted || o.State == orders.StateCancelled
	for i, l := range o.Lines {
		kind := "Industry line"
		switch {
		case p.Road:
			kind = fmt.Sprintf("Stop %d", i+1)
		case l.Kind == orders.LineYard:
			kind = "Yard line"
		}
		label := l.Target
		if l.Name != "" && l.Name != l.Target {
			label = l.Name + " (" + l.Target + ")"
		}
		ln := ordLine{
			Key: l.Key(), Label: label, Kind: kind, Done: l.Done, ShowState: started,
			SetOuts: ordCarTable(v, l.SetOuts, shared, "Nothing to set out."),
			PickUps: ordCarTable(v, l.PickUps, shared, "Nothing to pick up."),
		}
		if !p.Road && home != "" {
			ln.HomeNote = "Pick-ups return to " + home + "."
		}
		if started {
			d, t := ordCarProgress(&orders.Order{Lines: []orders.Line{l}})
			ln.Progress = fmt.Sprintf("%d of %d cars done", d, t)
		}
		if o.State == orders.StateAvailable && o.Kind == orders.KindLocal && len(o.Lines) > 1 {
			ln.Start = []ordAction{{
				Name: "start-line", Label: "Start this line",
				URL: ordHref(o.ID) + "/lines/" + url.PathEscape(l.Key()) + "/start",
			}}
		}
		p.Lines = append(p.Lines, ln)
	}
	return p
}

// ordTotals are the summary tiles of the departing train, with limits and required HP.
func ordTotals(o *orders.Order) []ptTile {
	t := o.Totals
	length := ptTile{Value: ptFt(t.LengthFt), Label: "Length"}
	if t.MaxLengthFt > 0 {
		length.Note = "limit " + ptFt(float64(t.MaxLengthFt))
		length.Attn = t.LengthFt > float64(t.MaxLengthFt)
	} else {
		length.Note = "no limit set"
	}
	tons := ptTile{Value: ptTons(t.GrossTons), Label: "Gross weight"}
	if t.MaxTons > 0 {
		tons.Note = "limit " + ptTons(float64(t.MaxTons))
		tons.Attn = t.GrossTons > float64(t.MaxTons)
	} else {
		tons.Note = "no limit set"
	}
	hp := ptTile{Value: ptGroup(int(t.RequiredHP + 0.5)), Label: "Required HP"}
	return []ptTile{length, tons, hp}
}

// ordHandlers serves the order pages and actions.
type ordHandlers struct {
	s *Server
	b engine.Backend
}

// RegisterOrders registers the orders list, the order detail page and the order
// actions. Actions answer a plain form post with a 303 redirect, and a fragment
// request (the page script's) with the updated page, or with the error text and a 4xx
// status.
func RegisterOrders(s *Server, b engine.Backend) {
	dashRegisterPages(s)
	h := &ordHandlers{s: s, b: b}
	s.HandleFunc("GET /orders", h.list)
	s.HandleFunc("GET /orders/{id}", h.detail)
	s.HandleFunc("POST /orders/{id}/start", h.start)
	s.HandleFunc("POST /orders/{id}/lines/{lineKey}/start", h.startLine)
	s.HandleFunc("POST /orders/{id}/confirm", h.apply(orders.ActConfirm, "Order confirmed."))
	s.HandleFunc("POST /orders/{id}/continue", h.apply(orders.ActContinue, "Order back in progress."))
	s.HandleFunc("POST /orders/{id}/complete", h.apply(orders.ActComplete, "Order marked complete."))
	s.HandleFunc("POST /orders/{id}/cancel", h.apply(orders.ActCancel, "Order cancelled."))
}

func (h *ordHandlers) list(w http.ResponseWriter, r *http.Request) {
	h.s.Render(w, r, "orders", ordListData(dashViewOf(h.b), ""))
}

func (h *ordHandlers) detail(w http.ResponseWriter, r *http.Request) {
	page, data, status := h.pageFor(dashViewOf(h.b), ordHref(r.PathValue("id")), "")
	h.s.RenderStatus(w, r, status, page, data)
}

// pageFor builds the page that lives at one of the paths an action may return to:
// "/", "/orders" or "/orders/<id>". An unknown order gives the not-found page and 404.
func (h *ordHandlers) pageFor(v *engine.View, path, notice string) (page string, data any, status int) {
	switch {
	case path == "/":
		return "dash_home", dashPageData(v, notice), http.StatusOK
	case path == "/orders":
		return "orders", ordListData(v, notice), http.StatusOK
	}
	id, _ := url.PathUnescape(strings.TrimPrefix(path, "/orders/"))
	o := ordFind(v, id)
	if o == nil {
		return "order", &ordPage{Notice: notice, NotFound: true, ID: id}, http.StatusNotFound
	}
	return "order", ordDetailData(v, o, notice), http.StatusOK
}

// ordNext reads the page to return to from the form's hidden "next" field. Only the
// dashboard, the list and an order page are accepted, so the field cannot send the
// user to another site.
func ordNext(r *http.Request, fallback string) string {
	n := r.PostFormValue("next")
	if n == "/" || n == "/orders" {
		return n
	}
	if rest, ok := strings.CutPrefix(n, "/orders/"); ok && rest != "" && !strings.ContainsAny(rest, "/?#\\ \t\r\n") {
		return n
	}
	return fallback
}

// ordStatus maps a backend error to an HTTP status.
func ordStatus(err error) int {
	if errors.Is(err, orders.ErrInvalidTransition) {
		return http.StatusConflict
	}
	return http.StatusUnprocessableEntity
}

// fail reports an action error: the bare message for the page script (it shows it
// as a toast), or the page with an inline message for a plain form post.
func (h *ordHandlers) fail(w http.ResponseWriter, r *http.Request, status int, msg string) {
	if IsFragment(r) {
		http.Error(w, msg, status)
		return
	}
	h.render(w, r, ordNext(r, "/orders"), msg, "", status)
}

// render writes the page at path as the answer to the current request. The request is
// cloned with that path so the tab bar and title match the page, not the POST URL.
func (h *ordHandlers) render(w http.ResponseWriter, r *http.Request, path, notice, toast string, status int) {
	page, data, st := h.pageFor(dashViewOf(h.b), path, notice)
	if st == http.StatusNotFound {
		status = st
	}
	r2 := r.Clone(r.Context())
	u := *r.URL
	u.Path, u.RawPath, u.RawQuery = path, "", ""
	r2.URL = &u
	if toast != "" {
		SetToast(w, toast)
	}
	h.s.RenderStatus(w, r2, status, page, data)
}

func (h *ordHandlers) start(w http.ResponseWriter, r *http.Request) {
	h.startOrder(w, r, r.PathValue("id"), "")
}

func (h *ordHandlers) startLine(w http.ResponseWriter, r *http.Request) {
	h.startOrder(w, r, r.PathValue("id"), r.PathValue("lineKey"))
}

// startOrder starts an order or one line of it and sends the user to the new order.
func (h *ordHandlers) startOrder(w http.ResponseWriter, r *http.Request, id, lineKey string) {
	if ordFind(dashViewOf(h.b), id) == nil {
		h.fail(w, r, http.StatusNotFound, "That order is no longer available. A new save may have changed the list.")
		return
	}
	o, err := h.b.StartOrder(id, lineKey)
	if err != nil {
		h.fail(w, r, ordStatus(err), err.Error())
		return
	}
	http.Redirect(w, r, ordHref(o.ID), http.StatusSeeOther)
}

// apply handles confirm, continue, complete and cancel.
func (h *ordHandlers) apply(a orders.Action, done string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if ordFind(dashViewOf(h.b), id) == nil {
			h.fail(w, r, http.StatusNotFound, "No order with ID "+id+".")
			return
		}
		o, err := h.b.ApplyOrder(id, a)
		if err != nil {
			h.fail(w, r, ordStatus(err), err.Error())
			return
		}
		next := ordNext(r, ordHref(o.ID))
		if IsFragment(r) {
			h.render(w, r, next, "", done, http.StatusOK)
			return
		}
		http.Redirect(w, r, next, http.StatusSeeOther)
	}
}
