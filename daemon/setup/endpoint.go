package setup

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/ivandosevv/stef/daemon/mcp"
	"github.com/pelletier/go-toml/v2"
)

// Endpoint reuses this root's configured endpoint or selects a free loopback port.
func Endpoint(repo, client string) (string, error) {
	root, err := os.OpenRoot(repo)
	if err != nil {
		return "", err
	}
	defer root.Close()
	clients := []string{client}
	if client == "codex" {
		clients = append(clients, "claude")
	} else {
		clients = append(clients, "codex")
	}
	for _, name := range clients {
		var document map[string]any
		path, key := ".mcp.json", "mcpServers"
		if name == "codex" {
			path, key = ".codex/config.toml", "mcp_servers"
		}
		raw, err := readOptional(root, path)
		if err != nil {
			return "", err
		}
		if len(raw) == 0 {
			continue
		}
		if name == "codex" {
			err = toml.Unmarshal(raw, &document)
		} else {
			err = json.Unmarshal(raw, &document)
		}
		if err != nil {
			return "", fmt.Errorf("invalid existing client configuration: %w", err)
		}
		servers, _ := document[key].(map[string]any)
		server, _ := servers["stef"].(map[string]any)
		if server == nil {
			continue
		}
		args, _ := server["args"].([]any)
		endpoint := "http://127.0.0.1:4820"
		for i, arg := range args {
			if arg == "--daemon" && i+1 < len(args) {
				endpoint, _ = args[i+1].(string)
			}
			if arg == "--repo" && i+1 < len(args) && args[i+1] != repo {
				return "", fmt.Errorf("existing stef configuration watches a different root")
			}
		}
		if err := mcp.ValidateEndpoint(endpoint); err != nil {
			return "", err
		}
		return strings.TrimSuffix(endpoint, "/"), nil
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	endpoint := "http://" + listener.Addr().String()
	listener.Close()
	return endpoint, nil
}
