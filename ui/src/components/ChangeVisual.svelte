<script lang="ts">
  import { isFileChange, type AgentEvent } from '../lib';
  import { isTruncatedDiff } from '../diff';
  import { milestoneAction, reportedParents, visualHunks } from '../visual';

  let { step, steps, onselect, ondiff, onexplain, connected, asking, requestStatus }: {
    step: AgentEvent;
    steps: AgentEvent[];
    onselect: (id: string) => void;
    ondiff: () => void;
    onexplain: () => void;
    connected: boolean;
    asking: boolean;
    requestStatus: string;
  } = $props();

  const file = $derived(isFileChange(step));
  const hunks = $derived(visualHunks(step.diff ?? ''));
  const truncated = $derived(isTruncatedDiff(step.diff ?? ''));
  const parents = $derived(reportedParents(step, steps));
  const reason = $derived(step.explanation ?? step.why);
  const nodeLabel = $derived(step.node_name === '@workflow' ? 'Current task' : step.node_name);
  let selectedHunk = $state({ stepId: '', index: 0 });
  const hunkIndex = $derived(selectedHunk.stepId === step.id ? Math.min(selectedHunk.index, Math.max(0, hunks.length - 1)) : 0);
  const hunk = $derived(hunks[hunkIndex]);
  const emptyMessage = $derived(step.action === 'created' ? 'No line changes. Empty file created.' : step.action === 'deleted' ? 'No line changes. Empty file deleted.' : 'No line changes were captured for this save.');

  function moveHunk(amount: number) {
    selectedHunk = { stepId: step.id, index: Math.min(Math.max(0, hunkIndex + amount), hunks.length - 1) };
  }
</script>

