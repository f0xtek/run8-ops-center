package web

import (
	"fmt"
	"html"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"regexp"
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

// vwBackend is a stateful engine.Backend for the yards, learn, setup and activity
// views. Its View comes from the real pipeline (parse, profile, carstatus.Derive,
// trackmap candidates and coverage) over the reference save, recomputed on every
// call, and its label commands change the real trackmap.Map the way the engine does.
type vwBackend struct {
	mu       sync.Mutex
	snap     *run8save.Snapshot
	base     *profile.Profile
	profiles []engine.ProfileInfo
	tm       *trackmap.Map
	settings store.Settings
	activity []store.ActivityEntry
	source   engine.Source
	guard    *store.Guard // answers SetInstallDir with the real protected-path rules
	dataDir  string
	subs     map[chan struct{}]struct{}

	calls   []string // every command, as text
	setErr  error    // returned by the next SetServer / SwitchProfile / SetThreshold when set
	thCalls []vwThreshold
}

type vwThreshold struct {
	Symbol string
	Min    *float64
	Age    *float64
}

func vwSnapshot(t *testing.T) *run8save.Snapshot {
	t.Helper()
	snap, err := run8save.ParseFile("../../testdata/WorldSave.xml")
	if err != nil {
		t.Fatal(err)
	}
	return snap
}

// vwProfile installs the bundled profile into a temp dir and loads it.
func vwProfile(t *testing.T) *profile.Profile {
	t.Helper()
	dir := t.TempDir()
	if err := profile.InstallBundled(store.NewGuard(), dir); err != nil {
		t.Fatal(err)
	}
	p, err := profile.Load(filepath.Join(dir, profile.BundledID))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// vwLabelledMap is the fixture track map: HTI's track and two Norris bowl tracks sit
// where the reference save parks covered hoppers and the CHA train.
func vwLabelledMap() *trackmap.Map {
	tm := trackmap.New()
	for _, sec := range vwHTISections {
		tm.Set(280, sec, trackmap.Label{Kind: trackmap.KindIndustry, Industries: []string{"HTI"}})
	}
	tm.Set(280, 3451, trackmap.Label{Kind: trackmap.KindYard, Yard: "NOR", Track: "Bowl 14"})
	tm.Set(280, 3452, trackmap.Label{Kind: trackmap.KindYard, Yard: "NOR", Track: "Bowl 2"})
	tm.Set(280, 3426, trackmap.Label{Kind: trackmap.KindYard, Yard: "NOR", Track: "Bowl 10"})
	return tm
}

var vwHTISections = []int{3437, 3439, 3449}

func vwNewBackend(t *testing.T, tm *trackmap.Map) *vwBackend {
	t.Helper()
	prof := vwProfile(t)
	dataDir := t.TempDir()
	g := store.NewGuard()
	g.SetDataDir(dataDir)
	b := &vwBackend{
		snap: vwSnapshot(t), base: prof, tm: tm, guard: g, dataDir: dataDir,
		profiles: []engine.ProfileInfo{{ID: prof.ID, Name: prof.Name}, {ID: "other-route", Name: "Other route"}},
		settings: store.DefaultSettings(),
		subs:     map[chan struct{}]struct{}{},
		source: engine.Source{
			SaveName: "ForRealThisTime", SaveDir: `C:\Run8\Trains`, AutoSaveDir: `C:\Run8\AutoSaves`,
			ActiveFile: "ForRealThisTime.xml", LastIngest: time.Date(2026, 10, 30, 19, 0, 0, 0, time.Local),
		},
	}
	b.settings.ProfileID = prof.ID
	b.source.GameDate = b.snap.GameDate
	return b
}

// effective applies the threshold overrides to a copy of the base profile, as the
// engine does.
func (b *vwBackend) effective() *profile.Profile {
	p := *b.base
	p.RoadTrains = append([]profile.RoadTrain(nil), b.base.RoadTrains...)
	for i := range p.RoadTrains {
		if th, ok := b.settings.Thresholds[p.RoadTrains[i].Symbol]; ok {
			if th.MinLengthPct != nil {
				p.RoadTrains[i].MinLengthPct = *th.MinLengthPct
			}
			if th.MaxCarAgeDays != nil {
				p.RoadTrains[i].MaxCarAgeDays = *th.MaxCarAgeDays
			}
		}
	}
	return &p
}

func (b *vwBackend) View() *engine.View {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.viewLocked()
}

func (b *vwBackend) viewLocked() *engine.View {
	prof := b.effective()
	st, _ := carstatus.Derive(b.snap, prof, b.tm, nil)
	v := &engine.View{
		Source: b.source, HasSnapshot: true, Profile: prof, Profiles: b.profiles,
		Status: st, TrackMap: b.tm,
		Candidates: b.tm.Candidates(b.snap, prof), Locations: b.tm.Locations(),
		Coverage: b.tm.Coverage(b.snap), Activity: append([]store.ActivityEntry(nil), b.activity...),
		Settings: b.settings,
	}
	v.Tiles.Unmapped = st.Unmapped
	return v
}

func (b *vwBackend) Chrome() Chrome {
	b.mu.Lock()
	defer b.mu.Unlock()
	s := b.source
	return Chrome{SaveName: s.SaveName, ActiveFile: s.ActiveFile, GameDate: s.GameDate, LastIngest: s.LastIngest,
		SaveDir: s.SaveDir, AutoSaveDir: s.AutoSaveDir}
}

func (b *vwBackend) Subscribe() (<-chan struct{}, func()) {
	ch := make(chan struct{}, 1)
	b.mu.Lock()
	b.subs[ch] = struct{}{}
	b.mu.Unlock()
	return ch, func() {
		b.mu.Lock()
		delete(b.subs, ch)
		b.mu.Unlock()
	}
}

func (b *vwBackend) record(format string, args ...any) {
	b.calls = append(b.calls, fmt.Sprintf(format, args...))
}

func (b *vwBackend) Calls() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]string(nil), b.calls...)
}

