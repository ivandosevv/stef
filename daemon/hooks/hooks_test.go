package hooks_test

import (
	"encoding/json"
	"github.com/ivandosevv/stef/daemon/hooks"
	"testing"
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
	a, err := hooks.Claude([]byte(`{"hook_event_name":"PreToolUse","tool_name":"Read","tool_input":{"file_path":"/outside/secret.txt"}}`), "/personal/project")
	if err != nil {
		t.Fatal(err)
	}
	if a.NodeName != "" {
		t.Fatal("outside path exposed")
	}
	a, err = hooks.Claude([]byte(`{"hook_event_name":"PreToolUse","tool_name":"Read","tool_input":{"file_path":"src/main.go"}}`), "/personal/project")
	if err != nil {
		t.Fatal(err)
	}
	if a.NodeName != "src/main.go" {
		t.Fatal("relative project path missing")
	}
}
