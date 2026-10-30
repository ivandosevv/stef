package setup

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAllClientsShareAnExistingProjectEndpoint(t *testing.T) {
	root := t.TempDir()
	const endpoint = "http://127.0.0.1:4837"
	if _, err := Configure(root, "/personal/stef", endpoint, "gemini"); err != nil {
		t.Fatal(err)
	}
	for _, client := range Clients() {
		t.Run(client.Name, func(t *testing.T) {
			found, err := Endpoint(root, client.Name)
			if err != nil || found != endpoint {
				t.Fatalf("different companion: %s %v", found, err)
			}
		})
	}
}

func TestImportedConfigurationCanReuseItsEndpoint(t *testing.T) {
	root := t.TempDir()
	if _, err := Configure(root, "/personal/stef", "http://127.0.0.1:4838", "intellij"); err != nil {
		t.Fatal(err)
	}
	found, err := Endpoint(root, "cursor")
	if err != nil || found != "http://127.0.0.1:4838" {
		t.Fatalf("import's root endpoint lost: %s %v", found, err)
	}
}

func TestEndpointRejectsForeignRootAndMalformedSelectedConfiguration(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".cursor"), 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, ".cursor/mcp.json")
	for _, raw := range []string{
		`{"mcpServers":{"stef":{"command":"stef","args":["--mcp","--repo","/another/root","--daemon","http://127.0.0.1:4838"]}}}`,
		`{"mcpServers":{"stef":{"command":"stef","args":["--mcp","--repo"]}}}`,
		`{"mcpServers":{"stef":{"url":"https://example.invalid/mcp"}}}`,
		`{"mcpServers":{"stef":{"command":"stef","args":[{"--repo":"invalid"}]}}}`,
		`{broken`,
	} {
		if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := Endpoint(root, "cursor"); err == nil {
			t.Fatalf("accepted ambiguous configuration: %s", raw)
		}
	}
}

func TestEndpointIgnoresUnrelatedInvalidConfiguration(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".gemini"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".gemini/settings.json"), []byte("{broken"), 0600); err != nil {
		t.Fatal(err)
	}
	if endpoint, err := Endpoint(root, "cursor"); err != nil || endpoint == "" {
		t.Fatalf("unrelated Gemini configuration blocked Cursor: %s %v", endpoint, err)
	}
	if _, err := Endpoint(root, "unknown-client"); err == nil {
		t.Fatal("unknown client selected a port")
	}
}
