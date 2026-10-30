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
a new chat, select a unique session ID with set_session and retain it in the
conversation. Reuse that ID when the chat resumes. Call get_state once at task
start and verify its repo matches the project above. Follow update_mode and
reporting_instructions in receipts.

Major milestones need only brief reasons. Every file needs the exact saved
revision explained immediately after each edit, before another edit or test.
Disabled stops reporting and extra check-ins for that turn. Only explain edits
from this chat in the selected project. Passive capture and local visuals make
no extra model calls.

Open the local view in the existing IDE browser when available. Otherwise show
the URL; do not claim to have opened a pane. If stef tools are missing after
setup, ask the user to restart Claude so its personal MCP entry can load.
