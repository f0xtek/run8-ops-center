package orders

import (
	"testing"

	"run8-ops-center/internal/carstatus"
)

type locs = map[string]carstatus.Location

func lcFlag(o Order, code string) (Flag, bool) {
	for _, f := range o.Flags {
		if f.Code == code {
			return f, true
		}
	}
	return Flag{}, false
}

func lcCar(t *testing.T, refs []CarRef, key string) CarRef {
	t.Helper()
	for _, r := range refs {
		if r.Key == key {
			return r
		}
	}
	t.Fatalf("car %s not found", key)
	return CarRef{}
}

// AB45/HTI started as a single line: set-outs h1 h2, pick-up hp.
func lcHTI() Order { return lcMustStart(lcAvailAB45(), "industry:HTI", "ord-1") }

func TestLifecycleReconcileJobWorkedAndReturned(t *testing.T) {
	st := lcStatus(locs{"h1": lcInd("HTI"), "h2": lcInd("HTI"), "hp": lcYard("NOR", "Bowl 3")})
	got := Reconcile(lcHTI(), st, lcProfile(), lcClock(60))
	if got.State != StateCompletionDetected {
		t.Fatalf("state = %s, want completion-detected", got.State)
	}
	if !got.DetectedAt.Equal(lcClock(60).Real) {
		t.Errorf("DetectedAt = %v", got.DetectedAt)
	}
	if !got.Lines[0].Done {
		t.Error("line not done")
	}
	if c := lcCar(t, got.Lines[0].SetOuts, "h1"); !c.Done || c.Location != "industry HTI" {
		t.Errorf("h1 = %+v", c)
	}
	if c := lcCar(t, got.Lines[0].PickUps, "hp"); !c.Done || c.Location != "NOR / Bowl 3" {
		t.Errorf("hp = %+v", c)
	}
}

func TestLifecycleReconcilePartialProgressStaysInProgress(t *testing.T) {
	in := lcHTI()
	st := lcStatus(locs{"h1": lcInd("HTI"), "h2": lcInd("HTI"), "hp": lcOnLine})
	got := Reconcile(in, st, lcProfile(), lcClock(60))
	if got.State != StateInProgress {
		t.Fatalf("state = %s, want in-progress", got.State)
	}
	if !lcCar(t, got.Lines[0].SetOuts, "h1").Done || !lcCar(t, got.Lines[0].SetOuts, "h2").Done {
		t.Error("set-outs at HTI should be done")
	}
	if lcCar(t, got.Lines[0].PickUps, "hp").Done {
		t.Error("pick-up on line should be pending")
	}
	if got.Lines[0].Done {
		t.Error("line should not be done")
	}
	if got.Lines[0].PickUps[0].Location != "on line" {
		t.Errorf("location = %q", got.Lines[0].PickUps[0].Location)
	}
	// The input order is untouched.
	if in.Lines[0].SetOuts[0].Done || in.Lines[0].SetOuts[0].Location != "NOR / Bowl 1" {
		t.Error("Reconcile modified its input")
	}
}

func TestLifecycleReconcileSetOutAtWrongIndustryNotDone(t *testing.T) {
	st := lcStatus(locs{"h1": lcInd("BRL"), "h2": lcInd("HTI"), "hp": lcYard("NOR", "1")})
	got := Reconcile(lcHTI(), st, lcProfile(), lcClock(1))
	if got.State != StateInProgress || lcCar(t, got.Lines[0].SetOuts, "h1").Done {
		t.Errorf("h1 at BRL must not count for HTI: %+v", got.Lines[0].SetOuts)
	}
}

func TestLifecycleReconcilePickUpInOtherYardNotDone(t *testing.T) {
	st := lcStatus(locs{"h1": lcInd("HTI"), "h2": lcInd("HTI"), "hp": lcYard("TUS", "1")})
	got := Reconcile(lcHTI(), st, lcProfile(), lcClock(1))
	if got.State != StateInProgress || got.Lines[0].PickUps[0].Done {
		t.Errorf("pick-up in TUS is not in the home yard NOR: %+v", got.Lines[0].PickUps)
	}
}

