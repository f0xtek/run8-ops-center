package web

import (
	"bufio"
	"context"
	"errors"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"
)

// fakeBackend is a Backend whose chrome data and events the test controls.
type fakeBackend struct {
	mu     sync.Mutex
	chrome Chrome
	subs   map[chan struct{}]struct{}
}

func newFakeBackend() *fakeBackend {
	return &fakeBackend{
		subs: map[chan struct{}]struct{}{},
		chrome: Chrome{
			SaveName:     "ForRealThisTime",
			ActiveFile:   "Auto Save World 2026.10.28 17.28.02.xml",
			GameDate:     time.Date(1985, 3, 14, 6, 30, 0, 0, time.UTC),
			LastIngest:   time.Date(2026, 10, 28, 17, 28, 5, 0, time.Local),
			SaveDir:      `C:\Run8\Trains`,
			AutoSaveDir:  `C:\Run8\AutoSaves`,
			WarningCount: 3,
		},
	}
}

func (f *fakeBackend) Chrome() Chrome {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.chrome
}

func (f *fakeBackend) Subscribe() (<-chan struct{}, func()) {
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

func (f *fakeBackend) subscribers() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.subs)
}

func (f *fakeBackend) fire() {
	f.mu.Lock()
	defer f.mu.Unlock()
	for ch := range f.subs {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

func newTestServer(t *testing.T, cfg Config) (*Server, *fakeBackend) {
	t.Helper()
	fb := newFakeBackend()
	s, err := New(cfg, fb)
	if err != nil {
		t.Fatal(err)
	}
	return s, fb
}

func get(t *testing.T, h http.Handler, target string, hdr ...string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, target, nil)
	for i := 0; i+1 < len(hdr); i += 2 {
		req.Header.Set(hdr[i], hdr[i+1])
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestLayoutRendersChrome(t *testing.T) {
	s, _ := newTestServer(t, Config{})
	rec := get(t, s.Handler(), "/")
	if rec.Code != 200 {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"<!DOCTYPE html>", `<main id="main"`, "ForRealThisTime", "1985-03-14 06:30",
		"Last ingest", `<time datetime="2026-10-`, `Warnings <b>3</b>`, `class="badge warn"`,
		`C:\Run8\Trains`, `C:\Run8\AutoSaves`, "Auto Save World 2026.10.28 17.28.02.xml",
		`id="toast"`, `role="status"`, `href="#main"`, `<nav aria-label="Primary">`,
		`/static/style.css`, `/static/app.js`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("layout missing %q\n%s", want, body)
		}
	}
	for _, tab := range []string{"Dashboard", "Orders", "Yards &amp; industries", "Learn mode", "Setup", "Activity"} {
		if !strings.Contains(body, ">"+tab+"</a>") {
			t.Errorf("tab %q missing", tab)
		}
	}
	for _, href := range []string{`href="/"`, `href="/orders"`, `href="/yards"`, `href="/learn"`, `href="/setup"`, `href="/activity"`} {
		if !strings.Contains(body, href) {
			t.Errorf("tab link %s missing", href)
		}
	}
	if n := strings.Count(body, `aria-current="page"`); n != 1 {
		t.Errorf("want exactly one current tab, got %d", n)
	}
	if !regexp.MustCompile(`<a href="/" aria-current="page">Dashboard</a>`).MatchString(body) {
		t.Error("dashboard tab not marked current on /")
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Errorf("content type %q", ct)
	}
	if csp := rec.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "default-src 'none'") || !strings.Contains(csp, "connect-src 'self'") {
		t.Errorf("CSP %q", csp)
	}
}

func TestLayoutBeforeFirstIngest(t *testing.T) {
	s, fb := newTestServer(t, Config{})
	fb.chrome = Chrome{SaveName: "WorldSave"}
	body := get(t, s.Handler(), "/").Body.String()
	for _, want := range []string{"No save yet", "Waiting for a save", `Warnings <b>0</b>`, "No save has been ingested yet"} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q", want)
		}
	}
	if strings.Contains(body, `badge warn`) {
		t.Error("zero warnings must not use the warning style")
	}
}

