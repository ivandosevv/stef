# stef

Stay involved while your AI codes.

stef is a local companion that shows what your agent changes and why. Follow
the workflow, inspect a diff, or send a suggestion while the agent works.

## What you can do

- Follow connected steps with short explanations from the agent.
- Click a file to see its saved diff and suggest something about an exact line.
- Choose major milestones, every file, or turn updates off.
- Keep the view minimal, or explore changes and supported CSS previews in Interactive.

The workflow and selected file scroll separately. Local browsing and previews
make no AI calls; explanations and questions may use agent tokens.

## Get started

Source builds need Git, Go 1.25+, Node 22.12+, npm, Python 3, and make.
Native Mac builds also need Xcode Command Line Tools.

From a stef checkout, run:

~~~sh
./scripts/bootstrap.sh --client codex --repo /absolute/path/to/your/project
~~~

Start your agent in that project and open the returned URL beside it.
No stef IDE extension is needed.
Replace codex with your client name, such as claude, cursor, opencode, or hermes.
Some clients need an import, trust approval, or reload. [Setup guide](docs/CLIENTS.md).

If the bundled skill is loaded, you can ask your agent to **enable stef for this project**.

## Current status

stef is an early personal project. Thirteen MCP configurations are tested;
verification inside every real client is still in progress. Public Brew and
prebuilt downloads are not available yet. General application behavior previews
are still planned.

[Getting started](docs/GETTING_STARTED.md) · [Validation](docs/VALIDATION.md) · [Roadmap](docs/ROADMAP.md)
