package setup

import (
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/ivandosevv/stef/daemon/mcp"
)

// Endpoint shares a root's existing managed companion across MCP clients.
func Endpoint(repo, client string) (string, error) {
	selected, err := LookupClient(client)
	if err != nil {
		return "", err
	}
	root, err := os.OpenRoot(repo)
	if err != nil {
		return "", err
	}
	defer root.Close()
	clients := append([]Client{selected}, Clients()...)
	seen := map[string]bool{}
	for _, definition := range clients {
		if seen[definition.ConfigPath] {
			continue
		}
		seen[definition.ConfigPath] = true
		raw, err := readOptional(root, definition.ConfigPath)
		if err != nil {
			if definition.Name == selected.Name {
				return "", err
			}
			continue
		}
		if len(raw) == 0 {
			continue
		}
		document, err := DecodeClientConfig(raw, definition.Name)
		if err != nil {
			if definition.Name == selected.Name {
				return "", fmt.Errorf("invalid existing %s configuration: %w", definition.Name, err)
			}
			continue // An unrelated malformed client must not block this client's setup.
		}
		servers, _ := document[definition.ServerKey].(map[string]any)
		value, exists := servers["stef"]
		if !exists {
			continue
		}
		server, ok := value.(map[string]any)
		if !ok {
			return "", fmt.Errorf("existing stef configuration in %s must be an object", definition.ConfigPath)
		}
		args, ok := server["args"].([]any)
		if !ok {
			return "", fmt.Errorf("existing stef configuration in %s is not a managed stdio server", definition.ConfigPath)
		}
		endpoint, trackedRoot := "http://127.0.0.1:4820", ""
		for i, arg := range args {
			flag, ok := arg.(string)
			if !ok {
				return "", fmt.Errorf("stef arguments in %s must be strings", definition.ConfigPath)
			}
			if flag != "--daemon" && flag != "--repo" {
				continue
			}
			if i+1 >= len(args) {
				return "", fmt.Errorf("incomplete stef arguments in %s", definition.ConfigPath)
			}
			value, ok := args[i+1].(string)
			if !ok || value == "" {
				return "", fmt.Errorf("invalid stef arguments in %s", definition.ConfigPath)
			}
			if flag == "--daemon" {
				endpoint = value
			} else {
				trackedRoot = value
			}
		}
		if trackedRoot != repo {
			return "", fmt.Errorf("existing stef configuration in %s watches a different root", definition.ConfigPath)
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
