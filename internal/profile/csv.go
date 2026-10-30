package profile

import (
	"bufio"
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// csvFixedFields is the number of trailing CSV columns after the industry
// name: incoming tag, local symbol, car type, time, capacity, produces and
// outgoing tags.
const csvFixedFields = 7

// LoadIndustryCSV parses a Run8 IndustryConfiguration.csv.
//
// The file has a title row, a header row that starts with "Industry Name",
// then one row per industry car type. A row with a blank name continues the
// industry above it. Tags are trimmed and have internal whitespace collapsed.
// Outgoing tags keep file order and keep bad or duplicate entries so that the
// validator can report them.
//
// Some industry names contain an unquoted comma ("Schnitzer Steel Industries,
// Inc"). The seven columns after the name are fixed, so the name is whatever
// precedes them.
func LoadIndustryCSV(r io.Reader) ([]Industry, error) {
	br := bufio.NewReader(r)
	if b, err := br.Peek(3); err == nil && bytes.Equal(b, []byte{0xEF, 0xBB, 0xBF}) {
		_, _ = br.Discard(3)
	}
	cr := csv.NewReader(br)
	cr.FieldsPerRecord = -1
	cr.LazyQuotes = true

	var (
		out        []Industry
		headerSeen bool
	)
	for {
		rec, err := cr.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("industry csv: %w", err)
		}
		line, _ := cr.FieldPos(0)
		if !headerSeen {
			headerSeen = strings.EqualFold(strings.TrimSpace(rec[0]), "Industry Name")
			continue
		}
		if allBlank(rec) {
			continue
		}
		if len(rec) < csvFixedFields+1 {
			return nil, fmt.Errorf("industry csv line %d: want %d fields, got %d", line, csvFixedFields+1, len(rec))
		}
		split := len(rec) - csvFixedFields
		name := strings.TrimSpace(strings.Join(rec[:split], ","))
		f := rec[split:]

		car, err := parseIndustryCar(f)
		if err != nil {
			return nil, fmt.Errorf("industry csv line %d: %w", line, err)
		}
		if name == "" {
			if len(out) == 0 {
				return nil, fmt.Errorf("industry csv line %d: continuation row before any industry", line)
			}
			cur := &out[len(out)-1]
			cur.Cars = append(cur.Cars, car)
			continue
		}
		out = append(out, Industry{
			Name:        name,
			Tag:         NormTag(f[0]),
			LocalSymbol: NormTag(f[1]),
			Cars:        []IndustryCar{car},
		})
	}
	if !headerSeen {
		return nil, errors.New("industry csv: header row (\"Industry Name,...\") not found")
	}
	return out, nil
}

// parseIndustryCar reads the car-type columns f[2:] of a CSV row.
func parseIndustryCar(f []string) (IndustryCar, error) {
	hours, err := parseOptionalInt(f[3])
	if err != nil {
		return IndustryCar{}, fmt.Errorf("time %q: %w", f[3], err)
	}
	capacity, err := parseOptionalInt(f[4])
	if err != nil {
		return IndustryCar{}, fmt.Errorf("capacity %q: %w", f[4], err)
	}
	return IndustryCar{
		CarType:      CarType(strings.TrimSpace(f[2])),
		TimeHours:    hours,
		Capacity:     capacity,
		Produces:     strings.TrimSpace(f[5]),
		OutgoingTags: splitTags(f[6]),
	}, nil
}

// splitTags splits a comma-separated tag list, normalising each tag and
// dropping empty entries.
func splitTags(s string) []string {
	var out []string
	for _, t := range strings.Split(s, ",") {
		if t = NormTag(t); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// parseOptionalInt parses an integer with optional thousands separators. A
// blank cell is 0.
func parseOptionalInt(s string) (int, error) {
	s = strings.ReplaceAll(strings.TrimSpace(s), ",", "")
	if s == "" {
		return 0, nil
	}
	return strconv.Atoi(s)
}

func allBlank(rec []string) bool {
	for _, f := range rec {
		if strings.TrimSpace(f) != "" {
			return false
		}
	}
	return true
}
