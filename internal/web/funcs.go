package web

import (
	"fmt"
	"html/template"
	"io/fs"
	"strconv"
	"strings"
	"testing/fstest"
	"time"

	"run8-ops-center/internal/orders"
)

// The pt* helpers are shared by the dashboard and order pages: car tables,
// load pills, state and flag chips, tiles and bars. Their templates live in
// templates/partials.html, which is prepended to every page source below, so
// a page can use them without registering anything else.

// ptTile is one summary tile.
type ptTile struct {
	Value string
	Label string
	Note  string // small line under the label
	Href  string // optional link on the note
	Attn  bool   // highlight: the count needs the user's attention
}

// ptBar is a horizontal bar with an optional threshold marker.
type ptBar struct {
	Fill  string // CSS width, "42.0%"
	Mark  string // CSS left offset of the threshold marker; "" for none
	Label string // accessible description
}

// ptCar is one row of a switchlist car table.
type ptCar struct {
	Number  string
	Type    string
	Loaded  bool
	Tag     string
	Where   string // current location, or the last known one when Gone
	Gone    bool   // the car is not in the current snapshot
	Missing bool
	Done    bool
	Shared  bool
}

// ptCarTable is a car table, or Empty text when there are no rows.
type ptCarTable struct {
	Rows  []ptCar
	Empty string
}

var ptFuncs = template.FuncMap{
	"ptStateLabel": ptStateLabel,
	"ptStateClass": ptStateClass,
	"ptStateStyle": ptStateStyle,
	"ptFlagLabel":  ptFlagLabel,
	"ptFlagClass":  ptFlagClass,
	"ptWhen":       ptWhen,
	"ptGame":       ptGame,
	"ptFt":         ptFt,
	"ptTons":       ptTons,
	"ptPlural":     ptPlural,
}

func ptStateLabel(s orders.State) string {
	switch s {
	case orders.StateAvailable:
		return "Available"
	case orders.StateInProgress:
		return "In progress"
	case orders.StateCompletionDetected:
		return "Completion detected"
	case orders.StateCompleted:
		return "Completed"
	case orders.StateCancelled:
		return "Cancelled"
	}
	return string(s)
}

func ptStateClass(s orders.State) string {
	switch s {
	case orders.StateCompletionDetected, orders.StateCompleted:
		return "ready"
	}
	return "wait"
}

// ptStateStyle gives an in-progress order the accent colour; the chip classes
// have no colour for it.
func ptStateStyle(s orders.State) template.CSS {
	if s == orders.StateInProgress {
		return "background:var(--accent);color:var(--accent-ink)"
	}
	return ""
}

func ptFlagLabel(code string) string {
	switch code {
	case "over-length":
		return "Over length"
	case "over-tons":
		return "Over tonnage"
	case orders.FlagMissingCar:
		return "Missing car"
	case "shared-cars":
		return "Shared cars"
	case "unmapped":
		return "Unmapped cars"
	}
	return code
}

func ptFlagClass(code string) string {
	if code == "shared-cars" {
		return "wait"
	}
	return "off"
}

// ptWhen formats a real-world time in the PC's zone; "" for the zero time.
func ptWhen(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Local().Format("2006-01-02 15:04")
}

// ptGame formats an in-game time; "" for the zero time.
func ptGame(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("2006-01-02 15:04")
}

// ptGroup writes n with thousands separators.
func ptGroup(n int) string {
	s := strconv.Itoa(n)
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	if neg {
		s = "-" + s
	}
	return s
}

func ptFt(f float64) string { return ptGroup(int(f+0.5)) + " ft" }

func ptTons(f float64) string { return ptGroup(int(f+0.5)) + " t" }

func ptPlural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}

// ptCSSPct renders a percentage, clamped to 0..100, as a CSS length.
func ptCSSPct(p float64) string {
	return strconv.FormatFloat(min(max(p, 0), 100), 'f', 1, 64) + "%"
}

// dashRegisterPages registers the template functions and the dashboard and order
// pages. It is idempotent and panics if an embedded template does not parse, which is
// a programming error caught by the tests.
func dashRegisterPages(s *Server) {
	s.Funcs(ptFuncs)
	partials, err := fs.ReadFile(assets, "templates/partials.html")
	if err != nil {
		panic("web: " + err.Error())
	}
	pages := fstest.MapFS{}
	for _, name := range []string{"dash_home", "orders", "order"} {
		src, err := fs.ReadFile(assets, "templates/"+name+".html")
		if err != nil {
			panic("web: " + err.Error())
		}
		pages[name+".html"] = &fstest.MapFile{Data: append(append([]byte{}, partials...), src...)}
	}
	if err := s.AddPages(pages, "*.html"); err != nil {
		panic("web: " + err.Error())
	}
}
