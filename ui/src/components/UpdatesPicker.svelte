<script lang="ts">
  import { tick } from 'svelte';
  import type { UpdateMode } from '../lib';

  let { mode, connected, onchange }: { mode: UpdateMode; connected: boolean; onchange: (mode: UpdateMode) => Promise<void> } = $props();
  const options = [
    { value: 'milestones', label: 'Major milestones', detail: 'New key steps only, with brief explanations.' },
    { value: 'files', label: 'Every file', detail: 'New saves from now on. Per-file explanations may use more tokens.' },
    { value: 'activity', label: 'All activity', detail: 'Tool activity and short explanations before actions. More agent reports may use tokens. Private thinking is not exposed.' },
    { value: 'disabled', label: 'Disabled', detail: 'Stop new updates. Keep saved history.' },
  ] as const;
  const label = $derived(mode === 'milestones' ? 'Milestones' : mode === 'disabled' ? 'Disabled' : mode === 'activity' ? 'All activity' : 'Every file');
  let open = $state(false);
  let saving = $state(false);
  let container: HTMLDivElement;
  let trigger: HTMLButtonElement;
  let menu = $state<HTMLDivElement>();

  function close() { open = false; trigger?.focus(); }
  async function pick(value: UpdateMode) {
    if (saving) return;
    saving = true;
    try { await onchange(value); close(); }
    catch { /* The app displays the failure and leaves the current mode intact. */ }
    finally { saving = false; }
  }
  async function toggle() {
    open = !open;
    if (open) { await tick(); menu?.querySelector<HTMLButtonElement>('[aria-checked="true"]')?.focus(); }
  }
  function navigate(event: KeyboardEvent) {
    if (event.key === 'Escape') { event.preventDefault(); event.stopPropagation(); close(); return; }
    if (event.key === 'Tab') { open = false; return; }
    if (!menu || !['ArrowUp', 'ArrowDown', 'Home', 'End'].includes(event.key)) return;
    event.preventDefault();
    const buttons = [...menu.querySelectorAll<HTMLButtonElement>('[role="menuitemradio"]')];
    const current = buttons.indexOf(document.activeElement as HTMLButtonElement);
    const index = event.key === 'Home' ? 0 : event.key === 'End' ? buttons.length - 1 : (current + (event.key === 'ArrowDown' ? 1 : -1) + buttons.length) % buttons.length;
    buttons[index]?.focus();
  }
</script>

<div class="updates" bind:this={container}>
  <button class="updates-trigger" bind:this={trigger} onclick={toggle} disabled={!connected || saving} aria-label="Updates" title={'Updates: ' + label} aria-haspopup="menu" aria-expanded={open}>{label}<span aria-hidden="true">⌄</span></button>
  {#if open}
    <div class="updates-menu" bind:this={menu} role="menu" aria-label="Updates" tabindex="-1" onkeydown={navigate}>
      {#each options as option (option.value)}
        <button class="updates-option" role="menuitemradio" aria-label={option.label} aria-describedby={'stef-updates-' + option.value + '-detail'} aria-checked={mode === option.value} tabindex={mode === option.value ? 0 : -1} disabled={saving || !connected} onclick={() => pick(option.value)}>
          <span>{option.label}<span aria-hidden="true">{mode === option.value ? '✓' : ''}</span></span>
          <small id={'stef-updates-' + option.value + '-detail'}>{option.detail}</small>
        </button>
      {/each}
    </div>
  {/if}
</div>
<svelte:window onpointerdown={event => { if (open && event.target instanceof Node && !container.contains(event.target)) open = false; }} />
