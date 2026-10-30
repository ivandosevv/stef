<script lang="ts">
  import { onMount } from 'svelte';
  import GraphCanvas from './components/GraphCanvas.svelte';
  import ThemePicker from './components/ThemePicker.svelte';
  import UpdatesPicker from './components/UpdatesPicker.svelte';
  import FileDiff from './components/FileDiff.svelte';
  import DesignPicker from './components/DesignPicker.svelte';
  import ChangeVisual from './components/ChangeVisual.svelte';
  import { readDesign, saveDesign, type Design } from './design';
  import { hasFileDiff } from './diff';
  import { connectStream, isEmbedded, sendToHost } from './transport';
  import { workflowSteps, workflowWindows, shortName, isUpdateMode, type AgentEvent, type UpdateMode } from './lib';

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
  let updatesFrom = $state('0');
  let showHistory = $state(false);
  let design = $state<Design>(readDesign());
  let asking = $state(false);
  let requestStatus = $state('');
  let contextRequest = $state<{ session: string; step: string; reference: string; after: string } | null>(null);
  const embedded = isEmbedded();
  const windows = $derived(workflowWindows(events, updateMode, updatesFrom));
  const steps = $derived(showHistory ? [...windows.saved, ...windows.current] : windows.current);
  const chosen = $derived(steps.find(step => step.id === selected));
  $effect(() => { if (diffOpen && (!chosen || !hasFileDiff(chosen))) diffOpen = false; });
  $effect(() => {
    if (contextRequest && contextRequest.session === session && contextRequest.step === selected && events.some(event => event.context_id === contextRequest!.reference && BigInt(event.id) > BigInt(contextRequest!.after))) {
      requestStatus = 'The agent added more context.'; contextRequest = null;
    }
  });
  type NativeWindow = Window & { stefNativeReady?: () => Promise<void> };
  const selectionKey = () => 'stef-selection:' + session;
  const historyKey = () => 'stef-history:' + session + ':' + updatesFrom;
  function changeDesign(value: Design) { design = value; saveDesign(value); }
  function toggleHistory(value = !showHistory) {
    showHistory = value;
    try { localStorage.setItem(historyKey(), String(value)); } catch { /* History is still available in this view. */ }
  }

  async function askContext() {
    if (!chosen || asking || !connected) return;
    const target = events.find(event => event.id === (chosen.reference_id ?? chosen.id));
    if (!target) return;
    asking = true; requestStatus = '';
    const targetSession = session, targetStep = chosen.id;
    const after = events.at(-1)?.id ?? '0';
    try {
      const reporter = [...events].reverse().find(event => event.source === 'agent' && !event.category)?.agent_id ?? '*';
      const text = 'Explain this retained step more clearly. Reply using report_event with context_id="' + target.id + '", the node from this note, action="' + target.action + '", a short summary and a fuller user-facing why. Update this step only; do not generate other file explanations.';
      const response = await fetch('/api/feedback', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ session_id: session, node_name: target.node_name, target_agent: reporter, kind: 'question', text }) });
      if (!response.ok) throw new Error('Could not ask for more context.');
      if (session === targetSession && selected === targetStep) { requestStatus = 'Asked. The agent can respond at its next check-in.'; contextRequest = { session, step: targetStep, reference: target.id, after }; }
    } catch (cause) { if (session === targetSession && selected === targetStep) requestStatus = cause instanceof Error ? cause.message : 'Could not ask for more context.'; }
    finally { asking = false; }
  }

  async function changeUpdates(mode: UpdateMode) {
    error = '';
    const previousMode = updateMode;
    try {
      const response = await fetch('/api/settings', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ update_mode: mode }) });
      if (!response.ok) throw new Error('Could not change updates.');
      const settings = await response.json();
      if (!isUpdateMode(settings.update_mode)) throw new Error('Could not change updates.');
      if (settings.update_mode !== previousMode) { toggleHistory(false); diffOpen = false; requestStatus = ''; contextRequest = null; }
      updateMode = settings.update_mode;
    } catch (cause) { error = cause instanceof Error ? cause.message : 'Could not change updates.'; throw cause; }
  }

  function select(id: string) {
    selected = selected === id ? null : id;
    diffOpen = false;
    requestStatus = '';
    contextRequest = null;
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
      if (snapshot.session_id === session) { events = []; selected = null; note = ''; noteStatus = ''; noteOpen = false; diffOpen = false; requestStatus = ''; showHistory = false; localStorage.removeItem(selectionKey()); }
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
        const boundary = data.updates_from && /^\d+$/.test(data.updates_from) ? data.updates_from : '0';
        const windowChanged = boundary !== updatesFrom || (isUpdateMode(data.update_mode) && data.update_mode !== updateMode);
        if (windowChanged) { showHistory = false; diffOpen = false; requestStatus = ''; contextRequest = null; }
        updatesFrom = boundary;
        if (isUpdateMode(data.update_mode)) updateMode = data.update_mode;
        const id = data.session_id ?? 'local';
        const changed = id !== session;
        const resumed = changed && Boolean(session);
        session = id; events = data.events ?? [];
        if (changed) { selected = localStorage.getItem(selectionKey()); note = ''; noteStatus = ''; noteOpen = false; diffOpen = false; requestStatus = ''; contextRequest = null; showHistory = localStorage.getItem(historyKey()) === 'true'; }
        else if (windowChanged) toggleHistory(false);
        if (!workflowSteps(events).some(step => step.id === selected)) { selected = null; localStorage.removeItem(selectionKey()); }
        if (resumed && selected && !workflowWindows(events, updateMode, updatesFrom).current.some(step => step.id === selected)) toggleHistory(true);
      } else if (!data.event.session_id || data.event.session_id === session) {
        events = [...events.filter(event => event.id !== data.event.id), data.event].slice(-5000);
      }
    }, (online) => { connected = online; });
    void (window as NativeWindow).stefNativeReady?.();
    return disconnect;
  });
