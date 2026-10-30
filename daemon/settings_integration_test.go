package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/ivandosevv/stef/daemon/hub"
	bridge "github.com/ivandosevv/stef/daemon/mcp"
	"github.com/ivandosevv/stef/daemon/telemetry"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestSettingsAPIValidatesAndUpdatesEveryLiveView(t *testing.T) {
	h, server := setup(t)
	post := func(body, media string) *http.Response {
		t.Helper()
		response, err := http.Post(server.URL+"/api/settings", media, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { response.Body.Close() })
		return response
	}
	response, err := http.Get(server.URL + "/api/settings")
	if err != nil {
		t.Fatal(err)
	}
	var settings telemetry.Settings
	if err := json.NewDecoder(response.Body).Decode(&settings); err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if settings.UpdateMode != telemetry.UpdatesFiles {
		t.Fatalf("default preference: %+v", settings)
	}
	for _, tc := range []struct {
		body, media string
		status      int
	}{
		{"{}", "application/json", 400},
		{"null", "application/json", 400},
		{"{\"update_mode\":\"off\"}", "application/json", 400},
		{"{\"update_mode\":\"files\",\"session_id\":\"chat-a\"}", "application/json", 400},
		{"{\"update_mode\":\"files\"}{}", "application/json", 400},
		{"{\"update_mode\":\"files\"}", "text/plain", 415},
	} {
		if response := post(tc.body, tc.media); response.StatusCode != tc.status {
			t.Fatalf("invalid settings %s returned %d", tc.body, response.StatusCode)
		}
	}
	var views []*websocket.Conn
	for range 2 {
		view, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/ws", nil)
		if err != nil {
			t.Fatal(err)
		}
		defer view.Close()
		_ = view.SetReadDeadline(time.Now().Add(5 * time.Second))
		var initial hub.Message
		if err := view.ReadJSON(&initial); err != nil || initial.UpdateMode != telemetry.UpdatesFiles {
			t.Fatalf("initial mode missing: %+v %v", initial, err)
		}
		views = append(views, view)
	}
	response = post("{\"update_mode\":\"milestones\"}", "application/json")
	if response.StatusCode != http.StatusOK {
		t.Fatal("mode update failed")
	}
	if err := json.NewDecoder(response.Body).Decode(&settings); err != nil || settings.UpdateMode != telemetry.UpdatesMilestones {
		t.Fatalf("mode acknowledgement: %+v %v", settings, err)
	}
	if snapshot := h.Snapshot(); snapshot.UpdatesFrom != "1" {
		t.Fatalf("HTTP mode change lost new reporting boundary: %+v", snapshot)
	}
	for _, view := range views {
		var updated hub.Message
		if err := view.ReadJSON(&updated); err != nil || updated.Type != "snapshot" || updated.UpdateMode != telemetry.UpdatesMilestones || updated.UpdatesFrom != "1" {
			t.Fatalf("live view missed shared mode: %+v %v", updated, err)
		}
	}
	if h.Settings().UpdateMode != telemetry.UpdatesMilestones {
		t.Fatal("API only updated its response")
	}
}

