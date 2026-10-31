<script lang="ts">
  import { tick } from 'svelte';
  import { workspaceName, type Workspace } from '../lib';
  let { origin, workspaceID, workspaces, connected, onselect, onchange }: {
    origin: string; workspaceID: string; workspaces: Workspace[]; connected: boolean;
    onselect: (origin: string) => void;
    onchange: (action: 'create' | 'join' | 'leave', workspaceID?: string) => Promise<void>;
  } = $props();
  let open = $state(false);
  let joinOpen = $state(false);
  let joinID = $state('');
  let busy = $state(false);
  let status = $state('');
  let container: HTMLDivElement;
  let trigger: HTMLButtonElement;
  let menu = $state<HTMLDivElement>();
  const current = $derived(workspaces.find(workspace => workspace.workspace_id === workspaceID));
  const currentName = $derived(workspaceName(current));
  const shared = $derived(Boolean(current?.shared));
  const choices = $derived(workspaces.filter(workspace => !workspace.shared || workspace.members.length > 0));
  const canConfigure = $derived(Boolean(origin && current && (current.shared ? current.members.includes(origin) : origin === current.workspace_id)));
  const sharingFor = (id: string) => workspaces.find(room => room.shared && room.members.includes(id));
  const memberNames = (workspace: Workspace) => workspace.members.map(id => workspaceName(workspaces.find(room => room.workspace_id === id), id)).join(', ');
  function close() { open = false; joinOpen = false; status = ''; trigger?.focus(); }
  async function toggle() { open = !open; status = ''; if (open) { await tick(); menu?.querySelector<HTMLElement>('button[aria-pressed="true"]')?.focus(); } }
  async function change(action: 'create' | 'join' | 'leave') {
    if (busy || !connected || !canConfigure || (action === 'join' && !joinID.trim())) return;
    busy = true; status = '';
    try { await onchange(action, action === 'join' ? joinID.trim() : undefined); close(); joinID = ''; }
    catch (cause) { status = cause instanceof Error ? cause.message : 'Could not change sharing.'; }
    finally { busy = false; }
  }
  async function copyName() {
    const invitation = current?.name?.trim() || workspaceID;
    try { await navigator.clipboard.writeText(invitation); status = 'Copied. Ask another agent to join ' + invitation + '.'; }
    catch { status = 'Share ' + invitation + ' with another agent.'; }
  }
  function navigate(event: KeyboardEvent) {
    if (event.key === 'Escape') { event.preventDefault(); event.stopPropagation(); close(); return; }
    if (event.key === 'Tab') { open = false; return; }
    if (!menu || event.target instanceof HTMLInputElement || !['ArrowUp', 'ArrowDown', 'Home', 'End'].includes(event.key)) return;
    event.preventDefault();
    const buttons = [...menu.querySelectorAll<HTMLButtonElement>('button:not(:disabled)')];
    const current = buttons.indexOf(document.activeElement as HTMLButtonElement);
    const next = event.key === 'Home' ? 0 : event.key === 'End' ? buttons.length - 1 : (current + (event.key === 'ArrowDown' ? 1 : -1) + buttons.length) % buttons.length;
    buttons[next]?.focus();
  }
</script>

