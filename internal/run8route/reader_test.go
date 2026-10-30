package run8route

import (
	"encoding/binary"
	"runtime"
	"strings"
	"testing"

	"run8-ops-center/internal/run8route/run8routetest"
)

func TestStringRoundTrip(t *testing.T) {
	for _, s := range []string{"", "A", " BBB", "Norris RCV 3 South End", "AB10  BRR", "Ünï \u0fff"} {
		r := newReader(run8routetest.EncodeString(s))
		if got := r.str(); got != s {
			t.Errorf("round trip of %q gave %q", s, got)
		}
		if err := r.finish(); err != nil {
			t.Errorf("%q: %v", s, err)
		}
	}
}

func TestStringEncoding(t *testing.T) {
	// 'A' is 0x41: the byte pair is 04 10.
	want := []byte{2, 0, 0, 0, 0x04, 0x10}
	if got := run8routetest.EncodeString("A"); string(got) != string(want) {
		t.Errorf("EncodeString(A) = % x, want % x", got, want)
	}
}

func TestStringRejectsBadLengths(t *testing.T) {
	for name, b := range map[string][]byte{
		"odd":      {3, 0, 0, 0, 1, 2, 3},
		"negative": {0xff, 0xff, 0xff, 0xff},
		"past end": {8, 0, 0, 0, 0, 0},
		"no count": {1, 0},
	} {
		r := newReader(b)
		r.str()
		if r.finish() == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}

func TestCountRejectsWithoutAllocating(t *testing.T) {
	cfg := run8routetest.EncodeConfigInd(run8routetest.Synthetic().Industries)
	track := run8routetest.EncodeTrackDB(run8routetest.Synthetic().Tracks)
	hump := run8routetest.EncodeHumpControllers(run8routetest.Synthetic().Hump)
	locs := run8routetest.EncodeSpecialLocations(run8routetest.Synthetic().Locations)
	mps := run8routetest.EncodeMileposts(run8routetest.Synthetic().Mileposts)
	decoders := map[string]struct {
		file   []byte
		decode func([]byte) error
	}{
		"Config.ind":            {cfg, func(b []byte) error { _, err := DecodeConfigInd(b); return err }},
		"TrackDatabase.r8":      {track, func(b []byte) error { _, err := DecodeTrackDatabase(b); return err }},
		"HumpControllerList.r8": {hump, func(b []byte) error { _, err := DecodeHumpControllers(b); return err }},
		"AISpecialLocations.r8": {locs, func(b []byte) error { _, err := DecodeSpecialLocations(b); return err }},
		"MilepostDatabase.r8":   {mps, func(b []byte) error { _, err := DecodeMileposts(b, 1, 1); return err }},
	}
	for name, d := range decoders {
		patched := append([]byte(nil), d.file...)
		binary.LittleEndian.PutUint32(patched[4:], 0x7fffffff) // record count
		var before, after runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&before)
		err := d.decode(patched)
		runtime.ReadMemStats(&after)
		if err == nil {
			t.Errorf("%s: inflated count accepted", name)
		}
		if grew := after.TotalAlloc - before.TotalAlloc; grew > 1<<20 {
			t.Errorf("%s: inflated count allocated %d bytes", name, grew)
		}
	}
}

func TestInnerCountRejected(t *testing.T) {
	// The first industry's track count sits after its three strings.
	spec := run8routetest.Synthetic()
	b := run8routetest.EncodeConfigInd(spec.Industries)
	off := 8 + 4
	for range 3 {
		n := int(binary.LittleEndian.Uint32(b[off:]))
		off += 4 + n
	}
	off++ // process in blocks
	patched := append([]byte(nil), b...)
	binary.LittleEndian.PutUint32(patched[off:], 1<<28)
	if _, err := DecodeConfigInd(patched); err == nil || !strings.Contains(err.Error(), "does not fit") {
		t.Errorf("inflated track count: %v", err)
	}
}

func TestTrailingBytesRejected(t *testing.T) {
	spec := run8routetest.Synthetic()
	files := map[string][]byte{
		"Config.ind": run8routetest.EncodeConfigInd(spec.Industries),
		"Track":      run8routetest.EncodeTrackDB(spec.Tracks),
		"Hump":       run8routetest.EncodeHumpControllers(spec.Hump),
		"Locations":  run8routetest.EncodeSpecialLocations(spec.Locations),
		"Mileposts":  run8routetest.EncodeMileposts(spec.Mileposts),
	}
	for name, b := range files {
		if err := decodeAny(name, append(append([]byte(nil), b...), 0)); err == nil || !strings.Contains(err.Error(), "trailing") {
			t.Errorf("%s with a trailing byte: %v", name, err)
		}
	}
}

func decodeAny(name string, b []byte) error {
	var err error
	switch name {
	case "Config.ind":
		_, err = DecodeConfigInd(b)
	case "Track":
		_, err = DecodeTrackDatabase(b)
	case "Hump":
		_, err = DecodeHumpControllers(b)
	case "Locations":
		_, err = DecodeSpecialLocations(b)
	case "Mileposts":
		_, err = DecodeMileposts(b, 1, 1)
	}
	return err
}

func TestTruncationRejected(t *testing.T) {
	spec := run8routetest.Synthetic()
	files := map[string][]byte{
		"Config.ind": run8routetest.EncodeConfigInd(spec.Industries),
		"Track":      run8routetest.EncodeTrackDB(spec.Tracks),
		"Hump":       run8routetest.EncodeHumpControllers(spec.Hump),
		"Locations":  run8routetest.EncodeSpecialLocations(spec.Locations),
		"Mileposts":  run8routetest.EncodeMileposts(spec.Mileposts),
	}
	for name, b := range files {
		if err := decodeAny(name, b); err != nil {
			t.Fatalf("%s: whole file: %v", name, err)
		}
		step := 1
		if len(b) > 4000 {
			step = 7
		}
		for n := 0; n < len(b); n += step {
			if err := decodeAny(name, b[:n]); err == nil {
				t.Fatalf("%s cut to %d of %d bytes was accepted", name, n, len(b))
			}
		}
	}
}
