package hub_test

import (
	"path/filepath"
	"testing"

	"github.com/ivandosevv/stef/daemon/hub"
	"github.com/ivandosevv/stef/daemon/telemetry"
)

func TestChatHistoryAndClearSurviveRestart(t *testing.T) {
	journal := filepath.Join(t.TempDir(), "events.jsonl")
	h, err := hub.New("personal", journal)
	if err != nil {
		t.Fatal(err)
	}
	activate := func(id string) {
		t.Helper()
		if _, err := h.ActivateSession(id); err != nil {
			t.Fatal(err)
		}
	}
	publish := func(node string) {
		t.Helper()
		if _, err := h.Publish(event("agent", node, "ready")); err != nil {
			t.Fatal(err)
		}
	}
	activate("chat-a")
	publish("a.go")
	activate("chat-b")
	if len(h.Snapshot().Events) != 0 {
		t.Fatal("new chat inherited another chat's workflow")
	}
	publish("b.go")
	activate("chat-a")
	if got := h.Snapshot(); len(got.Events) != 1 || got.Events[0].NodeName != "a.go" {
		t.Fatalf("resume lost workflow: %+v", got)
	}
	if _, err := h.ClearSession("chat-a"); err != nil {
		t.Fatal(err)
	}
	publish("fresh.go")
	if err := h.Close(); err != nil {
		t.Fatal(err)
	}
	h, err = hub.New("personal", journal)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	if got := h.Snapshot(); got.SessionID != "chat-a" || len(got.Events) != 1 || got.Events[0].NodeName != "fresh.go" {
		t.Fatalf("restart resurrected cleared steps: %+v", got)
	}
	activate("chat-b")
	if got := h.Snapshot(); len(got.Events) != 1 || got.Events[0].NodeName != "b.go" {
		t.Fatalf("clear damaged another chat: %+v", got)
	}
}

func TestLifecycleStartAndResumeSwitchTheLiveView(t *testing.T) {
	h := newHub(t)
	stream, stop := h.Subscribe()
	defer stop()
	<-stream
	start := func(id string) {
		t.Helper()
		e, err := (telemetry.Activity{SessionID: id, AgentID: "agent", Phase: "started"}).Event()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := h.Publish(e); err != nil {
			t.Fatal(err)
		}
	}
	start("chat-a")
	if snapshot := <-stream; snapshot.Type != "snapshot" || snapshot.SessionID != "chat-a" || len(snapshot.Events) != 0 {
		t.Fatalf("new chat wasn't reset: %+v", snapshot)
	}
	<-stream
	if _, err := h.Publish(event("agent", "a.go", "ready")); err != nil {
		t.Fatal(err)
	}
	<-stream
	start("chat-b")
	<-stream
	<-stream
	start("chat-a")
	if snapshot := <-stream; snapshot.SessionID != "chat-a" || len(snapshot.Events) != 2 {
		t.Fatalf("resume didn't restore: %+v", snapshot)
	}
	<-stream
	start("chat-a")
	if message := <-stream; message.Type != "event" {
		t.Fatal("a new turn reset the same chat")
	}
	e := event("background", "background.go", "ready")
	e.SessionID = "chat-b"
	if _, err := h.Publish(e); err != nil {
		t.Fatal(err)
	}
	if got := h.Snapshot(); got.SessionID != "chat-a" || len(got.Events) != 4 {
		t.Fatalf("background work changed active chat: %+v", got)
	}
	if got := h.SessionSnapshot("chat-b"); len(got.Events) != 2 {
		t.Fatal("background work was lost")
	}
}

func TestFeedbackAndClaimsStayInsideTheirChat(t *testing.T) {
	h := newHub(t)
	if _, err := h.ActivateSession("chat-a"); err != nil {
		t.Fatal(err)
	}
	note, err := (telemetry.Feedback{Kind: "suggestion", Text: "Check blanks", TargetAgent: "*"}).Event()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.Publish(note); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Publish(event("alpha", "main.go", "running")); err != nil {
		t.Fatal(err)
	}
	if _, err := h.ActivateSession("chat-b"); err != nil {
		t.Fatal(err)
	}
	e, err := h.Publish(event("beta", "main.go", "running"))
	if err != nil {
		t.Fatal(err)
	}
	if len(e.Collisions) != 0 {
		t.Fatal("claims leaked across chats")
	}
	if notes := h.OfferFeedback("beta"); len(notes) != 0 {
		t.Fatal("a developer note leaked across chats")
	}
	if notes := h.OfferFeedback("alpha", "chat-a"); len(notes) != 1 {
		t.Fatal("the original chat lost its note")
	}
}
