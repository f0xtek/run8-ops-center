package web

import (
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"sort"
	"strings"

	"run8-ops-center/internal/carstatus"
	"run8-ops-center/internal/engine"
	"run8-ops-center/internal/profile"
	"run8-ops-center/internal/trackmap"
)

// RegisterYards serves the yards and industries view at /yards and the
// departure track toggle at POST /yards/departure.
func RegisterYards(s *Server, b engine.Backend) {
	vwPages(s, "yards.html")
	h := &yardsHandler{s: s, b: b}
	s.HandleFunc("GET /yards", h.page)
	s.HandleFunc("POST /yards/departure", h.departure)
}

type yardsHandler struct {
	s *Server
	b engine.Backend
}

func (h *yardsHandler) page(w http.ResponseWriter, r *http.Request) {
	p := yardsBuild(h.b.View())
	q := r.URL.Query()
	if q.Get("departure") != "" {
		p.Notice = yardsNotice(q.Get("yard"), q.Get("track"), q.Get("departure") == "on")
	}
	h.s.Render(w, r, "yards", p)
}

// yardsNotice is the message shown after a departure mark changed.
func yardsNotice(yard, track string, on bool) string {
	if on {
		return fmt.Sprintf("%s / %s is now a departure track. Available orders were recomputed.", yard, track)
	}
	return fmt.Sprintf("%s / %s is no longer a departure track. Available orders were recomputed.", yard, track)
}

func (h *yardsHandler) fail(w http.ResponseWriter, r *http.Request, status int, msg string) {
	vwFail(h.s, w, r, status, "yards", msg, func(m string) any {
		p := yardsBuild(h.b.View())
		p.Error = m
		return p
	})
}

// departure handles a mark or unmark. The form posts the yard, the track and
// on=true or on=false.
func (h *yardsHandler) departure(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.fail(w, r, vwBadForm, "could not read the form: "+err.Error())
		return
	}
	yard, track := vwClean(r.PostFormValue("yard")), vwClean(r.PostFormValue("track"))
	if yard == "" || track == "" {
		h.fail(w, r, vwBadForm, "a departure mark needs a yard and a track")
		return
	}
	var on bool
	switch r.PostFormValue("on") {
	case "true":
		on = true
	case "false":
	default:
		h.fail(w, r, vwBadForm, fmt.Sprintf("%q is not true or false", r.PostFormValue("on")))
		return
	}
	if err := h.b.SetDeparture(yard, track, on); err != nil {
		h.fail(w, r, vwRefused, err.Error())
		return
	}
	notice := yardsNotice(yard, track, on)
	state := "off"
	if on {
		state = "on"
	}
	to := "/yards?" + url.Values{"departure": {state}, "yard": {yard}, "track": {track}}.Encode() + "#yard-" + url.PathEscape(yard)
	vwFinish(h.s, w, r, "yards", to, notice, func() any {
		p := yardsBuild(h.b.View())
		p.Notice = notice
		return p
	})
}

type yardsPage struct {
	HasProfile  bool
	HasSnapshot bool
	Unmapped    int
	Notice      string // what was just saved, from the query of the redirect
	Error       string
	Yards       []yardsYard
	Groups      []yardsGroup
}

type yardsYard struct {
	Code      string
	Name      string
	NotInProf bool   // a track is labelled for this code but the profile has no such yard
	Rule      string // which tracks feed orders; "" for a yard the profile lacks
	Cars      int
	Tracks    []yardsTrack
	Listed    []yardsListed // every track the profile lists for the yard
	Labelled  int           // how many of Listed are labelled
}

type yardsTrack struct {
	Name string
	Cars []yardsCar
	Dep  yardsDep
}

// yardsListed is one track a profile yard lists, with what the labels say about it.
type yardsListed struct {
	Name     string
	Kind     string // "" for a yard track, else "siding" or "main line"
	ListedFt string // "710 ft", "" when the profile gives no length
	Measured string // from the labelled sections, "" when there are none or no route data
	Labelled bool
	Cars     int
	Dep      yardsDep
}

// yardsDep is the departure-track state of one yard track and what its toggle
// form posts. Show is false for a track with no toggle.
type yardsDep struct {
	Show    bool
	Yard    string
	Track   string
	On      bool // the track is a departure track
	Changed bool // the state differs from the profile
}

type yardsCar struct {
	Number  string
	Type    string
	Loaded  bool
	Dest    string
	Service string // readable service state
	State   string // chip class
}

// yardsGroup is the industries one local job serves.
type yardsGroup struct {
	ID         string
	Symbol     string // "" for industries no local job serves
	Title      string
	Home       string
	Industries []yardsIndustry
}

type yardsIndustry struct {
	Tag       string
	Name      string
	Cars      []yardsCar
	Occupancy []yardsOccupancy
	Services  []yardsCount
}

// yardsOccupancy is the cars of one type at an industry against its capacity.
type yardsOccupancy struct {
	Type        string
	Count       int
	Capacity    int
	HasCapacity bool
	Pct         int // fill, capped at 100, for the bar
	Over        bool
}

type yardsCount struct {
	Label string
	Count int
}

