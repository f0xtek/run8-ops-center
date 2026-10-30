package engine

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"run8-ops-center/internal/watch"
)

// TestTruncatedSaveKeepsViewAndWarns is the task 9.2 check: a save that
// fails to parse reaches the engine as watch.Event{Err} and only adds a
// warning.
func TestTruncatedSaveKeepsViewAndWarns(t *testing.T) {
	h := start(t)
	h.ingest(snapshotEvent(parse(t, worldPath), "/saves/WorldSave.xml", false))
	before := h.labelAllYard()
	if _, err := h.eng.StartOrder("avail:131", ""); err != nil {
		t.Fatal(err)
	}
	before = h.eng.View()
	if len(ingestWarnings(before)) != 0 {
		t.Fatalf("warnings before: %+v", before.Warnings)
	}

	full, err := os.ReadFile(worldPath)
	if err != nil {
		t.Fatal(err)
	}
	truncated := filepath.Join(t.TempDir(), "WorldSave.xml")
	if err := os.WriteFile(truncated, full[:len(full)/2], 0o644); err != nil {
		t.Fatal(err)
	}
	_, perr := watchParse(truncated)
	if perr == nil {
		t.Fatal("the truncated fixture parsed")
	}

	h.clock.Advance(1)
	after := h.ingest(watch.Event{Path: truncated, Err: perr})

	if after.Status != before.Status || !after.HasSnapshot {
		t.Error("Status changed after a parse failure")
	}
	if !reflect.DeepEqual(after.Available, before.Available) || !reflect.DeepEqual(after.Active, before.Active) ||
		!reflect.DeepEqual(after.History, before.History) || !reflect.DeepEqual(after.Building, before.Building) {
		t.Error("orders changed after a parse failure")
	}
	if after.Source != before.Source || after.Tiles != before.Tiles || after.Coverage != before.Coverage {
		t.Errorf("source, tiles or coverage changed: %+v vs %+v", after.Source, before.Source)
	}
	if len(after.Activity) != len(before.Activity) {
		t.Error("a parse failure wrote to the activity log")
	}
	if len(ingestWarnings(after)) != 1 || after.Warnings[0].Kind != "parse" || !strings.Contains(after.Warnings[0].Message, "WorldSave.xml") ||
		!strings.Contains(after.Warnings[0].Message, perr.Error()) || !after.Warnings[0].Time.Equal(h.clock.Now()) {
		t.Errorf("warnings = %+v", after.Warnings)
	}
	if after.Seq != before.Seq+1 {
		t.Errorf("Seq = %d, want %d", after.Seq, before.Seq+1)
	}

	// The next good save supersedes the failure and the warning goes.
	good := parse(t, worldPath)
	good.GameDate = good.GameDate.Add(3600e9)
	v := h.ingest(snapshotEvent(good, "/saves/WorldSave.xml", false))
	for _, w := range v.Warnings {
		if w.Kind == "parse" {
			t.Errorf("parse warning survived a good ingest: %+v", w)
		}
	}
	if len(v.Active) != 1 {
		t.Error("the started order was lost")
	}
}

func TestParseFailureBeforeFirstSnapshot(t *testing.T) {
	h := start(t)
	v := h.ingest(watch.Event{Path: "/saves/WorldSave.xml", Err: os.ErrInvalid})
	if v.HasSnapshot || v.Status != nil || v.Source.ActiveFile != "" || len(ingestWarnings(v)) != 1 || v.Warnings[0].Kind != "parse" {
		t.Errorf("HasSnapshot=%v ActiveFile=%q warnings=%+v", v.HasSnapshot, v.Source.ActiveFile, v.Warnings)
	}
}

func TestWatcherWarningsBecomeIngestWarnings(t *testing.T) {
	h := start(t)
	h.ingest(snapshotEvent(parse(t, worldPath), "WorldSave.xml", false))
	before := h.eng.View()

	v := h.ingest(watch.Event{Warning: "auto-save folder X is missing", Path: "X", Auto: true})
	if len(ingestWarnings(v)) != 1 || v.Warnings[0].Kind != "autosave" || v.Warnings[0].Message != "auto-save folder X is missing" {
		t.Fatalf("warnings = %+v", v.Warnings)
	}
	v = h.ingest(watch.Event{Warning: "cannot watch saves"})
	if len(ingestWarnings(v)) != 2 || v.Warnings[0].Kind != "ingest" || v.Warnings[1].Kind != "autosave" {
		t.Fatalf("warnings = %+v", v.Warnings)
	}
	// The same warning again moves to the front instead of piling up.
	v = h.ingest(watch.Event{Warning: "auto-save folder X is missing", Auto: true})
	if len(ingestWarnings(v)) != 2 || v.Warnings[0].Kind != "autosave" {
		t.Fatalf("repeated warning: %+v", v.Warnings)
	}
	if v.Status != before.Status || v.Source != before.Source || !reflect.DeepEqual(v.Available, before.Available) {
		t.Error("a warning-only event changed the snapshot state")
	}
}

func TestRejectedWarningsAreCapped(t *testing.T) {
	h := start(t)
	var v *View
	for i := range 2 * maxWarnings {
		v = h.ingest(watch.Event{Warning: strings.Repeat("w", i+1)})
	}
	if len(v.Warnings) != maxWarnings || len(v.Warnings[0].Message) != 2*maxWarnings {
		t.Errorf("warnings = %d, newest length %d", len(v.Warnings), len(v.Warnings[0].Message))
	}
}
