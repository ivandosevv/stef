package main

import (
	"path/filepath"
	"strings"

	clientsetup "github.com/ivandosevv/stef/daemon/setup"
)

// Configuration and a running observer do not prove that a host loaded MCP tools.
func enableResult(configuration clientsetup.Configuration, root, endpoint, view string) map[string]any {
	result := map[string]any{
		"observing": true, "client": configuration.Client, "repo": root,
		"endpoint": endpoint, "view": view,
		"view_url":           strings.TrimSuffix(endpoint, "/") + "/?companion=1",
		"configuration_path": filepath.Join(root, configuration.ConfigPath),
		"client_configured":  configuration.Configured, "mcp_connected": false,
		"next_step": configuration.NextStep,
	}
	if configuration.HooksPath != "" {
		result["hooks_path"] = filepath.Join(root, configuration.HooksPath)
	}
	if configuration.SkillPath != "" {
		result["skill_path"] = filepath.Join(root, configuration.SkillPath)
	}
	if view == "ide" {
		result["view_opened"] = false
		result["view_next_step"] = "Open view_url using this host's built-in browser if available; use --view native for a companion beside other hosts."
		if configuration.Client == "vscode" {
			result["open_command"] = "workbench.action.browser.open"
		}
	}
	return result
}
