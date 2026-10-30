package watch

import (
	"context"
	"os"
	"time"

	"run8-ops-center/internal/run8save"
)

// stamp identifies the on-disk state of a save and its sidecar. Two equal
// stamps mean neither file changed in size or modification time.
type stamp struct {
	size, mod         int64
	infoSize, infoMod int64 // zero when there is no sidecar
}

// stampOf stats the save at xmlPath and its sidecar. A missing save is an
// error; a missing sidecar is not.
func stampOf(xmlPath string) (stamp, error) {
	fi, err := os.Stat(xmlPath)
	if err != nil {
		return stamp{}, err
	}
	s := stamp{size: fi.Size(), mod: fi.ModTime().UnixNano()}
	if ii, err := os.Stat(run8save.InfoPath(xmlPath)); err == nil {
		s.infoSize, s.infoMod = ii.Size(), ii.ModTime().UnixNano()
	}
	return s, nil
}

// waitStable returns once the stamp of xmlPath is unchanged across two checks
// settle apart, retrying until it is or ctx is done. It returns the stable
// stamp, or an error when ctx ends or the file vanishes.
func waitStable(ctx context.Context, xmlPath string, settle time.Duration) (stamp, error) {
	prev, err := stampOf(xmlPath)
	if err != nil {
		return stamp{}, err
	}
	timer := time.NewTimer(settle)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return stamp{}, ctx.Err()
		case <-timer.C:
		}
		cur, err := stampOf(xmlPath)
		if err != nil {
			return stamp{}, err
		}
		if cur == prev {
			return cur, nil
		}
		prev = cur
		timer.Reset(settle)
	}
}

// writeTime is the sidecar FileTimestamp when present, else the file's
// modification time. It matches run8save.Snapshot.WriteTime.
func writeTime(xmlPath string) (time.Time, error) {
	fi, err := os.Stat(xmlPath)
	if err != nil {
		return time.Time{}, err
	}
	if info, err := run8save.ReadInfo(run8save.InfoPath(xmlPath)); err == nil && !info.FileTimestamp.IsZero() {
		return info.FileTimestamp, nil
	}
	return fi.ModTime(), nil
}
