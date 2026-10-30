package web

import (
	"bytes"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"
)

// Tab is one entry of the primary tab bar.
type Tab struct {
	Key   string
	Href  string
	Label string
}

// tabs is the tab bar, in display order. View packages register their routes
// under these paths.
var tabs = []Tab{
	{"dashboard", "/", "Dashboard"},
	{"orders", "/orders", "Orders"},
	{"yards", "/yards", "Yards & industries"},
	{"learn", "/learn", "Learn mode"},
	{"setup", "/setup", "Setup"},
	{"activity", "/activity", "Activity"},
}

// Tabs returns the tab bar entries.
func Tabs() []Tab { return append([]Tab(nil), tabs...) }

// activeTab returns the key of the tab that owns urlPath, or "" if none does.
func activeTab(urlPath string) string {
	if urlPath == "/" {
		return "dashboard"
	}
	for _, t := range tabs[1:] {
		if urlPath == t.Href || strings.HasPrefix(urlPath, t.Href+"/") {
			return t.Key
		}
	}
	return ""
}

// View is the value every page template receives. Page templates define "main" and
// read their own data from Data.
type View struct {
	Chrome Chrome
	Tabs   []Tab
	Active string // key of the current tab
	Title  string // document title
	Path   string // request path
	Data   any    // whatever the handler passed to Render
}

// Titled may be implemented by Render data to override the document title.
type Titled interface{ PageTitle() string }

const appName = "Run8 Operations Centre"

var baseFuncs = template.FuncMap{
	"fmtGameDate": func(t time.Time) string { return t.Format("2006-01-02 15:04") },
	"fmtClock":    func(t time.Time) string { return t.Local().Format("15:04:05") },
	"iso":         func(t time.Time) string { return t.UTC().Format(time.RFC3339) },
}

// Funcs adds template functions for pages registered after the call. Call it before
// AddPages.
func (s *Server) Funcs(fm template.FuncMap) {
	s.tmplMu.Lock()
	defer s.tmplMu.Unlock()
	for k, v := range fm {
		s.funcs[k] = v
	}
}

// newTemplate parses the base layout and then the given page source into a fresh
// template set.
func (s *Server) newTemplate(name string, pageSrc ...string) (*template.Template, error) {
	t := template.New(name).Funcs(baseFuncs).Funcs(s.funcs)
	if _, err := t.New("base.html").Parse(baseSource); err != nil {
		return nil, fmt.Errorf("parse base layout: %w", err)
	}
	for i, src := range pageSrc {
		if _, err := t.New(fmt.Sprintf("%s#%d", name, i)).Parse(src); err != nil {
			return nil, fmt.Errorf("parse page %q: %w", name, err)
		}
	}
	return t, nil
}

// AddPages registers page templates from fsys. Every file matching one of patterns
// becomes a page named after the file without its extension (templates/orders.html is
// page "orders"). A page file must define a template called "main"; it is rendered
// inside the base layout, or alone for fragment requests, and receives a View.
// Registering a name again replaces the earlier page.
func (s *Server) AddPages(fsys fs.FS, patterns ...string) error {
	s.tmplMu.Lock()
	defer s.tmplMu.Unlock()
	for _, pat := range patterns {
		files, err := fs.Glob(fsys, pat)
		if err != nil {
			return fmt.Errorf("add pages %q: %w", pat, err)
		}
		if len(files) == 0 {
			return fmt.Errorf("add pages %q: no files match", pat)
		}
		for _, f := range files {
			src, err := fs.ReadFile(fsys, f)
			if err != nil {
				return fmt.Errorf("add pages: %w", err)
			}
			name := strings.TrimSuffix(path.Base(f), path.Ext(f))
			t, err := s.newTemplate(name, string(src))
			if err != nil {
				return err
			}
			if t.Lookup("main") == nil {
				return fmt.Errorf("add pages: %s does not define template \"main\"", f)
			}
			s.pages[name] = t
		}
	}
	return nil
}

// IsFragment reports whether the client wants only the main content: a request
// with fragment=1 in the query, or the X-Fragment: 1 header the page script sends
// (the header survives redirects, which the query string does not).
func IsFragment(r *http.Request) bool {
	return r.URL.Query().Get("fragment") == "1" || r.Header.Get("X-Fragment") == "1"
}

// SetToast asks the page script to show message in the toast area after it swaps in
// the response. Set it on the response that renders the fragment: a redirect's
// headers are dropped when fetch follows it, so an action handler that wants a toast
// should render directly when IsFragment(r) is true and redirect otherwise.
func SetToast(w http.ResponseWriter, message string) {
	w.Header().Set("X-Toast", url.PathEscape(message))
}

func (s *Server) view(urlPath string, data any) View {
	active := activeTab(urlPath)
	title := appName
	for _, t := range tabs {
		if t.Key == active {
			title = t.Label + " - " + appName
		}
	}
	if td, ok := data.(Titled); ok {
		title = td.PageTitle() + " - " + appName
	}
	return View{Chrome: s.backend.Chrome(), Tabs: tabs, Active: active, Title: title, Path: urlPath, Data: data}
}

// Render writes page inside the base layout with status 200, or only the page's
// main content when the request is a fragment request.
func (s *Server) Render(w http.ResponseWriter, r *http.Request, page string, data any) {
	s.RenderStatus(w, r, http.StatusOK, page, data)
}

// RenderStatus is Render with an explicit status code.
func (s *Server) RenderStatus(w http.ResponseWriter, r *http.Request, status int, page string, data any) {
	s.tmplMu.RLock()
	t := s.pages[page]
	s.tmplMu.RUnlock()
	if t == nil {
		http.Error(w, fmt.Sprintf("page %q is not registered", page), http.StatusInternalServerError)
		return
	}
	v := s.view(r.URL.Path, data)
	name := "layout"
	if IsFragment(r) {
		name = "main"
		w.Header().Set("X-Page-Title", url.PathEscape(v.Title))
	}
	var buf bytes.Buffer
	if err := t.ExecuteTemplate(&buf, name, v); err != nil {
		http.Error(w, "render "+page+": "+err.Error(), http.StatusInternalServerError)
		return
	}
	h := w.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Cache-Control", "no-store")
	h.Add("Vary", "X-Fragment")
	w.WriteHeader(status)
	_, _ = w.Write(buf.Bytes())
}

// chromeHandler serves the header and tab bar markup the page script swaps in after
// each change. The path query parameter says which tab is current.
func (s *Server) chromeHandler(w http.ResponseWriter, r *http.Request) {
	p := r.URL.Query().Get("path")
	if !strings.HasPrefix(p, "/") {
		p = "/"
	}
	var buf bytes.Buffer
	if err := s.chromeTmpl.ExecuteTemplate(&buf, "chrome", s.view(p, nil)); err != nil {
		http.Error(w, "render chrome: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(buf.Bytes())
}
