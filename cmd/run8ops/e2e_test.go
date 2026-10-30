package main

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"run8-ops-center/internal/orders"
	"run8-ops-center/internal/run8save"
	"run8-ops-center/internal/store"
)

// withoutTrain returns the save XML with the TrainLoader holding trainID cut out.
func withoutTrain(t *testing.T, xml []byte, trainID string) []byte {
	t.Helper()
	marker := []byte("<trainID>" + trainID + "</trainID>")
	i := bytes.Index(xml, marker)
	if i < 0 {
		t.Fatalf("train %s not found", trainID)
	}
	start := bytes.LastIndex(xml[:i], []byte("<TrainLoader>"))
	endTag := []byte("</TrainLoader>")
	end := bytes.Index(xml[i:], endTag)
	if start < 0 || end < 0 {
		t.Fatalf("train %s has no TrainLoader element", trainID)
	}
	out := append([]byte{}, xml[:start]...)
	return append(out, xml[i+end+len(endTag):]...)
}

func postForm(t *testing.T, client *http.Client, target string, v url.Values) (int, string) {
	t.Helper()
	resp, err := client.PostForm(target, v)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var b bytes.Buffer
	b.ReadFrom(resp.Body)
	return resp.StatusCode, b.String()
}

// storedOrder returns the persisted order for a road train symbol, or nil.
func storedOrder(t *testing.T, data, symbol string) *orders.Order {
	t.Helper()
	st, err := store.LoadState(data)
	if err != nil {
		t.Fatal(err)
	}
	for _, list := range st.ProfileOrders {
		for i := range list {
			if list[i].JobID == symbol {
				return &list[i]
			}
		}
	}
	return nil
}

func waitFor(t *testing.T, within time.Duration, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(150 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// TestOrderLifecycleThroughRealBinary follows one road train order from a world
// save to a confirmed completion: label the yard, start 108, then drop an
// auto-save in which the train has left the map, and confirm what the app
// detects.
func TestOrderLifecycleThroughRealBinary(t *testing.T) {
	saves, autos, data := saveFolders(t)
	saveFolderBefore := tree(t, saves)

	// The train that holds the 108 cars (tag CHA) in the reference save.
	snap, err := run8save.ParseFile(filepath.Join(testdata, "WorldSave.xml"))
	if err != nil {
		t.Fatal(err)
	}
	var trainIDs []string
	chaTrain := ""
	for _, tr := range snap.Trains {
		trainIDs = append(trainIDs, tr.ID)
		for _, v := range tr.Vehicles {
			if !v.IsLoco() && strings.TrimSpace(v.Tag) == "CHA" {
				chaTrain = tr.ID
			}
		}
	}
	if chaTrain == "" {
		t.Fatal("reference save has no CHA cars")
	}

	ready := make(chan string, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var out, errb bytes.Buffer
	done := make(chan int, 1)
	go func() {
		done <- run(ctx, []string{
			"--save", "WorldSave", "--save-dir", saves, "--autosave-dir", autos, "--data", data,
			"--port", fmt.Sprint(freePort(t)), "--no-browser",
		}, &out, &errb, options{ListenReady: func(u string) { ready <- u }})
	}()
	var base string
	select {
	case base = <-ready:
	case code := <-done:
		t.Fatalf("run exited early with %d:\n%s", code, errb.String())
	case <-time.After(15 * time.Second):
		t.Fatal("server never started")
	}
	if !pageEventuallyContains(t, base, "458", 15*time.Second) {
		t.Fatalf("named save never ingested:\n%s", fetch(t, base))
	}
	client := &http.Client{Timeout: 15 * time.Second}

	// Learn mode: label every train's sections as one yard track.
	for _, id := range trainIDs {
		code, body := postForm(t, client, base+"trackmap/label", url.Values{
			"kind": {"yard"}, "yard": {"NOR"}, "track": {"All"}, "train": {id},
		})
		if code >= 500 {
			t.Fatalf("labelling train %s: %d\n%s", id, code, body)
		}
	}
	if !pageEventuallyContains(t, base+"orders", "108", 10*time.Second) {
		t.Fatalf("order for 108 never became available:\n%s", fetch(t, base+"orders"))
	}

	// Start 108.
	if code, body := postForm(t, client, base+"orders/avail:108/start", url.Values{}); code != http.StatusOK {
		t.Fatalf("starting 108: %d\n%s", code, body)
	}
	var started *orders.Order
	waitFor(t, 10*time.Second, "108 persisted as in progress", func() bool {
		started = storedOrder(t, data, "108")
		return started != nil && started.State == orders.StateInProgress
	})

	// Listen for live updates, then drop an auto-save with the train gone.
	req, _ := http.NewRequestWithContext(ctx, "GET", base+"events", nil)
	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	live := make(chan string, 16)
	go func() {
		sc := bufio.NewScanner(resp.Body)
		for sc.Scan() {
			if strings.HasPrefix(sc.Text(), "event:") {
				live <- strings.TrimSpace(strings.TrimPrefix(sc.Text(), "event:"))
			}
		}
	}()

	worldXML, err := os.ReadFile(filepath.Join(testdata, "WorldSave.xml"))
	if err != nil {
		t.Fatal(err)
	}
	name := "Auto Save World 2026.10.30 19.15.00"
	if err := os.WriteFile(filepath.Join(autos, name+".xml"), withoutTrain(t, worldXML, chaTrain), 0o644); err != nil {
		t.Fatal(err)
	}
	info := `<?xml version="1.0"?>
<ScnInfo xmlns:xsd="http://www.w3.org/2001/XMLSchema" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <SaveType>World</SaveType>
  <Filename>` + filepath.Join(autos, name+".xml") + `</Filename>
  <DisplayFilename>` + name + `</DisplayFilename>
  <FileTimestamp>2026-10-30T19:15:00.0000000+00:00</FileTimestamp>
</ScnInfo>`
	if err := os.WriteFile(filepath.Join(autos, name+".info"), []byte(info), 0o644); err != nil {
		t.Fatal(err)
	}

	waitFor(t, 20*time.Second, "108 completion detected", func() bool {
		o := storedOrder(t, data, "108")
		return o != nil && o.State == orders.StateCompletionDetected
	})
	select {
	case ev := <-live:
		if ev != "view-changed" {
			t.Errorf("unexpected live event %q", ev)
		}
	case <-time.After(5 * time.Second):
		t.Error("no live update reached an open /events stream")
	}
	detail := fetch(t, base+"orders/"+started.ID)
	for _, want := range []string{"confirm", "continue"} {
		if !strings.Contains(strings.ToLower(detail), want) {
			t.Errorf("order page lacks the %q action:\n%s", want, detail)
		}
	}

	// Confirm.
	if code, body := postForm(t, client, base+"orders/"+started.ID+"/confirm", url.Values{}); code != http.StatusOK {
		t.Fatalf("confirming: %d\n%s", code, body)
	}
	waitFor(t, 10*time.Second, "108 completed in state.json", func() bool {
		o := storedOrder(t, data, "108")
		return o != nil && o.State == orders.StateCompleted && !o.FinishedAt.IsZero()
	})

	cancel()
	select {
	case code := <-done:
		if code != 0 {
			t.Fatalf("exit code %d:\n%s", code, errb.String())
		}
	case <-time.After(15 * time.Second):
		t.Fatal("did not stop")
	}
	if after := tree(t, saves); after != saveFolderBefore {
		t.Fatalf("the save folder changed:\nbefore:\n%s\nafter:\n%s", saveFolderBefore, after)
	}
}
