package run8routetest

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The synthetic install's names. The prefix is the track section prefix every
// section and every industry track uses.
const (
	Region = "GulfCoast"
	Route  = "NS_AGS_Phase01"
	Prefix = 280

	// TileW and TileH are the tile size planted in the synthetic track
	// database. Topology.TileSize must recover them.
	TileW = 800.0
	TileH = -1000.0
)

// Layout and section numbers of the synthetic route (see Synthetic). Every
// section has prefix 280. "SW" marks a switch, whose next list is
// [points, normal, reverse].
//
// Main line, east-west at z=-500. 101 crosses the tile edge at x=800.
//
//	100 - 101 - 102(SW) - 103 - 104   next(102) = [101, 103, 105]
//	                  \- 105 - 106    the siding; 100 and 104 are dead ends
//
// Hump lead, south of the main line. 200 crosses the tile edge at z=-1000.
//
//	200 - 201(SW) -normal-  202 - 204(SW) -normal-  206
//	              |                      -reverse- 207 - 210
//	              -reverse- 203 - 205(SW) -normal-  208
//	                                     -reverse- 209
//
//	next(201) = [200, 202, 203]  next(204) = [202, 206, 207]
//	next(205) = [203, 208, 209]
//
// Hump controller "Norris Yard Hump Controller", one path per bowl track.
// Each path lists the switches taken from the lead; the last one's leg opens
// the bowl track:
//
//	track "1": 201 normal, 204 normal   -> bowl track {206}
//	track "2": 201 normal, 204 reverse  -> bowl track {207, 210}
//	track "3": 201 reverse, 205 normal  -> bowl track {208}
//	track "4": 201 reverse, 205 reverse -> bowl track {209}
//
// Receiving yard, east of the hump. A ladder switch 340 at the north end and
// 341 at the south end serve RCV 1 and RCV 2:
//
//	next(340) = [345, 300, 310]   next(341) = [346, 302, 311]
//	RCV 1 = {300, 301, 302}       RCV 2 = {310, 311}
//	345 and 346 are dead-end leads. 320 is a lone section with no links.
//	RCV 4 crosses a switch: 330 - 331(SW) - 332, next(331) = [330, 332, 333].
//
// Special locations (type 1, except the hump lead):
//
//	"Norris RCV 1 North end" 300    "Norris RCV 1 South End" 302
//	"Norris RCV 2 North end" 310    "Norris RCV 2 South end" 311
//	"Norris RCV 3 North end" 320    (unpaired: no south end)
//	"Norris RCV 4 North end" 330    "Norris RCV 4 South end" 332 (path crosses 331)
//	"Norris Hump Lead" 200          (type 2, matches no end pattern)
//
// Industries (Config.ind):
//
//	"Synthetic Rail & Locomotive", tag "BRL", local "AB45": 105 and 106,
//	    two sections of a spur; it has car entries with tags and filters.
//	"Padded Tag Works", raw tag " BBB", local "AB10": 500
//	"Unknown Industry", tag "ZZZ", local "AB10": 510. No profile knows ZZZ.
//	    The bundled AGS profile does not know BBB either, so a test that wants
//	    it known adds it to its profile.
//
// Mileposts (name, station):
//
//	("MP 1.0", "Receiving  Yard") at world (1000, -1250), about 100 m from
//	    the bowl and the receiving yard; the station has a doubled space.
//	("MP 40.0", "Far Junction") at world (6000, -1200), over 3 km away.
//
// The tile size is 800 by -1000 m (TileW, TileH), so the track database has
// sections whose two ends lie in adjacent tiles, in both axes.
const (
	SecMainWestEnd = 100
	SecMainTile    = 101
	SecMainSwitch  = 102
	SecMainEast    = 103
	SecMainEastEnd = 104
	SecSiding1     = 105
	SecSiding2     = 106

	SecHumpLead    = 200
	SecHumpSwitch  = 201
	SecHumpLeftSw  = 204
	SecHumpRightSw = 205
	SecBowl1       = 206
	SecBowl2a      = 207
	SecBowl2b      = 210
	SecBowl3       = 208
	SecBowl4       = 209

	SecRcv1North = 300
	SecRcv1Mid   = 301
	SecRcv1South = 302
	SecRcv2North = 310
	SecRcv2South = 311
	SecRcv3North = 320
	SecRcv4North = 330
	SecRcv4Cross = 331
	SecRcv4South = 332
	SecLadderN   = 340
	SecLadderS   = 341

	SecPaddedSpur  = 500
	SecUnknownSpur = 510
)

// HumpControllerName is the synthetic hump controller's name.
const HumpControllerName = "Norris Yard Hump Controller"

