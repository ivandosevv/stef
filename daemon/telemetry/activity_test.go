package telemetry_test

import (
	"testing"

	"github.com/ivandosevv/stef/daemon/telemetry"
)

func TestActivityPreservesObservedToolStages(t *testing.T) {
	for _, phase := range []string{"started", "reading", "searching", "editing", "testing", "terminal", "tool_done", "completed", "failed"} {
		t.Run(phase, func(t *testing.T) {
			activity := telemetry.Activity{SessionID: "chat-observed", AgentID: "claude-1", Phase: phase, Tool: "Grep", NodeName: "src/main.go"}
			event, err := activity.Event()
			if err != nil {
				t.Fatal(err)
			}
			if event.Phase != phase || event.Tool != "Grep" || event.NodeName != "src/main.go" || event.SessionID != activity.SessionID || event.AgentID != activity.AgentID {
				t.Fatalf("observed activity lost its scope or stage: %#v", event)
			}
			wantStatus := "running"
			if phase == "tool_done" || phase == "completed" || phase == "failed" {
				wantStatus = "ready"
			}
			if event.Status != wantStatus {
				t.Fatalf("finished stage still looks active: %#v", event)
			}
			if event.Source != "hook" || event.Kind != "activity" || event.Category != "activity" || event.Action != "scouted" {
				t.Fatal("activity was misrepresented as an agent decision or test result")
			}
		})
	}
	if _, err := (telemetry.Activity{AgentID: "agent", Phase: "thinking"}).Event(); err == nil {
		t.Fatal("private thought stage accepted as observed activity")
	}
}
