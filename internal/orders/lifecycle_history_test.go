package orders

import "testing"

func TestLifecycleHistoryConfirmedAndCancelledOrders(t *testing.T) {
	// Order 1: detected then confirmed. Order 2: cancelled. Order 3: still in progress.
	o1 := lcMustStart(lcAvailAB45(), "industry:HTI", "ord-1")
	o1 = Reconcile(o1, lcStatus(locs{"h1": lcInd("HTI"), "h2": lcInd("HTI"), "hp": lcYard("NOR", "1")}), lcProfile(), lcClock(30))
	if o1.State != StateCompletionDetected {
		t.Fatalf("o1 state = %s", o1.State)
	}
	o1, err := Apply(o1, ActConfirm, lcClock(40))
	if err != nil {
		t.Fatal(err)
	}

	o2, err := Start(lcAvailA55(), "", "ord-2", lcClock(5))
	if err != nil {
		t.Fatal(err)
	}
	o2, err = Apply(o2, ActCancel, lcClock(50))
	if err != nil {
		t.Fatal(err)
	}
	o3 := lcMustStart(lcAvail131(), "", "ord-3")

	hist := History([]Order{o1, o3, o2})
	if len(hist) != 2 {
		t.Fatalf("history has %d entries, want 2", len(hist))
	}
	// Most recently finished first: the cancelled order (minute 50) then the confirmed one (minute 40).
	if hist[0].ID != "ord-2" || hist[1].ID != "ord-1" {
		t.Fatalf("history order = %s, %s", hist[0].ID, hist[1].ID)
	}

	conf, canc := hist[1], hist[0]
	if conf.State != StateCompleted || canc.State != StateCancelled {
		t.Errorf("states = %s, %s", conf.State, canc.State)
	}
	if !conf.StartedAt.Equal(lcClk0.Real) || !conf.GameStarted.Equal(lcClk0.Game) ||
		!conf.FinishedAt.Equal(lcClock(40).Real) || !conf.GameFinished.Equal(lcClock(40).Game) {
		t.Errorf("confirmed timestamps: %v %v %v %v", conf.StartedAt, conf.GameStarted, conf.FinishedAt, conf.GameFinished)
	}
	if !canc.StartedAt.Equal(lcClock(5).Real) || !canc.GameStarted.Equal(lcClock(5).Game) ||
		!canc.FinishedAt.Equal(lcClock(50).Real) || !canc.GameFinished.Equal(lcClock(50).Game) {
		t.Errorf("cancelled timestamps: %v %v %v %v", canc.StartedAt, canc.GameStarted, canc.FinishedAt, canc.GameFinished)
	}
	// Entries keep kind, job and car list.
	if conf.Kind != KindLocal || conf.JobID != "ab45" || len(conf.CarKeys()) != 3 {
		t.Errorf("confirmed entry: kind %s job %s cars %v", conf.Kind, conf.JobID, conf.CarKeys())
	}
	if canc.JobID != "a55" || len(canc.CarKeys()) != 3 {
		t.Errorf("cancelled entry: job %s cars %v", canc.JobID, canc.CarKeys())
	}
	if Fingerprint(conf) == "" || !conf.Lines[0].Done {
		t.Error("confirmed entry should keep its final progress")
	}
}

func TestLifecycleHistoryIsFinished(t *testing.T) {
	for s, want := range map[State]bool{
		StateAvailable: false, StateInProgress: false, StateCompletionDetected: false,
		StateCompleted: true, StateCancelled: true,
	} {
		o := Order{State: s}
		if IsFinished(&o) != want {
			t.Errorf("IsFinished(%s) = %v", s, !want)
		}
	}
}
