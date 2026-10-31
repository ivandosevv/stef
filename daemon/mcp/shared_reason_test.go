package mcp_test

import (
	"strings"
	"testing"

	"github.com/ivandosevv/stef/daemon/hub"
	bridge "github.com/ivandosevv/stef/daemon/mcp"
	"github.com/ivandosevv/stef/daemon/telemetry"
)

func TestSharedReasonKeepsBatchValidationAtomic(t *testing.T) {
	h, err := hub.New(t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	for _, session := range []string{"shared-reason-chat", "other-chat"} {
		if _, err := h.RegisterSession(session); err != nil {
			t.Fatal(err)
		}
	}
	observe := func(session, node string) telemetry.Event {
		t.Helper()
		event, err := h.Publish(telemetry.Event{SessionID: session, AgentID: "filesystem", NodeName: node, Action: "modified", Summary: "File modified", Why: "A file save was observed.", Source: "watcher", Kind: "file"})
		if err != nil {
			t.Fatal(err)
		}
		return event
	}
	old := observe("shared-reason-chat", "old.ts")
	if _, err := h.SetSessionUpdateMode("shared-reason-chat", telemetry.UpdatesActivity); err != nil {
		t.Fatal(err)
	}
	first := observe("shared-reason-chat", "first.ts")
	second := observe("shared-reason-chat", "second.ts")
	foreign := observe("other-chat", "foreign.ts")
	cases := []struct {
		name   string
		change func(*bridge.ChangesInput)
	}{
		{"missing reason", func(in *bridge.ChangesInput) { in.Why = "" }},
		{"missing label", func(in *bridge.ChangesInput) { in.Summary = "" }},
		{"blank override", func(in *bridge.ChangesInput) { in.Changes[1].Why = "   " }},
		{"oversized output", func(in *bridge.ChangesInput) { in.Why = strings.Repeat("x", 2001) }},
		{"unknown revision", func(in *bridge.ChangesInput) { in.Changes[1].ChangeID = "99999" }},
		{"different workspace", func(in *bridge.ChangesInput) { in.Changes[1].ChangeID = foreign.ID }},
		{"earlier window", func(in *bridge.ChangesInput) { in.Changes[1].ChangeID = old.ID }},
		{"duplicate revision", func(in *bridge.ChangesInput) { in.Changes[1].ChangeID = first.ID }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input := bridge.ChangesInput{SessionID: "shared-reason-chat", AgentID: "worker", Summary: "Reuse public explanation", Why: "Keep the saved preference available during startup.", Changes: []telemetry.ChangeExplanation{{ChangeID: first.ID}, {ChangeID: second.ID}}}
			tc.change(&input)
			before := len(h.SessionSnapshot(input.SessionID).Events)
			if _, err := bridge.ReportChanges(h, input); err == nil {
				t.Fatal("invalid shared output was accepted")
			}
			_, pending := h.PendingChanges(input.SessionID, 50)
			if len(h.SessionSnapshot(input.SessionID).Events) != before || pending != 2 {
				t.Fatal("invalid shared output partially changed the workflow")
			}
		})
	}
	for _, mode := range []telemetry.UpdateMode{telemetry.UpdatesMilestones, telemetry.UpdatesDisabled} {
		if _, err := h.SetSessionUpdateMode("shared-reason-chat", mode); err != nil {
			t.Fatal(err)
		}
		input := bridge.ChangesInput{SessionID: "shared-reason-chat", AgentID: "worker", Summary: "Reuse public explanation", Why: "No extra file reports in this mode.", Changes: []telemetry.ChangeExplanation{{ChangeID: "99999"}}}
		before := len(h.SessionSnapshot(input.SessionID).Events)
		receipt, err := bridge.ReportChanges(h, input)
		if err != nil || receipt.Accepted || len(receipt.IDs) != 0 || receipt.UpdateMode != mode || len(h.SessionSnapshot(input.SessionID).Events) != before {
			t.Fatalf("shared output bypassed %s mode: %+v %v", mode, receipt, err)
		}
	}
}
