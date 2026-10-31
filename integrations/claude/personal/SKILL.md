---
name: stef
description: Enable or check stef, the companion that shows an AI agent's file changes and short explanations in real time.
---
<!-- stef-personal-bootstrap-v1 -->

stef is connected to this explicitly selected personal project:

- Project: `{{repo}}`
- Daemon: {{endpoint}}
- Setup command: `{{connect_command}}`

The agent's current directory does not change the observed project. Never
automatically bind this connection to the home directory or another workspace.
Use the setup command only to restore this connection. Changing the observed
project requires the user's explicit choice.

For enable/status requests, use the stef MCP tools. When enabling or
starting/resuming a chat, call set_session once with a stable chat ID and unique
agent_id. Retain that original pair for set_session on resume; use its returned worker session_id
for other tools. When a trusted hook provides its identity,
bind that exact session_id with set_session omitting agent_id, then use the
hook's reporter agent_id for later calls. Reuse that identity on resume. Each
subagent needs its own unique agent_id and set_session call; do not pass the
parent's returned worker session_id as its identity.
Every explicit set_session starts privately, including resumed shared chats.
Use resume_shared=true only when the user asks to retain previous sharing.
Normal check-ins and reports keep active sharing; do not bind again every turn.
Receipts include workspace_name and view_path. Readable names such as Quiet
Fox 2487 come from local word lists and numbers, stay stable on resume and
Clear, and use no AI. Describe workspaces by name while retaining IDs for tools.
Call get_state once at task
start and verify its repo matches the project above. Follow update_mode and
reporting_instructions in receipts.

Major milestones need only brief reasons. Every file and All activity need the exact saved
revision explained immediately after each edit, before another edit or test.
Use trusted hook IDs or get_state with changed_paths from your actual edit and
settle_ms=300, then report_changes with the exact returned revision IDs.
All activity also shows available sanitized tool stages and brief public
decisions via report_event action=decided. Never send private thinking, prompts,
commands, or raw tool output. Extra agent reports may use tokens. Start with new work.
Disabled stops reporting and extra check-ins for that turn. Only explain edits
from this chat in the selected project. Passive capture and local visuals make
no extra model calls.

Use get_state limit=1 for post-edit pending IDs; request more history only when
coordination needs it. Pending IDs and developer notes are independent of that limit.

Reuse a concise public explanation as why instead of generating another.
report_changes accepts common summary/why with exact change_id entries; override
reasons that differ by file. Keep progress prose in stef and chat status brief;
essential questions and final outcomes stay in chat. This avoids repetition,
not the original token cost. Never infer exact revision reasons from a general reply.

Agents stay separate unless sharing is explicit. set_workspace action=create
returns workspace_id and name; chosen peers join with action=join
using that readable name in workspace_id, or the invitation ID. Keep
reporting with each worker's returned session_id. action=leave restores private
history without copying others' work. Shared check-ins expose only brief recorded
explanations and authors, not hidden reasoning or the peers' model conversations.
Updates and Clear apply to the selected workspace. Never poll for peer updates.

After binding, open {{endpoint}} plus the returned view_path in the existing
IDE browser when available. This scopes the view to this worker; do not open
an unscoped daemon URL. Otherwise show that URL; do not claim to have opened a
pane. If stef tools are missing after
setup, ask the user to restart Claude so its personal MCP entry can load.
