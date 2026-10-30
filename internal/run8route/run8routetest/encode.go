// Package run8routetest builds synthetic Run8 route files for tests. It
// encodes each format byte for byte, so no real Run8 file is ever needed in the
// repository. Import it only from _test.go files.
//
// It does not import run8route, so run8route's own tests can use it.
package run8routetest

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
)

type buf struct{ bytes.Buffer }

func (b *buf) u8(v byte) { b.WriteByte(v) }

func (b *buf) boolean(v bool) {
	if v {
		b.u8(1)
	} else {
		b.u8(0)
	}
}

func (b *buf) i16(v int) { _ = binary.Write(b, binary.LittleEndian, int16(v)) }
func (b *buf) i32(v int) { _ = binary.Write(b, binary.LittleEndian, int32(v)) }
func (b *buf) f32(v float32) {
	_ = binary.Write(b, binary.LittleEndian, math.Float32bits(v))
}
func (b *buf) f64(v float64) {
	_ = binary.Write(b, binary.LittleEndian, math.Float64bits(v))
}
func (b *buf) zero(n int) { b.Write(make([]byte, n)) }

// str writes an R8String: an int32 byte count, then two bytes per character,
// c>>4 and (c&15)<<4. It panics on a character above U+0FFF, which the format
// cannot hold.
func (b *buf) str(s string) {
	rs := []rune(s)
	b.i32(2 * len(rs))
	for _, c := range rs {
		if c > 0xFFF {
			panic(fmt.Sprintf("run8routetest: character %q does not fit an R8String", c))
		}
		b.u8(byte(c >> 4))
		b.u8(byte(c&15) << 4)
	}
}

// EncodeString returns s as an R8String.
func EncodeString(s string) []byte {
	var b buf
	b.str(s)
	return b.Bytes()
}

// IndustryTrack is one section an industry lists.
type IndustryTrack struct{ Prefix, Section, Node int }

// IndustryCar is one car entry of an industry. The decoder reads it but
// run8route does not keep it.
type IndustryCar struct {
	Version         int
	CarType         byte
	ProducesEmpties bool
	Hours, Capacity int
	Tags, Filters   []string
}

// Industry is one Config.ind industry.
type Industry struct {
	Name, Local, Tag string
	ProcessInBlocks  bool
	Tracks           []IndustryTrack
	Cars             []IndustryCar
}

// EncodeConfigInd encodes Config.ind.
func EncodeConfigInd(inds []Industry) []byte {
	var b buf
	b.i32(1)
	b.i32(len(inds))
	for _, in := range inds {
		b.i32(1)
		b.str(in.Name)
		b.str(in.Local)
		b.str(in.Tag)
		b.boolean(in.ProcessInBlocks)
		b.i32(len(in.Tracks))
		for _, t := range in.Tracks {
			b.i32(0)
			b.i32(t.Prefix)
			b.i32(t.Section)
			b.i32(t.Node)
		}
		b.i32(len(in.Cars))
		for _, c := range in.Cars {
			b.i32(c.Version)
			b.u8(c.CarType)
			b.boolean(c.ProducesEmpties)
			b.i32(c.Hours)
			b.i32(c.Capacity)
			b.i32(len(c.Tags))
			for _, t := range c.Tags {
				b.str(t)
			}
			b.i32(len(c.Filters))
			for _, f := range c.Filters {
				b.str(f)
			}
		}
	}
	return b.Bytes()
}

// TrackSection is one TrackDatabase.r8 section with two directions (four nodes
// for a switch). Start is local to Tile0 and End is local to Tile1, as in the
// real files. Use Place to build one from world coordinates.
type TrackSection struct {
	Index  int
	Next   []int // a switch lists [points, normal, reverse]
	Switch bool
	CTC    bool
	Tile0  [2]int
	Tile1  [2]int
	Start  [3]float32
	End    [3]float32
	// Heading is the direction of travel in degrees, clockwise from -Z.
	Heading float32
	// Curve is 0 for a straight section.
	Curve  float32
	Length float32
}

// Place builds a straight section from world start and end points (x, z in
// metres, height 100) for tiles w wide and h tall. h is negative on AGS.
func Place(index int, next []int, sw bool, x0, z0, x1, z1, w, h float64) TrackSection {
	tx0, tz0 := math.Floor(x0/w), math.Floor(z0/h)
	tx1, tz1 := math.Floor(x1/w), math.Floor(z1/h)
	dx, dz := x1-x0, z1-z0
	heading := math.Atan2(dx, -dz) * 180 / math.Pi
	if heading < 0 {
		heading += 360
	}
	return TrackSection{
		Index: index, Next: next, Switch: sw,
		Tile0: [2]int{int(tx0), int(tz0)}, Tile1: [2]int{int(tx1), int(tz1)},
		Start:   [3]float32{float32(x0 - tx0*w), 100, float32(z0 - tz0*h)},
		End:     [3]float32{float32(x1 - tx1*w), 100, float32(z1 - tz1*h)},
		Heading: float32(heading),
		Length:  float32(math.Hypot(dx, dz)),
	}
}

