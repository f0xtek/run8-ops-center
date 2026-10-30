package web

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"run8-ops-center/internal/carstatus"
	"run8-ops-center/internal/engine"
	"run8-ops-center/internal/orders"
	"run8-ops-center/internal/profile"
)

// dashAttn is one entry of the attention list.
type dashAttn struct {
	Kind     string // "warning", "profile", "unmapped", "unknown-tag", "unknown-type"
	Label    string
	Text     string
	Time     time.Time
	Link     string
	LinkText string
}

// dashBuild is one road train's build progress.
type dashBuild struct {
	Symbol   string
	Route    string // "NOR to New Orleans, LA"
	Waiting  string // "5,963 of 8,000 ft"
	Pct      string // "75%"
	Need     string // "needs 50%"
	Ready    bool
	Bar      ptBar
	OrderURL string // the available order, once there is one
}

// dashPage is the dashboard.
type dashPage struct {
	Notice      string
	Empty       bool // no snapshot has been read yet
	Source      engine.Source
	Tiles       []ptTile
	Detected    []ordRow
	InProgress  []ordRow
	Local       []ordRow
	Road        []ordRow
	Building    []dashBuild
	Idle        []dashBuild // trains with no cars waiting
	IdleNames   string
	Attention   []dashAttn
	ProfileName string
}

// RegisterDashboard registers the dashboard at "/", replacing the infrastructure's
// placeholder page. Its order buttons post to the routes RegisterOrders adds.
func RegisterDashboard(s *Server, b engine.Backend) {
	dashRegisterPages(s)
	s.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		s.Render(w, r, "dash_home", dashPageData(dashViewOf(b), ""))
	})
}

func dashPageData(v *engine.View, notice string) *dashPage {
	const next = "/"
	p := &dashPage{
		Notice:    notice,
		Empty:     !v.HasSnapshot,
		Source:    v.Source,
		Tiles:     dashTiles(v),
		Attention: dashAttention(v),
	}
	if v.Profile != nil {
		p.ProfileName = v.Profile.Name
	}
	p.Detected = ordRows(v, v.Active, next, orders.StateCompletionDetected)
	p.InProgress = ordRows(v, v.Active, next, orders.StateInProgress)
	for i := range v.Available {
		o := &v.Available[i]
		if o.State != orders.StateAvailable {
			continue
		}
		if o.Kind == orders.KindRoad {
			p.Road = append(p.Road, ordRowOf(v, o, next))
		} else {
			p.Local = append(p.Local, ordRowOf(v, o, next))
		}
	}
	var idle []string
	for _, rb := range v.Building {
		b := dashBuildOf(v, rb)
		if rb.WaitingFt > 0 || rb.Ready {
			p.Building = append(p.Building, b)
		} else {
			p.Idle = append(p.Idle, b)
			idle = append(idle, rb.Symbol)
		}
	}
	p.IdleNames = strings.Join(idle, ", ")
	return p
}

func dashTiles(v *engine.View) []ptTile {
	t := v.Tiles
	bad := t.Unmapped + t.Unknown
	badNote := ""
	switch {
	case t.Unmapped > 0 && t.Unknown > 0:
		badNote = fmt.Sprintf("%d unmapped, %d unknown", t.Unmapped, t.Unknown)
	case t.Unmapped > 0:
		badNote = "on unmapped track"
	case t.Unknown > 0:
		badNote = "unknown tag or type"
	}
	tiles := []ptTile{
		{Value: fmt.Sprint(t.OnRoute), Label: "Cars on route"},
		{Value: fmt.Sprint(t.AtIndustries), Label: "At industries"},
		{Value: fmt.Sprint(t.ReadyToPull), Label: "Ready to pull", Attn: t.ReadyToPull > 0},
		{Value: fmt.Sprint(t.AwaitingDelivery), Label: "Awaiting delivery"},
		{Value: fmt.Sprint(t.AwaitingTrain), Label: "Awaiting road trains", Attn: t.AwaitingTrain > 0},
		{Value: fmt.Sprint(bad), Label: "Unmapped or unknown", Attn: bad > 0, Note: badNote},
	}
	if t.Unmapped > 0 {
		tiles[5].Href = "/learn"
	} else if t.Unknown > 0 {
		tiles[5].Href = "/#attention"
	}
	return tiles
}

