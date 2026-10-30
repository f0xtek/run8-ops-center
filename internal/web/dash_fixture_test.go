package web

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"run8-ops-center/internal/carstatus"
	"run8-ops-center/internal/engine"
	"run8-ops-center/internal/orders"
	"run8-ops-center/internal/profile"
	"run8-ops-center/internal/run8save"
	"run8-ops-center/internal/store"
	"run8-ops-center/internal/trackmap"
)

var (
	dashGameNow = time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	dashRealNow = time.Date(2026, 10, 30, 19, 15, 0, 0, time.Local)
)

// dashWorld is the reference save run through the real pipeline.
type dashWorld struct {
	Profile *profile.Profile
	Snap    *run8save.Snapshot
	Status  *carstatus.Status
	Map     *trackmap.Map
}

// dashLoadWorld parses testdata/WorldSave.xml, loads the bundled profile and labels
// enough sections that cars land in yards and at industries: every fourth
// industry-bound car sits at its industry, other industry-bound and train-bound
// cars sit on NOR bowl tracks, and the rest stay unmapped.
func dashLoadWorld(t *testing.T) *dashWorld {
	t.Helper()
	dir := t.TempDir()
	if err := profile.InstallBundled(store.NewGuard(), dir); err != nil {
		t.Fatal(err)
	}
	p, err := profile.Load(filepath.Join(dir, profile.BundledID))
	if err != nil {
		t.Fatal(err)
	}
	snap, err := run8save.ParseFile("../../testdata/WorldSave.xml")
	if err != nil {
		t.Fatal(err)
	}
	bare, _ := carstatus.Derive(snap, p, trackmap.New(), nil)
	section := map[string]run8save.Truck{}
	for _, tr := range snap.Trains {
		for _, v := range tr.Vehicles {
			if len(v.Trucks) > 0 {
				section[v.Key()] = v.Trucks[0]
			}
		}
	}
	tm := trackmap.New()
	for i, c := range bare.Cars {
		tk, ok := section[c.Key]
		if !ok {
			continue
		}
		switch {
		case c.Dest.Kind == carstatus.DestIndustry && i%4 == 0:
			tm.Set(tk.RoutePrefix, tk.Section, trackmap.Label{Kind: trackmap.KindIndustry, Industries: []string{c.Dest.Industry}})
		case c.Dest.Kind == carstatus.DestIndustry || c.Dest.Kind == carstatus.DestTrain:
			tm.Set(tk.RoutePrefix, tk.Section, trackmap.Label{Kind: trackmap.KindYard, Yard: "NOR", Track: fmt.Sprintf("Bowl %d", 1+i%20)})
		}
	}
	st, _ := carstatus.Derive(snap, p, tm, nil)
	return &dashWorld{Profile: p, Snap: snap, Status: st, Map: tm}
}

// dashCountTiles counts the dashboard tiles from a Status the way the engine does.
func dashCountTiles(st *carstatus.Status) engine.Tiles {
	t := engine.Tiles{OnRoute: len(st.Cars), Unmapped: st.Unmapped, Unknown: st.Unknown}
	for i := range st.Cars {
		c := &st.Cars[i]
		if c.Loc.Kind == carstatus.LocIndustry {
			t.AtIndustries++
		}
		switch c.Service {
		case carstatus.ServiceReadyToPull:
			t.ReadyToPull++
		case carstatus.ServiceAwaitingDelivery:
			t.AwaitingDelivery++
		case carstatus.ServiceAwaitingTrain:
			t.AwaitingTrain++
		}
	}
	return t
}

// dashBaseView is a View with a snapshot, with orders left for the caller.
func dashBaseView(w *dashWorld) *engine.View {
	res := orders.Generate(orders.GenInput{Profile: w.Profile, Status: w.Status})
	return &engine.View{
		Seq: 1,
		Source: engine.Source{
			SaveName: "WorldSave", SaveDir: "/run8/Trains", AutoSaveDir: "/run8/AutoSaves",
			ActiveFile: "WorldSave.xml", LastIngest: dashRealNow, GameDate: dashGameNow,
		},
		HasSnapshot: true,
		Profile:     w.Profile,
		Status:      w.Status,
		Available:   res.Orders,
		Building:    res.Building,
		Tiles:       dashCountTiles(w.Status),
		TrackMap:    w.Map,
	}
}

// dashSim is a stateful fake engine: it runs the real order lifecycle over a fixed
// Status, records every command and can be told to fail them.
type dashSim struct {
	mu       sync.Mutex
	w        *dashWorld
	base     *engine.View
	all      []orders.Order // started and finished orders
	seq      int
	calls    []string
	startErr error
	applyErr error
	subs     map[chan struct{}]struct{}
}

func dashNewSim(w *dashWorld) *dashSim { return dashSimFrom(w, dashBaseView(w)) }

// dashSimFrom builds a fake around a hand-made View.
func dashSimFrom(w *dashWorld, base *engine.View) *dashSim {
	return &dashSim{w: w, base: base, subs: map[chan struct{}]struct{}{}}
}

func (f *dashSim) clock() orders.Clock { return orders.Clock{Real: dashRealNow, Game: dashGameNow} }

func (f *dashSim) View() *engine.View {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.viewLocked()
}

