package setup

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// The setup fixtures identify fake executables without requiring a Unix root.
func absoluteFixture(value string) string {
	if runtime.GOOS == "windows" {
		return "C:" + filepath.FromSlash(value)
	}
	return value
}

func TestWindowsSetupUsesMCPWithoutEditingHookSettings(t *testing.T) {
	for _, client := range []string{"codex", "claude"} {
		t.Run(client, func(t *testing.T) {
			repo := t.TempDir()
			executable, endpoint := absoluteFixture("/personal/stef.exe"), "http://127.0.0.1:4827"
			definition, err := LookupClient(client)
			if err != nil {
				t.Fatal(err)
			}
			definition = platformClient(definition, "windows")
			if definition.HooksPath != "" || !strings.Contains(definition.NextStep, "MCP") {
				t.Fatal("Windows setup claims to install shell hooks")
			}
			hookPath := ".codex/hooks.json"
			if client == "claude" {
				hookPath = ".claude/settings.local.json"
			}
			path := filepath.Join(repo, hookPath)
			if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				t.Fatal(err)
			}
			previous := []byte(`{"hooks":{"Stop":[{"hooks":[{"type":"command","command":"user-owned"}]}]},"private":true}`)
			if err := os.WriteFile(path, previous, 0600); err != nil {
				t.Fatal(err)
			}
			configure := codexForPlatform
			if client == "claude" {
				configure = claudeForPlatform
			}
			for attempt := 0; attempt < 2; attempt++ {
				if err := configure(repo, executable, endpoint, "windows"); err != nil {
					t.Fatal(err)
				}
				after, err := os.ReadFile(path)
				if err != nil || !bytes.Equal(after, previous) {
					t.Fatal("MCP setup changed Windows shell hooks")
				}
			}
			configured, err := Endpoint(repo, client)
			if err != nil || configured != endpoint {
				t.Fatalf("Windows MCP lost its project endpoint: %s %v", configured, err)
			}
			if client == "claude" {
				raw, err := os.ReadFile(filepath.Join(repo, ".mcp.json"))
				if err != nil {
					t.Fatal(err)
				}
				var document map[string]map[string]map[string]any
				if err := json.Unmarshal(raw, &document); err != nil || document["mcpServers"]["stef"]["command"] != executable {
					t.Fatalf("MCP setup did not retain an exact executable path: %s %v", raw, err)
				}
			}
		})
	}
}

func TestWindowsReconnectQuotesPathsForPowerShell(t *testing.T) {
	executable := `C:\Users\Pat's project $()\stef.exe`
	repo := `C:\Users\Pat's project $()`
	endpoint := "http://127.0.0.1:4827"
	want := `& 'C:\Users\Pat''s project $()\stef.exe' --enable --client claude --view ide --repo 'C:\Users\Pat''s project $()' --daemon 'http://127.0.0.1:4827'`
	if got := reconnectCommand("windows", executable, repo, endpoint); got != want {
		t.Fatalf("Windows paths were not retained as literal arguments: %s", got)
	}
	if got := reportCommand("windows", executable, repo, endpoint); !strings.HasPrefix(got, `& 'C:\Users\Pat''s project $()\stef.exe' --report-changes`) {
		t.Fatalf("Windows report command cannot invoke its quoted executable: %s", got)
	}
}