func TestLifecycleReconcileYardLineCompleted(t *testing.T) {
	o := lcMustStart(lcAvailA55(), "", "ord-1")
	// t1 on Tuscaloosa yard tracks, t2 already spotted at GES (served by A37, homed at TUS),
	// pick-up on Norris yard tracks.
	st := lcStatus(locs{"t1": lcYard("TUS", "Track 2"), "t2": lcInd("GES"), "tp": lcYard("NOR", "Bowl 9")})
	got := Reconcile(o, st, lcProfile(), lcClock(20))
	if !got.Lines[0].Done {
		t.Fatalf("Tuscaloosa line not done: %+v", got.Lines[0])
	}
	if got.State != StateCompletionDetected {
		t.Errorf("state = %s", got.State)
	}

	// An industry not served from TUS does not count as delivered.
	st = lcStatus(locs{"t1": lcYard("TUS", "Track 2"), "t2": lcInd("HTI"), "tp": lcYard("NOR", "Bowl 9")})
	got = Reconcile(o, st, lcProfile(), lcClock(20))
	if got.Lines[0].Done || got.State != StateInProgress {
		t.Errorf("car at an AB45 industry is not delivered to TUS: %+v", got.Lines[0])
	}
	if lcCar(t, got.Lines[0].SetOuts, "t1").Done != true || lcCar(t, got.Lines[0].SetOuts, "t2").Done {
		t.Errorf("per-car progress wrong: %+v", got.Lines[0].SetOuts)
	}
}

func TestLifecycleReconcileLocalCarVanishesIsMissing(t *testing.T) {
	o := lcMustStart(lcAvailAB45(), "industry:HTI", "ord-1")
	st := lcStatus(locs{"h1": lcInd("HTI"), "hp": lcYard("NOR", "1")}) // h2 gone
	got := Reconcile(o, st, lcProfile(), lcClock(5))
	if got.State != StateInProgress {
		t.Fatalf("state = %s, want in-progress", got.State)
	}
	c := lcCar(t, got.Lines[0].SetOuts, "h2")
	if !c.Missing || c.Done || c.Location != LocOffMap {
		t.Errorf("h2 = %+v", c)
	}
	f, ok := lcFlag(got, FlagMissingCar)
	if !ok || f.Message == "" {
		t.Fatalf("missing-car flag not set: %+v", got.Flags)
	}

	// The car reappears: flag cleared, order detects completion.
	st = lcStatus(locs{"h1": lcInd("HTI"), "h2": lcInd("HTI"), "hp": lcYard("NOR", "1")})
	got = Reconcile(got, st, lcProfile(), lcClock(6))
	if _, ok := lcFlag(got, FlagMissingCar); ok {
		t.Error("missing-car flag should be removed")
	}
	if got.State != StateCompletionDetected {
		t.Errorf("state = %s", got.State)
	}
}

func TestLifecycleReconcileMissingFlagNotDuplicated(t *testing.T) {
	o := lcHTI()
	st := lcStatus(locs{"h1": lcInd("HTI"), "hp": lcYard("NOR", "1")})
	for i := range 3 {
		o = Reconcile(o, st, lcProfile(), lcClock(i))
	}
	n := 0
	for _, f := range o.Flags {
		if f.Code == FlagMissingCar {
			n++
		}
	}
	if n != 1 {
		t.Errorf("missing-car flags = %d, want 1", n)
	}
}

func TestLifecycleReconcileRoadTrainLeavesMap(t *testing.T) {
	o := lcMustStart(lcAvail131(), "", "ord-1")
	// Only the BRI set-outs remain, in McCalla; consist and the TUS pick-up are gone.
	st := lcStatus(locs{"bri1": lcYard("MCC", "Track 4"), "bri2": lcYard("MCC", "Track 5")})
	got := Reconcile(o, st, lcProfile(), lcClock(90))
	if got.State != StateCompletionDetected {
		t.Fatalf("state = %s, want completion-detected; lines %+v consist %+v", got.State, got.Lines, got.Consist)
	}
	if _, ok := lcFlag(got, FlagMissingCar); ok {
		t.Error("no car should be missing")
	}
	if !lcCar(t, got.Consist, "n1").Done || lcCar(t, got.Consist, "n1").Location != LocOffMap {
		t.Errorf("n1 = %+v", lcCar(t, got.Consist, "n1"))
	}
	if !lcCar(t, got.Consist, "bri1").Done {
		t.Error("consist copy of a delivered set-out should be done")
	}
	if !got.Lines[0].Done || !got.Lines[1].Done {
		t.Errorf("lines done = %v %v", got.Lines[0].Done, got.Lines[1].Done)
	}
}

