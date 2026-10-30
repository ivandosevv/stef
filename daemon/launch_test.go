package main

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestCompanionReuseRequiresMatchingRootAndVersion(t *testing.T) {
	root := t.TempDir()
	for _, tc := range []struct {
		name, version, repo string
		wantError           bool
	}{
		{"matching", "0.3.0", root, false}, {"different root", "0.3.0", "/other", true}, {"old engine", "0.2.0", root, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]string{"name": "stef", "status": "ok", "version": tc.version, "repo": tc.repo})
			}))
			defer server.Close()
			stop, err := ensureCompanion(context.Background(), server.URL, root, true, true, "", true)
			if (err != nil) != tc.wantError {
				t.Fatalf("unexpected result: %v", err)
			}
			if stop != nil {
				stop()
			}
		})
	}
}

func TestStdioAutoStartsAndStopsOwnedEngine(t *testing.T) {
	executable, err := filepath.Abs("../bin/stef")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(executable); err != nil {
		t.Skip("make test builds the packaged executable first")
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	endpoint := "http://127.0.0.1:" + strconv.Itoa(port)
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	command := exec.Command(executable, "--mcp", "--headless", "--repo", root, "--daemon", endpoint, "--no-persist")
	command.Stderr = os.Stderr
	client := sdk.NewClient(&sdk.Implementation{Name: "stdio-smoke", Version: "1"}, nil)
	session, err := client.Connect(ctx, &sdk.CommandTransport{Command: command}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	if ready, err := probeCompanion(ctx, endpoint, root); err != nil || !ready {
		t.Fatalf("child not ready: %v", err)
	}
	result, err := session.CallTool(ctx, &sdk.CallToolParams{Name: "report_event", Arguments: map[string]any{
		"agent_id": "stdio-smoke", "node_name": "main.go", "action": "tested", "summary": "Verified automatic startup", "why": "MCP launches the companion for this project.", "status": "ready",
	}})
	if err != nil || result.IsError {
		t.Fatalf("stdio was not clean MCP: %+v %v", result, err)
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		ready, err := probeCompanion(ctx, endpoint, root)
		if err == nil && !ready {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("owned engine outlived MCP process")
}

func TestEnableSelectsPortAndOutlivesSetupCommand(t *testing.T) {
	executable, err := filepath.Abs("../bin/stef")
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
	command := exec.Command(executable, "--enable", "--client", "codex", "--repo", root, "--view", "ide", "--no-persist", "--quit-after", "5s")
	data, err := command.Output()
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Observing      bool
		Endpoint, Repo string
		View           string
		ViewURL        string `json:"view_url"`
		ViewOpened     bool   `json:"view_opened"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	if !result.Observing || result.Repo != root || result.Endpoint == "" {
		t.Fatalf("invalid enable receipt: %s", data)
	}
	if result.View != "ide" || result.ViewURL != result.Endpoint+"/?companion=1" || result.ViewOpened {
		t.Fatalf("incorrect IDE presentation receipt: %s", data)
	}
	if view, err := projectPresentation(root, ""); err != nil || view != "ide" {
		t.Fatalf("MCP autostart would forget the IDE presentation: %q, %v", view, err)
	}
	if ready, err := probeCompanion(context.Background(), result.Endpoint, root); err != nil || !ready {
		t.Fatalf("resident engine did not outlive setup: %v", err)
	}
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		if ready, err := probeCompanion(context.Background(), result.Endpoint, root); err == nil && !ready {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("demo lifetime did not close resident engine")
}
