package run8route

import "fmt"

// DecodeHumpControllers decodes HumpControllerList.r8. Each path is a list of
// the switches a car takes from the hump lead to a bowl track.
func DecodeHumpControllers(b []byte) ([]HumpController, error) {
	const (
		minController = 4 + 4 + 8 + 12 + 4 + 4
		minPath       = 4 + 4 + 4
		minSwitch     = 4 + 4 + 1
	)
	r := newReader(b)
	r.i32() // reserved
	n := r.count(minController, "controller")
	out := make([]HumpController, 0, n)
	for i := 0; i < n && r.err == nil; i++ {
		r.i32()
		hc := HumpController{Name: r.str()}
		r.skip(8 + 12) // tile, position
		r.str()
		np := r.count(minPath, "path")
		hc.Tracks = make([]HumpTrack, 0, np)
		for j := 0; j < np && r.err == nil; j++ {
			r.i32()
			ht := HumpTrack{Name: r.str()}
			ns := r.count(minSwitch, "switch")
			ht.Switches = make([]HumpSwitch, 0, ns)
			for k := 0; k < ns && r.err == nil; k++ {
				r.i32()
				ht.Switches = append(ht.Switches, HumpSwitch{Section: r.i32(), Reversed: r.u8() != 0})
			}
			hc.Tracks = append(hc.Tracks, ht)
		}
		out = append(out, hc)
	}
	if err := r.finish(); err != nil {
		return nil, fmt.Errorf("decode %s: %w", HumpFile, err)
	}
	return out, nil
}

// DecodeSpecialLocations decodes AISpecialLocations.r8.
func DecodeSpecialLocations(b []byte) ([]SpecialLocation, error) {
	// After the name: type, prefix, section, two u8, two bytes, an i32, an
	// i16 time, two bytes and a u8 skip flag.
	const tail = 1 + 4 + 4 + 1 + 1 + 2 + 4 + 2 + 2 + 1
	r := newReader(b)
	r.i32() // reserved
	n := r.count(4+4+tail, "location")
	out := make([]SpecialLocation, 0, n)
	for i := 0; i < n && r.err == nil; i++ {
		r.i32()
		loc := SpecialLocation{Name: r.str()}
		loc.Type = int(r.u8())
		loc.Prefix = r.i32()
		loc.Section = r.i32()
		r.skip(tail - 1 - 4 - 4)
		out = append(out, loc)
	}
	if err := r.finish(); err != nil {
		return nil, fmt.Errorf("decode %s: %w", LocationsFile, err)
	}
	return out, nil
}

// DecodeMileposts decodes MilepostDatabase.r8. The world coordinates use the
// tile size w by h, normally Topology.TileSize.
func DecodeMileposts(b []byte, w, h float64) ([]Milepost, error) {
	const minMilepost = 4 + 4 + 4 + 8 + 12
	r := newReader(b)
	r.i32() // reserved
	n := r.count(minMilepost, "milepost")
	out := make([]Milepost, 0, n)
	for i := 0; i < n && r.err == nil; i++ {
		r.i32()
		mp := Milepost{Name: r.str(), Station: r.str()}
		tx, tz := r.i32(), r.i32()
		x, _, z := r.f32(), r.f32(), r.f32()
		mp.X = float64(tx)*w + float64(x)
		mp.Z = float64(tz)*h + float64(z)
		out = append(out, mp)
	}
	if err := r.finish(); err != nil {
		return nil, fmt.Errorf("decode %s: %w", MilepostsFile, err)
	}
	return out, nil
}
