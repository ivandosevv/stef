# stef architecture

~~~mermaid
flowchart LR
    A[Agent] -->|Per-file reasons and parents| M[MCP]
    A -->|Start, resume and tool signals| H[Lifecycle adapter]
    F[File observer] --> E[Local engine and journal]
    M --> E
    H --> E
    E -->|Active chat snapshot and steps| V[Existing IDE browser or native view]
    V -->|Suggestion at next check-in| M
    V -->|Clear this chat| E
~~~

The Go engine embeds the Svelte renderer and serves it on loopback. IDE mode uses
the existing browser editor; native mode uses macOS WebKit. The observer,
client adapters, MCP contract, and presentation remain independent.

## Chat identity and storage

Every event has a session_id. Missing IDs attach to the currently selected chat;
legacy journal entries belong to local. Codex and Claude adapters hash their
reported session_id into a stable client-specific chat identity. Start/resume
selects that identity and emits a fresh snapshot. Repeated prompts in the same
chat append activity and never clear its history.

Without lifecycle signals, set_session selects a stable conversation ID through
stdio or Streamable HTTP MCP. The agent retains that ID in the conversation and
uses it for later milestones. MCP transport-session IDs are not treated as chat
IDs: one MCP connection can serve several conversations.

The journal stores events and activate/clear control records. Recovery restores
all chat histories, the selected chat, monotonic event IDs, and cleared segments.
Each chat retains up to 5000 events in memory. Clear resets one chat's visible
history and claims; other chats are preserved. Older raw records remain in the
append-only journal behind the clear marker. Clear is a workflow reset, not a
secure data-erasure operation.

Background events explicitly assigned to another chat are stored without
changing or entering the active view. Feedback and write claims are also scoped
to the chat. Passive filesystem attribution follows the active chat; it does not
establish which agent changed a file. Whole-file watcher events create visible
steps automatically; symbol events remain stored without duplicating them.

A browser subscription receives only the active chat's snapshot/events. Chat
activation and Clear broadcast replacement snapshots. The view remembers its
selected step per chat and removes stale selections on Clear. Every saved file
revision keeps its observation ID. Agent explanations reference that exact ID,
updating the reason without moving the step or losing the selection.

## Workflow and explanations

The visible workflow contains each saved text-file revision, agent milestones,
and supported read/edit/test signals. It excludes watcher symbols, lifecycle noise,
feedback events, and backend diagnostics. Closed steps are 38px high and show
the filename and short label. Selecting one expands its brief explanation inline;
Suggest reveals the input on demand. The header contains stef, Clear, and a small
appearance button plus the compact Updates control. Show diff on a selected file
opens its recorded unified patch in a responsive dialog, with the same live AI
reason above old/new line numbers and colored additions/removals. The renderer
parses hunks locally, escapes source as text, and preserves whitespace. Exact
observation IDs keep earlier diffs stable when later saves or reasons arrive.
Captures compare consecutive observed saves; they do not compare against Git HEAD.
Bounded 16 KiB previews retain a visible truncation warning. Empty files do not
invent changed lines. Opening or closing a diff makes no model call; closing it
preserves the selected step and suggestion draft.
The appearance menu offers dark, light, warm Eye comfort, and OS-following
System. Shared CSS variables cover the page, labels, controls, inputs, and SVG
arrows/arrowheads. The saved view preference is applied before mounting and
survives reloads, chat switches, and Clear without modifying workflow history.
System changes use the browser's media-query signal and make no model calls.
The root-wide update_mode is stored as a journal settings control and included
in snapshots and MCP receipts. files keeps automatic revisions and pending
explanations. milestones records unlinked agent milestones with short reasons,
while suppressing new watcher/hook/file explanation events. disabled suppresses
all new activity and model feedback, retaining earlier history. The watcher
advances its baseline without constructing suppressed diffs; epoch checks prevent
queued saves crossing a mode boundary from reappearing after re-enable.
MCP guidance follows mode changes at the next check-in without model polling.

In files mode, MCP initialization requests a report_changes batch after each edit tool, covering
every affected file. get_state exposes bounded unexplained IDs/paths separately
from recent activity, with an optional bounded debounce delay. The hub validates
exact file/action/chat references and persists explanation batches atomically.
Late reasons fill the original step; another save stays a new unexplained step.
Legacy adjacent read/test reports still coalesce. Missing reasons stay visible.

Solid arrows connect the previously visited parents reported by the agent.
Dashed arrows connect consecutive steps in the chat. These sequences do not
claim that one file calls another. Lifecycle observations without supplied
rationale show no invented reason. No hidden chain-of-thought is requested.

## Transport and ownership

- GET /healthz verifies product, canonical root, and version.
- GET/POST /api/settings reads or saves the root-wide update_mode.
- GET /ws streams active-chat snapshots and events.
- GET /api/events optionally reads one session_id.
- POST /api/events accepts a report, optionally linked by change_id.
- POST /api/changes attaches a batch of exact per-file explanations.
- POST /api/activity accepts supported lifecycle observations.
- POST /api/sessions/activate selects/restores a chat.
- POST /api/sessions/clear clears only the specified chat.
- POST /api/feedback queues a chat-scoped suggestion.
- POST /api/check-in returns compact activity, pending changes, and unseen notes.
- /mcp exposes set_session, report_event, report_changes, and get_state through the SDK.

Host/Origin checks confine network access to the local view. MCP stdout contains
only protocol messages. Passive hooks and file watching never call a model.

--enable starts or attaches to a resident root-matched engine. IDE presentation
is remembered in root-confined .stef/presentation.json, independently of MCP
client configuration. view_opened=false means the host still needs to open the
view_url. The skill uses a host browser tool when available, otherwise supplies
the URL. Closing an IDE tab disconnects the view; a resident engine continues.
MCP-owned engines stop with their bridge. Native windows stop their own engines.

Automatic chat selection depends on the host actually emitting start/resume or
MCP activation. An MCP server cannot independently observe an idle VS Code chat
tab switch. Direct panel integration remains separately verifiable.
