<script lang="ts">
  import { activeCollisions, type AgentEvent } from '../lib';
  let { events, now }: { events: AgentEvent[]; now: number } = $props();
  const collisions = $derived(activeCollisions(events, now));
</script>

<div class="collision-radar" class:warning={collisions.length > 0} role="status">
  <span class="radar-icon">{collisions.length ? '△' : '◎'}</span>
  <div><strong>{collisions.length ? 'Shared file activity' : 'Collision radar clear'}</strong><span>{collisions.length ? collisions[0].agent_id + ' + ' + collisions[0].collisions?.join(', ') + ' · ' + collisions[0].node_name.split('#')[0] : 'No overlapping write claims in the last 30s'}</span></div>
</div>
