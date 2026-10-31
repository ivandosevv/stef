<script lang="ts">
  import { shortName, timeLabel, type AgentEvent } from '../lib';
  let { event, history, onselect, onopen }: { event: AgentEvent | undefined; history: AgentEvent[]; onselect: (name: string) => void; onopen?: (name: string) => void } = $props();
  const trail = $derived(event ? history.filter((item) => item.node_name === event.node_name).slice(-8).reverse() : []);
  const codeEvent = $derived(event ? [...history].reverse().find((item) => item.node_name === event.node_name && (item.diff || item.after || item.before)) : undefined);
  const displayDiff = $derived(codeEvent?.diff ?? codeEvent?.after ?? codeEvent?.before);
</script>

<aside class="decision-panel panel" aria-label="Decision inspector">
  <div class="panel-heading"><span class="eyebrow">THE WHY TRAIL</span><span class="tiny-tag">{event?.source === 'watcher' ? 'OBSERVED' : event?.source === 'demo' ? 'DEMO' : 'AGENT CONTEXT'}</span></div>
  {#if event}
    <div class="decision-body">
      <div class="decision-symbol">{event.kind === 'type' ? '◇' : 'ƒ'}</div>
      <h2>{shortName(event.node_name)}</h2>
      <p class="decision-path">{event.node_name}{event.line ? ':' + event.line : ''}</p>
      {#if onopen}<button class="text-button" onclick={() => onopen?.(event!.node_name)}>Open in editor ↗</button>{/if}
      <div class="decision-byline"><span class="avatar small">{event.agent_id.slice(0, 1).toUpperCase()}</span><span>{event.agent_id}</span><span class="muted">{timeLabel(event.timestamp)}</span></div>
      <div class="rationale"><span class="eyebrow">{event.source === 'watcher' ? 'FILESYSTEM OBSERVATION' : 'DECISION RATIONALE'}</span><p>{event.why}</p></div>
      <div class="diff-header"><strong>{codeEvent && !codeEvent.diff ? 'Code snapshot' : codeEvent?.kind === 'file' ? 'File diff' : 'Code diff'}</strong><span>{codeEvent ? codeEvent.language ?? 'code' : 'awaiting code'}</span></div>
      {#if displayDiff}
        <pre class="code-diff" aria-label="Code diff">{#each displayDiff.split('\n') as line, index (index)}<span class:added={line.startsWith('+') && !line.startsWith('+++')} class:removed={line.startsWith('-') && !line.startsWith('---')} class:hunk={line.startsWith('@@')}>{line || ' '}</span>{/each}</pre>
      {:else}
        <div class="no-diff">No code attached yet. File tracking adds diffs when this node changes in the watched repository.</div>
      {/if}
      <div class="trail-heading"><strong>Node history</strong><span>{trail.length} recent events</span></div>
      <div class="node-trail">{#each trail as item (item.id)}<div><i class:tested={item.action === 'tested'}></i><span><strong>{item.summary}</strong><small>{item.agent_id} · {timeLabel(item.timestamp)}</small></span></div>{/each}</div>
    </div>
  {:else}
    <div class="inspector-empty"><div class="inspector-art">⌖</div><h2>Every change has a story.</h2><p>Select a node to see the decision behind it, inspect the code, and follow its history.</p></div>
    {#if history.length}<div class="recent-picks"><span class="eyebrow">RECENT CHANGES</span>{#each history.slice(-4).reverse() as item (item.id)}<button onclick={() => onselect(item.node_name)}><span>{shortName(item.node_name)}</span><small>{item.summary}</small></button>{/each}</div>{/if}
  {/if}
</aside>
