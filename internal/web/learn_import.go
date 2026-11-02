package web

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"run8-ops-center/internal/engine"
	"run8-ops-center/internal/trackmap"
)

// learnImportOf fills the "Import from Run8" card from the engine's Run8 state.
func learnImportOf(v *engine.View) learnImport {
	r := v.Run8
	return learnImport{
		Configured: r.HasRun8Config,
		Status:     r.Status,
		Reason:     r.Reason,
		Files:      learnFiles(r.Unavailable),
		Last:       ptWhen(r.LastImport),
		Changed:    r.Changed,
		Available:  r.ImportAvailable,
		CanPreview: r.HasRun8Config && r.Status != engine.Run8Unavailable,
	}
}

// importPage is the import preview page.
type importPage struct {
	Token       string
	Counts      importCounts
	Groups      []importGroup // locations with something to add, replace or remove
	Quiet       int           // locations the import leaves exactly as they are
	Kept        []importEntry // sections a manual label holds
	Skipped     []trackmap.Skip
	Unavailable []string
	Unusable    []learnFile
}

// PageTitle names the page in the document title.
func (importPage) PageTitle() string { return "Import preview" }

// importCounts are the preview's totals, in sections except Skipped.
type importCounts struct {
	New, Changed, Removed, KeptManual, Unchanged, Skipped int
}

// Apply is how many sections confirming adds, replaces or removes.
func (c importCounts) Apply() int { return c.New + c.Changed + c.Removed }

// importGroup is one location's share of the diff. Summary counts its new,
// changed and removed track sections in words. Length and Cars describe the
// same sections.
type importGroup struct {
	Name                  string
	Kind                  string
	New, Changed, Removed []importEntry
	Summary               string // "4 track sections, all new", "3 track sections (2 new, 1 changed)"
	Length                string // "1,217 ft", "" without route data
	Cars                  string // "No cars there now", "3 cars there now: A 1, B 2", "" with no save loaded
}

// sectionSummary counts a group's track sections and splits them by outcome.
func sectionSummary(g importGroup) string {
	total := len(g.New) + len(g.Changed) + len(g.Removed)
	s := fmt.Sprintf("%d track section%s", total, plural(total))
	var parts []string
	for _, p := range []struct {
		n    int
		word string
	}{{len(g.New), "new"}, {len(g.Changed), "changed"}, {len(g.Removed), "removed"}} {
		if p.n == total {
			return s + ", all " + p.word
		}
		if p.n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", p.n, p.word))
		}
	}
	return s + " (" + strings.Join(parts, ", ") + ")"
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// importCars words the cars standing on a location's sections. known is false
// when no save is loaded, which says nothing about cars.
func importCars(known bool, loc engine.ImportLocation) string {
	switch {
	case !known:
		return ""
	case loc.Cars == 0:
		return "No cars there now"
	}
	s := fmt.Sprintf("%d car%s there now: %s", loc.Cars, plural(loc.Cars), strings.Join(loc.CarNumbers, ", "))
	if more := loc.Cars - len(loc.CarNumbers); more > 0 {
		s += fmt.Sprintf(" and %d more", more)
	}
	return s
}

// importEntry is one section. Was and Now are the readable labels before and after.
type importEntry struct {
	Key, Was, Now string
}

func importEntries(list []trackmap.DiffEntry) []importEntry {
	out := make([]importEntry, len(list))
	for i, e := range list {
		out[i] = importEntry{Key: e.Key}
		if e.Old != nil {
			out[i].Was = e.Old.Location().String()
		}
		if e.New != nil {
			out[i].Now = e.New.Location().String()
		}
	}
	return out
}

func importBuild(p engine.ImportPreview) importPage {
	d := p.Diff
	page := importPage{
		Token: p.Token, Skipped: d.Skipped, Unavailable: d.Unavailable, Unusable: learnFiles(p.Unusable),
		Kept: importEntries(d.KeptManual),
		Counts: importCounts{
			New: len(d.New), Changed: len(d.Changed), Removed: len(d.Removed),
			KeptManual: len(d.KeptManual), Unchanged: len(d.Unchanged), Skipped: len(d.Skipped),
		},
	}
	for _, g := range d.Groups() {
		if len(g.New)+len(g.Changed)+len(g.Removed) == 0 {
			if len(g.Unchanged) > 0 {
				page.Quiet++
			}
			continue
		}
		ig := importGroup{
			Name: g.Location.String(), Kind: string(g.Location.Kind),
			New: importEntries(g.New), Changed: importEntries(g.Changed), Removed: importEntries(g.Removed),
		}
		loc := p.Locations[g.Location.Key()]
		ig.Summary = sectionSummary(ig)
		if loc.LengthFt > 0 {
			ig.Length = ptGroup(loc.LengthFt) + " ft"
		}
		ig.Cars = importCars(p.CarsKnown, loc)
		page.Groups = append(page.Groups, ig)
	}
	return page
}

// importPreview renders what an import would change. It changes no label, and
// the page is the response to the POST, so a reload asks the browser to resend it.
func (h *learnHandler) importPreview(w http.ResponseWriter, r *http.Request) {
	p, err := h.b.PreviewImport()
	if err != nil {
		h.fail(w, r, vwRefused, "The Run8 import preview could not be built: "+err.Error())
		return
	}
	h.s.Render(w, r, "import", importBuild(p))
}

// importConfirm applies the import the token's preview described. When the Run8
// files or the track map moved since, nothing is applied and the user is asked to
// review a new preview.
func (h *learnHandler) importConfirm(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.fail(w, r, vwBadForm, "could not read the form: "+err.Error())
		return
	}
	token := vwClean(r.PostFormValue("token"))
	if token == "" {
		h.fail(w, r, vwBadForm, "the preview to confirm is missing; review a new preview")
		return
	}
	if err := h.b.ConfirmImport(token); err != nil {
		if errors.Is(err, engine.ErrStaleImport) {
			h.fail(w, r, vwRefused, "The Run8 track data or the track map changed since the preview, so nothing was imported. Please review a new preview.")
			return
		}
		h.fail(w, r, vwRefused, "The Run8 import was not applied: "+err.Error())
		return
	}
	cov := h.b.View().Coverage
	toast := fmt.Sprintf("Run8 import applied. %d unmapped car%s left.", cov.Unmapped, learnPlural(cov.Unmapped))
	if IsFragment(r) {
		// The confirm form sits on the preview page; the page script moves to /learn.
		w.Header().Set("X-Navigate", "/learn")
	}
	h.done(w, r, toast)
}
