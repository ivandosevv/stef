# Local validation — 2026-10-30

The 0.3.0 native companion prototype was built and verified on macOS ARM64.
All working files and fixtures are inside the personal stef project; Go unit tests
also use automatically cleaned standard temporary directories.

- make check passed: Go vet and Svelte/TypeScript diagnostics, with zero UI errors/warnings.
- make test passed: backend race tests and nine UI timeline/graph/collision tests.
- Production UI build was embedded into bin/stef; bin/stefd remains a headless alias.
- Four Chromium end-to-end tests passed against the real isolated engine:
  demo/inspector/themes/filter/replay/export/mobile; actual file changes and
  telemetry/reconnects; optional embedded host bridge; native-layout note drawer,
  targeted feedback queue/check-in, and graph/rationale preservation.
- The actual macOS native window launched, loaded the bundled Svelte renderer,
  and reported its ready callback. A separate actual stdio MCP smoke test
  automatically launched the native companion, queued a human note via HTTP,
  received it in a real SDK report_event reply, and verified owned shutdown at EOF.
- Backend tests cover root/version-matched reuse, actual stdio auto-start and
  shutdown, targeted/wildcard notes, batches of three, once-per-agent offers,
  Unicode/size/path validation, and feedback over forwarding and Streamable HTTP.
- Native session export tests cover valid JSON output and symlink escape rejection.
- A Linux headless executable cross-compiled successfully with CGO disabled.
- An unsigned macOS ARM64 archive and SHA256 were generated; the draft local Brew
  recipe passed Ruby syntax validation. No Brew install or public tap was tested.
- The companion-layout screenshot was inspected. Screenshots and
  artifacts/native-smoke.log are in the ignored artifacts directory.

The webview dependency emits three upstream C++ deprecation warnings when
compiled with this machine's toolchain. Builds and tests complete successfully.
Earlier optional VS Code experiment tests verified its isolated host/profile and
packaged VSIX; they are outside the default native product checks now. No normal
IDE profile, global MCP configuration, or external project was changed.

The current tests exercise stef and the official MCP SDK. They do not certify
real Claude Code, Codex, Cursor, VS Code agent, IntelliJ-agent, or Hermes sessions,
Windows/Linux native presentation, clean-machine Brew distribution, signing,
long-session performance, or measured token cost. Native mounting and MCP
feedback are verified; exhaustive native WebKit control automation remains work.

The previous dependency audit reported a clean production UI dependency tree,
with two low development-tool advisories from the Svelte plugin/deepmerge. The
runtime dependencies were not changed in this increment. Coverage percentage
was not recalculated for this increment.

A GitHub Actions workflow targets the headless core and browser tests; optional
IDE installation is outside that default path. No remote repository, release,
Homebrew tap, or GitHub workflow execution has been created by this work.

## Codex setup increment

- make check and make test pass, including the new Codex hook adapter and
  project-local configuration preservation/idempotency/confinement tests.
- Actual Codex CLI 0.165.0 loaded trusted-project MCP configuration, launched
  stef, and shut down its owned companion on exit. This startup-only test used
  --ignore-user-config and isolated generated/vetted hooks, with no global setup.
- A real addition-fix task was attempted, but standalone CLI credentials were
  rejected with HTTP 401 before model execution. This is an authentication limit,
  not a passing task test. Full task, live client hook, milestone, and developer
  note verification through Codex remain pending.
- The personal checkout now contains local, Git-ignored .codex/config.toml and
  .codex/hooks.json on port 4825. No SAPDevelop or Hyperspace files or global MCP
  settings were changed. Normal users review/trust hooks through Codex /hooks.
- Official release behavior was checked against OpenAI Docs MCP, hooks, and
  configuration references before implementing the adapter.

## Agent-chat enable increment

- make check and make test pass after adding Claude Code setup, endpoint discovery,
  and resident enable. Tests verify preservation/idempotency/conflicts and sharing
  a root's configured endpoint, plus the actual CLI enable process selecting a
  port and outliving its setup command before controlled demo shutdown.
- The bundled stef skill passes the Skill Creator validator using a disposable
  project-local Python environment. Codex and Claude discover the same source
  through their documented project skill locations.
- A separate personal VS Code workspace was opened without installing a stef
  extension. The resident native window is launched against this personal root.
