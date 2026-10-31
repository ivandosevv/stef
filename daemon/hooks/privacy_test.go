package hooks_test

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ivandosevv/stef/daemon/hooks"
	"github.com/ivandosevv/stef/daemon/telemetry"
)

func TestAllActivityAdaptersPersistOnlySanitizedObservations(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for client, adapt := range map[string]func([]byte, string) (telemetry.Activity, error){"claude": hooks.Claude, "codex": hooks.Codex, "cursor": hooks.Cursor} {
		for tool, want := range map[string]string{"Grep": "Grep", "Glob": "Glob", "Search": "Search", "custom-private-name": "Tool", "mcp__private_provider__private_tool": "MCP"} {
			t.Run(client+tool, func(t *testing.T) {
				event := "PostToolUse"
				if client == "cursor" {
					event = "postToolUse"
				}
				raw, _ := json.Marshal(map[string]any{"hook_event_name": event, "session_id": "private-session", "conversation_id": "private-conversation", "cwd": root, "workspace_roots": []string{root}, "tool_name": tool, "tool_input": map[string]string{"pattern": "private-query", "command": "private-command", "file_path": "src/main.go"}, "tool_response": map[string]string{"output": "private-output"}, "prompt": "private-prompt", "thinking": "private-thought", "transcript_path": "private-transcript"})
				activity, err := adapt(raw, root)
				if err != nil {
					t.Fatal(err)
				}
				if activity.Tool != want || activity.Phase != "tool_done" || activity.NodeName != "src/main.go" {
					t.Fatalf("lost safe observation: %#v", activity)
				}
				saved, _ := json.Marshal(activity)
				if strings.Contains(string(saved), "private-") || strings.Contains(string(saved), "private_") {
					t.Fatalf("private hook payload retained: %s", saved)
				}
			})
		}
	}
}
