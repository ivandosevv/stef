package setup

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestCursorSetupPreservesProjectSettingsAndInstallsOnlyPostHooks(t *testing.T) {
	root := t.TempDir()
	os.Mkdir(filepath.Join(root, ".cursor"), 0700)
	originalServer := map[string]any{"mcpServers": map[string]any{"other": map[string]any{"command": "other-server"}}, "privateOption": "preserved"}
	originalHooks := map[string]any{"version": 1, "privateOption": "preserved", "hooks": map[string]any{"stop": []any{map[string]any{"command": "existing-hook"}}, "beforeReadFile": []any{map[string]any{"command": "existing-policy"}}}}
	for name, document := range map[string]map[string]any{"mcp.json": originalServer, "hooks.json": originalHooks} {
		raw, _ := json.Marshal(document)
		if err := os.WriteFile(filepath.Join(root, ".cursor", name), raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	executable, endpoint := absoluteFixture("/personal/stef bin's/stef"), "http://127.0.0.1:4827"
	if err := Cursor(root, executable, endpoint); err != nil {
		t.Fatal(err)
	}
	firstServer, _ := os.ReadFile(filepath.Join(root, ".cursor/mcp.json"))
	firstHooks, _ := os.ReadFile(filepath.Join(root, ".cursor/hooks.json"))
	var server, hookDocument map[string]any
	json.Unmarshal(firstServer, &server)
	json.Unmarshal(firstHooks, &hookDocument)
	if server["privateOption"] != "preserved" || hookDocument["privateOption"] != "preserved" || server["mcpServers"].(map[string]any)["other"] == nil {
		t.Fatal("unrelated options removed")
	}
	own := server["mcpServers"].(map[string]any)["stef"].(map[string]any)
	if own["command"] != executable || own["type"] != "stdio" || !reflect.DeepEqual(own["args"], []any{"--mcp", "--repo", root, "--daemon", endpoint}) {
		t.Fatal("wrong stdio startup")
	}
	groups := hookDocument["hooks"].(map[string]any)
	if len(groups) != 6 || len(groups["stop"].([]any)) != 2 || !strings.Contains(string(firstHooks), "existing-policy") {
		t.Fatal("unrelated hooks removed or events duplicated")
	}
	for _, event := range []string{"sessionStart", "sessionEnd", "postToolUse", "postToolUseFailure", "stop"} {
		entries := groups[event].([]any)
		own := entries[len(entries)-1].(map[string]any)
		if own["failClosed"] != false || own["timeout"] != float64(3) || own["command"] != cursorHookCommand(runtime.GOOS, executable, endpoint) {
			t.Fatalf("unsafe hook %s: %#v", event, own)
		}
	}
	for _, forbidden := range []string{"preToolUse", "beforeShellExecution", "beforeMCPExecution", "afterAgentThought", "afterAgentResponse", "beforeSubmitPrompt"} {
		if _, exists := groups[forbidden]; exists {
			t.Fatalf("unexpected hook %s", forbidden)
		}
	}
	if err := Cursor(root, executable, endpoint); err != nil {
		t.Fatal(err)
	}
	nextServer, _ := os.ReadFile(filepath.Join(root, ".cursor/mcp.json"))
	nextHooks, _ := os.ReadFile(filepath.Join(root, ".cursor/hooks.json"))
	if string(firstServer) != string(nextServer) || string(firstHooks) != string(nextHooks) {
		t.Fatal("idempotent setup changed files")
	}
}

func TestCursorHookCommandQuotesThePlatformShell(t *testing.T) {
	executable, endpoint := "C:/personal/stef bin's/stef.exe", "http://127.0.0.1:4827"
	if got := cursorHookCommand("windows", executable, endpoint); got != "& 'C:/personal/stef bin''s/stef.exe' --hook --client cursor --daemon 'http://127.0.0.1:4827'" {
		t.Fatalf("wrong PowerShell invocation: %s", got)
	}
	if got := cursorHookCommand("darwin", executable, endpoint); got != shellQuote(executable)+" --hook --client cursor --daemon "+shellQuote(endpoint) {
		t.Fatalf("wrong POSIX invocation: %s", got)
	}
}

func TestCursorConflictsAndInvalidHooksLeaveExistingFilesUntouched(t *testing.T) {
	for name, document := range map[string]map[string]any{
		"server-conflict":           {"mcpServers": map[string]any{"stef": map[string]any{"command": "custom-server", "args": []any{}}}},
		"unsupported-hooks-version": {"version": 2, "hooks": map[string]any{}},
		"invalid-hooks-group":       {"version": 1, "hooks": map[string]any{"postToolUse": true}},
		"old-stef-hook":             {"version": 1, "hooks": map[string]any{"stop": []any{map[string]any{"command": "old-stef --hook --client cursor --daemon http://127.0.0.1:1111"}}}},
		"model-hook-type":           {"version": 1, "hooks": map[string]any{"stop": []any{map[string]any{"command": cursorHookCommand(runtime.GOOS, absoluteFixture("/personal/stef"), "http://127.0.0.1:4827"), "type": "prompt"}}}},
		"invalid-own-matcher":       {"version": 1, "hooks": map[string]any{"stop": []any{map[string]any{"command": cursorHookCommand(runtime.GOOS, absoluteFixture("/personal/stef"), "http://127.0.0.1:4827"), "matcher": true}}}},
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			os.Mkdir(filepath.Join(root, ".cursor"), 0700)
			file := "hooks.json"
			if name == "server-conflict" {
				file = "mcp.json"
			}
			raw, _ := json.Marshal(document)
			path := filepath.Join(root, ".cursor", file)
			os.WriteFile(path, raw, 0600)
			if err := Cursor(root, absoluteFixture("/personal/stef"), "http://127.0.0.1:4827"); err == nil {
				t.Fatal("unsafe configuration accepted")
			}
			after, _ := os.ReadFile(path)
			if string(after) != string(raw) {
				t.Fatal("conflicting file overwritten")
			}
			other := "mcp.json"
			if file == other {
				other = "hooks.json"
			}
			if _, err := os.Stat(filepath.Join(root, ".cursor", other)); !os.IsNotExist(err) {
				t.Fatal("partial configuration written")
			}
		})
	}
}

func TestCursorSetupDoesNotEscapeProjectRoot(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, ".cursor")); err != nil {
		t.Fatal(err)
	}
	if err := Cursor(root, absoluteFixture("/personal/stef"), "http://127.0.0.1:4827"); err == nil {
		t.Fatal("symlink escape accepted")
	}
	entries, _ := os.ReadDir(outside)
	if len(entries) != 0 {
		t.Fatal("outside configuration written")
	}
	if err := Cursor(t.TempDir(), absoluteFixture("/personal/stef"), "http://example.com"); err == nil {
		t.Fatal("remote daemon accepted")
	}
}
