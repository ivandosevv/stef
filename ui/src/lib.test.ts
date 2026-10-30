import { describe, it, expect } from 'vitest';
import { activeCollisions, activityEdges, eventsAt, latestNodes, mergeEvents, workflowAgents, workflowSteps, workflowEdges, type AgentEvent } from './lib';

function event(id: number, node = 'main.go', options: Partial<AgentEvent> = {}): AgentEvent {
  return { id: String(id), timestamp: new Date(100000 + id * 1000).toISOString(), agent_id: 'alpha', node_name: node, action: 'modified', summary: 'Updated the request boundary', why: 'Requests need a clear boundary.', status: 'running', source: 'agent', ...options };
}

describe('simple workflow', () => {
  it('shows explained milestones without file or hook noise and can disable the view', () => {
    const events = [
      event(1, 'main.ts', { source: 'watcher', kind: 'file' }),
      event(2, 'main.ts', { change_id: '1', summary: 'Restore saved colors' }),
      event(3, 'main.ts', { source: 'hook', category: 'activity', phase: 'editing' }),
      event(4, 'main.ts', { summary: 'Add appearance modes', status: 'ready' }),
    ];
    expect(workflowSteps(events, 'files')).toHaveLength(2);
    expect(workflowSteps(events, 'milestones').map(step => step.id)).toEqual(['4']);
    expect(workflowSteps(events, 'milestones')[0].why).toBe('Requests need a clear boundary.');
    expect(workflowSteps(events, 'disabled')).toEqual([]);
    expect(events).toHaveLength(4);
  });
  it('shows every file save before an explanation arrives, without symbol noise', () => {
    const steps = workflowSteps([
      event(1, 'theme.ts', { source: 'watcher', kind: 'file', action: 'created' }),
      event(2, 'theme.ts#applyTheme', { source: 'watcher', kind: 'function' }),
      event(3, 'main.ts', { source: 'watcher', kind: 'file' }),
      event(4, 'theme.ts', { source: 'watcher', kind: 'file' }),
    ]);
    expect(steps.map(step => step.id)).toEqual(['1', '3', '4']);
    expect(steps.map(step => step.summary)).toEqual(['theme.ts · created', 'main.ts · modified', 'theme.ts · modified']);
    expect(steps.every(step => step.why === '' && step.explanation_pending)).toBe(true);
  });
  it('attaches delayed explanations to their exact revision without a duplicate step', () => {
    const edits = [
      event(1, 'theme.ts', { source: 'watcher', kind: 'file', action: 'created' }),
      event(2, 'main.ts', { source: 'watcher', kind: 'file' }),
      event(3, 'theme.ts', { source: 'watcher', kind: 'file' }),
    ];
    const steps = workflowSteps([...edits,
      event(4, 'theme.ts', { change_id: '1', action: 'created', summary: 'Save appearance', why: 'Keep the selected palette across reloads.' }),
      event(5, 'main.ts', { change_id: '2', summary: 'Restore before render', why: 'Apply the saved palette before mounting.' }),
    ]);
    expect(steps).toHaveLength(3);
    expect(steps.map(step => step.id)).toEqual(['1', '2', '3']);
    expect(steps[0].why).toBe('Keep the selected palette across reloads.');
    expect(steps[1].why).toBe('Apply the saved palette before mounting.');
    expect(steps[2].why).toBe('');
    expect(steps[2].explanation_pending).toBe(true);
  });
  it('does not reuse a reason from another file, revision, or chat', () => {
    const steps = workflowSteps([
      event(1, 'theme.ts', { session_id: 'a', source: 'watcher', kind: 'file' }),
      event(2, 'main.ts', { session_id: 'a', change_id: '1' }),
      event(3, 'theme.ts', { session_id: 'b', change_id: '1' }),
      event(4, 'theme.ts', { session_id: 'a', change_id: 'missing' }),
      event(5, 'theme.ts', { session_id: 'a', change_id: '1', source: 'hook', category: 'activity', phase: 'editing' }),
    ]);
    expect(steps).toHaveLength(1);
    expect(steps[0].why).toBe('');
  });
  it('updates a test step in place without duplicating the passive test hook', () => {
    const steps = workflowSteps([
      event(1, 'main_test.go', { action: 'tested', summary: 'Test greetings', status: 'running' }),
      event(2, '@workflow', { source: 'hook', category: 'activity', phase: 'testing', action: 'scouted' }),
      event(3, 'main_test.go', { action: 'tested', summary: 'Tests pass', status: 'ready', why: 'Default and Unicode names pass.' }),
    ]);
    expect(steps).toHaveLength(1);
    expect(steps[0].id).toBe('1');
    expect(steps[0].summary).toBe('Tests pass');
    expect(steps[0].status).toBe('ready');
  });
  it('keeps watcher noise out of the AI workflow', () => {
    expect(workflowSteps([event(1, 'main.go', { source: 'watcher' }), event(2, 'main.go#Run', { source: 'watcher' })])).toEqual([]);
  });
  it('attaches an agent explanation to the adjacent observed edit', () => {
    const steps = workflowSteps([
      event(1, 'main.go', { source: 'hook', category: 'activity', phase: 'reading', action: 'scouted' }),
      event(2, 'main.go', { source: 'hook', category: 'activity', phase: 'editing', action: 'scouted' }),
      event(3, 'main.go#Run', { agent_id: 'reporter', why: 'Blank names need a default.' }),
    ]);
    expect(steps).toHaveLength(2);
    expect(steps[0].why).toBe('');
    expect(steps[1].why).toBe('Blank names need a default.');
    expect(steps[1].id).toBe('2');
    expect(workflowEdges(steps)).toEqual([{ from: '1', to: '2', explicit: false }]);
  });
  it('distinguishes reported file relationships from chronological arrows', () => {
    const steps = workflowSteps([event(1, 'entry.go'), event(2, 'other.go'), event(3, 'handler.go', { parents: ['entry.go'] })]);
    expect(workflowEdges(steps)).toEqual([
      { from: '1', to: '2', explicit: false },
      { from: '2', to: '3', explicit: false },
      { from: '1', to: '3', explicit: true },
    ]);
  });
});

