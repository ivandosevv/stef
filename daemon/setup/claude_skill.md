---
name: stef
description: Connect stef to this configured project and explain coding progress at the developer's chosen update level. Use when asked to enable stef or while working with stef connected.
---

stef visualizes this agent's work locally. This skill is configured for project
{{.Root}}, executable {{.Executable}}, and daemon {{.Endpoint}}.
Use these exact paths even when Claude starts elsewhere. Observe only this
project; select another project only when the user explicitly requests it.

To enable or reconnect the companion for this project:

~~~sh
{{.EnableCommand}}
~~~

Use Claude's /mcp menu to approve or reconnect stef. Newly written configuration
may require restarting Claude from this project. A project skill and MCP entry
apply to that project, not other shell directories. Honor the client's trust
controls. Do not write global profiles automatically.
Open the returned view_url using the host's built-in browser capability if
available; otherwise give the localhost link for the existing browser tab.
The engine being ready does not mean the host has opened the view.

At each task's start, call get_state once and follow update_mode and the
reporting_instructions in receipts. Reuse the chat's session_id; if lifecycle
hooks have not supplied it, use set_session once and retain its returned ID.

- milestones: report only major steps through report_event with a short label
  and a brief reason. Skip per-file explanations and edit check-ins.
- files: prepare each file's short reason with the edit. Immediately after the
  edit, use exact pending revision IDs from the trusted hook, or get_state once
  with settle_ms=300, then report_changes before another edit, test, or final
  reply. Batch every changed file, including helpers, tests, and cooperating
  agents' edits. Use 2 to 4 word summaries and one short user-facing reason.
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

~~~sh
{{.ReportCommand}}
~~~

Minimal and Interactive use saved data locally. Changing design, inspecting
diffs, and scrolling make no extra model calls. Explain more and Request
explanation queue explicit questions in feedback; answer only the referenced
step through report_event with context_id and a brief user-facing why. Disabled
still stops reporting. Keep reasons practical and concise, without hidden
reasoning.