func TestLifecycleReconcileRoadTrainStillOnMapNotDetected(t *testing.T) {
	o := lcMustStart(lcAvail131(), "", "ord-1")
	// Set-outs delivered, but n2 is still on the map and the pick-up has not left.
	st := lcStatus(locs{
		"bri1": lcYard("MCC", "4"), "bri2": lcYard("MCC", "5"),
		"n2": lcOnLine, "tp1": lcYard("TUS", "1"),
	})
	got := Reconcile(o, st, lcProfile(), lcClock(90))
	if got.State != StateInProgress {
		t.Fatalf("state = %s, want in-progress", got.State)
	}
	if lcCar(t, got.Consist, "n2").Done || lcCar(t, got.Lines[1].PickUps, "tp1").Done {
		t.Error("cars still on the map are not done")
	}
	if lcCar(t, got.Consist, "n2").Missing {
		t.Error("a non-set-out car on the map is not missing")
	}
	if !got.Lines[0].Done || got.Lines[1].Done {
		t.Errorf("line done = %v %v", got.Lines[0].Done, got.Lines[1].Done)
	}
}

func TestLifecycleReconcileRoadSetOutCarriedOffMap(t *testing.T) {
	o := lcMustStart(lcAvail131(), "", "ord-1")
	// Everything absent, including the BRI set-outs that never reached McCalla.
	st := lcStatus(locs{})
	got := Reconcile(o, st, lcProfile(), lcClock(90))
	if got.State != StateInProgress {
		t.Fatalf("state = %s, want in-progress", got.State)
	}
	for _, k := range []string{"bri1", "bri2"} {
		c := lcCar(t, got.Lines[0].SetOuts, k)
		if !c.Missing || c.Done {
			t.Errorf("%s = %+v, want missing and not done", k, c)
		}
	}
	if lcCar(t, got.Lines[1].PickUps, "tp1").Missing || lcCar(t, got.Consist, "n1").Missing {
		t.Error("pick-ups and other consist cars absent are done, not missing")
	}
	if _, ok := lcFlag(got, FlagMissingCar); !ok {
		t.Error("missing-car flag expected")
	}

	// A set-out reaches McCalla, then is absent later: it stays done, not missing.
	st = lcStatus(locs{"bri1": lcYard("MCC", "4"), "bri2": lcYard("MCC", "5")})
	got = Reconcile(got, st, lcProfile(), lcClock(91))
	if got.State != StateCompletionDetected {
		t.Fatalf("after delivery state = %s", got.State)
	}
}

func TestLifecycleReconcileRoadIndustryStop(t *testing.T) {
	o := Order{
		ID: "ord-1", Kind: KindRoad, JobID: "339", Symbol: "339", State: StateInProgress,
		Consist: lcRefs("g1"),
		Lines:   []Line{{Kind: LineStop, Target: "GES", SetOuts: lcRefs("g1")}},
	}
	got := Reconcile(o, lcStatus(locs{"g1": lcInd("GES")}), lcProfile(), lcClock(1))
	if got.State != StateCompletionDetected {
		t.Errorf("at the industry: state = %s", got.State)
	}
	got = Reconcile(o, lcStatus(locs{"g1": lcYard("TUS", "1")}), lcProfile(), lcClock(1))
	if got.State != StateInProgress {
		t.Errorf("in the TUS yard is not at the GES industry stop: state = %s", got.State)
	}
}

func TestLifecycleReconcileOnlyInProgressOrdersChange(t *testing.T) {
	st := lcStatus(locs{"h1": lcInd("HTI"), "h2": lcInd("HTI"), "hp": lcYard("NOR", "1")})
	for _, s := range []State{StateAvailable, StateCompletionDetected, StateCompleted, StateCancelled} {
		in := lcInState(t, s)
		got := Reconcile(in, st, lcProfile(), lcClock(1))
		if got.State != s || got.Lines[0].SetOuts[0].Location != in.Lines[0].SetOuts[0].Location || got.Lines[0].SetOuts[0].Done {
			t.Errorf("state %s: order changed: %+v", s, got.Lines[0].SetOuts[0])
		}
	}
}

