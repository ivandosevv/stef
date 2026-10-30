<script lang="ts">
  import type { AgentEvent } from '../lib';
  let { node, agents, events, enabled, demo, onsubmit }: {
    node: string | null; agents: string[]; events: AgentEvent[]; enabled: boolean; demo: boolean;
    onsubmit: (note: { node_name: string; target_agent: string; kind: string; text: string }) => Promise<string>;
  } = $props();
  let kind = $state('question');
  let target = $state('*');
  let text = $state('');
  let busy = $state(false);
  let status = $state('');
  let failed = $state(false);
  const notes = $derived(events.filter((event) => event.category === 'feedback').slice(-3).reverse());
  const length = $derived(Array.from(text).length);
  async function submit(event: SubmitEvent) {
    event.preventDefault(); if (busy || !enabled || !text.trim() || length > 500) return;
    busy = true; failed = false; status = '';
    try {
      await onsubmit({ node_name: node ?? '@workflow', target_agent: target, kind, text: text.trim() });
      text = ''; status = demo ? 'Added to this demo. Try the live session to reach your agent.' : 'Queued for the next MCP check-in. Your agent can consider it when it checks in.';
    } catch (error) { failed = true; status = error instanceof Error ? error.message : 'Could not queue your note.'; }
    finally { busy = false; }
  }
</script>

<section class="chip-in panel" aria-label="Chip in">
  <div class="chip-heading"><div><span class="eyebrow">MAKE THIS A CONVERSATION</span><h2>Chip in while they work.</h2></div><span class="tiny-tag">{demo ? 'DEMO' : 'NEXT CHECK-IN'}</span></div>
  <p class="chip-context">{node ? 'About ' + node : 'About the current workflow'} <span>· Questions, suggestions, and concerns stay visible in your session.</span></p>
  <form onsubmit={submit}>
    <div class="chip-controls"><label>Type<select aria-label="Note type" bind:value={kind}><option value="question">Question</option><option value="suggestion">Suggestion</option><option value="concern">Concern</option></select></label><label>For<select aria-label="Note recipient" bind:value={target}><option value="*">All reporting agents</option>{#each agents as agent}<option value={agent}>{agent}</option>{/each}</select></label></div>
    <label class="sr-only" for="stef-note">Your note</label><textarea id="stef-note" placeholder="Is this handling the edge case we discussed?" bind:value={text} rows="3" disabled={!enabled || busy}></textarea>
    <div class="chip-submit"><small>{length}/500</small><button class="primary-button" type="submit" disabled={!enabled || busy || !text.trim() || length > 500}>{busy ? 'Queuing…' : 'Queue note'} <span>↗</span></button></div>
  </form>
  {#if status}<p class="chip-status" class:failed role="status">{status}</p>{/if}
  {#if !enabled}<p class="chip-status">Connect the local engine to leave a note, or explore the demo.</p>{/if}
  {#if notes.length}<div class="chip-notes" aria-label="Recent developer notes">{#each notes as note (note.id)}<article><div><strong>{note.note_kind}</strong><span>{note.target_agent === '*' ? 'All agents' : note.target_agent} · {note.node_name === '@workflow' ? 'Workflow' : note.node_name}</span></div><p>{note.why}</p></article>{/each}</div>{/if}
</section>
