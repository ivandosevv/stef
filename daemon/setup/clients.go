package setup

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ivandosevv/stef/daemon/mcp"
)

// Client describes a documented configuration path, not a certified agent session.
type Client struct {
	Name       string `json:"name"`
	Label      string `json:"label"`
	SetupMode  string `json:"setup_mode"`
	ConfigPath string `json:"config_path"`
	HooksPath  string `json:"hooks_path,omitempty"`
	ServerKey  string `json:"server_key"`
	NextStep   string `json:"next_step"`
	DocsURL    string `json:"docs_url"`
}

type Configuration struct {
	Client     string `json:"client"`
	ConfigPath string `json:"config_path"`
	HooksPath  string `json:"hooks_path,omitempty"`
	Configured bool   `json:"configured"`
	NextStep   string `json:"next_step"`
}

var clientCatalog = []Client{
	{Name: "codex", Label: "Codex", SetupMode: "project", ConfigPath: ".codex/config.toml", HooksPath: ".codex/hooks.json", ServerKey: "mcp_servers", NextStep: "Restart the Codex extension once to load MCP; hook trust is reviewed with /hooks in the Codex CLI.", DocsURL: "https://developers.openai.com/codex/mcp/"},
	{Name: "claude", Label: "Claude Code", SetupMode: "project", ConfigPath: ".mcp.json", HooksPath: ".claude/settings.local.json", ServerKey: "mcpServers", NextStep: "Use Claude Code's /mcp menu to approve/reconnect the project server; restart the session if needed.", DocsURL: "https://code.claude.com/docs/en/mcp"},
	{Name: "cursor", Label: "Cursor", SetupMode: "project", ConfigPath: ".cursor/mcp.json", HooksPath: ".cursor/hooks.json", ServerKey: "mcpServers", NextStep: "Open Cursor's Customize > Plugins & MCPs, approve/reconnect stef, and restart Cursor if the project server does not reload.", DocsURL: "https://cursor.com/docs/mcp.md"},
	{Name: "vscode", Label: "VS Code Copilot", SetupMode: "project", ConfigPath: ".vscode/mcp.json", ServerKey: "servers", NextStep: "Run VS Code's MCP: List Servers, start/trust stef, and enable its tools. VS Code forwards eligible entries to Copilot; this workspace format remains available for compatibility.", DocsURL: "https://code.visualstudio.com/docs/agents/reference/mcp-configuration"},
	{Name: "gemini", Label: "Gemini CLI", SetupMode: "project", ConfigPath: ".gemini/settings.json", ServerKey: "mcpServers", NextStep: "Start Gemini CLI from this project, review project trust, and use /mcp status or gemini mcp list to verify stef.", DocsURL: "https://geminicli.com/docs/tools/mcp-server/"},
	{Name: "roo", Label: "Roo Code", SetupMode: "project", ConfigPath: ".roo/mcp.json", ServerKey: "mcpServers", NextStep: "Open Roo Code's MCP Servers view and approve/reconnect stef from the project configuration.", DocsURL: "https://docs.roocode.com/features/mcp/using-mcp-in-roo"},
	{Name: "devin", Label: "Devin Local", SetupMode: "project", ConfigPath: ".devin/mcp_config.json", ServerKey: "mcpServers", NextStep: "Open Devin Local's MCP configuration and reconnect stef from this project's .devin/mcp_config.json. Start a new session if needed.", DocsURL: "https://docs.devin.ai/cli/extensibility/mcp/configuration"},
	{Name: "jetbrains", Label: "JetBrains AI Assistant", SetupMode: "import", ConfigPath: ".stef/clients/jetbrains.json", ServerKey: "mcpServers", NextStep: "In Settings > Tools > AI Assistant > Model Context Protocol (MCP), add a STDIO server, select Project level, paste the generated JSON, and apply/reconnect.", DocsURL: "https://www.jetbrains.com/help/ai-assistant/mcp.html"},
	{Name: "windsurf", Label: "Windsurf / Cascade", SetupMode: "import", ConfigPath: ".stef/clients/windsurf.json", ServerKey: "mcpServers", NextStep: "Open your Windsurf/Cascade agent's MCP configuration, merge the generated mcpServers.stef entry, and reconnect. The generated file has not been imported into the client.", DocsURL: "https://code.visualstudio.com/docs/agents/reference/mcp-configuration"},
	{Name: "cline", Label: "Cline", SetupMode: "import", ConfigPath: ".stef/clients/cline.json", ServerKey: "mcpServers", NextStep: "In Cline's MCP Servers > Configure > Configure MCP Servers, merge the generated mcpServers.stef entry and reconnect. The generated file has not been imported into the client.", DocsURL: "https://docs.cline.bot/mcp/configuring-mcp-servers"},
	{Name: "hermes", Label: "Hermes", SetupMode: "import", ConfigPath: ".stef/clients/hermes.json", ServerKey: "mcp_servers", NextStep: "Merge the generated mcp_servers.stef object into the existing mcp_servers mapping in Hermes config.yaml, preserving your other settings, then use /reload-mcp or start a new session. The generated file has not been imported.", DocsURL: "https://hermes-agent.nousresearch.com/docs/user-guide/features/mcp/"},
	{Name: "generic", Label: "Other MCP clients", SetupMode: "import", ConfigPath: ".stef/clients/generic.json", ServerKey: "mcpServers", NextStep: "Import or merge the generated stdio configuration in your agent's MCP settings, adapting its top-level key if required, and reconnect. The generated file has not been imported into the client.", DocsURL: "https://modelcontextprotocol.io/docs/develop/connect-local-servers"},
}

