package web

import (
	"net/http"
	"slices"
	"time"

	"run8-ops-center/internal/engine"
	"run8-ops-center/internal/store"
)

// RegisterActivity serves the activity log at /activity.
func RegisterActivity(s *Server, b engine.Backend) {
	vwPages(s, "activity.html")
	s.HandleFunc("GET /activity", func(w http.ResponseWriter, r *http.Request) {
		s.Render(w, r, "activity", activityBuild(b.View()))
	})
}

// activityShown is the most entries one page lists.
const activityShown = 500

type activityPage struct {
	Entries []activityRow
	Total   int // entries in the log, which may exceed those shown
}

type activityRow struct {
	Time    string
	ISO     string
	Game    string // in-game date, "" when the entry has none
	Kind    string
	Message string
}

func activityBuild(v *engine.View) activityPage {
	log := slices.Clone(v.Activity)
	// The engine publishes newest first; sorting again makes the page correct
	// whatever order the backend hands over. Equal times keep the given order.
	slices.SortStableFunc(log, func(a, b store.ActivityEntry) int { return b.Time.Compare(a.Time) })
	p := activityPage{Total: len(log)}
	if len(log) > activityShown {
		log = log[:activityShown]
	}
	for _, e := range log {
		row := activityRow{
			Time: e.Time.Local().Format("2006-01-02 15:04:05"), ISO: e.Time.UTC().Format(time.RFC3339),
			Kind: e.Kind, Message: e.Message,
		}
		if !e.Game.IsZero() {
			row.Game = e.Game.Format("2006-01-02 15:04")
		}
		p.Entries = append(p.Entries, row)
	}
	return p
}
