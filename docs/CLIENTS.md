# Agent and client compatibility

stef accompanies the agent independently of its editor. Local file tracking works
with any process editing the chosen root. MCP provides per-file explanations and
human-note check-ins when that client supports MCP. Rich tool-level observation
requires a documented lifecycle adapter; MCP cannot intercept other tool calls
or universally push a user message into a running turn.

| Client | Setup through --client | Verification boundary |
| --- | --- | --- |
| Any local coding agent | Local file observer; workflow through reported steps | Watcher tested; meaningful UI steps require MCP or supported hooks |
| Codex, including its IDE panels | codex: .codex/config.toml and passive hooks | Actual CLI startup and local bridge verified; automatic panel discovery pending |
| Claude Code, terminal or IDE | claude: .mcp.json and passive .claude hooks | Actual CLI tasks verified in all three update modes, with saved diffs, explanations and suggestions; IDE pane verification pending |
| Cursor local Agent | cursor: .cursor/mcp.json and passive .cursor/hooks.json | Project setup, lifecycle fixtures, and SDK transport tested; real Cursor task/pane pending |
| VS Code Copilot | vscode or copilot: .vscode/mcp.json | JSONC preservation and SDK transport tested; real Copilot task pending |
| Gemini CLI | gemini: .gemini/settings.json | Project configuration and SDK transport tested; MCP-only adapter, real task pending |
| Roo Code | roo: .roo/mcp.json | Project configuration and SDK transport tested; real task pending |
| OpenCode local agent | opencode or open-code: project opencode.json, or existing opencode.jsonc | JSON/JSONC preservation and SDK command transport tested; actual OpenCode task/pane pending |
| Devin Local / Desktop / CLI | devin: .devin/mcp_config.json | Current project format and SDK transport tested; real task pending |
| JetBrains AI Assistant in IntelliJ and supported JetBrains IDEs | jetbrains or intellij: project-scoped JSON import in AI Assistant settings | Export and SDK transport tested; actual import/task/pane pending |
| Cline, Windsurf / Cascade | cline or windsurf: generated JSON to merge in the client's MCP settings | Export and SDK transport tested; real tasks pending; Windsurf-specific current docs redirect to Devin |
| Hermes | hermes: generated mcp_servers mapping to merge into config.yaml | Export and SDK transport tested; actual import/task pending |
| Other MCP agents | generic: common stdio JSON to adapt/import | Transport tested; the host must support its own MCP schema |

Choose the actual agent rather than the outer editor. Codex running in Cursor
uses --client codex; Claude Code running in IntelliJ uses --client claude.
The cursor option is for Cursor's agent and vscode is for Copilot. JetBrains
targets AI Assistant's documented MCP settings; it does not certify every
agent in IntelliJ or Junie.

The current native archive targets macOS ARM64. Headless Linux and Windows amd64 source builds
compile with CGO disabled. Those binaries have not been run on their target OS;
compiling a target is distinct from verifying that target's editor/client. No public release asset or Homebrew tap is
available yet, so `brew install stef` is not a working public installation command.

Workflow histories are scoped to stable chat IDs. Codex/Claude lifecycle signals
select new/resumed chats when loaded; otherwise use MCP set_session once and
retain its ID in that conversation. Clear affects only that chat. MCP transport
connection IDs are independent of chat identity. An idle panel switch that the
client never reports cannot select a saved workflow automatically.

Additional native hosts are planned. Hosted or remote agent execution needs an
explicit event bridge and cannot be observed through local files alone.

## One-command source setup

With Git, Go 1.25+, Node 22.12+, npm, Python 3, and make already available, a checkout
can build and enable the selected agent with one command:

~~~sh
./scripts/bootstrap.sh --client opencode --repo /absolute/path/to/personal/project
~~~

For a new checkout, download the same published script before running it, or use
this single shell invocation after reviewing the source:

~~~sh
bash -c "$(curl -fsSL https://raw.githubusercontent.com/ivandosevv/stef/master/scripts/bootstrap.sh)" -- --client opencode --repo /absolute/path/to/personal/project
~~~

This is a source bootstrap, not a prebuilt installer. The default source lives in
`<project>/.stef/runtime/stef`, which the observer ignores. `--source-dir` can
instead reuse an explicitly chosen stef checkout; existing checkouts are never
pulled, reset, or overwritten. Builds remain in that checkout. No toolchain,
IDE extension, global client profile, or PATH entry is installed. macOS native
builds also require Xcode Command Line Tools. `--build-only` builds without
configuring or launching the companion. The eight isolated bootstrap tests use
fake download/build commands, so they verify orchestration and confinement rather
than a clean-machine installation with downloaded toolchains.

