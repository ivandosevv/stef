# stef inside VS Code and compatible editors

The extension adds a dockable **stef → Workflow** view and an optional editor pane.
Keep stef in the primary sidebar on the left and your coding agent in the secondary
sidebar or editor on the right. It does not replace or control the coding agent.

The extension packages the UI and a local Go engine. The webview receives events
through the extension host; it opens no browser tab and makes no direct network
connection. No separate development server is needed.

Build from the repository root with **make extension**. The current VSIX target is
macOS ARM64. Other operating systems need their matching Go binary and VSIX package.

Install the VSIX using **Extensions: Install from VSIX…**, open a trusted personal
project, and run **stef: Start Tracking Workspace**. File changes are then visible
regardless of which agent produced them. **stef: Show MCP Configuration** opens an
unsaved configuration example to copy into your chosen client's settings.

Tracking starts only on an explicit command unless stef.autoStart is enabled for
that workspace. An already-running stef engine is reused only when its tracked
root matches. A process owned by the extension is stopped when tracking is stopped;
an engine started independently is left running.

The same extension API is used by VS Code and compatible editors such as Cursor.
It has been tested in VS Code; editor-specific marketplace and API differences
still need verification. IntelliJ requires a separate JetBrains plugin.
