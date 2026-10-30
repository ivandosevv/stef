package main

import (
	"context"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/ivandosevv/stef/daemon/hub"
	bridge "github.com/ivandosevv/stef/daemon/mcp"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// Assert the public wire contract so local and forwarded clients receive the same
// readable labels and scoped opening path, regardless of internal routing IDs.
type workspaceStartReceipt struct {
	SessionID     string `json:"session_id"`
	WorkspaceID   string `json:"workspace_id"`
	WorkspaceName string `json:"workspace_name"`
	Shared        bool   `json:"shared"`
	ViewPath      string `json:"view_path"`
}
type workspaceStartState struct {
	workspaceStartReceipt
	Events []bridge.StateEvent `json:"events"`
}

func workspaceStartClient(t *testing.T, ctx context.Context, transport string, h *hub.Hub, endpoint, name string) *sdk.ClientSession {
	t.Helper()
	if transport != "local" {
		return workspaceClient(t, ctx, transport, endpoint, h.Snapshot().Repo, name)
	}
	server := bridge.NewLocalServer(h)
	remote, local := sdk.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, remote, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { serverSession.Close() })
	client := sdk.NewClient(&sdk.Implementation{Name: name, Version: "1"}, nil)
	session, err := client.Connect(ctx, local, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { session.Close() })
	return session
}

func assertWorkspaceOpening(t *testing.T, receipt workspaceStartReceipt) {
	t.Helper()
	if !regexp.MustCompile("^[A-Za-z]+ [A-Za-z]+ [0-9]{4,}$").MatchString(receipt.WorkspaceName) || receipt.WorkspaceName == receipt.SessionID || receipt.WorkspaceName == receipt.WorkspaceID {
		t.Fatalf("workspace did not expose a readable local name: %+v", receipt)
	}
	path, err := url.Parse(receipt.ViewPath)
	if err != nil || path.IsAbs() || path.Host != "" || path.Path != "/" || path.Query().Get("companion") != "1" || path.Query().Get("session_id") != receipt.SessionID || path.Fragment != "" {
		t.Fatalf("opening path is not scoped to this origin: %+v (%v)", receipt, err)
	}
}

