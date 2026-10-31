<script lang="ts">
  import type { SavedComparison } from '../interaction';

  let { comparison, oninspect }: { comparison: SavedComparison; oninspect?: () => void } = $props();
  let side = $state<'before' | 'after'>('after');
  const sourceSides = ['before', 'after'] as const;
  const lines = $derived(side === 'before' ? comparison.beforeLines : comparison.afterLines);
  const noNewline = $derived(side === 'before' ? comparison.beforeNoNewline : comparison.afterNoNewline);
  const lineRange = $derived(lines.length === 1 ? 'Line ' + lines[0] : lines.length ? 'Lines ' + lines[0] + ' to ' + lines.at(-1) : 'No source lines on this side.');
  function choose(value: 'before' | 'after') { side = value; oninspect?.(); }
</script>

<section class="change-lens" aria-label="Saved source comparison" data-testid="change-lens" data-side={side}>
  <div class="lens-heading"><span>Saved change</span><div role="group" aria-label="Compare saved source"><button type="button" aria-pressed={side === 'before'} onclick={() => choose('before')}>Before</button><button type="button" aria-pressed={side === 'after'} onclick={() => choose('after')}>After</button></div></div>
  <p class="line-range" aria-live="polite">{lineRange}{noNewline ? ' · No newline at end of file' : ''}</p>
  <div class="source-stage">
    {#each sourceSides as value}
      <pre class:active={side === value} aria-hidden={side !== value} data-testid={'comparison-' + value}><code>{#if (value === 'before' ? comparison.beforeLines : comparison.afterLines).length}{comparison.prefix}<mark data-side={value}>{comparison[value]}</mark>{comparison.suffix}{:else}<span class="empty-side">No source lines on this side.</span>{/if}</code></pre>
    {/each}
  </div>
  <p class="lens-help">Exact saved block. Highlight marks changed text.</p>
</section>

<style>
  .change-lens { margin-top: 10px; padding: 10px; border: 1px solid var(--border); border-radius: 5px; background: var(--surface); }
  .lens-heading { display: flex; align-items: center; justify-content: space-between; gap: 8px; color: var(--muted); font-size: 10px; }
  .lens-heading > div { display: flex; gap: 3px; }
  button { padding: 4px 6px; border: 0; border-radius: 4px; color: var(--muted); background: none; font: inherit; }
  button[aria-pressed="true"] { color: var(--text); background: var(--hover); }
  button:focus-visible { outline: 1px solid var(--focus); outline-offset: 2px; }
  .line-range, .lens-help { margin: 7px 0; color: var(--faint); font-size: 10px; overflow-wrap: anywhere; }
  .lens-help { margin-bottom: 0; }
  .source-stage { display: grid; min-width: 0; padding: 8px; border-radius: 4px; background: var(--hover); }
  pre { grid-area: 1 / 1; min-width: 0; margin: 0; opacity: 0; pointer-events: none; user-select: none; color: var(--text); font: 11px/1.7 ui-monospace, SFMono-Regular, Menlo, Consolas, monospace; transition: opacity .16s ease; }
  pre.active { opacity: 1; pointer-events: auto; user-select: text; }
  code { white-space: break-spaces; overflow-wrap: anywhere; font: inherit; tab-size: 4; }
  mark { color: inherit; border-radius: 2px; padding: 1px 0; box-decoration-break: clone; }
  mark[data-side="before"] { color: var(--diff-remove); background: var(--diff-remove-bg); }
  mark[data-side="after"] { color: var(--diff-add); background: var(--diff-add-bg); }
  .empty-side { color: var(--muted); font: 11px/1.7 system-ui, sans-serif; }
  @media (prefers-reduced-motion: reduce) { pre { transition: none; } }
</style>