- Existing browser and native MCP verification is retained. Full real Codex/Claude
  task execution inside the VS Code panels has not been certified by these tests.
  The previous standalone Codex HTTP 401 does not establish a panel auth failure.

## Live personal-project test

- The existing Codex conversation fixed examples/sandbox/main.go while the resident
  companion observed only /Users/I751316/PersonalProjects/stef on port 4825.
  The greeting tests first failed for padded, blank, whitespace-only, and Unicode
  names, then all five cases passed after trimming and adding the friend default.
- A browser connected before the edit received four live watcher events, including
  the actual Greeting.Message modification. It displayed the method diff containing
  strings.TrimSpace beside the real MCP milestone and reported no JavaScript errors.
  artifacts/stef-live-vscode-test.json and .png retain the observation.
- No developer note was pending at the task check-in. A clearly labeled synthetic
  suggestion was submitted through the actual Chip in UI, targeted at
  codex-ide-live-test, and returned by get_state through the official stdio MCP
  bridge. A subsequent milestone confirmed the requested cases were tested.
  artifacts/stef-live-chip-in-test.png shows the submitted test note.
- make check and make test passed, including Go race checks, the five greeting
  cases now included in make test, and nine UI tests.
- This conversation's tool catalog has not loaded stef, so these genuine MCP calls
  used scripts/mcp-call.py against the running bridge. This verifies live local
  observation, milestone display, and the UI-to-MCP feedback path; it does not
  certify automatic Codex panel hooks or direct tool discovery in this conversation.
  The configured personal workspace still needs the client's MCP reload/trust step.

## IDE browser presentation

- --enable --view ide starts a resident headless engine and returns its view_url,
  with view_opened=false until the host actually opens a browser tab. The choice
  is remembered in root-confined .stef/presentation.json for future MCP starts.
- Tests verify saved selection, explicit overrides, invalid-state recovery,
  symlink escape rejection, and an actual IDE-mode resident CLI startup without
  --headless on the setup command.
- make check, make test, and all four browser end-to-end tests passed after this
  change. The personal checkout was enabled on port 4825 with --view ide; no
  SAPDevelop files, workspace configuration, or extension installations changed.
- The installed VS Code 1.144.0 includes workbench.action.browser.open, and its
  official integrated-browser documentation confirms localhost editor tabs.
  This conversation has no host browser-control tool, and macOS accessibility
  automation is disabled. Automatic navigation of the user's existing tab was
  therefore not performed or certified; the ready URL can be pasted there.

## Minimal workflow and chat histories

- The interface now contains stef, Clear, workflow cards and arrows, short
  explanations, and a compact suggestion input on the selected step. The old
  metrics, filters, themes, code inspector, replay/export/demo controls, and
  note drawer are removed from the rendered product.
- Watcher details stay out of the AI workflow. Adjacent observed edits can be
  paired with agent explanations. Solid arrows represent reported parents;
  dashed arrows show chronological order without claiming code dependencies.
- New chat IDs start empty. Existing IDs restore their steps and selection.
  Tests verify journal recovery across engine restart, durable Clear markers,
  same-chat new turns, background-chat isolation, and scoped feedback/claims.
- Codex and Claude adapters expose stable hashed session IDs from their
  documented lifecycle payloads. Start/resume switches the active view; an
  ordinary new prompt in the same chat does not erase it. MCP set_session
  provides the corresponding new/resumed-chat path for other connected clients.
- Official-SDK tests verify chat selection, restore, clearing, and feedback
  through both stdio forwarding and Streamable HTTP. make check and make test
  pass, including race checks, five greeting cases, and twelve UI tests.
- Three current browser tests pass: minimal real stream/arrows/reasons/notes and
  mobile layout; new/resumed chats and scoped Clear; optional host file opening.
  The Skill Creator validator also passes. Screenshots are in artifacts.
- The resident personal engine was restarted without a native window on 4825.
  The old local workflow was cleared, and three real task milestones were sent
  through the MCP bridge in chat-b704246ab996444295ecc75a4d1ac8bd.
  artifacts/stef-minimal-live-check.json records the compact MCP receipts.
- Actual idle-chat switching in a real Codex/Claude panel remains unverified.
  This existing conversation has not loaded stef into its tool catalog, and a
  host must emit start/resume or call set_session for automatic selection.
  No SAP/Hyperspace files, global client settings, or extension installs changed.

