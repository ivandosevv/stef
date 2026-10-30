package setup

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenCodeSetupUsesDocumentedLocalCommandAndPreservesJSONC(t *testing.T) {
	for _, path := range []string{"opencode.json", "opencode.jsonc"} {
		t.Run(path, func(t *testing.T) {
			root := t.TempDir()
			original := `{/* keep settings */"number":9007199254740993,"mcp":{"other":{"type":"local","command":["other"],},},"tools":{"other*":false},}`
			if err := os.WriteFile(filepath.Join(root, path), []byte(original), 0600); err != nil {
				t.Fatal(err)
			}
			result, err := Configure(root, absoluteFixture("/personal/stef"), "http://127.0.0.1:4837", "open-code")
			if err != nil || result.ConfigPath != path || !result.Configured || result.Client != "opencode" {
				t.Fatalf("configuration: %+v %v", result, err)
			}
			first, err := os.ReadFile(filepath.Join(root, path))
			if err != nil {
				t.Fatal(err)
			}
			document, err := DecodeClientConfig(first, "opencode")
			if err != nil {
				t.Fatal(err)
			}
			if document["number"] != json.Number("9007199254740993") || document["tools"] == nil || !bytes.Contains(first, []byte("/* keep settings */")) {
				t.Fatalf("settings lost: %s", first)
			}
			server := document["mcp"].(map[string]any)["stef"].(map[string]any)
			command, args, err := StdioCommand(server, "opencode")
			if err != nil || command != absoluteFixture("/personal/stef") || strings.Join(args, "|") != "--mcp|--repo|"+root+"|--daemon|http://127.0.0.1:4837" || server["enabled"] != true || server["args"] != nil {
				t.Fatalf("command: %+v %v", server, err)
			}
			if _, err := Configure(root, absoluteFixture("/personal/stef"), "http://127.0.0.1:4837", "opencode"); err != nil {
				t.Fatal(err)
			}
			repeated, _ := os.ReadFile(filepath.Join(root, path))
			if !bytes.Equal(first, repeated) {
				t.Fatal("repeat rewrote source")
			}
			server["enabled"] = false
			server["environment"] = map[string]any{"FLAG": "keep"}
			withPolicy, _ := json.Marshal(document)
			os.WriteFile(filepath.Join(root, path), withPolicy, 0600)
			if _, err := Configure(root, absoluteFixture("/personal/stef"), "http://127.0.0.1:4837", "opencode"); err != nil {
				t.Fatal(err)
			}
			kept, _ := os.ReadFile(filepath.Join(root, path))
			if !bytes.Equal(withPolicy, kept) {
				t.Fatal("explicit disable/environment overwritten")
			}
			found, err := Endpoint(root, "opencode")
			if err != nil || found != "http://127.0.0.1:4837" {
				t.Fatalf("endpoint: %s %v", found, err)
			}
			found, err = Endpoint(root, "generic")
			if err != nil || found != "http://127.0.0.1:4837" {
				t.Fatalf("shared endpoint: %s %v", found, err)
			}
		})
	}
}

func TestOpenCodeRejectsConflictsAndAmbiguousProjectFiles(t *testing.T) {
	for _, original := range []string{
		`{"mcp":{"stef":{"type":"remote","url":"https://example.invalid/mcp"}}}`,
		`{"mcp":{"stef":{"type":"local","command":["other"]}}}`,
		`{"mcp":{"stef":"wrong"}}`, `{"mcp":[]}`, `{"mcp":{},"mcp":{}}`, `{broken`,
	} {
		t.Run(original, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "opencode.jsonc")
			os.WriteFile(path, []byte(original), 0600)
			if _, err := Configure(root, absoluteFixture("/personal/stef"), "http://127.0.0.1:4837", "opencode"); err == nil {
				t.Fatal("conflicting configuration accepted")
			}
			kept, _ := os.ReadFile(path)
			if string(kept) != original {
				t.Fatal("failure altered settings")
			}
		})
	}
	root := t.TempDir()
	for _, path := range []string{"opencode.json", "opencode.jsonc"} {
		os.WriteFile(filepath.Join(root, path), []byte(`{}`), 0600)
	}
	if _, err := Configure(root, absoluteFixture("/personal/stef"), "http://127.0.0.1:4837", "opencode"); err == nil {
		t.Fatal("ambiguous project configuration accepted")
	}
	if _, err := Endpoint(root, "opencode"); err == nil {
		t.Fatal("ambiguous endpoint accepted")
	}
}

func TestOpenCodeNewConfigAndSymlinkConfinement(t *testing.T) {
	root := t.TempDir()
	result, err := Configure(root, absoluteFixture("/personal/stef"), "http://127.0.0.1:4837", "opencode")
	if err != nil || result.ConfigPath != "opencode.json" {
		t.Fatalf("new project: %+v %v", result, err)
	}
	for _, path := range []string{"opencode.json", "opencode.jsonc"} {
		root, outside := t.TempDir(), t.TempDir()
		outsideFile := filepath.Join(outside, "config.json")
		os.WriteFile(outsideFile, []byte(`{}`), 0600)
		if err := os.Symlink(outsideFile, filepath.Join(root, path)); err != nil {
			t.Skip(err)
		}
		if _, err := Configure(root, absoluteFixture("/personal/stef"), "http://127.0.0.1:4837", "opencode"); err == nil {
			t.Fatal("configuration escaped root")
		}
		kept, _ := os.ReadFile(outsideFile)
		if string(kept) != `{}` {
			t.Fatal("outside configuration changed")
		}
	}
}

func TestStdioCommandRejectsInvalidOpenCodeArrays(t *testing.T) {
	for _, server := range []map[string]any{
		{"type": "remote", "command": []any{"stef"}}, {"type": "local", "command": []any{}},
		{"type": "local", "command": []any{42}}, {"type": "local", "command": []any{"stef", 42}},
		{"type": "local", "command": "stef"}, {"type": "local", "command": []any{"stef"}, "url": "https://example.invalid"},
	} {
		if _, _, err := StdioCommand(server, "opencode"); err == nil {
			t.Fatalf("invalid command accepted: %+v", server)
		}
	}
}
