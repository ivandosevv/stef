package setup

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"

	"github.com/ivandosevv/stef/daemon/mcp"
)

func Claude(repo, executable, endpoint string) error {
	return claudeForPlatform(repo, executable, endpoint, runtime.GOOS)
}

func claudeForPlatform(repo, executable, endpoint, platform string) error {
	if err := mcp.ValidateEndpoint(endpoint); err != nil {
		return err
	}
	repo, err := filepath.Abs(repo)
	if err != nil {
		return err
	}
	root, err := os.OpenRoot(repo)
	if err != nil {
		return err
	}
	defer root.Close()
	skill, err := prepareClaudeSkill(root, repo, executable, endpoint)
	if err != nil {
		return err
	}
	raw, err := readOptional(root, ".mcp.json")
	if err != nil {
		return err
	}
	var settings, nextSettings []byte
	if platform != "windows" {
		settings, err = readOptional(root, ".claude/settings.local.json")
		if err != nil {
			return err
		}
	}
	next, err := configureClaudeServer(raw, executable, repo, endpoint)
	if err != nil {
		return err
	}
	// Git Bash and PowerShell use different command forms. MCP reporting works
	// without either shell or an unverified version-specific hook handler.
	if platform != "windows" {
		nextSettings, err = configureHookEvents(settings, shellQuote(executable)+" --hook --client claude --daemon "+shellQuote(endpoint), []string{"SessionStart", "PreToolUse", "PostToolUse", "PostToolUseFailure", "Stop"}, false)
		if err != nil {
			return err
		}
	}
	if err := root.MkdirAll(".claude", 0700); err != nil {
		return err
	}
	if skill != nil {
		if err := root.MkdirAll(filepath.Dir(ClaudeSkillPath), 0700); err != nil {
			return err
		}
		if err := atomicWrite(root, ClaudeSkillPath, skill); err != nil {
			return err
		}
	}
	if !bytes.Equal(raw, next) {
		if err := atomicWrite(root, ".mcp.json", next); err != nil {
			return err
		}
	}
	if !bytes.Equal(settings, nextSettings) {
		if err := atomicWrite(root, ".claude/settings.local.json", nextSettings); err != nil {
			return err
		}
	}
	return nil
}

func configureClaudeServer(raw []byte, executable, repo, endpoint string) ([]byte, error) {
	document := map[string]any{}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &document); err != nil || document == nil {
			return nil, fmt.Errorf("invalid existing .mcp.json")
		}
	}
	servers, ok := document["mcpServers"].(map[string]any)
	if !ok {
		if _, exists := document["mcpServers"]; exists {
			return nil, fmt.Errorf("mcpServers must be an object")
		}
		servers = map[string]any{}
		document["mcpServers"] = servers
	}
	args := []any{"--mcp", "--repo", repo, "--daemon", endpoint}
	if existing, exists := servers["stef"]; exists {
		server, ok := existing.(map[string]any)
		if !ok || server["command"] != executable || !reflect.DeepEqual(server["args"], args) {
			return nil, fmt.Errorf("existing stef MCP configuration differs; review .mcp.json")
		}
		return raw, nil
	}
	servers["stef"] = map[string]any{"type": "stdio", "command": executable, "args": args}
	data, err := json.MarshalIndent(document, "", "  ")
	return append(data, '\n'), err
}
