export interface AgentEvent {
  session_id?: string;
  id: string;
  timestamp: string;
  agent_id: string;
  node_name: string;
  action: 'scouted' | 'created' | 'modified' | 'deleted' | 'tested';
  summary: string;
  why: string;
  status: 'running' | 'ready';
  source: 'agent' | 'watcher' | 'demo' | 'hook' | 'human';
  category?: 'activity' | 'feedback';
  target_agent?: string;
  note_kind?: 'question' | 'suggestion' | 'concern';
  phase?: string;
  tool?: string;
  kind?: string;
  language?: string;
  line?: number;
  before?: string;
  after?: string;
  diff?: string;
  parents?: string[];
  collisions?: string[];
  change_id?: string;
  explanation_pending?: boolean;
}

export type UpdateMode = 'milestones' | 'files' | 'disabled';
export const isUpdateMode = (value: unknown): value is UpdateMode => ['milestones', 'files', 'disabled'].includes(value as string);

export type StreamMessage =
  | { type: 'snapshot'; repo: string; session_id?: string; events?: AgentEvent[]; update_mode?: UpdateMode }
  | { type: 'event'; session_id?: string; event: AgentEvent };

export const HISTORY_LIMIT = 5000;
export const REPLAY_MS = 6000;

export function isFileChange(event: AgentEvent): boolean {
  return event.source === 'watcher' && !event.category && event.kind === 'file' && !event.node_name.includes('#');
}

// Each saved file revision remains a step. Only an exact agent reference supplies its why.
export function workflowSteps(events: AgentEvent[], mode: UpdateMode = 'files'): AgentEvent[] {
  if (mode === 'disabled') return [];
  if (mode === 'milestones') events = events.filter(event => event.source === 'agent' && !event.change_id);
  const steps: AgentEvent[] = [];
  const changes = new Map(events.filter(isFileChange).map(event => [event.id, event]));
  const explanations = new Map<string, AgentEvent>();
  for (const event of events) {
    const change = event.change_id ? changes.get(event.change_id) : undefined;
    if (change && event.source === 'agent' && event.session_id === change.session_id && event.node_name === change.node_name && event.action === change.action) explanations.set(change.id, event);
  }
  const brief = (why: string) => {
    const words = why.trim().split(/\s+/);
    return words.slice(0, 24).join(' ') + (words.length > 24 ? '…' : '');
  };
  for (const event of events) {
    if (event.source === 'human' || event.category === 'feedback' || event.change_id) continue;
    if (event.source === 'watcher') {
      if (!isFileChange(event)) continue;
      const explanation = explanations.get(event.id);
      steps.push({ ...event, change_id: event.id, summary: shortName(event.node_name) + ' · ' + (explanation?.summary ?? event.action), why: explanation ? brief(explanation.why) : '', parents: explanation?.parents ?? [], explanation_pending: !explanation });
      continue;
    }
    const hook = event.source === 'hook';
    if (hook && !['reading', 'editing', 'testing', 'failed'].includes(event.phase ?? '')) continue;
    if (hook && event.node_name === '@workflow' && event.phase !== 'testing' && event.phase !== 'failed') continue;
    const action = hook && event.phase === 'editing' ? 'modified' : hook && event.phase === 'reading' ? 'scouted' : hook && event.phase === 'testing' ? 'tested' : event.action;
    const why = hook ? '' : brief(event.why);
    const step: AgentEvent = { ...event, action, why };
    const previous = steps.at(-1);
    const genericTest = previous && action === 'tested' && previous.action === 'tested' && ((hook && step.phase === 'testing' && step.node_name === '@workflow') || (previous.source === 'hook' && previous.phase === 'testing' && previous.node_name === '@workflow'));
    const sameNode = previous && (genericTest || previous.node_name === step.node_name || ((previous.source === 'hook' || hook) && previous.node_name.split('#')[0] === step.node_name.split('#')[0]));
    if (previous && previous.source !== 'watcher' && sameNode && previous.action === action && (previous.agent_id === step.agent_id || previous.source === 'hook' || hook)) {
      steps[steps.length - 1] = hook && previous.source === 'agent' ? previous : { ...step, id: previous.id, parents: [...new Set([...(previous.parents ?? []), ...(step.parents ?? [])])] };
    } else steps.push(step);
  }
  return steps;
}