func TestExplicitStartIsPrivateAndSharingResumeRequiresOptIn(t *testing.T) {
	for _, transport := range []string{"local", "forwarding", "http"} {
		t.Run(transport, func(t *testing.T) {
			h, server := workspaceTestSetup(t)
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			first := workspaceStartClient(t, ctx, transport, h, server.URL, "first-worker")
			peer := workspaceStartClient(t, ctx, transport, h, server.URL, "second-worker")
			start := func(client *sdk.ClientSession, chat, worker string, resumeShared bool) workspaceStartReceipt {
				t.Helper()
				input := map[string]any{"session_id": chat, "agent_id": worker}
				if resumeShared {
					input["resume_shared"] = true
				}
				var receipt workspaceStartReceipt
				workspaceCall(t, ctx, client, "set_session", input, &receipt)
				assertWorkspaceOpening(t, receipt)
				return receipt
			}
			state := func(client *sdk.ClientSession, origin, worker string) workspaceStartState {
				t.Helper()
				var state workspaceStartState
				workspaceCall(t, ctx, client, "get_state", bridge.StateInput{SessionID: origin, AgentID: worker}, &state)
				assertWorkspaceOpening(t, state.workspaceStartReceipt)
				return state
			}
			publish := func(client *sdk.ClientSession, origin, worker, node string) {
				t.Helper()
				var receipt bridge.Receipt
				workspaceCall(t, ctx, client, "report_event", bridge.Input{SessionID: origin, AgentID: worker, NodeName: node, Action: "modified", Summary: "Keep scoped behavior", Why: "Keep this recorded decision in its chosen workspace.", Status: "ready"}, &receipt)
				if !receipt.Accepted {
					t.Fatal("scoped report was rejected")
				}
			}

			// Hosts may provide stable origins containing URL-reserved characters.
			rawOrigin := "host chat & branch?one/two"
			hostBound := start(first, rawOrigin, "", false)
			if hostBound.SessionID != rawOrigin || hostBound.Shared {
				t.Fatal("trusted host identity changed during scoped opening")
			}
			a := start(first, "first-stable-chat", "codex", false)
			b := start(peer, "second-stable-chat", "claude", false)
			if a.Shared || b.Shared || a.WorkspaceID != a.SessionID || b.WorkspaceID != b.SessionID || a.SessionID == b.SessionID || a.WorkspaceName == b.WorkspaceName {
				t.Fatalf("workers did not start in distinct named private workspaces: %+v %+v", a, b)
			}
			publish(first, a.SessionID, "codex", "private-a.go")
			publish(peer, b.SessionID, "claude", "private-b.go")
			var room bridge.WorkspaceReceipt
			workspaceCall(t, ctx, first, "set_workspace", bridge.WorkspaceInput{SessionID: a.SessionID, AgentID: "codex", Action: "create"}, &room)
			if !room.Shared || room.Name == "" || room.Name == a.WorkspaceName || room.Name == b.WorkspaceName {
				t.Fatalf("shared room lost a distinct friendly name: %+v", room)
			}
			roomID, roomName := room.WorkspaceID, room.Name
			workspaceCall(t, ctx, peer, "set_workspace", bridge.WorkspaceInput{SessionID: b.SessionID, AgentID: "claude", Action: "join", WorkspaceID: "  " + strings.ToLower(roomName) + "  "}, &room)
			if room.WorkspaceID != roomID || room.Name != roomName || len(room.Members) != 2 {
				t.Fatalf("friendly-name join changed room identity: %+v", room)
			}
			publish(first, a.SessionID, "codex", "shared.go")
			sharedState := state(peer, b.SessionID, "claude")
			if !sharedState.Shared || sharedState.WorkspaceName != roomName || len(sharedState.Events) != 1 || sharedState.Events[0].NodeName != "shared.go" {
				t.Fatalf("shared check-in lost named context: %+v", sharedState)
			}

			// A transport reconnect with an explicit retained origin is a check-in,
			// not a new startup request. It must keep the active shared membership.
			reconnected := workspaceStartClient(t, ctx, transport, h, server.URL, "reconnected-worker")
			reconnectedState := state(reconnected, a.SessionID, "codex")
			if !reconnectedState.Shared || reconnectedState.WorkspaceID != roomID || reconnectedState.ViewPath != a.ViewPath {
				t.Fatalf("transport reconnect reset sharing or changed the scoped view: %+v", reconnectedState)
			}
			retained := start(reconnected, "first-stable-chat", "codex", true)
			if !retained.Shared || retained.SessionID != a.SessionID || retained.WorkspaceID != roomID || retained.WorkspaceName != roomName {
				t.Fatalf("explicit sharing resume did not retain membership: %+v", retained)
			}

			private := start(reconnected, "first-stable-chat", "codex", false)
			if private.Shared || private.SessionID != a.SessionID || private.WorkspaceID != a.WorkspaceID || private.WorkspaceName != a.WorkspaceName || private.ViewPath != a.ViewPath {
				t.Fatalf("default startup did not restore the same named private workspace: %+v", private)
			}
			privateState := state(reconnected, a.SessionID, "codex")
			if len(privateState.Events) != 1 || privateState.Events[0].NodeName != "private-a.go" {
				t.Fatalf("default private resume lost history or copied shared history: %+v", privateState)
			}
			peerState := state(peer, b.SessionID, "claude")
			if !peerState.Shared || peerState.WorkspaceID != roomID || peerState.WorkspaceName != roomName || len(peerState.Events) != 1 {
				t.Fatalf("one worker's private startup changed its peer: %+v", peerState)
			}

			// A private label can be viewed, but cannot be used as a sharing invite.
			invalid, err := peer.CallTool(ctx, &sdk.CallToolParams{Name: "set_workspace", Arguments: bridge.WorkspaceInput{SessionID: b.SessionID, AgentID: "claude", Action: "join", WorkspaceID: a.WorkspaceName}})
			if err != nil || !invalid.IsError {
				t.Fatalf("private workspace name was accepted as a shared invite: %+v %v", invalid, err)
			}
			if unchanged := state(peer, b.SessionID, "claude"); !unchanged.Shared || unchanged.WorkspaceID != roomID {
				t.Fatal("rejected name join altered membership")
			}
			if _, err := h.ClearSession(a.SessionID); err != nil {
				t.Fatal(err)
			}
			afterClear := start(reconnected, "first-stable-chat", "codex", false)
			if afterClear.WorkspaceName != a.WorkspaceName || afterClear.SessionID != a.SessionID || afterClear.Shared {
				t.Fatal("Clear changed the private name or startup policy")
			}
			if cleared := state(reconnected, a.SessionID, "codex"); len(cleared.Events) != 0 {
				t.Fatal("Clear did not reset the selected private workflow")
			}
			if peerState = state(peer, b.SessionID, "claude"); len(peerState.Events) != 1 || peerState.WorkspaceName != roomName {
				t.Fatal("private Clear erased or renamed its peer's shared workflow")
			}
		})
	}
}
