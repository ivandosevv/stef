package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ivandosevv/stef/daemon/hub"
	bridge "github.com/ivandosevv/stef/daemon/mcp"
	"github.com/ivandosevv/stef/daemon/telemetry"
)

func TestCLIReportsExactChangesThroughMCP(t *testing.T) {
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
	if _, err := h.RegisterSession("cli-chat"); err != nil {
		t.Fatal(err)
	}
	observed, err := h.Publish(telemetry.Event{AgentID: "filesystem", NodeName: "theme.ts", Action: "modified", Summary: "File modified", Why: "A file save was observed.", Source: "watcher", Kind: "file"})
	if err != nil {
		t.Fatal(err)
	}
	if err := h.ClaimChanges("cli-chat", []string{"theme.ts"}); err != nil {
		t.Fatal(err)
	}
	for _, captured := range h.SessionSnapshot("cli-chat").Events {
		if captured.Source == "watcher" {
			observed = captured
		}
	}
	arguments := bridge.ChangesInput{SessionID: "cli-chat", AgentID: "cli-agent", Changes: []telemetry.ChangeExplanation{{ChangeID: observed.ID, Summary: "Share theme preference", Why: "Keep appearance consistent across startup and controls."}}}
	input, _ := json.Marshal(arguments)
	var output bytes.Buffer
	if err := reportChangesCLI(context.Background(), server.URL+"/", root, bytes.NewReader(input), &output); err != nil {
		t.Fatal(err)
	}
	var receipt bridge.ChangesReceipt
	if err := json.Unmarshal(output.Bytes(), &receipt); err != nil {
		t.Fatal(err)
	}
	if !receipt.Accepted || len(receipt.IDs) != 1 || receipt.PendingCount != 0 {
		t.Fatalf("missing exact MCP receipt: %+v", receipt)
	}
	events := h.SessionSnapshot("cli-chat").Events
	if last := events[len(events)-1]; last.ChangeID != observed.ID || last.AgentID != "cli-agent" || last.Why != arguments.Changes[0].Why {
		t.Fatalf("wrong revision or reason: %+v", last)
	}
	arguments.Changes[0].ChangeID = "99999"
	input, _ = json.Marshal(arguments)
	output.Reset()
	if err := reportChangesCLI(context.Background(), server.URL, root, bytes.NewReader(input), &output); err == nil || output.Len() != 0 || len(h.SessionSnapshot("cli-chat").Events) != len(events) {
		t.Fatal("rejected exact revision changed history or returned a receipt")
	}
	if _, err := h.SetUpdateMode(telemetry.UpdatesDisabled, "cli-chat"); err != nil {
		t.Fatal(err)
	}
	if err := reportChangesCLI(context.Background(), server.URL, root, bytes.NewReader(input), &output); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(output.Bytes(), &receipt); err != nil || receipt.Accepted || receipt.UpdateMode != telemetry.UpdatesDisabled {
		t.Fatalf("disabled mode receipt lost: %+v %v", receipt, err)
	}
}

func TestCLIRejectsWrongCompanionBeforeMCP(t *testing.T) {
	for _, invalid := range []string{"root", "product", "version", "redirect"} {
		t.Run(invalid, func(t *testing.T) {
			mcpCalls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/healthz" {
					mcpCalls++
					http.Error(w, "must not call MCP", 500)
					return
				}
				if invalid == "redirect" {
					http.Redirect(w, r, "/mcp", http.StatusTemporaryRedirect)
					return
				}
				health := map[string]string{"repo": "/personal/project", "name": "stef", "version": "0.3.0", "status": "ok"}
				switch invalid {
				case "root":
					health["repo"] = "/another/project"
				case "product":
					health["name"] = "other"
				case "version":
					health["version"] = "unknown"
				}
				_ = json.NewEncoder(w).Encode(health)
			}))
			defer server.Close()
			var output bytes.Buffer
			if err := reportChangesCLI(context.Background(), server.URL, "/personal/project", strings.NewReader(`{"agent_id":"agent","changes":[]}`), &output); err == nil || mcpCalls != 0 || output.Len() != 0 {
				t.Fatal("unverified companion received a report")
			}
		})
	}
}

func TestCLIRejectsMalformedInputWithoutConnecting(t *testing.T) {
	for _, input := range []string{"", "{", `{"command":"unexpected"}`, `{} {}`, strings.Repeat("x", 1024*1024+1)} {
		if err := reportChangesCLI(context.Background(), "http://127.0.0.1:1", "/personal/project", strings.NewReader(input), &bytes.Buffer{}); err == nil || strings.Contains(err.Error(), "start stef") {
			t.Fatal("malformed input reached connection attempt")
		}
	}
	if err := reportChangesCLI(context.Background(), "https://example.com", "/personal/project", strings.NewReader(`{}`), &bytes.Buffer{}); err == nil {
		t.Fatal("accepted a non-loopback endpoint")
	}
}
