import { isFileChange, type AgentEvent } from './lib';

export interface DiffLine {
  kind: 'addition' | 'removal' | 'context' | 'hunk' | 'meta';
  text: string;
  prefix: string;
  oldLine?: number;
  newLine?: number;
}

// Read the captured unified patch, rather than comparing against a newer file.
export function diffLines(diff: string): DiffLine[] {
  const lines = diff.split('\n');
  if (lines.at(-1) === '') lines.pop();
  let oldLine = 0, newLine = 0, inHunk = false;
  return lines.map(line => {
    const hunk = /^@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@/.exec(line);
    if (hunk) {
      oldLine = Number(hunk[1]); newLine = Number(hunk[2]); inHunk = true;
      return { kind: 'hunk', prefix: '', text: line };
    }
    if (inHunk && line.startsWith('+')) return { kind: 'addition', prefix: '+', text: line.slice(1), newLine: newLine++ };
    if (inHunk && line.startsWith('-')) return { kind: 'removal', prefix: '-', text: line.slice(1), oldLine: oldLine++ };
    if (inHunk && line.startsWith(' ')) return { kind: 'context', prefix: ' ', text: line.slice(1), oldLine: oldLine++, newLine: newLine++ };
    return { kind: 'meta', prefix: '', text: line };
  });
}

// Keep the parsed rows in their saved order, including each hunk's range and EOF markers.
export function diffHunks(diff: string): DiffLine[][] {
  const hunks: DiffLine[][] = [];
  let current: DiffLine[] | undefined;
  const truncated = isTruncatedDiff(diff);
  const rows = diffLines(diff);
  for (const [index, row] of rows.entries()) {
    if (row.kind === 'hunk') {
      current = [];
      hunks.push(current);
    }
    if (current && !(truncated && index === rows.length - 1)) current.push(row);
  }
  return hunks;
}

export function hasFileDiff(step: AgentEvent): boolean {
  return isFileChange(step);
}

export function isTruncatedDiff(diff: string): boolean {
  return diff.endsWith('\n… [truncated]');
}
