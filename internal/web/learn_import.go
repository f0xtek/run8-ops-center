package web

import (
	"errors"
	"fmt"
	"net/http"

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

// importGroup is one location's share of the diff.
type importGroup struct {
	Name                  string
	Kind                  string
	New, Changed, Removed []importEntry
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
		page.Groups = append(page.Groups, importGroup{
			Name: g.Location.String(), Kind: string(g.Location.Kind),
			New: importEntries(g.New), Changed: importEntries(g.Changed), Removed: importEntries(g.Removed),
		})
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
