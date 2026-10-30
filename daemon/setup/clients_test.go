package setup

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestClientCatalogAliasesAndIsolation(t *testing.T) {
	for alias, name := range map[string]string{"claude-code": "claude", " IntelliJ ": "jetbrains", "PyCharm": "jetbrains", "copilot": "vscode", "gemini-cli": "gemini", "roo-code": "roo", "devin-local": "devin"} {
		client, err := LookupClient(alias)
		if err != nil || client.Name != name {
			t.Fatalf("%s: %#v %v", alias, client, err)
		}
	}
	clients := Clients()
	if len(clients) < 12 {
		t.Fatalf("missing documented clients: %d", len(clients))
	}
	seen := map[string]bool{}
	for _, client := range clients {
		if seen[client.Name] || client.ConfigPath == "" || client.ServerKey == "" || client.NextStep == "" || !strings.HasPrefix(client.DocsURL, "https://") {
			t.Fatalf("incomplete or repeated client: %#v", client)
		}
		seen[client.Name] = true
	}
	clients[0].Name = "changed"
	if Clients()[0].Name == "changed" {
		t.Fatal("caller changed catalog")
	}
}

func TestJSONClientSetupPreservesSettingsAndPolicies(t *testing.T) {
	for _, name := range []string{"vscode", "gemini", "roo", "devin"} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			client, _ := LookupClient(name)
			path := filepath.Join(root, client.ConfigPath)
			if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				t.Fatal(err)
			}
			original := `{"number":9007199254740993,"inputs":[{"id":"existing"}],"` + client.ServerKey + `":{"other":{"command":"existing-server","args":["untouched"]}}}`
			if err := os.WriteFile(path, []byte(original), 0600); err != nil {
				t.Fatal(err)
			}
			result, err := Configure(root, "/personal/stef/bin/stef", "http://127.0.0.1:4827", name)
			if err != nil || !result.Configured || result.Client != name || result.ConfigPath != client.ConfigPath || result.HooksPath != "" {
				t.Fatalf("configure: %#v %v", result, err)
			}
			first, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			document, err := DecodeClientConfig(first, name)
			if err != nil || document["number"] != json.Number("9007199254740993") || document["inputs"] == nil {
				t.Fatalf("unrelated settings altered: %#v %v", document, err)
			}
			servers := document[client.ServerKey].(map[string]any)
			if servers["other"].(map[string]any)["command"] != "existing-server" {
				t.Fatal("other server changed")
			}
			server := servers["stef"].(map[string]any)
			args, _ := json.Marshal(server["args"])
			expected, _ := json.Marshal([]string{"--mcp", "--repo", root, "--daemon", "http://127.0.0.1:4827"})
			if server["command"] != "/personal/stef/bin/stef" || !bytes.Equal(args, expected) {
				t.Fatalf("incorrect shared stdio setup: %#v", server)
			}
			if name == "vscode" && server["type"] != "stdio" {
				t.Fatal("VS Code requires its own type field")
			}
			if _, err := Configure(root, "/personal/stef/bin/stef", "http://127.0.0.1:4827", name); err != nil {
				t.Fatal(err)
			}
			second, _ := os.ReadFile(path)
			if !bytes.Equal(first, second) {
				t.Fatal("repeated setup rewrote existing settings")
			}
			// A user's explicit disable/tool policy must survive another enable command.
			server["disabled"] = true
			server["env"] = map[string]any{"CUSTOM_FLAG": "kept"}
			withPolicy, _ := json.Marshal(document)
			os.WriteFile(path, withPolicy, 0600)
			if _, err := Configure(root, "/personal/stef/bin/stef", "http://127.0.0.1:4827", name); err != nil {
				t.Fatal(err)
			}
			preserved, _ := os.ReadFile(path)
			if !bytes.Equal(withPolicy, preserved) {
				t.Fatal("existing stef policy overwritten")
			}
		})
	}
}

func TestImportClientsExportTheirOwnSchemaWithoutProfileWrites(t *testing.T) {
	for _, name := range []string{"jetbrains", "windsurf", "cline", "hermes", "generic"} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			client, _ := LookupClient(name)
			result, err := Configure(root, "/personal/stef", "http://127.0.0.1:4827", name)
			if err != nil || result.Configured || !strings.HasPrefix(result.ConfigPath, ".stef/clients/") || result.NextStep == "" {
				t.Fatalf("import result falsely claims loading: %#v %v", result, err)
			}
			data, err := os.ReadFile(filepath.Join(root, result.ConfigPath))
			if err != nil {
				t.Fatal(err)
			}
			document, err := DecodeClientConfig(data, name)
			if err != nil || document[client.ServerKey] == nil || len(document) != 1 {
				t.Fatalf("wrong export schema: %#v %v", document, err)
			}
			if name == "hermes" && document["mcpServers"] != nil {
				t.Fatal("Hermes does not use camel-case mcpServers")
			}
			if _, err := Configure(root, "/personal/stef", "http://127.0.0.1:4827", name); err != nil {
				t.Fatal(err)
			}
			repeated, _ := os.ReadFile(filepath.Join(root, result.ConfigPath))
			if !bytes.Equal(data, repeated) {
				t.Fatal("export changed on repeat")
			}
			entries, _ := os.ReadDir(root)
			if len(entries) != 1 || entries[0].Name() != ".stef" {
				t.Fatalf("unexpected client/global configuration: %v", entries)
			}
		})
	}
}

