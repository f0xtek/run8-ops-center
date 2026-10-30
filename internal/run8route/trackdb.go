package run8route

import (
	"encoding/binary"
	"fmt"
	"math"
	"sort"
)

const (
	trackNodeSize = 79
	// A section with no nodes and no next links: i32, node count, index, lever,
	// next count, type, retarder f64, occupied, stand side, stand type, CTC.
	minTrackSection = 4 + 4 + 4 + 1 + 4 + 1 + 8 + 1 + 1 + 4 + 1
)

// trackNode is the part of a 79-byte node the topology uses. Positions are
// local to the node's tile.
type trackNode struct {
	tx, tz        int
	sx, sy, sz    float64
	ex, ey, ez    float64
	heading       float64 // degrees, clockwise from -Z
	curve, length float64
}

func parseNode(b []byte) trackNode {
	le := binary.LittleEndian
	f := func(off int) float64 { return float64(math.Float32frombits(le.Uint32(b[off:]))) }
	return trackNode{
		tx: int(int32(le.Uint32(b[4:]))), tz: int(int32(le.Uint32(b[8:]))),
		sx: f(12), sy: f(16), sz: f(20),
		heading: f(28),
		ex:      f(36), ey: f(40), ez: f(44),
		curve:  f(54),
		length: f(66),
	}
}

// rawSection is one decoded section before the tile size is known.
type rawSection struct {
	index  int
	next   []int
	sw     bool
	n0, n1 trackNode
}

// DecodeTrackDatabase decodes TrackDatabase.r8 into a Topology. A section with
// four nodes is a switch. Fewer than two nodes, or a repeated section index,
// rejects the file.
func DecodeTrackDatabase(b []byte) (*Topology, error) {
	t, err := decodeTrackDatabase(b)
	if err != nil {
		return nil, fmt.Errorf("decode %s: %w", TrackDBFile, err)
	}
	return t, nil
}

func decodeTrackDatabase(b []byte) (*Topology, error) {
	r := newReader(b)
	r.i32() // reserved
	n := r.count(minTrackSection, "section")
	raw := make([]rawSection, 0, n)
	for i := 0; i < n && r.err == nil; i++ {
		r.i32()
		nn := r.count(trackNodeSize, "node")
		var s rawSection
		for k := 0; k < nn && r.err == nil; k++ {
			nb := r.take(trackNodeSize)
			if nb == nil {
				break
			}
			switch k {
			case 0:
				s.n0 = parseNode(nb)
			case 1:
				s.n1 = parseNode(nb)
			}
		}
		s.index = r.i32()
		r.u8() // lever
		nx := r.count(4, "next section")
		s.next = make([]int, 0, nx)
		for k := 0; k < nx && r.err == nil; k++ {
			s.next = append(s.next, r.i32())
		}
		r.u8()  // type
		r.f64() // retarder
		r.u8()  // occupied
		r.u8()  // stand side
		r.i32() // stand type
		r.u8()  // CTC
		if r.err == nil {
			if nn < 2 {
				return nil, fmt.Errorf("section %d has %d nodes, want at least 2", s.index, nn)
			}
			s.sw = nn == 4
			raw = append(raw, s)
		}
	}
	if err := r.finish(); err != nil {
		return nil, err
	}
	return buildTopology(raw)
}

func buildTopology(raw []rawSection) (*Topology, error) {
	w, h := estimateTileSize(raw)
	t := &Topology{
		secs:  make([]Section, len(raw)),
		pos:   make(map[int]int, len(raw)),
		tileW: w, tileH: h,
	}
	for i, s := range raw {
		if _, dup := t.pos[s.index]; dup {
			return nil, fmt.Errorf("section %d appears twice", s.index)
		}
		t.pos[s.index] = i
		// Node 0 starts in its own tile and ends where node 1 starts, in
		// node 1's tile.
		x0 := float64(s.n0.tx)*w + s.n0.sx
		z0 := float64(s.n0.tz)*h + s.n0.sz
		x1 := float64(s.n1.tx)*w + s.n0.ex
		z1 := float64(s.n1.tz)*h + s.n0.ez
		t.secs[i] = Section{
			Index: s.index, Next: s.next, Switch: s.sw,
			LengthM: s.n0.length,
			X:       (x0 + x1) / 2, Z: (z0 + z1) / 2,
		}
		if s.sw {
			t.switches++
		}
	}
	return t, nil
}

// estimateTileSize recovers the tile width and height from the sections
// themselves: the median over straight, non-switch sections whose two ends lie
// in adjacent tiles. For such a section the world displacement has the known
// length, a known component along the other axis, and a sign given by the
// heading, which leaves the tile size as the only unknown. It returns 0 for an
// axis with no such section.
func estimateTileSize(raw []rawSection) (w, h float64) {
	var ws, hs []float64
	for _, s := range raw {
		if s.sw || s.n0.curve != 0 || s.n0.length <= 0 {
			continue
		}
		dtx, dtz := s.n1.tx-s.n0.tx, s.n1.tz-s.n0.tz
		dx, dy, dz := s.n0.ex-s.n0.sx, s.n0.ey-s.n0.sy, s.n0.ez-s.n0.sz
		rad := s.n0.heading * math.Pi / 180
		switch {
		case (dtx == 1 || dtx == -1) && dtz == 0:
			r2 := s.n0.length*s.n0.length - dy*dy - dz*dz
			if r2 < 0 {
				continue
			}
			ws = append(ws, (math.Copysign(math.Sqrt(r2), math.Sin(rad))-dx)/float64(dtx))
		case dtx == 0 && (dtz == 1 || dtz == -1):
			r2 := s.n0.length*s.n0.length - dy*dy - dx*dx
			if r2 < 0 {
				continue
			}
			hs = append(hs, (math.Copysign(math.Sqrt(r2), -math.Cos(rad))-dz)/float64(dtz))
		}
	}
	return median(ws), median(hs)
}

func median(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	sort.Float64s(v)
	if len(v)%2 == 1 {
		return v[len(v)/2]
	}
	return (v[len(v)/2-1] + v[len(v)/2]) / 2
}
