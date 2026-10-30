package web

import (
	"cmp"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"run8-ops-center/internal/engine"
	"run8-ops-center/internal/profile"
	"run8-ops-center/internal/run8route"
	"run8-ops-center/internal/trackmap"
)

// RegisterLearn serves the learn-mode view at /learn and its label actions:
// POST /trackmap/label, /trackmap/rename and /trackmap/remove. The import from
// Run8 adds POST /trackmap/import/preview, which renders the preview page, and
// POST /trackmap/import/confirm.
func RegisterLearn(s *Server, b engine.Backend) {
	vwPages(s, "learn.html")
	vwPages(s, "import.html")
	h := &learnHandler{s: s, b: b}
	s.HandleFunc("GET /learn", h.page)
	s.HandleFunc("POST /trackmap/label", h.label)
	s.HandleFunc("POST /trackmap/rename", h.rename)
	s.HandleFunc("POST /trackmap/remove", h.remove)
	s.HandleFunc("POST /trackmap/import/preview", h.importPreview)
	s.HandleFunc("POST /trackmap/import/confirm", h.importConfirm)
}

type learnHandler struct {
	s *Server
	b engine.Backend
}

// learnSectionsShown is how many sections a candidate lists before "and N more".
const learnSectionsShown = 12

type learnPage struct {
	HasSnapshot bool
	HasProfile  bool
	Cars        int
	Unmapped    int
	Share       string
	Import      learnImport
	Candidates  []learnCandidate
	Groups      []learnGroup
	Sections    int // labelled sections in total
	Yards       []profile.Yard
	Industries  []learnIndustry
	Edit        *learnEdit
	Error       string
}

// learnImport is the "Import from Run8" card.
type learnImport struct {
	Configured bool   // the profile names Run8 folders
	Status     string // engine.Run8Ready, Run8Partial or Run8Unavailable
	Reason     string // why Status is unavailable
	Files      []learnFile
	Last       string // when the last import was made, "" for never
	Changed    bool   // the Run8 files differ from the last import
	Available  bool   // readable data and no import yet
	CanPreview bool
}

// learnFile is a Run8 route file that could not be used.
type learnFile struct{ Name, Path, Reason string }

func learnFiles(list []run8route.FileStatus) []learnFile {
	out := make([]learnFile, len(list))
	for i, f := range list {
		out[i] = learnFile{Name: f.Name, Path: f.Path, Reason: dashFileReason(f)}
	}
	return out
}

type learnIndustry struct{ Tag, Name string }

type learnCandidate struct {
	TrainID     string
	Title       string
	Locos       bool // no cars, locomotives only
	Cars        int
	Samples     []string
	Tags        []trackmap.TagCount
	Sections    []trackmap.SectionRef // every unmapped section it occupies
	Shown       []trackmap.SectionRef
	More        int
	Track       []trackmap.SectionRef // every section a label will cover: Sections plus the rest of its track
	Extra       []trackmap.SectionRef // Track without Sections
	ExtraShown  []trackmap.SectionRef
	ExtraMore   int
	Suggestion  *learnSuggestion
	prefillForm learnFields
}

type learnSuggestion struct {
	Text       string // the suggested label, readable
	Reason     string
	Label      learnFields
	NeedsTrack bool // a yard with no track name: it pre-fills the form and has no one-click accept
}

// learnFields are the form values that make up a label.
type learnFields struct {
	Kind       string
	Yard       string
	Track      string
	Industries string // comma separated tags
}

type learnGroup struct {
	Name      string
	Kind      string
	CanRename bool // not the ignored group
	Key       string
	Sections  []learnSection
}

// learnSection is one labelled section of a group.
type learnSection struct {
	Key      string // "prefix/section"
	Imported bool   // the label came from the Run8 import
}

// learnEdit is the card at the top of the page for relabelling one section or
// renaming one location.
type learnEdit struct {
	Rename  bool
	Title   string
	Section string // "prefix/section", relabel only
	Old     learnFields
	Fields  learnFields
}

