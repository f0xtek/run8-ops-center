package web

import (
	"fmt"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"run8-ops-center/internal/engine"
	"run8-ops-center/internal/profile"
)

// RegisterSetup serves the setup view at /setup and its settings actions:
// POST /settings/threshold, /settings/install-dir, /settings/server and
// /settings/profile.
func RegisterSetup(s *Server, b engine.Backend) {
	vwPages(s, "setup.html")
	h := &setupHandler{s: s, b: b}
	s.HandleFunc("GET /setup", h.page)
	s.HandleFunc("POST /settings/threshold", h.threshold)
	s.HandleFunc("POST /settings/install-dir", h.installDir)
	s.HandleFunc("POST /settings/server", h.server)
	s.HandleFunc("POST /settings/profile", h.profile)
}

type setupHandler struct {
	s *Server
	b engine.Backend
}

type setupPage struct {
	Source      engine.Source
	HasProfile  bool
	ProfileName string
	ProfileDir  string
	Profiles    []setupProfile
	InstallDir  string
	Port        string
	AllIfaces   bool
	Trains      []setupTrain
	Report      []setupProblemGroup
	Problems    int
	Notice      string // what was just saved, from ?saved= or a direct render
	Error       string
}

type setupProfile struct {
	ID       string
	Name     string
	Selected bool
}

type setupTrain struct {
	Symbol      string
	Route       string
	MaxLength   string // "7,000 ft" or "no limit"
	MinPct      string // effective value
	MaxAge      string // effective value, "" for none
	PctOverride bool
	AgeOverride bool
}

type setupProblemGroup struct {
	Code  string
	Title string
	Items []profile.Problem
}

// setupNotices maps the ?saved= value a redirect carries to the text shown.
var setupNotices = map[string]string{
	"threshold":   "Train limits saved. Available orders were recomputed.",
	"install-dir": "Run8 install folder saved.",
	"server":      "Port and bind saved. Restart the app for them to take effect.",
	"profile":     "Profile switched. Available orders were recomputed.",
}

// setupProblemTitles name the validation problem codes, in display order.
var setupProblemTitles = []struct{ Code, Title string }{
	{profile.CodeMalformedTag, "Malformed outgoing tags"},
	{profile.CodeOrphanTag, "Outgoing tags that reach no train"},
	{profile.CodeDuplicateTag, "Duplicate tags"},
	{profile.CodeUnservedIndustry, "Industries nothing serves"},
	{profile.CodeUnknownRef, "References to unknown yards or industries"},
	{profile.CodeUnmatchedModel, "Car models with no car type"},
	{profile.CodeUnmappedCarType, "Car types with no defaults"},
}

func setupFloat(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }

func setupFeet(n int) string {
	if n <= 0 {
		return "no limit"
	}
	s := strconv.Itoa(n)
	if len(s) > 3 {
		s = s[:len(s)-3] + "," + s[len(s)-3:]
	}
	return s + " ft"
}

// setupDraft carries form values back into the page after a rejected save.
type setupDraft struct {
	InstallDir *string
	Port       *string
	AllIfaces  *bool
}

func setupBuild(v *engine.View, notice, errMsg string, d setupDraft) setupPage {
	p := setupPage{
		Source: v.Source, HasProfile: v.Profile != nil, Notice: notice, Error: errMsg,
		InstallDir: v.Settings.RunInstallDir, AllIfaces: v.Settings.AllInterfaces,
	}
	port := v.Settings.Port
	if port == 0 {
		port = DefaultPort
	}
	p.Port = strconv.Itoa(port)
	if d.InstallDir != nil {
		p.InstallDir = *d.InstallDir
	}
	if d.Port != nil {
		p.Port = *d.Port
	}
	if d.AllIfaces != nil {
		p.AllIfaces = *d.AllIfaces
	}
	current := v.Settings.ProfileID
	if v.Profile != nil {
		current = v.Profile.ID
		p.ProfileName, p.ProfileDir = v.Profile.Name, v.Profile.Dir
	}
	for _, pi := range v.Profiles {
		p.Profiles = append(p.Profiles, setupProfile{ID: pi.ID, Name: pi.Name, Selected: pi.ID == current})
	}
	if v.Profile == nil {
		return p
	}
	for _, rt := range v.Profile.RoadTrains {
		t := setupTrain{
			Symbol: rt.Symbol, MaxLength: setupFeet(rt.Limit.MaxLengthFt),
			MinPct: setupFloat(rt.MinLengthPct),
		}
		if rt.MaxCarAgeDays > 0 {
			t.MaxAge = setupFloat(rt.MaxCarAgeDays)
		}
		switch {
		case rt.Origin == "":
			t.Route = "through train"
		case rt.Destination != "":
			t.Route = "from " + rt.Origin + " to " + rt.Destination
		default:
			t.Route = "from " + rt.Origin
		}
		if th, ok := v.Settings.Thresholds[rt.Symbol]; ok {
			t.PctOverride, t.AgeOverride = th.MinLengthPct != nil, th.MaxCarAgeDays != nil
		}
		p.Trains = append(p.Trains, t)
	}
	byCode := map[string][]profile.Problem{}
	for _, pr := range v.Profile.Report.Problems {
		byCode[pr.Code] = append(byCode[pr.Code], pr)
		p.Problems++
	}
	for _, pt := range setupProblemTitles {
		if items := byCode[pt.Code]; len(items) > 0 {
			p.Report = append(p.Report, setupProblemGroup{pt.Code, pt.Title, items})
			delete(byCode, pt.Code)
		}
	}
	var rest []string
	for code := range byCode {
		rest = append(rest, code)
	}
	sort.Strings(rest)
	for _, code := range rest {
		p.Report = append(p.Report, setupProblemGroup{code, code, byCode[code]})
	}
	return p
}

