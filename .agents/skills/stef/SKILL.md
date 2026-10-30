---
name: stef
description: Enable/connect stef and report coding progress at its chosen update level. Use when asked to enable stef or visualize a task, and for ongoing work while stef is connected.
---

Enable stef in the user's current coding client while keeping the agent and its
IDE in place. No stef editor extension or standalone agent session is needed.

Resolve the project root the user wants observed and honor their exclusions.
Locate the stef executable on PATH; in the stef source checkout use bin/stef at
the repository root. If it is missing there, build with make build (make install
first when dependencies are absent). Outside the checkout, use the user's known
release/source location; do not invent a public Brew tap or download URL.
The repository's source bootstrap is an explicit fallback with build prerequisites;
read docs/CLIENTS.md in that checkout for the current command and limits.

For the Codex panel in VS Code, use its built-in browser editor:

~~~sh
stef --enable --client codex --view ide --repo "$project_root"
~~~

Choose --client for the actual agent, independently of its outer IDE: Codex in
Cursor still uses codex; Claude Code in IntelliJ still uses claude. Run
stef --list-clients for documented setup paths and their remaining client steps.
Direct project setup is available for codex, claude, cursor, vscode (Copilot),
opencode, gemini, roo, and devin. intellij aliases jetbrains (AI Assistant); jetbrains,
windsurf, cline, hermes, and generic export a configuration for explicit import.
Use --view native for a terminal's native window or when explicitly requested.
Honor the actual editor's documented presentation capabilities and the user's
choice. Quote the executable and resolved root.
Enable selects/reuses a loopback port, preserves client settings, writes
project-local MCP/hooks or a root-confined import file, and starts a resident
engine. An import receipt has client_configured=false; show its configuration_path
and next_step so the user can import it through that client's controls. Never
write global profiles automatically or claim the host imported an export.
The project remembers
--view ide so later MCP autostart also runs without a native window.
A conflicting existing stef entry needs review; do not overwrite it.
Use the returned endpoint to verify /healthz matches the intended root.

For IDE view, first bind this worker with set_session as described below. Open
the verified endpoint plus its returned view_path in the current editor group's
built-in browser using the host's browser-control tool or command API when
available. The setup view_url is not scoped to this worker; do not use it for
the agent's private view. Complete the client's MCP trust/reload step first if
the tools have not loaded.
VS Code's command is workbench.action.browser.open with the URL as its argument.
Do not send this command to Cursor or JetBrains merely because their agent is
Codex or Claude; use a browser capability actually exposed by that host.
The CLI's view_opened=false means the engine is ready but the host has not opened
the view. If no browser-control capability is available, provide the localhost
link and tell the user to paste it into the existing browser tab's address bar.
Do not launch another application window or install an extension for this mode;
report that the tab opened only after actually opening or observing it.

Describe what is active now: local file observation. Codex loads new MCP settings
after Restart extension in its MCP settings; its project hooks require one-time
review through /hooks in the Codex CLI, as documented by OpenAI. Do not claim the
VS Code panel provides a verified hook-trust interface; MCP milestones and file
tracking can be used without hooks. For Claude Code, use /mcp to approve/reconnect its project
server, restarting the session if needed. Honor the client's trust controls.
Changing files does not itself load tools into an already-running conversation.

Other clients use the same stdio/HTTP MCP contract with their documented schema.
Follow the returned next_step for MCP trust, import, or reload. Hermes imports
mcp_servers into its YAML configuration; do not substitute mcpServers or append
a second document. JetBrains uses its MCP settings JSON at Project level rather
than a guessed .idea file. Configuration-tested adapters and SDK transport tests
do not prove that a real host session loaded tools, emitted hooks, or opened a
pane; describe the actual verification performed.

When enabling stef or starting/resuming a chat, call set_session once with the
originally chosen stable chat ID as session_id and this worker's unique agent_id. Retain
that pair for later set_session calls. Retain the returned worker session_id for
every other tool; do not pass the returned derived ID back with agent_id on resume.
If no chat ID is retained yet, create a unique one. A trusted lifecycle hook may supply the
worker identity. Bind its exact session_id with set_session, omitting agent_id
in that binding call; use the hook's reporter agent_id for later calls. Each
explicit set_session starts privately, including when resuming a previously
shared chat. Use resume_shared=true only when the user explicitly wants to
retain its previous shared workspace. Ordinary get_state and reports preserve
active sharing; do not call set_session again for every task or check-in.
The receipt supplies workspace_name and a worker-scoped view_path. Open that
path on the verified endpoint after binding, then call get_state once at task
start and follow update_mode and reporting_instructions in receipts.
The Updates control belongs to the selected workspace and persists on restart.
Names such as Quiet Fox 2487 come from local word lists and numbers, remain
stable across resume and Clear, and require no AI. Keep IDs for routing and
describe workspaces to the user by name.

Each agent and subagent starts with its own workspace. Give every subagent its
own unique agent_id and tell it to set_session before reporting. MCP cannot
identify anonymous workers using the same transport, so do not pass the parent's
worker session_id as their default. Retain each chat/reporter pair on resume.

