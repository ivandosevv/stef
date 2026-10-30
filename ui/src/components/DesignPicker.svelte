<script lang="ts">
  import { tick } from 'svelte';
  import type { Design } from '../design';
  let { design, onchange }: { design: Design; onchange: (value: Design) => void } = $props();
  const options = [
    { value: 'minimal', label: 'Minimal', detail: 'Quiet workflow and brief reasons.' },
    { value: 'interactive', label: 'Interactive', detail: 'Visual change previews. Extra AI context only when you ask.' },
  ] as const;
  let open = $state(false);
  let container: HTMLDivElement;
  let trigger: HTMLButtonElement;
  let menu = $state<HTMLDivElement>();
  function close() { open = false; trigger?.focus(); }
  async function toggle() { open = !open; if (open) { await tick(); menu?.querySelector<HTMLButtonElement>('[aria-checked="true"]')?.focus(); } }
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

<div class="design" bind:this={container}>
  <button class="design-trigger" bind:this={trigger} onclick={toggle} aria-label="Design" title={'Design: ' + (design === 'minimal' ? 'Minimal' : 'Interactive')} aria-haspopup="menu" aria-expanded={open}><span aria-hidden="true">{design === 'minimal' ? '☷' : '◇'}</span></button>
  {#if open}
    <div class="design-menu" bind:this={menu} role="menu" aria-label="Design" tabindex="-1" onkeydown={navigate}>
      {#each options as option}
        <button class="design-option" role="menuitemradio" aria-label={option.label} aria-describedby={'stef-design-' + option.value} aria-checked={design === option.value} tabindex={design === option.value ? 0 : -1} onclick={() => { onchange(option.value); close(); }}>
          <span>{option.label}<span aria-hidden="true">{design === option.value ? '✓' : ''}</span></span><small id={'stef-design-' + option.value}>{option.detail}</small>
        </button>
      {/each}
      <small class="design-cost">Changing the design uses no AI calls.</small>
    </div>
  {/if}
</div>
<svelte:window onpointerdown={event => { if (open && event.target instanceof Node && !container.contains(event.target)) open = false; }} />
