<script lang="ts">
  import type { DiffLine } from '../diff';

  let { lines, wrap = false, testId = 'diff-line' }: {
    lines: DiffLine[];
    wrap?: boolean;
    testId?: string;
  } = $props();
</script>

<div class="diff-lines" class:wrap>
  {#each lines as line, index (index)}
    <div class="diff-line" data-testid={testId} data-kind={line.kind}>
      <span class="line-number" aria-hidden="true">{line.oldLine ?? ''}</span><span class="line-number" aria-hidden="true">{line.newLine ?? ''}</span><span class="diff-prefix">{line.prefix}</span><code class="diff-code">{line.text}</code>
    </div>
  {/each}
</div>

<style>
  .diff-lines { width: max-content; min-width: 100%; }
  .diff-line { display: grid; grid-template-columns: 38px 38px 18px minmax(0, 1fr); padding-right: 16px; }
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