func (h *setupHandler) page(w http.ResponseWriter, r *http.Request) {
	notice := setupNotices[r.URL.Query().Get("saved")]
	h.s.Render(w, r, "setup", setupBuild(h.b.View(), notice, "", setupDraft{}))
}

func (h *setupHandler) fail(w http.ResponseWriter, r *http.Request, status int, msg string, d setupDraft) {
	vwFail(h.s, w, r, status, "setup", msg, func(m string) any {
		return setupBuild(h.b.View(), "", m, d)
	})
}

// done ends a saved change. The redirect carries which setting was saved so the page
// can say so.
func (h *setupHandler) done(w http.ResponseWriter, r *http.Request, what string) {
	notice := setupNotices[what]
	vwFinish(h.s, w, r, "setup", "/setup?saved="+url.QueryEscape(what)+"#"+what, notice, func() any {
		return setupBuild(h.b.View(), notice, "", setupDraft{})
	})
}

// setupOptional parses a number from the form: blank is nil (no override).
func setupOptional(raw, what string, lo, hi float64) (*float64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	f, err := strconv.ParseFloat(raw, 64)
	if err != nil || math.IsNaN(f) {
		return nil, fmt.Errorf("%s must be a number, not %q", what, raw)
	}
	if f < lo || f > hi {
		return nil, fmt.Errorf("%s must be between %s and %s", what, setupFloat(lo), setupFloat(hi))
	}
	return &f, nil
}

func (h *setupHandler) threshold(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.fail(w, r, vwBadForm, "could not read the form: "+err.Error(), setupDraft{})
		return
	}
	v := h.b.View()
	sym := vwClean(r.PostFormValue("symbol"))
	if v.Profile == nil || v.Profile.RoadTrainBySymbol(sym) == nil {
		h.fail(w, r, vwBadForm, fmt.Sprintf("%q is not a road train in the profile", sym), setupDraft{})
		return
	}
	var minPct, maxAge *float64
	if r.PostFormValue("reset") == "" {
		var err error
		if minPct, err = setupOptional(r.PostFormValue("min_length_pct"), "Minimum length", 0, 100); err != nil {
			h.fail(w, r, vwBadForm, sym+": "+err.Error(), setupDraft{})
			return
		}
		if maxAge, err = setupOptional(r.PostFormValue("max_car_age_days"), "Maximum car age", 0, 3650); err != nil {
			h.fail(w, r, vwBadForm, sym+": "+err.Error(), setupDraft{})
			return
		}
		// The page shows effective values, so a value left as shown is not a new
		// override; one that is already an override stays.
		rt, th := v.Profile.RoadTrainBySymbol(sym), v.Settings.Thresholds[sym]
		if minPct != nil && th.MinLengthPct == nil && *minPct == rt.MinLengthPct {
			minPct = nil
		}
		if maxAge != nil && th.MaxCarAgeDays == nil && *maxAge == rt.MaxCarAgeDays {
			maxAge = nil
		}
	}
	if err := h.b.SetThreshold(sym, minPct, maxAge); err != nil {
		h.fail(w, r, vwRefused, err.Error(), setupDraft{})
		return
	}
	h.done(w, r, "threshold")
}

func (h *setupHandler) installDir(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.fail(w, r, vwBadForm, "could not read the form: "+err.Error(), setupDraft{})
		return
	}
	dir := strings.TrimSpace(r.PostFormValue("dir"))
	if err := h.b.SetInstallDir(dir); err != nil {
		h.fail(w, r, vwRefused, "The Run8 install folder was not changed: "+err.Error(), setupDraft{InstallDir: &dir})
		return
	}
	h.done(w, r, "install-dir")
}

func (h *setupHandler) server(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.fail(w, r, vwBadForm, "could not read the form: "+err.Error(), setupDraft{})
		return
	}
	raw := strings.TrimSpace(r.PostFormValue("port"))
	all := r.PostFormValue("all_interfaces") != ""
	draft := setupDraft{Port: &raw, AllIfaces: &all}
	port, err := strconv.Atoi(raw)
	if err != nil || port < 1 || port > 65535 {
		h.fail(w, r, vwBadForm, fmt.Sprintf("The port must be a number from 1 to 65535, not %q", raw), draft)
		return
	}
	if err := h.b.SetServer(port, all); err != nil {
		h.fail(w, r, vwRefused, err.Error(), draft)
		return
	}
	h.done(w, r, "server")
}

func (h *setupHandler) profile(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.fail(w, r, vwBadForm, "could not read the form: "+err.Error(), setupDraft{})
		return
	}
	id := strings.TrimSpace(r.PostFormValue("id"))
	known := false
	for _, pi := range h.b.View().Profiles {
		known = known || pi.ID == id
	}
	if !known {
		h.fail(w, r, vwBadForm, fmt.Sprintf("%q is not an installed profile", id), setupDraft{})
		return
	}
	if err := h.b.SwitchProfile(id); err != nil {
		h.fail(w, r, vwRefused, err.Error(), setupDraft{})
		return
	}
	h.done(w, r, "profile")
}
