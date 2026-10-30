package web

import (
	"fmt"
	"io/fs"
	"net/http"
	"strings"
	"unicode"
)

// vwPages registers the embedded page templates matching name. A parse failure is
// a bug in the shipped templates, so it panics at start-up like an invalid route.
func vwPages(s *Server, name string) {
	sub, err := fs.Sub(assets, "templates")
	if err != nil {
		panic("web: embedded templates: " + err.Error())
	}
	if err := s.AddPages(sub, name); err != nil {
		panic(err)
	}
}

// vwFinish ends a successful POST. A fragment request (the page script) gets the
// refreshed page directly with a toast; anything else is redirected to the page.
func vwFinish(s *Server, w http.ResponseWriter, r *http.Request, page, to, toast string, data func() any) {
	if IsFragment(r) {
		SetToast(w, toast)
		s.Render(w, r, page, data())
		return
	}
	http.Redirect(w, r, to, http.StatusSeeOther)
}

// vwFail ends a rejected POST with a 4xx status and a visible message. A fragment
// request gets only the message, because the page script shows the response text
// as a toast and keeps the page as it is; anything else gets the whole page with
// the message in a banner.
func vwFail(s *Server, w http.ResponseWriter, r *http.Request, status int, page, msg string, data func(msg string) any) {
	if IsFragment(r) {
		h := w.Header()
		h.Set("Content-Type", "text/html; charset=utf-8")
		h.Set("Cache-Control", "no-store")
		w.WriteHeader(status)
		fmt.Fprintf(w, `<p role="alert">%s</p>`, vwTextEscaper.Replace(msg))
		return
	}
	s.RenderStatus(w, r, status, page, data(msg))
}

// vwTextEscaper escapes what must be escaped in a text node. Quotes are left alone: the
// page script shows the response text as a toast without decoding entities.
var vwTextEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")

// vwBadForm is the status for a form the user filled in wrongly; vwRefused is the
// status for a well-formed request the backend turned down.
const (
	vwBadForm = http.StatusBadRequest
	vwRefused = http.StatusUnprocessableEntity
)

// vwClean trims s and collapses runs of white space.
func vwClean(s string) string { return strings.Join(strings.Fields(s), " ") }

// vwNaturalLess orders names so that "Bowl 2" sorts before "Bowl 10".
func vwNaturalLess(a, b string) bool {
	ra, rb := []rune(a), []rune(b)
	i, j := 0, 0
	for i < len(ra) && j < len(rb) {
		if unicode.IsDigit(ra[i]) && unicode.IsDigit(rb[j]) {
			si := i
			for i < len(ra) && unicode.IsDigit(ra[i]) {
				i++
			}
			sj := j
			for j < len(rb) && unicode.IsDigit(rb[j]) {
				j++
			}
			na := strings.TrimLeft(string(ra[si:i]), "0")
			nb := strings.TrimLeft(string(rb[sj:j]), "0")
			if len(na) != len(nb) {
				return len(na) < len(nb)
			}
			if na != nb {
				return na < nb
			}
			continue
		}
		ca, cb := unicode.ToLower(ra[i]), unicode.ToLower(rb[j])
		if ca != cb {
			return ca < cb
		}
		i++
		j++
	}
	if len(ra)-i != len(rb)-j {
		return len(ra)-i < len(rb)-j
	}
	return a < b
}

// vwPercent renders part of whole as a whole percent, "<1%" for a small non-zero share.
func vwPercent(part, whole int) string {
	if whole <= 0 || part <= 0 {
		return "0%"
	}
	p := float64(part) * 100 / float64(whole)
	if p < 1 {
		return "<1%"
	}
	return fmt.Sprintf("%.0f%%", p)
}