// InstallSpec describes the files of a fake Run8 install. A nil slice leaves
// its file out.
type InstallSpec struct {
	Region, Route string
	Industries    []Industry
	Tracks        []TrackSection
	Hump          []HumpController
	Locations     []SpecialLocation
	Mileposts     []Milepost
}

// ConfigPath is where Config.ind sits under an install root.
func ConfigPath(root, region string) string {
	return filepath.Join(root, "Content", "V3Routes", "Regions", region, "Config.ind")
}

// RoutePath is where a route file sits under an install root.
func RoutePath(root, route, name string) string {
	return filepath.Join(root, "Content", "V3Routes", route, name)
}

// WriteInstall writes spec as an install tree under root, which must be a test
// directory. It refuses a root inside a Run8 install.
func WriteInstall(t testing.TB, root string, spec InstallSpec) {
	t.Helper()
	if strings.Contains(filepath.ToSlash(root), "Run8Studios") {
		t.Fatalf("run8routetest: refusing to write inside a Run8 install: %s", root)
	}
	write := func(path string, data []byte) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if spec.Industries != nil {
		write(ConfigPath(root, spec.Region), EncodeConfigInd(spec.Industries))
	}
	if spec.Tracks != nil {
		write(RoutePath(root, spec.Route, "TrackDatabase.r8"), EncodeTrackDB(spec.Tracks))
	}
	if spec.Hump != nil {
		write(RoutePath(root, spec.Route, "HumpControllerList.r8"), EncodeHumpControllers(spec.Hump))
	}
	if spec.Locations != nil {
		write(RoutePath(root, spec.Route, "AISpecialLocations.r8"), EncodeSpecialLocations(spec.Locations))
	}
	if spec.Mileposts != nil {
		write(RoutePath(root, spec.Route, "MilepostDatabase.r8"), EncodeMileposts(spec.Mileposts))
	}
}

