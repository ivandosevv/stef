<script lang="ts">
  import { onMount, type Snippet } from 'svelte';
  import { workflowEdges, type AgentEvent } from '../lib';
  let { steps, selected, onselect, connected, detail, detailHeight = 144 }: {
    steps: AgentEvent[]; selected: string | null; onselect: (id: string) => void;
    connected: boolean; detail: Snippet<[AgentEvent]>; detailHeight?: number;
  } = $props();
  let viewport: HTMLDivElement;
  let availableWidth = $state(560);
  const width = $derived(Math.max(240, Math.min(availableWidth, 560)));
  const nodeWidth = $derived(Math.min(320, width - 56));
  const positions = $derived.by(() => {
    let y = 22;
    return new Map(steps.map(step => {
      const height = selected === step.id ? detailHeight : 38;
      const position = { x: (width - nodeWidth) / 2, y, height };
      y += height + 28;
      return [step.id, position];
    }));
  });
  const height = $derived(steps.length ? positions.get(steps[steps.length - 1].id)!.y + positions.get(steps[steps.length - 1].id)!.height + 24 : 220);
  const edges = $derived(workflowEdges(steps));
  function arrow(from: string, to: string): string {
    const a = positions.get(from)!; const b = positions.get(to)!;
    const x = a.x + nodeWidth / 2;
    const ay = a.y + a.height; const by = b.y;
    if (by - ay > 28) {
      const lane = a.x - 16;
      return 'M ' + x + ' ' + ay + ' V ' + (ay + 12) + ' H ' + lane + ' V ' + (by - 12) + ' H ' + x + ' V ' + by;
    }
    return 'M ' + x + ' ' + ay + ' V ' + by;
  }
  onMount(() => { const observer = new ResizeObserver(() => availableWidth = viewport.clientWidth); observer.observe(viewport); return () => observer.disconnect(); });
</script>
<div class="workflow-canvas" bind:this={viewport}>
  {#if steps.length}
    <svg viewBox={'0 0 ' + width + ' ' + height} width={width} height={height} aria-label="Workflow steps and arrows">
      <defs><marker id="workflow-arrow" markerWidth="7" markerHeight="7" refX="6" refY="3.5" orient="auto"><path class="flow-arrowhead" d="M1 1L6 3.5L1 6" /></marker></defs>
      {#each edges as edge (edge.from + ':' + edge.to)}
        <path class="flow-arrow" class:sequence={!edge.explicit} d={arrow(edge.from, edge.to)} marker-end="url(#workflow-arrow)" data-testid="workflow-arrow"><title>{edge.explicit ? 'Connection reported by the agent' : 'Next step in this chat'}</title></path>
      {/each}
      {#each steps as step (step.id)}
        {@const position = positions.get(step.id)!}
        <foreignObject x={position.x} y={position.y} width={nodeWidth} height={position.height}>
          <button class="step" class:selected={selected === step.id} onclick={() => onselect(step.id)} title={step.node_name + (step.why ? ' — ' + step.why : step.explanation_pending ? ' — Waiting for the agent’s explanation.' : '')} data-testid="workflow-step" data-change-id={step.change_id} data-explanation-pending={step.explanation_pending} aria-pressed={selected === step.id}>
            {step.summary}
          </button>
          {#if selected === step.id}{@render detail(step)}{/if}
        </foreignObject>
      {/each}
    </svg>
  {:else}<div class="empty"><p>{connected ? 'Listening…' : 'Connecting…'}</p></div>{/if}
</div>
