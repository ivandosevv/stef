package hub_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ivandosevv/stef/daemon/hub"
	"github.com/ivandosevv/stef/daemon/telemetry"
)

func setMode(t *testing.T, h *hub.Hub, mode telemetry.UpdateMode) {
	t.Helper()
	settings, err := h.SetUpdateMode(mode)
	if err != nil || settings.UpdateMode != mode {
		t.Fatalf("set mode: %+v %v", settings, err)
	}
}

func TestUpdateModePersistsAcrossChatsAndClearWithoutDeletingHistory(t *testing.T) {
	journal := filepath.Join(t.TempDir(), "events.jsonl")
	h, err := hub.New("personal", journal)
	if err != nil {
		t.Fatal(err)
	}
	if h.Settings().UpdateMode != telemetry.UpdatesFiles {
		t.Fatal("an existing prototype must keep every-file mode by default")
	}
	observe(t, h, "chat-a", "a.ts", "file")
	setMode(t, h, telemetry.UpdatesMilestones)
	if _, err := h.ActivateSession("chat-b"); err != nil {
		t.Fatal(err)
	}
	if got := h.Snapshot(); got.UpdateMode != telemetry.UpdatesMilestones || len(got.Events) != 0 {
		t.Fatalf("new chat reset shared preference: %+v", got)
	}
	if _, err := h.ClearSession("chat-b"); err != nil {
		t.Fatal(err)
	}
	setMode(t, h, telemetry.UpdatesDisabled)
	if _, err := h.ActivateSession("chat-a"); err != nil {
		t.Fatal(err)
	}
	if err := h.Close(); err != nil {
		t.Fatal(err)
	}
	h, err = hub.New("personal", journal)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	if got := h.Snapshot(); got.UpdateMode != telemetry.UpdatesDisabled || got.SessionID != "chat-a" || len(got.Events) != 1 {
		t.Fatalf("recovery lost policy or history: %+v", got)
	}
	setMode(t, h, telemetry.UpdatesFiles)
	if pending, count := h.PendingChanges("chat-a", 50); len(pending) != 1 || count != 1 {
		t.Fatal("mode switch deleted earlier retained observations")
	}
}

func TestModeBroadcastsToAllSubscribersAndRejectsInvalidValues(t *testing.T) {
	h := newHub(t)
	a, stopA := h.Subscribe()
	defer stopA()
	b, stopB := h.Subscribe()
	defer stopB()
	<-a
	<-b
	setMode(t, h, telemetry.UpdatesMilestones)
	for _, stream := range []<-chan hub.Message{a, b} {
		if message := <-stream; message.Type != "snapshot" || message.UpdateMode != telemetry.UpdatesMilestones {
			t.Fatalf("view did not receive replacement policy: %+v", message)
		}
	}
	for _, invalid := range []telemetry.UpdateMode{"", "off", "everything"} {
		if _, err := h.SetUpdateMode(invalid); err == nil {
			t.Fatalf("invalid mode accepted: %q", invalid)
		}
	}
	setMode(t, h, telemetry.UpdatesMilestones)
	select {
	case message := <-a:
		t.Fatalf("unchanged/invalid mode published a new snapshot: %+v", message)
	default:
	}
}

func TestMilestonesSuppressPassiveAndLinkedReportsButAllowMajorChanges(t *testing.T) {
	h := newHub(t)
	first := observe(t, h, "local", "theme.ts", "file")
	setMode(t, h, telemetry.UpdatesMilestones)
	stream, stop := h.Subscribe()
	defer stop()
	<-stream
	if e := observe(t, h, "local", "main.ts", "file"); e.ID != "" {
		t.Fatal("suppressed watcher save received an ID")
	}
	hook, _ := (telemetry.Activity{AgentID: "host", Phase: "editing", NodeName: "main.ts"}).Event()
	linked := event("agent", "theme.ts", "ready")
	linked.ChangeID = first.ID
	stale := linked
	stale.ChangeID = "99999"
	for _, input := range []telemetry.Event{hook, linked, stale} {
		stored, err := h.Publish(input)
		if err != nil || stored.ID != "" {
			t.Fatalf("suppressed report errored or persisted: %+v %v", stored, err)
		}
	}
	if batch, err := h.ExplainChanges("local", "agent", []telemetry.ChangeExplanation{explanation("99999")}); err != nil || len(batch) != 0 {
		t.Fatalf("late batch must be ignored: %+v %v", batch, err)
	}
	if pending, count := h.PendingChanges("local", 50); len(pending) != 0 || count != 0 {
		t.Fatal("milestones requested detailed explanation coverage")
	}
	select {
	case message := <-stream:
		t.Fatalf("suppressed activity was broadcast: %+v", message)
	default:
	}
	for _, action := range []string{"scouted", "modified", "created", "deleted", "tested"} {
		e := event("agent", "workflow", "ready")
		e.Action = action
		stored, err := h.Publish(e)
		if err != nil || stored.ID == "" {
			t.Fatalf("major %s milestone rejected: %+v %v", action, stored, err)
		}
	}
	if len(h.Snapshot().Events) != 6 {
		t.Fatal("policy either deleted earlier history or stored suppressed activity")
	}
}

func TestDisabledSuppressesActivityAndFeedbackWithoutConsumingIt(t *testing.T) {
	h := newHub(t)
	note, _ := (telemetry.Feedback{Kind: "question", Text: "Check retries?", TargetAgent: "*"}).Event()
	if _, err := h.Publish(note); err != nil {
		t.Fatal(err)
	}
	setMode(t, h, telemetry.UpdatesDisabled)
	for _, input := range []telemetry.Event{event("agent", "main.ts", "ready"), note} {
		stored, err := h.Publish(input)
		if err != nil || stored.ID != "" {
			t.Fatalf("disabled activity persisted: %+v %v", stored, err)
		}
	}
	observe(t, h, "local", "suppressed.ts", "file")
	if notes := h.OfferFeedback("agent"); len(notes) != 0 {
		t.Fatal("disabled mode delivered developer feedback")
	}
	if _, count := h.PendingChanges("local", 50); count != 0 {
		t.Fatal("disabled mode requested file explanations")
	}
	setMode(t, h, telemetry.UpdatesFiles)
	if notes := h.OfferFeedback("agent"); len(notes) != 1 {
		t.Fatal("disabled check-in consumed old feedback")
	}
	next, err := h.Publish(event("agent", "main.ts", "ready"))
	if err != nil || next.ID != "2" || len(h.Snapshot().Events) != 2 {
		t.Fatalf("disabled events consumed history IDs: %+v %v", next, err)
	}
}

func TestInvalidPersistedPolicyFailsRecovery(t *testing.T) {
	journal := filepath.Join(t.TempDir(), "events.jsonl")
	if err := os.WriteFile(journal, []byte("{\"stef_control\":\"settings\",\"update_mode\":\"off\"}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if h, err := hub.New("personal", journal); err == nil {
		h.Close()
		t.Fatal("invalid persisted policy silently fell back to recording")
	}
}