func TestFragmentModeReturnsOnlyMain(t *testing.T) {
	s, _ := newTestServer(t, Config{})
	full := get(t, s.Handler(), "/").Body.String()
	for name, rec := range map[string]*httptest.ResponseRecorder{
		"query":  get(t, s.Handler(), "/?fragment=1"),
		"header": get(t, s.Handler(), "/", "X-Fragment", "1"),
	} {
		body := rec.Body.String()
		if rec.Code != 200 {
			t.Fatalf("%s: status %d", name, rec.Code)
		}
		for _, bad := range []string{"<html", "<header", "<nav", "<main", "<script", "Last ingest"} {
			if strings.Contains(body, bad) {
				t.Errorf("%s: fragment contains %q:\n%s", name, bad, body)
			}
		}
		if !strings.Contains(body, "Dashboard") || !strings.Contains(body, `id="attention"`) {
			t.Errorf("%s: fragment lost the page content:\n%s", name, body)
		}
		if !strings.Contains(full, strings.TrimSpace(body)) {
			t.Errorf("%s: fragment is not the same markup as the main of the full page", name)
		}
		if got := rec.Header().Get("X-Page-Title"); got != "Dashboard%20-%20Run8%20Operations%20Centre" {
			t.Errorf("%s: X-Page-Title %q", name, got)
		}
		if rec.Header().Get("Cache-Control") != "no-store" {
			t.Errorf("%s: fragment must not be cached", name)
		}
	}
}

func TestChromeEndpointTracksPathAndData(t *testing.T) {
	s, fb := newTestServer(t, Config{})
	rec := get(t, s.Handler(), "/_chrome?path=/orders/42")
	body := rec.Body.String()
	if !strings.Contains(body, `<a href="/orders" aria-current="page">Orders</a>`) {
		t.Errorf("orders tab not current:\n%s", body)
	}
	if strings.Contains(body, "<main") || strings.Contains(body, "<html") {
		t.Errorf("chrome fragment contains page body:\n%s", body)
	}
	fb.mu.Lock()
	fb.chrome.WarningCount = 7
	fb.mu.Unlock()
	if body := get(t, s.Handler(), "/_chrome?path=/").Body.String(); !strings.Contains(body, "Warnings <b>7</b>") {
		t.Errorf("chrome did not reflect new backend data:\n%s", body)
	}
	if body := get(t, s.Handler(), "/_chrome?path=junk").Body.String(); !strings.Contains(body, `aria-current="page">Dashboard`) {
		t.Errorf("bad path should fall back to dashboard:\n%s", body)
	}
}

func TestActiveTab(t *testing.T) {
	for p, want := range map[string]string{
		"/": "dashboard", "/orders": "orders", "/orders/ab10/start": "orders", "/ordersx": "",
		"/yards": "yards", "/learn": "learn", "/setup": "setup", "/activity": "activity", "/nope": "",
	} {
		if got := activeTab(p); got != want {
			t.Errorf("activeTab(%q) = %q, want %q", p, got, want)
		}
	}
}

type titled struct{ T string }

func (t titled) PageTitle() string { return t.T }

func TestAddPagesRenderAndOverrideRoot(t *testing.T) {
	s, _ := newTestServer(t, Config{})
	s.Funcs(map[string]any{"shout": strings.ToUpper})
	err := s.AddPages(fstest.MapFS{
		"orders.html": {Data: []byte(`{{define "main"}}<h2>Orders for {{shout .Data.T}}</h2>{{end}}`)},
		"home.html":   {Data: []byte(`{{define "main"}}<p id="custom-home">mine</p>{{end}}`)},
	}, "*.html")
	if err != nil {
		t.Fatal(err)
	}
	s.Handle("GET /orders", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.Render(w, r, "orders", titled{"ab10"})
	}))
	s.Handle("GET /{$}", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.Render(w, r, "home", nil)
	}))

	root := get(t, s.Handler(), "/").Body.String()
	if !strings.Contains(root, `id="custom-home"`) || strings.Contains(root, `id="attention"`) {
		t.Errorf("registering the root route must replace the shell:\n%s", root)
	}
	rec := get(t, s.Handler(), "/orders")
	body := rec.Body.String()
	if !strings.Contains(body, "<h2>Orders for AB10</h2>") || !strings.Contains(body, "<title>ab10 - Run8 Operations Centre</title>") {
		t.Errorf("orders page wrong:\n%s", body)
	}
	if !strings.Contains(body, `<a href="/orders" aria-current="page">`) {
		t.Error("orders tab should be current")
	}
	if frag := get(t, s.Handler(), "/orders", "X-Fragment", "1").Body.String(); strings.TrimSpace(frag) != "<h2>Orders for AB10</h2>" {
		t.Errorf("fragment = %q", frag)
	}
}

