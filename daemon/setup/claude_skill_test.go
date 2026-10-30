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

func TestClaudeSetupInstallsStandaloneSkill(t *testing.T) {
	repo := filepath.Join(t.TempDir(), "developer's project")
	if err := os.Mkdir(repo, 0700); err != nil {
		t.Fatal(err)
	}
	executable := absoluteFixture("/personal/stef's runtime/bin/stef")
	endpoint := "http://127.0.0.1:4827"
	if err := Claude(repo, executable, endpoint); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(repo, ClaudeSkillPath)
	first, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// The installed skill can reconnect without checkout-relative assumptions,
	// and shell paths with spaces or quotes remain a single argument.
	command := reconnectCommand(runtime.GOOS, executable, repo, endpoint)
	if !strings.Contains(string(first), command) || strings.Contains(string(first), "{{") {
		t.Fatalf("skill lacks resolved reconnect command: %s", first)
	}
	if !strings.Contains(string(first), "name: stef") || !strings.Contains(string(first), "report_changes") || !strings.Contains(string(first), "update_mode") {
		t.Fatal("installed skill lacks discovery or reporting instructions")
	}
	if err := os.Chmod(path, 0640); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := Claude(repo, executable, endpoint); err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) || !before.ModTime().Equal(after.ModTime()) || after.Mode().Perm() != before.Mode().Perm() {
		t.Fatal("repeated setup rewrote the installed skill")
	}
	mcpRaw, err := os.ReadFile(filepath.Join(repo, ".mcp.json"))
	if err != nil {
		t.Fatal(err)
	}
	var config map[string]any
	if err := json.Unmarshal(mcpRaw, &config); err != nil {
		t.Fatal(err)
	}
	server := config["mcpServers"].(map[string]any)["stef"].(map[string]any)
	if server["command"] != executable || server["args"].([]any)[2] != repo {
		t.Fatal("skill and MCP configuration use different project paths")
	}
}

func TestClaudeSetupPreservesExistingSkill(t *testing.T) {
	for _, linked := range []bool{false, true} {
		name := "directory"
		if linked {
			name = "checkout symlink"
		}
		t.Run(name, func(t *testing.T) {
			repo := t.TempDir()
			skillDir := filepath.Join(repo, ".claude/skills/stef")
			if linked {
				skillDir = filepath.Join(repo, ".agents/skills/stef")
			}
			if err := os.MkdirAll(skillDir, 0700); err != nil {
				t.Fatal(err)
			}
			original := []byte("---\nname: stef\ndescription: The developer's existing workflow.\n---\n\nKeep these custom instructions.\n")
			if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), original, 0640); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(skillDir, "helper.md"), []byte("custom helper"), 0600); err != nil {
				t.Fatal(err)
			}
			link := filepath.Join(repo, ".claude/skills/stef")
			var originalLink os.FileInfo
			var originalTarget string
			if linked {
				if err := os.MkdirAll(filepath.Join(repo, ".claude/skills"), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink("../../.agents/skills/stef", link); err != nil {
					t.Fatal(err)
				}
				info, err := os.Lstat(link)
				if err != nil || info.Mode()&os.ModeSymlink == 0 {
					t.Fatalf("fixture is not a skill symlink: %v", err)
				}
				originalLink = info
				target, err := os.Readlink(link)
				if err != nil {
					t.Fatal(err)
				}
				originalTarget = target
			}
			if err := Claude(repo, absoluteFixture("/personal/stef"), "http://127.0.0.1:4827"); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(filepath.Join(repo, ClaudeSkillPath))
			if err != nil || !bytes.Equal(data, original) {
				t.Fatalf("existing skill changed: %v", err)
			}
			helper, err := os.ReadFile(filepath.Join(skillDir, "helper.md"))
			if err != nil || string(helper) != "custom helper" {
				t.Fatalf("existing helper changed: %v", err)
			}
			if linked {
				after, err := os.Lstat(link)
				if err != nil || after.Mode()&os.ModeSymlink == 0 || !os.SameFile(originalLink, after) || after.Mode() != originalLink.Mode() || !after.ModTime().Equal(originalLink.ModTime()) {
					t.Fatalf("checkout skill link changed: %v", err)
				}
				target, err := os.Readlink(link)
				if err != nil || target != originalTarget {
					t.Fatalf("checkout skill link target = %q, want %q: %v", target, originalTarget, err)
				}
			}
		})
	}
}

