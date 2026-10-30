import { describe, expect, it } from 'vitest';
import { diffLines, isTruncatedDiff } from './diff';
import { workflowSteps, type AgentEvent } from './lib';

describe('captured unified diff', () => {
  it('numbers both sides of a replacement and keeps headers separate from code', () => {
    const rows = diffLines('--- a/main.ts\n+++ b/main.ts\n@@ -8,3 +8,3 @@\n context\n-old\n+new\n end\n');
    expect(rows.slice(0, 3).map(row => row.kind)).toEqual(['meta', 'meta', 'hunk']);
    expect(rows.slice(3)).toEqual([
      { kind: 'context', prefix: ' ', text: 'context', oldLine: 8, newLine: 8 },
      { kind: 'removal', prefix: '-', text: 'old', oldLine: 9 },
      { kind: 'addition', prefix: '+', text: 'new', newLine: 9 },
      { kind: 'context', prefix: ' ', text: 'end', oldLine: 10, newLine: 10 },
    ]);
  });
  it('uses hunk ranges across distant changes, including omitted and zero counts', () => {
    const rows = diffLines('@@ -0,0 +1 @@\n+first\n@@ -42 +43,0 @@\n-last\n');
    expect(rows[1]).toMatchObject({ newLine: 1, text: 'first' });
    expect(rows[3]).toMatchObject({ oldLine: 42, text: 'last' });
    expect(rows[1].oldLine).toBeUndefined(); expect(rows[3].newLine).toBeUndefined();
  });
  it('treats header-like file content as real added and removed lines', () => {
    const rows = diffLines('--- a/a.txt\n+++ b/a.txt\n@@ -1 +1 @@\n----removed\n++++added\n');
    expect(rows[3]).toMatchObject({ kind: 'removal', text: '---removed', oldLine: 1 });
    expect(rows[4]).toMatchObject({ kind: 'addition', text: '+++added', newLine: 1 });
  });
  it('preserves whitespace and Unicode without numbering newline markers', () => {
    const rows = diffLines('@@ -1 +1,2 @@\n-Иване\n\\ No newline at end of file\n+\tИване  \n+\n');
    expect(rows[2]).toMatchObject({ kind: 'meta', text: '\\ No newline at end of file' });
    expect(rows[3]).toMatchObject({ text: '\tИване  ', newLine: 1 });
    expect(rows[4]).toMatchObject({ text: '', newLine: 2 });
    expect(diffLines('')).toEqual([]);
  });
  it('detects incomplete captured previews', () => {
    expect(isTruncatedDiff('@@ -1 +1 @@\n-old\n+new\n… [truncated]')).toBe(true);
    expect(isTruncatedDiff('@@ -1 +1 @@\n+… [truncated]\n')).toBe(false);
  });
  it('keeps the observed diff when its exact explanation arrives', () => {
    const base: AgentEvent = { id: '1', timestamp: '', agent_id: 'filesystem', node_name: 'main.ts', action: 'modified', summary: 'File modified', why: 'Observed', status: 'ready', source: 'watcher', kind: 'file', diff: '@@ -1 +1 @@\n-old\n+new\n' };
    const steps = workflowSteps([base, { ...base, id: '2', source: 'agent', change_id: '1', why: 'Keep the default consistent.', diff: 'Different model-supplied patch' }, { ...base, id: '3', diff: '@@ -1 +1 @@\n-new\n+latest\n' }]);
    expect(steps[0].diff).toBe(base.diff);
    expect(steps[0].why).toBe('Keep the default consistent.');
    expect(steps[1].diff).toContain('+latest');
    expect(steps[1].why).toBe('');
  });
});
