package carstatus

import "strings"

// locKey is the stable dwell key of a location. Ignored and unmapped track
// collapse to one key each, so a car travelling over many sections does not
// reset its dwell on every snapshot.
func locKey(l Location) string {
	switch l.Kind {
	case LocYard:
		return "yard:" + l.Yard + "/" + l.Track
	case LocIndustry:
		return "industry:" + strings.Join(l.Industries, "/")
	case LocIgnored:
		return "ignored"
	}
	return "unmapped"
}
