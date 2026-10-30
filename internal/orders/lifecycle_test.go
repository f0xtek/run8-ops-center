package orders

import (
	"errors"
	"testing"
)

// started returns a started AB45 order (whole job) in the given state.
func lcInState(t *testing.T, s State) Order {
	t.Helper()
	o := lcMustStart(lcAvailAB45(), "", "ord-1")
	switch s {
	case StateAvailable:
		return lcAvailAB45()
	case StateInProgress:
	case StateCompletionDetected:
		o.State = StateCompletionDetected
		o.DetectedAt = lcClock(5).Real
	case StateCompleted, StateCancelled:
		o.State = s
	default:
		t.Fatalf("bad state %q", s)
	}
	return o
}

func TestLifecycleApplyTransitionTable(t *testing.T) {
	states := []State{StateAvailable, StateInProgress, StateCompletionDetected, StateCompleted, StateCancelled}
	actions := []Action{ActStart, ActConfirm, ActContinue, ActComplete, ActCancel}
	allowed := map[State]map[Action]State{
		StateInProgress:         {ActComplete: StateCompleted, ActCancel: StateCancelled},
		StateCompletionDetected: {ActConfirm: StateCompleted, ActContinue: StateInProgress},
	}
	for _, s := range states {
		for _, a := range actions {
			t.Run(string(s)+"/"+string(a), func(t *testing.T) {
				in := lcInState(t, s)
				got, err := Apply(in, a, lcClock(10))
				want, ok := allowed[s][a]
				if !ok {
					if !errors.Is(err, ErrInvalidTransition) {
						t.Fatalf("err = %v, want ErrInvalidTransition", err)
					}
					return
				}
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if got.State != want {
					t.Errorf("state = %s, want %s", got.State, want)
				}
				if in.State != s {
					t.Errorf("input mutated: state %s", in.State)
				}
			})
		}
	}
}

func TestLifecycleApplyFinishedOrdersStampTimes(t *testing.T) {
	for _, tc := range []struct {
		name  string
		state State
		act   Action
		want  State
	}{
		{"manual complete", StateInProgress, ActComplete, StateCompleted},
		{"cancel", StateInProgress, ActCancel, StateCancelled},
		{"confirm", StateCompletionDetected, ActConfirm, StateCompleted},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := lcInState(t, tc.state)
			c := lcClock(30)
			got, err := Apply(in, tc.act, c)
			if err != nil {
				t.Fatal(err)
			}
			if got.State != tc.want || !got.FinishedAt.Equal(c.Real) || !got.GameFinished.Equal(c.Game) {
				t.Errorf("got state %s finished %v game %v", got.State, got.FinishedAt, got.GameFinished)
			}
			if !got.StartedAt.Equal(lcClk0.Real) || !got.GameStarted.Equal(lcClk0.Game) {
				t.Errorf("start stamps lost: %v %v", got.StartedAt, got.GameStarted)
			}
			if len(got.Lines) != len(in.Lines) || got.ID != in.ID {
				t.Errorf("order not kept intact")
			}
		})
	}
}

func TestLifecycleApplyContinueRecordsFingerprint(t *testing.T) {
	in := lcInState(t, StateCompletionDetected)
	got, err := Apply(in, ActContinue, lcClock(10))
	if err != nil {
		t.Fatal(err)
	}
	if got.SuppressFP == "" || got.SuppressFP != Fingerprint(in) {
		t.Errorf("SuppressFP = %q, want fingerprint %q", got.SuppressFP, Fingerprint(in))
	}
	if !got.DetectedAt.IsZero() {
		t.Errorf("DetectedAt should be cleared, got %v", got.DetectedAt)
	}
	if !got.FinishedAt.IsZero() {
		t.Errorf("continue must not finish the order")
	}
}

func TestLifecycleFingerprintTracksLocationsOnly(t *testing.T) {
	o := lcInState(t, StateInProgress)
	fp := Fingerprint(o)
	o.Lines[0].SetOuts[0].Done = true
	o.Lines[0].Done = true
	if Fingerprint(o) != fp {
		t.Error("fingerprint changed with progress flags")
	}
	o.Lines[0].SetOuts[0].Location = "industry HTI"
	if Fingerprint(o) == fp {
		t.Error("fingerprint did not change when a car moved")
	}
}

func TestLifecycleStartWholeJob(t *testing.T) {
	avail := lcAvailAB45()
	c := lcClock(3)
	got, err := Start(avail, "", "ord-7", c)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "ord-7" || got.State != StateInProgress || got.JobID != "ab45" || got.Symbol != "AB45" {
		t.Errorf("got %+v", got)
	}
	if !got.StartedAt.Equal(c.Real) || !got.GameStarted.Equal(c.Game) {
		t.Errorf("start stamps = %v / %v", got.StartedAt, got.GameStarted)
	}
	if len(got.Lines) != 2 || len(got.CarKeys()) != 4 || got.Totals != avail.Totals {
		t.Errorf("car list not frozen as available: lines %d keys %v totals %+v", len(got.Lines), got.CarKeys(), got.Totals)
	}
	if !got.Reserved() {
		t.Error("started order should reserve its cars")
	}
	if len(got.Shared) != 0 {
		t.Errorf("Shared = %v, want none on a started order", got.Shared)
	}
	for _, f := range got.Flags {
		if f.Code == "shared-cars" {
			t.Error("shared-cars flag kept on started order")
		}
	}
	// Input untouched and not aliased.
	got.Lines[0].SetOuts[0].Key = "changed"
	if avail.State != StateAvailable || avail.ID != "avail:ab45" || avail.Lines[0].SetOuts[0].Key != "h1" {
		t.Error("Start modified or aliased the available order")
	}
}

