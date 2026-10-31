package hub_test

import (
	"path/filepath"
	"testing"

	"github.com/ivandosevv/stef/daemon/hub"
	"github.com/ivandosevv/stef/daemon/telemetry"
)

func contextReply(target telemetry.Event) telemetry.Event {
	return telemetry.Event{SessionID: target.SessionID, AgentID: "agent", ContextID: target.ID, NodeName: target.NodeName, Action: target.Action, Summary: "Explain selected step", Why: "Keep this decision near the workflow step the developer selected.", Status: "ready", Source: "agent"}
}

func TestRequestedContextLinksRetainedFileMilestoneAndVisibleHook(t *testing.T) {
	journal := filepath.Join(t.TempDir(), "events.jsonl")
	h, err := hub.New("personal", journal)
	if err != nil {
		t.Fatal(err)
	}
	file := observe(t, h, "chat-a", "main.ts", "file")
	milestone := event("agent", "feature", "ready")
	milestone.SessionID = "chat-a"
	storedMilestone, err := h.Publish(milestone)
	if err != nil {
		t.Fatal(err)
	}
	hook, _ := (telemetry.Activity{SessionID: "chat-a", AgentID: "host", Phase: "reading", NodeName: "main.ts"}).Event()
	storedHook, err := h.Publish(hook)
	if err != nil {
		t.Fatal(err)
	}
	setMode(t, h, telemetry.UpdatesMilestones)
	for _, target := range []telemetry.Event{file, storedMilestone, storedHook} {
		reply, err := h.Publish(contextReply(target))
		if err != nil || reply.ID == "" || reply.ContextID != target.ID || reply.ChangeID != "" {
			t.Fatalf("requested retained-step explanation rejected: %+v %v", reply, err)
		}
	}
	if err := h.Close(); err != nil {
		t.Fatal(err)
	}
	h, err = hub.New("personal", journal)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	if events := h.SessionSnapshot("chat-a").Events; len(events) != 6 || events[3].ContextID != file.ID || events[0].Why != file.Why {
		t.Fatalf("context recovery mutated or lost original step: %+v", events)
	}
	setMode(t, h, telemetry.UpdatesDisabled)
	if reply, err := h.Publish(contextReply(file)); err != nil || reply.ID != "" {
		t.Fatal("disabled mode recorded explicit context")
	}
	setMode(t, h, telemetry.UpdatesFiles)
	fresh := observe(t, h, "chat-a", "fresh.ts", "file")
	if _, err := h.Publish(contextReply(fresh)); err != nil {
		t.Fatal(err)
	}
	if pending, count := h.PendingChanges("chat-a", 50); count != 0 || len(pending) != 0 {
		t.Fatal("manual context left a redundant automatic explanation pending")
	}
}

func TestRequestedFileContextCoversOnlyExactCurrentRevisionAndRecovers(t *testing.T) {
	journal := filepath.Join(t.TempDir(), "events.jsonl")
	h, err := hub.New("personal", journal)
	if err != nil {
		t.Fatal(err)
	}
	old := observe(t, h, "chat-a", "old.ts", "file")
	setMode(t, h, telemetry.UpdatesMilestones)
	setMode(t, h, telemetry.UpdatesFiles)
	claim := event("agent", "main.ts", "running")
	claim.SessionID = "chat-a"
	if _, err := h.Publish(claim); err != nil {
		t.Fatal(err)
	}
	target, err := h.Publish(telemetry.Event{SessionID: "chat-a", AgentID: "filesystem", NodeName: "main.ts", Kind: "file", Action: "modified", Summary: "File modified", Why: "A saved file changed.", Status: "ready", Source: "watcher", Before: "old value\n", After: "new value\n", Diff: "-old value\n+new value\n"})
	if err != nil {
		t.Fatal(err)
	}
	otherRevision := observe(t, h, "chat-a", "main.ts", "file")
	otherFile := observe(t, h, "chat-a", "helper.ts", "file")
	for _, selected := range []telemetry.Event{old, target} {
		if _, err := h.Publish(contextReply(selected)); err != nil {
			t.Fatal(err)
		}
	}
	assertPending := func(h *hub.Hub) {
		t.Helper()
		pending, count := h.PendingChanges("chat-a", 50)
		if count != 2 || len(pending) != 2 || pending[0].ChangeID != otherRevision.ID || pending[1].ChangeID != otherFile.ID {
			t.Fatalf("context changed unrelated revision or historical coverage: %+v %d", pending, count)
		}
		for _, stored := range h.SessionSnapshot("chat-a").Events {
			if stored.ID == target.ID && (stored.Before != target.Before || stored.After != target.After || stored.Diff != target.Diff || stored.Why != target.Why) {
				t.Fatalf("context changed captured revision: %+v", stored)
			}
		}
	}
	assertPending(h)
	contender := claim
	contender.AgentID = "other-agent"
	stored, err := h.Publish(contender)
	if err != nil || len(stored.Collisions) != 1 || stored.Collisions[0] != claim.AgentID {
		t.Fatalf("context released a working agent's claim: %+v %v", stored, err)
	}
	if err := h.Close(); err != nil {
		t.Fatal(err)
	}
	h, err = hub.New("personal", journal)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	assertPending(h)
}

