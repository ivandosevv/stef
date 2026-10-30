package setup

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestClaudeSetupPreservesSettingsAndSharesCodexEndpoint(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".claude"), 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, ".claude/settings.local.json")
	original := `{"permissions":{"deny":["Bash(rm *)"]},"hooks":{"Stop":[{"hooks":[{"type":"command","command":"existing-hook"}]}]}}`
	if err := os.WriteFile(path, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	if err := Codex(root, absoluteFixture("/personal/stef"), "http://127.0.0.1:4827"); err != nil {
		t.Fatal(err)
	}
	endpoint, err := Endpoint(root, "claude")
	if err != nil || endpoint != "http://127.0.0.1:4827" {
		t.Fatalf("did not share project endpoint: %s %v", endpoint, err)
	}
	if err := Claude(root, absoluteFixture("/personal/stef"), endpoint); err != nil {
		t.Fatal(err)
	}
	first, _ := os.ReadFile(path)
	mcpPath := filepath.Join(root, ".mcp.json")
	mcpFirst, _ := os.ReadFile(mcpPath)
	var settings map[string]any
	if err := json.Unmarshal(first, &settings); err != nil {
		t.Fatal(err)
	}
	if settings["permissions"] == nil || !strings.Contains(string(first), "existing-hook") {
		t.Fatal("unrelated settings overwritten")
	}
	if err := Claude(root, absoluteFixture("/personal/stef"), endpoint); err != nil {
		t.Fatal(err)
	}
	second, _ := os.ReadFile(path)
	mcpSecond, _ := os.ReadFile(mcpPath)
	if string(first) != string(second) || string(mcpFirst) != string(mcpSecond) {
		t.Fatal("repeated setup duplicated configuration")
	}
}
func TestClaudeConflictPreservesExistingFiles(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ".mcp.json")
	original := `{"mcpServers":{"stef":{"command":"custom-server","args":[]}}}`
	os.WriteFile(path, []byte(original), 0600)
	if err := Claude(root, absoluteFixture("/personal/stef"), "http://127.0.0.1:4827"); err == nil {
		t.Fatal("conflicting entry replaced")
	}
	data, _ := os.ReadFile(path)
	if string(data) != original {
		t.Fatal("configuration changed despite conflict")
	}
	if _, err := os.Stat(filepath.Join(root, ".claude")); !os.IsNotExist(err) {
		t.Fatal("wrote settings despite conflict")
	}
}