func TestRootOverrideWithPlainSlash(t *testing.T) {
	s, _ := newTestServer(t, Config{})
	s.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "catch-all") }))
	if got := get(t, s.Handler(), "/").Body.String(); got != "catch-all" {
		t.Errorf("got %q", got)
	}
}

func TestUnknownPathAndMissingPage(t *testing.T) {
	s, _ := newTestServer(t, Config{})
	if rec := get(t, s.Handler(), "/orders"); rec.Code != 404 {
		t.Errorf("unregistered tab: status %d", rec.Code)
	}
	s.Handle("GET /x", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { s.Render(w, r, "nope", nil) }))
	if rec := get(t, s.Handler(), "/x"); rec.Code != 500 {
		t.Errorf("missing page template: status %d", rec.Code)
	}
}

func TestAddPagesErrors(t *testing.T) {
	s, _ := newTestServer(t, Config{})
	if err := s.AddPages(fstest.MapFS{"a.html": {Data: []byte("no main here")}}, "*.html"); err == nil {
		t.Error("page without main must be rejected")
	}
	if err := s.AddPages(fstest.MapFS{}, "*.html"); err == nil {
		t.Error("pattern with no files must be rejected")
	}
	if err := s.AddPages(fstest.MapFS{"b.html": {Data: []byte("{{define")}}, "*.html"); err == nil {
		t.Error("syntax error must be reported")
	}
}

func TestHandleConflictKeepsRoutes(t *testing.T) {
	s, _ := newTestServer(t, Config{})
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "ok") })
	s.Handle("GET /a/{x}", ok)
	func() {
		defer func() {
			if recover() == nil {
				t.Error("expected panic for an invalid pattern")
			}
		}()
		s.Handle("GET /bad/{", ok)
	}()
	if got := get(t, s.Handler(), "/a/1").Body.String(); got != "ok" {
		t.Errorf("routes broken after failed Handle: %q", got)
	}
}

// readEvent reads one SSE block (lines up to a blank line).
func readEvent(t *testing.T, br *bufio.Reader) string {
	t.Helper()
	var sb strings.Builder
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			t.Fatalf("read event: %v (got %q)", err, sb.String())
		}
		if line == "\n" {
			return sb.String()
		}
		sb.WriteString(line)
	}
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestEventsDeliversViewChangedAndStopsOnDisconnect(t *testing.T) {
	s, fb := newTestServer(t, Config{})
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	ctx, cancel := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/events", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("content type %q", ct)
	}
	br := bufio.NewReader(resp.Body)
	if first := readEvent(t, br); !strings.Contains(first, ": connected") {
		t.Fatalf("first block %q", first)
	}
	waitFor(t, "subscription", func() bool { return fb.subscribers() == 1 })

	fb.fire()
	if ev := readEvent(t, br); !strings.Contains(ev, "event: view-changed") {
		t.Fatalf("got %q, want view-changed", ev)
	}
	fb.fire()
	if ev := readEvent(t, br); !strings.Contains(ev, "event: view-changed") {
		t.Fatalf("second event %q", ev)
	}

	cancel() // client goes away
	waitFor(t, "handler to unsubscribe after disconnect", func() bool { return fb.subscribers() == 0 })
}

func TestEventsKeepAlive(t *testing.T) {
	s, _ := newTestServer(t, Config{})
	s.keepAlive = 20 * time.Millisecond
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()
	resp, err := http.Get(ts.URL + "/events")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	br := bufio.NewReader(resp.Body)
	readEvent(t, br) // connected
	if ka := readEvent(t, br); ka != ": keepalive\n" {
		t.Errorf("keepalive block %q", ka)
	}
}

func TestEventsEndsWhenBackendChannelCloses(t *testing.T) {
	s, _ := newTestServer(t, Config{})
	closed := make(chan struct{})
	close(closed)
	s.backend = closingBackend{s.backend, closed}
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()
	resp, err := http.Get(ts.URL + "/events")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if _, err := io.ReadAll(resp.Body); err != nil {
		t.Fatalf("stream should end cleanly: %v", err)
	}
}

type closingBackend struct {
	Backend
	ch chan struct{}
}

