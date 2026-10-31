import { HISTORY_LIMIT, isUpdateMode, type AgentEvent, type StreamMessage, type Workspace } from './lib';

// Validate host/daemon data before rendering or numeric ID comparisons.
const object = (value: unknown): value is Record<string, unknown> => value !== null && typeof value === 'object' && !Array.isArray(value);
const text = (value: unknown, limit: number): value is string => typeof value === 'string' && value.length <= limit;
export function eventID(value: unknown, zero = false): value is string {
  return typeof value === 'string' && /^(?:0|[1-9][0-9]{0,19})$/.test(value) && (zero || value !== '0') && BigInt(value) <= 18446744073709551615n;
}
const optionalText = (value: unknown, limit: number) => value === undefined || text(value, limit);
const strings = (value: unknown, count: number, length: number): value is string[] => Array.isArray(value) && value.length <= count && value.every(item => text(item, length));
export function isWorkspace(value: unknown): value is Workspace {
  return object(value) && text(value.workspace_id, 160) && Boolean(value.workspace_id) && optionalText(value.name, 160) && typeof value.shared === 'boolean' && strings(value.members, 1024, 160);
}
export const isWorkspaceList = (value: unknown): value is Workspace[] => Array.isArray(value) && value.length <= 1024 && value.every(isWorkspace);
export function isAgentEvent(value: unknown): value is AgentEvent {
  if (!object(value) || !eventID(value.id) || !text(value.timestamp, 100) || !Number.isFinite(Date.parse(value.timestamp))) return false;
  if (!text(value.agent_id, 80) || !value.agent_id || !text(value.node_name, 500) || !value.node_name || !text(value.summary, 160) || !text(value.why, 2000)) return false;
  if (!['scouted', 'created', 'modified', 'deleted', 'tested'].includes(value.action as string) || !['running', 'ready'].includes(value.status as string) || !['agent', 'watcher', 'demo', 'hook', 'human'].includes(value.source as string)) return false;
  if (value.category !== undefined && !['activity', 'feedback'].includes(value.category as string)) return false;
  if (!optionalText(value.session_id, 160) || !optionalText(value.phase, 100) || !optionalText(value.tool, 100) || !optionalText(value.kind, 80) || !optionalText(value.language, 40) || !optionalText(value.target_agent, 80) || !optionalText(value.note_kind, 20)) return false;
  if (!['before', 'after', 'diff'].every(key => optionalText(value[key], 16384))) return false;
  if (value.line !== undefined && (!Number.isSafeInteger(value.line) || (value.line as number) < 0)) return false;
  if (!['change_id', 'context_id'].every(key => value[key] === undefined || eventID(value[key]))) return false;
  if (value.parents !== undefined && !strings(value.parents, 20, 500)) return false;
  return value.collisions === undefined || strings(value.collisions, 1024, 80);
}
export function isStreamMessage(value: unknown): value is StreamMessage {
  if (!object(value) || !optionalText(value.session_id, 160)) return false;
  if (value.type === 'workspaces') return isWorkspaceList(value.workspaces) && optionalText(value.current_workspace_id, 160);
  if (value.type === 'event') return isAgentEvent(value.event);
  if (value.type !== 'snapshot' || !text(value.repo, 32768)) return false;
  return (value.events === undefined || (Array.isArray(value.events) && value.events.length <= HISTORY_LIMIT && value.events.every(isAgentEvent))) &&
    (value.workspaces === undefined || isWorkspaceList(value.workspaces)) &&
    (value.update_mode === undefined || isUpdateMode(value.update_mode)) &&
    (value.updates_from === undefined || eventID(value.updates_from, true));
}

// VS Code's host dispatch has no source/origin. Real window messages must come
// from this same window and origin; sandboxed or foreign frames are rejected.
export function trustedHostMessage(event: Pick<MessageEvent, 'source' | 'origin'>, current: Window, origin: string): boolean {
  return (event.source === null && event.origin === '') || (event.source === current && event.origin === origin);
}
