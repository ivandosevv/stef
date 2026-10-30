package setup

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"runtime"
	"strings"

	"github.com/ivandosevv/stef/daemon/mcp"
)

// Cursor installs only local MCP and observational hooks, never permission hooks.
// Contract: https://cursor.com/docs/mcp and https://cursor.com/docs/hooks.
func Cursor(repo, executable, endpoint string) error {
	if err := mcp.ValidateEndpoint(endpoint); err != nil {
		return err
	}
	root, err := os.OpenRoot(repo)
	if err != nil {
		return err
	}
	defer root.Close()
	raw, err := readOptional(root, ".cursor/mcp.json")
	if err != nil {
		return err
	}
	hooks, err := readOptional(root, ".cursor/hooks.json")
	if err != nil {
		return err
	}
	next, err := configureCursorServer(raw, executable, repo, endpoint)
	if err != nil {
		return err
	}
	command := cursorHookCommand(runtime.GOOS, executable, endpoint)
	nextHooks, err := configureCursorHooks(hooks, command)
	if err != nil {
		return err
	}
	if err := root.MkdirAll(".cursor", 0700); err != nil {
		return err
	}
	if !bytes.Equal(raw, next) {
		if err := atomicWrite(root, ".cursor/mcp.json", next); err != nil {
			return err
		}
	}
	if !bytes.Equal(hooks, nextHooks) {
		if err := atomicWrite(root, ".cursor/hooks.json", nextHooks); err != nil {
			return err
		}
	}
	return nil
}

func configureCursorServer(raw []byte, executable, repo, endpoint string) ([]byte, error) {
	document := map[string]any{}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &document); err != nil || document == nil {
			return nil, fmt.Errorf("invalid existing .cursor/mcp.json")
		}
	}
	servers, ok := document["mcpServers"].(map[string]any)
	if !ok {
		if _, exists := document["mcpServers"]; exists {
			return nil, fmt.Errorf("Cursor mcpServers must be an object")
		}
		servers = map[string]any{}
		document["mcpServers"] = servers
	}
	args := []any{"--mcp", "--repo", repo, "--daemon", endpoint}
	if existing, exists := servers["stef"]; exists {
		server, ok := existing.(map[string]any)
		if !ok || server["command"] != executable || !reflect.DeepEqual(server["args"], args) || (server["type"] != nil && server["type"] != "stdio") {
			return nil, fmt.Errorf("existing stef MCP configuration differs; review .cursor/mcp.json")
		}
		return raw, nil
	}
	servers["stef"] = map[string]any{"type": "stdio", "command": executable, "args": args}
	data, err := json.MarshalIndent(document, "", "  ")
	return append(data, byte(10)), err
}

// Cursor runs desktop hooks through PowerShell on Windows, including quoted paths.
func cursorHookCommand(platform, executable, endpoint string) string {
	if platform == "windows" {
		quote := func(value string) string { return "'" + strings.ReplaceAll(value, "'", "''") + "'" }
		return "& " + quote(executable) + " --hook --client cursor --daemon " + quote(endpoint)
	}
	return shellQuote(executable) + " --hook --client cursor --daemon " + shellQuote(endpoint)
}

func configureCursorHooks(raw []byte, command string) ([]byte, error) {
	document := map[string]any{}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &document); err != nil || document == nil {
			return nil, fmt.Errorf("invalid existing .cursor/hooks.json")
		}
	}
	changed := false
	if version, exists := document["version"]; exists {
		if version != float64(1) {
			return nil, fmt.Errorf("unsupported Cursor hooks version; review .cursor/hooks.json")
		}
	} else {
		document["version"] = 1
		changed = true
	}
	groups, ok := document["hooks"].(map[string]any)
	if !ok {
		if _, exists := document["hooks"]; exists {
			return nil, fmt.Errorf("Cursor hooks must be an object")
		}
		groups = map[string]any{}
		document["hooks"] = groups
	}
	// Cursor permission hooks block on empty/invalid output even with failClosed=false.
	// stef intentionally returns no stdout, so install only observational callbacks.
	for _, event := range []string{"sessionStart", "sessionEnd", "postToolUse", "postToolUseFailure", "stop"} {
		var entries []any
		if value, exists := groups[event]; exists {
			var ok bool
			entries, ok = value.([]any)
			if !ok {
				return nil, fmt.Errorf("invalid Cursor %s hooks", event)
			}
		}
		found := false
		for _, entry := range entries {
			handler, ok := entry.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("invalid Cursor %s hook entry", event)
			}
			current, _ := handler["command"].(string)
			if strings.Contains(current, "--hook") && strings.Contains(current, "--client cursor") && current != command {
				return nil, fmt.Errorf("existing stef Cursor hook differs; review .cursor/hooks.json")
			}
			if current == command {
				if kind, exists := handler["type"]; exists && kind != "command" {
					return nil, fmt.Errorf("stef Cursor hooks must use command execution")
				}
				if matcher, exists := handler["matcher"]; exists {
					if _, ok := matcher.(string); !ok {
						return nil, fmt.Errorf("invalid stef Cursor hook matcher")
					}
				}
				matcher, _ := handler["matcher"].(string)
				if matcher == "" || matcher == "*" {
					found = true
				}
			}
		}
		if !found {
			groups[event] = append(entries, map[string]any{"command": command, "timeout": 3, "failClosed": false})
			changed = true
		}
	}
	if !changed {
		return raw, nil
	}
	data, err := json.MarshalIndent(document, "", "  ")
	return append(data, byte(10)), err
}
