<script lang="ts">
  import { onMount, tick } from 'svelte';
  import GraphCanvas from './components/GraphCanvas.svelte';
  import ThemePicker from './components/ThemePicker.svelte';
  import UpdatesPicker from './components/UpdatesPicker.svelte';
  import FileDiff from './components/FileDiff.svelte';
  import DesignPicker from './components/DesignPicker.svelte';
  import WorkspacePicker from './components/WorkspacePicker.svelte';
  import ChangeVisual from './components/ChangeVisual.svelte';
  import { readDesign, saveDesign, type Design } from './design';
  import { hasFileDiff } from './diff';
  import { isWorkspaceList } from './stream-security';
  import { connectStream, isEmbedded, sendToHost } from './transport';
  import { stepReason, workflowWindows, mergeEvents, shortName, isUpdateMode, type AgentEvent, type UpdateMode, type Workspace, type StreamMessage } from './lib';

  let events = $state<AgentEvent[]>([]);
  let session = $state('');
  let origin = $state('');
  let workspaces = $state<Workspace[]>([]);
  let disconnectStream: (() => void) | undefined;
  let streamEpoch = 0;
  let disposed = false;
  const workspaceKey = 'stef-workspace:' + location.host;
  const shared = $derived(Boolean(workspaces.find(workspace => workspace.workspace_id === session)?.shared));
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
  let inspecting = $state(false);
  let following = $state(true);
  let detailsViewport = $state<HTMLDivElement>();
  let detailsKey = '';
  let noteInput = $state<HTMLInputElement>();
  let followed = $state(false);
  let pendingFollow = $state<string | null>(null);
  let reveal = $state<{ id: string; version: number } | null>(null);
  let revealVersion = 0;
  const embedded = isEmbedded();
  const windows = $derived(workflowWindows(events, updateMode, updatesFrom));
  const steps = $derived(showHistory ? [...windows.saved, ...windows.current] : windows.current);
  const chosen = $derived(steps.find(step => step.id === selected));
  const chosenRequest = $derived(contextRequests.find(request => request.session === session && request.reference === (chosen?.reference_id ?? chosen?.id)));
  const followBlocked = $derived(!following || diffOpen || typing || Boolean(note.trim()) || inspecting);
  $effect(() => {
    const viewport = detailsViewport;
    const key = session + ':' + stepKey(chosen);
    if (!viewport || key === detailsKey) return;
    detailsKey = key;
    viewport.scrollTo({ top: 0, behavior: 'auto' });
  });
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
  const followingKey = () => 'stef-follow:' + session + ':' + updatesFrom;
  function rememberSelection() {
    try {
      if (selected) localStorage.setItem(selectionKey(), selected);
      else localStorage.removeItem(selectionKey());
    } catch { /* Selection still works when view storage is unavailable. */ }
  }
  function rememberFollowing() {
    try { localStorage.setItem(followingKey(), String(following)); } catch { /* Following still works without view storage. */ }
  }
  function pauseFollowing() { following = false; followed = false; reveal = null; rememberFollowing(); }
  function resetFollow(fresh = false) { pendingFollow = null; reveal = null; followed = false; inspecting = false; if (fresh) following = true; }
  const stepKey = (step?: AgentEvent) => step ? step.id + ':' + (step.reference_id ?? step.id) : '';
  function followUpdate(previous?: AgentEvent) {
    const latest = workflowWindows(events, updateMode, updatesFrom).current.at(-1);
    if (!latest) return;
    if (stepKey(latest) !== stepKey(previous)) pendingFollow = latest.id;
    else if (followed && selected === latest.id && !followBlocked && (latest.why !== previous?.why || latest.summary !== previous?.summary || latest.status !== previous?.status)) {
      reveal = { id: latest.id, version: ++revealVersion };
    }
  }
  function changeDesign(value: Design) { design = value; if (value === 'minimal') inspecting = false; saveDesign(value); if (selected) reveal = { id: selected, version: ++revealVersion }; }
  function inspectChange() { inspecting = true; pauseFollowing(); }
  function resumeFollowing() {
    const latest = windows.current.at(-1);
    if (diffOpen || note.trim()) return;
    following = true; rememberFollowing();
    inspecting = false; typing = false;
    pendingFollow = latest?.id ?? null;
  }
  function openDiff() { pauseFollowing(); diffOpen = true; }
  function openSuggestion() { pauseFollowing(); noteOpen = true; }
  function openFile(node: string) { pauseFollowing(); sendToHost({ type: 'stef.openFile', node }); }
  function suggestSelection(text: string) {
    if (!chosen || note.trim()) return;
    inspectChange(); noteOpen = true; note = text.slice(0, 500); noteStatus = '';
    void tick().then(() => {
      noteInput?.focus({ preventScroll: true });
      if (!noteInput || !detailsViewport) return;
      const input = noteInput.getBoundingClientRect(), pane = detailsViewport.getBoundingClientRect();
      const delta = input.top < pane.top ? input.top - pane.top - 12 : input.bottom > pane.bottom ? input.bottom - pane.bottom + 12 : 0;
      if (delta) detailsViewport.scrollTo({ top: detailsViewport.scrollTop + delta, behavior: 'auto' });
    });
  }
  function resetRequests() { requestEpoch++; contextRequests = []; requestStatus = ''; asking = false; }
  function toggleHistory(value = !showHistory) {
    showHistory = value;
    if (!value && !windows.current.some(step => step.id === selected)) {
      inspecting = false;
      note = ''; noteStatus = ''; noteOpen = false; typing = false;
    }
    try { localStorage.setItem(historyKey(), String(value)); } catch { /* History is still available in this view. */ }
  }

  async function askContext() {
    if (!chosen || asking || chosenRequest || !connected || updateMode === 'disabled') return;
    const target = events.find(event => event.id === (chosen.reference_id ?? chosen.id));
    if (!target) return;
    pauseFollowing();
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
      const response = await fetch('/api/settings', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ update_mode: mode, session_id: session }) });
      if (!response.ok) throw new Error('Could not change updates.');
      const settings = await response.json();
      if (!isUpdateMode(settings.update_mode)) throw new Error('Could not change updates.');
      if (settings.update_mode !== previousMode) { toggleHistory(false); note = ''; noteStatus = ''; noteOpen = false; typing = false; diffOpen = false; resetRequests(); resetFollow(true); }
      updateMode = settings.update_mode;
      if (settings.update_mode !== previousMode) rememberFollowing();
    } catch (cause) { error = cause instanceof Error ? cause.message : 'Could not change updates.'; throw cause; }
  }

  function select(id: string) {
    const wasFollowing = following;
    resetFollow(); pauseFollowing();
    selected = selected === id && !wasFollowing ? null : id;
    diffOpen = false;
    requestStatus = '';
    note = ''; noteStatus = ''; noteOpen = false;
    typing = false;
    rememberSelection();
  }
  function dismissSelection() {
    if (!selected || diffOpen) return;
    if (pendingFollow && (noteOpen || typing || note.trim() || inspecting)) {
      inspecting = false;
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
      if (snapshot.session_id === session) { events = []; selected = null; note = ''; noteStatus = ''; noteOpen = false; typing = false; diffOpen = false; resetRequests(); resetFollow(true); rememberFollowing(); showHistory = false; rememberSelection(); }
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
  function receiveStream(data: StreamMessage) {
      if (data.type === 'workspaces') { workspaces = data.workspaces; return; }
      if (data.type === 'snapshot' && data.workspaces) workspaces = data.workspaces;
      if (data.type === 'snapshot' && origin && data.session_id !== origin && !workspaces.some(room => room.workspace_id === data.session_id && room.members.includes(origin))) return;
      const previous = workflowWindows(events, updateMode, updatesFrom).current.at(-1);
      if (data.type === 'snapshot') {
        const boundary = data.updates_from && /^\d+$/.test(data.updates_from) ? data.updates_from : '0';
        const windowChanged = boundary !== updatesFrom || (isUpdateMode(data.update_mode) && data.update_mode !== updateMode);
        if (windowChanged) { showHistory = false; note = ''; noteStatus = ''; noteOpen = false; typing = false; diffOpen = false; resetRequests(); resetFollow(true); }
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
        if (changed) { selected = localStorage.getItem(selectionKey()); note = ''; noteStatus = ''; noteOpen = false; typing = false; diffOpen = false; resetRequests(); resetFollow(true); showHistory = localStorage.getItem(historyKey()) === 'true';
          try { following = localStorage.getItem(followingKey()) !== 'false'; } catch { /* Fresh views follow by default. */ }
        } else if (windowChanged) { toggleHistory(false); rememberFollowing(); }
        const restoredWindows = workflowWindows(events, updateMode, updatesFrom);
        if (![...restoredWindows.saved, ...restoredWindows.current].some(step => step.id === selected)) { selected = null; requestStatus = ''; note = ''; noteStatus = ''; noteOpen = false; typing = false; diffOpen = false; resetFollow(); rememberSelection(); }
        if (resumed && selected && !restoredWindows.current.some(step => step.id === selected)) toggleHistory(true);
        if (changed) {
          const latest = restoredWindows.current.at(-1);
          if (following && latest) pendingFollow = latest.id;
          else if (selected) {
            reveal = { id: selected, version: ++revealVersion };
            if (latest && latest.id !== selected) pendingFollow = latest.id;
          } else pendingFollow = latest?.id ?? null;
        } else if (windowChanged) pendingFollow = restoredWindows.current.at(-1)?.id ?? null;
        else followUpdate(previous);
      } else if (!data.event.session_id || data.event.session_id === session) {
        events = mergeEvents(events, [data.event]);
        followUpdate(previous);
      }
  }
  function selectWorkspace(id: string) {
    if (!id || disposed) return;
    const epoch = ++streamEpoch;
    disconnectStream?.(); connected = false; origin = id;
    try { localStorage.setItem(workspaceKey, id); } catch { /* Selection remains local to this view. */ }
    const url = new URL(location.href); url.searchParams.set('session_id', id);
    history.replaceState(null, '', url);
    disconnectStream = connectStream(data => { if (epoch === streamEpoch) receiveStream(data); }, online => { if (epoch === streamEpoch) connected = online; }, id);
  }
  async function loadWorkspaces() {
    const response = await fetch('/api/workspaces');
    if (!response.ok) throw new Error('Could not load workspaces.');
    const catalog = await response.json();
    if (!isWorkspaceList(catalog.workspaces)) throw new Error('Could not load workspaces.');
    workspaces = catalog.workspaces;
    return String(catalog.current_workspace_id ?? 'local');
  }
  async function changeSharing(action: 'create' | 'join' | 'leave', workspaceID?: string) {
    const current = workspaces.find(room => room.workspace_id === session);
    if (!current?.members.includes(origin)) throw new Error('Select an agent before changing sharing.');
    const response = await fetch('/api/workspaces', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ session_id: origin, action, ...(workspaceID ? { workspace_id: workspaceID } : {}) }) });
    if (!response.ok) throw new Error(action === 'join' ? 'That sharing ID could not be joined.' : 'Could not change sharing.');
    const receipt = await response.json();
    await loadWorkspaces();
    selectWorkspace(String(receipt.session_id ?? origin));
  }
  onMount(() => {
    void (async () => {
      let current = 'local';
      try { current = await loadWorkspaces(); } catch { /* Older local engines still expose a workflow stream. */ }
      if (disposed) return;
      const requested = new URL(location.href).searchParams.get('session_id');
      let saved: string | null = null;
      try { saved = localStorage.getItem(workspaceKey); } catch { /* Use the URL or latest workspace. */ }
      const remembered = saved && workspaces.some(room => room.workspace_id === saved || room.members.includes(saved)) ? saved : null;
      selectWorkspace(requested ?? remembered ?? current);
    })();
    void (window as NativeWindow).stefNativeReady?.();
    return () => { disposed = true; streamEpoch++; disconnectStream?.(); };
  });