func (c closingBackend) Subscribe() (<-chan struct{}, func()) { return c.ch, func() {} }

func TestServeShutsDownAndEndsEventStreams(t *testing.T) {
	s, fb := newTestServer(t, Config{Port: 0})
	ln, err := s.Listen()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Serve(ctx, ln) }()

	url := s.URL(ln)
	if !strings.HasPrefix(url, "http://127.0.0.1:") || !strings.HasSuffix(url, "/") {
		t.Fatalf("URL %q", url)
	}
	resp, err := http.Get(url + "events")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	br := bufio.NewReader(resp.Body)
	readEvent(t, br)
	waitFor(t, "subscription", func() bool { return fb.subscribers() == 1 })

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Serve returned %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Serve did not return after cancel while an event stream was open")
	}
	if _, err := io.ReadAll(br); err != nil {
		t.Errorf("stream should end cleanly on shutdown: %v", err)
	}
	if _, err := net.DialTimeout("tcp", ln.Addr().String(), 200*time.Millisecond); err == nil {
		t.Error("listener still accepting after shutdown")
	}
}

func TestBindAddress(t *testing.T) {
	for _, tc := range []struct {
		name string
		cfg  Config
		host string
	}{
		{"loopback default", Config{}, "127.0.0.1"},
		{"all interfaces", Config{AllInterfaces: true}, "0.0.0.0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, _ := newTestServer(t, tc.cfg)
			ln, err := s.Listen()
			if err != nil {
				t.Fatal(err)
			}
			defer ln.Close()
			host, _, err := net.SplitHostPort(ln.Addr().String())
			if err != nil {
				t.Fatal(err)
			}
			if tc.cfg.AllInterfaces {
				// Go listens dual-stack for the IPv4 wildcard, so the OS may report "::".
				if ip := net.ParseIP(host); ip == nil || !ip.IsUnspecified() {
					t.Errorf("bound to %q, want the wildcard address", host)
				}
			} else if host != tc.host {
				t.Errorf("bound to %q, want %q", host, tc.host)
			}
			if u := s.URL(ln); !strings.HasPrefix(u, "http://127.0.0.1:") {
				t.Errorf("URL %q should always use loopback", u)
			}
		})
	}
	if got := (Config{Port: 8484}).Addr(); got != "127.0.0.1:8484" {
		t.Errorf("Addr %q", got)
	}
	if got := (Config{Port: 8484, AllInterfaces: true}).Addr(); got != "0.0.0.0:8484" {
		t.Errorf("Addr %q", got)
	}
}

// A loopback bind must be unreachable on the machine's LAN address; all-interfaces
// must be reachable there.
func TestBindReachability(t *testing.T) {
	var lan net.IP
	addrs, _ := net.InterfaceAddrs()
	for _, a := range addrs {
		if ipn, ok := a.(*net.IPNet); ok && ipn.IP.To4() != nil && !ipn.IP.IsLoopback() && !ipn.IP.IsLinkLocalUnicast() {
			lan = ipn.IP
			break
		}
	}
	if lan == nil {
		t.Skip("no non-loopback IPv4 address on this machine")
	}
	for _, all := range []bool{false, true} {
		s, _ := newTestServer(t, Config{AllInterfaces: all})
		ln, err := s.Listen()
		if err != nil {
			t.Fatal(err)
		}
		port := ln.Addr().(*net.TCPAddr).Port
		c, err := net.DialTimeout("tcp", net.JoinHostPort(lan.String(), strconv.Itoa(port)), time.Second)
		if c != nil {
			c.Close()
		}
		if all && err != nil {
			t.Errorf("AllInterfaces: not reachable on %s: %v", lan, err)
		}
		if !all && err == nil {
			t.Errorf("loopback bind reachable on %s", lan)
		}
		ln.Close()
	}
}

func TestPortInUse(t *testing.T) {
	first, _ := newTestServer(t, Config{})
	ln, err := first.Listen()
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port

	second, _ := newTestServer(t, Config{Port: port})
	_, err = second.Listen()
	if err == nil {
		t.Fatal("second listener on the same port succeeded")
	}
	var pe *PortInUseError
	if !errors.As(err, &pe) || !errors.Is(err, ErrPortInUse) {
		t.Fatalf("want PortInUseError, got %T: %v", err, err)
	}
	msg := err.Error()
	if !strings.Contains(msg, strconv.Itoa(port)) || !strings.Contains(msg, "--port") || !strings.Contains(msg, "Setup") {
		t.Errorf("message must name the port and the setting to change: %q", msg)
	}
}