func (b *vwBackend) StartOrder(string, string) (orders.Order, error) {
	return orders.Order{}, fmt.Errorf("not used by these views")
}

func (b *vwBackend) ApplyOrder(string, orders.Action) (orders.Order, error) {
	return orders.Order{}, fmt.Errorf("not used by these views")
}

func (b *vwBackend) LabelCandidate(trainID string, l trackmap.Label) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.record("label-candidate %s %s", trainID, l.Location())
	for _, c := range b.tm.Candidates(b.snap, b.effective()) {
		if c.TrainID == trainID {
			b.tm.Apply(c, l)
			return nil
		}
	}
	return fmt.Errorf("train %s has no unmapped sections", trainID)
}

func (b *vwBackend) LabelSection(prefix, section int, l trackmap.Label) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.record("label-section %s %s", trackmap.Key(prefix, section), l.Location())
	b.tm.Set(prefix, section, l)
	return nil
}

func (b *vwBackend) RemoveLabel(prefix, section int) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.record("remove %s", trackmap.Key(prefix, section))
	b.tm.Remove(prefix, section)
	return nil
}

func (b *vwBackend) RenameLocation(old, new trackmap.Location) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.record("rename %s -> %s", old, new)
	n := b.tm.Rename(old, new)
	if n == 0 {
		return 0, fmt.Errorf("no section is labelled %s", old)
	}
	return n, nil
}

func (b *vwBackend) SetThreshold(symbol string, minPct, maxAge *float64) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.thCalls = append(b.thCalls, vwThreshold{symbol, minPct, maxAge})
	b.record("threshold %s", symbol)
	if err := b.setErr; err != nil {
		return err
	}
	th := store.Threshold{MinLengthPct: minPct, MaxCarAgeDays: maxAge}
	next := map[string]store.Threshold{}
	for k, v := range b.settings.Thresholds {
		next[k] = v
	}
	if minPct == nil && maxAge == nil {
		delete(next, symbol)
	} else {
		next[symbol] = th
	}
	b.settings.Thresholds = next
	return nil
}

func (b *vwBackend) SetInstallDir(dir string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.record("install-dir %q", dir)
	if err := b.guard.SetInstallDir(dir); err != nil {
		return err
	}
	b.settings.RunInstallDir = dir
	return nil
}

func (b *vwBackend) SetServer(port int, all bool) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.record("server %d %v", port, all)
	if b.setErr != nil {
		return b.setErr
	}
	b.settings.Port, b.settings.AllInterfaces = port, all
	return nil
}

func (b *vwBackend) SwitchProfile(id string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.record("profile %s", id)
	if b.setErr != nil {
		return b.setErr
	}
	b.settings.ProfileID = id
	return nil
}

// vwServer builds a server with the four views registered against b.
func vwServer(t *testing.T, b *vwBackend) http.Handler {
	t.Helper()
	s, err := New(Config{}, b)
	if err != nil {
		t.Fatal(err)
	}
	RegisterYards(s, b)
	RegisterLearn(s, b)
	RegisterSetup(s, b)
	RegisterActivity(s, b)
	return s.Handler()
}

func vwGet(t *testing.T, h http.Handler, target string, hdr ...string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, target, nil)
	for i := 0; i+1 < len(hdr); i += 2 {
		req.Header.Set(hdr[i], hdr[i+1])
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func vwPost(t *testing.T, h http.Handler, target string, form url.Values, hdr ...string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, target, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for i := 0; i+1 < len(hdr); i += 2 {
		req.Header.Set(hdr[i], hdr[i+1])
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

var (
	vwTagRE   = regexp.MustCompile(`(?s)<style.*?</style>|<[^>]*>`)
	vwSpaceRE = regexp.MustCompile(`\s+`)
)

// vwText reduces markup to the text a reader sees.
func vwText(markup string) string {
	return strings.TrimSpace(vwSpaceRE.ReplaceAllString(html.UnescapeString(vwTagRE.ReplaceAllString(markup, " ")), " "))
}

// vwBetween returns the markup from the first occurrence of from up to the next
// occurrence of to after it (or the end).
func vwBetween(t *testing.T, markup, from, to string) string {
	t.Helper()
	i := strings.Index(markup, from)
	if i < 0 {
		t.Fatalf("markup has no %q", from)
	}
	rest := markup[i+len(from):]
	if j := strings.Index(rest, to); j >= 0 {
		rest = rest[:j]
	}
	return markup[i:i+len(from)] + rest
}

// vwUnmappedCars counts snapshot cars whose first truck, then second truck, is not on
// a labelled section; it works from the raw snapshot, independent of carstatus.
func vwUnmappedCars(snap *run8save.Snapshot, tm *trackmap.Map) int {
	n := 0
	for _, c := range snap.Cars() {
		mapped := false
		for _, tr := range c.Trucks {
			if _, ok := tm.Lookup(tr.RoutePrefix, tr.Section); ok {
				mapped = true
				break
			}
		}
		if !mapped {
			n++
		}
	}
	return n
}

// vwNext is the page a redirect leads to, without the fragment a browser would keep to itself.
func vwNext(rec *httptest.ResponseRecorder) string {
	loc, _, _ := strings.Cut(rec.Header().Get("Location"), "#")
	return loc
}