func TestReportingModesOverMCPTransports(t *testing.T) {
	for _, transport := range []string{"forwarding", "streamable-http", "stdio"} {
		t.Run(transport, func(t *testing.T) {
			root, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			h, err := hub.New(root, "")
			if err != nil {
				t.Fatal(err)
			}
			defer h.Close()
			server := httptest.NewServer(handler(h, t.TempDir(), "4820"))
			defer server.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			var connection sdk.Transport
			switch transport {
			case "streamable-http":
				connection = &sdk.StreamableClientTransport{Endpoint: server.URL + "/mcp"}
			case "stdio":
				executable, err := filepath.Abs("../bin/stef")
				if err != nil {
					t.Fatal(err)
				}
				if _, err := os.Stat(executable); err != nil {
					t.Skip("make test builds the executable first")
				}
				command := exec.Command(executable, "--mcp", "--no-autostart", "--repo", root, "--daemon", server.URL)
				command.Stderr = os.Stderr
				connection = &sdk.CommandTransport{Command: command}
			default:
				mcpServer, err := bridge.NewServer(server.URL)
				if err != nil {
					t.Fatal(err)
				}
				peer, local := sdk.NewInMemoryTransports()
				serverSession, err := mcpServer.Connect(ctx, peer, nil)
				if err != nil {
					t.Fatal(err)
				}
				defer serverSession.Close()
				connection = local
			}
			client := sdk.NewClient(&sdk.Implementation{Name: "mode-test", Version: "1"}, nil)
			session, err := client.Connect(ctx, connection, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer session.Close()
			call := func(tool string, input, output any) {
				t.Helper()
				result, err := session.CallTool(ctx, &sdk.CallToolParams{Name: tool, Arguments: input})
				if err != nil || result.IsError {
					t.Fatalf("%s failed: %+v %v", tool, result, err)
				}
				data, _ := json.Marshal(result.StructuredContent)
				if err := json.Unmarshal(data, output); err != nil {
					t.Fatal(err)
				}
			}
			mode := func(mode telemetry.UpdateMode) {
				t.Helper()
				body, _ := json.Marshal(telemetry.Settings{UpdateMode: mode})
				response, err := http.Post(server.URL+"/api/settings", "application/json", bytes.NewReader(body))
				if err != nil {
					t.Fatal(err)
				}
				response.Body.Close()
				if response.StatusCode != http.StatusOK {
					t.Fatal("cannot set reporting preference")
				}
			}
			var selected bridge.SessionReceipt
			call("set_session", bridge.SessionInput{SessionID: "mode-chat"}, &selected)
			if selected.UpdateMode != telemetry.UpdatesFiles || selected.ReportingInstructions == "" || selected.UpdatesFrom != "0" {
				t.Fatalf("set_session did not return the current policy: %+v", selected)
			}
			observe := func(path string) telemetry.Event {
				t.Helper()
				event, err := h.Publish(telemetry.Event{AgentID: "filesystem", NodeName: path, Action: "modified", Summary: "File modified", Why: "A saved file changed.", Source: "watcher", Kind: "file"})
				if err != nil {
					t.Fatal(err)
				}
				return event
			}
			first := observe("theme.ts")
			var explained bridge.ChangesReceipt
			call("report_changes", bridge.ChangesInput{AgentID: "agent", Changes: []telemetry.ChangeExplanation{{ChangeID: first.ID, Summary: "Share appearance preference", Why: "Use the same saved choice across the interface."}}}, &explained)
			if !explained.Accepted || explained.UpdateMode != telemetry.UpdatesFiles || explained.ReportingInstructions == "" {
				t.Fatalf("file receipt lost policy: %+v", explained)
			}
			oldPending := observe("old-unexplained.ts")
			note, _ := (telemetry.Feedback{Kind: "question", Text: "Check retries?", TargetAgent: "agent"}).Event()
			if _, err := h.Publish(note); err != nil {
				t.Fatal(err)
			}
			mode(telemetry.UpdatesDisabled)
			var state bridge.State
			call("get_state", bridge.StateInput{AgentID: "agent"}, &state)
			if state.UpdateMode != telemetry.UpdatesDisabled || state.ReportingInstructions == "" || state.UpdatesFrom != h.Snapshot().UpdatesFrom || len(state.Events) != 0 || len(state.Feedback) != 0 || state.PendingCount != 0 || len(state.PendingChanges) != 0 {
				t.Fatalf("disabled check-in still asked for updates: %+v", state)
			}
			input := bridge.Input{AgentID: "agent", NodeName: "main.ts", Action: "modified", Summary: "Deliver the feature", Why: "Make the reporting preference apply across clients.", Status: "ready"}
			var receipt bridge.Receipt
			call("report_event", input, &receipt)
			if receipt.Accepted || receipt.ID != "" || receipt.UpdateMode != telemetry.UpdatesDisabled || receipt.ReportingInstructions == "" {
				t.Fatalf("disabled event was accepted: %+v", receipt)
			}
			call("report_changes", bridge.ChangesInput{AgentID: "agent", Changes: []telemetry.ChangeExplanation{{ChangeID: "99999", Summary: "Late file explanation", Why: "An old mode still had a pending report."}}}, &explained)
			if explained.Accepted || len(explained.IDs) != 0 || explained.UpdateMode != telemetry.UpdatesDisabled {
				t.Fatalf("disabled late batch was not ignored: %+v", explained)
			}
			if e := observe("disabled.ts"); e.ID != "" {
				t.Fatal("disabled watcher event persisted")
			}
			mode(telemetry.UpdatesMilestones)
			call("get_state", bridge.StateInput{AgentID: "agent"}, &state)
			if state.UpdateMode != telemetry.UpdatesMilestones || len(state.Events) != 0 || len(state.Feedback) != 1 || state.PendingCount != 0 {
				t.Fatalf("major mode included old file activity or lost feedback: %+v", state)
			}
			call("report_event", input, &receipt)
			if !receipt.Accepted || receipt.UpdateMode != telemetry.UpdatesMilestones || receipt.PendingCount != 0 {
				t.Fatalf("major implementation milestone rejected: %+v", receipt)
			}
			contextInput := input
			contextInput.NodeName, contextInput.Action, contextInput.ContextID = first.NodeName, first.Action, first.ID
			contextInput.Why = "The developer requested why this retained theme revision was made."
			var contextReceipt bridge.Receipt
			call("report_event", contextInput, &contextReceipt)
			if !contextReceipt.Accepted || contextReceipt.UpdatesFrom != h.Snapshot().UpdatesFrom || contextReceipt.PendingCount != 0 {
				t.Fatalf("requested retained context not forwarded: %+v", contextReceipt)
			}
			lastContext := h.Snapshot().Events
			if lastContext[len(lastContext)-1].ContextID != first.ID {
				t.Fatal("transport lost exact requested context ID")
			}
			contextInput.ContextID = contextReceipt.ID
			invalid, err := session.CallTool(ctx, &sdk.CallToolParams{Name: "report_event", Arguments: contextInput})
			if err != nil || !invalid.IsError {
				t.Fatalf("context reply incorrectly accepted as another target: %+v %v", invalid, err)
			}
			call("get_state", bridge.StateInput{AgentID: "agent"}, &state)
			if len(state.Events) != 1 {
				t.Fatalf("manual context became automatic milestone activity: %+v", state)
			}
			input.ChangeID = first.ID
			input.NodeName = "theme.ts"
			call("report_event", input, &receipt)
			if receipt.Accepted || receipt.UpdateMode != telemetry.UpdatesMilestones {
				t.Fatal("major mode accepted an exact per-file explanation")
			}
			call("report_changes", bridge.ChangesInput{AgentID: "agent", Changes: []telemetry.ChangeExplanation{{ChangeID: "99999", Summary: "Late file explanation", Why: "An old mode still had a pending report."}}}, &explained)
			if explained.Accepted || len(explained.IDs) != 0 || explained.UpdateMode != telemetry.UpdatesMilestones {
				t.Fatal("major mode did not suppress per-file batches")
			}
			if e := observe("milestone.ts"); e.ID != "" {
				t.Fatal("major mode watcher event persisted")
			}
			mode(telemetry.UpdatesFiles)
			last := observe("main.ts")
			call("get_state", bridge.StateInput{AgentID: "agent"}, &state)
			if state.UpdateMode != telemetry.UpdatesFiles || state.UpdatesFrom != last.ID || len(state.Events) != 1 || state.PendingCount != 1 || len(state.PendingChanges) != 1 || state.PendingChanges[0].ChangeID != last.ID {
				t.Fatalf("re-enabling replayed suppressed changes or lost fresh coverage: %+v", state)
			}
			staleBatch, err := session.CallTool(ctx, &sdk.CallToolParams{Name: "report_changes", Arguments: bridge.ChangesInput{AgentID: "agent", Changes: []telemetry.ChangeExplanation{{ChangeID: oldPending.ID, Summary: "Late old-window explanation", Why: "This file predates the selected reporting window."}}}})
			if err != nil || !staleBatch.IsError {
				t.Fatalf("transport accepted automatic old-window backfill: %+v %v", staleBatch, err)
			}
			mode(telemetry.UpdatesDisabled)
			contextInput.ContextID = first.ID
			call("report_event", contextInput, &contextReceipt)
			if contextReceipt.Accepted || contextReceipt.UpdatesFrom != h.Snapshot().UpdatesFrom {
				t.Fatalf("disabled transport accepted requested context: %+v", contextReceipt)
			}
		})
	}
}
