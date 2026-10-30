# Agent and client compatibility

stef accompanies the agent independently of its editor. Local file tracking works
with any process editing the chosen root. MCP provides per-file explanations and
human-note check-ins when that client supports MCP. Rich tool-level observation
requires a documented lifecycle adapter; MCP cannot intercept other tool calls
or universally push a user message into a running turn.

| Client | Available path | Verification boundary |
| --- | --- | --- |
| Any local coding agent | Local file observer; workflow through reported steps | Watcher tested; meaningful UI steps require MCP or supported hooks |
| Claude Code | Agent skill, automatic project setup, MCP and lifecycle hooks | Configuration preservation and hook fixtures tested; real Claude session pending |
| Codex / VS Code panel | Built-in browser presentation, agent skill, MCP autostart and passive lifecycle adapter | IDE-mode resident startup and remembered presentation tested; direct panel tool/hook loading and automatic tab opening pending |
| Cursor / VS Code | MCP where the selected agent supports it | Native companion requires no editor extension; real agent sessions pending |
| IntelliJ | Companion beside IDE, MCP where selected agent supports it | No JetBrains plugin required; real agent session pending |
| Hermes / other clients | Standard MCP and neutral lifecycle endpoint | Client capabilities and real sessions must be verified individually |

The current native binary runs on macOS ARM64. --headless supports other platforms.

Workflow histories are scoped to stable chat IDs. Codex/Claude lifecycle signals
select new/resumed chats when loaded; otherwise use MCP set_session once and
retain its ID in that conversation. Clear affects only that chat. MCP transport
connection IDs are independent of chat identity. An idle panel switch that the
client never reports cannot select a saved workflow automatically.

Additional native hosts are planned. Hosted or remote agent execution needs an
explicit event bridge and cannot be observed through local files alone.

## Common stdio configuration

~~~json
{
  "mcpServers": {
    "stef": {
      "command": "/absolute/path/to/stef/bin/stef",
      "args": ["--mcp", "--repo", "/absolute/path/to/your/personal/project"]
    }
  }
}
~~~

The bridge automatically starts or reuses a root-matched companion. Add --daemon
http://127.0.0.1:4825 for a second project. If the companion is launched separately,
use the same root and --port 4825. No global configurations are changed automatically.

## Agent-chat setup

In this checkout, .agents/skills/stef is discoverable by Codex and its VS Code
panel; .claude/skills/stef points to the same portable skill. Ask **Enable stef for
this project**, **$stef enable** in Codex, or **/stef enable** in Claude Code.

The skill uses stef --enable --client codex|claude --repo <authorized-root> to
configure and launch from the current agent. It selects/reuses a loopback port
and starts a resident engine immediately; no separate agent CLI or stef VSIX is
needed. In VS Code, add --view ide and open the returned view_url in the existing
integrated browser. The skill uses a host browser tool if available, otherwise
supplies the URL; CLI startup alone does not open a tab. The presentation choice
is remembered independently of the MCP client. Codex loads newly configured MCP
through Restart extension; Claude Code
uses its /mcp approval/reconnect controls. Required project/hook trust remains.
See [GETTING_STARTED.md](GETTING_STARTED.md) for the personal VS Code walkthrough.

## Codex

This checkout is configured through its local .codex/config.toml and hooks.json.
Start a new Codex session with this personal folder as its root. stef uses 4825
for this root; the standalone example sandbox can remain on 4820.

For an explicitly chosen personal root, run:

~~~sh
/absolute/path/to/stef/bin/stef --setup-codex --repo /absolute/path/to/your/personal/project --daemon http://127.0.0.1:4826
~~~

Codex requires a trusted project before loading its local config. Review the exact
passive hooks once with **/hooks** in the Codex CLI; panel hook-trust controls have
not been verified here. Changed hook definitions require review again.
Normal setup does not bypass hook trust or change global settings. The opt-in
isolated test vets its generated hooks and bypasses trust only for that invocation.

The adapter follows current official Codex hooks: Bash/apply_patch tool coverage,
PreToolUse and PostToolUse, SessionStart/UserPromptSubmit, and Stop. It reports
read/edit/test-command intent and observed completion/failure, without persisting
prompts, commands, output, or transcripts. stef's own MCP calls are excluded.
Shell-based arbitrary edits are still detected by the filesystem observer even
when their command cannot be classified. Hosted tools are outside local hook coverage.

Configuration and trust references:
https://developers.openai.com/codex/mcp/
https://developers.openai.com/codex/hooks/

Actual CLI 0.165.0 startup/shutdown was verified using project config in an isolated
fixture, without loading user MCP settings. A full real task was attempted, but
CLI authentication returned HTTP 401 before model execution. That full test
remains pending. The standalone CLI credentials must work before make test-codex
can verify a real fix, milestones, and note receipt. SDK transport/note tests pass.

For HTTP, first launch the companion and configure url =
"http://127.0.0.1:4825/mcp" instead of command/args. HTTP cannot launch a local
process. Startup in an already-open IDE conversation is not retroactive; start a
new session in the configured personal root. Other roots need their own setup.

## Claude lifecycle hooks

Reference: https://code.claude.com/docs/en/hooks

examples/claude-hooks.json uses asynchronous command hooks for SessionStart,
PreToolUse, PostToolUse, PostToolUseFailure, and Stop. Replace the executable
path and endpoint for your chosen project. The --hook process reads stdin,
produces no stdout, and fails open if tracking is unavailable. It strips prompts,
raw commands, and output. A different client's hook payload must be adapted
explicitly; use examples/generic-activity.json for the neutral contract.

## Human participation

Use a stable agent_id. Read update_mode at task start and follow the compact
reporting_instructions in receipts. milestones uses only major report_event
updates with short reasons; disabled stops new reporting/check-ins for the turn.
User mode changes reach an already-working agent at its next check-in, not by
interrupting the client's generation. No polling or measured token savings are implied.
In files mode, after each edit, get_state with settle_ms=300 returns
pending file IDs; explain every file you changed in one report_changes batch.
Keep each reason brief and tied to that revision. Missing reasons remain visible
and a later edit needs a new reason. report_event handles reads/tests/plans. Developer
notes target these reporter IDs. A hook's session identity may differ from the
reporter's chosen ID; “All reporting agents” avoids that mismatch. Without a real
MCP check-in the note stays in stef history. Notes are offered once per agent
per engine lifetime, and can be offered again after restart. Agents decide how to
respond within their own client's existing instructions and user authorization.

The earlier integrations/vscode wrapper is an optional presentation experiment.
It is not required to run stef; the new Chip in flow currently lives in the native
companion and local preview. No editor extension has been installed globally.
