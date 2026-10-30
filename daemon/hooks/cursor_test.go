package hooks_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ivandosevv/stef/daemon/hooks"
	"github.com/ivandosevv/stef/daemon/telemetry"
)

func cursorPayload(t *testing.T, root, event string, extra map[string]any) []byte {
	t.Helper()
	payload := map[string]any{"hook_event_name": event, "conversation_id": "private-conversation", "generation_id": "first-turn", "workspace_roots": []string{root}, "cwd": root}
	for key, value := range extra {
		payload[key] = value
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestCursorUsesDocumentedPostCallbacksWithoutInferringResults(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct{ event, status, phase string }{
		{"sessionStart", "", "started"}, {"sessionEnd", "", "completed"},
		{"postToolUse", "", "tool_done"}, {"postToolUseFailure", "", "failed"},
		{"stop", "completed", "completed"}, {"stop", "aborted", "completed"}, {"stop", "error", "failed"},
	}
	for _, tc := range cases {
		t.Run(tc.event+tc.status, func(t *testing.T) {
			raw := cursorPayload(t, root, tc.event, map[string]any{"status": tc.status, "tool_name": "Shell", "tool_input": map[string]any{"command": "npm test"}, "tool_output": "secret failures"})
			a, err := hooks.Cursor(raw, root)
			if err != nil {
				t.Fatal(err)
			}
			if a.Phase != tc.phase {
				t.Fatalf("phase=%s want %s", a.Phase, tc.phase)
			}
			event, err := a.Event()
			if err != nil {
				t.Fatal(err)
			}
			if event.Source != "hook" || event.Category != "activity" || event.Action != "scouted" {
				t.Fatal("observation was represented as agent reasoning or a passing test")
			}
		})
	}
	for _, event := range []string{"preToolUse", "beforeReadFile", "beforeShellExecution", "afterAgentThought", "afterAgentResponse", "workspaceOpen"} {
		if _, err := hooks.Cursor(cursorPayload(t, root, event, nil), root); !errors.Is(err, hooks.ErrIgnored) {
			t.Fatalf("unregistered event %s was retained: %v", event, err)
		}
	}
}

func TestCursorIdentityIsStableAcrossTurnsAndDistinctAcrossChats(t *testing.T) {
	root, _ := filepath.EvalSymlinks(t.TempDir())
	first, err := hooks.Cursor(cursorPayload(t, root, "sessionStart", nil), root)
	if err != nil {
		t.Fatal(err)
	}
	next, err := hooks.Cursor(cursorPayload(t, root, "postToolUse", map[string]any{"generation_id": "second-turn"}), root)
	if err != nil {
		t.Fatal(err)
	}
	other, err := hooks.Cursor(cursorPayload(t, root, "sessionStart", map[string]any{"conversation_id": "another-chat"}), root)
	if err != nil {
		t.Fatal(err)
	}
	if first.SessionID != next.SessionID || first.AgentID != next.AgentID || first.SessionID == other.SessionID || !strings.HasPrefix(first.SessionID, "cursor-") {
		t.Fatal("conversation identity was not stable and distinct")
	}
	fallback, err := hooks.Cursor(cursorPayload(t, root, "sessionStart", map[string]any{"conversation_id": "", "session_id": "private-conversation"}), root)
	if err != nil || fallback.SessionID != first.SessionID {
		t.Fatal("documented session identity fallback changed the chat")
	}
	if _, err := hooks.Cursor(cursorPayload(t, root, "sessionStart", map[string]any{"conversation_id": ""}), root); err == nil {
		t.Fatal("missing chat identity accepted")
	}
}

func TestCursorStripsSensitiveFieldsAndRejectsOutsideRoots(t *testing.T) {
	root, _ := filepath.EvalSymlinks(t.TempDir())
	raw := cursorPayload(t, root, "postToolUse", map[string]any{"tool_name": "Read", "tool_input": map[string]any{"file_path": "src/main.go", "command": "secret-command"}, "tool_output": "secret-output", "user_email": "secret-email", "transcript_path": "secret-transcript", "text": "secret-thought", "model": "secret-model"})
	a, err := hooks.Cursor(raw, root)
	if err != nil {
		t.Fatal(err)
	}
	if a.NodeName != "src/main.go" {
		t.Fatalf("file missing: %#v", a)
	}
	saved, _ := json.Marshal(a)
	for _, private := range []string{"secret-", "private-conversation", "first-turn", "tool_input", "tool_output", "transcript", "model"} {
		if strings.Contains(string(saved), private) {
			t.Fatalf("private field %s retained", private)
		}
	}
	outside, _ := filepath.EvalSymlinks(t.TempDir())
	for name, extra := range map[string]map[string]any{
		"workspace":         {"workspace_roots": []string{outside}},
		"multiroot":         {"workspace_roots": []string{root, outside}},
		"missing-roots":     {"workspace_roots": []string{}},
		"cwd":               {"cwd": outside},
		"working-directory": {"tool_input": map[string]any{"working_directory": outside}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := hooks.Cursor(cursorPayload(t, root, "postToolUse", extra), root); err == nil {
				t.Fatal("ambiguous/outside root accepted")
			}
		})
	}
	if _, err := hooks.Cursor(cursorPayload(t, root, "postToolUse", map[string]any{"tool_input": map[string]any{"file_path": filepath.Join(outside, "secret.txt")}}), root); !errors.Is(err, hooks.ErrIgnored) {
		t.Fatal("outside file leaked")
	}
	if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	if _, err := hooks.Cursor(cursorPayload(t, root, "postToolUse", map[string]any{"tool_input": map[string]any{"file_path": "link/secret.txt"}}), root); !errors.Is(err, hooks.ErrIgnored) {
		t.Fatalf("symlink escape accepted: %v", err)
	}
}

func TestCursorOwnMCPCallbacksAreIgnoredAndToolLabelsBounded(t *testing.T) {
	root, _ := filepath.EvalSymlinks(t.TempDir())
	for _, tool := range []string{"MCP:report_event", "MCP:get_state", "MCP:report_changes", "MCP:set_session", "MCP:set_workspace", "MCP:stef:set_workspace", "MCP:stef:report_event", "mcp__stef__get_state"} {
		if _, err := hooks.Cursor(cursorPayload(t, root, "postToolUse", map[string]any{"tool_name": tool}), root); !errors.Is(err, hooks.ErrIgnored) {
			t.Fatalf("stef callback %s accepted", tool)
		}
	}
	for tool, want := range map[string]string{"custom-secret-tool": "Tool", "MCP:external-secret-tool": "MCP", "Read": "Read"} {
		a, err := hooks.Cursor(cursorPayload(t, root, "postToolUse", map[string]any{"tool_name": tool}), root)
		if err != nil || a.Tool != want {
			t.Fatalf("unsafe label %q: %#v %v", tool, a, err)
		}
	}
}

func TestCursorSendForwardsOnlySanitizedObservations(t *testing.T) {
	root, _ := filepath.EvalSymlinks(t.TempDir())
	posts := 0
	var received telemetry.Activity
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz":
			json.NewEncoder(w).Encode(map[string]string{"name": "stef", "status": "ok", "repo": root})
		case "/api/activity":
			posts++
			if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
				t.Error(err)
			}
			w.WriteHeader(http.StatusCreated)
		default:
			t.Errorf("unexpected request: %s", r.URL.Path)
		}
	}))
	defer server.Close()
	payload := cursorPayload(t, root, "postToolUseFailure", map[string]any{"tool_name": "Write", "tool_input": map[string]any{"file_path": "main.go"}, "error_message": "secret-error"})
	if err := hooks.Send(context.Background(), server.URL, "cursor", strings.NewReader(string(payload))); err != nil {
		t.Fatal(err)
	}
	if posts != 1 || received.Phase != "failed" || received.NodeName != "main.go" {
		t.Fatalf("observation missing: %#v", received)
	}
	own := cursorPayload(t, root, "postToolUse", map[string]any{"tool_name": "MCP:get_state"})
	if err := hooks.Send(context.Background(), server.URL, "cursor", strings.NewReader(string(own))); err != nil || posts != 1 {
		t.Fatal("own MCP callback forwarded")
	}
}

func TestCursorSendRejectsNonStefEndpoint(t *testing.T) {
	root, _ := filepath.EvalSymlinks(t.TempDir())
	posts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			json.NewEncoder(w).Encode(map[string]string{"repo": root})
			return
		}
		posts++
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()
	if err := hooks.Send(context.Background(), server.URL, "cursor", strings.NewReader(string(cursorPayload(t, root, "sessionStart", nil)))); err == nil || posts != 0 {
		t.Fatal("incompatible endpoint accepted")
	}
}
