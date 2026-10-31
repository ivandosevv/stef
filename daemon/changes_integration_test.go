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
	"testing"
	"time"

	"github.com/ivandosevv/stef/daemon/hub"
	bridge "github.com/ivandosevv/stef/daemon/mcp"
	"github.com/ivandosevv/stef/daemon/telemetry"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestPerChangeCoverageOverMCP(t *testing.T) {
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
				executable, err := filepath.Abs(filepath.Join("..", "bin", testBinaryName()))
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
			client := sdk.NewClient(&sdk.Implementation{Name: "change-test", Version: "1"}, nil)
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
				raw, _ := json.Marshal(result.StructuredContent)
				if err := json.Unmarshal(raw, output); err != nil {
					t.Fatal(err)
				}
			}
			var selected bridge.SessionReceipt
			call("set_session", bridge.SessionInput{SessionID: "changes-chat", AgentID: "agent"}, &selected)
			observe := func(node string) telemetry.Event {
				t.Helper()
				event, err := h.Publish(telemetry.Event{AgentID: "filesystem", NodeName: node, Action: "modified", Summary: "File modified", Why: "A file save was observed.", Source: "watcher", Kind: "file"})
				if err != nil {
					t.Fatal(err)
				}
				var claimed bridge.State
				call("get_state", bridge.StateInput{SessionID: selected.SessionID, AgentID: "agent", ChangedPaths: []string{node}}, &claimed)
				capturedEvents := h.SessionSnapshot(selected.SessionID).Events
				for i := len(capturedEvents) - 1; i >= 0; i-- {
					captured := capturedEvents[i]
					if captured.Source == "watcher" && captured.NodeName == node && captured.ID != event.ID {
						return captured
					}
				}
				t.Fatal("saved path was not captured in this agent workspace")
				return telemetry.Event{}
			}
			first := observe("theme.ts")
			second := observe("main.ts")
			var milestone bridge.Receipt
			call("report_event", bridge.Input{SessionID: selected.SessionID, AgentID: "agent", NodeName: "main.ts", Action: "modified", Summary: "Apply appearance", Why: "Restore the saved appearance on startup.", Status: "ready"}, &milestone)
			if milestone.PendingCount != 2 || len(milestone.PendingChanges) != 2 {
				t.Fatalf("receipt hid pending files: %+v", milestone)
			}
			var state bridge.State
			call("get_state", bridge.StateInput{SessionID: selected.SessionID, AgentID: "agent", Limit: 1, SettleMS: 1}, &state)
			if len(state.Events) != 1 || state.PendingCount != 2 {
				t.Fatalf("state truncated pending revisions: %+v", state)
			}
			var receipt bridge.ChangesReceipt
			call("report_changes", bridge.ChangesInput{SessionID: selected.SessionID, AgentID: "agent", Changes: []telemetry.ChangeExplanation{
				{ChangeID: first.ID, Summary: "Save theme preference", Why: "Keep preference logic separate from rendering."},
				{ChangeID: second.ID, Summary: "Restore before mounting", Why: "Apply the saved choice before rendering.", Parents: []string{"theme.ts"}},
			}}, &receipt)
			if !receipt.Accepted || len(receipt.IDs) != 2 || receipt.PendingCount != 0 {
				t.Fatalf("batch receipt missing exact coverage: %+v", receipt)
			}
			last := h.SessionSnapshot(selected.SessionID).Events
			if last[len(last)-1].ChangeID != second.ID || last[len(last)-2].ChangeID != first.ID {
				t.Fatal("batch lost exact links")
			}
			third := observe("main.ts")
			call("get_state", bridge.StateInput{SessionID: selected.SessionID, AgentID: "agent", Limit: 1}, &state)
			if state.PendingCount != 1 || state.PendingChanges[0].ChangeID != third.ID {
				t.Fatal("later save inherited an old explanation")
			}
			fourth := observe("theme.ts")
			fifth := observe("main_test.ts")
			shared := bridge.ChangesInput{SessionID: selected.SessionID, AgentID: "agent", Summary: "Reuse saved output", Why: "Use the saved appearance across the startup flow.", Changes: []telemetry.ChangeExplanation{
				{ChangeID: third.ID},
				{ChangeID: fourth.ID, Summary: "Retain preference", Parents: []string{"main.ts"}},
				{ChangeID: fifth.ID, Why: "Verify the preference survives a fresh startup."},
			}}
			call("report_changes", shared, &receipt)
			if !receipt.Accepted || receipt.PendingCount != 0 || len(receipt.IDs) != 3 {
				t.Fatalf("reused output lost exact coverage: %+v", receipt)
			}
			last = h.SessionSnapshot(selected.SessionID).Events
			group := last[len(last)-3:]
			for i, id := range []string{third.ID, fourth.ID, fifth.ID} {
				if group[i].ChangeID != id || group[i].AgentID != "agent" {
					t.Fatalf("reused output changed revision attribution: %+v", group)
				}
			}
			if group[0].Summary != shared.Summary || group[0].Why != shared.Why || group[1].Summary != "Retain preference" || group[1].Why != shared.Why || len(group[1].Parents) != 1 || group[2].Summary != shared.Summary || group[2].Why != shared.Changes[2].Why {
				t.Fatalf("shared output or individual overrides changed: %+v", group)
			}
			raw, _ := json.Marshal(receipt)
			if bytes.Contains(raw, []byte(shared.Why)) {
				t.Fatal("receipt repeated the supplied explanation")
			}
		})
	}
}

func TestHTTPChangeBatchRejectsInvalidInputsBeforeEffects(t *testing.T) {
	h, server := setup(t)
	file, err := h.Publish(telemetry.Event{AgentID: "filesystem", NodeName: "theme.ts", Action: "created", Summary: "File created", Why: "A file save was observed.", Source: "watcher", Kind: "file"})
	if err != nil {
		t.Fatal(err)
	}
	input := bridge.ChangesInput{AgentID: "agent", Changes: []telemetry.ChangeExplanation{{ChangeID: file.ID, Summary: "Keep theme logic", Why: "Share theme preferences between startup and controls."}, {ChangeID: "999", Summary: "Other file", Why: "Invalid observation."}}}
	raw, _ := json.Marshal(input)
	response, err := http.Post(server.URL+"/api/changes", "application/json", bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 400 || len(h.Snapshot().Events) != 1 {
		t.Fatal("invalid HTTP batch had partial effects")
	}
	input.Changes = input.Changes[:1]
	raw, _ = json.Marshal(input)
	response, err = http.Post(server.URL+"/api/changes", "application/json", bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var receipt bridge.ChangesReceipt
	if err := json.NewDecoder(response.Body).Decode(&receipt); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != 201 || receipt.PendingCount != 0 {
		t.Fatalf("valid HTTP batch failed: %d %+v", response.StatusCode, receipt)
	}
}