The default `--view ide` starts the headless engine and returns its local URL.
Opening that URL inside an editor still uses that editor's built-in browser.
Use `--view native` on a supported macOS host to open the companion. The chosen
MCP client must load/trust the new project entry, or import its generated entry,
using its own controls. The current running model cannot acquire tools merely
because a config file was saved. A terminal is a presentation environment; pick
its actual agent, such as `codex`, `claude`, `hermes`, or `opencode`.

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

The skill uses stef --enable --client <client> --repo <authorized-root> to
configure and launch from the current agent. stef --list-clients returns all
documented names, configuration paths, and remaining setup steps. It selects/reuses a loopback port
and starts a resident engine immediately; no separate agent CLI or stef VSIX is
needed. In VS Code, add --view ide and open the returned view_url in the existing
integrated browser. The skill uses a host browser tool if available, otherwise
supplies the URL; CLI startup alone does not open a tab. The presentation choice
is remembered independently of the MCP client. Codex loads newly configured MCP
through Restart extension; Claude Code
uses its /mcp approval/reconnect controls. Other clients follow next_step in
the setup result. Required project/hook trust remains.
See [GETTING_STARTED.md](GETTING_STARTED.md) for the personal VS Code walkthrough.

## Other hosts

~~~sh
stef --list-clients
stef --enable --client cursor --view ide --repo /absolute/path/to/project
stef --enable --client intellij --view ide --repo /absolute/path/to/project
stef --enable --client opencode --view ide --repo /absolute/path/to/project
~~~

Direct adapters extend the documented project file, preserve other servers and
settings, and reject conflicting stef entries. Codex/Claude/Cursor additionally
configure passive command hooks. Every client for the same root can reuse the
existing endpoint, including one originally prepared through an import export.

Import adapters write only .stef/clients/<client>.json in the selected project.
Their result has client_configured=false and supplies configuration_path plus
next_step. The observer is already active, but the client has not imported or
connected the server. Merge only the generated stef entry, preserving existing
client settings. No global user profile is changed by setup. Hermes uses the
snake-case mcp_servers key in YAML; merge the mapping rather than appending JSON
as a second YAML document. JetBrains accepts the common mcpServers JSON through
Settings > Tools > AI Assistant > Model Context Protocol, with server level
Project; no guessed .idea configuration is written.

For presentation, use a built-in browser capability that the actual editor
provides. workbench.action.browser.open is a VS Code command, not a universal
IDE API. Cursor and JetBrains pane mounting remains unverified. --view native
opens the macOS companion beside hosts without a built-in browser; --view ide
returns the ready URL and never installs an extension or claims a pane opened.

VS Code's .vscode/mcp.json is a supported compatibility format. VS Code forwards
eligible absolute stdio command/args to Copilot's harness. The guided portable
.mcp.json creation option has an experimental setting; existing portable files
are still discovered when that setting is off. Gemini and Roo setup enables
MCP only; their hook payloads are not treated as Claude/Cursor payloads. Devin
uses the current dedicated .devin/mcp_config.json, not older config*.json files.

OpenCode uses `mcp.stef.type = "local"` and a single `command` array containing
the executable and all arguments. Setup extends the existing `opencode.json` or
`opencode.jsonc` while preserving comments, unrelated servers, precise values,
and explicit `enabled: false`/environment policies. If both project files exist,
setup stops without changing either rather than guessing which file should own
stef. Project configuration and managed tool policy can still prevent discovery
or tool access; use `opencode mcp list` in the actual host to verify it.

