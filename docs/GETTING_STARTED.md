# Get started

Open the project you want to work on in your usual coding agent. Tell it:

> Install https://github.com/ivandosevv/stef for this project and enable it.

The agent uses [INSTALL.md](../INSTALL.md) to choose the OS and MCP setup. stef
lives in that project's `.stef` folder. Continue working in the same project.
macOS, Windows, and Linux use the same workflow and MCP contract. Binary setup
does not need Go, Node, make, or a stef IDE extension. Source setup has its own
build prerequisites.

The first setup may require your agent's normal MCP trust, reload, or import
step. After updating stef from source, reconnect an already-running MCP server
to load new reporting actions. The installer reports the actual step. Once connected, the agent binds
its private workspace before opening the scoped view beside you. IDEs with a
built-in browser can show it in an editor tab; other hosts can use its local
browser link. A native macOS companion remains an optional source-build choice.

## Use it while coding

New changes select automatically. Selecting a step pauses following so you can
read; Follow latest resumes it. Show diff opens that exact saved revision, with
additions and removals. Suggest lets you send a note at the
agent's next check-in. Clear resets the selected workspace's workflow.

Agents can reuse an existing short public explanation in stef. One reason can
cover matching file revisions, with individual reasons where the purpose differs.
Detailed progress stays in stef; chat keeps brief status and final outcomes. This
reduces repeated text, while the original output and MCP communication still cost
tokens.

Updates chooses Major milestones, Every file, All activity, or Disabled. Changes
apply to new work. Earlier steps stay available through Show saved history.
All activity adds available searches, reads, tool stages, and brief reasons before
the agent changes direction. For example: “I’m checking startup because the saved
preference resets there.” That reason stays visible as newer actions arrive.
Agents reuse one short public sentence per meaningful action group, skipping
repeated tools and housekeeping. It cannot expose private thinking. Every file and All activity
may use more agent tokens for explanations; observation makes no AI calls.

Design chooses Minimal or Interactive. Interactive explores the selected saved
change, with independent scrolling for the workflow and its details. Local
visuals, appearance changes, names, and saved diffs make no AI calls. Explain
more requests extra agent context only when you choose it.

Every agent starts privately with a readable name. Start a shared workspace and
give its name to cooperating agents to join. Leaving restores private history.
MCP does not expose hidden reasoning or merge model conversations.

[Client setup and verification](CLIENTS.md) · [Reporting skill](../.agents/skills/stef/SKILL.md)
