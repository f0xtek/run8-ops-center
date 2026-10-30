package trackmap

import (
	"regexp"
	"slices"
	"testing"

	"run8-ops-center/internal/profile"
	"run8-ops-center/internal/run8route"
	"run8-ops-center/internal/run8route/run8routetest"
	"run8-ops-center/internal/run8save"
)

// The CI tests here run on the synthetic route in run8routetest (see the
// comment above its Sec* constants for the layout). Nothing reads a real Run8
// file.

// synthRoute writes the synthetic install and loads it.
func synthRoute(t testing.TB) run8route.Result {
	t.Helper()
	root := t.TempDir()
	run8routetest.WriteInstall(t, root, run8routetest.Synthetic())
	res := run8route.Load(root, run8routetest.Region, []run8route.Route{{Folder: run8routetest.Route}})
	for _, f := range res.Files {
		if !f.OK {
			t.Fatalf("%s: %v", f.Name, f.Err)
		}
	}
	return res
}

// synthProfile is a profile for the synthetic route. The end pattern is
// compiled as written, so it is case-sensitive and the import must match
// "South End" itself. Yards default to NOR, anchored on the milepost whose
// station is "Receiving Yard".
func synthProfile(yards ...profile.Yard) *profile.Profile {
	if len(yards) == 0 {
		yards = []profile.Yard{{Code: "NOR", Name: "Norris", AnchorMileposts: []string{"Receiving Yard"}}}
	}
	return &profile.Profile{
		Yards: yards,
		Industries: []profile.Industry{
			{Name: "Synthetic Rail & Locomotive", Tag: "BRL"},
			{Name: "Padded Tag Works", Tag: "BBB"},
			{Name: "Bridge Industries", Tag: "BRI"},
			{Name: "Gulf Export Systems", Tag: "GES"},
		},
		RoutePrefixes: []int{run8routetest.Prefix},
		Run8: &profile.Run8Config{
			Region: run8routetest.Region,
			Routes: []profile.Run8Route{{Folder: run8routetest.Route, Prefix: run8routetest.Prefix}},
			Humps: []profile.Run8Hump{
				{Controller: run8routetest.HumpControllerName, Yard: "NOR", TrackPrefix: "Bowl"},
			},
			TrackEnds: []profile.Run8TrackEnds{{
				Pattern:     regexp.MustCompile(`^Norris RCV (?P<n>\d+) (?P<end>North|South) end$`),
				Yard:        "NOR",
				TrackPrefix: "Receiving",
			}},
		},
	}
}

func sec(n int) string { return Key(run8routetest.Prefix, n) }

func norRcv(n string) Label {
	return Label{Kind: KindYard, Yard: "NOR", Track: "Receiving " + n}
}

// cutOn is an unpowered train with one car on each given section.
func cutOn(id, tag string, sections ...int) *run8save.Snapshot {
	var vs []run8save.Vehicle
	for i, s := range sections {
		vs = append(vs, car(string(rune('a'+i)), tag, s, s))
	}
	return &run8save.Snapshot{Trains: []run8save.Train{{ID: id, Vehicles: vs}}}
}

// sectionsWhere returns the sorted section numbers of the labels that satisfy pred.
func sectionsWhere(labels map[string]Label, pred func(Label) bool) []int {
	var out []int
	for k, l := range labels {
		if !pred(l) {
			continue
		}
		_, s, _ := ParseKey(k)
		out = append(out, s)
	}
	slices.Sort(out)
	return out
}

func trackIs(name string) func(Label) bool {
	return func(l Label) bool { return l.Kind == KindYard && l.Track == name }
}

func skipFor(skips []Skip, what string) (Skip, bool) {
	for _, s := range skips {
		if s.What == what {
			return s, true
		}
	}
	return Skip{}, false
}
