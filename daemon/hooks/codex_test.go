package hooks_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ivandosevv/stef/daemon/hooks"
)

func TestCodexLifecycleObservations(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		event, tool, command string
		response             any
		want                 string
	}{
		{"SessionStart", "", "", nil, "started"},
		{"UserPromptSubmit", "", "", nil, "started"},
		{"PreToolUse", "Bash", "rg --files", nil, "searching"},
		{"PreToolUse", "Grep", "", nil, "searching"},
		{"PreToolUse", "Glob", "", nil, "searching"},
		{"PreToolUse", "Search", "", nil, "searching"},
		{"PreToolUse", "exec_command", "grep private-pattern src/main.go", nil, "searching"},
		{"PreToolUse", "Bash", "find src -name '*.go'", nil, "searching"},
		{"PreToolUse", "Bash", "sed -n '1,20p' src/main.go", nil, "reading"},
		{"PreToolUse", "apply_patch", "*** Begin Patch\n*** Update File: main.go\n*** End Patch", nil, "editing"},
		{"PreToolUse", "Bash", "python3 -m unittest", nil, "testing"},
		{"PreToolUse", "Bash", "cd ui && npm run test:e2e", nil, "testing"},
		{"PreToolUse", "Bash", "echo go test", nil, "terminal"},
		{"PreToolUse", "Bash", "git status", nil, "terminal"},
		{"PostToolUse", "Bash", "", map[string]any{"exit_code": 0}, "tool_done"},
		{"PostToolUse", "Bash", "", map[string]any{"metadata": map[string]int{"exit_code": 1}}, "failed"},
		{"PostToolUse", "Bash", "", "Process exited with code 1\nPrivate output", "failed"},
		{"Stop", "", "", nil, "completed"},
	} {
		t.Run(tc.event+tc.tool+tc.want, func(t *testing.T) {
			raw, _ := json.Marshal(map[string]any{"session_id": "test-session", "cwd": root, "hook_event_name": tc.event, "tool_name": tc.tool, "tool_input": map[string]string{"command": tc.command}, "tool_response": tc.response, "prompt": "do not persist this prompt", "transcript_path": "/private/transcript"})
			activity, err := hooks.Codex(raw, root)
			if err != nil {
				t.Fatal(err)
			}
			if activity.Phase != tc.want {
				t.Fatalf("got %s, want %s", activity.Phase, tc.want)
			}
			if tc.tool == "apply_patch" && activity.NodeName != "main.go" {
				t.Fatal("patch context missing")
			}
			event, err := activity.Event()
			if err != nil {
				t.Fatal(err)
			}
			if event.Source != "hook" || event.Category != "activity" {
				t.Fatal("hook misrepresented as rationale")
			}
			data, _ := json.Marshal(event)
			for _, secret := range []string{"do not persist", "Private output", "transcript", "*** Begin Patch"} {
				if strings.Contains(string(data), secret) {
					t.Fatal("raw hook material persisted")
				}
			}
		})
	}
}
func TestCodexHookConfinementAndSelfObservation(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(root, "src")
	if err := os.Mkdir(sub, 0700); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(map[string]any{"cwd": sub, "hook_event_name": "PreToolUse", "tool_name": "Read", "tool_input": map[string]string{"file_path": "file.go"}})
	a, err := hooks.Codex(raw, root)
	if err != nil || a.NodeName != "src/file.go" {
		t.Fatalf("nested cwd rejected: %+v %v", a, err)
	}
	raw, _ = json.Marshal(map[string]any{"cwd": root, "hook_event_name": "PreToolUse", "tool_name": "Read", "tool_input": map[string]string{"file_path": "../outside.txt"}})
	a, err = hooks.Codex(raw, root)
	if err != nil || a.NodeName != "" {
		t.Fatal("outside path exposed")
	}
	raw, _ = json.Marshal(map[string]any{"cwd": root, "hook_event_name": "PreToolUse", "tool_name": "mcp__stef__report_event"})
	if _, err = hooks.Codex(raw, root); !errors.Is(err, hooks.ErrIgnored) {
		t.Fatal("stef's own calls were re-observed")
	}
	raw, _ = json.Marshal(map[string]any{"cwd": t.TempDir(), "hook_event_name": "SessionStart"})
	if _, err = hooks.Codex(raw, root); err == nil {
		t.Fatal("outside cwd accepted")
	}
}