func learnFieldsOf(l trackmap.Label) learnFields {
	return learnFields{Kind: string(l.Kind), Yard: l.Yard, Track: l.Track, Industries: strings.Join(l.Industries, ", ")}
}

func learnBuild(v *engine.View, q url.Values, errMsg string) learnPage {
	p := learnPage{HasSnapshot: v.HasSnapshot, HasProfile: v.Profile != nil, Error: errMsg}
	p.Cars, p.Unmapped = v.Coverage.Cars, v.Coverage.Unmapped
	p.Share = vwPercent(p.Unmapped, p.Cars)
	if v.Profile != nil {
		p.Yards = v.Profile.Yards
		for _, in := range v.Profile.Industries {
			p.Industries = append(p.Industries, learnIndustry{in.Tag, in.Name})
		}
	}
	p.Import = learnImportOf(v)
	for _, c := range v.Candidates {
		lc := learnCandidate{
			TrainID: c.TrainID, Cars: c.Cars, Samples: c.Samples, Tags: c.Tags,
			Sections: c.Sections, Locos: c.Cars == 0,
		}
		switch {
		case !c.Powered:
			lc.Title = "Cut"
		case c.Symbol != "":
			lc.Title = "Train " + c.Symbol
		default:
			lc.Title = "Train"
		}
		lc.Shown = c.Sections
		if len(lc.Shown) > learnSectionsShown {
			lc.Shown = lc.Shown[:learnSectionsShown]
			lc.More = len(c.Sections) - learnSectionsShown
		}
		lc.Track = c.Track
		if len(lc.Track) == 0 {
			lc.Track = c.Sections
		}
		for _, s := range lc.Track {
			if !slices.Contains(c.Sections, s) {
				lc.Extra = append(lc.Extra, s)
			}
		}
		lc.ExtraShown = lc.Extra
		if len(lc.ExtraShown) > learnSectionsShown {
			lc.ExtraShown = lc.ExtraShown[:learnSectionsShown]
			lc.ExtraMore = len(lc.Extra) - learnSectionsShown
		}
		if sg := c.Suggestion; sg != nil {
			lc.Suggestion = &learnSuggestion{
				Text: learnLabelText(sg.Label), Reason: sg.Reason, Label: learnFieldsOf(sg.Label),
				NeedsTrack: sg.NeedsTrackName,
			}
			if sg.NeedsTrackName {
				lc.prefillForm = learnFieldsOf(sg.Label)
			}
		}
		p.Candidates = append(p.Candidates, lc)
	}
	// Cuts and trains with cars come first: they are what the user is here to label.
	// Locomotive-only trains follow, in the order the snapshot lists them.
	slices.SortStableFunc(p.Candidates, func(a, b learnCandidate) int { return cmp.Compare(learnBool(a.Locos), learnBool(b.Locos)) })
	for _, g := range v.Locations {
		lg := learnGroup{
			Name: g.Location.String(), Kind: string(g.Location.Kind),
			CanRename: g.Location.Kind != trackmap.KindIgnored, Key: g.Location.Key(),
		}
		for _, s := range g.Sections {
			ls := learnSection{Key: s.Key()}
			if v.TrackMap != nil {
				l, ok := v.TrackMap.Lookup(s.Prefix, s.Section)
				ls.Imported = ok && l.Source == trackmap.SourceRun8
			}
			lg.Sections = append(lg.Sections, ls)
		}
		p.Sections += len(g.Sections)
		p.Groups = append(p.Groups, lg)
	}
	if key := q.Get("relabel"); key != "" {
		if prefix, section, err := trackmap.ParseKey(key); err == nil && v.TrackMap != nil {
			if l, ok := v.TrackMap.Lookup(prefix, section); ok {
				f := learnFieldsOf(l)
				p.Edit = &learnEdit{Title: "Relabel section " + key, Section: key, Fields: f}
			}
		}
	} else if key := q.Get("rename"); key != "" {
		for _, g := range v.Locations {
			if g.Location.Key() == key && g.Location.Kind != trackmap.KindIgnored {
				f := learnFieldsOf(g.Location.Label())
				p.Edit = &learnEdit{Rename: true, Title: "Rename " + g.Location.String(), Old: f, Fields: f}
			}
		}
	}
	return p
}

