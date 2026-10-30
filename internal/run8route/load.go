package run8route

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// routeFiles is the number of files read per route.
const routeFiles = 4

// maxFileSize bounds what Load reads into memory. The largest real file is
// about 2 MB.
const maxFileSize = 256 << 20

// fileRef is one file the loader reads.
type fileRef struct {
	name, path string
	err        error // set when the path is unusable
}

// refs lists the files for an install in a fixed order: Config.ind, then the
// four route files of each route.
func refs(installRoot, region string, routes []Route) []fileRef {
	var out []fileRef
	v3 := filepath.Join(installRoot, "Content", "V3Routes")
	add := func(name, dir string, bad error) {
		ref := fileRef{name: name, path: filepath.Join(dir, name), err: bad}
		if bad != nil {
			ref.path = ""
		}
		out = append(out, ref)
	}
	var bad error
	switch {
	case installRoot == "":
		bad = errors.New("no Run8 install folder set")
	case !validComponent(region):
		bad = fmt.Errorf("region folder %q is not a single folder name", region)
	}
	add(ConfigFile, filepath.Join(v3, "Regions", region), bad)
	for _, rt := range routes {
		bad = nil
		switch {
		case installRoot == "":
			bad = errors.New("no Run8 install folder set")
		case !validComponent(rt.Folder):
			bad = fmt.Errorf("route folder %q is not a single folder name", rt.Folder)
		}
		for _, name := range []string{TrackDBFile, HumpFile, LocationsFile, MilepostsFile} {
			add(name, filepath.Join(v3, rt.Folder), bad)
		}
	}
	return out
}

// validComponent reports whether s is one plain folder name: not empty, not
// "." or "..", and without a separator of either kind.
func validComponent(s string) bool {
	return s != "" && s != "." && s != ".." && !strings.ContainsAny(s, `/\:`) && !strings.ContainsRune(s, 0)
}

// Load reads the route files under installRoot: Config.ind from the region
// folder, and the track, hump, special-location and milepost files from the
// route folders. Each file fails on its own and is reported in Result.Files.
// Files are only opened for reading.
//
// A route's data comes from the first route in the list. Further routes are
// fingerprinted so a change is noticed, but their files are not decoded,
// because Data has a single topology.
func Load(installRoot, region string, routes []Route) Result {
	var res Result
	loaded := map[string][]byte{}
	for i, ref := range refs(installRoot, region, routes) {
		decoded := i < 1+routeFiles // Config.ind and the first route's files
		st := FileStatus{Name: ref.name, Path: ref.path}
		if ref.err != nil {
			st.Err = ref.err
		} else if !decoded {
			if fp, ok := statFile(ref.path); ok {
				res.Fingerprint = append(res.Fingerprint, fp)
			}
			continue
		} else {
			data, fp, err := readFile(ref.path)
			if fp.Path != "" {
				res.Fingerprint = append(res.Fingerprint, fp)
			}
			st.Err = err
			if err == nil {
				loaded[ref.name] = data
			}
		}
		if decoded {
			res.Files = append(res.Files, st)
		}
	}

	var mileposts []byte
	for i := range res.Files {
		st := &res.Files[i]
		data, ok := loaded[st.Name]
		if !ok {
			continue
		}
		var err error
		switch st.Name {
		case ConfigFile:
			res.Data.Industries, err = DecodeConfigInd(data)
		case TrackDBFile:
			res.Data.Topology, err = DecodeTrackDatabase(data)
		case HumpFile:
			res.Data.Hump, err = DecodeHumpControllers(data)
		case LocationsFile:
			res.Data.Locations, err = DecodeSpecialLocations(data)
		case MilepostsFile:
			// Needs the tile size, so it is decoded after the loop.
			mileposts = data
			continue
		}
		st.Err, st.OK = err, err == nil
	}
	if mileposts != nil {
		var w, h float64
		if res.Data.Topology != nil {
			w, h = res.Data.Topology.TileSize()
		}
		for i := range res.Files {
			if st := &res.Files[i]; st.Name == MilepostsFile {
				var err error
				res.Data.Mileposts, err = DecodeMileposts(mileposts, w, h)
				st.Err, st.OK = err, err == nil
			}
		}
	}
	return res
}

// readFile reads a whole file with a read-only open. The fingerprint comes from
// the open handle, so it describes the bytes that were read. It is the zero
// value when the file is missing or not a regular file.
func readFile(path string) ([]byte, FileFingerprint, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, FileFingerprint{}, err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return nil, FileFingerprint{}, err
	}
	if !fi.Mode().IsRegular() {
		return nil, FileFingerprint{}, fmt.Errorf("%s is not a regular file", path)
	}
	fp := FileFingerprint{Path: path, Size: fi.Size(), ModTime: fi.ModTime()}
	if fi.Size() > maxFileSize {
		return nil, fp, fmt.Errorf("file is %d bytes, over the %d byte limit", fi.Size(), int64(maxFileSize))
	}
	data, err := io.ReadAll(io.LimitReader(f, maxFileSize+1))
	if err != nil {
		return nil, fp, err
	}
	return data, fp, nil
}

func statFile(path string) (FileFingerprint, bool) {
	fi, err := os.Stat(path)
	if err != nil || !fi.Mode().IsRegular() {
		return FileFingerprint{}, false
	}
	return FileFingerprint{Path: path, Size: fi.Size(), ModTime: fi.ModTime()}, true
}

// Stat returns the fingerprint of every file that exists, without decoding
// anything. A file that is missing is left out, so a file appearing or
// disappearing also changes the result.
func Stat(installRoot, region string, routes []Route) []FileFingerprint {
	var out []FileFingerprint
	for _, ref := range refs(installRoot, region, routes) {
		if ref.err != nil {
			continue
		}
		if fp, ok := statFile(ref.path); ok {
			out = append(out, fp)
		}
	}
	return out
}

// SameFingerprint reports whether two fingerprints list the same files with
// the same sizes and modification times.
func SameFingerprint(a, b []FileFingerprint) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Path != b[i].Path || a[i].Size != b[i].Size || !a[i].ModTime.Equal(b[i].ModTime) {
			return false
		}
	}
	return true
}
