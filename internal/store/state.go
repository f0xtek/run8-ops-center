package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"run8-ops-center/internal/carstatus"
	"run8-ops-center/internal/orders"
)

// StateFile is the file name of the operations state inside the data directory.
const StateFile = "state.json"

// StateVersion is the current state.json format version.
const StateVersion = 1

// MaxActivity caps State.Activity.
const MaxActivity = 2000

// ActivityEntry is one line of the activity log.
type ActivityEntry struct {
	Time    time.Time `json:"time"` // real time
	Game    time.Time `json:"game"` // in-game date
	Kind    string    `json:"kind"`
	Message string    `json:"message"`
}

// State is the content of state.json.
type State struct {
	Version int `json:"version"`
	// ProfileOrders holds the non-available orders (started, completed,
	// cancelled) by profile ID. Orders of inactive profiles are kept.
	ProfileOrders map[string][]orders.Order `json:"profileOrders"`
	// Seq is the counter behind started-order IDs ("ord-<n>").
	Seq int `json:"seq"`
	// Dwell holds the per-car dwell records by profile ID.
	Dwell map[string]carstatus.DwellState `json:"dwell"`
	// Activity is the activity log, newest first, at most MaxActivity long.
	Activity []ActivityEntry `json:"activity"`
}

// NewState returns an empty state with non-nil maps.
func NewState() State {
	return State{
		Version:       StateVersion,
		ProfileOrders: map[string][]orders.Order{},
		Dwell:         map[string]carstatus.DwellState{},
	}
}

// AppendActivity adds entries to the log. Pass them in the order they
// happened: the last argument becomes the newest entry, at index 0. The log is
// then cut to MaxActivity entries, dropping the oldest.
func (s *State) AppendActivity(entries ...ActivityEntry) {
	if len(entries) == 0 {
		return
	}
	n := min(len(entries)+len(s.Activity), MaxActivity)
	out := make([]ActivityEntry, 0, n)
	for i := len(entries) - 1; i >= 0 && len(out) < n; i-- {
		out = append(out, entries[i])
	}
	for _, e := range s.Activity {
		if len(out) == n {
			break
		}
		out = append(out, e)
	}
	s.Activity = out
}

// LoadState reads state.json from dir. A missing file yields an empty state.
func LoadState(dir string) (State, error) {
	path := filepath.Join(dir, StateFile)
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return NewState(), nil
	}
	if err != nil {
		return State{}, fmt.Errorf("read state: %w", err)
	}
	st := NewState()
	if err := json.Unmarshal(data, &st); err != nil {
		return State{}, fmt.Errorf("parse %s: %w", path, err)
	}
	if st.Version > StateVersion {
		return State{}, fmt.Errorf("%s is version %d, newer than the supported version %d", path, st.Version, StateVersion)
	}
	if st.ProfileOrders == nil {
		st.ProfileOrders = map[string][]orders.Order{}
	}
	if st.Dwell == nil {
		st.Dwell = map[string]carstatus.DwellState{}
	}
	if len(st.Activity) > MaxActivity {
		st.Activity = st.Activity[:MaxActivity]
	}
	st.Version = StateVersion
	return st, nil
}

// SaveState writes state.json into dir atomically through the guard. The
// activity log is cut to MaxActivity entries first.
func SaveState(g *Guard, dir string, st State) error {
	st.Version = StateVersion
	if len(st.Activity) > MaxActivity {
		st.Activity = st.Activity[:MaxActivity]
	}
	return saveJSON(g, filepath.Join(dir, StateFile), st)
}
