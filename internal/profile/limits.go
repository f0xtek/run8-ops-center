package profile

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// limitRow is one data row of the limits file.
type limitRow struct {
	Symbol string
	Codes  []string
	Dest   string
	Limit  Limit
}

// limitColumns is the number of columns in the limits table.
const limitColumns = 7

// LoadLimits parses a Run8 yard_and_train_limits.txt style file.
//
// The table is tab-separated with columns base train symbol, class codes,
// destination, minimum HP per ton, maximum length, maximum tons and maximum
// trailing tons. The first line is the header. The table ends at the first
// blank line, so the subdivision table below it is ignored. Numbers may carry
// thousands separators ("4,000"). A blank numeric cell means no limit (zero).
// The maximum trailing tons column is read but not returned.
//
// The results are keyed by base train symbol: limits, and class codes.
func LoadLimits(r io.Reader) (map[string]Limit, map[string][]string, error) {
	rows, err := parseLimitRows(r)
	if err != nil {
		return nil, nil, err
	}
	limits := make(map[string]Limit, len(rows))
	codes := make(map[string][]string, len(rows))
	for _, row := range rows {
		limits[row.Symbol] = row.Limit
		codes[row.Symbol] = row.Codes
	}
	return limits, codes, nil
}

// parseLimitRows reads the table into rows in file order.
func parseLimitRows(r io.Reader) ([]limitRow, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	var rows []limitRow
	lineNo := 0
	for sc.Scan() {
		lineNo++
		line := strings.TrimRight(sc.Text(), "\r")
		if lineNo == 1 {
			line = strings.TrimPrefix(line, "\ufeff")
			continue // header
		}
		if strings.TrimSpace(line) == "" {
			break
		}
		cells := strings.Split(line, "\t")
		for len(cells) < limitColumns {
			cells = append(cells, "")
		}
		sym := NormTag(cells[0])
		if sym == "" {
			return nil, fmt.Errorf("limits line %d: blank train symbol", lineNo)
		}
		hpt, err := parseOptionalFloat(cells[3])
		if err != nil {
			return nil, fmt.Errorf("limits line %d: min HP/T %q: %w", lineNo, cells[3], err)
		}
		length, err := parseOptionalInt(cells[4])
		if err != nil {
			return nil, fmt.Errorf("limits line %d: max length %q: %w", lineNo, cells[4], err)
		}
		tons, err := parseOptionalInt(cells[5])
		if err != nil {
			return nil, fmt.Errorf("limits line %d: max tons %q: %w", lineNo, cells[5], err)
		}
		if _, err := parseOptionalInt(cells[6]); err != nil {
			return nil, fmt.Errorf("limits line %d: max trailing tons %q: %w", lineNo, cells[6], err)
		}
		rows = append(rows, limitRow{
			Symbol: sym,
			Codes:  splitTags(cells[1]),
			Dest:   strings.TrimSpace(cells[2]),
			Limit:  Limit{MinHPT: hpt, MaxLengthFt: length, MaxTons: tons},
		})
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("limits: %w", err)
	}
	return rows, nil
}

// parseOptionalFloat parses a number with optional thousands separators. A
// blank cell is 0.
func parseOptionalFloat(s string) (float64, error) {
	s = strings.ReplaceAll(strings.TrimSpace(s), ",", "")
	if s == "" {
		return 0, nil
	}
	return strconv.ParseFloat(s, 64)
}
