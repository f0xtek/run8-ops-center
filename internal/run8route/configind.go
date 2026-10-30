package run8route

import "fmt"

// Smallest sizes of the variable parts of a Config.ind record, used to bound
// counts: an industry is an i32, three empty strings, a u8 and two counts; a
// track is four i32; a car entry is its fixed fields plus two counts.
const (
	minIndustry      = 4 + 3*4 + 1 + 4 + 4
	minIndustryTrack = 16
	minIndustryCar   = 4 + 1 + 1 + 4 + 4 + 4 + 4
)

// DecodeConfigInd decodes Config.ind. Car entries are read to keep the
// decoder in step with the file but are not kept.
func DecodeConfigInd(b []byte) ([]Industry, error) {
	r := newReader(b)
	r.i32() // reserved
	n := r.count(minIndustry, "industry")
	out := make([]Industry, 0, n)
	for i := 0; i < n && r.err == nil; i++ {
		r.i32()
		ind := Industry{Name: r.str(), Local: r.str(), Tag: r.str()}
		r.u8() // process in blocks
		nt := r.count(minIndustryTrack, "track")
		ind.Tracks = make([]IndustryTrack, 0, nt)
		for j := 0; j < nt && r.err == nil; j++ {
			r.i32()
			ind.Tracks = append(ind.Tracks, IndustryTrack{Prefix: r.i32(), Section: r.i32(), Node: r.i32()})
		}
		nc := r.count(minIndustryCar, "car")
		for j := 0; j < nc && r.err == nil; j++ {
			r.i32() // version
			r.u8()  // car type
			r.u8()  // produces empties
			r.i32() // hours
			r.i32() // capacity
			for _, what := range []string{"car tag", "car filter"} {
				m := r.count(4, what)
				for k := 0; k < m && r.err == nil; k++ {
					r.str()
				}
			}
		}
		out = append(out, ind)
	}
	if err := r.finish(); err != nil {
		return nil, fmt.Errorf("decode %s: %w", ConfigFile, err)
	}
	return out, nil
}