func TestClaudeSetupRejectsEscapingOrBrokenSkillLinksBeforeMutating(t *testing.T) {
	for _, component := range []string{".claude", ".claude/skills", ".claude/skills/stef", ClaudeSkillPath} {
		for _, broken := range []bool{false, true} {
			name := component + "/escaping"
			if broken {
				name = component + "/dangling"
			}
			t.Run(name, func(t *testing.T) {
				repo := t.TempDir()
				outside := t.TempDir()
				marker := filepath.Join(outside, "keep")
				if err := os.WriteFile(marker, []byte("original"), 0600); err != nil {
					t.Fatal(err)
				}
				path := filepath.Join(repo, component)
				if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
					t.Fatal(err)
				}
				target := outside
				if component == ClaudeSkillPath {
					target = marker
				}
				if broken {
					target = filepath.Join(repo, "missing")
				}
				// Relative escaping links exercise Root's traversal protection,
				// rather than rejection based on an absolute link alone.
				relative, err := filepath.Rel(filepath.Dir(path), target)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(relative, path); err != nil {
					t.Fatal(err)
				}
				mcpPath := filepath.Join(repo, ".mcp.json")
				original := []byte("{\"mcpServers\":{\"custom\":{\"command\":\"keep\"}}}")
				if err := os.WriteFile(mcpPath, original, 0600); err != nil {
					t.Fatal(err)
				}
				if err := Claude(repo, absoluteFixture("/personal/stef"), "http://127.0.0.1:4827"); err == nil {
					t.Fatal("invalid skill link accepted")
				}
				data, err := os.ReadFile(mcpPath)
				if err != nil || !bytes.Equal(data, original) {
					t.Fatalf("MCP configuration changed before skill validation: %v", err)
				}
				entries, err := os.ReadDir(outside)
				if err != nil || len(entries) != 1 || entries[0].Name() != "keep" {
					t.Fatalf("setup wrote outside the selected root: %v %v", entries, err)
				}
				data, err = os.ReadFile(marker)
				if err != nil || string(data) != "original" {
					t.Fatalf("outside file changed: %v", err)
				}
			})
		}
	}
}

func TestClaudeSetupRejectsMalformedSkillPaths(t *testing.T) {
	for _, malformed := range []string{".claude file", "skills file", "stef file", "missing entrypoint", "empty entrypoint", "directory entrypoint"} {
		t.Run(malformed, func(t *testing.T) {
			repo := t.TempDir()
			path := filepath.Join(repo, ".claude")
			if malformed != ".claude file" {
				path = filepath.Join(repo, ".claude/skills")
			}
			if malformed != ".claude file" && malformed != "skills file" {
				path = filepath.Join(repo, ".claude/skills/stef")
			}
			if strings.Contains(malformed, "entrypoint") {
				path = filepath.Join(repo, ClaudeSkillPath)
			}
			if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				t.Fatal(err)
			}
			switch malformed {
			case "missing entrypoint":
			case "directory entrypoint":
				if err := os.Mkdir(path, 0700); err != nil {
					t.Fatal(err)
				}
			default:
				if err := os.WriteFile(path, nil, 0600); err != nil {
					t.Fatal(err)
				}
			}
			if err := Claude(repo, absoluteFixture("/personal/stef"), "http://127.0.0.1:4827"); err == nil {
				t.Fatal("malformed skill accepted")
			}
			if _, err := os.Stat(filepath.Join(repo, ".mcp.json")); !os.IsNotExist(err) {
				t.Fatal("wrote MCP despite malformed skill")
			}
			if _, err := os.Stat(filepath.Join(repo, ".claude/settings.local.json")); err == nil {
				t.Fatal("wrote hooks despite malformed skill")
			}
		})
	}
}