</script>

<div class="stef-app" data-update-mode={updateMode} data-design={design}>
  <header><span class="brand">stef</span><div class="header-actions"><UpdatesPicker mode={updateMode} {connected} onchange={changeUpdates} /><DesignPicker {design} onchange={changeDesign} /><ThemePicker /><button class="clear" onclick={clear} disabled={busy || !events.length} aria-label="Clear this chat’s workflow" title="Clear this chat’s workflow">↺</button></div></header>
  <main aria-label="Workflow">
    {#if error}<p class="notice" role="status">{error}</p>{/if}
    {#if updateMode === 'disabled'}<div class="empty"><p>Updates disabled</p></div>{:else}
    {#if windows.saved.length}<div class="history-control"><button onclick={() => toggleHistory()}>{showHistory ? 'Hide saved history' : 'Show saved history'}</button><small>{showHistory ? 'Saved history — viewing it uses no AI calls.' : 'New updates only. Earlier work is saved.'}</small></div>{/if}
    <div class="workflow-layout" class:interactive={design === 'interactive'}>
    <GraphCanvas {steps} {selected} onselect={select} {connected} detailHeight={design === 'interactive' ? 38 : noteOpen ? 208 : 144}>
      {#snippet detail(step)}
        {#if design === 'minimal'}
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
        {/if}
      {/snippet}
    </GraphCanvas>
    {#if design === 'interactive'}
      <div class="interactive-detail">
        {#if chosen}
          <ChangeVisual step={chosen} {steps} onselect={select} ondiff={() => diffOpen = true} onexplain={askContext} {connected} asking={asking || contextRequest?.step === chosen.id} {requestStatus} />
          <section class="step-detail" aria-label="Suggest for this step">
            {#if embedded && chosen.node_name !== '@workflow'}<button class="open-file" onclick={() => sendToHost({ type: 'stef.openFile', node: chosen.node_name })}>Open {shortName(chosen.node_name)}</button>{/if}
            {#if !noteOpen}<button class="suggest" onclick={() => noteOpen = true}>Suggest</button>{:else}
              <form onsubmit={suggest}><input aria-label="Suggest something to the agent" placeholder="Suggest something…" bind:value={note} maxlength="500" /><button aria-label="Send suggestion" disabled={!note.trim() || busy || !connected}>↑</button></form>
              {#if noteStatus}<small role="status">{noteStatus}</small>{/if}
            {/if}
          </section>
        {:else}<div class="visual-empty"><span aria-hidden="true">◇</span><p>Select a step to explore the change.</p></div>{/if}
      </div>
    {/if}
    </div>
    {/if}
  </main>
</div>
{#if diffOpen && chosen && hasFileDiff(chosen)}<FileDiff step={design === 'interactive' ? { ...chosen, why: chosen.explanation || chosen.why } : chosen} onclose={() => diffOpen = false} />{/if}
<svelte:window onkeydown={event => { if (event.key === 'Escape' && selected && !diffOpen) select(selected); }} />