func dashBuildOf(v *engine.View, rb orders.RoadBuild) dashBuild {
	b := dashBuild{Symbol: rb.Symbol, Ready: rb.Ready}
	switch {
	case rb.Origin != "" && rb.Destination != "":
		b.Route = rb.Origin + " to " + rb.Destination
	case rb.Destination != "":
		b.Route = "to " + rb.Destination
	}
	if rb.MaxLengthFt > 0 {
		b.Waiting = ptFt(rb.WaitingFt) + " of " + ptFt(float64(rb.MaxLengthFt))
		b.Pct = fmt.Sprintf("%.0f%%", rb.Pct)
		b.Bar.Fill = ptCSSPct(rb.Pct)
		if rb.ThresholdPct > 0 {
			b.Bar.Mark = ptCSSPct(rb.ThresholdPct)
		}
	} else {
		b.Waiting = ptFt(rb.WaitingFt) + ", no length limit"
		b.Bar.Fill = ptCSSPct(0)
	}
	if rb.ThresholdPct > 0 {
		b.Need = fmt.Sprintf("needs %.0f%%", rb.ThresholdPct)
	} else {
		b.Need = "any length"
	}
	b.Bar.Label = fmt.Sprintf("%s: %s, %s", rb.Symbol, b.Waiting, b.Need)
	for i := range v.Available {
		o := &v.Available[i]
		if o.Kind == orders.KindRoad && (o.Symbol == rb.Symbol || o.JobID == rb.Symbol) {
			b.OrderURL = ordHref(o.ID)
			break
		}
	}
	return b
}

// dashList joins at most max items with commas and counts the rest.
func dashList(items []string, max int) string {
	if len(items) <= max {
		return strings.Join(items, ", ")
	}
	return fmt.Sprintf("%s and %d more", strings.Join(items[:max], ", "), len(items)-max)
}

func dashSorted(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// dashAttention builds the attention list: ingest warnings, profile validation
// problems, unmapped cars and cars with an unknown tag or type. The header's warning
// count is its length.
func dashAttention(v *engine.View) []dashAttn {
	var out []dashAttn
	for _, w := range v.Warnings {
		out = append(out, dashAttn{Kind: "warning", Label: w.Kind, Text: w.Message, Time: w.Time})
	}
	if v.Profile != nil {
		for _, pr := range v.Profile.Report.Problems {
			out = append(out, dashAttn{
				Kind: "profile", Label: pr.Code, Text: pr.Message,
				Link: "/setup#profile-report", LinkText: "Profile report",
			})
		}
	}
	if n := v.Tiles.Unmapped; n > 0 {
		out = append(out, dashAttn{
			Kind: "unmapped", Label: "unmapped",
			Text: fmt.Sprintf("%s on track sections that have no label. Label them so orders can find them.", ptPlural(n, "car is", "cars are")),
			Link: "/learn", LinkText: "Open learn mode",
		})
	}
	if v.Status != nil {
		tags, models := map[string]bool{}, map[string]bool{}
		var tagCars, typeCars int
		for i := range v.Status.Cars {
			c := &v.Status.Cars[i]
			if c.Dest.Kind == carstatus.DestUnknown {
				tagCars++
				tags[c.Dest.Tag] = true
			}
			if c.Type == profile.Unknown {
				typeCars++
				models[c.Model] = true
			}
		}
		if tagCars > 0 {
			out = append(out, dashAttn{
				Kind: "unknown-tag", Label: "unknown tag",
				Text: fmt.Sprintf("%s a destination tag the profile does not know: %s.",
					ptPlural(tagCars, "car has", "cars have"), dashList(dashSorted(tags), 6)),
				Link: "/setup#profile-report", LinkText: "Profile report",
			})
		}
		if typeCars > 0 {
			out = append(out, dashAttn{
				Kind: "unknown-type", Label: "unknown type",
				Text: fmt.Sprintf("%s a model that matches no car type: %s.",
					ptPlural(typeCars, "car has", "cars have"), dashList(dashSorted(models), 6)),
				Link: "/setup#profile-report", LinkText: "Profile report",
			})
		}
	}
	return out
}

// dashViewOf returns the backend's current View, or an empty one if it has none.
func dashViewOf(b engine.Backend) *engine.View {
	if v := b.View(); v != nil {
		return v
	}
	return &engine.View{}
}
