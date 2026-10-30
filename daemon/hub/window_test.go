package hub_test

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/ivandosevv/stef/daemon/hub"
	"github.com/ivandosevv/stef/daemon/telemetry"
)

func TestWindowPersistsOrderedBoundaryAndNoOpWithoutBackfill(t *testing.T) {
	journal := filepath.Join(t.TempDir(), "events.jsonl")
	h, err := hub.New("personal", journal)
	if err != nil {
		t.Fatal(err)
	}
	if h.Snapshot().UpdatesFrom != "0" {
		t.Fatal("legacy window must include all history")
	}
	old := observe(t, h, "chat-a", "old.ts", "file")
	setMode(t, h, telemetry.UpdatesMilestones)
	if h.Snapshot().UpdatesFrom != "2" {
		t.Fatal("boundary must be inclusive first next ID")
	}
	milestone := event("agent", "feature", "ready")
	milestone.SessionID = "chat-a"
	if _, err := h.Publish(milestone); err != nil {
		t.Fatal(err)
	}
	setMode(t, h, telemetry.UpdatesMilestones)
	if h.Snapshot().UpdatesFrom != "2" {
		t.Fatal("no-op settings advanced window")
	}
	setMode(t, h, telemetry.UpdatesFiles)
	fresh := observe(t, h, "chat-a", "fresh.ts", "file")
	if fresh.ID != "3" || h.Snapshot().UpdatesFrom != fresh.ID {
		t.Fatal("new file does not start new reporting window")
	}
	if err := h.Close(); err != nil {
		t.Fatal(err)
	}
	h, err = hub.New("personal", journal)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	if snapshot := h.SessionSnapshot("chat-a"); snapshot.UpdatesFrom != fresh.ID || len(snapshot.Events) != 3 {
		t.Fatalf("restart lost boundary or history: %+v", snapshot)
	}
	if pending, count := h.PendingChanges("chat-a", 50); count != 1 || len(pending) != 1 || pending[0].ChangeID != fresh.ID {
		t.Fatalf("restart backfilled old files: %+v %d", pending, count)
	}
	if _, err := h.ExplainChanges("chat-a", "agent", []telemetry.ChangeExplanation{explanation(fresh.ID), explanation(old.ID)}); err == nil {
		t.Fatal("automatic batch accepted an old-window file")
	}
	if len(h.SessionSnapshot("chat-a").Events) != 3 {
		t.Fatal("mixed-window batch had partial effects")
	}
	explicit := event("agent", old.NodeName, "ready")
	explicit.SessionID, explicit.ChangeID = "chat-a", old.ID
	if _, err := h.Publish(explicit); err != nil {
		t.Fatal("explicit retained-file clarification rejected", err)
	}
	if snapshot, err := h.ActivateSession("chat-a"); err != nil || snapshot.UpdatesFrom != fresh.ID {
		t.Fatal("session switch reset the recording boundary")
	}
	if snapshot, err := h.ClearSession("chat-a"); err != nil || snapshot.UpdatesFrom != fresh.ID {
		t.Fatal("clear reset the recording boundary")
	}
}

func TestLegacySettingsRecoverBoundaryAndIgnoreRepeatedModes(t *testing.T) {
	journal := filepath.Join(t.TempDir(), "events.jsonl")
	// Old journal settings lack an explicit boundary; ordered event IDs supply it.
	data := "{\"id\":\"41\",\"session_id\":\"local\",\"source\":\"watcher\",\"kind\":\"file\",\"node_name\":\"old.ts\",\"action\":\"modified\"}\n" +
		"{\"stef_control\":\"settings\",\"update_mode\":\"milestones\"}\n" +
		"{\"id\":\"42\",\"session_id\":\"local\",\"source\":\"agent\",\"node_name\":\"feature\",\"action\":\"modified\"}\n" +
		"{\"stef_control\":\"settings\",\"update_mode\":\"milestones\"}\n"
	if err := os.WriteFile(journal, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	h, err := hub.New("personal", journal)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	if h.Snapshot().UpdatesFrom != "42" {
		t.Fatalf("legacy ordered recovery changed window: %+v", h.Snapshot())
	}
	setMode(t, h, telemetry.UpdatesFiles)
	if h.Snapshot().UpdatesFrom != strconv.FormatUint(43, 10) {
		t.Fatal("recovery did not retain next event ID")
	}
	if _, count := h.PendingChanges("local", 50); count != 0 {
		t.Fatal("legacy old file was automatically requested")
	}
}
