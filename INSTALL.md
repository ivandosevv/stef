# Install stef for the current project

Instructions for an agent receiving “install https://github.com/ivandosevv/stef”.
Keep the user's current project and coding agent. Do the setup; report any actual
client trust or reload step that remains.

1. Resolve the project already open or being worked on. Honor its exclusions.
   If only a home directory or filesystem root is known, ask for the project
   folder. The stef source checkout is the installer source, not the user's
   working project.
2. Choose the actual agent independently of the IDE. Codex in Cursor uses
   `codex`; Claude Code in IntelliJ uses `claude`. Supported setup names include
   `cursor`, `vscode` (Copilot), `gemini`, `opencode`, `roo`, `devin`, `jetbrains`,
   `windsurf`, `cline`, and `hermes`. For another MCP client, use `generic` and
   import its exported entry through the client's documented controls.
3. Download and run the installer below for the current OS. Pass the absolute
   project folder as `--repo` and the chosen agent as `--client`. Use the host's
   process tool with separate arguments. Keep paths containing spaces intact.
   Setup defaults to the IDE/browser view, with no editor extension.

## macOS and Linux

Fetch `https://raw.githubusercontent.com/ivandosevv/stef/master/install.sh` to a
temporary file, then run it with the system shell:

~~~sh
sh /path/to/stef-install.sh --repo /absolute/project --client codex
~~~

This downloads the matching release and verifies its checksum using standard
macOS/Linux tools. No Python, Git, Go, Node, or make is needed.

For source development, run `python3 install.py --source` from a stef checkout
with the same project and client arguments. Source builds need Python 3.9+,
Git, Go 1.25+, Node 22.12+, and npm.

## Windows

Fetch `https://raw.githubusercontent.com/ivandosevv/stef/master/scripts/bootstrap.ps1`
to a temporary file and run it in a new PowerShell process:

~~~powershell
powershell.exe -NoProfile -ExecutionPolicy Bypass -File 'C:\path\to\stef-bootstrap.ps1' -Repo 'C:\my project' -Client codex
~~~

Use the current project path, not the downloaded script's folder. Binary setup
uses the Windows release without Python, Bash, or compilers. Use -Source for
an explicit source build with Python 3.9+, Git, Go, Node, and npm.

## Finish inside the same agent

The installer returns the executable, selected project, endpoint, configuration
or import path, and the client's next step. Preserve existing client settings.
An export is ready for import; it does not mean the client loaded the server.

After the client loads stef, call `set_session` once with the original stable
chat ID and a distinct `agent_id`. Retain the returned origin `session_id` for
later tools. New starts are private; `resume_shared=true` explicitly keeps prior
sharing. Every subagent uses its own identity.

Check `get_state` once and verify `repo` matches the user's project. Open the
returned endpoint plus `view_path` with the IDE's actual built-in browser tool.
If it has no such tool, provide that scoped localhost link. Say a view opened
only after observing it. Follow `update_mode` and `reporting_instructions`;
explain actual new work at that level. Do not generate sample activity.

The bundled [stef skill](.agents/skills/stef/SKILL.md) describes reporting, saved
diffs, suggestions, and shared workspaces. The MCP server provides the same
reporting instructions to clients that do not support skills.
