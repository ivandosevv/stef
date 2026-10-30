package hooks_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/ivandosevv/stef/daemon/hooks"
)

func TestClaudeLifecycleObservations(t *testing.T) {
	cases := []struct{ event, tool, input, want string }{
		{"SessionStart", "", "{}", "started"},
		{"PreToolUse", "Read", "{}", "reading"},
		{"PreToolUse", "Edit", "{}", "editing"},
		{"PreToolUse", "Bash", `{"command":"go test ./..."}`, "testing"},
		{"PostToolUse", "Bash", "{}", "tool_done"},
		{"PostToolUseFailure", "Bash", "{}", "failed"},
		{"Stop", "", "{}", "completed"},
	}
	for _, tc := range cases {
		t.Run(tc.event+tc.tool, func(t *testing.T) {
			raw := []byte(`{"hook_event_name":"` + tc.event + `","tool_name":"` + tc.tool + `","tool_input":` + tc.input + `}`)
			a, err := hooks.Claude(raw, "/personal/project")
			if err != nil {
				t.Fatal(err)
			}
			if a.Phase != tc.want {
				t.Fatalf("got %s, want %s", a.Phase, tc.want)
			}
			e, err := a.Event()
			if err != nil {
				t.Fatal(err)
			}
			if e.Source != "hook" || e.Category != "activity" {
				t.Fatal("observation was represented as agent reasoning")
			}
		})
	}
}

func TestClaudeWorkersRemainDistinctAndResume(t *testing.T) {
	observe := func(worker string) (string, string) {
		t.Helper()
		raw, _ := json.Marshal(map[string]any{"hook_event_name": "PostToolUse", "session_id": "shared-host-chat", "agent_id": worker, "agent_type": "Explore", "tool_name": "Read", "tool_input": map[string]string{"file_path": "main.go"}})
		activity, err := hooks.Claude(raw, "/personal/project")
		if err != nil {
			t.Fatal(err)
		}
		return activity.SessionID, activity.AgentID
	}
	parentSession, parentAgent := observe("")
	firstSession, firstAgent := observe("worker-a")
	secondSession, secondAgent := observe("worker-b")
	if parentSession == firstSession || parentSession == secondSession || firstSession == secondSession || parentAgent == firstAgent || firstAgent == secondAgent {
		t.Fatal("independent Claude workers share an identity")
	}
	resumedSession, resumedAgent := observe("worker-a")
	if resumedSession != firstSession || resumedAgent != firstAgent {
		t.Fatal("worker identity changed on resume")
	}
}

func TestHookDoesNotLeakPathsOutsideTheTrackedRoot(t *testing.T) {
	fixture := t.TempDir()
	root, outside := filepath.Join(fixture, "project"), filepath.Join(fixture, "outside")
	for _, directory := range []string{filepath.Join(root, "src"), outside} {
		if err := os.MkdirAll(directory, 0700); err != nil {
			t.Fatal(err)
		}
	}
	secret, main := filepath.Join(outside, "secret.txt"), filepath.Join(root, "src", "main.go")
	for _, path := range []string{secret, main} {
		if err := os.WriteFile(path, []byte("fixture content"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct{ name, path, want string }{
		{"outside absolute", secret, ""},
		{"outside traversal", filepath.Join("..", "outside", "secret.txt"), ""},
		{"project absolute", main, "src/main.go"},
		{"project relative", filepath.Join("src", "main.go"), "src/main.go"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := json.Marshal(map[string]any{"hook_event_name": "PreToolUse", "tool_name": "Read", "tool_input": map[string]string{"file_path": tc.path}})
			if err != nil {
				t.Fatal(err)
			}
			activity, err := hooks.Claude(raw, root)
			if err != nil {
				t.Fatal(err)
			}
			if activity.NodeName != tc.want {
				t.Fatalf("observed node = %q, want %q for %q", activity.NodeName, tc.want, tc.path)
			}
		})
	}
}