func TestLifecycleStartRequiresAvailable(t *testing.T) {
	for _, s := range []State{StateInProgress, StateCompletionDetected, StateCompleted, StateCancelled} {
		in := lcInState(t, s)
		if _, err := Start(in, "", "ord-9", lcClk0); !errors.Is(err, ErrInvalidTransition) {
			t.Errorf("Start from %s: err = %v, want ErrInvalidTransition", s, err)
		}
	}
}

func TestLifecycleStartOneIndustryLine(t *testing.T) {
	avail := lcAvailAB45()
	got, err := Start(avail, "industry:HTI", "ord-2", lcClk0)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "ord-2" || got.State != StateInProgress || got.JobID != "ab45" || got.Symbol != "AB45" || got.Kind != KindLocal {
		t.Errorf("got %+v", got)
	}
	if len(got.Lines) != 1 || got.Lines[0].Key() != "industry:HTI" {
		t.Fatalf("lines = %+v", got.Lines)
	}
	if keys := got.CarKeys(); len(keys) != 3 {
		t.Errorf("keys = %v, want h1 h2 hp", keys)
	}
	// Totals from the two HTI set-outs only: 2 x (50 ft, 100 t), HP scaled by the original ratio (2).
	if got.Totals.LengthFt != 100 || got.Totals.GrossTons != 200 || got.Totals.RequiredHP != 400 {
		t.Errorf("totals = %+v", got.Totals)
	}
	if got.Totals.MaxLengthFt != 5000 || got.Totals.MaxTons != 6000 {
		t.Errorf("limits lost: %+v", got.Totals)
	}
	// The over-length flag no longer applies (100 ft < 5000); shared-cars is dropped.
	if len(got.Flags) != 0 {
		t.Errorf("flags = %+v, want none", got.Flags)
	}
	// The caller still holds the full available order (it will regenerate without the line's cars).
	if len(avail.Lines) != 2 || avail.State != StateAvailable {
		t.Error("available order modified")
	}
}

func TestLifecycleStartLineKeepsApplicableFlags(t *testing.T) {
	avail := lcAvailAB45()
	avail.Totals.MaxLengthFt = 80 // HTI set-outs are 100 ft: still over length
	avail.Lines[0].SetOuts[0].Location = "unmapped 4535"
	avail.Flags = append(avail.Flags, Flag{Code: "unmapped", Message: "u"}, Flag{Code: FlagMissingCar, Message: "m"})
	got, err := Start(avail, "industry:HTI", "ord-2", lcClk0)
	if err != nil {
		t.Fatal(err)
	}
	var codes []string
	for _, f := range got.Flags {
		codes = append(codes, f.Code)
	}
	if len(codes) != 2 || codes[0] != "over-length" || codes[1] != "unmapped" {
		t.Errorf("flag codes = %v, want [over-length unmapped]", codes)
	}
	// BRL's single 50 ft set-out fits and has no unmapped car, so those flags are dropped there.
	other, err := Start(avail, "industry:BRL", "ord-3", lcClk0)
	if err != nil {
		t.Fatal(err)
	}
	if len(other.Flags) != 0 {
		t.Errorf("BRL flags = %+v, want none", other.Flags)
	}
}

func TestLifecycleStartOneYardLine(t *testing.T) {
	avail := lcAvailA55()
	got, err := Start(avail, "yard:TUS", "ord-4", lcClk0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Lines) != 1 || got.Lines[0].Kind != LineYard || got.Lines[0].Target != "TUS" {
		t.Fatalf("lines = %+v", got.Lines)
	}
	if got.JobID != "a55" || got.Totals.LengthFt != 100 || got.Totals.GrossTons != 200 || got.Totals.RequiredHP != 400 {
		t.Errorf("got %+v", got)
	}
	if got.State != StateInProgress {
		t.Errorf("state = %s", got.State)
	}
}

func TestLifecycleStartRoadOrderWholeOnly(t *testing.T) {
	avail := lcAvail131()
	if _, err := Start(avail, "stop:MCC", "ord-5", lcClk0); err == nil {
		t.Fatal("starting a single stop of a road order should fail")
	}
	got, err := Start(avail, "", "ord-5", lcClk0)
	if err != nil {
		t.Fatal(err)
	}
	if got.State != StateInProgress || len(got.Consist) != 4 || len(got.Lines) != 2 {
		t.Errorf("got %+v", got)
	}
}

func TestLifecycleStartUnknownLine(t *testing.T) {
	for _, key := range []string{"industry:NOPE", "yard:HTI", "bogus"} {
		if _, err := Start(lcAvailAB45(), key, "ord-6", lcClk0); err == nil {
			t.Errorf("Start with line %q should fail", key)
		}
	}
}

func TestLifecycleStartRejectsEmptyID(t *testing.T) {
	if _, err := Start(lcAvailAB45(), "", "", lcClk0); err == nil {
		t.Error("empty new ID should fail")
	}
}
