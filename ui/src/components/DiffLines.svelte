<script lang="ts">
  import type { DiffLine } from '../diff';
  import { lineKey, lineLabel, selectableLine } from '../interaction';

  let { lines, wrap = false, testId = 'diff-line', onselect, selected = null }: {
    lines: DiffLine[];
    wrap?: boolean;
    testId?: string;
    onselect?: (line: DiffLine) => void;
    selected?: string | null;
  } = $props();
  function navigate(event: KeyboardEvent) {
    if (!['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key)) return;
    const current = event.currentTarget as HTMLButtonElement;
    const buttons = Array.from(current.parentElement!.querySelectorAll<HTMLButtonElement>('button.diff-line'));
    const index = buttons.indexOf(current);
    const target = event.key === 'Home' ? buttons[0] : event.key === 'End' ? buttons.at(-1) : buttons[Math.max(0, Math.min(buttons.length - 1, index + (event.key === 'ArrowDown' ? 1 : -1)))];
    event.preventDefault(); target?.focus(); target?.click();
  }
</script>

<div class="diff-lines" class:wrap>
  {#each lines as line, index (index)}
    {#if onselect && selectableLine(line)}
      <button type="button" class="diff-line" class:chosen={selected === lineKey(line)} data-testid={testId} data-kind={line.kind} aria-label={'Select ' + lineLabel(line).toLowerCase()} aria-pressed={selected === lineKey(line)} onclick={() => onselect?.(line)} onkeydown={navigate}>
        <span class="line-number" aria-hidden="true">{line.oldLine ?? ''}</span><span class="line-number" aria-hidden="true">{line.newLine ?? ''}</span><span class="diff-prefix">{line.prefix}</span><code class="diff-code">{line.text}</code>
      </button>
    {:else}
      <div class="diff-line" data-testid={testId} data-kind={line.kind}>
        <span class="line-number" aria-hidden="true">{line.oldLine ?? ''}</span><span class="line-number" aria-hidden="true">{line.newLine ?? ''}</span><span class="diff-prefix">{line.prefix}</span><code class="diff-code">{line.text}</code>
      </div>
    {/if}
  {/each}
</div>

<style>
  .diff-lines { width: max-content; min-width: 100%; }
  .diff-line { display: grid; grid-template-columns: 38px 38px 18px minmax(0, 1fr); padding-right: 16px; }
  button.diff-line { width: 100%; margin: 0; padding-block: 0; padding-left: 0; border: 0; border-radius: 0; color: inherit; background: none; text-align: left; font: inherit; user-select: text; }
  button.diff-line:hover { box-shadow: inset 3px 0 var(--control); }
  button.diff-line.chosen { box-shadow: inset 3px 0 var(--focus); outline: 1px solid var(--focus); outline-offset: -1px; }
  button.diff-line:focus-visible { outline: 1px solid var(--focus); outline-offset: -1px; }
  .line-number { color: var(--muted); text-align: right; padding-right: 8px; white-space: nowrap; user-select: none; }
  .diff-prefix, .diff-code { white-space: pre; font: inherit; tab-size: 4; }
  .diff-prefix { text-align: center; }
  .diff-line[data-kind="addition"] { background: var(--diff-add-bg); color: var(--diff-add); }
  .diff-line[data-kind="removal"] { background: var(--diff-remove-bg); color: var(--diff-remove); }
  .diff-line[data-kind="hunk"], .diff-line[data-kind="meta"] { color: var(--muted); }
  .diff-line[data-kind="hunk"] { background: var(--hover); }
  .wrap { width: 100%; min-width: 0; }
  .wrap .diff-line { grid-template-columns: 32px 32px 16px minmax(0, 1fr); padding-right: 8px; }
  .wrap .line-number { padding-right: 5px; }
  .wrap .diff-code { min-width: 0; white-space: break-spaces; overflow-wrap: anywhere; }
  @media (max-width: 420px) { .diff-line, .wrap .diff-line { grid-template-columns: 28px 28px 16px minmax(0, 1fr); } }
</style>
