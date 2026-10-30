package mcp_test

import (
	"context"
	"testing"
	"time"

	"github.com/ivandosevv/stef/daemon/hub"
	bridge "github.com/ivandosevv/stef/daemon/mcp"
	"github.com/ivandosevv/stef/daemon/telemetry"
)

func TestCheckInIncludesPendingOutsideRecentLimitAndCancelSafeSettle(t *testing.T) {
	h, err := hub.New(t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	file, err := h.Publish(telemetry.Event{AgentID: "filesystem", NodeName: "theme.ts", Action: "modified", Summary: "File modified", Why: "Saved file changed.", Source: "watcher", Kind: "file"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.Publish(telemetry.Event{AgentID: "agent", NodeName: "main.ts", Action: "tested", Summary: "Tests pass", Why: "The greeting cases passed."}); err != nil {
		t.Fatal(err)
	}
	state, err := bridge.CheckInRequest(context.Background(), h, bridge.StateInput{Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Events) != 1 || state.PendingCount != 1 || len(state.PendingChanges) != 1 || state.PendingChanges[0].ChangeID != file.ID {
		t.Fatalf("recent-event truncation hid unreported change: %+v", state)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	started := time.Now()
	if _, err := bridge.CheckInRequest(ctx, h, bridge.StateInput{SettleMS: 1000}); err == nil {
		t.Fatal("canceled settle accepted")
	}
	if time.Since(started) > 100*time.Millisecond {
		t.Fatal("cancel did not interrupt settle")
	}
	for _, delay := range []int{-1, 1001} {
		if _, err := bridge.CheckInRequest(context.Background(), h, bridge.StateInput{SettleMS: delay}); err == nil {
			t.Fatal("unbounded settle accepted")
		}
	}
}