func TestLifecycleReconcileContinueSuppressesUntilCarMoves(t *testing.T) {
	done := locs{"h1": lcInd("HTI"), "h2": lcInd("HTI"), "hp": lcYard("NOR", "1")}
	o := Reconcile(lcHTI(), lcStatus(done), lcProfile(), lcClock(10))
	if o.State != StateCompletionDetected {
		t.Fatalf("first detection: %s", o.State)
	}
	o, err := Apply(o, ActContinue, lcClock(11))
	if err != nil {
		t.Fatal(err)
	}
	if o.State != StateInProgress || o.SuppressFP == "" {
		t.Fatalf("after continue: %s fp %q", o.State, o.SuppressFP)
	}

	// An identical snapshot (and a second one) is not re-detected.
	for i := range 2 {
		o = Reconcile(o, lcStatus(done), lcProfile(), lcClock(12+i))
		if o.State != StateInProgress {
			t.Fatalf("identical snapshot %d re-detected: %s", i, o.State)
		}
		if o.SuppressFP == "" {
			t.Fatal("SuppressFP cleared by an identical snapshot")
		}
		if !o.Lines[0].Done {
			t.Error("progress should still be tracked while suppressed")
		}
	}

	// A car moves (pick-up goes to another Norris track): re-detected.
	moved := locs{"h1": lcInd("HTI"), "h2": lcInd("HTI"), "hp": lcYard("NOR", "2")}
	o = Reconcile(o, lcStatus(moved), lcProfile(), lcClock(20))
	if o.State != StateCompletionDetected {
		t.Fatalf("after a car moved: %s, want completion-detected", o.State)
	}
	if o.SuppressFP != "" {
		t.Errorf("SuppressFP = %q, want cleared", o.SuppressFP)
	}
	if !o.DetectedAt.Equal(lcClock(20).Real) {
		t.Errorf("DetectedAt = %v", o.DetectedAt)
	}
}

func TestLifecycleReconcileSuppressionClearsEvenWhenNotComplete(t *testing.T) {
	done := locs{"h1": lcInd("HTI"), "h2": lcInd("HTI"), "hp": lcYard("NOR", "1")}
	o := Reconcile(lcHTI(), lcStatus(done), lcProfile(), lcClock(10))
	o, _ = Apply(o, ActContinue, lcClock(11))
	// The pick-up goes back out on line: fingerprint differs, suppression cleared, order in progress.
	o = Reconcile(o, lcStatus(locs{"h1": lcInd("HTI"), "h2": lcInd("HTI"), "hp": lcOnLine}), lcProfile(), lcClock(12))
	if o.State != StateInProgress || o.SuppressFP != "" {
		t.Errorf("state %s fp %q", o.State, o.SuppressFP)
	}
	// Returning to the done layout now detects again.
	o = Reconcile(o, lcStatus(done), lcProfile(), lcClock(13))
	if o.State != StateCompletionDetected {
		t.Errorf("state = %s", o.State)
	}
}

func TestLifecycleReconcileRoadTrainLeavingAfterContinueRedetects(t *testing.T) {
	st := lcStatus(locs{"bri1": lcYard("MCC", "4"), "bri2": lcYard("MCC", "5"), "n1": lcOnLine, "n2": lcOnLine, "tp1": lcOnLine})
	o := lcMustStart(lcAvail131(), "", "ord-1")
	o = Reconcile(o, st, lcProfile(), lcClock(1))
	if o.State != StateInProgress {
		t.Fatalf("state = %s", o.State)
	}
	// The remaining cars leave the map: detected (absence changes the fingerprint).
	st = lcStatus(locs{"bri1": lcYard("MCC", "4"), "bri2": lcYard("MCC", "5")})
	o = Reconcile(o, st, lcProfile(), lcClock(2))
	if o.State != StateCompletionDetected {
		t.Fatalf("state = %s", o.State)
	}
	o, _ = Apply(o, ActContinue, lcClock(3))
	o = Reconcile(o, st, lcProfile(), lcClock(4))
	if o.State != StateInProgress {
		t.Errorf("identical snapshot re-detected: %s", o.State)
	}
}

func TestLifecycleReconcileNilStatusOrProfileUnchanged(t *testing.T) {
	o := lcHTI()
	if got := Reconcile(o, nil, lcProfile(), lcClk0); got.State != StateInProgress || got.Lines[0].Done {
		t.Error("nil status should leave the order alone")
	}
	if got := Reconcile(o, lcStatus(locs{}), nil, lcClk0); got.State != StateInProgress || got.Lines[0].SetOuts[0].Missing {
		t.Error("nil profile should leave the order alone")
	}
}