<section class="change-visual" aria-label="Selected change" data-testid="change-visual" data-visual-kind={file ? 'file' : 'milestone'}>
  <header class="visual-heading">
    <div class="action-state"><span class="action">{milestoneAction(step.action)}</span><span class="state" data-state={step.status}><span class="state-dot" aria-hidden="true"></span>{step.status === 'running' ? 'Running' : 'Ready'}</span></div>
    <h2 title={nodeLabel}>{nodeLabel}</h2>
    {#if !file}<p class="summary">{step.summary}</p>{/if}
  </header>

  {#if !file}
    {#if parents.length}
      <div class="reported-path" aria-label="Reported connections">
        <span class="section-label">Reported path</span>
        <div class="path-row">
          <div class="parent-paths">
            {#each parents as parent (parent.path)}
              {#if parent.stepId}<button class="path-node parent-node" onclick={() => onselect(parent.stepId!)} title={parent.path}>{parent.path}</button>
              {:else}<span class="path-node parent-node" title={parent.path}>{parent.path}</span>{/if}
            {/each}
          </div>
          <svg class="path-arrow" viewBox="0 0 42 24" aria-hidden="true"><path d="M2 12h35m-7-7 7 7-7 7" /></svg>
          <span class="path-node selected-node" title={nodeLabel}>{nodeLabel}</span>
        </div>
      </div>
    {:else}
      <div class="milestone-symbol" aria-hidden="true">
        {#if step.action === 'scouted'}<svg viewBox="0 0 64 64"><circle cx="27" cy="27" r="15" /><path d="m38 38 14 14M20 27h14m-7-7v14" /></svg>
        {:else if step.action === 'tested'}<svg viewBox="0 0 64 64"><path d="M24 9h16M27 9v18L14 47a5 5 0 0 0 4 8h28a5 5 0 0 0 4-8L37 27V9M22 36h20" /><circle cx="28" cy="44" r="1" /><circle cx="37" cy="48" r="1" /></svg>
        {:else}<svg viewBox="0 0 64 64"><path d="m21 20-12 12 12 12m22-24 12 12-12 12M37 15 27 49" /></svg>{/if}
      </div>
    {/if}
  {/if}

  <div class="visual-reason" aria-label="Why this change">
    <span class="section-label">Why</span><p>{reason || (step.explanation_pending ? 'Waiting for the agent’s explanation.' : 'No explanation was supplied for this step.')}</p>
  </div>

  {#if file}
    <div class="saved-change">
      {#if truncated}<p class="notice" data-testid="visual-truncation">Preview truncated. Only the captured portion is available.</p>{/if}
      {#if hunk}
        <div class="hunk-controls">
          <span class="section-label" aria-live="polite">Hunk {hunkIndex + 1} of {hunks.length}</span>
          <div class="hunk-buttons"><button aria-label="Previous hunk" disabled={hunkIndex === 0} onclick={() => moveHunk(-1)}>←</button><button aria-label="Next hunk" disabled={hunkIndex >= hunks.length - 1} onclick={() => moveHunk(1)}>→</button></div>
        </div>
        <code class="hunk-range">{hunk.header}</code>
        <div class="code-comparison">
          {#each [{ label: 'Before', lines: hunk.before }, { label: 'After', lines: hunk.after }] as side (side.label)}
            <section class="code-side" aria-label={side.label}>
              <h3>{side.label}</h3>
              <!-- Saved source may extend horizontally; focus enables keyboard scrolling. -->
              <!-- svelte-ignore a11y_no_noninteractive_tabindex -->
              <div class="code-scroll" role="region" aria-label={side.label + ' code'} tabindex="0">
                {#if side.lines.length}
                  {#each side.lines as line, index (index)}
                    <div class="source-line" data-testid="visual-line" data-kind={line.kind}><span class="line-number">{line.line ?? ''}</span><span class="line-prefix" aria-hidden="true">{line.kind === 'addition' ? '+' : line.kind === 'removal' ? '-' : ''}</span><code>{line.text}</code></div>
                  {/each}
                {:else}<p class="empty-side">No lines in this hunk.</p>{/if}
              </div>
            </section>
            {#if side.label === 'Before'}<svg class="comparison-arrow" viewBox="0 0 24 24" aria-hidden="true"><path d="M3 12h17m-6-6 6 6-6 6" /></svg>{/if}
          {/each}
        </div>
      {:else}<p class="notice">{step.diff ? 'No line preview is available for this saved revision.' : emptyMessage}</p>{/if}
      <button class="text-button full-diff" onclick={ondiff}>Show diff</button>
    </div>
  {/if}

  <footer class="explain-request">
    <button class="text-button" onclick={onexplain} disabled={!connected || asking}>{asking ? 'Request queued' : 'Explain more'}</button>
    <p class="request-help">Extra AI context may use tokens. The agent receives your request at its next check-in.</p>
    {#if requestStatus}<p class="request-status" role="status">{requestStatus}</p>{/if}
  </footer>
</section>

<style>
  .change-visual { min-width: 0; padding: 20px; color: var(--text); font-size: 12px; line-height: 1.5; }
  .visual-heading { display: block; height: auto; margin: 0 0 20px; }
  .action-state { display: flex; align-items: center; gap: 10px; margin-bottom: 8px; }
  .action { font-size: 10px; font-weight: 600; letter-spacing: .08em; text-transform: uppercase; color: var(--control); }
  .state { display: inline-flex; align-items: center; gap: 5px; color: var(--muted); font-size: 10px; }
  .state-dot { width: 5px; height: 5px; border-radius: 50%; background: var(--control); }
  .state[data-state="running"] .state-dot { animation: breathe 1.8s ease-in-out infinite; }
  h2 { margin: 0; font-size: 15px; font-weight: 500; overflow-wrap: anywhere; }
  .summary { margin: 6px 0 0; color: var(--muted); }
  .section-label { color: var(--muted); font-size: 10px; letter-spacing: .04em; }
  .reported-path { margin: 0 0 20px; }
  .path-row { display: grid; grid-template-columns: minmax(0, 1fr) 32px minmax(0, 1fr); align-items: center; gap: 6px; margin-top: 8px; }
  .parent-paths { display: grid; gap: 5px; min-width: 0; }
  .path-node { display: block; min-width: 0; text-align: left; padding: 9px 10px; border: 1px solid var(--border); border-radius: 5px; background: var(--surface); color: var(--muted); font: inherit; overflow-wrap: anywhere; }
  button.parent-node:hover { background: var(--hover); color: var(--text); }
  .selected-node { background: var(--selected); border-color: var(--selected-border); color: var(--text); }
  .path-arrow { width: 32px; height: 24px; fill: none; stroke: var(--control); stroke-width: 1.5; stroke-linecap: round; stroke-linejoin: round; }
  .milestone-symbol { display: grid; place-items: center; height: 96px; margin: 0 0 20px; border-radius: 6px; background: var(--hover); }
  .milestone-symbol svg { width: 52px; height: 52px; fill: none; stroke: var(--control); stroke-width: 1.5; stroke-linecap: round; stroke-linejoin: round; }
  .visual-reason p { margin: 4px 0 0; overflow-wrap: anywhere; white-space: pre-wrap; }
  .saved-change { margin-top: 20px; }
  .hunk-controls { display: flex; align-items: center; justify-content: space-between; gap: 8px; margin-bottom: 5px; }
  .hunk-buttons { display: flex; gap: 4px; }
  .hunk-buttons button { width: 26px; height: 24px; padding: 0; border: 1px solid var(--border); border-radius: 4px; background: var(--surface); color: var(--control); font: inherit; }
  .hunk-buttons button:hover:not(:disabled) { background: var(--hover); color: var(--text); }
  .hunk-range { display: block; margin-bottom: 8px; color: var(--muted); font: 10px/1.5 ui-monospace, SFMono-Regular, Menlo, Consolas, monospace; overflow-wrap: anywhere; }
  .code-comparison { display: grid; grid-template-columns: minmax(0, 1fr) 20px minmax(0, 1fr); gap: 6px; align-items: start; }
  .comparison-arrow { width: 20px; height: 24px; margin-top: 3px; fill: none; stroke: var(--control); stroke-width: 1.5; stroke-linecap: round; stroke-linejoin: round; }
  .code-side { min-width: 0; overflow: hidden; border: 1px solid var(--border); border-radius: 5px; background: var(--surface); }
  h3 { margin: 0; padding: 6px 8px; border-bottom: 1px solid var(--border); font-size: 10px; font-weight: 500; color: var(--muted); }
  .code-scroll { max-height: 18rem; overflow: auto; padding: 5px 0; font: 10px/1.8 ui-monospace, SFMono-Regular, Menlo, Consolas, monospace; }
  .source-line { display: grid; grid-template-columns: 32px 14px 1fr; width: max-content; min-width: 100%; padding-right: 10px; }
  .line-number { text-align: right; padding-right: 5px; color: var(--muted); user-select: none; }
  .line-prefix { text-align: center; }
  .source-line code { font: inherit; white-space: pre; tab-size: 4; }
  .source-line[data-kind="addition"] { background: var(--diff-add-bg); color: var(--diff-add); }
  .source-line[data-kind="removal"] { background: var(--diff-remove-bg); color: var(--diff-remove); }
  .source-line[data-kind="meta"] { color: var(--muted); }
  .empty-side { margin: 0; padding: 4px 8px; color: var(--muted); font: 11px/1.6 system-ui, sans-serif; }
  .notice { margin: 0 0 8px; color: var(--muted); font-size: 11px; }
  .text-button { padding: 5px 8px; border: 1px solid var(--border); border-radius: 4px; background: var(--surface); color: var(--control); font: inherit; font-size: 11px; }
  .text-button:hover:not(:disabled) { background: var(--hover); color: var(--text); }
  .full-diff { margin-top: 10px; }
  .explain-request { margin-top: 24px; padding-top: 12px; border-top: 1px solid var(--border); }
  .request-help { max-width: 42ch; margin: 7px 0 0; color: var(--muted); font-size: 10px; line-height: 1.5; }
  .request-status { margin: 7px 0 0; color: var(--muted); font-size: 11px; }
  button:disabled { opacity: .5; cursor: default; }
  button:focus-visible, .code-scroll:focus-visible { outline: 1px solid var(--focus); outline-offset: 2px; }
  .code-scroll:focus-visible { outline-offset: -2px; }
  @keyframes breathe { 50% { opacity: .3; } }
  @media (prefers-reduced-motion: reduce) { .state[data-state="running"] .state-dot { animation: none; } }
  @media (max-width: 440px) { .change-visual { padding: 16px; } .code-comparison { grid-template-columns: minmax(0, 1fr); } .comparison-arrow { transform: rotate(90deg); justify-self: center; margin: 0; } .path-row { grid-template-columns: minmax(0, 1fr); gap: 4px; } .path-arrow { transform: rotate(90deg); justify-self: center; } }
</style>
