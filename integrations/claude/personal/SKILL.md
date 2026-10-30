---
name: stef
description: Enable or check stef, the companion that shows an AI agent's file changes and short explanations in real time.
---
<!-- stef-personal-bootstrap-v1 -->

stef is connected to this explicitly selected personal project:

- Project: `{{repo}}`
- Local view: {{endpoint}}/?companion=1
- Setup command: `{{connect_command}}`

The agent's current directory does not change the observed project. Never
automatically bind this connection to the home directory or another workspace.
Use the setup command only to restore this connection. Changing the observed
project requires the user's explicit choice.

For enable/status requests, use the stef MCP tools. On the first coding task in
a new chat, call set_session with a stable chat ID and unique agent_id. Retain
that original pair for set_session on resume; use its returned worker session_id
for other tools. When a trusted hook provides its identity,
bind that exact session_id with set_session omitting agent_id, then use the
hook's reporter agent_id for later calls. Reuse that identity on resume. Each
subagent needs its own unique agent_id and set_session call; do not pass the
parent's returned worker session_id as its identity.
Call get_state once at task
start and verify its repo matches the project above. Follow update_mode and
reporting_instructions in receipts.

Major milestones need only brief reasons. Every file needs the exact saved
revision explained immediately after each edit, before another edit or test.
Use trusted hook IDs or get_state with changed_paths from your actual edit and
settle_ms=300, then report_changes with the exact returned revision IDs.
Disabled stops reporting and extra check-ins for that turn. Only explain edits
from this chat in the selected project. Passive capture and local visuals make
no extra model calls.

Agents stay separate unless sharing is explicit. set_workspace action=create
returns a workspace_id; chosen peers join with action=join and that code. Keep
reporting with each worker's returned session_id. action=leave restores private
history without copying others' work. Shared check-ins expose only brief recorded
explanations and authors, not hidden reasoning or the peers' model conversations.
Updates and Clear apply to the selected workspace. Never poll for peer updates.

Open the local view in the existing IDE browser when available. Otherwise show
the URL; do not claim to have opened a pane. If stef tools are missing after
setup, ask the user to restart Claude so its personal MCP entry can load.