</script>

<div class="stef-app" data-update-mode={updateMode} data-design={design} data-following={following}>
  <header><div class="brand-group"><span class="brand">stef</span><WorkspacePicker {origin} workspaceID={session} {workspaces} {connected} onselect={selectWorkspace} onchange={changeSharing} /></div><div class="header-actions"><UpdatesPicker mode={updateMode} {connected} onchange={changeUpdates} /><DesignPicker {design} onchange={changeDesign} /><ThemePicker /><button class="clear" onclick={clear} disabled={busy || !events.length} aria-label="Clear this chat’s workflow" title={shared ? 'Clear this shared workflow for all members' : 'Clear this chat’s workflow'}>↺</button></div></header>
  <main aria-label="Workflow">
    {#if error}<p class="notice" role="status">{error}</p>{/if}
    {#if updateMode === 'disabled'}<div class="empty"><p>Updates disabled</p></div>{:else}
    {#if windows.saved.length}<div class="history-control"><button onclick={() => { pauseFollowing(); toggleHistory(); }}>{showHistory ? 'Hide saved history' : 'Show saved history'}</button><small>{showHistory ? 'Saved history — viewing it uses no AI calls.' : 'New updates only. Earlier work is saved.'}</small></div>{/if}
    {#if !following}
      <div class="follow-control" role="group" aria-label="Following updates">
        <span>{pendingFollow ? 'New work is waiting.' : 'Paused'}</span>
        <button onclick={resumeFollowing} disabled={diffOpen || Boolean(note.trim())} title={diffOpen ? 'Close the diff first.' : note.trim() ? 'Send or discard your suggestion first.' : 'Select new changes automatically.'}>Follow latest</button>
      </div>
    {/if}
    <div class="workflow-layout" class:interactive={design === 'interactive'}>
    <GraphCanvas {steps} {selected} {reveal} localScroll={design === 'interactive'} onselect={select} {connected} detailHeight={design === 'interactive' ? 38 : (noteOpen ? 208 : 144) + (shared ? 18 : 0) + (chosen?.explanation_pending || chosenRequest || requestStatus ? 52 : 0)}>
      {#snippet detail(step)}
        {#if design === 'minimal'}
        <section class="step-detail" aria-label="Why this step">
          {#if shared}<small class="contributor">{step.agent_id === 'filesystem' ? 'Observed file change' : step.agent_id}</small>{/if}
          <p>{step.why || stepReason(step)}</p>
          <small class="file" title={step.node_name}>{step.node_name === '@workflow' ? 'Current task' : step.node_name}</small>
          {#if step.explanation_pending}
            <button class="request-explanation" onclick={askContext} disabled={!connected || asking || Boolean(chosenRequest)}>{asking || chosenRequest ? 'Request queued' : 'Request explanation'}</button>
          {/if}
          {#if requestStatus}<small class="request-status" role="status">{requestStatus}</small>{/if}
          {#if hasFileDiff(step)}<button class="show-diff" onclick={openDiff}>Show diff</button>{/if}
          {#if embedded && step.node_name !== '@workflow'}<button class="open-file" onclick={() => openFile(step.node_name)}>Open {shortName(step.node_name)}</button>{/if}
          {#if !noteOpen}<button class="suggest" onclick={openSuggestion}>Suggest</button>{:else}
            <form onsubmit={suggest}><input aria-label="Suggest something to the agent" placeholder="Suggest something…" bind:value={note} bind:this={noteInput} onfocus={() => typing = true} onblur={() => typing = false} maxlength="500" /><button aria-label="Send suggestion" disabled={!note.trim() || busy || !connected}>↑</button></form>
            {#if noteStatus}<small role="status">{noteStatus}</small>{/if}
          {/if}
        </section>
        {/if}
      {/snippet}
    </GraphCanvas>
    {#if design === 'interactive'}
      <!-- svelte-ignore a11y_no_noninteractive_tabindex -->
      <div class="interactive-detail" role="region" aria-label="Selected file properties" tabindex="0" bind:this={detailsViewport}>
        {#if chosen}
          {#if shared}<small class="shared-contributor">{chosen.agent_id === 'filesystem' ? 'Observed file change' : chosen.agent_id}</small>{/if}
          <ChangeVisual step={chosen} {steps} oninspect={inspectChange} onsuggest={suggestSelection} onselect={select} ondiff={openDiff} onexplain={askContext} {connected} asking={asking || Boolean(chosenRequest)} {requestStatus} />
          <section class="step-detail" aria-label="Suggest for this step">
            {#if embedded && chosen.node_name !== '@workflow'}<button class="open-file" onclick={() => openFile(chosen.node_name)}>Open {shortName(chosen.node_name)}</button>{/if}
            {#if !noteOpen}<button class="suggest" onclick={openSuggestion}>Suggest</button>{:else}
              <form onsubmit={suggest}><input aria-label="Suggest something to the agent" placeholder="Suggest something…" bind:value={note} bind:this={noteInput} onfocus={() => typing = true} onblur={() => typing = false} maxlength="500" /><button aria-label="Send suggestion" disabled={!note.trim() || busy || !connected}>↑</button></form>
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

<style>
  .brand-group { display: flex; align-items: center; gap: 8px; }
  .contributor { margin-bottom: 4px; }
  .shared-contributor { display: block; color: var(--faint); font-size: 10px; padding: 10px 20px 0; overflow-wrap: anywhere; }
</style>