Official references: [OpenCode MCP](https://opencode.ai/docs/mcp-servers/),
[OpenCode project config](https://opencode.ai/docs/config/), [Cursor](https://cursor.com/docs/mcp.md),
[Cursor hooks](https://cursor.com/docs/hooks.md),
[JetBrains AI Assistant](https://www.jetbrains.com/help/ai-assistant/mcp.html),
[VS Code formats](https://code.visualstudio.com/docs/agents/reference/mcp-configuration),
[Gemini](https://geminicli.com/docs/tools/mcp-server/),
[Roo](https://docs.roocode.com/features/mcp/using-mcp-in-roo),
[Cline](https://docs.cline.bot/mcp/configuring-mcp-servers),
[Hermes](https://hermes-agent.nousresearch.com/docs/user-guide/features/mcp/),
[Devin](https://docs.devin.ai/cli/extensibility/mcp/configuration).
Windsurf's former MCP documentation currently redirects to Devin; the legacy
Windsurf option remains an explicit import, without claimed project hooks.

## Cursor lifecycle hooks

Cursor's project command hooks report documented conversation and post-tool
signals without reading transcripts, responses, thoughts, prompts, or raw output.
Chat IDs derive from conversation_id, retaining the same ID across turns.
Tracking fails open and ignores stef's own MCP calls. Ambiguous multiroot
callbacks are skipped; explicit MCP reporting and root-local file observation
remain available. Hosted Cursor agents cannot be observed automatically through
this local loopback companion. No permission or model-evaluated hooks are added.

## Codex

Codex and Claude post-edit hooks now return documented PostToolUse
hookSpecificOutput.additionalContext with current pending IDs for concrete
edit paths, only in Every file mode. The read-only reminder endpoint waits up to
300 ms for ordinary watcher debounce and never consumes developer notes. Reads,
tests, shell commands, stef MCP calls, and other clients emit no reminder.
The agent supplies the actual reason through report_changes; hooks do not infer
intent. Generated PostToolUse handlers deliver context before the agent resumes
with a bounded local wait and fail-open errors. Rerun setup migrates only stef's
older async handler. Async Codex context reaches the next safe model request;
async Claude context may wait for the next user turn. Fixture/HTTP validation does not prove
that a real host loaded/trusted the hooks or delivered their context.

An agent without loaded stef tools may use --report-changes --repo <root>
--daemon <endpoint> with the same JSON on stdin. This calls the official SDK's
Streamable HTTP MCP client after validating the existing engine's root/version.
It makes no edits and does not start another agent or companion.

Hook references: [Codex](https://developers.openai.com/codex/hooks/) and
[Claude Code](https://code.claude.com/docs/en/hooks).

This checkout is configured through its local .codex/config.toml and hooks.json.
Start a new Codex session with this personal folder as its root. stef uses 4825
for this root; the standalone example sandbox can remain on 4820.

For an explicitly chosen personal root, run:

~~~sh
/absolute/path/to/stef/bin/stef --setup-codex --repo /absolute/path/to/your/personal/project --daemon http://127.0.0.1:4826
~~~

Official OpenAI documentation confirms Codex CLI and IDE share MCP configuration,
and `.codex/config.toml` applies only to trusted projects. Cloud orchestration
can limit local MCP reachability and does not support hooks from local command
configuration. Verify those environments separately; the local observer cannot
see remote files without an explicit bridge.

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

Start Claude in the configured project folder. The bundled `/stef` skill and
project `.mcp.json` belong to that folder. Use `claude mcp get stef` there to
check discovery; Pending approval means Claude needs its own `/mcp` approval.
An existing conversation in another folder does not load this project's setup.

Claude Code 2.1.223 completed real isolated fixes in Every file, Major milestones,
and Disabled. Every file supplied the exact revision's explanation before running
tests. Milestones reported the validated outcome without per-file reports. Disabled
made one initial mode check, then edited and tested without recording progress.
Developer suggestions reached Claude through MCP. Both designs displayed the saved
Claude revision and explanation without requesting more context.

Run `make test-claude` with existing authentication in the environment. If your
API setup lives in Claude settings, run
`python3 scripts/smoke-claude.py --auth-settings /absolute/path/to/settings.json`.
This reads only authentication and model environment fields into the test process,
without loading other user hooks, tools, or MCP servers. It never saves credentials.
The test uses real agent tokens and a temporary personal fixture. `--startup-only`
stops after MCP and hook discovery; `--keep-view` retains the isolated companion
for one hour. Changing the update mode preserves older steps under Show saved history.

examples/claude-hooks.json keeps lifecycle commands asynchronous, with a bounded
synchronous PostToolUse handler for prompt edit reminders. Replace the executable
path and endpoint for your chosen project. The --hook process reads stdin,
returns standard additionalContext only for eligible pending edits, and fails
open if tracking is unavailable. It strips prompts,
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
It is not required to run stef; the current workflow and suggestions live in
the same renderer across IDE-browser and native presentations.
