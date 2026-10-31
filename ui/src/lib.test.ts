import { describe, it, expect } from 'vitest';
import { activeCollisions, activityEdges, eventsAt, latestNodes, mergeEvents, workflowAgents, workflowSteps, workflowWindows, workflowEdges, isUpdateMode, phaseLabel, stepReason, type AgentEvent } from './lib';

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
  it('attributes an exact shared explanation to its reporter without inventing a watcher author', () => {
    const captured = event(1, 'shared.ts', { session_id: 'room', source: 'watcher', kind: 'file', agent_id: 'filesystem' });
    expect(workflowSteps([captured])[0].agent_id).toBe('filesystem');
    const explained = workflowSteps([captured, event(2, 'shared.ts', { session_id: 'room', change_id: '1', agent_id: 'claude', why: 'Keep the shared boundary consistent.' })]);
    expect(explained[0]).toMatchObject({ agent_id: 'claude', why: 'Keep the shared boundary consistent.', id: '1' });
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
  it('separates saved history from newly enabled file reporting', () => {
    const events = [event(1, 'old.ts', { source: 'watcher', kind: 'file' }), event(2, '@workflow'), event(3, 'new.ts', { source: 'watcher', kind: 'file' })];
    expect(workflowWindows(events, 'files', '3').current.map(step => step.id)).toEqual(['3']);
    expect(workflowWindows(events, 'files', '3').saved.map(step => step.id)).toEqual(['1', '2']);
    expect(workflowWindows(events, 'files').current).toHaveLength(3);
    expect(workflowWindows(events, 'disabled', '3').current).toEqual([]);
  });
  it('does not coalesce a new milestone into a step from an earlier window', () => {
    const windows = workflowWindows([event(1), event(2, 'main.go', { why: 'Now verify the new behavior.' })], 'milestones', '2');
    expect(windows.current[0]).toMatchObject({ id: '2', why: 'Now verify the new behavior.' });
    expect(windows.saved[0].id).toBe('1');
  });
  it('updates requested context on the exact historical revision without creating a current step', () => {
    const diff = '@@ -1 +1 @@\n-old\n+new\n';
    const explanation = 'Explain the existing choice with enough context for the developer to follow the actual behavior while leaving the original saved code revision intact across future edits and reloads.';
    const windows = workflowWindows([
      event(1, 'main.ts', { source: 'watcher', kind: 'file', diff }),
      event(2, 'main.ts', { change_id: '1', why: 'Use the shared default.' }),
      event(4, 'main.ts', { context_id: '1', why: explanation }),
    ], 'milestones', '3');
    expect(windows.current).toEqual([]);
    expect(windows.saved).toHaveLength(1);
    expect(windows.saved[0]).toMatchObject({ id: '1', explanation, diff, reference_id: '1' });
    expect(windows.saved[0].why.endsWith('…')).toBe(true);
  });
  it('uses the latest real milestone as the reference while preserving selection across updates', () => {
    const steps = workflowSteps([event(1), event(2), event(3, 'main.go', { context_id: '2', why: 'A requested explanation of the finished step.' })]);
    expect(steps).toHaveLength(1);
    expect(steps[0]).toMatchObject({ id: '1', reference_id: '2', explanation: 'A requested explanation of the finished step.' });
  });
  it('rejects context from another chat, action, path, or non-workflow target', () => {
    const steps = workflowSteps([
      event(1, 'a.ts', { source: 'watcher', kind: 'file', session_id: 'a' }),
      event(2, 'b.ts', { context_id: '1', session_id: 'a' }),
      event(3, 'a.ts', { context_id: '1', session_id: 'b' }),
      event(4, 'a.ts', { context_id: '1', session_id: 'a', action: 'tested' }),
      event(5, '@workflow', { source: 'human', category: 'feedback' }),
      event(6, '@workflow', { context_id: '5' }),
    ]);
    expect(steps).toHaveLength(1); expect(steps[0].why).toBe('');
  });
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


describe('All activity recording', () => {
  const observed = (id: number, phase: string, options: Partial<AgentEvent> = {}) => event(id, '@workflow', {
    source: 'hook', category: 'activity', action: 'scouted', phase, tool: 'exec_command',
    why: 'Generic observer text is not an agent explanation.', recording_mode: 'activity', ...options,
  });

  it('preserves each supported operation and decision without coalescing separate calls', () => {
    const phases = ['started', 'reading', 'searching', 'editing', 'testing', 'terminal', 'tool_done', 'completed', 'failed'];
    const events = phases.map((phase, index) => observed(index + 1, phase));
    events.push(observed(10, 'terminal'), observed(11, 'terminal'));
    events.push(event(12, '@workflow', { action: 'decided', recording_mode: 'activity', summary: 'Keep validation at entry', why: 'Reject unsafe input before any file is read.' }));
    const steps = workflowSteps(events, 'activity');
    expect(steps.map(step => step.id)).toEqual(events.map(step => step.id));
    expect(steps[5].summary).toBe('Running terminal command · exec_command');
    expect(steps[2].summary).toBe('Searching project context · exec_command');
    expect(steps[11]).toMatchObject({ action: 'decided', why: 'Reject unsafe input before any file is read.' });
    expect(steps.slice(0, 11).every(step => step.why === '' && !step.explanation_pending)).toBe(true);
    expect(workflowEdges(steps)).toHaveLength(11);
  });

  it('keeps the existing file and milestone views free of extended lifecycle and decision rows', () => {
    const events = [observed(1, 'started'), observed(2, 'searching'), observed(3, 'terminal'), observed(4, 'completed'),
      event(5, '@workflow', { action: 'decided', recording_mode: 'activity' }),
      event(6, 'main.ts', { summary: 'Save the boundary', recording_mode: 'files' })];
    expect(workflowSteps(events, 'files').map(step => step.id)).toEqual(['6']);
    expect(workflowSteps(events, 'milestones').map(step => step.id)).toEqual(['6']);
    expect(workflowSteps(events, 'disabled')).toEqual([]);
    expect(workflowSteps([observed(1, 'private_analysis'), observed(2, 'terminal', { recording_mode: undefined })], 'activity')).toEqual([]);
  });

  it('preserves concrete file searches and exact context in Every file while hiding generic searches', () => {
    const target = observed(1, 'searching', { session_id: 'a', node_name: 'main.ts', tool: 'Grep', recording_mode: 'files', summary: 'Search the entry point' });
    const generic = observed(2, 'searching', { recording_mode: 'files' });
    const context = event(3, 'main.ts', { session_id: 'a', action: 'scouted', context_id: '1', why: 'Find the existing validation before changing the entry point.' });
    const files = workflowSteps([target, generic, context], 'files');
    expect(files).toHaveLength(1);
    expect(files[0]).toMatchObject({ id: '1', action: 'scouted', summary: 'Search the entry point', why: context.why, explanation: context.why });
    expect(workflowSteps([{ ...target, recording_mode: undefined }], 'files')[0].action).toBe('scouted');
    const full = workflowSteps([{ ...target, recording_mode: 'activity' }], 'activity');
    expect(full[0].summary).toBe('Searching project context · Grep · main.ts');
    expect(full[0].action).toBe('scouted');
    expect(workflowWindows([target, generic, context], 'activity', '4').saved).toHaveLength(1);
  });

  it('uses exact requested context for recorded activity without treating observed text as a reason', () => {
    const target = observed(1, 'terminal', { session_id: 'a' });
    const steps = workflowSteps([target,
      event(2, '@workflow', { context_id: '1', session_id: 'a', action: 'scouted', why: 'Run the focused test before changing the shared boundary.' }),
      event(3, '@workflow', { context_id: '1', session_id: 'b', action: 'scouted', why: 'Another chat cannot overwrite the reason.' }),
      event(4, '@workflow', { context_id: '1', session_id: 'a', action: 'modified', why: 'A mismatched action cannot overwrite the reason.' }),
    ], 'activity');
    expect(steps).toHaveLength(1);
    expect(steps[0].why).toBe('Run the focused test before changing the shared boundary.');
    expect(stepReason(workflowSteps([target], 'activity')[0])).toBe('Observed activity. The agent has not shared an explanation.');
    expect(stepReason(steps[0])).toBe(steps[0].explanation);
    expect(phaseLabel('searching')).toBe('Searching project context');
  });

  it('starts future activity at the boundary and restores only originally recorded activity in history', () => {
    const events = [observed(1, 'terminal', { recording_mode: undefined }),
      observed(2, 'terminal', { recording_mode: 'files' }),
      observed(3, 'terminal'), observed(4, 'terminal'),
      event(5, '@workflow', { action: 'decided', recording_mode: 'activity', why: 'Use a single trusted input boundary.' }),
      event(6, 'main.ts', { recording_mode: 'milestones' }), observed(7, 'searching')];
    const windows = workflowWindows(events, 'activity', '7');
    expect(windows.current.map(step => step.id)).toEqual(['7']);
    expect(windows.saved.map(step => step.id)).toEqual(['3', '4', '5', '6']);
    const files = workflowWindows(events, 'files', '8');
    expect(files.current).toEqual([]);
    expect(files.saved.map(step => step.id)).toEqual(['3', '4', '5', '6', '7']);
  });

  it('restores the exact saved diff and late context across later recording modes', () => {
    const diff = `@@ -1 +1 @@
-old
+new
`;
    const events = [event(1, 'main.ts', { source: 'watcher', kind: 'file', recording_mode: 'activity', diff }),
      event(2, 'main.ts', { change_id: '1', recording_mode: 'activity', why: 'Use the validated default.' }),
      observed(3, 'terminal'),
      event(5, 'main.ts', { context_id: '1', recording_mode: 'milestones', why: 'The entry point now validates the default before opening a source file.' }),
      event(6, '@workflow', { context_id: '3', action: 'scouted', recording_mode: 'milestones', why: 'Run the boundary tests using the existing local test runner.' })];
    const windows = workflowWindows(events, 'milestones', '4');
    expect(windows.current).toEqual([]);
    expect(windows.saved).toHaveLength(2);
    expect(windows.saved[0]).toMatchObject({ id: '1', diff, explanation: events[3].why });
    expect(windows.saved[1]).toMatchObject({ id: '3', explanation: events[4].why });
  });

  it('does not merge a repeated file step across separate saved recording windows', () => {
    const windows = workflowWindows([
      event(1, 'main.ts', { recording_mode: 'files', recording_from: '1' }),
      event(2, 'main.ts', { recording_mode: 'files', recording_from: '2', why: 'Now verify the next saved window.' }),
    ], 'activity', '3');
    expect(windows.saved.map(step => step.id)).toEqual(['1', '2']);
    expect(windows.saved[1].why).toBe('Now verify the next saved window.');
  });

  it('recognizes exactly the four requested recording options', () => {
    for (const mode of ['milestones', 'files', 'activity', 'disabled']) expect(isUpdateMode(mode)).toBe(true);
    for (const mode of ['thinking', 'all', null, {}]) expect(isUpdateMode(mode)).toBe(false);
  });
});
