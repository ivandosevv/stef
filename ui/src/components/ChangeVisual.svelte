<script lang="ts">
  import { isFileChange, stepReason, type AgentEvent } from '../lib';
  import { diffHunks, isTruncatedDiff, type DiffLine } from '../diff';
  import { effectValue, lineKey, lineLabel, lineSuggestion, savedComparison, selectableLine, styleEffects, visibleLines, type DiffFilter } from '../interaction';
  import { stepAction, reportedParents } from '../visual';
  import DiffLines from './DiffLines.svelte';
  import ChangeLens from './ChangeLens.svelte';

  let { step, steps, onselect, ondiff, onexplain, connected, asking, requestStatus, oninspect, onsuggest }: {
    step: AgentEvent;
    steps: AgentEvent[];
    onselect: (id: string) => void;
    ondiff: () => void;
    onexplain: () => void;
    connected: boolean;
    asking: boolean;
    requestStatus: string;
    oninspect?: () => void;
    onsuggest?: (text: string) => void;
  } = $props();

  const file = $derived(isFileChange(step));
  const hunks = $derived(diffHunks(step.diff ?? ''));
  const truncated = $derived(isTruncatedDiff(step.diff ?? ''));
  const parents = $derived(reportedParents(step, steps));
  const reason = $derived(stepReason(step));
  const observed = $derived(step.source === 'hook' && !step.explanation && !step.why);
  const nodeLabel = $derived(step.node_name === '@workflow' ? 'Current task' : step.node_name);
  let selectedHunk = $state({ stepId: '', index: 0 });
  const hunkIndex = $derived(selectedHunk.stepId === step.id ? Math.min(selectedHunk.index, Math.max(0, hunks.length - 1)) : 0);
  const hunk = $derived(hunks[hunkIndex]);
  let filterChoice = $state<{ stepId: string; value: DiffFilter }>({ stepId: '', value: 'all' });
  const filter = $derived(filterChoice.stepId === step.id ? filterChoice.value : 'all');
  const shownLines = $derived(visibleLines(hunk ?? [], filter));
  let selection = $state<{ stepId: string; hunk: number; key: string } | null>(null);
  const selectedLine = $derived(selection?.stepId === step.id && selection.hunk === hunkIndex ? hunk?.find(line => lineKey(line) === selection?.key) : undefined);
  const comparison = $derived(selectedLine && hunk ? savedComparison(hunk, selectedLine) : undefined);
  const comparisonKey = $derived(selectedLine ? step.id + ':' + hunkIndex + ':' + lineKey(selectedLine) : '');
  let openedComparison = $state('');
  const effects = $derived(file ? styleEffects(step.diff ?? '', step.node_name) : []);
  let preview = $state({ stepId: '', index: 0, position: 100, scrubbing: false });
  const effectIndex = $derived(preview.stepId === step.id ? Math.min(preview.index, Math.max(0, effects.length - 1)) : 0);
  const effectPosition = $derived(preview.stepId === step.id ? preview.position : 100);
  const effect = $derived(effects[effectIndex]);
  const filters: { value: DiffFilter; label: string; accessible: string }[] = [
    { value: 'all', label: 'All', accessible: 'Show all lines' }, { value: 'changes', label: 'Changes', accessible: 'Show changed lines' },
    { value: 'addition', label: 'Added', accessible: 'Show added lines' }, { value: 'removal', label: 'Removed', accessible: 'Show removed lines' },
  ];
  const emptyMessage = $derived(step.action === 'created' ? 'No line changes. Empty file created.' : step.action === 'deleted' ? 'No line changes. Empty file deleted.' : 'No line changes were captured for this save.');

  function chooseHunk(index: number) {
    selectedHunk = { stepId: step.id, index: Math.min(Math.max(0, index), hunks.length - 1) };
    selection = null; oninspect?.();
  }
  function moveHunk(amount: number) { chooseHunk(hunkIndex + amount); }
  function changeFilter(value: DiffFilter) { filterChoice = { stepId: step.id, value }; selection = null; oninspect?.(); }
  function chooseLine(line: DiffLine) { selection = { stepId: step.id, hunk: hunkIndex, key: lineKey(line) }; oninspect?.(); }
  function changeEffect(position: number, index = effectIndex, scrubbing = false) { preview = { stepId: step.id, index, position, scrubbing }; oninspect?.(); }
  function suggestLine() {
    if (selectedLine) onsuggest?.(lineSuggestion(step.node_name, step.reference_id ?? step.id, selectedLine));
  }
