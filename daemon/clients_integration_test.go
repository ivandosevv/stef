package main

import (
	"context"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/ivandosevv/stef/daemon/hub"
	clientsetup "github.com/ivandosevv/stef/daemon/setup"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// These are real SDK transports consuming generated configs, not real host sessions.
func TestGeneratedClientCommandsConnectThroughTheSameMCP(t *testing.T) {
	executable, err := filepath.Abs(filepath.Join("..", "bin", testBinaryName()))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(executable); err != nil {
		t.Skip("make test builds the executable")
	}
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	h, err := hub.New(root, "")
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	server := httptest.NewServer(handler(h, "", "0"))
	defer server.Close()
	for _, definition := range clientsetup.Clients() {
		t.Run(definition.Name, func(t *testing.T) {
			configured, err := clientsetup.Configure(root, executable, server.URL, definition.Name)
			if err != nil {
				t.Fatal(err)
			}
			raw, err := os.ReadFile(filepath.Join(root, configured.ConfigPath))
			if err != nil {
				t.Fatal(err)
			}
			document, err := clientsetup.DecodeClientConfig(raw, definition.Name)
			if err != nil {
				t.Fatal(err)
			}
			servers := document[definition.ServerKey].(map[string]any)
			target := servers["stef"].(map[string]any)
			configuredCommand, args, err := clientsetup.StdioCommand(target, definition.Name)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			command := exec.Command(configuredCommand, args...)
			command.Dir = root
			client := sdk.NewClient(&sdk.Implementation{Name: "configured-" + definition.Name, Version: "test"}, nil)
			session, err := client.Connect(ctx, &sdk.CommandTransport{Command: command}, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer session.Close()
			chat := "configuration-" + definition.Name
			result, err := session.CallTool(ctx, &sdk.CallToolParams{Name: "set_session", Arguments: map[string]any{"session_id": chat}})
			if err != nil || result.IsError {
				t.Fatalf("chat selection failed: %+v %v", result, err)
			}
			result, err = session.CallTool(ctx, &sdk.CallToolParams{Name: "report_event", Arguments: map[string]any{
				"agent_id": "configured-" + definition.Name, "session_id": chat, "node_name": "entry.go", "action": "tested",
				"summary": "Verify client transport", "why": "This generated configuration reaches the shared project MCP.", "status": "ready",
			}})
			if err != nil || result.IsError {
				t.Fatalf("report failed: %+v %v", result, err)
			}
			snapshot := h.SessionSnapshot(chat)
			if len(snapshot.Events) != 1 || snapshot.Events[0].AgentID != "configured-"+definition.Name {
				t.Fatalf("client report missing from its chat: %+v", snapshot.Events)
			}
		})
	}
}

func TestClientListingDoesNotNeedAProjectOrLaunchACompanion(t *testing.T) {
	executable, err := filepath.Abs(filepath.Join("..", "bin", testBinaryName()))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(executable); err != nil {
		t.Skip("make test builds the executable")
	}
	data, err := exec.Command(executable, "--list-clients", "--repo", filepath.Join(t.TempDir(), "does-not-exist")).Output()
	if err != nil || len(data) == 0 {
		t.Fatalf("client listing required a project: %s %v", data, err)
	}
}