describe('session timeline', () => {
  it('deduplicates reconnect deliveries and preserves chronological event order', () => {
    const current = [event(1), event(2)];
    expect(mergeEvents(current, [event(2), event(4), event(3)]).map((e) => e.id)).toEqual(['1', '2', '3', '4']);
  });
  it('replays the timestamp distribution and never mutates the captured session', () => {
    const events = [event(1), event(2), event(3, 'other.go', { timestamp: new Date(110000).toISOString() })];
    expect(eventsAt(events, 0)).toEqual([]);
    expect(eventsAt(events, 0.5).map((e) => e.id)).toEqual(['1', '2']);
    expect(eventsAt(events, 1)).toEqual(events);
    expect(events).toHaveLength(3);
  });
  it('handles events with identical timestamps without losing their order', () => {
    const events = [event(1), event(2), event(3)].map((e) => ({ ...e, timestamp: new Date(100000).toISOString() }));
    expect(eventsAt(events, 0.5)).toHaveLength(2);
  });
  it('keeps deleted nodes in history and updates their latest state', () => {
    expect(latestNodes([event(1), event(2, 'main.go', { action: 'deleted' })])).toEqual([event(2, 'main.go', { action: 'deleted' })]);
  });
  it('does not infer architecture dependencies from filesystem observations', () => {
    expect(activityEdges([event(1, 'a.go', { source: 'watcher' }), event(2, 'b.go', { source: 'watcher' })])).toEqual([]);
    expect(activityEdges([event(1, 'a.go'), event(2, 'b.go')])).toEqual([{ from: 'a.go', to: 'b.go', explicit: false }]);
  });
});

describe('collision lifecycle', () => {
  const writes = [event(1, 'main.go#Run'), event(2, 'main.go#Stop', { agent_id: 'beta', collisions: ['alpha'] })];
  it('shows same-file writes from different symbols and expires the warning', () => {
    expect(activeCollisions(writes, 105000)).toHaveLength(1);
    expect(activeCollisions(writes, 200000)).toHaveLength(0);
  });
  it('clears the warning when the other agent releases its claim', () => {
    expect(activeCollisions([...writes, event(3, 'main.go#Run', { status: 'ready', action: 'tested' })], 105000)).toHaveLength(0);
  });
});

it('keeps lifecycle observations out of the architecture graph and tracks each agent independently', () => {
  const events = [event(1, 'main.go'), event(2, 'main.go', { source: 'hook', category: 'activity', phase: 'reading' }), event(3, 'main.go', { source: 'hook', category: 'activity', phase: 'editing' }), event(4, '@workflow', { source: 'hook', category: 'activity', phase: 'testing', agent_id: 'beta' })];
  expect(latestNodes(events)).toHaveLength(1);
  expect(latestNodes(events)[0].id).toBe('1');
  expect(activityEdges(events)).toHaveLength(0);
  expect(workflowAgents(events).map((e) => e.phase)).toEqual(['testing', 'editing']);
});

it('keeps developer notes from replacing code nodes, rationale, or workflow stages', () => {
  const events = [event(1, 'main.go'), event(2, 'main.go', { source: 'human', category: 'feedback', note_kind: 'question', target_agent: 'alpha', why: 'Does this cover retries?' }), event(3, '@workflow', { source: 'human', category: 'feedback' })];
  expect(latestNodes(events)).toEqual([events[0]]);
  expect(activityEdges(events)).toEqual([]);
  expect(workflowAgents(events)).toEqual([]);
});
