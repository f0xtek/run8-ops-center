package run8route

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"strings"
)

var errShort = errors.New("unexpected end of data")

// reader is a bounded little-endian reader over a whole file. The first
// failure sticks: later reads return zero values, so a decoder can read a
// whole record and check err once.
type reader struct {
	b    []byte
	size int
	err  error
}

func newReader(b []byte) *reader { return &reader{b: b, size: len(b)} }

func (r *reader) offset() int { return r.size - len(r.b) }

func (r *reader) fail(format string, args ...any) {
	if r.err == nil {
		r.err = fmt.Errorf("offset %d: %s", r.offset(), fmt.Sprintf(format, args...))
	}
}

func (r *reader) take(n int) []byte {
	if r.err != nil {
		return nil
	}
	if n < 0 || n > len(r.b) {
		r.fail("%v", errShort)
		return nil
	}
	s := r.b[:n]
	r.b = r.b[n:]
	return s
}

func (r *reader) skip(n int) { r.take(n) }

func (r *reader) u8() byte {
	s := r.take(1)
	if s == nil {
		return 0
	}
	return s[0]
}

func (r *reader) i16() int {
	s := r.take(2)
	if s == nil {
		return 0
	}
	return int(int16(binary.LittleEndian.Uint16(s)))
}

func (r *reader) i32() int {
	s := r.take(4)
	if s == nil {
		return 0
	}
	return int(int32(binary.LittleEndian.Uint32(s)))
}

func (r *reader) f32() float32 {
	s := r.take(4)
	if s == nil {
		return 0
	}
	return math.Float32frombits(binary.LittleEndian.Uint32(s))
}

func (r *reader) f64() float64 {
	s := r.take(8)
	if s == nil {
		return 0
	}
	return math.Float64frombits(binary.LittleEndian.Uint64(s))
}

// count reads an int32 record count and checks it against the bytes left,
// given the smallest size one record can have. A count that cannot fit is
// rejected before anything is allocated for it.
func (r *reader) count(minRecord int, what string) int {
	n := r.i32()
	if r.err != nil {
		return 0
	}
	if n < 0 {
		r.fail("negative %s count %d", what, n)
		return 0
	}
	if n > len(r.b)/minRecord {
		r.fail("%s count %d does not fit in the %d bytes left", what, n, len(r.b))
		return 0
	}
	return n
}

// str reads an R8String: an int32 byte count, then two bytes per character.
// A pair b0 b1 encodes the character b0<<4 | b1>>4.
func (r *reader) str() string {
	n := r.i32()
	if r.err != nil {
		return ""
	}
	if n < 0 || n > len(r.b) {
		r.fail("string length %d does not fit in the %d bytes left", n, len(r.b))
		return ""
	}
	if n%2 != 0 {
		r.fail("string length %d is odd", n)
		return ""
	}
	s := r.take(n)
	var sb strings.Builder
	sb.Grow(n / 2)
	for i := 0; i < n; i += 2 {
		sb.WriteRune(rune(s[i])<<4 | rune(s[i+1])>>4)
	}
	return sb.String()
}

// finish reports the first failure, or trailing bytes after the last record.
func (r *reader) finish() error {
	if r.err != nil {
		return r.err
	}
	if len(r.b) != 0 {
		return fmt.Errorf("offset %d: %d trailing bytes after the last record", r.offset(), len(r.b))
	}
	return nil
}
