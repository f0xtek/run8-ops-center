package run8route

import "sort"

// Section is one track section of the route.
type Section struct {
	Index int
	// Next holds the linked sections. For a switch it is
	// [points, normal, reverse]. Callers must not modify it.
	Next    []int
	Switch  bool
	LengthM float64
	// X and Z are the world midpoint in metres, from the estimated tile size.
	X, Z float64
}

// Topology is the track graph of one route. It is read-only after decoding,
// so it is safe for concurrent use.
type Topology struct {
	secs         []Section
	pos          map[int]int
	switches     int
	tileW, tileH float64
}

// Section returns the section with the given index.
func (t *Topology) Section(i int) (Section, bool) {
	p, ok := t.pos[i]
	if !ok {
		return Section{}, false
	}
	return t.secs[p], true
}

// Len is the number of sections.
func (t *Topology) Len() int { return len(t.secs) }

// SwitchCount is the number of switch sections.
func (t *Topology) SwitchCount() int { return t.switches }

// TileSize is the estimated tile width and height in metres. The height is
// negative on AGS, because world z falls as the tile row rises. It is 0 on an
// axis where the database holds no straight section across a tile edge.
func (t *Topology) TileSize() (w, h float64) { return t.tileW, t.tileH }

// TrackOf returns the switch-bounded track holding a section: the maximal set
// of non-switch sections reached by following next links, which stops at a
// switch or a dead end. A switch returns only itself. An unknown section
// returns nil. The result is sorted.
func (t *Topology) TrackOf(section int) []int {
	s, ok := t.Section(section)
	if !ok {
		return nil
	}
	if s.Switch {
		return []int{section}
	}
	seen := map[int]bool{section: true}
	stack := []int{section}
	for len(stack) > 0 {
		cur := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		c, _ := t.Section(cur)
		for _, n := range c.Next {
			if seen[n] {
				continue
			}
			ns, ok := t.Section(n)
			if !ok || ns.Switch {
				continue
			}
			seen[n] = true
			stack = append(stack, n)
		}
	}
	return sortedKeys(seen)
}

// Adjacent returns the sections linked to the run of sections given, plus the
// sections linked across exactly one switch from any of them. The run itself
// is excluded. The result is sorted.
func (t *Topology) Adjacent(sections []int) []int {
	run := make(map[int]bool, len(sections))
	for _, s := range sections {
		run[s] = true
	}
	out := map[int]bool{}
	for _, s := range sections {
		sec, ok := t.Section(s)
		if !ok {
			continue
		}
		for _, n := range sec.Next {
			if run[n] {
				continue
			}
			ns, ok := t.Section(n)
			if !ok {
				continue
			}
			out[n] = true
			if !ns.Switch {
				continue
			}
			for _, m := range ns.Next {
				if _, known := t.pos[m]; known && !run[m] {
					out[m] = true
				}
			}
		}
	}
	return sortedKeys(out)
}

// Leg returns the section a switch leads to: next[1] for the normal leg and
// next[2] for the reverse leg. It reports false for a section that is not a
// switch or has no such leg.
func (t *Topology) Leg(sw int, reversed bool) (int, bool) {
	s, ok := t.Section(sw)
	if !ok || !s.Switch {
		return 0, false
	}
	i := 1
	if reversed {
		i = 2
	}
	if i >= len(s.Next) {
		return 0, false
	}
	return s.Next[i], true
}

// Path returns the shortest path from one section to another over next links,
// both ends included, holding at most maxSections sections. Ties go to the
// link listed first. It reports false when there is no such path.
func (t *Topology) Path(from, to int, maxSections int) ([]int, bool) {
	if maxSections < 1 {
		return nil, false
	}
	if _, ok := t.pos[from]; !ok {
		return nil, false
	}
	if _, ok := t.pos[to]; !ok {
		return nil, false
	}
	if from == to {
		return []int{from}, true
	}
	parent := map[int]int{from: from}
	layer := []int{from}
	for depth := 1; depth < maxSections && len(layer) > 0; depth++ {
		var nextLayer []int
		for _, cur := range layer {
			c, _ := t.Section(cur)
			for _, n := range c.Next {
				if _, seen := parent[n]; seen {
					continue
				}
				if _, known := t.pos[n]; !known {
					continue
				}
				parent[n] = cur
				if n == to {
					path := []int{to}
					for p := cur; ; p = parent[p] {
						path = append(path, p)
						if p == from {
							break
						}
					}
					for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
						path[i], path[j] = path[j], path[i]
					}
					return path, true
				}
				nextLayer = append(nextLayer, n)
			}
		}
		layer = nextLayer
	}
	return nil, false
}

func sortedKeys(m map[int]bool) []int {
	out := make([]int, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Ints(out)
	return out
}
