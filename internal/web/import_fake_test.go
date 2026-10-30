package web

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"testing"
	"time"

	"run8-ops-center/internal/engine"
	"run8-ops-center/internal/profile"
	"run8-ops-center/internal/run8route"
	"run8-ops-center/internal/run8route/run8routetest"
	"run8-ops-center/internal/run8save"
	"run8-ops-center/internal/trackmap"
)

// vwImportTime is the time the fake records for a confirmed import.
var vwImportTime = time.Date(2026, 10, 30, 19, 30, 0, 0, time.Local)

// vwImporter is the import half of a fake backend. It runs the real route
// loader, import builder, diff and apply over a synthetic Run8 install in a temp
// directory, and keeps the engine's stale-token rule: the token hashes the route
// file fingerprints with the track map revision.
type vwImporter struct {
	root string
	prof *profile.Profile
}

// vwNewImporter writes spec as an install tree and reads it for prof's run8 block.
func vwNewImporter(t *testing.T, prof *profile.Profile, spec run8routetest.InstallSpec) *vwImporter {
	t.Helper()
	if prof.Run8 == nil {
		t.Fatal("the profile has no run8 block")
	}
	root := t.TempDir()
	run8routetest.WriteInstall(t, root, spec)
	return &vwImporter{root: root, prof: prof}
}

func (im *vwImporter) routes() []run8route.Route {
	out := make([]run8route.Route, len(im.prof.Run8.Routes))
	for i, r := range im.prof.Run8.Routes {
		out[i] = run8route.Route{Folder: r.Folder}
	}
	return out
}

func (im *vwImporter) load() run8route.Result {
	return run8route.Load(im.root, im.prof.Run8.Region, im.routes())
}

// data is the route data learn mode builds on.
func (im *vwImporter) data() *run8route.Data {
	res := im.load()
	return &res.Data
}

func (im *vwImporter) token(res run8route.Result, tm *trackmap.Map) string {
	h := sha256.New()
	for _, f := range res.Fingerprint {
		fmt.Fprintf(h, "%s\x00%d\x00%s\n", f.Path, f.Size, strconv.FormatInt(f.ModTime.UnixNano(), 10))
	}
	fmt.Fprintf(h, "map\x00%s", tm.Revision())
	return hex.EncodeToString(h.Sum(nil))
}

// build reads the files and diffs the import against tm.
func (im *vwImporter) build(tm *trackmap.Map) (run8route.Result, engine.ImportPreview, error) {
	res := im.load()
	var p engine.ImportPreview
	usable := 0
	for _, f := range res.Files {
		if f.OK {
			usable++
		} else {
			p.Unusable = append(p.Unusable, f)
		}
	}
	if usable == 0 {
		return res, p, errors.New("no Run8 route file could be read")
	}
	p.Diff = tm.Diff(trackmap.BuildImport(res.Data, im.prof))
	p.Token = im.token(res, tm)
	return res, p, nil
}

func (im *vwImporter) preview(tm *trackmap.Map) (engine.ImportPreview, error) {
	_, p, err := im.build(tm)
	if err != nil {
		return engine.ImportPreview{}, err
	}
	return p, nil
}

func (im *vwImporter) confirm(tm *trackmap.Map, token string) error {
	res, p, err := im.build(tm)
	if err != nil {
		return err
	}
	if token == "" || p.Token != token {
		return engine.ErrStaleImport
	}
	tm.ApplyImport(p.Diff, res.Fingerprint, vwImportTime)
	return nil
}

// state is the View's Run8 block, refreshed the way the engine does it.
func (im *vwImporter) state(tm *trackmap.Map) engine.Run8State {
	st := engine.Run8State{HasRun8Config: true}
	res := im.load()
	usable := 0
	for _, f := range res.Files {
		if f.OK {
			usable++
		} else {
			st.Unavailable = append(st.Unavailable, f)
		}
	}
	switch {
	case usable == 0:
		st.Status, st.Reason = engine.Run8Unavailable, "no Run8 route file could be read"
	case len(st.Unavailable) > 0:
		st.Status = engine.Run8Partial
	default:
		st.Status = engine.Run8Ready
	}
	if imp := tm.Run8Import; imp != nil {
		st.LastImport = imp.At
		st.Changed = !run8route.SameFingerprint(run8route.Stat(im.root, im.prof.Run8.Region, im.routes()), imp.Files)
	}
	st.ImportAvailable = usable > 0 && tm.Run8Import == nil
	return st
}

// vwCutSnapshot is a snapshot with one unpowered cut of cars tagged tag on the
// given sections of the synthetic route.
func vwCutSnapshot(trainID, tag string, sections ...int) *run8save.Snapshot {
	var vs []run8save.Vehicle
	for i, s := range sections {
		vs = append(vs, run8save.Vehicle{
			Model: "R8_Boxcar_X.xml", UnitType: "US_Freightcar", UnitNumber: strconv.Itoa(1000 + i), Tag: tag,
			Trucks: []run8save.Truck{{RoutePrefix: run8routetest.Prefix, Section: s}, {RoutePrefix: run8routetest.Prefix, Section: s}},
		})
	}
	return &run8save.Snapshot{Trains: []run8save.Train{{ID: trainID, Vehicles: vs}}}
}
