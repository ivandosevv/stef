<script lang="ts">
  import { onMount } from 'svelte';
  import { diffLines, isTruncatedDiff } from '../diff';
  import type { AgentEvent } from '../lib';
  let { step, onclose }: { step: AgentEvent; onclose: () => void } = $props();
  let dialog: HTMLDialogElement;
  const lines = $derived(diffLines(step.diff ?? ''));
  const truncated = $derived(isTruncatedDiff(step.diff ?? ''));
  const emptyMessage = $derived(step.action === 'created' ? 'No line changes. Empty file created.' : step.action === 'deleted' ? 'No line changes. Empty file deleted.' : 'No line changes were captured for this save.');
  onMount(() => {
    const opener = document.activeElement;
    dialog.showModal();
    return () => { dialog.close(); if (opener instanceof HTMLElement && opener.isConnected) opener.focus(); };
  });
</script>

<dialog bind:this={dialog} aria-label="File diff" oncancel={event => { event.preventDefault(); onclose(); }} onkeydown={event => { if (event.key === 'Escape') event.stopPropagation(); }}>
  <div class="diff-header"><h2 title={step.node_name}>{step.node_name}</h2><button aria-label="Close diff" onclick={onclose}>×</button></div>
  <section class="diff-reason" aria-label="Why this change"><p>{step.why || 'Waiting for the agent’s explanation.'}</p></section>
  {#if truncated}<p class="diff-notice">Diff preview truncated. Only the captured portion is available.</p>{/if}
  {#if lines.length}
    <!-- Keyboard users need to focus this region to scroll long source lines. -->
    <!-- svelte-ignore a11y_no_noninteractive_tabindex -->
    <div class="diff-scroll" role="region" aria-label="Code diff" tabindex="0">
      {#each lines as line, index (index)}
        <div class="diff-line" data-testid="diff-line" data-kind={line.kind}>
          <span class="line-number" aria-hidden="true">{line.oldLine ?? ''}</span><span class="line-number" aria-hidden="true">{line.newLine ?? ''}</span><span class="diff-prefix">{line.prefix}</span><code class="diff-code">{line.text}</code>
        </div>
      {/each}
    </div>
  {:else}<p class="diff-empty">{emptyMessage}</p>{/if}
</dialog>

<style>
  dialog { width: min(960px, calc(100vw - 24px)); max-height: min(760px, calc(100dvh - 24px)); padding: 0; border: 1px solid var(--border); border-radius: 8px; background: var(--surface); color: var(--text); overflow: hidden; }
  dialog[open] { display: flex; flex-direction: column; }
  dialog::backdrop { background: #0007; }
  .diff-header { display: flex; align-items: center; gap: 12px; padding: 12px 16px 4px; flex-shrink: 0; }
  h2 { min-width: 0; flex: 1; font-size: 12px; font-weight: 500; margin: 0; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  button { border: 0; background: none; color: var(--control); padding: 4px 6px; font-size: 20px; border-radius: 4px; }
  button:hover { color: var(--text); background: var(--hover); }
  .diff-reason { padding: 0 16px 12px; flex-shrink: 0; }
  .diff-reason p, .diff-notice, .diff-empty { margin: 0; font-size: 12px; line-height: 1.5; color: var(--muted); }
  .diff-notice, .diff-empty { padding: 0 16px 12px; }
  .diff-notice { flex-shrink: 0; }
  .diff-scroll { overflow: auto; min-height: 0; border-top: 1px solid var(--border); padding: 6px 0; font: 11px/1.7 ui-monospace, SFMono-Regular, Menlo, Consolas, monospace; }
  .diff-scroll:focus-visible { outline: 1px solid var(--focus); outline-offset: -2px; }
  .diff-line { display: grid; grid-template-columns: 38px 38px 18px 1fr; width: max-content; min-width: 100%; padding-right: 16px; }
  .line-number { color: var(--muted); text-align: right; padding-right: 8px; user-select: none; }
  .diff-prefix, .diff-code { white-space: pre; font: inherit; tab-size: 4; }
  .diff-prefix { text-align: center; }
  .diff-line[data-kind="addition"] { background: var(--diff-add-bg); color: var(--diff-add); }
  .diff-line[data-kind="removal"] { background: var(--diff-remove-bg); color: var(--diff-remove); }
  .diff-line[data-kind="hunk"], .diff-line[data-kind="meta"] { color: var(--muted); }
  .diff-line[data-kind="hunk"] { background: var(--hover); }
  @media (max-width: 420px) { .diff-line { grid-template-columns: 28px 28px 16px 1fr; } .diff-header { padding-left: 12px; padding-right: 12px; } .diff-reason { padding-left: 12px; padding-right: 12px; } }
</style>