// learnLabelText renders a label for display, e.g. "yard Norris / Bowl 14". A yard
// label with no track name is a suggestion that only pre-fills the form.
func learnLabelText(l trackmap.Label) string {
	switch l.Kind {
	case trackmap.KindYard:
		if l.Track == "" {
			return "yard " + l.Yard + ", with the track name left for you to enter"
		}
		return "yard track " + l.Yard + " / " + l.Track
	case trackmap.KindIndustry:
		return "industry track " + strings.Join(l.Industries, ", ")
	default:
		return "ignored (mainline or other track)"
	}
}

func (h *learnHandler) page(w http.ResponseWriter, r *http.Request) {
	h.s.Render(w, r, "learn", learnBuild(h.b.View(), r.URL.Query(), ""))
}

func (h *learnHandler) fail(w http.ResponseWriter, r *http.Request, status int, msg string) {
	vwFail(h.s, w, r, status, "learn", msg, func(m string) any {
		return learnBuild(h.b.View(), r.URL.Query(), m)
	})
}

func (h *learnHandler) done(w http.ResponseWriter, r *http.Request, toast string) {
	vwFinish(h.s, w, r, "learn", "/learn", toast, func() any { return learnBuild(h.b.View(), nil, "") })
}

// learnParseLabel reads kind, yard, track and industries from the form, with the
// given prefix on each field name ("" for the label, "old_" for a rename's source).
func learnParseLabel(r *http.Request, prefix string, prof *profile.Profile) (trackmap.Label, error) {
	if prof == nil {
		return trackmap.Label{}, errors.New("no route profile is loaded")
	}
	switch kind := trackmap.Kind(r.PostFormValue(prefix + "kind")); kind {
	case trackmap.KindIgnored:
		return trackmap.Label{Kind: kind}, nil
	case trackmap.KindYard:
		code := vwClean(r.PostFormValue(prefix + "yard"))
		if prof.YardByCode(code) == nil {
			return trackmap.Label{}, fmt.Errorf("choose a yard from the list (%q is not in the profile)", code)
		}
		track := vwClean(r.PostFormValue(prefix + "track"))
		if track == "" {
			return trackmap.Label{}, errors.New("enter a track name for the yard track")
		}
		return trackmap.Label{Kind: kind, Yard: code, Track: track}, nil
	case trackmap.KindIndustry:
		var tags []string
		for _, f := range strings.FieldsFunc(r.PostFormValue(prefix+"industries"), func(c rune) bool { return c == ',' || c == ';' || c == ' ' || c == '\t' }) {
			tag := f
			if prof.IndustryByTag(tag) == nil {
				tag = strings.ToUpper(f)
			}
			if prof.IndustryByTag(tag) == nil {
				return trackmap.Label{}, fmt.Errorf("%q is not an industry tag in the profile", f)
			}
			dup := false
			for _, t := range tags {
				dup = dup || t == tag
			}
			if !dup {
				tags = append(tags, tag)
			}
		}
		if len(tags) == 0 {
			return trackmap.Label{}, errors.New("enter at least one industry tag for the industry track")
		}
		return trackmap.Label{Kind: kind, Industries: tags}, nil
	default:
		return trackmap.Label{}, errors.New("choose what kind of track this is: yard, industry or ignored")
	}
}

