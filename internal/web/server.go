// Package web is the HTTP layer: listener setup, the shared page layout, static
// assets, the live-update event stream and helpers for views to render into the layout.
//
// Views register routes with [Server.Handle] and render with [Server.Render].
package web

import (
	"context"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"net"
	"net/http"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

// DefaultPort is the port the app uses when the user does not choose one.
const DefaultPort = 8484

// Config controls where the server listens.
type Config struct {
	Port          int  // TCP port; 0 asks the OS for a free one
	AllInterfaces bool // bind 0.0.0.0 instead of 127.0.0.1
	NoBrowser     bool // the caller skips OpenBrowser when set
}

// Addr returns the listen address: loopback only unless AllInterfaces is set.
func (c Config) Addr() string {
	host := "127.0.0.1"
	if c.AllInterfaces {
		host = "0.0.0.0"
	}
	return net.JoinHostPort(host, strconv.Itoa(c.Port))
}

// ErrPortInUse is matched by errors.Is for the error Listen returns when the port
// is taken.
var ErrPortInUse = errors.New("port in use")

// PortInUseError says which port could not be bound and how to change it.
type PortInUseError struct {
	Port int
	Err  error
}

func (e *PortInUseError) Error() string {
	return fmt.Sprintf("port %d is already in use (another copy of the app may be running): choose a different port with --port or in Setup", e.Port)
}

func (e *PortInUseError) Unwrap() []error { return []error{ErrPortInUse, e.Err} }

// wsaeaddrinuse is the Windows Sockets code for "address already in use"; Go's
// syscall.EADDRINUSE does not match it on Windows.
const wsaeaddrinuse = 10048

func isAddrInUse(err error) bool {
	if errors.Is(err, syscall.EADDRINUSE) {
		return true
	}
	var en syscall.Errno
	return runtime.GOOS == "windows" && errors.As(err, &en) && uintptr(en) == wsaeaddrinuse
}

// Server is the web server. Create one with [New], register views with
// [Server.Handle] and run it with [Server.Listen] and [Server.Serve].
type Server struct {
	cfg     Config
	backend Backend

	keepAlive time.Duration // SSE keepalive interval

	routeMu sync.Mutex
	routes  map[string]http.Handler
	mux     atomic.Pointer[http.ServeMux]

	tmplMu     sync.RWMutex
	funcs      template.FuncMap
	pages      map[string]*template.Template
	chromeTmpl *template.Template
}

// New builds a server with the layout, static assets, /events and a minimal
// dashboard shell at /. A view replaces the shell by calling Handle for "/" (or
// "/{$}", optionally with a method) and registering its own page templates.
func New(cfg Config, b Backend) (*Server, error) {
	if b == nil {
		return nil, errors.New("web: nil backend")
	}
	if cfg.Port < 0 || cfg.Port > 65535 {
		return nil, fmt.Errorf("web: invalid port %d", cfg.Port)
	}
	s := &Server{
		cfg:       cfg,
		backend:   b,
		keepAlive: 20 * time.Second,
		routes:    map[string]http.Handler{},
		funcs:     template.FuncMap{},
		pages:     map[string]*template.Template{},
	}
	var err error
	if s.chromeTmpl, err = s.newTemplate("chrome"); err != nil {
		return nil, err
	}
	sub, err := fs.Sub(assets, "templates")
	if err != nil {
		return nil, fmt.Errorf("web: embedded templates: %w", err)
	}
	if err := s.AddPages(sub, "dashboard.html"); err != nil {
		return nil, err
	}
	s.routes["GET /static/"] = staticHandler()
	s.routes["GET /events"] = http.HandlerFunc(s.events)
	s.routes["GET /_chrome"] = http.HandlerFunc(s.chromeHandler)
	s.rebuild()
	return s, nil
}

// Handle registers handler for pattern, using net/http.ServeMux pattern syntax
// ("GET /orders/{id}"). Registering the same pattern again replaces the earlier
// handler. Registering a pattern for the root path ("/", "/{$}", with or without a
// method) replaces the built-in dashboard shell. It is safe to call while serving.
func (s *Server) Handle(pattern string, handler http.Handler) {
	if handler == nil {
		panic("web: nil handler for " + pattern)
	}
	s.routeMu.Lock()
	defer s.routeMu.Unlock()
	old, had := s.routes[pattern]
	s.routes[pattern] = handler
	defer func() {
		if p := recover(); p != nil { // invalid or conflicting pattern: keep the old table
			if had {
				s.routes[pattern] = old
			} else {
				delete(s.routes, pattern)
			}
			panic(p)
		}
	}()
	s.rebuildLocked()
}

// HandleFunc is Handle for a function.
func (s *Server) HandleFunc(pattern string, f http.HandlerFunc) { s.Handle(pattern, f) }

func (s *Server) rebuild() {
	s.routeMu.Lock()
	defer s.routeMu.Unlock()
	s.rebuildLocked()
}

// isRootPattern reports whether a ServeMux pattern targets exactly the site root.
func isRootPattern(p string) bool {
	if i := strings.IndexByte(p, ' '); i >= 0 {
		p = strings.TrimLeft(p[i+1:], " ")
	}
	return p == "/" || p == "/{$}"
}

func (s *Server) rebuildLocked() {
	mux := http.NewServeMux()
	hasRoot := false
	for p, h := range s.routes {
		mux.Handle(p, h)
		hasRoot = hasRoot || isRootPattern(p)
	}
	if !hasRoot {
		mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
			s.Render(w, r, "dashboard", nil)
		})
	}
	s.mux.Store(mux)
}

