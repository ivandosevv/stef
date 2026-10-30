import { diffLines, type DiffLine } from './diff';
import type { AgentEvent } from './lib';

export interface VisualLine {
  kind: 'context' | 'addition' | 'removal' | 'meta';
  text: string;
  line?: number;
}

export interface VisualHunk {
  header: string;
  before: VisualLine[];
  after: VisualLine[];
}

// Both panes use only the saved patch. They do not read the current file.
export function visualHunks(diff: string): VisualHunk[] {
  const hunks: VisualHunk[] = [];
  let current: VisualHunk | undefined;
  let previousKind: DiffLine['kind'] | undefined;
  for (const row of diffLines(diff)) {
    if (row.kind === 'hunk') {
      current = { header: row.text, before: [], after: [] };
      hunks.push(current);
    } else if (current) {
      if (row.kind === 'context' || row.kind === 'removal') {
        current.before.push({ kind: row.kind, text: row.text, line: row.oldLine });
      }
      if (row.kind === 'context' || row.kind === 'addition') {
        current.after.push({ kind: row.kind, text: row.text, line: row.newLine });
      }
      // A newline marker belongs to the immediately preceding side(s).
      if (row.kind === 'meta' && row.text === '\\ No newline at end of file') {
        const marker: VisualLine = { kind: 'meta', text: row.text };
        if (previousKind === 'context' || previousKind === 'removal') current.before.push(marker);
        if (previousKind === 'context' || previousKind === 'addition') current.after.push(marker);
      }
    }
    previousKind = row.kind;
  }
  return hunks;
}

export function milestoneAction(action: AgentEvent['action']): 'Explore' | 'Change' | 'Verify' {
  return action === 'scouted' ? 'Explore' : action === 'tested' ? 'Verify' : 'Change';
}

export interface ReportedParent {
  path: string;
  stepId?: string;
}

// Resolve an agent-reported path only against an earlier visible workflow step.
export function reportedParents(step: AgentEvent, steps: AgentEvent[]): ReportedParent[] {
  const selectedIndex = steps.findIndex(candidate => candidate.id === step.id);
  const earlier = selectedIndex < 0 ? [] : steps.slice(0, selectedIndex).reverse();
  return [...new Set(step.parents ?? [])].map(path => {
    const parent = earlier.find(candidate => candidate.node_name === path)
      ?? (!path.includes('#') ? earlier.find(candidate => candidate.node_name.split('#')[0] === path) : undefined);
    return { path, ...(parent ? { stepId: parent.id } : {}) };
  });
}