var yardsServiceLabels = map[carstatus.Service]string{
	carstatus.ServiceAwaitingDelivery: "awaiting delivery",
	carstatus.ServiceWorking:          "working",
	carstatus.ServiceReadyToPull:      "ready to pull",
	carstatus.ServiceAwaitingTrain:    "awaiting train",
	carstatus.ServiceEnRoute:          "en route",
	carstatus.ServiceOther:            "other",
}

// yardsServiceOrder is the order service counts are listed in.
var yardsServiceOrder = []carstatus.Service{
	carstatus.ServiceReadyToPull, carstatus.ServiceWorking, carstatus.ServiceAwaitingDelivery,
	carstatus.ServiceAwaitingTrain, carstatus.ServiceEnRoute, carstatus.ServiceOther,
}

func yardsChip(sv carstatus.Service) string {
	switch sv {
	case carstatus.ServiceReadyToPull:
		return "ready"
	case carstatus.ServiceWorking:
		return "off"
	default:
		return "wait"
	}
}

func yardsMakeCar(c *carstatus.Car) yardsCar {
	label := yardsServiceLabels[c.Service]
	if label == "" {
		label = string(c.Service)
	}
	return yardsCar{
		Number:  c.Number,
		Type:    string(c.Type),
		Loaded:  c.Loaded,
		Dest:    c.Dest.Tag,
		Service: label,
		State:   yardsChip(c.Service),
	}
}

// yardsRule says which cars of a yard feed orders, given how many departure
// tracks the yard has once the user's marks are applied.
func yardsRule(departureTracks int) string {
	switch departureTracks {
	case 0:
		return "Orders take cars from every yard track."
	case 1:
		return "Orders take cars from its 1 departure track only."
	}
	return fmt.Sprintf("Orders take cars from its %d departure tracks only.", departureTracks)
}

// yardsDepOf describes the departure state of one track of a profile yard. A
// listed siding or main line has no toggle, because a car on one is never in
// the yard. The yard's departure set is passed in, so it is built once.
func yardsDepOf(y *profile.Yard, set map[string]bool, name string) yardsDep {
	t, listed := y.TrackByName(name)
	if listed && t.Kind != profile.TrackYard {
		return yardsDep{}
	}
	d := yardsDep{Show: true, Yard: y.Code, Track: name, On: set[profile.TrackKey(name)]}
	d.Changed = d.On != (listed && t.Departure)
	return d
}

func yardsBuild(v *engine.View) yardsPage {
	p := yardsPage{HasProfile: v.Profile != nil, HasSnapshot: v.Status != nil}
	if v.Profile == nil {
		return p
	}
	prof := v.Profile
	var cars []carstatus.Car
	if v.Status != nil {
		cars = v.Status.Cars
		p.Unmapped = v.Status.Unmapped
	}

	// Yard tracks come from the labels as well as the cars, so a labelled track
	// with no cars is listed as empty.
	type trackKey struct{ yard, track string }
	byTrack := map[trackKey][]yardsCar{}
	yardTracks := map[string][]string{}
	addTrack := func(yard, track string) {
		k := trackKey{yard, track}
		if _, ok := byTrack[k]; !ok {
			byTrack[k] = nil
			yardTracks[yard] = append(yardTracks[yard], track)
		}
	}
	for _, g := range v.Locations {
		if g.Location.Kind == trackmap.KindYard {
			addTrack(g.Location.Yard, g.Location.Track)
		}
	}
	for i := range cars {
		c := &cars[i]
		if c.Loc.Kind != carstatus.LocYard {
			continue
		}
		addTrack(c.Loc.Yard, c.Loc.Track)
		k := trackKey{c.Loc.Yard, c.Loc.Track}
		byTrack[k] = append(byTrack[k], yardsMakeCar(c))
	}
	known := map[string]bool{}
	mkYard := func(code, name string, notIn bool) yardsYard {
		y := yardsYard{Code: code, Name: name, NotInProf: notIn}
		names := slices.Clone(yardTracks[code])
		sort.SliceStable(names, func(i, j int) bool { return vwNaturalLess(names[i], names[j]) })
		for _, n := range names {
			t := yardsTrack{Name: n, Cars: byTrack[trackKey{code, n}]}
			y.Cars += len(t.Cars)
			y.Tracks = append(y.Tracks, t)
		}
		return y
	}
	for i := range prof.Yards {
		y := &prof.Yards[i]
		known[y.Code] = true
		yy := mkYard(y.Code, y.Name, false)
		set := y.DepartureSet()
		yy.Rule = yardsRule(len(set))
		for j := range yy.Tracks {
			yy.Tracks[j].Dep = yardsDepOf(y, set, yy.Tracks[j].Name)
		}
		yy.Listed = yardsListedTracks(*y, yy.Tracks, v.LocationFt)
		for _, e := range yy.Listed {
			if e.Labelled {
				yy.Labelled++
			}
		}
		p.Yards = append(p.Yards, yy)
	}
	var extra []string
	for code := range yardTracks {
		if !known[code] {
			extra = append(extra, code)
		}
	}
	sort.Strings(extra)
	for _, code := range extra {
		p.Yards = append(p.Yards, mkYard(code, "", true))
	}

	// A car at an industry belongs to one industry. When its track carries several
	// tags it goes to the industry it is tagged for, else the first tag, so a car is
	// never counted against two capacities.
	atIndustry := map[string][]*carstatus.Car{}
	for i := range cars {
		c := &cars[i]
		if c.Loc.Kind != carstatus.LocIndustry || len(c.Loc.Industries) == 0 {
			continue
		}
		tag := c.Loc.Industries[0]
		if slices.Contains(c.Loc.Industries, c.Dest.Industry) {
			tag = c.Dest.Industry
		}
		atIndustry[tag] = append(atIndustry[tag], c)
	}
	mkIndustry := func(ind *profile.Industry) yardsIndustry {
		return yardsBuildIndustry(ind, atIndustry[ind.Tag])
	}
	listed := map[string]bool{}
	for i := range prof.LocalJobs {
		j := &prof.LocalJobs[i]
		g := yardsGroup{ID: j.ID, Symbol: j.Symbol, Title: j.Symbol}
		if y := prof.YardByCode(j.HomeYard); y != nil {
			g.Home = y.Name
		} else {
			g.Home = j.HomeYard
		}
		for _, tag := range j.Industries {
			if ind := prof.IndustryByTag(tag); ind != nil {
				g.Industries = append(g.Industries, mkIndustry(ind))
				listed[ind.Tag] = true
			}
		}
		if len(g.Industries) > 0 {
			p.Groups = append(p.Groups, g)
		}
	}
	rest := yardsGroup{ID: "other", Title: "Not served by a local job"}
	for i := range prof.Industries {
		if !listed[prof.Industries[i].Tag] {
			rest.Industries = append(rest.Industries, mkIndustry(&prof.Industries[i]))
		}
	}
	if len(rest.Industries) > 0 {
		p.Groups = append(p.Groups, rest)
	}
	return p
}