// Handler returns the full HTTP handler, with the security headers applied.
func (s *Server) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self' 'unsafe-inline'; "+
			"img-src 'self' data:; connect-src 'self'; form-action 'self'; base-uri 'none'; frame-ancestors 'none'")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		s.mux.Load().ServeHTTP(w, r)
	})
}

// Listen binds the configured address. A taken port yields a *PortInUseError.
func (s *Server) Listen() (net.Listener, error) {
	addr := s.cfg.Addr()
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		if isAddrInUse(err) {
			return nil, &PortInUseError{Port: s.cfg.Port, Err: err}
		}
		return nil, fmt.Errorf("listen on %s: %w (choose another port with --port or in Setup)", addr, err)
	}
	return ln, nil
}

// URL returns the address a browser on this PC should open for ln. It always uses
// the loopback address, even when the server is bound to all interfaces.
func (s *Server) URL(ln net.Listener) string {
	port := s.cfg.Port
	if ta, ok := ln.Addr().(*net.TCPAddr); ok {
		port = ta.Port
	}
	return "http://" + net.JoinHostPort("127.0.0.1", strconv.Itoa(port)) + "/"
}

// Serve serves on ln until ctx is cancelled, then shuts down gracefully (open event
// streams are ended). It returns nil after a clean shutdown.
func (s *Server) Serve(ctx context.Context, ln net.Listener) error {
	srv := &http.Server{
		Handler:           s.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       2 * time.Minute,
		BaseContext:       func(net.Listener) context.Context { return ctx },
	}
	errc := make(chan error, 1)
	go func() { errc <- srv.Serve(ln) }()
	select {
	case err := <-errc:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("serve: %w", err)
	case <-ctx.Done():
	}
	shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutCtx); err != nil {
		_ = srv.Close()
	}
	<-errc
	return nil
}

// events is the Server-Sent Events stream: "view-changed" whenever the backend
// signals, a comment every keepAlive to hold proxies and idle timers open.
func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	rc := http.NewResponseController(w)
	ch, cancel := s.backend.Subscribe()
	defer cancel()

	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache, no-transform")
	h.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	// Opening comment plus a reconnect delay for EventSource.
	if _, err := fmt.Fprint(w, "retry: 2000\n: connected\n\n"); err != nil || rc.Flush() != nil {
		return
	}

	tick := time.NewTicker(s.keepAlive)
	defer tick.Stop()
	for {
		var msg string
		select {
		case <-r.Context().Done():
			return
		case _, ok := <-ch:
			if !ok {
				return
			}
			msg = "event: view-changed\ndata: {}\n\n"
		case <-tick.C:
			msg = ": keepalive\n\n"
		}
		if _, err := fmt.Fprint(w, msg); err != nil || rc.Flush() != nil {
			return
		}
	}
}