## Quieter design and observed coding task

- The header now contains only stef and a discreet Clear control. Closed steps
  contain labels only; selection reveals the brief reason/file inline and Suggest
  reveals its input. The document title is stef.
- MCP initialization explicitly says stef is connected and requests sparse
  start/result reporting with the same node/action. The portable skill gives the
  same guidance. scripts/mcp-call.py --tool connection verifies the real initialize
  response and tool catalog; connection is a helper mode, not another MCP tool.
- make check and make test pass: zero Svelte/TypeScript diagnostics, backend race
  tests, eight greeting cases, and thirteen UI tests. All three browser E2E tests
  pass for the current UI. The production renderer is embedded in the executable.
- The verified personal headless engine was restarted on port 4825 in IDE mode.
  Only chat-b704246ab996444295ecc75a4d1ac8bd was cleared for the live task.
- The actual task added command-line names to examples/sandbox/main.go and three
  argument tests. A browser attached before work received six genuine stdio MCP
  reports (three starts and three results) plus six live sandbox file events.
  It displayed three steps and two reported-parent arrows, with no JavaScript
  errors. go run with the argument Иване printed Hello, Иване!
- A real user note, can you explain more, was submitted through stef and returned
  in report_event receipt 163. The next report updated the same test step to
  explain the default, trimming, and blank-name behavior. A browser confirmed
  the update without adding a fourth step. No synthetic note was used in this test.
- artifacts/stef-quiet-live.json and its screenshots retain stream/UI evidence;
  artifacts/stef-quiet-live-feedback.json retains the real note and agent reply.
- These calls used the actual local MCP bridge because this conversation has no
  directly registered stef tools. Automatic discovery and idle chat switching in
  the real Codex/Claude panels remain unverified. The resident view stays ready
  in the existing IDE browser; no native window or extension was installed.

## Appearance and live feature development

- Added a small appearance button beside Clear with Dark, Light, Eye comfort,
  and System choices. Eye comfort uses a warm background with muted green
  accents. The page, text, inputs, arrows, and arrowheads share CSS palettes.
- The view saves its choice locally, applies it before mounting, and follows OS
  changes when System is selected. Theme changes preserve the workflow, selected
  explanation, and suggestion draft. Reload/chat/Clear persistence is verified.
- make check passes with zero Svelte/TypeScript errors or warnings. make test
  passes backend race tests, eight greeting cases, and thirteen UI tests. All
  four browser E2E tests pass, including the new appearance test. Rendered text
  meets 4.5:1 contrast and workflow arrows meet 3:1 in every tested palette.
  Picker Escape/outside dismissal, focus return, and narrow layouts are verified.
- An observer connected before implementation received six live MCP reports
  (three meaningful starts/results) and eleven UI file observations during the
  actual work. The same chat was retained, with no clearing or synthetic notes.
  The final resident engine uses the updated renderer on port 4825 in IDE mode.
- The observer reloaded the updated view, exercised all three palettes, and
  reported no JavaScript errors. artifacts/stef-theme-live.json records the real
  events; stef-theme-live-dark/light/comfort.png show the resulting workflow.
  Browser E2E screenshots also capture selected explanations and suggestion drafts.
- Agent reports used the genuine local stdio MCP bridge; this conversation's
  directly registered tool catalog still has no stef tools. This test verifies
  live feature development and the local transport, not automatic host discovery.

## Automatic saved-file revisions and complete explanations

- The UI now renders every retained whole-file watcher event, including helpers,
  entry points, tests, documentation, and repeated saves. Symbol-level events
  stay in storage without duplicate steps. Closed steps show filename plus label;
  selection shows the reason or explicitly says the agent has not supplied one.
- Exact change_id references attach explanations to the original revision and
  preserve its selection. A later save remains a separate unexplained revision.
  Wrong file/action/chat, symbol, hook, and cleared references are rejected.
- report_changes explains 1–50 revisions in a validated, atomically persisted
  batch. get_state and report receipts expose bounded pending IDs/paths plus the
  total count. An optional settle_ms delay accommodates watcher debounce without
  polling, diffs in model receipts, or an extra model call.
- MCP initialization, the portable stef skill, and project guidance now require
  explanations for every file after each edit tool and a final pending check.
  This guides cooperating connected agents; it cannot discover private intent or
  force an unconnected client to supply reasons. Direct tool discovery in this
  conversation remains unloaded; its genuine local MCP bridge is used here.