func (h *learnHandler) label(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.fail(w, r, vwBadForm, "could not read the form: "+err.Error())
		return
	}
	l, err := learnParseLabel(r, "", h.b.View().Profile)
	if err != nil {
		h.fail(w, r, vwBadForm, err.Error())
		return
	}
	section, train := vwClean(r.PostFormValue("section")), vwClean(r.PostFormValue("train"))
	var toast string
	switch {
	case section != "":
		prefix, sec, perr := trackmap.ParseKey(section)
		if perr != nil {
			h.fail(w, r, vwBadForm, fmt.Sprintf("%q is not a section (expected prefix/section)", section))
			return
		}
		if err = h.b.LabelSection(prefix, sec, l); err == nil {
			toast = fmt.Sprintf("Section %s labelled as %s", section, learnLabelText(l))
		}
	case train != "":
		if err = h.b.LabelCandidate(train, l); err == nil {
			toast = "Labelled as " + learnLabelText(l)
		}
	default:
		h.fail(w, r, vwBadForm, "say which cut or section to label")
		return
	}
	if err != nil {
		h.fail(w, r, vwRefused, err.Error())
		return
	}
	h.done(w, r, toast)
}

func (h *learnHandler) rename(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.fail(w, r, vwBadForm, "could not read the form: "+err.Error())
		return
	}
	prof := h.b.View().Profile
	oldL, err := learnParseOldLocation(r)
	if err != nil {
		h.fail(w, r, vwBadForm, err.Error())
		return
	}
	newL, err := learnParseLabel(r, "", prof)
	if err != nil {
		h.fail(w, r, vwBadForm, err.Error())
		return
	}
	n, err := h.b.RenameLocation(oldL.Location(), newL.Location())
	if err != nil {
		h.fail(w, r, vwRefused, err.Error())
		return
	}
	h.done(w, r, fmt.Sprintf("Renamed %s to %s (%d section%s)", oldL.Location(), newL.Location(), n, learnPlural(n)))
}

// learnParseOldLocation reads the location being renamed. The yard is not checked
// against the profile: the labels may name a yard the profile no longer has.
func learnParseOldLocation(r *http.Request) (trackmap.Label, error) {
	l := trackmap.Label{Kind: trackmap.Kind(r.PostFormValue("old_kind"))}
	switch l.Kind {
	case trackmap.KindYard:
		l.Yard, l.Track = vwClean(r.PostFormValue("old_yard")), vwClean(r.PostFormValue("old_track"))
		if l.Yard == "" || l.Track == "" {
			return l, errors.New("the location to rename is incomplete")
		}
	case trackmap.KindIndustry:
		l.Industries = strings.FieldsFunc(r.PostFormValue("old_industries"), func(c rune) bool { return c == ',' || c == ' ' })
		if len(l.Industries) == 0 {
			return l, errors.New("the location to rename is incomplete")
		}
	default:
		return l, errors.New("only yard and industry locations can be renamed")
	}
	return l, nil
}

func (h *learnHandler) remove(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.fail(w, r, vwBadForm, "could not read the form: "+err.Error())
		return
	}
	section := vwClean(r.PostFormValue("section"))
	prefix, sec, err := trackmap.ParseKey(section)
	if err != nil {
		h.fail(w, r, vwBadForm, fmt.Sprintf("%q is not a section (expected prefix/section)", section))
		return
	}
	if err := h.b.RemoveLabel(prefix, sec); err != nil {
		h.fail(w, r, vwRefused, err.Error())
		return
	}
	h.done(w, r, "Removed the label from section "+section)
}

func learnPlural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// learnForm is what the shared label fields template renders.
type learnForm struct {
	Fields learnFields
	Yards  []profile.Yard
}

// Form pairs label values with the yards to choose from.
func (p learnPage) Form(f learnFields) learnForm { return learnForm{Fields: f, Yards: p.Yards} }

// Prefill is what the candidate's label form starts from: the suggested yard with
// the track name blank when the suggestion needs one, otherwise empty.
func (c learnCandidate) Prefill() learnFields { return c.prefillForm }

func learnBool(b bool) int {
	if b {
		return 1
	}
	return 0
}
