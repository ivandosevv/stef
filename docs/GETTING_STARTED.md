# Enable stef from the agent in VS Code

Keep the existing Codex or Claude Code panel. stef can run in the same integrated
browser editor tab as any localhost preview, beside the agent. For the configured
personal checkout, put **http://127.0.0.1:4825/?companion=1** in that tab's address
bar. The view observes the personal stef root even when displayed in another
VS Code window; it does not add that window's workspace to the observer.

For a new agent session that discovers this checkout's skill and MCP settings,
use **stef.code-workspace** as the task workspace. Keep the watched project
outside SAPDevelop and honor all exclusions.

In the Codex panel, start a conversation and type:

> $stef enable for this project

In Claude Code, type **/stef enable**. Plain **Enable stef for this project** can
also match the skill automatically. The skill is included in this checkout;
if a newly discovered skill is missing from the selector, reload the agent.

The skill runs --enable --client codex --view ide --repo <authorized-root>
(--client claude for Claude Code), checks the root-matched health endpoint, and
opens view_url with a host browser tool when available. Otherwise it supplies
the URL for the existing tab's address bar. The CLI prepares the view and returns
view_opened=false; it does not pretend that starting the engine opens an IDE tab.
It writes only the chosen project's configuration and remembers the presentation
in .stef/presentation.json. Later MCP autostart respects IDE mode and does not
launch a native window. A loopback port is chosen/reused automatically, and the
resident engine outlives setup. No stef extension or additional app is required.

Use --view native for the separate macOS companion when desired. Closing an IDE
tab disconnects that view; the resident engine can keep observing.

The first setup can still require the client's controls:

- Codex: open the gear menu → MCP servers → Restart extension to load the newly
  added server. The official hook documentation describes /hooks in the Codex
  CLI for reviewing/trusting project hooks; hook trust from the VS Code panel
  has not been verified here. MCP milestones and file tracking do not require
  the passive hook adapter.
- Claude Code: approve the project MCP entry and reconnect through /mcp; restart
  the session if a new server or hook configuration has not loaded.

These steps belong to the agent client. The skill does not bypass them or claim
that changing files dynamically adds tools to the active conversation. Until
that connection is active, local file observation is available in stef. After
trusted hooks load, the companion can show read/edit/test/completion signals.
The Updates control offers Major milestones, Every file, or Disabled. Every file
warns that per-file explanations may use more tokens. The project's choice is
saved across chats/restarts, and the agent learns changes at its next check-in.
Disabled retains saved history and stops new recording/reporting.
Every mode change starts a new reporting window. Every file requests explanations
only for subsequent saves. Show saved history opens earlier work locally and
does not request missing historical explanations.

Use Design to choose Minimal or Interactive. Interactive shows exact before/after
code hunks and reported milestone paths beside the selected step. This visual
choice makes no AI call. Click Explain more only when you want extra agent context;
the token warning applies to that explicit question, delivered at the next check-in.

In Every file mode, each changed file needs a short agent-supplied reason. The MCP instructions/skill
ask the agent to get_state with settle_ms=300 after each edit, then report_changes
for all affected observation IDs in one batch. Pending files make omissions
visible. Read/test milestones and queued notes still use MCP check-ins.

Try a task in examples/sandbox from that VS Code agent. stef shows connected
workflow steps with filenames and short labels. Every saved text-file change
appears automatically, even before its explanation. Select one for the brief
reason. Choose Show diff to inspect that save's added and removed lines beside
the AI explanation, or Suggest to open the input. Your note is offered at the next
identified MCP check-in. Clear at the top resets this chat's workflow.

Start/resume hooks select a stable chat ID: new IDs start empty and existing IDs
restore the saved workflow, including after engine restart. When those hooks
are unavailable, the agent calls set_session once and retains the ID in the
conversation for later milestones/resume. This must be connected in the host;
stef cannot observe an idle chat-tab switch that produces no signal.
The selected step is remembered per chat. Diffs stay closed until requested,
use the recorded revision rather than the current file, and make no model calls.
Large captured previews show a truncation warning when incomplete. An explicitly
requested fuller explanation remains attached to its original step.
The small appearance button beside Clear offers Dark, Light, Eye comfort (warm
background), and System (follow the OS). Appearance is saved for this view;
switching chats or clearing a workflow leaves that choice intact.

The latest live checkout test added command-line greeting support while an
already-connected browser received start/result milestones and displayed three
steps with two arrows. A real user suggestion reached the agent through MCP and
the agent updated that step's explanation. This conversation uses
scripts/mcp-call.py because its tool catalog has not loaded stef directly.
See [VALIDATION.md](VALIDATION.md) for the exact boundary.
Official setup references: [Codex MCP](https://developers.openai.com/codex/mcp/)
and [Codex hooks](https://developers.openai.com/codex/hooks/).

## Outside this checkout

A normal user first connects the stef skill/MCP package or supplies its known
source/download location to their agent. After that, the chat request can handle
project setup and launch. The local release archive contains the executable and
portable skill; public Brew and plugin distribution are not published yet.

Codex and Claude Code have automatic setup adapters. Another client's MCP support
and configuration schema must be verified before claiming the same one-prompt
setup there. File observation remains independent of the chosen agent/editor.