func TestNewValidates(t *testing.T) {
	if _, err := New(Config{}, nil); err == nil {
		t.Error("nil backend accepted")
	}
	if _, err := New(Config{Port: 70000}, newFakeBackend()); err == nil {
		t.Error("bad port accepted")
	}
}

func TestStaticAssets(t *testing.T) {
	s, _ := newTestServer(t, Config{})
	for path, wantType := range map[string]string{
		"/static/style.css":   "text/css; charset=utf-8",
		"/static/app.js":      "text/javascript; charset=utf-8",
		"/static/theme.js":    "text/javascript; charset=utf-8",
		"/static/favicon.svg": "image/svg+xml",
	} {
		rec := get(t, s.Handler(), path)
		if rec.Code != 200 {
			t.Errorf("%s: status %d", path, rec.Code)
			continue
		}
		if ct := rec.Header().Get("Content-Type"); ct != wantType {
			t.Errorf("%s: content type %q, want %q", path, ct, wantType)
		}
		if rec.Body.Len() == 0 {
			t.Errorf("%s: empty body", path)
		}
		if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
			t.Errorf("%s: nosniff missing", path)
		}
	}
	if rec := get(t, s.Handler(), "/static/missing.js"); rec.Code != 404 {
		t.Errorf("missing asset status %d", rec.Code)
	}
	if rec := get(t, s.Handler(), "/static/../templates/base.html"); rec.Code == 200 && strings.Contains(rec.Body.String(), "define") {
		t.Error("templates must not be served as static files")
	}
}

func TestStylesheetPortsExampleTheme(t *testing.T) {
	b, err := fs.ReadFile(assets, "static/style.css")
	if err != nil {
		t.Fatal(err)
	}
	css := string(b)
	for _, want := range []string{
		"prefers-color-scheme: dark", `[data-theme="dark"]`, `:root:not([data-theme="light"])`,
		`"IBM Plex Sans"`, `"IBM Plex Mono"`, `"Barlow Condensed"`, ".card{", ".tile{", ".pill{", ".chip{", ".chip.ready", "table{",
		"prefers-reduced-motion",
	} {
		if !strings.Contains(css, want) {
			t.Errorf("stylesheet missing %q", want)
		}
	}
}

// The app must work offline: nothing shipped may reference another origin.
func TestNoExternalReferences(t *testing.T) {
	external := regexp.MustCompile(`(?i)(https?:)?//[a-z0-9-]+\.[a-z]{2,}|@import|fonts\.googleapis|fonts\.gstatic`)
	err := fs.WalkDir(assets, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := fs.ReadFile(assets, p)
		if err != nil {
			return err
		}
		text := strings.ReplaceAll(string(b), `xmlns="http://www.w3.org/2000/svg"`, "")
		if m := external.FindString(text); m != "" {
			t.Errorf("%s references an external resource: %q", p, m)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	body := get(t, mustServer(t).Handler(), "/").Body.String()
	if m := regexp.MustCompile(`(?i)(src|href)="(https?:)?//`).FindString(body); m != "" {
		t.Errorf("rendered page references another origin: %q", m)
	}
}

func mustServer(t *testing.T) *Server {
	s, _ := newTestServer(t, Config{})
	return s
}

func TestBrowserCommand(t *testing.T) {
	const u = "http://127.0.0.1:8484/"
	for _, tc := range []struct {
		goos string
		name string
		args []string
	}{
		{"windows", "rundll32", []string{"url.dll,FileProtocolHandler", u}},
		{"darwin", "open", []string{u}},
		{"linux", "xdg-open", []string{u}},
	} {
		name, args := browserCommand(tc.goos, u)
		if name != tc.name || strings.Join(args, "|") != strings.Join(tc.args, "|") {
			t.Errorf("%s: got %s %v", tc.goos, name, args)
		}
	}
	for _, bad := range []string{"file:///etc/passwd", "calc.exe", "javascript:alert(1)"} {
		if err := OpenBrowser(bad); err == nil {
			t.Errorf("OpenBrowser(%q) must refuse non-http URLs", bad)
		}
	}
}
