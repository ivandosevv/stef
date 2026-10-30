---
name: stef
description: Connect stef to this configured project and explain coding progress at the developer's chosen update level. Use when asked to enable stef or while working with stef connected.
---

stef visualizes this agent's work locally. This skill is configured for project
{{.Root}}, executable {{.Executable}}, and daemon {{.Endpoint}}.
Use these exact paths even when Claude starts elsewhere. Observe only this
project; select another project only when the user explicitly requests it.

To enable or reconnect the companion for this project, use this command in
the current platform's shell (PowerShell on Windows, POSIX elsewhere):

~~~text
{{.EnableCommand}}
~~~

Use Claude's /mcp menu to approve or reconnect stef. Newly written configuration
may require restarting Claude from this project. A project skill and MCP entry
apply to that project, not other shell directories. Honor the client's trust
controls. Do not write global profiles automatically.
Bind this worker with set_session before opening the view. Use {{.Endpoint}}
plus the returned view_path, not the unscoped setup view_url. Open it using the
host's built-in browser capability if available; otherwise give that localhost
link for the existing browser tab. Complete Claude's trust/reload step first
if its tools are not loaded.
The engine being ready does not mean the host has opened the view.

When enabling stef or starting/resuming a chat, call set_session once with the
originally chosen stable chat ID as session_id and this worker's unique agent_id. Retain
that pair for set_session on resume and use the returned worker session_id for
every other tool. When a trusted hook supplies worker metadata,
bind its exact session_id with set_session omitting agent_id, then use the hook's
agent_id on reports. Otherwise create a unique ID and retain it. Every explicit
set_session starts privately, including resumed shared chats. Set
resume_shared=true only when the user asks to retain prior sharing. Normal
check-ins and reports keep active sharing; do not bind again every turn.
Receipts include workspace_name and view_path. Names such as Quiet Fox 2487
are generated locally from word lists and numbers and remain stable on resume
and Clear, without AI calls. Then call get_state once at task start and follow
update_mode and reporting_instructions.
Native Windows setup uses MCP without changing Claude's passive hook settings.
Use changed_paths plus the bounded MCP check-in after edits when no trusted
hook supplies exact revision IDs.
Each agent and subagent has a private workspace by default. Give subagents their
own unique agent_id and set_session call; MCP cannot distinguish anonymous
workers sharing a transport. Do not give them the parent's returned session_id.

For explicit cooperation, set_workspace action=create returns a shared
workspace_id and name. Give the readable name to chosen workers and
ask each to call action=join with that name in workspace_id; its invitation ID
is also accepted. Keep using each worker's returned session_id for all
reports. action=leave restores its private history without copying peer steps.
Shared get_state includes bounded recorded explanations and authors at natural
check-ins. Sharing never merges hidden reasoning or model chats, and does not
start model calls. Updates and Clear apply to the selected workspace. Never poll.

- milestones: report only major steps through report_event with a short label
  and a brief reason. Skip per-file explanations and edit check-ins.
- files: prepare each file's short reason with the edit. Immediately after the
  edit, use exact pending revision IDs from the trusted hook, or get_state once
  with settle_ms=300 and changed_paths listing only the actual relative edit
  paths, then report_changes before another edit, test, or final
  reply. Batch every changed file, including helpers, tests, and cooperating
  agents' edits when they explicitly share this workspace. Use 2 to 4 word
  summaries and one short user-facing reason.
- disabled: stop reporting and extra check-ins for this turn. Recheck the mode
  at the next task's start.

Sequence each edit, bounded check-in, and its exact report in one orchestration
call when possible. Changes to Updates start a new recording window. Explain
only your new revisions, never backfill retained history automatically. Check
pending_changes once before finishing and explain remaining edits you authored.
Filesystem observation cannot supply the author's reason. Missing MCP tools
mean the client has not connected; do not invent explanations or claim success.
For exact pending IDs already obtained from stef, the same report_changes JSON
can be passed through stdin to the configured executable:

~~~text
{{.ReportCommand}}
~~~

Minimal and Interactive use saved data locally. Changing design, inspecting
diffs, and scrolling make no extra model calls. Explain more and Request
explanation queue explicit questions in feedback; answer only the referenced
step through report_event with context_id and a brief user-facing why. Disabled
still stops reporting. Keep reasons practical and concise, without hidden
reasoning.
