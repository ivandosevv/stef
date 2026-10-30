# stef

Stay involved while your AI codes.

stef is a local companion that shows what your agent changes and why. Follow
the workflow, inspect a saved diff, or suggest something while the agent works.
Choose major milestones, every file, or no updates. Keep the view minimal or
explore changes in Interactive. Local visuals make no AI calls; agent
explanations and questions may use tokens.

Each agent and subagent starts privately with a name like **Quiet Fox 2487**,
generated locally without AI. Start a shared workspace and pass its name to
chosen agents to work together. Leaving restores your private history.

## Get started

Source builds need Git, Go 1.25+, Node 22.12+, npm, Python 3, and make.
Native Mac builds also need Xcode Command Line Tools.

From a stef checkout, run:

~~~sh
./scripts/bootstrap.sh --client codex --repo /absolute/path/to/your/project
~~~

Replace codex with your client, such as claude, cursor, opencode, or hermes.
Start your agent there, load its MCP, and ask it to **enable stef for this project**.
It binds a private workspace before opening its view beside you. No stef IDE
extension is needed. Some clients need trust, reload, or import. [Setup guide](docs/CLIENTS.md).

For Claude launched from home, run `python3 scripts/connect-claude.py --repo /path/to/personal/project`.
Restart Claude once to load `/stef` and its MCP from your personal settings.

## Current status

stef is an early personal project. Client configurations are tested; real-client
verification continues. Public Brew and prebuilt downloads are not available yet.

[Getting started](docs/GETTING_STARTED.md) · [Validation](docs/VALIDATION.md) · [Roadmap](docs/ROADMAP.md)
