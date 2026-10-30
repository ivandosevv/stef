<script lang="ts">
  import { onMount } from 'svelte';
  import GraphCanvas from './components/GraphCanvas.svelte';
  import ThemePicker from './components/ThemePicker.svelte';
  import UpdatesPicker from './components/UpdatesPicker.svelte';
  import FileDiff from './components/FileDiff.svelte';
  import { hasFileDiff } from './diff';
  import { connectStream, isEmbedded, sendToHost } from './transport';
  import { workflowSteps, shortName, isUpdateMode, type AgentEvent, type UpdateMode } from './lib';

  let events = $state<AgentEvent[]>([]);
  let session = $state('');
  let selected = $state<string | null>(null);
  let connected = $state(false);
  let error = $state('');
  let note = $state('');
  let noteStatus = $state('');
  let noteOpen = $state(false);
  let diffOpen = $state(false);
  let busy = $state(false);
  let updateMode = $state<UpdateMode>('files');
  const embedded = isEmbedded();
  const steps = $derived(workflowSteps(events, updateMode));
  const chosen = $derived(steps.find(step => step.id === selected));
  $effect(() => { if (diffOpen && (!chosen || !hasFileDiff(chosen))) diffOpen = false; });
  type NativeWindow = Window & { stefNativeReady?: () => Promise<void> };
  const selectionKey = () => 'stef-selection:' + session;

  async function changeUpdates(mode: UpdateMode) {
    error = '';
    try {
      const response = await fetch('/api/settings', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ update_mode: mode }) });
      if (!response.ok) throw new Error('Could not change updates.');
      const settings = await response.json();
      if (!isUpdateMode(settings.update_mode)) throw new Error('Could not change updates.');
      updateMode = settings.update_mode;
    } catch (cause) { error = cause instanceof Error ? cause.message : 'Could not change updates.'; throw cause; }
  }

  function select(id: string) {
    selected = selected === id ? null : id;
    diffOpen = false;
    note = ''; noteStatus = ''; noteOpen = false;
    if (selected) localStorage.setItem(selectionKey(), selected);
    else localStorage.removeItem(selectionKey());
  }
  async function clear() {
    if (busy || !session) return;
    busy = true; error = '';
    try {
      const response = await fetch('/api/sessions/clear', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ session_id: session }) });
      if (!response.ok) throw new Error('Could not clear this workflow.');
      const snapshot = await response.json();
      if (snapshot.session_id === session) { events = []; selected = null; note = ''; noteStatus = ''; noteOpen = false; diffOpen = false; localStorage.removeItem(selectionKey()); }
    } catch (cause) { error = cause instanceof Error ? cause.message : 'Could not clear this workflow.'; }
    finally { busy = false; }
  }
  async function suggest(event: SubmitEvent) {
    event.preventDefault();
    if (!chosen || !note.trim() || busy) return;
    busy = true; noteStatus = '';
    try {
      const response = await fetch('/api/feedback', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ session_id: session, node_name: chosen.node_name, target_agent: '*', kind: 'suggestion', text: note.trim() }) });
      if (!response.ok) throw new Error('Could not send your note.');
      note = ''; noteStatus = 'Queued for the agent’s next check-in.';
    } catch (cause) { noteStatus = cause instanceof Error ? cause.message : 'Could not send your note.'; }
    finally { busy = false; }
  }
  onMount(() => {
    const disconnect = connectStream(data => {
      if (data.type === 'snapshot') {
        if (isUpdateMode(data.update_mode)) updateMode = data.update_mode;
        const id = data.session_id ?? 'local';
        const changed = id !== session;
        session = id; events = data.events ?? [];
        if (changed) { selected = localStorage.getItem(selectionKey()); note = ''; noteStatus = ''; noteOpen = false; diffOpen = false; }
        if (!workflowSteps(events).some(step => step.id === selected)) { selected = null; localStorage.removeItem(selectionKey()); }
      } else if (!data.event.session_id || data.event.session_id === session) {
        events = [...events.filter(event => event.id !== data.event.id), data.event].slice(-5000);
      }
    }, (online) => { connected = online; });
    void (window as NativeWindow).stefNativeReady?.();
    return disconnect;
  });
</script>

<div class="stef-app" data-update-mode={updateMode}>
  <header><span class="brand">stef</span><div class="header-actions"><UpdatesPicker mode={updateMode} {connected} onchange={changeUpdates} /><ThemePicker /><button class="clear" onclick={clear} disabled={busy || !events.length} aria-label="Clear this chat’s workflow" title="Clear this chat’s workflow">↺</button></div></header>
  <main aria-label="Workflow">
    {#if error}<p class="notice" role="status">{error}</p>{/if}
    {#if updateMode === 'disabled'}<div class="empty"><p>Updates disabled</p></div>{:else}
    <GraphCanvas {steps} {selected} onselect={select} {connected} detailHeight={noteOpen ? 208 : 144}>
      {#snippet detail(step)}
        <section class="step-detail" aria-label="Why this step">
          <p>{step.why || (step.source === 'watcher' ? 'File saved. Waiting for the agent’s explanation.' : 'Waiting for the agent’s explanation.')}</p>
          <small class="file" title={step.node_name}>{step.node_name === '@workflow' ? 'Current task' : step.node_name}</small>
          {#if hasFileDiff(step)}<button class="show-diff" onclick={() => diffOpen = true}>Show diff</button>{/if}
          {#if embedded && step.node_name !== '@workflow'}<button class="open-file" onclick={() => sendToHost({ type: 'stef.openFile', node: step.node_name })}>Open {shortName(step.node_name)}</button>{/if}
          {#if !noteOpen}<button class="suggest" onclick={() => noteOpen = true}>Suggest</button>{:else}
            <form onsubmit={suggest}><input aria-label="Suggest something to the agent" placeholder="Suggest something…" bind:value={note} maxlength="500" /><button aria-label="Send suggestion" disabled={!note.trim() || busy || !connected}>↑</button></form>
            {#if noteStatus}<small role="status">{noteStatus}</small>{/if}
          {/if}
        </section>
      {/snippet}
    </GraphCanvas>
    {/if}
  </main>
</div>
{#if diffOpen && chosen && hasFileDiff(chosen)}<FileDiff step={chosen} onclose={() => diffOpen = false} />{/if}
<svelte:window onkeydown={event => { if (event.key === 'Escape' && selected && !diffOpen) select(selected); }} />