func TestRequestedContextHookTargetsMatchVisibleWorkflow(t *testing.T) {
	h := newHub(t)
	for _, tc := range []struct {
		phase, node string
		visible     bool
	}{
		{"reading", "main.ts", true},
		{"searching", "main.ts", true},
		{"editing", "main.ts", true},
		{"testing", "main.ts", true},
		{"failed", "main.ts", true},
		{"testing", "", true},
		{"failed", "", true},
		{"reading", "", false},
		{"searching", "", false},
		{"editing", "", false},
		{"started", "main.ts", false},
		{"terminal", "main.ts", false},
		{"tool_done", "main.ts", false},
		{"completed", "main.ts", false},
	} {
		t.Run(tc.phase+"/"+tc.node, func(t *testing.T) {
			hook, err := (telemetry.Activity{SessionID: "chat-a", AgentID: "host", Phase: tc.phase, NodeName: tc.node}).Event()
			if err != nil {
				t.Fatal(err)
			}
			target, err := h.Publish(hook)
			if err != nil {
				t.Fatal(err)
			}
			_, err = h.Publish(contextReply(target))
			if (err == nil) != tc.visible {
				t.Fatalf("hook context visibility mismatch: %+v, error: %v", target, err)
			}
		})
	}
}

func TestContextRejectsUnsupportedTargetAndMismatchedReferences(t *testing.T) {
	h := newHub(t)
	file := observe(t, h, "chat-a", "main.ts", "file")
	symbol := observe(t, h, "chat-a", "main.ts#run", "function")
	note, _ := (telemetry.Feedback{SessionID: "chat-a", Kind: "question", Text: "Check retries?", TargetAgent: "agent"}).Event()
	note, err := h.Publish(note)
	if err != nil {
		t.Fatal(err)
	}
	explained := event("agent", file.NodeName, "ready")
	explained.SessionID, explained.ChangeID = file.SessionID, file.ID
	explained, err = h.Publish(explained)
	if err != nil {
		t.Fatal(err)
	}
	context, err := h.Publish(contextReply(file))
	if err != nil {
		t.Fatal(err)
	}
	terminal, _ := (telemetry.Activity{SessionID: "chat-a", AgentID: "host", Phase: "terminal", NodeName: "main.ts"}).Event()
	terminal, err = h.Publish(terminal)
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range []telemetry.Event{symbol, note, explained, context, terminal} {
		if _, err := h.Publish(contextReply(target)); err == nil {
			t.Fatalf("unsupported context target accepted: %+v", target)
		}
	}
	for _, mutate := range []func(*telemetry.Event){
		func(e *telemetry.Event) { e.ContextID = "999" },
		func(e *telemetry.Event) { e.SessionID = "chat-b" },
		func(e *telemetry.Event) { e.NodeName = "other.ts" },
		func(e *telemetry.Event) { e.Action = "created" },
		func(e *telemetry.Event) { e.Source = "watcher" },
		func(e *telemetry.Event) { e.ChangeID = file.ID },
	} {
		reply := contextReply(file)
		mutate(&reply)
		if _, err := h.Publish(reply); err == nil {
			t.Fatalf("invalid context reference accepted: %+v", reply)
		}
	}
	if _, err := h.ClearSession("chat-a"); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Publish(contextReply(file)); err == nil {
		t.Fatal("cleared context target accepted")
	}
}