<div class="workspace" bind:this={container}>
  <button class="workspace-trigger" bind:this={trigger} onclick={toggle} aria-label="Workspace" aria-haspopup="dialog" aria-expanded={open} title={currentName + ' · ' + (shared ? 'Shared' : 'Private')}><span class="workspace-name">{currentName}</span><span aria-hidden="true">⌄</span></button>
  {#if open}
    <div class="workspace-menu" bind:this={menu} role="dialog" aria-label="Workspaces and sharing" tabindex="-1" onkeydown={navigate}>
      <div class="workspace-list">
        {#each choices as workspace (workspace.workspace_id)}
          <button class="workspace-choice" aria-label={workspaceName(workspace)} aria-pressed={workspace.workspace_id === origin} title={workspaceName(workspace) + ' · ' + (workspace.shared ? 'Shared with ' + memberNames(workspace) : sharingFor(workspace.workspace_id) ? 'Sharing ' + workspaceName(sharingFor(workspace.workspace_id)) : 'Private')} disabled={busy} onclick={() => { onselect(workspace.workspace_id); close(); }}>
            <span>{workspaceName(workspace)}<span aria-hidden="true">{workspace.workspace_id === origin ? '✓' : ''}</span></span>
            <small>{workspace.shared ? 'Shared · ' + workspace.members.length + (workspace.members.length === 1 ? ' agent' : ' agents') : sharingFor(workspace.workspace_id) ? 'In ' + workspaceName(sharingFor(workspace.workspace_id)) : 'Private'}</small>
          </button>
        {/each}
      </div>
      <div class="workspace-sharing">
        {#if shared}
          <button onclick={copyName} disabled={busy}>Copy sharing name</button>
          {#if canConfigure}<button onclick={() => change('leave')} disabled={busy || !connected}>Return to private</button>{/if}
        {:else if canConfigure}
          <button onclick={() => change('create')} disabled={busy || !connected || !origin}>Start shared workspace</button>
        {/if}
        {#if canConfigure}<button onclick={() => { joinOpen = !joinOpen; status = ''; }} disabled={busy || !connected}>Join shared workspace</button>{/if}
        {#if joinOpen}
          <form onsubmit={event => { event.preventDefault(); void change('join'); }}>
            <input aria-label="Sharing name" placeholder="Quiet Fox 248" bind:value={joinID} maxlength="160" />
            <button disabled={busy || !joinID.trim() || !connected}>Join</button>
          </form>
        {/if}
        <small>{canConfigure ? 'Agents stay separate until they join. New shared workflows start empty.' : 'Viewing this workflow. Select an agent to change its sharing.'}</small>
        {#if status}<small role="status">{status}</small>{/if}
      </div>
    </div>
  {/if}
</div>
<svelte:window onpointerdown={event => { if (open && event.target instanceof Node && !container.contains(event.target)) { open = false; joinOpen = false; } }} />

<style>
  .workspace { position: relative; min-width: 0; }
  .workspace-trigger { display: flex; align-items: center; gap: 5px; min-width: 0; width: 100%; border: 0; background: none; color: var(--control); border-radius: 5px; padding: 6px 4px; font-size: 10px; }
  .workspace-trigger:hover { color: var(--text); }
  .workspace-name { min-width: 0; max-width: 130px; overflow: hidden; white-space: nowrap; text-overflow: ellipsis; }
  .workspace-menu { position: absolute; z-index: 8; left: 0; top: calc(100% + 5px); width: min(250px, calc(100vw - 80px)); padding: 4px; border: 1px solid var(--border); border-radius: 7px; background: var(--surface); box-shadow: 0 4px 16px #00000016; }
  .workspace-list { max-height: 180px; overflow-y: auto; overscroll-behavior: contain; }
  .workspace-choice { display: block; width: 100%; padding: 8px; border: 0; border-radius: 4px; background: none; color: var(--muted); font-size: 11px; text-align: left; }
  .workspace-choice > span { display: flex; justify-content: space-between; gap: 8px; color: var(--text); overflow-wrap: anywhere; }
  .workspace-choice:hover, .workspace-choice:focus-visible, .workspace-sharing > button:hover { background: var(--hover); }
  small { display: block; margin-top: 4px; color: var(--faint); font-size: 10px; line-height: 1.5; overflow-wrap: anywhere; }
  .workspace-sharing { padding: 4px; border-top: 1px solid var(--border); }
  .workspace-sharing > button { display: block; width: 100%; background: none; color: var(--control); border: 0; border-radius: 4px; padding: 7px 4px; text-align: left; font-size: 11px; }
  .workspace-sharing > small { padding: 0 4px; }
  form { display: flex; gap: 4px; margin: 5px 0 8px; }
  input { min-width: 0; width: 100%; padding: 7px; border: 1px solid var(--input-border); border-radius: 4px; background: var(--input); color: var(--text); font-size: 11px; }
  form button { border: 1px solid var(--border); border-radius: 4px; background: none; color: var(--control); font-size: 11px; }
</style>
