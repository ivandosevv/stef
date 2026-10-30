# stef

stef is a quiet, interactive workflow beside your coding agent. It shows the
steps the agent takes, arrows between them, and a brief reason for each file or
change. Steps show only a compact label. Select one to reveal its brief reason
and file; choose Show diff for that saved revision or Suggest to open the input.

The interface keeps the stef name, the workflow, Clear, and small Updates and
appearance controls, plus a Design choice. Selected file changes open a Git-style diff on demand with
added/removed lines and the AI explanation. The workflow stays compact.

Updates offers Major milestones with brief explanations, Every file with a
reason for each saved change, and Disabled to stop new recording and reporting.
The Every file option notes that its explanations may use more agent tokens.
The choice is saved for the project across chats/restarts and shared by its views.
Existing history remains available; edits made while capture is off are not replayed.
Each Updates change begins a new reporting window. Every file requests reasons
only for new saves from that point. Earlier revisions remain behind Show saved
history and never become an automatic explanation backlog after re-enabling.

Design offers Minimal and Interactive. Minimal keeps the current quiet workflow.
Interactive adds a selected-change panel with exact before/after hunks, navigation,
and reported milestone actions, states, and paths. Switching design, clicking
steps, or inspecting history makes no model call. Explain more alone queues an
explicit request for fuller AI context, with a token warning, answered at the
agent's next check-in. The answer updates the same step and preserves its diff.

The appearance button beside Clear offers Dark, Light, Eye comfort, and System.
Eye comfort uses a warm background and muted green accents. Your choice is
saved in this view across reloads and chats; System follows the OS appearance.

This is a personal project outside SAPDevelop, with no SAP or Hyperspace integration.

## Run in the existing VS Code browser tab

The working macOS ARM64 prototype embeds its renderer in one Go executable.
Build prerequisites are Go 1.25+, Node 22.12+ or a supported newer release, and
Apple's command-line build tools. End users need no Node server or stef extension.

~~~sh
cd /Users/I751316/PersonalProjects/stef
make install
make build
./bin/stef --enable --client codex --view ide --repo /Users/I751316/PersonalProjects/stef
~~~

Open the returned view_url in VS Code's existing integrated browser editor.
For this configured checkout it is http://127.0.0.1:4825/?companion=1.
An agent with a host browser tool can open it there; otherwise paste the URL in
that tab's address bar. The engine saves IDE mode so subsequent MCP starts do
not open a native window. Use --view native for a separate macOS companion.

For isolated development, use examples/sandbox or a directory under .test-workspace.
Never add SAPDevelop or Hyperspace to this project's observer.

## Enable from chat

The checkout includes a stef skill in .agents/skills/stef, shared with Claude
Code through .claude/skills/stef. Ask **Enable stef for this project**, or use
**$stef enable** in Codex / **/stef enable** in Claude Code.

The skill configures only the chosen root, starts the local engine, and uses the
current host's available browser and MCP controls. Codex loads new MCP settings
through **Restart extension**. Project hook trust is documented through **/hooks**
in the Codex CLI; panel hook-trust controls remain unverified. Claude Code uses
**/mcp** to approve/reconnect its project server. See [Getting started](docs/GETTING_STARTED.md).

Public Homebrew/plugin distribution is not published. An agent first needs this
source checkout, the local release package, or another known executable location.
The earlier local release archive predates the minimal workflow increment.

## One workflow per chat

A new chat ID starts with an empty workflow. Activating an existing ID restores
its steps and arrows; this also works after engine restart. Clear starts an
empty segment for that chat and leaves other chats intact. The selected step is
remembered per chat in the view.

Codex/Claude start and resume hooks provide a stable, hashed chat ID without a
model call. They must actually be loaded and trusted by the host. MCP clients
without those signals can call set_session once for a new/resumed chat, retain
that ID in the conversation, and include session_id in their milestones. Do not
create a new ID per turn. stef cannot detect an idle chat-tab switch that the
host never reports. This existing VS Code conversation still uses a local MCP
bridge; automatic panel hook discovery has not been certified.

In Every file mode, each saved text-file revision appears automatically with its filename, including
helpers, entry points, tests, and docs. Repeated saves remain separate changes.
Selecting an unexplained change says the agent's reason is still missing. Exact
change IDs attach reasons in place; a later edit never inherits an older reason.
Show diff displays that observation's unified patch with old/new line numbers,
green additions, red removals, and its current agent explanation. Opening it makes
no model call. It compares the captured save against the preceding observed
version, rather than Git HEAD or the current file. Existing 16 KiB previews can
be truncated; the view labels those explicitly instead of implying a complete diff.
Solid arrows are connections supplied through parents. Dashed arrows show step
order without claiming code dependencies. Symbol observations remain in storage
without creating duplicate file steps.

Passive hooks and file watching make no model calls. MCP initialization and the
bundled skill tell connected agents to follow the selected Updates mode. In Every
file mode, check pending changes after each edit and
explain every file using one report_changes batch. get_state with settle_ms=300
allows the watcher debounce; pending IDs/paths are returned without code/diffs.
Before finishing, agents check for omitted explanations once. report_event still
handles reads, tests, or plans. No second model is used; token cost is unmeasured.
An agent must follow the reporting contract: filesystem monitoring cannot
discover the author's reason or force an unconnected host to provide one.
Major milestones requests only new key steps with reasons and suppresses new file
capture. Disabled rejects new activity and returns no pending files or feedback
to the model. Changes reach a working agent in its next natural MCP check-in;
stef does not poll the model or interrupt its current generation.

## Checks

~~~sh
make check
make test
cd ui && npm run test:e2e
~~~

The default tests use isolated personal fixtures. They cover chat persistence,
resume, clearing, scoped feedback, stdio/HTTP MCP, arrows, and the minimal UI.
See [Validation](docs/VALIDATION.md) and [Architecture](docs/ARCHITECTURE.md).
The optional VS Code extension experiment is outside the default product path.
