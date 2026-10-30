package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/ivandosevv/stef/daemon/hub"
	bridge "github.com/ivandosevv/stef/daemon/mcp"
	"github.com/ivandosevv/stef/daemon/telemetry"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func workspaceTestSetup(t *testing.T) (*hub.Hub, *httptest.Server) {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	h, err := hub.New(root, "")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler(h, t.TempDir(), "4820"))
	t.Cleanup(func() { server.Close(); h.Close() })
	return h, server
}

func workspaceClient(t *testing.T, ctx context.Context, transport, endpoint, root, name string) *sdk.ClientSession {
	t.Helper()
	var connection sdk.Transport
	switch transport {
	case "http":
		connection = &sdk.StreamableClientTransport{Endpoint: endpoint + "/mcp"}
	case "stdio":
		executable, err := filepath.Abs("../bin/stef")
		if err != nil {
			t.Fatal(err)
		}
		connection = &sdk.CommandTransport{Command: exec.Command(executable, "--mcp", "--no-autostart", "--repo", root, "--daemon", endpoint)}
	default:
		server, err := bridge.NewServer(endpoint)
		if err != nil {
			t.Fatal(err)
		}
		remote, local := sdk.NewInMemoryTransports()
		ss, err := server.Connect(ctx, remote, nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { ss.Close() })
		connection = local
	}
	client := sdk.NewClient(&sdk.Implementation{Name: name, Version: "1"}, nil)
	session, err := client.Connect(ctx, connection, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { session.Close() })
	return session
}
func workspaceCall(t *testing.T, ctx context.Context, session *sdk.ClientSession, tool string, input, output any) {
	t.Helper()
	result, err := session.CallTool(ctx, &sdk.CallToolParams{Name: tool, Arguments: input})
	if err != nil || result.IsError {
		t.Fatalf("%s: %v %+v", tool, err, result)
	}
	raw, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, output); err != nil {
		t.Fatal(err)
	}
}

func TestPrivateWorkersAndExplicitSharingAcrossMCP(t *testing.T) {
	for _, transport := range []string{"forwarding", "http", "stdio"} {
		t.Run(transport, func(t *testing.T) {
			h, server := workspaceTestSetup(t)
			root, err := filepath.EvalSymlinks(h.Snapshot().Repo)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			codex := workspaceClient(t, ctx, transport, server.URL, root, "codex-test")
			claude := workspaceClient(t, ctx, transport, server.URL, root, "claude-test")
			var a, b, child bridge.State
			workspaceCall(t, ctx, codex, "get_state", bridge.StateInput{AgentID: "codex"}, &a)
			workspaceCall(t, ctx, claude, "get_state", bridge.StateInput{AgentID: "claude"}, &b)
			workspaceCall(t, ctx, codex, "get_state", bridge.StateInput{AgentID: "subagent"}, &child)
			if a.SessionID == b.SessionID || a.SessionID == child.SessionID || a.Shared || b.Shared || child.Shared {
				t.Fatal("workers did not default to independent private workspaces")
			}
			publish := func(client *sdk.ClientSession, origin, agent, node, why string) {
				t.Helper()
				var receipt bridge.Receipt
				workspaceCall(t, ctx, client, "report_event", bridge.Input{SessionID: origin, AgentID: agent, NodeName: node, Action: "modified", Summary: "Fix behavior", Why: why, Status: "ready"}, &receipt)
				if !receipt.Accepted {
					t.Fatal("step rejected")
				}
			}
			publish(codex, a.SessionID, "codex", "a.go", "Private Codex decision.")
			workspaceCall(t, ctx, claude, "get_state", bridge.StateInput{SessionID: b.SessionID, AgentID: "claude"}, &b)
			if len(b.Events) != 0 {
				t.Fatal("private context leaked to Claude")
			}
			var room bridge.WorkspaceReceipt
			workspaceCall(t, ctx, codex, "set_workspace", bridge.WorkspaceInput{SessionID: a.SessionID, AgentID: "codex", Action: "create"}, &room)
			shared := room.WorkspaceID
			workspaceCall(t, ctx, claude, "set_workspace", bridge.WorkspaceInput{SessionID: b.SessionID, AgentID: "claude", Action: "join", WorkspaceID: shared}, &room)
			workspaceCall(t, ctx, codex, "set_workspace", bridge.WorkspaceInput{SessionID: child.SessionID, AgentID: "subagent", Action: "join", WorkspaceID: shared}, &room)
			if len(room.Members) != 3 {
				t.Fatal("three-worker sharing failed")
			}
			publish(codex, a.SessionID, "codex", "same.go", "First exact shared reason.")
			publish(claude, b.SessionID, "claude", "same.go", "Claude shared decision.")
			publish(codex, a.SessionID, "codex", "same.go", "Second exact shared reason.")
			workspaceCall(t, ctx, claude, "get_state", bridge.StateInput{SessionID: b.SessionID, AgentID: "claude", Limit: 10}, &b)
			if b.SessionID == shared || b.WorkspaceID != shared || !b.Shared || len(b.Events) != 3 || b.Events[0].Why != "First exact shared reason." || b.Events[1].AgentID != "claude" || b.Events[2].Why != "Second exact shared reason." {
				t.Fatalf("shared context or authors lost: %+v", b)
			}
			if _, err := h.SetUpdateMode(telemetry.UpdatesDisabled, child.SessionID); err != nil {
				t.Fatal(err)
			}
			workspaceCall(t, ctx, claude, "get_state", bridge.StateInput{SessionID: b.SessionID, AgentID: "claude"}, &b)
			if b.UpdateMode != telemetry.UpdatesDisabled {
				t.Fatal("shared updates not scoped to room")
			}
			workspaceCall(t, ctx, codex, "set_workspace", bridge.WorkspaceInput{SessionID: a.SessionID, AgentID: "codex", Action: "leave"}, &room)
			workspaceCall(t, ctx, codex, "get_state", bridge.StateInput{SessionID: a.SessionID, AgentID: "codex"}, &a)
			if a.Shared || a.UpdateMode != telemetry.UpdatesFiles || len(a.Events) != 1 || a.Events[0].NodeName != "a.go" || a.Events[0].Why != "" {
				t.Fatalf("leaving did not restore compact private history: %+v", a)
			}
			if _, err := h.SetUpdateMode(telemetry.UpdatesFiles, b.SessionID); err != nil {
				t.Fatal(err)
			}
			if _, err := h.ClearSession(b.SessionID); err != nil {
				t.Fatal(err)
			}
			workspaceCall(t, ctx, codex, "get_state", bridge.StateInput{SessionID: child.SessionID, AgentID: "subagent"}, &child)
			if len(child.Events) != 0 {
				t.Fatal("shared Clear did not reach peer")
			}
			if len(h.SessionSnapshot(a.SessionID).Events) != 1 {
				t.Fatal("shared Clear erased private history")
			}
		})
	}
}

