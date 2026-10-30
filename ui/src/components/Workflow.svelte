<script lang="ts">
  import { phaseLabel, workflowAgents, type AgentEvent } from '../lib';
  let { events }: { events: AgentEvent[] } = $props();
  const agents = $derived(workflowAgents(events));
  const latestFile = $derived([...events].reverse().find((event) => event.source === 'watcher'));
</script>

<section class="workflow-panel panel" aria-label="Live workflow">
  <div class="workflow-heading"><span class="eyebrow">WHAT’S HAPPENING</span><span class="observation-badge">◎ Passive tracking + hooks</span></div>
  {#if agents.length}
    {#each agents as event (event.agent_id)}
      <div class="workflow-agent">
        <span class="avatar">{event.agent_id.slice(0, 1).toUpperCase()}</span>
        <div class="workflow-current"><strong>{phaseLabel(event.phase)}</strong><span>{event.agent_id}{event.tool ? ' · ' + event.tool : ''}</span>{#if event.node_name !== '@workflow'}<small>{event.node_name}</small>{/if}</div>
        <div class="workflow-stages" aria-label="Observed stage"><span class:stage-active={event.phase === 'reading'}>Read</span><i>→</i><span class:stage-active={event.phase === 'editing'}>Edit</span><i>→</i><span class:stage-active={event.phase === 'testing'}>Test</span></div>
      </div>
    {/each}
  {:else}
    <div class="workflow-observation"><i class="signal-dot"></i><div><strong>{latestFile ? 'File changes observed' : 'Listening for project activity'}</strong><span>{latestFile ? latestFile.summary + ' · ' + latestFile.node_name : 'File tracking works with any local agent. Add a supported hook for tool-level activity.'}</span></div></div>
  {/if}
</section>
