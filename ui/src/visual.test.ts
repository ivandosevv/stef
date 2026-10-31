import { describe, expect, it } from 'vitest';
import { milestoneAction, stepAction, reportedParents, visualHunks } from './visual';
import type { AgentEvent } from './lib';

const step = (id: string, node_name: string, parents?: string[]): AgentEvent => ({
  id, node_name, parents, timestamp: '', agent_id: 'agent', action: 'modified',
  summary: 'Saved a change', why: 'Supplied reason', status: 'ready', source: 'agent',
});

describe('local change visualization', () => {
  it('separates saved before and after rows while keeping exact line numbers', () => {
    const hunks = visualHunks('--- a/main.ts\n+++ b/main.ts\n@@ -8,3 +8,3 @@\n context\n-old\n+new\n end\n@@ -42 +42 @@\n-before\n+after\n');
    expect(hunks).toHaveLength(2);
    expect(hunks[0].header).toBe('@@ -8,3 +8,3 @@');
    expect(hunks[0].before).toEqual([
      { kind: 'context', text: 'context', line: 8 },
      { kind: 'removal', text: 'old', line: 9 },
      { kind: 'context', text: 'end', line: 10 },
    ]);
    expect(hunks[0].after).toEqual([
      { kind: 'context', text: 'context', line: 8 },
      { kind: 'addition', text: 'new', line: 9 },
      { kind: 'context', text: 'end', line: 10 },
    ]);
    expect(hunks[1].before[0].line).toBe(42);
    expect(hunks[1].after[0].line).toBe(42);
  });

  it('represents addition-only and removal-only hunks without invented source', () => {
    const hunks = visualHunks('@@ -0,0 +1 @@\n+first\n@@ -42 +43,0 @@\n-last\n');
    expect(hunks[0].before).toEqual([]);
    expect(hunks[0].after).toEqual([{ kind: 'addition', text: 'first', line: 1 }]);
    expect(hunks[1].before).toEqual([{ kind: 'removal', text: 'last', line: 42 }]);
    expect(hunks[1].after).toEqual([]);
  });

  it('preserves source whitespace, Unicode, and the side of newline markers', () => {
    const hunks = visualHunks('@@ -1 +1,2 @@\n-Иване\n\\ No newline at end of file\n+\tИване  \n+\n');
    expect(hunks[0].before).toEqual([
      { kind: 'removal', text: 'Иване', line: 1 },
      { kind: 'meta', text: '\\ No newline at end of file' },
    ]);
    expect(hunks[0].after).toEqual([
      { kind: 'addition', text: '\tИване  ', line: 1 },
      { kind: 'addition', text: '', line: 2 },
    ]);
    const shared = visualHunks('@@ -1 +1 @@\n unchanged\n\\ No newline at end of file\n')[0];
    expect(shared.before[1]).toEqual(shared.after[1]);
  });

  it('keeps truncation notices out of code and handles captures without hunks', () => {
    expect(visualHunks('')).toEqual([]);
    expect(visualHunks('--- a/empty\n+++ b/empty\n')).toEqual([]);
    const hunk = visualHunks('@@ -1 +1 @@\n-old\n+new\n… [truncated]')[0];
    expect(hunk.before.map(row => row.text)).toEqual(['old']);
    expect(hunk.after.map(row => row.text)).toEqual(['new']);
  });

  it('labels only the reported action rather than inferring code behavior', () => {
    expect(milestoneAction('scouted')).toBe('Explore');
    expect(milestoneAction('tested')).toBe('Verify');
    expect(milestoneAction('decided')).toBe('Decision');
    expect(stepAction({ ...step('1', '@workflow'), source: 'hook', phase: 'terminal' })).toBe('Terminal');
    expect(stepAction({ ...step('1', '@workflow'), source: 'hook', phase: 'searching' })).toBe('Searching');
    expect(stepAction({ ...step('1', '@workflow'), source: 'hook', phase: 'tool_done' })).toBe('Finished');
    for (const action of ['created', 'modified', 'deleted'] as const) expect(milestoneAction(action)).toBe('Change');
  });

  it('resolves only reported parents found before the selected visible step', () => {
    const selected = step('4', 'main.ts', ['helper.ts', 'missing.ts', 'future.ts', 'helper.ts']);
    const steps = [step('1', 'helper.ts'), step('2', 'helper.ts#format'), step('3', 'unrelated.ts'), selected, step('5', 'future.ts')];
    expect(reportedParents(selected, steps)).toEqual([
      { path: 'helper.ts', stepId: '1' }, { path: 'missing.ts' }, { path: 'future.ts' },
    ]);
    expect(reportedParents(step('9', 'outside.ts', ['helper.ts']), steps)).toEqual([{ path: 'helper.ts' }]);
    expect(reportedParents(step('4', 'main.ts', ['helper.ts#format']), steps)).toEqual([{ path: 'helper.ts#format', stepId: '2' }]);
  });
});
