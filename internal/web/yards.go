package web

import (
	"net/http"
	"slices"
	"sort"

	"run8-ops-center/internal/carstatus"
	"run8-ops-center/internal/engine"
	"run8-ops-center/internal/profile"
	"run8-ops-center/internal/trackmap"
)

// RegisterYards serves the yards and industries view at /yards.
func RegisterYards(s *Server, b engine.Backend) {
	vwPages(s, "yards.html")
	s.HandleFunc("GET /yards", func(w http.ResponseWriter, r *http.Request) {
		s.Render(w, r, "yards", yardsBuild(b.View()))
	})
}

type yardsPage struct {
	HasProfile  bool
	HasSnapshot bool
	Unmapped    int
	Yards       []yardsYard
	Groups      []yardsGroup
}

type yardsYard struct {
	Code      string
	Name      string
	NotInProf bool // a track is labelled for this code but the profile has no such yard
	Cars      int
	Tracks    []yardsTrack
}

type yardsTrack struct {
	Name string
	Cars []yardsCar
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
	for _, y := range prof.Yards {
		known[y.Code] = true
		p.Yards = append(p.Yards, mkYard(y.Code, y.Name, false))
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