- make check and make test pass: zero Svelte/TypeScript diagnostics, backend
  race tests including actual stdio/forwarding/Streamable HTTP explanation tests,
  eight greeting cases, and sixteen UI tests. All five browser E2E tests pass.
  The updated skill passes the Skill Creator validator.
- The resident personal engine was restarted in IDE mode on port 4825. Forty-six
  previously recorded revisions from the theme task and this fix received exact
  explanations; artifacts/stef-backfill-explanations.json and its receipt retain
  that retrospective completion. These reasons were added afterwards, not claimed
  to have been present during the earlier task.
- An observer connected before two new comment edits to ui/src/theme.ts and
  ui/src/main.ts. Both appeared automatically before any explanation, and exact
  MCP batch reports filled their original steps without duplication or losing
  selection. No JavaScript errors occurred. artifacts/stef-file-live.json and
  the related screenshots retain the live observation.
- File capture covers eligible saved UTF-8 text files up to 256 KiB inside the
  chosen root, respecting ignore rules and excluding symlinks/generated output.
  Events within the 180 ms debounce window may coalesce; unsaved keystrokes are
  not filesystem events. Saved-file visibility is automatic; reasons need the
  agent's reporting contract.

## Update preferences

- A compact Updates picker offers Major milestones, Every file, and Disabled.
  Every file warns that per-file explanations may use more tokens. The default
  remains Every file, preserving the existing detailed workflow.
- The selected mode persists per project across chats and engine restarts and
  synchronizes across connected views. Changing modes preserves saved history;
  Disabled stops new activity recording and feedback delivery to the model.
- Milestones suppresses file observations and linked file explanations while
  accepting major agent steps with reasons. Suppressed reports return
  accepted:false. Watcher baselines continue advancing without suppressed diffs
  or delayed edits being replayed when Every file is re-enabled.
- MCP initialization and receipts expose the selected mode and reporting
  instructions. Agents follow changes at their next natural check-in; the
  setting does not interrupt an in-flight model turn or require polling.
- make check and make test pass: zero Svelte/TypeScript errors or warnings, Go
  vet, backend race tests, actual stdio/forwarding/Streamable HTTP mode coverage,
  eight greeting cases, and seventeen UI unit tests. All nine browser E2E tests
  pass, including actual recording suppression, no delayed backlog, shared
  preferences, preserved history, keyboard/narrow layouts, and failed saves.
  The updated portable skill passes the Skill Creator validator.
- The personal engine was restarted on port 4825 in IDE mode. A live browser
  verified all three options and the token warning without changing the user
  setting or chats, with no JavaScript errors. The production renderer is
  embedded; refreshing the existing stef tab loads the picker.
  artifacts/stef-live-update-options.png records the live menu.
- Actual automatic Codex/Claude panel discovery remains unverified in this
  conversation; real reports here use the existing local stdio MCP bridge.
  No SAP/Hyperspace files, global settings, or extension installations changed.

## Saved-file diff review

- Show diff on a selected saved-file step opens a compact dialog with the file
  path, live agent explanation, unified hunks, old/new line numbers, green
  additions, and red removals. It stays closed until requested. Source remains
  escaped text with preserved whitespace and horizontal scrolling. All palettes
  share the diff colors, and keyboard Close/Escape returns focus to Show diff.
- Diffs belong to exact observation IDs and compare consecutive observed saves,
  rather than Git HEAD or current filesystem content. New saves do not replace
  an open older diff; late agent explanations update its reason in place.
  Empty created/deleted files do not invent line changes. A mode change from
  another view closes a hidden diff without reopening it when files resumes.
- Fixed the watcher's phantom final blank line and missing-final-newline loss.
  Thirteen exact backend cases cover creation, deletion, empty/blank files,
  EOF newline changes, CRLF, and Unicode; nine cases compare hunks against actual
  git diff, including separated hunks. Earlier stored patches retain their
  original formatting; corrected captures start after the engine reload.
- make check and make test pass: Go vet, backend race checks, eight greeting
  cases, and twenty-three UI unit tests, with zero Svelte/TypeScript errors or
  warnings. All twelve browser E2E tests pass, including real saved revisions,
  line numbers, live reasons, reload, deletion, escaped source, empty files,
  focus/Escape, external mode changes, and narrow Light/Eye comfort views.
  Diff text contrast is verified at 4.5:1 or higher in those browser cases.
