package setup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCodexSetupPreservesOtherSettingsAndIsIdempotent(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".codex"), 0700); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(root, ".codex/config.toml")
	hooksPath := filepath.Join(root, ".codex/hooks.json")
	original := `model = "keep-my-model"
[mcp_servers.other]
command = "other-server"
`
	if err := os.WriteFile(configPath, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(hooksPath, []byte(`{"description":"keep this","hooks":{"Stop":[{"hooks":[{"type":"command","command":"custom-command"}]}]}}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := Codex(root, absoluteFixture("/personal/stef"), "http://127.0.0.1:4825"); err != nil {
		t.Fatal(err)
	}
	config, _ := os.ReadFile(configPath)
	hooks, _ := os.ReadFile(hooksPath)
	if !strings.HasPrefix(string(config), original) || !strings.Contains(string(hooks), "custom-command") {
		t.Fatal("unrelated configuration lost")
	}
	if err := Codex(root, absoluteFixture("/personal/stef"), "http://127.0.0.1:4825"); err != nil {
		t.Fatal(err)
	}
	again, _ := os.ReadFile(configPath)
	hookAgain, _ := os.ReadFile(hooksPath)
	if string(config) != string(again) || string(hooks) != string(hookAgain) {
		t.Fatal("setup duplicated or changed existing configuration")
	}
}
func TestCodexSetupRefusesConflictsAndOutsideSymlinks(t *testing.T) {
	root := t.TempDir()
	os.Mkdir(filepath.Join(root, ".codex"), 0700)
	path := filepath.Join(root, ".codex/config.toml")
	original := `[mcp_servers.stef]
command = "someone-else"
`
	os.WriteFile(path, []byte(original), 0600)
	if err := Codex(root, absoluteFixture("/personal/stef"), "http://127.0.0.1:4825"); err == nil {
		t.Fatal("conflicting server replaced")
	}
	data, _ := os.ReadFile(path)
	if string(data) != original {
		t.Fatal("conflict mutated config")
	}
	if _, err := os.Stat(filepath.Join(root, ".codex/hooks.json")); !os.IsNotExist(err) {
		t.Fatal("wrote hooks despite conflict")
	}
	root = t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, ".codex")); err != nil {
		t.Fatal(err)
	}
	if err := Codex(root, absoluteFixture("/personal/stef"), "http://127.0.0.1:4825"); err == nil {
		t.Fatal("wrote through outside symlink")
	}
	files, _ := os.ReadDir(outside)
	if len(files) != 0 {
		t.Fatal("modified outside root")
	}
}
