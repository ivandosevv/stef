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

## Client setup

The client catalog maps canonical names and aliases to documented configuration
paths, schema keys, and trust/reload instructions. --list-clients has no project
or process side effects. --enable extends the selected project configuration
or exports a root-confined import file, then starts or reuses its resident engine.
Endpoint discovery recognizes every catalog schema, including VS Code JSONC,
and requires an exact watched-root match. Import receipts explicitly distinguish
the observer from a client that has not yet imported or connected the server.

Codex, Claude Code, and Cursor have distinct lifecycle parsers. Other hosts
use the same MCP contract and local watcher without guessed hook formats.
No client setup installs an IDE extension or writes global user profiles.
The agent client and outer editor are separate choices; a Codex adapter does
not imply a VS Code browser-command API.

## Chat identity and storage

Every event has a session_id. Missing IDs attach to the currently selected chat;
legacy journal entries belong to local. Codex and Claude adapters hash their
reported session_id; Cursor hashes its stable conversation_id. These form
client-specific chat identities. Start/resume
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

Every file shows saved revisions, agent milestones, and supported read/edit/test
signals. All activity additionally keeps distinct supported lifecycle and tool
operations, including searches, plus brief agent-shared decisions (decided).
Watcher symbols, feedback events, and backend diagnostics stay hidden. Hook
rows display neutral observed context without inventing an agent reason. Closed steps are 38px high and show
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
Each workspace's update_mode is stored as a journal settings control and included
in snapshots and MCP receipts. files keeps automatic revisions and pending
explanations. activity also retains sanitized tool stages and brief decided
reports. milestones records unlinked agent milestones with short reasons,
while suppressing new watcher/hook/file explanation events. disabled suppresses
all new activity and model feedback, retaining earlier history. The watcher
advances its baseline without constructing suppressed diffs; epoch checks prevent
queued saves crossing a mode boundary from reappearing after re-enable.
MCP guidance follows mode changes at the next check-in without model polling.
The updates_from field marks the inclusive first event ID in the current reporting
window. Each actual mode change advances it to the next ID; no-op changes keep
it. Journal recovery derives it from ordered settings controls and event IDs.
The hub assigns recording_mode and recording_from to events from the actual
workspace setting, ignoring caller-supplied provenance. Saved activity is
reconstructed in its original window. Legacy unmarked events use files behavior,
so switching to activity never reveals older hidden hook noise.
Automatic pending lists and model activity exclude earlier windows, and stale
report_changes batches are rejected atomically. Old history remains available
for explicit inspection and clarification rather than automatic backfill.

Design is a local view preference, independent of reporting. Minimal keeps inline
reasons; Interactive shows the selected exact hunk's wrapped unified diff or a
milestone's reported action, state, and parent paths. All previews are computed
locally from saved events. Interactive and Show diff share the same parsed rows,
old/new line numbers, prefixes, source text, and colors; Interactive wraps source
without horizontal scrolling. Interactive bounds the workflow and properties
inside separate scroll containers with overscroll containment; narrow layouts
stack those containers. Following scrolls only the workflow container. Selecting
another file resets the properties pane; same-step late reasons retain its scroll.
Manual selection, saved history, and deliberate map/filter/line/sample
interactions pause following. A compact Follow latest control appears above
both designs and resumes automatic selection without replacing a nonempty
draft or an open diff. Pause is stored locally per workspace/recording window
and survives reload, reconnect, and design changes. New chats, Clear, and new
Updates windows follow by default. Exact-line suggestions
only open a bounded editable draft and focus it inside the properties pane.
Selecting another file or hunk resets the preview
scroll to its start; late reasons on the same revision preserve its position.
Closing a diff restores the opener after rendering, with a selected-step fallback
when following has replaced that button. Live tail step/revision changes select and reveal
the latest node after the DOM updates, without moving keyboard focus. An open
diff, focused suggestion input, or nonempty draft protects the inspected step.
Closing a diff, clearing a draft, or Escape keeps manual following paused.
Snapshot chat/window boundaries reset it; reload restores both selection and
the saved pause. A paused reconnect retains the inspected step while new tail
activity remains available through Follow latest. Late historical
reasons never steal selection. The view remembers the preference and suggestion draft
without requesting agent output. Explain more alone submits a targeted feedback
question. A report_event context_id reply validates the retained chat/node/action
and attaches full context to the original step. It creates no new file revision,
changes no diff or claims, and does not generate context for other steps.
An unexplained revision offers Request explanation in either design, using the
same exact feedback reference. A matching normal change_id explanation or
context_id reply resolves its queued state; unrelated or later saves do not.

In files and activity modes, MCP initialization requests a report_changes batch after each edit tool, covering
every affected file. get_state exposes bounded unexplained IDs/paths separately
from recent activity, with an optional bounded debounce delay. The hub validates
exact file/action/chat references and persists explanation batches atomically.
Late reasons fill the original step; another save stays a new unexplained step.
Legacy adjacent read/test reports still coalesce. Missing reasons stay visible.
Reasons should be authored with the edit, then reported before the next edit or
test. An orchestration call can sequence edit, bounded check-in, and exact report
without another model turn. Codex/Claude PostToolUse hooks can instead deliver
current pending IDs for their concrete edit paths through documented nonblocking
additionalContext. These metadata-only reminders make no model calls, do not
consume feedback, and skip milestones/disabled and earlier reporting windows.
Generated PostToolUse handlers are synchronous with a bounded local wait so
context reaches the agent before it resumes. Other lifecycle timing is preserved,
and errors fail open. Setup migrates only stef's own older async handler; actual
host trust/loading remains separate from adapter verification.
Reminder caps disclose omitted revision/path counts. The agent reports included
IDs first, then queries the path-filtered reminder endpoint for remaining files
from its actual edit, in batches of at most 50 paths. This prevents unrelated
pending revisions starving recovery; queries repeat only after an accepted batch
makes progress or for the next path batch. A host preview is not a complete list.
The --report-changes stdin fallback uses the official SDK Streamable HTTP
client to report to an existing root/version-matched daemon.

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
- Explicit context_id reports answer a developer question about one retained step.
- POST /api/changes attaches a batch of exact per-file explanations.
- POST /api/activity accepts supported lifecycle observations.
- POST /api/sessions/activate selects/restores a chat.
- POST /api/sessions/clear clears only the specified chat.
- POST /api/feedback queues a chat-scoped suggestion.
- POST /api/check-in returns compact activity, pending changes, and unseen notes.
- POST /api/explanation-reminder returns only current matching-path pending metadata, without offering notes.
- /mcp exposes set_session, report_event, report_changes, and get_state through the SDK.

The listener accepts only loopback authority with its actual port. Exact browser
Origin and Fetch Metadata checks reject unrelated local sites; development uses
a guarded same-origin proxy. Native local processes remain trusted. CSP and
stream validation protect display boundaries. MCP stdout contains only protocol
messages. Passive hooks and file watching never call a model. See
[Security](SECURITY.md) for the trust boundary and retention limits.

--enable starts or attaches to a resident root-matched engine. IDE presentation
is remembered in root-confined .stef/presentation.json, independently of MCP
client configuration. view_opened=false means the host still needs to open the
view_url. The skill uses a host browser tool when available, otherwise supplies
the URL. Closing an IDE tab disconnects the view; a resident engine continues.
MCP-owned engines stop with their bridge. Native windows stop their own engines.

Automatic chat selection depends on the host actually emitting start/resume or
MCP activation. An MCP server cannot independently observe an idle VS Code chat
tab switch. Direct panel integration remains separately verifiable.