func TestClientSetupFailuresPreserveFiles(t *testing.T) {
	for _, name := range []string{"vscode", "gemini", "roo", "devin", "jetbrains", "hermes"} {
		for _, body := range []string{
			`{"KEY":{"stef":{"command":"other","args":[]}}}`,
			`{"KEY":[]}`,
			`{"KEY":{"stef":"not-an-object"}}`,
			`null`, `{"unterminated":`,
		} {
			t.Run(name+body, func(t *testing.T) {
				root := t.TempDir()
				client, _ := LookupClient(name)
				path := filepath.Join(root, client.ConfigPath)
				os.MkdirAll(filepath.Dir(path), 0700)
				original := []byte(strings.ReplaceAll(body, "KEY", client.ServerKey))
				os.WriteFile(path, original, 0600)
				if _, err := Configure(root, "/personal/stef", "http://127.0.0.1:4827", name); err == nil {
					t.Fatal("invalid or conflicting settings accepted")
				}
				data, _ := os.ReadFile(path)
				if !bytes.Equal(data, original) {
					t.Fatal("failure changed settings")
				}
			})
		}
	}
}

func TestClientSetupRefusesUnsupportedNamesInvalidEndpointsAndEscapes(t *testing.T) {
	root := t.TempDir()
	for _, input := range []struct{ name, executable, endpoint string }{
		{"unknown", "/personal/stef", "http://127.0.0.1:4827"},
		{"../escape", "/personal/stef", "http://127.0.0.1:4827"},
		{"gemini", "relative-stef", "http://127.0.0.1:4827"},
		{"gemini", "/personal/stef", "https://external.example.com"},
	} {
		if _, err := Configure(root, input.executable, input.endpoint, input.name); err == nil {
			t.Fatalf("unsafe setup accepted: %#v", input)
		}
	}
	entries, _ := os.ReadDir(root)
	if len(entries) != 0 {
		t.Fatal("invalid setup wrote files")
	}
	for _, name := range []string{"gemini", "jetbrains"} {
		t.Run(name, func(t *testing.T) {
			root, outside := t.TempDir(), t.TempDir()
			client, _ := LookupClient(name)
			parent := strings.Split(client.ConfigPath, "/")[0]
			if err := os.Symlink(outside, filepath.Join(root, parent)); err != nil {
				t.Skip(err)
			}
			if _, err := Configure(root, "/personal/stef", "http://127.0.0.1:4827", name); err == nil {
				t.Fatal("configuration escaped the selected root")
			}
			entries, _ := os.ReadDir(outside)
			if len(entries) != 0 {
				t.Fatal("outside directory modified")
			}
		})
	}
}

func TestVSCodeJSONCInsertionPreservesCommentsAndExactSettings(t *testing.T) {
	for _, original := range []string{
		`{/* keep empty root */}`,
		`{"inputs":[{"id":"brace } and // quoted",}], /* keep root */}`,
		`{"servers":{/* keep empty servers */},"value":9007199254740993}`,
		`{"servers":{"other":{"command":"kept","args":["http://quoted/* value */",],}, // keep server
},"inputs":[1,2,],}`,
		`{"servers":{"other":{"command":"kept"}} /* no trailing comma */}`,
	} {
		t.Run(original, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, ".vscode/mcp.json")
			os.MkdirAll(filepath.Dir(path), 0700)
			os.WriteFile(path, []byte(original), 0600)
			if _, err := Configure(root, "/personal/stef", "http://127.0.0.1:4827", "vscode"); err != nil {
				t.Fatal(err)
			}
			next, _ := os.ReadFile(path)
			if _, err := DecodeClientConfig(next, "vscode"); err != nil {
				t.Fatal(err)
			}
			// Removing only the inserted source must recover the exact original bytes.
			prefix := 0
			for prefix < len(original) && prefix < len(next) && original[prefix] == next[prefix] {
				prefix++
			}
			suffix := len(original) - prefix
			if suffix < 0 || !bytes.Equal(next[len(next)-suffix:], []byte(original[prefix:])) {
				t.Fatal("existing JSONC source/comments were rewritten")
			}
			if _, err := Configure(root, "/personal/stef", "http://127.0.0.1:4827", "vscode"); err != nil {
				t.Fatal(err)
			}
			repeated, _ := os.ReadFile(path)
			if !bytes.Equal(next, repeated) {
				t.Fatal("repeat changed JSONC comments")
			}
		})
	}
	for _, original := range []string{
		`{"servers":{},"servers":{}}`,
		`{/* unterminated}`,
		`{"servers":"not an object"}`,
	} {
		root := t.TempDir()
		path := filepath.Join(root, ".vscode/mcp.json")
		os.MkdirAll(filepath.Dir(path), 0700)
		os.WriteFile(path, []byte(original), 0600)
		if _, err := Configure(root, "/personal/stef", "http://127.0.0.1:4827", "vscode"); err == nil {
			t.Fatalf("unsafe JSONC accepted: %s", original)
		}
		data, _ := os.ReadFile(path)
		if string(data) != original {
			t.Fatal("invalid JSONC changed")
		}
	}
}
