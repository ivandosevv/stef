<script lang="ts">
  import { onMount, tick } from 'svelte';
  import { applyTheme, readTheme, saveTheme, themeOptions, type Theme } from '../theme';

  let theme = $state<Theme>('dark');
  let open = $state(false);
  let container: HTMLDivElement;
  let trigger: HTMLButtonElement;
  let menu = $state<HTMLDivElement>();

  function close() { open = false; trigger?.focus(); }
  function pick(value: Theme) { theme = value; saveTheme(value); close(); }
  async function toggle() {
    open = !open;
    if (open) {
      await tick();
      menu?.querySelector<HTMLButtonElement>('[aria-checked="true"]')?.focus();
    }
  }
  function navigate(event: KeyboardEvent) {
    if (event.key === 'Escape') { event.preventDefault(); event.stopPropagation(); close(); return; }
    if (event.key === 'Tab') { open = false; return; }
    if (!['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key)) return;
    event.preventDefault();
    if (!menu) return;
    const options = [...menu.querySelectorAll<HTMLButtonElement>('[role="menuitemradio"]')];
    const current = options.indexOf(document.activeElement as HTMLButtonElement);
    const index = event.key === 'Home' ? 0 : event.key === 'End' ? options.length - 1 : (current + (event.key === 'ArrowDown' ? 1 : -1) + options.length) % options.length;
    options[index]?.focus();
  }
  onMount(() => {
    theme = readTheme();
    applyTheme(theme);
    const system = matchMedia('(prefers-color-scheme: dark)');
    const update = () => { if (theme === 'system') applyTheme(theme); };
    system.addEventListener('change', update);
    return () => system.removeEventListener('change', update);
  });
</script>

<div class="appearance" bind:this={container}>
  <button class="appearance-trigger" bind:this={trigger} onclick={toggle} aria-label="Appearance" title={'Appearance: ' + themeOptions.find(option => option.value === theme)?.label} aria-haspopup="menu" aria-expanded={open}>
    <svg width="14" height="14" viewBox="0 0 16 16" aria-hidden="true"><circle cx="8" cy="8" r="5.5" fill="none" stroke="currentColor" /><path d="M8 2.5a5.5 5.5 0 0 1 0 11Z" fill="currentColor" /></svg>
  </button>
  {#if open}
    <div class="appearance-menu" bind:this={menu} role="menu" aria-label="Appearance" tabindex="-1" onkeydown={navigate}>
      {#each themeOptions as option (option.value)}
        <button class="appearance-option" role="menuitemradio" aria-checked={theme === option.value} tabindex={theme === option.value ? 0 : -1} onclick={() => pick(option.value)}>
          <span>{option.label}</span><span aria-hidden="true">{theme === option.value ? '✓' : ''}</span>
        </button>
      {/each}
    </div>
  {/if}
</div>
<svelte:window onpointerdown={event => { if (open && event.target instanceof Node && !container.contains(event.target)) open = false; }} />
