package hub_test

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/ivandosevv/stef/daemon/hub"
	"github.com/ivandosevv/stef/daemon/telemetry"
)

func TestReminderFiltersBeforeLimitAndKeepsFeedbackUntouched(t *testing.T) {
	h := newHub(t)
	for i := 0; i < 60; i++ {
		observe(t, h, "chat-a", fmt.Sprintf("unrelated-%d.ts", i), "file")
	}
	first := observe(t, h, "chat-a", "main.ts", "file")
	for i := 1; i < 60; i++ {
		observe(t, h, "chat-a", "main.ts", "file")
	}
	observe(t, h, "chat-a", "main.ts#run", "function")
	observe(t, h, "chat-b", "main.ts", "file")
	private := observe(t, h, "chat-a", "hidden.ts", "file")
	private.NodeName, private.ID = "main.ts", ""
	private.Before, private.After, private.Diff = "private old code", "private new code", "private diff"
	if _, err := h.Publish(private); err != nil {
		t.Fatal(err)
	}
	note, _ := (telemetry.Feedback{SessionID: "chat-a", TargetAgent: "agent", Kind: "question", Text: "private developer note"}).Event()
	note, err := h.Publish(note)
	if err != nil {
		t.Fatal(err)
	}
	before := h.SessionSnapshot("chat-a")
	state, err := h.ExplanationReminder("chat-a", []string{"main.ts", "main.ts"})
	if err != nil || state.PendingCount != 61 || len(state.PendingChanges) != 50 || state.PendingChanges[0].ChangeID != first.ID {
		t.Fatalf("unrelated backlog starved edited file: %+v %v", state, err)
	}
	if state.SessionID != "chat-a" || state.Repo != before.Repo || state.UpdateMode != telemetry.UpdatesFiles || state.UpdatesFrom != before.UpdatesFrom {
		t.Fatalf("incorrect reporting snapshot: %+v", state)
	}
	for _, change := range state.PendingChanges {
		if change.NodeName != "main.ts" || change.Action != "modified" {
			t.Fatalf("unrelated change leaked: %+v", change)
		}
	}
	raw, _ := json.Marshal(state)
	for _, forbidden := range []string{"private", "before", "after", "diff", "feedback", "summary", "why", "events"} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatalf("reminder exposed %s: %s", forbidden, raw)
		}
	}
	if after := h.SessionSnapshot("chat-a"); len(after.Events) != len(before.Events) || h.Snapshot().SessionID != "local" {
		t.Fatal("reminder published an event or activated another chat")
	}
	if notes := h.OfferFeedback("agent", "chat-a"); len(notes) != 1 || notes[0].ID != note.ID {
		t.Fatal("reminder consumed pending feedback")
	}
}

func TestReminderExactCoverageWindowRecoveryAndClear(t *testing.T) {
	journal := filepath.Join(t.TempDir(), "events.jsonl")
	h, err := hub.New("personal", journal)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { h.Close() }()
	observe(t, h, "chat-a", "main.ts", "file")
	setMode(t, h, telemetry.UpdatesMilestones)
	setMode(t, h, telemetry.UpdatesFiles)
	first := observe(t, h, "chat-a", "main.ts", "file")
	second := observe(t, h, "chat-a", "main.ts", "file")
	third := observe(t, h, "chat-a", "main.ts", "file")
	other := observe(t, h, "chat-b", "main.ts", "file")
	if _, err := h.ExplainChanges("chat-a", "agent", []telemetry.ChangeExplanation{explanation(first.ID)}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Publish(contextReply(second)); err != nil {
		t.Fatal(err)
	}
	assertPending := func() {
		t.Helper()
		state, err := h.ExplanationReminder("chat-a", []string{"main.ts"})
		if err != nil || state.PendingCount != 1 || len(state.PendingChanges) != 1 || state.PendingChanges[0].ChangeID != third.ID {
			t.Fatalf("wrong exact revision or window coverage: %+v %v", state, err)
		}
	}
	assertPending()
	if err := h.Close(); err != nil {
		t.Fatal(err)
	}
	h, err = hub.New("personal", journal)
	if err != nil {
		t.Fatal(err)
	}
	assertPending()
	for _, mode := range []telemetry.UpdateMode{telemetry.UpdatesMilestones, telemetry.UpdatesDisabled} {
		setMode(t, h, mode)
		state, err := h.ExplanationReminder("chat-a", []string{"main.ts"})
		if err != nil || state.UpdateMode != mode || state.PendingCount != 0 || len(state.PendingChanges) != 0 {
			t.Fatalf("suppressed mode returned file reminders: %+v %v", state, err)
		}
	}
	setMode(t, h, telemetry.UpdatesFiles)
	fresh := observe(t, h, "chat-b", "main.ts", "file")
	if _, err := h.ActivateSession("chat-b"); err != nil {
		t.Fatal(err)
	}
	state, err := h.ExplanationReminder("", []string{"main.ts"})
	if err != nil || state.SessionID != "chat-b" || state.PendingCount != 1 || state.PendingChanges[0].ChangeID != fresh.ID || state.PendingChanges[0].ChangeID == other.ID {
		t.Fatalf("active chat fallback crossed a window: %+v %v", state, err)
	}
	if _, err := h.ClearSession("chat-b"); err != nil {
		t.Fatal(err)
	}
	state, err = h.ExplanationReminder("chat-b", []string{"main.ts"})
	if err != nil || state.PendingCount != 0 || len(state.PendingChanges) != 0 {
		t.Fatalf("cleared revisions remained pending: %+v %v", state, err)
	}
}

func TestReminderRejectsInvalidInputWithoutEffects(t *testing.T) {
	h := newHub(t)
	for _, input := range []struct {
		session string
		paths   []string
	}{{" bad-session", []string{"main.ts"}}, {"local", nil}, {"local", []string{"../main.ts"}}, {"local", []string{"main.ts#run"}}} {
		if _, err := h.ExplanationReminder(input.session, input.paths); err == nil {
			t.Fatalf("invalid reminder accepted: %+v", input)
		}
	}
	state, err := h.ExplanationReminder("unknown-chat", []string{"main.ts"})
	if err != nil || state.PendingCount != 0 || state.SessionID != "unknown-chat" || len(h.Snapshot().Events) != 0 || h.Snapshot().SessionID != "local" {
		t.Fatal("unknown chat lookup had effects")
	}
}

func TestReminderSnapshotStaysConsistentAcrossConcurrentModeChanges(t *testing.T) {
	h := newHub(t)
	var writers sync.WaitGroup
	writers.Add(1)
	go func() {
		defer writers.Done()
		for i := 0; i < 100; i++ {
			setMode(t, h, telemetry.UpdatesMilestones)
			setMode(t, h, telemetry.UpdatesFiles)
			observe(t, h, "local", "main.ts", "file")
		}
	}()
	for i := 0; i < 200; i++ {
		state, err := h.ExplanationReminder("local", []string{"main.ts"})
		if err != nil {
			t.Fatal(err)
		}
		if state.UpdateMode != telemetry.UpdatesFiles && (state.PendingCount != 0 || len(state.PendingChanges) != 0) {
			t.Fatalf("mode and pending state came from different snapshots: %+v", state)
		}
		from, _ := strconv.ParseUint(state.UpdatesFrom, 10, 64)
		for _, change := range state.PendingChanges {
			id, _ := strconv.ParseUint(change.ChangeID, 10, 64)
			if id < from {
				t.Fatalf("window and pending IDs came from different snapshots: %+v", state)
			}
		}
	}
	writers.Wait()
}