func Clients() []Client { return append([]Client(nil), clientCatalog...) }

func LookupClient(name string) (Client, error) {
	name = strings.ToLower(strings.TrimSpace(name))
	aliases := map[string]string{"claude-code": "claude", "intellij": "jetbrains", "intellij-idea": "jetbrains", "pycharm": "jetbrains", "webstorm": "jetbrains", "goland": "jetbrains", "rider": "jetbrains", "copilot": "vscode", "vs-code": "vscode", "gemini-cli": "gemini", "roo-code": "roo", "devin-local": "devin", "devin-desktop": "devin"}
	if canonical, ok := aliases[name]; ok {
		name = canonical
	}
	for _, client := range clientCatalog {
		if client.Name == name {
			return client, nil
		}
	}
	return Client{}, fmt.Errorf("unknown client %q; use --list-clients for available setup paths", name)
}

// Configure writes only inside repo. Import paths export a file rather than changing user profiles.
func Configure(repo, executable, endpoint, name string) (Configuration, error) {
	client, err := LookupClient(name)
	if err != nil {
		return Configuration{}, err
	}
	if err := mcp.ValidateEndpoint(endpoint); err != nil {
		return Configuration{}, err
	}
	if !filepath.IsAbs(executable) {
		return Configuration{}, fmt.Errorf("stef executable must be an absolute path")
	}
	result := Configuration{Client: client.Name, ConfigPath: client.ConfigPath, HooksPath: client.HooksPath, Configured: client.SetupMode == "project", NextStep: client.NextStep}
	switch client.Name {
	case "codex":
		err = Codex(repo, executable, endpoint)
	case "claude":
		err = Claude(repo, executable, endpoint)
	case "cursor":
		err = Cursor(repo, executable, endpoint)
	default:
		err = configureJSONClient(repo, executable, endpoint, client)
	}
	if err != nil {
		return Configuration{}, err
	}
	return result, nil
}

func configureJSONClient(repo, executable, endpoint string, client Client) error {
	root, err := os.OpenRoot(repo)
	if err != nil {
		return err
	}
	defer root.Close()
	raw, err := readOptional(root, client.ConfigPath)
	if err != nil {
		return err
	}
	server := map[string]any{"command": executable, "args": []any{"--mcp", "--repo", repo, "--daemon", endpoint}}
	if client.Name == "vscode" {
		server["type"] = "stdio"
	}
	if client.Name == "cline" {
		server["disabled"] = false
		server["autoApprove"] = []any{}
	}
	next, err := extendJSONClient(raw, client, server)
	if err != nil {
		return err
	}
	if bytes.Equal(raw, next) {
		return nil
	}
	if err := root.MkdirAll(filepath.Dir(client.ConfigPath), 0700); err != nil {
		return err
	}
	return atomicWrite(root, client.ConfigPath, next)
}

func extendJSONClient(raw []byte, client Client, desired map[string]any) ([]byte, error) {
	document, normalized, err := decodeJSONClient(raw, client.Name == "vscode")
	if err != nil {
		return nil, fmt.Errorf("invalid existing %s: %w", client.ConfigPath, err)
	}
	servers, ok := document[client.ServerKey].(map[string]any)
	if !ok {
		if _, exists := document[client.ServerKey]; exists {
			return nil, fmt.Errorf("%s in %s must be an object", client.ServerKey, client.ConfigPath)
		}
		servers = map[string]any{}
	}
	if value, exists := servers["stef"]; exists {
		current, ok := value.(map[string]any)
		currentArgs, _ := json.Marshal(current["args"])
		desiredArgs, _ := json.Marshal(desired["args"])
		if !ok || current["command"] != desired["command"] || !bytes.Equal(currentArgs, desiredArgs) || (current["type"] != nil && current["type"] != "stdio") || (client.Name == "vscode" && current["type"] != "stdio") || current["url"] != nil {
			return nil, fmt.Errorf("existing stef MCP configuration differs; review %s", client.ConfigPath)
		}
		return raw, nil // Preserve explicit policies, including disabled=true and tool restrictions.
	}
	if len(raw) > 0 && client.Name == "vscode" {
		return appendJSONCServer(raw, normalized, client.ServerKey, servers, desired)
	}
	servers["stef"] = desired
	document[client.ServerKey] = servers
	data, err := json.MarshalIndent(document, "", "  ")
	return append(data, '\n'), err
}
