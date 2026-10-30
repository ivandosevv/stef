package hub_test

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/ivandosevv/stef/daemon/hub"
	"github.com/ivandosevv/stef/daemon/telemetry"
)

func observe(t *testing.T, h *hub.Hub, session, node, kind string) telemetry.Event {
	t.Helper()
	e, err := h.Publish(telemetry.Event{SessionID: session, AgentID: "filesystem", NodeName: node, Kind: kind, Action: "modified", Summary: "File modified", Why: "A saved file changed.", Status: "ready", Source: "watcher"})
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func explanation(id string) telemetry.ChangeExplanation {
	return telemetry.ChangeExplanation{ChangeID: id, Summary: "Restore saved appearance", Why: "Apply the saved choice before the interface mounts."}
}

func TestReasonsCoverExactRevisionsAndIgnoreLegacyMilestones(t *testing.T) {
	h := newHub(t)
	first := observe(t, h, "local", "main.ts", "file")
	second := observe(t, h, "local", "main.ts", "file")
	observe(t, h, "local", "main.ts#run", "function")
	if _, err := h.Publish(event("agent", "main.ts", "ready")); err != nil {
		t.Fatal(err)
	}
	if pending, count := h.PendingChanges("local", 50); count != 2 || len(pending) != 2 {
		t.Fatalf("legacy report or symbol changed coverage: %+v %d", pending, count)
	}
	stored, err := h.ExplainChanges("local", "agent", []telemetry.ChangeExplanation{explanation(second.ID)})
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != 1 || stored[0].ChangeID != second.ID || stored[0].NodeName != second.NodeName || stored[0].Action != second.Action {
		t.Fatalf("wrong explanation target: %+v", stored)
	}
	if pending, count := h.PendingChanges("local", 50); count != 1 || len(pending) != 1 || pending[0].ChangeID != first.ID {
		t.Fatalf("the older revision was incorrectly covered: %+v %d", pending, count)
	}
}

func TestLinkedPublicationRejectsInvalidObservationTargets(t *testing.T) {
	h := newHub(t)
	file := observe(t, h, "chat-a", "main.ts", "file")
	symbol := observe(t, h, "chat-a", "main.ts#run", "function")
	hook, err := h.Publish(telemetry.Event{SessionID: "chat-a", AgentID: "client", NodeName: "main.ts", Action: "modified", Summary: "Editing file", Why: "Tool activity was observed.", Source: "hook", Category: "activity", Phase: "editing"})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, session, id, node, action, source string }{
		{"unknown ID", "chat-a", "999", "main.ts", "modified", "agent"},
		{"another chat", "chat-b", file.ID, "main.ts", "modified", "agent"},
		{"symbol", "chat-a", symbol.ID, "main.ts#run", "modified", "agent"},
		{"hook", "chat-a", hook.ID, "main.ts", "modified", "agent"},
		{"different path", "chat-a", file.ID, "other.ts", "modified", "agent"},
		{"different action", "chat-a", file.ID, "main.ts", "created", "agent"},
		{"watcher source", "chat-a", file.ID, "main.ts", "modified", "watcher"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := event("agent", tc.node, "ready")
			e.SessionID = tc.session
			e.ChangeID = tc.id
			e.Action = tc.action
			e.Source = tc.source
			if _, err := h.Publish(e); err == nil {
				t.Fatal("invalid change link accepted")
			}
		})
	}
	e := event("agent", file.NodeName, "ready")
	e.SessionID = "chat-a"
	e.ChangeID = file.ID
	if _, err := h.Publish(e); err != nil {
		t.Fatal("valid link rejected", err)
	}
	if _, err := h.ClearSession("chat-a"); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Publish(e); err == nil {
		t.Fatal("cleared observation could be explained")
	}
}

func TestExplanationBatchIsAtomicAndRecoversFromJournal(t *testing.T) {
	journal := filepath.Join(t.TempDir(), "events.jsonl")
	h, err := hub.New("repo", journal)
	if err != nil {
		t.Fatal(err)
	}
	first := observe(t, h, "local", "theme.ts", "file")
	second := observe(t, h, "local", "main.ts", "file")
	invalid := explanation(second.ID)
	invalid.Summary = "one two three four five six"
	if _, err := h.ExplainChanges("local", "agent", []telemetry.ChangeExplanation{explanation(first.ID), invalid}); err == nil {
		t.Fatal("invalid batch accepted")
	}
	if len(h.Snapshot().Events) != 2 {
		t.Fatal("invalid batch had partial effects")
	}
	if _, err := h.ExplainChanges("local", "agent", []telemetry.ChangeExplanation{explanation(first.ID), explanation(first.ID)}); err == nil {
		t.Fatal("duplicate batch accepted")
	}
	if _, err := h.ExplainChanges("local", "agent", []telemetry.ChangeExplanation{explanation(first.ID), explanation(second.ID)}); err != nil {
		t.Fatal(err)
	}
	if err := h.Close(); err != nil {
		t.Fatal(err)
	}
	h, err = hub.New("repo", journal)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	if len(h.Snapshot().Events) != 4 {
		t.Fatal("batch journal lost records")
	}
	if pending, count := h.PendingChanges("local", 50); count != 0 || len(pending) != 0 {
		t.Fatalf("recovery lost exact coverage: %+v %d", pending, count)
	}
}

func TestPendingChangesBoundedWithoutLosingTotal(t *testing.T) {
	h := newHub(t)
	for i := 0; i < 60; i++ {
		observe(t, h, "local", fmt.Sprintf("file-%d.ts", i), "file")
	}
	pending, count := h.PendingChanges("local", 50)
	if len(pending) != 50 || count != 60 || pending[0].ChangeID != "1" {
		t.Fatalf("incorrect bounded coverage: %d %d %+v", len(pending), count, pending)
	}
}
