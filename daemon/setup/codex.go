// Package setup writes explicitly requested, project-local client configuration.
package setup

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"reflect"
	"strings"
	"time"

	"github.com/ivandosevv/stef/daemon/mcp"
	"github.com/pelletier/go-toml/v2"
)

type codexServer struct {
	Command        string   `toml:"command"`
	Args           []string `toml:"args"`
	Enabled        bool     `toml:"enabled"`
	StartupTimeout int      `toml:"startup_timeout_sec"`
}

func Codex(repo, executable, endpoint string) error {
	if err := mcp.ValidateEndpoint(endpoint); err != nil {
		return err
	}
	root, err := os.OpenRoot(repo)
	if err != nil {
		return err
	}
	defer root.Close()
	config, err := readOptional(root, ".codex/config.toml")
	if err != nil {
		return err
	}
	hooks, err := readOptional(root, ".codex/hooks.json")
	if err != nil {
		return err
	}
	nextConfig, err := configureServer(config, codexServer{Command: executable, Args: []string{"--mcp", "--repo", repo, "--daemon", endpoint}, Enabled: true, StartupTimeout: 20})
	if err != nil {
		return err
	}
	nextHooks, err := configureHooks(hooks, shellQuote(executable)+" --hook --client codex --daemon "+shellQuote(endpoint))
	if err != nil {
		return err
	}
	if err := root.MkdirAll(".codex", 0700); err != nil {
		return err
	}
	if !bytes.Equal(config, nextConfig) {
		if err := atomicWrite(root, ".codex/config.toml", nextConfig); err != nil {
			return err
		}
	}
	if !bytes.Equal(hooks, nextHooks) {
		if err := atomicWrite(root, ".codex/hooks.json", nextHooks); err != nil {
			return err
		}
	}
	return nil
}

func configureServer(raw []byte, server codexServer) ([]byte, error) {
	var existing map[string]any
	if err := toml.Unmarshal(raw, &existing); err != nil {
		return nil, fmt.Errorf("invalid existing Codex config: %w", err)
	}
	fields, err := toml.Marshal(server)
	if err != nil {
		return nil, err
	}
	var desired map[string]any
	_ = toml.Unmarshal(fields, &desired)
	if servers, ok := existing["mcp_servers"].(map[string]any); ok {
		if value, exists := servers["stef"]; exists {
			current, ok := value.(map[string]any)
			if !ok || !reflect.DeepEqual(current["command"], desired["command"]) || !reflect.DeepEqual(current["args"], desired["args"]) {
				return nil, fmt.Errorf("existing mcp_servers.stef differs; review .codex/config.toml before configuring this root and endpoint")
			}
			return raw, nil // Preserve explicit client/tool policies, including enabled=false.
		}
	}
	next := append([]byte(nil), raw...)
	if len(next) > 0 && next[len(next)-1] != '\n' {
		next = append(next, '\n')
	}
	next = append(next, []byte("\n# stef: local companion; no global client configuration changed.\n[mcp_servers.stef]\n")...)
	next = append(next, fields...)
	if err := toml.Unmarshal(next, &existing); err != nil {
		return nil, fmt.Errorf("cannot safely extend existing MCP configuration: %w", err)
	}
	return next, nil
}

func configureHooks(raw []byte, command string) ([]byte, error) {
	return configureHookEvents(raw, command, []string{"SessionStart", "UserPromptSubmit", "PreToolUse", "PostToolUse", "Stop"}, true)
}
func configureHookEvents(raw []byte, command string, events []string, syncStop bool) ([]byte, error) {
	document := map[string]any{}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &document); err != nil || document == nil {
			return nil, fmt.Errorf("invalid existing Codex hooks")
		}
	}
	groups, ok := document["hooks"].(map[string]any)
	if !ok {
		if _, exists := document["hooks"]; exists {
			return nil, fmt.Errorf("Codex hooks must be an object")
		}
		groups = map[string]any{}
		document["hooks"] = groups
	}
	changed := false
	for _, event := range events {
		var entries []any
		if value, exists := groups[event]; exists {
			var ok bool
			entries, ok = value.([]any)
			if !ok {
				return nil, fmt.Errorf("invalid %s hook groups", event)
			}
		}
		found := false
		for _, entry := range entries {
			group, ok := entry.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("invalid hook group")
			}
			if matcher, ok := group["matcher"].(string); ok && matcher != "" && matcher != "*" {
				continue
			}
			handlers, ok := group["hooks"].([]any)
			if !ok {
				return nil, fmt.Errorf("invalid hook handlers")
			}
			for _, handler := range handlers {
				if value, ok := handler.(map[string]any); ok && value["type"] == "command" && value["command"] == command {
					found = true
				}
			}
		}
		if !found {
			groups[event] = append(entries, map[string]any{"hooks": []any{map[string]any{"type": "command", "command": command, "async": !(syncStop && event == "Stop"), "timeout": 3}}})
			changed = true
		}
	}
	if !changed {
		return raw, nil
	}
	encoded, err := json.MarshalIndent(document, "", "  ")
	return append(encoded, '\n'), err
}

func shellQuote(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'" }
func readOptional(root *os.Root, name string) ([]byte, error) {
	file, err := root.Open(name)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, 256*1024+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > 256*1024 {
		return nil, fmt.Errorf("configuration exceeds 256 KiB")
	}
	return raw, nil
}
func atomicWrite(root *os.Root, name string, data []byte) error {
	temp := fmt.Sprintf("%s.stef-%d.tmp", name, time.Now().UnixNano())
	file, err := root.OpenFile(temp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer root.Remove(temp)
	_, err = file.Write(data)
	if err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return root.Rename(temp, name)
}
