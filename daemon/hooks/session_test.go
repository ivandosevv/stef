package hooks

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestHookChatIdentitySurvivesStartResumeAndNewTurns(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, client := range []string{"codex", "claude"} {
		t.Run(client, func(t *testing.T) {
			parse := Claude
			if client == "codex" {
				parse = Codex
			}
			adapt := func(id, event string) string {
				t.Helper()
				raw, _ := json.Marshal(map[string]string{"hook_event_name": event, "session_id": id, "cwd": root})
				activity, err := parse(raw, root)
				if err != nil {
					t.Fatal(err)
				}
				if activity.SessionID == "" || activity.SessionID == id {
					t.Fatal("missing private stable chat identity")
				}
				return activity.SessionID
			}
			first := adapt("first-chat", "SessionStart")
			resumed := adapt("first-chat", "SessionStart")
			next := adapt("second-chat", "SessionStart")
			if first != resumed || first == next {
				t.Fatal("resume or new-chat identity is wrong")
			}
			if client == "codex" && adapt("first-chat", "UserPromptSubmit") != first {
				t.Fatal("new prompt changed the chat identity")
			}
		})
	}
}