// Synthetic returns the small route described above: prefix 280, region
// GulfCoast, route NS_AGS_Phase01.
func Synthetic() InstallSpec {
	p := func(index int, next []int, sw bool, x0, z0, x1, z1 float64) TrackSection {
		return Place(index, next, sw, x0, z0, x1, z1, TileW, TileH)
	}
	tracks := []TrackSection{
		// Main line with a CTC switch and a siding.
		p(100, []int{101}, false, 100, -500, 400, -500),
		p(101, []int{100, 102}, false, 400, -500, 850, -500),
		p(102, []int{101, 103, 105}, true, 850, -500, 880, -500),
		p(103, []int{102, 104}, false, 880, -500, 1300, -500),
		p(104, []int{103}, false, 1300, -500, 1500, -500),
		p(105, []int{102, 106}, false, 880, -500, 1100, -440),
		p(106, []int{105}, false, 1100, -440, 1250, -440),

		// Hump lead and four bowl tracks.
		p(200, []int{201}, false, 900, -900, 900, -1100),
		p(201, []int{200, 202, 203}, true, 900, -1100, 900, -1130),
		p(202, []int{201, 204}, false, 900, -1130, 900, -1200),
		p(203, []int{201, 205}, false, 900, -1130, 860, -1200),
		p(204, []int{202, 206, 207}, true, 900, -1200, 900, -1230),
		p(205, []int{203, 208, 209}, true, 860, -1200, 860, -1230),
		p(206, []int{204}, false, 900, -1230, 900, -1400),
		p(207, []int{204, 210}, false, 900, -1230, 930, -1330),
		p(210, []int{207}, false, 930, -1330, 930, -1450),
		p(208, []int{205}, false, 860, -1230, 860, -1400),
		p(209, []int{205}, false, 860, -1230, 830, -1330),

		// Receiving yard.
		p(345, []int{340}, false, 1000, -1030, 1000, -1100),
		p(340, []int{345, 300, 310}, true, 1000, -1100, 1000, -1130),
		p(300, []int{340, 301}, false, 1000, -1130, 1000, -1200),
		p(301, []int{300, 302}, false, 1000, -1200, 1000, -1300),
		p(302, []int{301, 341}, false, 1000, -1300, 1000, -1400),
		p(341, []int{346, 302, 311}, true, 1000, -1400, 1000, -1430),
		p(346, []int{341}, false, 1000, -1430, 1000, -1500),
		p(310, []int{340, 311}, false, 1040, -1130, 1040, -1250),
		p(311, []int{310, 341}, false, 1040, -1250, 1040, -1400),
		p(320, []int{}, false, 1080, -1100, 1080, -1300),
		p(330, []int{331}, false, 1120, -1100, 1120, -1200),
		p(331, []int{330, 332, 333}, true, 1120, -1200, 1120, -1230),
		p(332, []int{331}, false, 1120, -1230, 1120, -1350),
		p(333, []int{331}, false, 1120, -1230, 1150, -1300),

		// Industry spurs with no links.
		p(500, []int{}, false, 1300, -700, 1400, -700),
		p(510, []int{}, false, 1300, -750, 1400, -750),
	}
	tracks[2].CTC = true // 102

	hump := []HumpController{{
		Name: HumpControllerName,
		Tracks: []HumpTrack{
			{Name: "1", Switches: []HumpSwitch{{201, false}, {204, false}}},
			{Name: "2", Switches: []HumpSwitch{{201, false}, {204, true}}},
			{Name: "3", Switches: []HumpSwitch{{201, true}, {205, false}}},
			{Name: "4", Switches: []HumpSwitch{{201, true}, {205, true}}},
		},
	}}

	loc := func(name string, typ, section int) SpecialLocation {
		return SpecialLocation{Name: name, Type: typ, Prefix: Prefix, Section: section}
	}
	locations := []SpecialLocation{
		loc("Norris RCV 1 North end", 1, 300),
		loc("Norris RCV 1 South End", 1, 302),
		loc("Norris RCV 2 North end", 1, 310),
		loc("Norris RCV 2 South end", 1, 311),
		loc("Norris RCV 3 North end", 1, 320),
		loc("Norris RCV 4 North end", 1, 330),
		loc("Norris RCV 4 South end", 1, 332),
		loc("Norris Hump Lead", 2, 200),
	}

	industries := []Industry{
		{
			Name: "Synthetic Rail & Locomotive", Local: "AB45", Tag: "BRL",
			Tracks: []IndustryTrack{{Prefix, 105, 0}, {Prefix, 106, 1}},
			Cars: []IndustryCar{
				{Version: 2, CarType: 3, ProducesEmpties: true, Hours: 24, Capacity: 4,
					Tags: []string{"SEL", "ATL"}, Filters: []string{"R8_Gon_", "R8_Bulkhead_"}},
				{Version: 2, CarType: 5, Hours: 40, Capacity: 4, Tags: []string{"BNSF"}},
			},
		},
		{
			Name: "Padded Tag Works", Local: "AB10", Tag: " BBB", ProcessInBlocks: true,
			Tracks: []IndustryTrack{{Prefix, SecPaddedSpur, 0}},
			Cars:   []IndustryCar{{Version: 1, CarType: 1, Hours: 24, Capacity: 2, Tags: []string{" BBB"}}},
		},
		{
			Name: "Unknown Industry", Local: "AB10", Tag: "ZZZ",
			Tracks: []IndustryTrack{{Prefix, SecUnknownSpur, 0}},
		},
	}

	mileposts := []Milepost{
		MilepostAt("MP 1.0", "Receiving  Yard", 1000, -1250, TileW, TileH),
		MilepostAt("MP 40.0", "Far Junction", 6000, -1200, TileW, TileH),
	}

	return InstallSpec{
		Region: Region, Route: Route,
		Industries: industries, Tracks: tracks, Hump: hump,
		Locations: locations, Mileposts: mileposts,
	}
}

// CopyLocal copies the git-ignored real route files from references/AutoLabel
// into root, in the install layout, and skips the test when they are absent.
// Run8 files are never committed, so this is the only way a test sees them.
func CopyLocal(t testing.TB, root string) {
	t.Helper()
	src := localDir(t)
	cfg := filepath.Join(src, "Config.ind")
	if _, err := os.Stat(cfg); err != nil {
		t.Skipf("real Run8 files not present (%s)", src)
	}
	copies := map[string]string{
		"Config.ind":            ConfigPath(root, Region),
		"Hump.r8":               filepath.Join(filepath.Dir(ConfigPath(root, Region)), "Hump.r8"),
		"TrackDatabase.r8":      RoutePath(root, Route, "TrackDatabase.r8"),
		"HumpControllerList.r8": RoutePath(root, Route, "HumpControllerList.r8"),
		"AISpecialLocations.r8": RoutePath(root, Route, "AISpecialLocations.r8"),
		"MilepostDatabase.r8":   RoutePath(root, Route, "MilepostDatabase.r8"),
	}
	for name, dst := range copies {
		if err := copyFile(filepath.Join(src, name), dst); err != nil {
			if os.IsNotExist(err) {
				t.Skipf("real Run8 file %s not present", name)
			}
			t.Fatal(err)
		}
	}
}

// localDir finds references/AutoLabel by walking up from the test's directory
// to the module root.
func localDir(t testing.TB) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return filepath.Join(dir, "references", "AutoLabel")
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Skip("module root not found")
		}
		dir = parent
	}
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
