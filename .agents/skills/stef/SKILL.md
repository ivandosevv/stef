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

For the Codex panel in VS Code, use its built-in browser editor:

~~~sh
stef --enable --client codex --view ide --repo "$project_root"
~~~

For Claude Code, use --client claude. Use --view native for a terminal's native
window or when explicitly requested. Honor another host's documented presentation
capabilities and the user's choice. Quote the executable and resolved root.
Enable selects/reuses a loopback port, preserves client settings, writes
project-local MCP/hooks, and starts a resident engine. The project remembers
--view ide so later MCP autostart also runs without a native window.
A conflicting existing stef entry needs review; do not overwrite it.
Use the returned endpoint to verify /healthz matches the intended root.

For IDE view, open the returned view_url in the current editor group's built-in
browser using the host's browser-control tool or command API when available.
VS Code's command is workbench.action.browser.open with the URL as its argument.
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

Other clients can use the same stdio/HTTP MCP contract, but only configure a
schema documented for that client. Codex and Claude Code have the automated
setup adapters; do not claim verified support for another host.

At the start of a task, get_state once and follow update_mode and the compact
reporting_instructions returned in each receipt. The Updates control is shared
by the project's views and persists across chats and engine restart.
Updates changes start a new reporting window, identified by updates_from.
Only new work enters automatic reporting. Do not request or backfill reasons
for saved history when the user changes the mode or opens older steps.

In milestones mode, report only major steps with brief reasons through
report_event. Use the real action, running/ready state, and reported parents
to support its local visualization. Skip file explanations and per-edit check-ins.

In files mode, every eligible saved text-file revision appears automatically. After each
edit tool, call get_state once with settle_ms=300 to allow the watcher debounce,
then report_changes using exact pending change_id values for every file you
changed. Send one batch with 2–4 word summaries and short user-facing reasons
(about 12 words). Include entry points, helper files, tests, docs, configuration,
and edits made by your cooperating agents. A multi-file patch needs a reason for
each file; a later revision needs a new explanation. Do not wait until the final
reply. Before finishing, check pending_changes once and explain your remaining
edits. Only explain changes you can attribute to your task; other edits stay
visibly unexplained. Per-file explanations can use more agent tokens than major
milestones; no measured token amount is promised.

In disabled mode, stop reporting and additional check-ins for the rest of this
turn. Read the mode again at the start of the next task. Saved history is retained;
edits made while capture is off are not replayed when it is re-enabled.

Mode changes reach a working agent at its next check-in. Never poll to discover
changes. Passive observation makes no model calls, and no second model is needed
for the agent's short reasons. Avoid hidden reasoning.

Minimal and Interactive are local presentation choices. Switching design,
selecting a step, viewing saved history, and navigating diffs make no requests
for extra AI output. Do not generate richer explanations merely because the
user chose Interactive. Explain more queues an explicit developer question.
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

stef keeps one workflow per chat. Trusted Codex/Claude start/resume hooks select
a stable hashed chat ID. If the host does not supply those signals, call
set_session once on a new/resumed conversation: reuse an ID already retained in
that conversation, otherwise create a unique ID and retain the returned ID in
the chat context. Include that session_id in later report_event/get_state calls.
Do not generate a new ID on every turn or confuse an MCP connection ID with a
chat ID. Opening an idle chat that emits no signal cannot switch stef by itself.

Keep each milestone to a short label and a brief user-facing reason for choosing
the file or making the change. Supply parents only for actual relationships;
other arrows represent step order. The UI contains the workflow, explanations,
a small suggestion field, and Clear for this chat. No dashboard narration is
needed. Clear starts a new segment without clearing another chat's workflow.