</script>

<section class="change-visual" aria-label="Selected change" data-testid="change-visual" data-visual-kind={file ? 'file' : step.source === 'hook' ? 'activity' : step.action === 'decided' ? 'decision' : 'milestone'}>
  <header class="visual-heading">
    <div class="action-state"><span class="action">{stepAction(step)}</span><span class="state" data-state={step.status}><span class="state-dot" aria-hidden="true"></span>{step.status === 'running' ? 'Running' : 'Ready'}</span></div>
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
        {#if step.action === 'decided'}<svg viewBox="0 0 64 64"><path d="M32 8v18m0 0L15 40m17-14 17 14M9 40h12v15H9zM43 40h12v15H43z" /></svg>
        {:else if step.action === 'scouted'}<svg viewBox="0 0 64 64"><circle cx="27" cy="27" r="15" /><path d="m38 38 14 14M20 27h14m-7-7v14" /></svg>
        {:else if step.action === 'tested'}<svg viewBox="0 0 64 64"><path d="M24 9h16M27 9v18L14 47a5 5 0 0 0 4 8h28a5 5 0 0 0 4-8L37 27V9M22 36h20" /><circle cx="28" cy="44" r="1" /><circle cx="37" cy="48" r="1" /></svg>
        {:else}<svg viewBox="0 0 64 64"><path d="m21 20-12 12 12 12m22-24 12 12-12 12M37 15 27 49" /></svg>{/if}
      </div>
    {/if}
  {/if}

  <div class="visual-reason" aria-label="Why this change">
    <span class="section-label">{observed ? 'Observed' : 'Why'}</span><p>{reason}</p>
  </div>

  {#if effect}
    <section class="effect-preview" aria-label="Style effect preview" data-testid="style-effect">
      <div class="effect-heading"><span class="section-label">Style sample</span>
        {#if effects.length > 1}<select aria-label="Preview style property" value={effectIndex} onchange={event => changeEffect(100, Number(event.currentTarget.value))}>{#each effects as candidate, index (index)}<option value={index}>{candidate.sourceProperty}{effects.filter(item => item.sourceProperty === candidate.sourceProperty).length > 1 ? ' · ' + candidate.selector : ''}</option>{/each}</select>
        {:else}<span class="section-label">{effect.sourceProperty}</span>{/if}
      </div>
      <p class="effect-scope"><code>{effect.selector}</code></p>
      <div class="effect-stage"><button type="button" class="effect-target" class:dimension={effect.property === 'height' || effect.property === 'width'} class:scrubbing={preview.stepId === step.id && preview.scrubbing} data-testid="style-effect-target" style={effect.property + ':' + effectValue(effect, effectPosition)} onclick={() => oninspect?.()}><span aria-hidden="true">●</span><span>Preview</span></button></div>
      <div class="effect-controls"><button type="button" aria-label="Before style" aria-pressed={effectPosition === 0} onclick={() => changeEffect(0)}>Before</button><input type="range" aria-label="Style effect position" min="0" max="100" value={effectPosition} oninput={event => changeEffect(Number(event.currentTarget.value), effectIndex, true)} /><button type="button" aria-label="After style" aria-pressed={effectPosition === 100} onclick={() => changeEffect(100)}>After</button></div>
      <div class="effect-values"><code>{effect.before}</code><span aria-hidden="true">→</span><code>{effect.after}</code></div>
      <p class="sample-help">{effect.sourceProperty.startsWith('--') ? 'Color value sample. Usage in the app is not inferred.' : 'This sample previews the changed style, not the running app.'}</p>
    </section>
  {/if}

  {#if file}
    <div class="saved-change">
      {#if truncated}<p class="notice" data-testid="visual-truncation">Preview truncated. Only the captured portion is available.</p>{/if}
      {#if hunk}
        <section class="change-map" aria-label="Change map">
          {#each hunks as lines, index (index)}
            {@const added = lines.filter(line => line.kind === 'addition').length}
            {@const removed = lines.filter(line => line.kind === 'removal').length}
            <button type="button" class="map-hunk" class:active={hunkIndex === index} data-testid="hunk-map" aria-label={'Explore hunk ' + (index + 1) + ': ' + removed + ' removed, ' + added + ' added'} aria-pressed={hunkIndex === index} title={lines[0]?.text} onclick={() => chooseHunk(index)}>
              <span class="map-counts"><span>{index + 1}</span><span class="removed">−{removed}</span><span class="added">+{added}</span></span>
              <span class="map-bars" aria-hidden="true"><span class="removed" style={'flex-grow:' + removed}></span><span class="added" style={'flex-grow:' + added}></span></span>
            </button>
          {/each}
        </section>
        {#if hunks.length > 1}
          <div class="hunk-controls">
            <span class="section-label" aria-live="polite">Hunk {hunkIndex + 1} of {hunks.length}</span>
            <div class="hunk-buttons"><button aria-label="Previous hunk" disabled={hunkIndex === 0} onclick={() => moveHunk(-1)}>←</button><button aria-label="Next hunk" disabled={hunkIndex >= hunks.length - 1} onclick={() => moveHunk(1)}>→</button></div>
          </div>
        {/if}
        <details class="filter-tools"><summary onclick={() => oninspect?.()}>Filter source</summary><div class="diff-filters" role="group" aria-label="Visible diff lines">{#each filters as item (item.value)}<button type="button" aria-label={item.accessible} aria-pressed={filter === item.value} onclick={() => changeFilter(item.value)}>{item.label}</button>{/each}</div></details>
        {#key step.id + ':' + hunkIndex}
        <!-- The selected-change pane owns scrolling; source rows support keyboard selection. -->
        <!-- svelte-ignore a11y_no_noninteractive_tabindex -->
        <div class="code-diff" role="region" aria-label="Code diff" tabindex="0">
          <DiffLines lines={shownLines} wrap testId="visual-line" onselect={chooseLine} selected={selectedLine ? lineKey(selectedLine) : null} />
          {#if !shownLines.some(selectableLine)}<p class="filter-empty">No {filter === 'addition' ? 'added' : filter === 'removal' ? 'removed' : 'changed'} lines in this section.</p>{/if}
        </div>
        {/key}
        {#if selectedLine}<div class="line-selection" aria-label="Selected line"><span>{lineLabel(selectedLine)}</span><div>{#if comparison}<button type="button" class="text-button" aria-expanded={openedComparison === comparisonKey} onclick={() => { openedComparison = openedComparison === comparisonKey ? '' : comparisonKey; oninspect?.(); }}>Compare line</button>{/if}{#if onsuggest}<button type="button" class="text-button" onclick={suggestLine}>Suggest about this line</button>{/if}<button type="button" class="text-button" aria-label="Clear line selection" onclick={() => { selection = null; oninspect?.(); }}>×</button></div></div>{/if}
        {#if comparison && openedComparison === comparisonKey}{#key comparisonKey}<ChangeLens {comparison} {oninspect} />{/key}{/if}
      {:else}<p class="notice">{step.diff?.startsWith('Diff unavailable') ? step.diff : step.diff ? 'No line preview is available for this saved revision.' : emptyMessage}</p>{/if}
      <button class="text-button full-diff" onclick={ondiff}>Show diff</button>
    </div>
  {/if}

  <footer class="explain-request">
    <button class="text-button" onclick={onexplain} disabled={!connected || asking}>{asking ? 'Request queued' : step.explanation_pending ? 'Request explanation' : 'Explain more'}</button>
    <p class="request-help">{step.explanation_pending ? 'The agent receives your request at its next check-in.' : 'Extra AI context may use tokens. The agent receives your request at its next check-in.'}</p>
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
  .code-diff { min-width: 0; overflow-x: clip; padding: 5px 0; border: 1px solid var(--border); border-radius: 5px; background: var(--surface); font: 11px/1.7 ui-monospace, SFMono-Regular, Menlo, Consolas, monospace; }
  .effect-preview { margin-top: 20px; padding: 12px; border: 1px solid var(--border); border-radius: 6px; background: var(--surface); }
  .effect-heading, .effect-values { display: flex; align-items: center; justify-content: space-between; gap: 8px; }
  .effect-heading select { max-width: 60%; border: 0; background: var(--surface); color: var(--muted); font: inherit; font-size: 10px; }
  .effect-scope { margin: 5px 0 0; color: var(--faint); font-size: 10px; overflow-wrap: anywhere; }
  .effect-stage { display: grid; place-items: center; min-height: 100px; padding: 10px; overflow: hidden; }
  .effect-target { display: inline-flex; align-items: center; justify-content: center; gap: 6px; width: max-content; max-width: 100%; padding: 8px; border: 1px solid var(--border); border-radius: 5px; background: var(--hover); color: var(--text); font: inherit; transition: box-shadow .15s, color .18s, background-color .18s, padding .18s, gap .18s, border-radius .18s, border-width .18s, font-size .18s, width .18s, height .18s, opacity .18s; }
  .effect-target.dimension { padding: 0; border-width: 0; overflow: hidden; }
  .effect-target.scrubbing { transition: none; }
  .effect-target:hover { box-shadow: 0 2px 12px #0002; }
  .effect-controls { display: grid; grid-template-columns: auto minmax(0, 1fr) auto; align-items: center; gap: 8px; }
  .effect-controls button, .diff-filters button { border: 0; border-radius: 4px; padding: 4px 6px; color: var(--muted); background: none; font: inherit; font-size: 10px; }
  .effect-controls button[aria-pressed="true"], .diff-filters button[aria-pressed="true"] { color: var(--text); background: var(--hover); }
  .effect-controls input { min-width: 0; width: 100%; accent-color: var(--control); }
  .effect-values { margin-top: 6px; color: var(--muted); font-size: 10px; }
  .effect-values code { overflow-wrap: anywhere; }
  .sample-help { margin: 8px 0 0; color: var(--faint); font-size: 10px; }
  .change-map { display: flex; gap: 5px; flex-wrap: wrap; margin-bottom: 10px; }
  .map-hunk { flex: 1 1 58px; min-width: 0; max-width: 120px; padding: 6px; border: 1px solid var(--border); border-radius: 4px; background: var(--surface); color: var(--muted); font: inherit; font-size: 10px; }
  .map-hunk.active { border-color: var(--selected-border); background: var(--selected); }
  .map-counts { display: flex; gap: 6px; align-items: center; justify-content: space-between; }
  .map-counts .removed { color: var(--diff-remove); }
  .map-counts .added { color: var(--diff-add); }
  .map-bars { display: flex; height: 3px; margin-top: 5px; gap: 2px; }
  .map-bars .removed { background: var(--diff-remove); }
  .map-bars .added { background: var(--diff-add); }
  .filter-tools { margin-bottom: 8px; color: var(--muted); font-size: 10px; }
  .filter-tools summary { cursor: pointer; }
  .diff-filters { display: flex; gap: 3px; margin-top: 6px; }
  .line-selection { display: flex; align-items: center; justify-content: space-between; flex-wrap: wrap; gap: 6px; margin-top: 8px; color: var(--muted); font-size: 10px; }
  .line-selection > div { display: flex; gap: 4px; }
  .filter-empty { margin: 6px 10px; color: var(--muted); font-size: 11px; }
  .notice { margin: 0 0 8px; color: var(--muted); font-size: 11px; }
  .text-button { padding: 5px 8px; border: 1px solid var(--border); border-radius: 4px; background: var(--surface); color: var(--control); font: inherit; font-size: 11px; }
  .text-button:hover:not(:disabled) { background: var(--hover); color: var(--text); }
  .full-diff { margin-top: 10px; }
  .explain-request { margin-top: 24px; padding-top: 12px; border-top: 1px solid var(--border); }
  .request-help { max-width: 42ch; margin: 7px 0 0; color: var(--muted); font-size: 10px; line-height: 1.5; }
  .request-status { margin: 7px 0 0; color: var(--muted); font-size: 11px; }
  button:disabled { opacity: .5; cursor: default; }
  button:focus-visible, .code-diff:focus-visible { outline: 1px solid var(--focus); outline-offset: 2px; }
  .code-diff:focus-visible { outline-offset: -2px; }
  @keyframes breathe { 50% { opacity: .3; } }
  @media (prefers-reduced-motion: reduce) { .state[data-state="running"] .state-dot { animation: none; } .effect-target { transition: none; } }
  @media (max-width: 440px) { .change-visual { padding: 16px; } .path-row { grid-template-columns: minmax(0, 1fr); gap: 4px; } .path-arrow { transform: rotate(90deg); justify-self: center; } }
</style>