- Existing 16 KiB diff previews remain bounded, with an explicit truncation
  notice when incomplete. Opening or closing a diff makes no model call and
  leaves workflow history and suggestions intact.
- The final renderer is embedded in bin/stef, and the personal headless engine
  was reloaded on port 4825 in IDE mode. A live browser opened App.svelte revision
  572 and displayed the actual added effect beside its MCP-supplied reason, with
  no JavaScript errors or chat/settings changes. Closing preserved selection.
  artifacts/stef-file-diff-live.json and .png retain the live evidence; browser
  screenshots include creation/deletion and narrow theme views. Refresh the
  existing stef tab to load the updated renderer.

## GitHub source checkpoint

- Published the working prototype to https://github.com/ivandosevv/stef on its
  existing master branch. Commit bc833782db342ca02e2e53fada736f4ad7d60178
  contains the 139 tracked source files, preserving the initial commit and MIT
  license. GitHub's commit API confirmed the same hash as the local checkout.
- Reviewed the staged source and ignore rules. Credentials, local MCP/client
  configuration, runtime histories, test artifacts, generated builds, and Python
  bytecode stay outside Git. Commit identity is configured only in this personal
  repository using the owner's GitHub noreply address.
- The existing OAuth login can push source but lacks workflow permission; the
  available SSH identity cannot write to this repository. The full CI definition
  is preserved byte-for-byte in packaging/github-actions/check.yml as a template,
  with activation instructions beside it. Hosted CI has not run.
- No functional source changed for this checkpoint. The preceding backend race
  checks, twenty-three UI unit tests, and twelve browser tests remain its local
  validation. The resident personal engine remains healthy on port 4825.
- AGENTS.md records the user's request to commit and push completed, verified
  milestones as development continues. No background scheduler was installed.

## Adaptive reporting and interactive design

- Design now offers Minimal and Interactive as independent local presentation
  choices. Interactive shows the selected saved revision's before/after hunks
  and line numbers, or a milestone's action, state, explanation, and reported
  parent paths. Design, selection, history, hunk navigation, and Show diff
  neither request agent output nor change the reporting mode.
- Each actual Updates change starts a new inclusive updates_from boundary.
  No-op changes preserve it. Recovery derives the boundary from the journal;
  current pending files and compact model activity exclude earlier windows.
  Stale or mixed-window automatic explanation batches are rejected atomically.
  Earlier work remains available through an explicit saved-history control.
- Explain more alone queues an exact retained-step question with a token warning.
  A matching context_id reply updates that step's explanation without replacing
  captured code, creating another workflow item, or changing claims. Historical
  context works in files and milestones; Disabled still suppresses reporting.
  Requested file context satisfies only that exact revision's pending reason.
- make check and make test pass: Go vet, backend race tests, eight greeting
  cases, and thirty-four UI unit tests, with zero Svelte/TypeScript errors or
  warnings. All fifteen browser E2E scenarios pass, including future-only
  reporting across reloads, mode suppression, history restoration, explicit
  context, exact escaped source, narrow Light/Eye comfort layouts, and the
  optional embedded host's file-open action in both designs. The portable skill
  also passes the Skill Creator validator.
- Independent source review found no remaining reporting/context integration
  defects. Light and Eye comfort screenshots were visually inspected. The
  production renderer is embedded and the verified personal engine reloaded
  headlessly on 4825, retaining the existing chat and Every file preference.
- A live browser inspected actual daemon/hub/changes.go revision 748 and its
  MCP reason, historical ChangeVisual.svelte revision 711, and real verification
  milestone 758. Design switches, saved-history access, diff opening, and reload
  left event IDs, chat, mode, and reporting boundary unchanged; no mutation
  requests or JavaScript errors occurred. artifacts/stef-adaptive-live.json and
  its screenshots retain that evidence. No synthetic feedback was submitted
  to the live chat; the explicit context path is covered in isolated E2E tests.
- Mode changes reach a cooperating agent at its next check-in. These checks do
  not certify automatic tool discovery or chat-switch signals in arbitrary
  IDE agents. This conversation continues to use the genuine local stdio MCP
  bridge. Token costs remain unmeasured; local visuals call no model.