// yardsListedTracks pairs every track the profile lists for y with the labelled
// track of the same name, ignoring case and extra spaces. A labelled track shows
// its car count and, when route data measured it, its measured length.
func yardsListedTracks(y profile.Yard, labelled []yardsTrack, measuredFt map[string]int) []yardsListed {
	if len(y.Tracks) == 0 {
		return nil
	}
	set := y.DepartureSet()
	byName := make(map[string]yardsTrack, len(labelled))
	for _, t := range labelled {
		byName[strings.ToLower(profile.NormTag(t.Name))] = t
	}
	out := make([]yardsListed, 0, len(y.Tracks))
	for _, lt := range y.Tracks {
		e := yardsListed{Name: lt.Name, Dep: yardsDepOf(&y, set, lt.Name)}
		switch lt.Kind {
		case profile.TrackSiding:
			e.Kind = "siding"
		case profile.TrackMain:
			e.Kind = "main line"
		}
		if lt.LengthFt > 0 {
			e.ListedFt = ptGroup(lt.LengthFt) + " ft"
		}
		if t, ok := byName[strings.ToLower(profile.NormTag(lt.Name))]; ok {
			e.Labelled, e.Cars = true, len(t.Cars)
			key := trackmap.Location{Kind: trackmap.KindYard, Yard: y.Code, Track: t.Name}.Key()
			if ft := measuredFt[key]; ft > 0 {
				e.Measured = ptGroup(ft) + " ft"
			}
		}
		out = append(out, e)
	}
	return out
}

func yardsBuildIndustry(ind *profile.Industry, cars []*carstatus.Car) yardsIndustry {
	out := yardsIndustry{Tag: ind.Tag, Name: ind.Name}
	counts := map[profile.CarType]int{}
	service := map[carstatus.Service]int{}
	for _, c := range cars {
		out.Cars = append(out.Cars, yardsMakeCar(c))
		counts[c.Type]++
		service[c.Service]++
	}
	for _, sv := range yardsServiceOrder {
		if n := service[sv]; n > 0 {
			out.Services = append(out.Services, yardsCount{yardsServiceLabels[sv], n})
		}
	}
	// Every type the industry handles, in file order, then types parked there that
	// it has no capacity for.
	seen := map[profile.CarType]bool{}
	for _, row := range ind.Cars {
		if seen[row.CarType] {
			continue
		}
		seen[row.CarType] = true
		capacity, _ := ind.Capacity(row.CarType)
		out.Occupancy = append(out.Occupancy, yardsOcc(string(row.CarType), counts[row.CarType], capacity, true))
	}
	var extra []string
	for t := range counts {
		if !seen[t] {
			extra = append(extra, string(t))
		}
	}
	sort.Strings(extra)
	for _, t := range extra {
		out.Occupancy = append(out.Occupancy, yardsOcc(t, counts[profile.CarType(t)], 0, false))
	}
	return out
}

func yardsOcc(typ string, n, capacity int, has bool) yardsOccupancy {
	o := yardsOccupancy{Type: typ, Count: n, Capacity: capacity, HasCapacity: has}
	if has && capacity > 0 {
		o.Pct = min(100, n*100/capacity)
		o.Over = n > capacity
	}
	return o
}
