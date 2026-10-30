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
  import { workflowSteps, workflowWindows, mergeEvents, shortName, isUpdateMode, type AgentEvent, type UpdateMode } from './lib';

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
  type ContextRequest = { session: string; step: string; reference: string; after: string; missing: boolean; node: string; action: AgentEvent['action'] };
  let contextRequests = $state<ContextRequest[]>([]);
  let requestEpoch = 0;
  let typing = $state(false);
  let followed = $state(false);
  let pendingFollow = $state<string | null>(null);
  let reveal = $state<{ id: string; version: number } | null>(null);
  let revealVersion = 0;
  const embedded = isEmbedded();
  const windows = $derived(workflowWindows(events, updateMode, updatesFrom));
  const steps = $derived(showHistory ? [...windows.saved, ...windows.current] : windows.current);
  const chosen = $derived(steps.find(step => step.id === selected));
  const chosenRequest = $derived(contextRequests.find(request => request.session === session && request.reference === (chosen?.reference_id ?? chosen?.id)));
  const followBlocked = $derived(diffOpen || typing || Boolean(note.trim()));
  $effect(() => {
    if (!pendingFollow || followBlocked) return;
    const latest = windows.current.at(-1);
    pendingFollow = null;
    if (!latest) return;
    selected = latest.id; followed = true;
    requestStatus = ''; note = ''; noteStatus = ''; noteOpen = false;
    rememberSelection();
    reveal = { id: latest.id, version: ++revealVersion };
  });
  $effect(() => { if (diffOpen && (!chosen || !hasFileDiff(chosen))) diffOpen = false; });
  $effect(() => {
    const answered = contextRequests.filter(request => events.some(event =>
      event.source === 'agent' && event.session_id === request.session && event.node_name === request.node && event.action === request.action &&
      BigInt(event.id) > BigInt(request.after) && (event.context_id === request.reference || (request.missing && event.change_id === request.reference))
    ));
    if (!answered.length) return;
    const current = answered.find(request => request.session === session && request.reference === (chosen?.reference_id ?? chosen?.id));
    if (current) requestStatus = current.missing ? 'The agent added an explanation.' : 'The agent added more context.';
    contextRequests = contextRequests.filter(request => !answered.includes(request));
  });
  type NativeWindow = Window & { stefNativeReady?: () => Promise<void> };
  const selectionKey = () => 'stef-selection:' + session;
  const historyKey = () => 'stef-history:' + session + ':' + updatesFrom;
  function rememberSelection() {
    try {
      if (selected) localStorage.setItem(selectionKey(), selected);
      else localStorage.removeItem(selectionKey());
    } catch { /* Selection still works when view storage is unavailable. */ }
  }
  function resetFollow() { pendingFollow = null; reveal = null; followed = false; }
  const stepKey = (step?: AgentEvent) => step ? step.id + ':' + (step.reference_id ?? step.id) : '';
  function followUpdate(previous?: AgentEvent) {
    const latest = workflowWindows(events, updateMode, updatesFrom).current.at(-1);
    if (!latest) return;
    if (stepKey(latest) !== stepKey(previous)) pendingFollow = latest.id;
    else if (followed && selected === latest.id && !followBlocked && (latest.why !== previous?.why || latest.summary !== previous?.summary || latest.status !== previous?.status)) {
      reveal = { id: latest.id, version: ++revealVersion };
    }
  }
  function changeDesign(value: Design) { design = value; saveDesign(value); }
  function resetRequests() { requestEpoch++; contextRequests = []; requestStatus = ''; asking = false; }
  function toggleHistory(value = !showHistory) {
    showHistory = value;
    if (!value && !windows.current.some(step => step.id === selected)) {
      note = ''; noteStatus = ''; noteOpen = false; typing = false;
    }
    try { localStorage.setItem(historyKey(), String(value)); } catch { /* History is still available in this view. */ }
  }

  async function askContext() {
    if (!chosen || asking || chosenRequest || !connected || updateMode === 'disabled') return;
    const target = events.find(event => event.id === (chosen.reference_id ?? chosen.id));
    if (!target) return;
    asking = true; requestStatus = '';
    const targetSession = session, targetStep = chosen.id;
    const after = events.at(-1)?.id ?? '0';
    const missing = Boolean(chosen.explanation_pending);
    const epoch = requestEpoch;
    const queued: ContextRequest = { session, step: targetStep, reference: target.id, after, missing, node: target.node_name, action: target.action };
    contextRequests = [...contextRequests, queued];
    try {
      const exactReporter = [...events].reverse().find(event => event.source === 'agent' && event.session_id === target.session_id && event.node_name === target.node_name && event.action === target.action && (event.context_id === target.id || event.change_id === target.id))?.agent_id;
      const reporter = missing ? '*' : exactReporter ?? (target.source === 'agent' ? target.agent_id : '*');
      const text = (missing ? 'Explain why you made this exact saved-file change. Only answer if this change is attributable to your task; otherwise leave it pending. ' : 'Explain this retained step more clearly. ') + 'Reply using report_event with context_id="' + target.id + '", the node from this note, action="' + target.action + '", a short summary and ' + (missing ? 'a brief user-facing why. ' : 'a fuller user-facing why. ') + 'Update this step only; do not generate other file explanations.';
      const response = await fetch('/api/feedback', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ session_id: session, node_name: target.node_name, target_agent: reporter, kind: 'question', text }) });
      if (!response.ok) throw new Error(missing ? 'Could not request an explanation.' : 'Could not ask for more context.');
      if (epoch === requestEpoch && session === targetSession && selected === targetStep && contextRequests.some(request => request.reference === target.id)) requestStatus = 'Asked. The agent can respond at its next check-in.';
    } catch (cause) {
      if (epoch === requestEpoch) {
        contextRequests = contextRequests.filter(request => request.reference !== target.id || request.session !== targetSession);
        if (session === targetSession && selected === targetStep) requestStatus = cause instanceof Error ? cause.message : 'Could not send the request.';
      }
    } finally { if (epoch === requestEpoch) asking = false; }
  }

  async function changeUpdates(mode: UpdateMode) {
    error = '';
    const previousMode = updateMode;
    try {
      const response = await fetch('/api/settings', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ update_mode: mode }) });
      if (!response.ok) throw new Error('Could not change updates.');
      const settings = await response.json();
      if (!isUpdateMode(settings.update_mode)) throw new Error('Could not change updates.');
      if (settings.update_mode !== previousMode) { toggleHistory(false); note = ''; noteStatus = ''; noteOpen = false; typing = false; diffOpen = false; resetRequests(); resetFollow(); }
      updateMode = settings.update_mode;
    } catch (cause) { error = cause instanceof Error ? cause.message : 'Could not change updates.'; throw cause; }
  }

  function select(id: string) {
    resetFollow();
    selected = selected === id ? null : id;
    diffOpen = false;
    requestStatus = '';
    note = ''; noteStatus = ''; noteOpen = false;
    typing = false;
    rememberSelection();
  }
  function dismissSelection() {
    if (!selected || diffOpen) return;
    if (pendingFollow && (noteOpen || typing || note.trim())) {
      note = ''; noteStatus = ''; noteOpen = false; typing = false;
    } else select(selected);
  }
  async function clear() {
    if (busy || !session) return;
    busy = true; error = '';
    try {
      const response = await fetch('/api/sessions/clear', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ session_id: session }) });
      if (!response.ok) throw new Error('Could not clear this workflow.');
      const snapshot = await response.json();
      if (snapshot.session_id === session) { events = []; selected = null; note = ''; noteStatus = ''; noteOpen = false; typing = false; diffOpen = false; resetRequests(); resetFollow(); showHistory = false; rememberSelection(); }
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
      note = ''; typing = false; noteStatus = 'Queued for the agent’s next check-in.';
    } catch (cause) { noteStatus = cause instanceof Error ? cause.message : 'Could not send your note.'; }
    finally { busy = false; }
  }
  onMount(() => {
    const disconnect = connectStream(data => {
      const previous = workflowWindows(events, updateMode, updatesFrom).current.at(-1);
      if (data.type === 'snapshot') {
        const boundary = data.updates_from && /^\d+$/.test(data.updates_from) ? data.updates_from : '0';
        const windowChanged = boundary !== updatesFrom || (isUpdateMode(data.update_mode) && data.update_mode !== updateMode);
        if (windowChanged) { showHistory = false; note = ''; noteStatus = ''; noteOpen = false; typing = false; diffOpen = false; resetRequests(); resetFollow(); }
        updatesFrom = boundary;
        if (isUpdateMode(data.update_mode)) updateMode = data.update_mode;
        const id = data.session_id ?? 'local';
        const changed = id !== session;
        const resumed = changed && Boolean(session);
        session = id; events = data.events ?? [];
        const retainedRequests = contextRequests.filter(request => events.some(event => event.id === request.reference));
        if (retainedRequests.length !== contextRequests.length) {
          if (!retainedRequests.length) resetRequests();
          else contextRequests = retainedRequests;
        }
        if (changed) { selected = localStorage.getItem(selectionKey()); note = ''; noteStatus = ''; noteOpen = false; typing = false; diffOpen = false; resetRequests(); resetFollow(); showHistory = localStorage.getItem(historyKey()) === 'true'; }
        else if (windowChanged) toggleHistory(false);
        if (!workflowSteps(events).some(step => step.id === selected)) { selected = null; requestStatus = ''; note = ''; noteStatus = ''; noteOpen = false; typing = false; diffOpen = false; resetFollow(); rememberSelection(); }
        if (resumed && selected && !workflowWindows(events, updateMode, updatesFrom).current.some(step => step.id === selected)) toggleHistory(true);
        if (changed) {
          if (selected) reveal = { id: selected, version: ++revealVersion };
          else pendingFollow = workflowWindows(events, updateMode, updatesFrom).current.at(-1)?.id ?? null;
        } else if (!windowChanged) followUpdate(previous);
      } else if (!data.event.session_id || data.event.session_id === session) {
        events = mergeEvents(events, [data.event]);
        followUpdate(previous);
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
    <GraphCanvas {steps} {selected} {reveal} onselect={select} {connected} detailHeight={design === 'interactive' ? 38 : (noteOpen ? 208 : 144) + (chosen?.explanation_pending || chosenRequest || requestStatus ? 52 : 0)}>
      {#snippet detail(step)}
        {#if design === 'minimal'}
        <section class="step-detail" aria-label="Why this step">
          <p>{step.why || (step.source === 'watcher' ? 'The agent hasn’t explained this save yet.' : 'The agent hasn’t explained this step yet.')}</p>
          <small class="file" title={step.node_name}>{step.node_name === '@workflow' ? 'Current task' : step.node_name}</small>
          {#if step.explanation_pending}
            <button class="request-explanation" onclick={askContext} disabled={!connected || asking || Boolean(chosenRequest)}>{asking || chosenRequest ? 'Request queued' : 'Request explanation'}</button>
          {/if}
          {#if requestStatus}<small class="request-status" role="status">{requestStatus}</small>{/if}
          {#if hasFileDiff(step)}<button class="show-diff" onclick={() => diffOpen = true}>Show diff</button>{/if}
          {#if embedded && step.node_name !== '@workflow'}<button class="open-file" onclick={() => sendToHost({ type: 'stef.openFile', node: step.node_name })}>Open {shortName(step.node_name)}</button>{/if}
          {#if !noteOpen}<button class="suggest" onclick={() => noteOpen = true}>Suggest</button>{:else}
            <form onsubmit={suggest}><input aria-label="Suggest something to the agent" placeholder="Suggest something…" bind:value={note} onfocus={() => typing = true} onblur={() => typing = false} maxlength="500" /><button aria-label="Send suggestion" disabled={!note.trim() || busy || !connected}>↑</button></form>
            {#if noteStatus}<small role="status">{noteStatus}</small>{/if}
          {/if}
        </section>
        {/if}
      {/snippet}
    </GraphCanvas>
    {#if design === 'interactive'}
      <div class="interactive-detail">
        {#if chosen}
          <ChangeVisual step={chosen} {steps} onselect={select} ondiff={() => diffOpen = true} onexplain={askContext} {connected} asking={asking || Boolean(chosenRequest)} {requestStatus} />
          <section class="step-detail" aria-label="Suggest for this step">
            {#if embedded && chosen.node_name !== '@workflow'}<button class="open-file" onclick={() => sendToHost({ type: 'stef.openFile', node: chosen.node_name })}>Open {shortName(chosen.node_name)}</button>{/if}
            {#if !noteOpen}<button class="suggest" onclick={() => noteOpen = true}>Suggest</button>{:else}
              <form onsubmit={suggest}><input aria-label="Suggest something to the agent" placeholder="Suggest something…" bind:value={note} onfocus={() => typing = true} onblur={() => typing = false} maxlength="500" /><button aria-label="Send suggestion" disabled={!note.trim() || busy || !connected}>↑</button></form>
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
<svelte:window onkeydown={event => { if (event.key === 'Escape') dismissSelection(); }} />