func (b *buf) node(s TrackSection, reverse bool, idx int) {
	tile, start, end, heading := s.Tile0, s.Start, s.End, s.Heading
	if reverse {
		tile, start, end = s.Tile1, s.End, s.Start
		heading = float32(math.Mod(float64(heading)+180, 360))
	}
	b.i32(1)
	b.i32(tile[0])
	b.i32(tile[1])
	for _, v := range start {
		b.f32(v)
	}
	b.f32(0.24)
	b.f32(heading)
	b.f32(0)
	for _, v := range end {
		b.f32(v)
	}
	b.i32(idx)
	b.boolean(s.Switch && idx < 2) // switch flag
	b.boolean(reverse)
	b.f32(s.Curve)
	b.i32(0) // sign
	b.f32(0) // radius
	b.f32(s.Length)
	b.i32(2) // segments
	b.i32(s.Index)
	b.u8(0) // sound
}

// EncodeTrackDB encodes TrackDatabase.r8.
func EncodeTrackDB(secs []TrackSection) []byte {
	var b buf
	b.i32(1)
	b.i32(len(secs))
	for _, s := range secs {
		b.i32(1)
		nodes := 2
		if s.Switch {
			nodes = 4
		}
		b.i32(nodes)
		for i := range nodes {
			b.node(s, i%2 == 1, i)
		}
		b.i32(s.Index)
		b.u8(0) // lever
		b.i32(len(s.Next))
		for _, n := range s.Next {
			b.i32(n)
		}
		b.u8(0)  // type
		b.f64(0) // retarder
		b.u8(0)  // occupied
		b.u8(0)  // stand side
		b.i32(0) // stand type
		b.boolean(s.CTC)
	}
	return b.Bytes()
}

// HumpSwitch is one switch of a hump path.
type HumpSwitch struct {
	Section  int
	Reversed bool
}

// HumpTrack is one bowl track path.
type HumpTrack struct {
	Name     string
	Switches []HumpSwitch
}

// HumpController is one hump controller.
type HumpController struct {
	Name   string
	Tracks []HumpTrack
}

// EncodeHumpControllers encodes HumpControllerList.r8.
func EncodeHumpControllers(hcs []HumpController) []byte {
	var b buf
	b.i32(1)
	b.i32(len(hcs))
	for _, hc := range hcs {
		b.i32(1)
		b.str(hc.Name)
		b.zero(8 + 12) // tile, position
		b.str("")
		b.i32(len(hc.Tracks))
		for _, t := range hc.Tracks {
			b.i32(1)
			b.str(t.Name)
			b.i32(len(t.Switches))
			for _, s := range t.Switches {
				b.i32(1)
				b.i32(s.Section)
				b.boolean(s.Reversed)
			}
		}
	}
	return b.Bytes()
}

// SpecialLocation is one AISpecialLocations.r8 entry.
type SpecialLocation struct {
	Name                  string
	Type, Prefix, Section int
}

// EncodeSpecialLocations encodes AISpecialLocations.r8.
func EncodeSpecialLocations(locs []SpecialLocation) []byte {
	var b buf
	b.i32(1)
	b.i32(len(locs))
	for _, l := range locs {
		b.i32(1)
		b.str(l.Name)
		b.u8(byte(l.Type))
		b.i32(l.Prefix)
		b.i32(l.Section)
		b.u8(0)
		b.u8(0)
		b.zero(2)
		b.i32(0)
		b.i16(0) // time
		b.zero(2)
		b.u8(0) // skip
	}
	return b.Bytes()
}

// Milepost is one MilepostDatabase.r8 entry, with position local to Tile.
type Milepost struct {
	Name, Station string
	Tile          [2]int
	Pos           [3]float32
}

// MilepostAt places a milepost at world x, z for tiles w wide and h tall.
func MilepostAt(name, station string, x, z, w, h float64) Milepost {
	tx, tz := math.Floor(x/w), math.Floor(z/h)
	return Milepost{
		Name: name, Station: station,
		Tile: [2]int{int(tx), int(tz)},
		Pos:  [3]float32{float32(x - tx*w), 100, float32(z - tz*h)},
	}
}

// EncodeMileposts encodes MilepostDatabase.r8.
func EncodeMileposts(mps []Milepost) []byte {
	var b buf
	b.i32(1)
	b.i32(len(mps))
	for _, m := range mps {
		b.i32(1)
		b.str(m.Name)
		b.str(m.Station)
		b.i32(m.Tile[0])
		b.i32(m.Tile[1])
		for _, v := range m.Pos {
			b.f32(v)
		}
	}
	return b.Bytes()
}