Share only when requested or explicitly agreed for cooperating workers. Call
set_workspace with your returned session_id, agent_id, and action=create. Give
its returned name to the chosen agents and ask each to call action=join with
that name in workspace_id; the invitation ID is also accepted. Keep using each worker's returned
session_id for reports and check-ins; workspace_id identifies the shared
view, not a replacement chat ID. action=leave restores that worker's private
history without copying others' work. A new shared workspace starts empty.
Shared get_state returns bounded recorded explanations with their authors; read
it at natural coordination points. Sharing does not expose hidden reasoning,
merge model conversations, or start extra model calls. Never poll.
Updates changes start a new reporting window, identified by updates_from.
Only new work enters automatic reporting. Do not request or backfill reasons
for saved history when the user changes the mode or opens older steps.

In milestones mode, report only major steps with brief reasons through
report_event. Use the real action, running/ready state, and reported parents
to support its local visualization. Skip file explanations and per-edit check-ins.

In files mode, saved revisions are attributed using concrete edit paths.
Prepare each file's brief reason while preparing the edit. After each edit tool, use
the exact pending IDs from a trusted post-edit reminder, or call get_state once
with settle_ms=300 and changed_paths containing only the relative paths from
your actual edit. This claims observed revisions for your workspace without
copying another private agent's saves. Immediately report_changes for
every file you changed, before another edit, test, or final reply. When tool
orchestration is available, sequence the successful edit, bounded check-in, and
exact report in one call without another model turn between them. Send one batch
with 2–4 word summaries and short user-facing reasons
(about 12 words). Include entry points, helper files, tests, docs, configuration,
and edits made by cooperating agents that explicitly share this workspace. A
multi-file patch needs a reason for each file; a later revision needs a new
explanation. Do not wait until the final
reply. Before finishing, check pending_changes once and explain your remaining
edits. Only explain changes you can attribute to your task; other edits stay
visibly unexplained. Per-file explanations can use more agent tokens than major
milestones; no measured token amount is promised.

Codex and Claude PostToolUse hooks can return bounded metadata reminders for
their concrete edit paths in files mode. They neither generate reasons nor
consume developer questions. Generated PostToolUse handlers deliver their context
before the agent resumes, with a bounded local wait and fail-open errors. Rerun
setup to migrate older async stef handlers; changed definitions may need host
review. Async Claude output can wait for the next user turn. Host trust/loading
still matters. Other clients keep the MCP contract without guessed hooks.
Bounded reminders disclose omitted revision/path counts. Report included IDs
first, then use POST /api/explanation-reminder with session_id and remaining
relative paths from the actual edit (at most 50 paths per request, settle_ms=0).
This avoids unrelated pending files crowding out recovery. Query again only after
an accepted batch makes progress or for the next path batch. Never poll or treat
a host's truncated preview as the complete file list.
If the host has not loaded stef tools, the existing executable accepts the same
report_changes JSON through stdin with --report-changes --repo <verified-root>
--daemon <verified-endpoint>. This uses MCP to an existing root-matched daemon;
it neither starts another agent nor executes edits. Use only current exact IDs.

In disabled mode, stop reporting and additional check-ins for the rest of this
turn. Read the mode again at the start of the next task. Saved history is retained;
edits made while capture is off are not replayed when it is re-enabled.

Mode changes reach a working agent at its next check-in. Never poll to discover
changes. Passive observation makes no model calls, and no second model is needed
for the agent's short reasons. Avoid hidden reasoning.

Minimal and Interactive are local presentation choices. Switching design,
selecting a step, viewing saved history, and navigating diffs make no requests
for extra AI output. Do not generate richer explanations merely because the
user chose Interactive. Explain more queues an explicit developer question;
Request explanation asks for the missing reason on that exact saved revision.
When that question arrives in feedback, answer only its referenced retained
step using report_event with context_id, matching the note's node and action,
and a concise fuller user-facing why. This updates the original step without
changing its recorded code. Explicit context can answer a historical step
in files or milestones; disabled still stops reporting. Do not expand other
steps or retry an old automatic batch.

When the selected mode permits it, use report_event for reads, tests, or plans; change_id can attach a single reason
to a saved revision. Consider advisory developer notes in receipts at natural
breaks. A missing explanation remains visible in the workflow. MCP instructions
guide connected agents; they cannot force an unconnected or non-cooperating host
to report. Do not claim that filesystem monitoring reveals the author's intent.

Include the returned session_id in later report_event, report_changes, and
get_state calls, including while sharing. Do not generate a new ID every turn
or confuse MCP transport identity with chat identity. When the host supplies no
chat ID, a connection's private fallback cannot restore a previous chat after
reconnection; retain a stable ID through set_session instead. Opening an idle
chat that emits no signal cannot switch stef by itself.

Keep each milestone to a short label and a brief user-facing reason for choosing
the file or making the change. Supply parents only for actual relationships;
other arrows represent step order. The UI contains the workflow, explanations,
a small suggestion field, a compact workspace/sharing control, and Clear. No
dashboard narration is needed. Clear affects the selected workspace; in a shared
workspace all members see the cleared workflow. Other private histories remain.