func TestStableChatWorkerResumeAndScopedExactChanges(t *testing.T) {
	h, server := setup(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	first := workspaceClient(t, ctx, "http", server.URL, h.Snapshot().Repo, "first")
	var receipt, other bridge.SessionReceipt
	workspaceCall(t, ctx, first, "set_session", bridge.SessionInput{SessionID: "stable-chat", AgentID: "codex"}, &receipt)
	workspaceCall(t, ctx, first, "set_session", bridge.SessionInput{SessionID: "stable-chat", AgentID: "child"}, &other)
	if receipt.SessionID == other.SessionID {
		t.Fatal("stable chat workers collided")
	}
	observed, err := h.Publish(telemetry.Event{AgentID: "filesystem", Source: "watcher", Kind: "file", NodeName: "a.go", Action: "modified", Summary: "File modified", Why: "Observed save.", Diff: "-old\n+new"})
	if err != nil {
		t.Fatal(err)
	}
	var state bridge.State
	workspaceCall(t, ctx, first, "get_state", bridge.StateInput{SessionID: receipt.SessionID, AgentID: "codex", ChangedPaths: []string{"a.go"}}, &state)
	if state.PendingCount != 1 || state.PendingChanges[0].ChangeID == observed.ID {
		t.Fatal("exact revision was not assigned to the worker")
	}
	change := state.PendingChanges[0].ChangeID
	workspaceCall(t, ctx, first, "get_state", bridge.StateInput{SessionID: other.SessionID, AgentID: "child", ChangedPaths: []string{"a.go"}}, &state)
	if state.PendingCount != 0 {
		t.Fatal("observation copied into another private workflow")
	}
	var explained bridge.ChangesReceipt
	workspaceCall(t, ctx, first, "report_changes", bridge.ChangesInput{SessionID: receipt.SessionID, AgentID: "codex", Changes: []telemetry.ChangeExplanation{{ChangeID: change, Summary: "Fix saved behavior", Why: "Preserve the exact revision reason."}}}, &explained)
	if explained.PendingCount != 0 || !explained.Accepted {
		t.Fatal("exact explanation failed")
	}
	second := workspaceClient(t, ctx, "http", server.URL, h.Snapshot().Repo, "reconnected")
	var resumed bridge.SessionReceipt
	workspaceCall(t, ctx, second, "set_session", bridge.SessionInput{SessionID: "stable-chat", AgentID: "codex"}, &resumed)
	if resumed.SessionID != receipt.SessionID {
		t.Fatal("transport reconnect changed durable chat identity")
	}
	snapshot := h.SessionSnapshot(resumed.SessionID)
	if len(snapshot.Events) != 2 || snapshot.Events[0].Diff != observed.Diff || snapshot.Events[1].ChangeID != change {
		t.Fatal("resume lost exact saved history")
	}
}
func TestSubagentToolCallbackRegistersBeforeExactCapture(t *testing.T) {
	h, server := workspaceTestSetup(t)
	if _, err := h.RegisterSession("parent"); err != nil {
		t.Fatal(err)
	}
	activity := telemetry.Activity{SessionID: "claude-child", AgentID: "child", Phase: "editing", NodeName: "child.ts", Tool: "Edit"}
	data, _ := json.Marshal(activity)
	response, err := http.Post(server.URL+"/api/activity", "application/json", bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 201 {
		t.Fatal("first subagent tool callback rejected")
	}
	if _, err := h.Publish(telemetry.Event{AgentID: "filesystem", Source: "watcher", Kind: "file", NodeName: "child.ts", Action: "modified", Summary: "File modified", Why: "Observed save."}); err != nil {
		t.Fatal(err)
	}
	data, _ = json.Marshal(map[string]any{"session_id": activity.SessionID, "paths": []string{"child.ts"}})
	response, err = http.Post(server.URL+"/api/explanation-reminder", "application/json", bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var reminder telemetry.ReminderState
	if err := json.NewDecoder(response.Body).Decode(&reminder); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != 200 || reminder.SessionID != activity.SessionID || reminder.PendingCount != 1 {
		t.Fatalf("subagent exact change not captured: %+v", reminder)
	}
	if len(h.SessionSnapshot("parent").Events) != 0 {
		t.Fatal("subagent callback or change leaked into parent workflow")
	}
}