export function workflowEdges(steps: AgentEvent[]): { from: string; to: string; explicit: boolean }[] {
  const edges = new Map<string, { from: string; to: string; explicit: boolean }>();
  const visited = new Map<string, AgentEvent>();
  const files = new Map<string, AgentEvent>();
  let previous: AgentEvent | undefined;
  for (const step of steps) {
    if (previous) edges.set(previous.id + ':' + step.id, { from: previous.id, to: step.id, explicit: false });
    for (const parent of step.parents ?? []) {
      const from = visited.get(parent) ?? (!parent.includes('#') ? files.get(parent) : undefined);
      if (from) edges.set(from.id + ':' + step.id, { from: from.id, to: step.id, explicit: true });
    }
    visited.set(step.node_name, step); files.set(step.node_name.split('#')[0], step); previous = step;
  }
  return [...edges.values()];
}

export function mergeEvents(current: AgentEvent[], incoming: AgentEvent[]): AgentEvent[] {
  const merged = new Map(current.map((event) => [event.id, event]));
  for (const event of incoming) merged.set(event.id, event);
  return [...merged.values()].sort((a, b) => Number(a.id) - Number(b.id)).slice(-HISTORY_LIMIT);
}

// Compress the captured timeline into six seconds, preserving real timing ratios.
export function eventsAt(events: AgentEvent[], progress: number): AgentEvent[] {
  if (!events.length || progress <= 0) return [];
  if (progress >= 1) return events;
  const first = Date.parse(events[0].timestamp);
  const last = Date.parse(events[events.length - 1].timestamp);
  if (last <= first) return events.slice(0, Math.ceil(events.length * progress));
  const cutoff = first + (last - first) * progress;
  return events.filter((event) => Date.parse(event.timestamp) <= cutoff);
}

export function latestNodes(events: AgentEvent[]): AgentEvent[] {
  const nodes = new Map<string, AgentEvent>();
  for (const event of events) if (!event.category) nodes.set(event.node_name, event);
  return [...nodes.values()];
}

export function activityEdges(events: AgentEvent[]): { from: string; to: string; explicit: boolean }[] {
  const edges = new Map<string, { from: string; to: string; explicit: boolean }>();
  const previous = new Map<string, string>();
  for (const event of events) {
    if (event.category) continue;
    const parents = event.parents?.length ? event.parents : event.source === 'watcher' ? [] : [previous.get(event.agent_id)].filter((p): p is string => Boolean(p));
    for (const from of parents) {
      if (from !== event.node_name) edges.set(from + '→' + event.node_name, { from, to: event.node_name, explicit: Boolean(event.parents?.length) });
    }
    previous.set(event.agent_id, event.node_name);
  }
  return [...edges.values()];
}

export function workflowAgents(events: AgentEvent[]): AgentEvent[] {
  const agents = new Map<string, AgentEvent>();
  for (const event of events) if (event.category === 'activity') agents.set(event.agent_id, event);
  return [...agents.values()].slice(-8).reverse();
}
export function phaseLabel(phase?: string): string {
  const labels: Record<string, string> = { started: 'Session started', reading: 'Reading project context', editing: 'Applying code changes', testing: 'Running test command', terminal: 'Running terminal command', tool_done: 'Tool finished', completed: 'Agent turn finished', failed: 'Tool reported a failure' };
  return labels[phase ?? ''] ?? 'Activity observed';
}

export function activeCollisions(events: AgentEvent[], now = Date.now()): AgentEvent[] {
  const last = new Map<string, AgentEvent>();
  for (const event of events) {
    if (event.source !== 'agent' && event.source !== 'demo') continue;
    const file = event.node_name.split('#')[0];
    const key = file + '::' + event.agent_id;
    if (event.status === 'ready' || event.action === 'deleted') last.delete(key);
    else if (event.action === 'modified' || event.action === 'created') last.set(key, event);
  }
  return [...last.values()].filter((event) => {
    if (!event.collisions?.length || event.status === 'ready' || event.action === 'deleted' || now - Date.parse(event.timestamp) > 30000) return false;
    const file = event.node_name.split('#')[0];
    return event.collisions.some((agent) => {
      const other = last.get(file + '::' + agent);
      return other && other.status !== 'ready' && other.action !== 'deleted' && now - Date.parse(other.timestamp) <= 30000;
    });
  });
}

export function shortName(node: string): string {
  return node.includes('#') ? node.split('#').slice(1).join('#') : node.split('/').pop() ?? node;
}
export function timeLabel(timestamp: string): string {
  return new Date(timestamp).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' });
}
