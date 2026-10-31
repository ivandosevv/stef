package mcp_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/ivandosevv/stef/daemon/hub"
	bridge "github.com/ivandosevv/stef/daemon/mcp"
	"github.com/ivandosevv/stef/daemon/telemetry"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestActivityMCPReportsChoicesAndExactFilesWithoutHistoricalBackfill(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	h, err := hub.New(t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	remote, local := sdk.NewInMemoryTransports()
	serverSession, err := bridge.NewLocalServer(h).Connect(ctx, remote, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer serverSession.Close()
	client := sdk.NewClient(&sdk.Implementation{Name: "activity-test", Version: "1"}, nil)
	session, err := client.Connect(ctx, local, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	call := func(name string, input, output any) {
		t.Helper()
		result, err := session.CallTool(ctx, &sdk.CallToolParams{Name: name, Arguments: input})
		if err != nil || result.IsError {
			t.Fatalf("%s failed: %+v %v", name, result, err)
		}
		raw, err := json.Marshal(result.StructuredContent)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(raw, output); err != nil {
			t.Fatal(err)
		}
	}
	var selected bridge.SessionReceipt
	call("set_session", bridge.SessionInput{SessionID: "activity-chat", AgentID: "worker"}, &selected)
	choice := bridge.Input{SessionID: selected.SessionID, AgentID: "worker", NodeName: "@workflow", Action: "decided", Summary: "Keep source private", Why: "Show shared decisions without copying captured source into model receipts.", Status: "ready"}
	var receipt bridge.Receipt
	call("report_event", choice, &receipt)
	if receipt.Accepted || receipt.ID != "" || receipt.UpdateMode != telemetry.UpdatesFiles {
		t.Fatalf("file mode spent a choice report: %+v", receipt)
	}
	observe := func(path string) telemetry.Event {
		t.Helper()
		event, err := h.Publish(telemetry.Event{SessionID: selected.SessionID, AgentID: "filesystem", NodeName: path, Action: "modified", Summary: "File modified", Why: "Observed a saved revision.", Source: "watcher", Kind: "file", Before: "captured-before", After: "captured-after", Diff: "captured-diff"})
		if err != nil {
			t.Fatal(err)
		}
		return event
	}
	old := observe("old.ts")
	if _, err := h.SetSessionUpdateMode(selected.SessionID, telemetry.UpdatesActivity); err != nil {
		t.Fatal(err)
	}
	stale := bridge.Input{SessionID: selected.SessionID, AgentID: "worker", NodeName: old.NodeName, Action: old.Action, ChangeID: old.ID, Summary: "Explain old save", Why: "This automatic report targets an earlier recording window.", Status: "ready"}
	before := len(h.SessionSnapshot(selected.SessionID).Events)
	rejected, err := session.CallTool(ctx, &sdk.CallToolParams{Name: "report_event", Arguments: stale})
	if err != nil || !rejected.IsError || len(h.SessionSnapshot(selected.SessionID).Events) != before {
		t.Fatalf("MCP accepted old-window single reason or changed history: %+v %v", rejected, err)
	}
	stale.ChangeID, stale.ContextID = "", old.ID
	stale.Why = "The developer explicitly requested context about this retained file revision."
	call("report_event", stale, &receipt)
	if !receipt.Accepted {
		t.Fatal("MCP blocked explicit historical file context")
	}
	call("report_event", choice, &receipt)
	if !receipt.Accepted || receipt.ID == "" || receipt.UpdateMode != telemetry.UpdatesActivity || !strings.Contains(receipt.ReportingInstructions, "decided") {
		t.Fatalf("activity choice not accepted with compact guidance: %+v", receipt)
	}
	decisionID := receipt.ID
	fresh := observe("main.ts")
	var state bridge.State
	call("get_state", bridge.StateInput{SessionID: selected.SessionID, AgentID: "worker", Limit: 1}, &state)
	if state.UpdateMode != telemetry.UpdatesActivity || len(state.Events) != 1 || state.PendingCount != 1 || len(state.PendingChanges) != 1 || state.PendingChanges[0].ChangeID != fresh.ID || state.UpdatesFrom != "2" {
		t.Fatalf("activity check-in lost exact pending or backfilled: %+v (old %s)", state, old.ID)
	}
	raw, _ := json.Marshal(state)
	for _, code := range []string{"captured-before", "captured-after", "captured-diff"} {
		if strings.Contains(string(raw), code) {
			t.Fatalf("compact activity check-in copied source: %s", raw)
		}
	}
	var batch bridge.ChangesReceipt
	call("report_changes", bridge.ChangesInput{SessionID: selected.SessionID, AgentID: "worker", Changes: []telemetry.ChangeExplanation{{ChangeID: fresh.ID, Summary: "Keep revisions exact", Why: "Attach the explanation to the captured file revision."}}}, &batch)
	if !batch.Accepted || batch.PendingCount != 0 || len(batch.IDs) != 1 || batch.UpdateMode != telemetry.UpdatesActivity {
		t.Fatalf("activity file explanation failed: %+v", batch)
	}
	for _, mode := range []telemetry.UpdateMode{telemetry.UpdatesMilestones, telemetry.UpdatesDisabled, telemetry.UpdatesFiles} {
		if _, err := h.SetSessionUpdateMode(selected.SessionID, mode); err != nil {
			t.Fatal(err)
		}
		call("report_event", choice, &receipt)
		if receipt.Accepted || receipt.ID != "" || receipt.UpdateMode != mode || receipt.PendingCount != 0 {
			t.Fatalf("%s did not stop future choice reports: %+v", mode, receipt)
		}
	}
	choice.ContextID = decisionID
	call("report_event", choice, &receipt)
	if !receipt.Accepted || receipt.ID == "" {
		t.Fatal("explicit retained activity question was blocked in file mode")
	}
}

func TestSharedActivityCheckInIncludesBriefReasonsAndSupportedPhasesOnly(t *testing.T) {
	h, err := hub.New(t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	room, err := h.ConfigureWorkspace(hub.WorkspaceInput{SessionID: "owner", Action: "create"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.ConfigureWorkspace(hub.WorkspaceInput{SessionID: "peer", Action: "join", WorkspaceID: room.WorkspaceID}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.SetSessionUpdateMode("owner", telemetry.UpdatesActivity); err != nil {
		t.Fatal(err)
	}
	for _, phase := range []string{"started", "reading", "searching", "terminal", "tool_done", "completed"} {
		event, _ := (telemetry.Activity{SessionID: "owner", AgentID: "host", Phase: phase, Tool: "Bash"}).Event()
		if _, err := h.Publish(event); err != nil {
			t.Fatal(err)
		}
	}
	why := "Keep the checks local while showing the selected workflow."
	if _, err := h.Publish(telemetry.Event{SessionID: "owner", AgentID: "owner-agent", NodeName: "@workflow", Action: "decided", Summary: "Run local checks", Why: why, Status: "ready"}); err != nil {
		t.Fatal(err)
	}
	state := bridge.CheckIn(h, 50, "peer-agent", "peer")
	if !state.Shared || state.WorkspaceID != room.WorkspaceID || len(state.Events) != 7 || state.Events[6].Why != why {
		t.Fatalf("shared supported activity or public reason missing: %+v", state)
	}
	for _, event := range state.Events[:6] {
		if event.Why != "" {
			t.Fatalf("observed hook was presented as a peer reason: %+v", event)
		}
	}
}