func (f *dashSim) viewLocked() *engine.View {
	v := *f.base
	var active []orders.Order
	for _, o := range f.all {
		if o.Reserved() {
			active = append(active, o)
		}
	}
	res := orders.Generate(orders.GenInput{Profile: f.w.Profile, Status: f.w.Status, Active: active})
	v.Available, v.Building = res.Orders, res.Building
	v.Active = nil
	// newest first
	for i := len(active) - 1; i >= 0; i-- {
		v.Active = append(v.Active, active[i])
	}
	v.History = nil
	for i := len(f.all) - 1; i >= 0; i-- {
		if !f.all[i].Reserved() {
			v.History = append(v.History, f.all[i])
		}
	}
	return &v
}

func (f *dashSim) notifyLocked() {
	for ch := range f.subs {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

func (f *dashSim) Subscribe() (<-chan struct{}, func()) {
	ch := make(chan struct{}, 1)
	f.mu.Lock()
	f.subs[ch] = struct{}{}
	f.mu.Unlock()
	return ch, func() {
		f.mu.Lock()
		delete(f.subs, ch)
		f.mu.Unlock()
	}
}

func (f *dashSim) StartOrder(id, lineKey string) (orders.Order, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, fmt.Sprintf("start %s %q", id, lineKey))
	if f.startErr != nil {
		return orders.Order{}, f.startErr
	}
	for _, a := range f.viewLocked().Available {
		if a.ID != id {
			continue
		}
		f.seq++
		o, err := orders.Start(a, lineKey, fmt.Sprintf("ord-%d", f.seq), f.clock())
		if err != nil {
			f.seq--
			return orders.Order{}, err
		}
		o = orders.Reconcile(o, f.w.Status, f.w.Profile, f.clock())
		f.all = append(f.all, o)
		f.notifyLocked()
		return o, nil
	}
	return orders.Order{}, fmt.Errorf("no available order %q", id)
}

func (f *dashSim) ApplyOrder(id string, a orders.Action) (orders.Order, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, fmt.Sprintf("apply %s %s", id, a))
	if f.applyErr != nil {
		return orders.Order{}, f.applyErr
	}
	for i := range f.all {
		if f.all[i].ID == id {
			o, err := orders.Apply(f.all[i], a, f.clock())
			if err != nil {
				return orders.Order{}, err
			}
			f.all[i] = o
			f.notifyLocked()
			return o, nil
		}
	}
	return orders.Order{}, fmt.Errorf("no started order %q", id)
}

// markDetected puts an In Progress order into Completion Detected, as a newer
// snapshot would.
func (f *dashSim) markDetected(id string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range f.all {
		if f.all[i].ID == id {
			f.all[i].State = orders.StateCompletionDetected
			f.all[i].DetectedAt = dashRealNow.Add(time.Hour)
		}
	}
	f.notifyLocked()
}

func (f *dashSim) callLog() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.calls...)
}

func (f *dashSim) LabelCandidate(string, trackmap.Label) error { return errors.New("unused") }
func (f *dashSim) LabelSection(int, int, trackmap.Label) error { return errors.New("unused") }
func (f *dashSim) RemoveLabel(int, int) error                  { return errors.New("unused") }
func (f *dashSim) RenameLocation(old, new trackmap.Location) (int, error) {
	return 0, errors.New("unused")
}
func (f *dashSim) SetThreshold(string, *float64, *float64) error { return errors.New("unused") }
func (f *dashSim) SetInstallDir(string) error                    { return errors.New("unused") }
func (f *dashSim) SetServer(int, bool) error                     { return errors.New("unused") }
func (f *dashSim) SwitchProfile(string) error                    { return errors.New("unused") }
func (f *dashSim) PreviewImport() (engine.ImportPreview, error) {
	return engine.ImportPreview{}, errors.New("unused")
}
func (f *dashSim) ConfirmImport(string) error { return errors.New("unused") }

// dashNewServer registers the dashboard and order pages on a Server over b.
func dashNewServer(t *testing.T, b engine.Backend) http.Handler {
	t.Helper()
	s, err := New(Config{}, NewBackendAdapter(b))
	if err != nil {
		t.Fatal(err)
	}
	RegisterDashboard(s, b)
	RegisterOrders(s, b)
	return s.Handler()
}

// dashReq sends a request and returns the recorded response. A form is sent as the
// urlencoded body of a POST; hdr is alternating header names and values.
func dashReq(t *testing.T, h http.Handler, method, target string, form url.Values, hdr ...string) *httptest.ResponseRecorder {
	t.Helper()
	var req *http.Request
	if form != nil {
		req = httptest.NewRequest(method, target, strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	} else {
		req = httptest.NewRequest(method, target, nil)
	}
	for i := 0; i+1 < len(hdr); i += 2 {
		req.Header.Set(hdr[i], hdr[i+1])
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// dashOrder returns the available order for a job, failing the test if none exists.
func dashOrder(t *testing.T, v *engine.View, id string) orders.Order {
	t.Helper()
	for _, o := range v.Available {
		if o.ID == id {
			return o
		}
	}
	t.Fatalf("no available order %s; have %v", id, dashIDs(v.Available))
	return orders.Order{}
}

func dashIDs(list []orders.Order) []string {
	var ids []string
	for _, o := range list {
		ids = append(ids, o.ID)
	}
	return ids
}
