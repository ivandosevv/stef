import { describe, expect, it } from 'vitest';
import { eventID, isAgentEvent, isStreamMessage, trustedHostMessage } from './stream-security';

const event = () => ({ id: '1', timestamp: '2026-10-31T10:00:00Z', agent_id: 'worker', session_id: 'private', node_name: 'main.ts', action: 'modified', status: 'ready', source: 'agent', summary: 'Check inputs', why: 'Validate incoming data.' });
describe('live data boundary', () => {
  it('accepts actual snapshot, event and workspace wire shapes', () => {
    const room = { workspace_id: 'private', name: 'Quiet Fox 2487', shared: false, members: ['private'] };
    expect(isStreamMessage({ type: 'snapshot', repo: '/project', session_id: 'private', events: [event()], update_mode: 'files', updates_from: '0', workspaces: [room] })).toBe(true);
    expect(isStreamMessage({ type: 'event', event: event() })).toBe(true);
    expect(isStreamMessage({ type: 'workspaces', workspaces: [room] })).toBe(true);
    expect(isStreamMessage({ type: 'snapshot', repo: '/project' })).toBe(true);
  });
  it('validates activity provenance and decisions without accepting arbitrary actions or modes', () => {
    expect(isStreamMessage({ type: 'snapshot', repo: '/project', update_mode: 'activity', events: [{ ...event(), action: 'decided', recording_mode: 'activity' }] })).toBe(true);
    for (const recording_mode of ['thinking', null, {}, '']) expect(isAgentEvent({ ...event(), recording_mode })).toBe(false);
    for (const recording_from of ['NaN', '-1', '9'.repeat(10000), null]) expect(isAgentEvent({ ...event(), recording_from })).toBe(false);
    expect(isAgentEvent({ ...event(), recording_mode: 'activity', recording_from: '0' })).toBe(true);
    expect(isAgentEvent({ ...event(), action: 'thinking' })).toBe(false);
    expect(isStreamMessage({ type: 'snapshot', repo: '/project', update_mode: 'thinking' })).toBe(false);
  });
  it('rejects IDs that would crash or exhaust numeric comparisons', () => {
    for (const id of ['0', '-1', '+1', '01', 'abc', '18446744073709551616', '9'.repeat(10000), null]) {
      expect(isAgentEvent({ ...event(), id })).toBe(false);
    }
    expect(eventID('18446744073709551615')).toBe(true);
    expect(eventID('0', true)).toBe(true);
  });
  it('rejects malformed source fields, links, arrays and oversized snapshots', () => {
    for (const fields of [{ node_name: {} }, { source: 'external' }, { action: 'run' }, { timestamp: 'invalid' }, { status: [] }, { diff: 'x'.repeat(16385) }, { parents: {} }, { parents: [null] }, { change_id: 'NaN' }, { line: -1 }]) {
      expect(isStreamMessage({ type: 'event', event: { ...event(), ...fields } })).toBe(false);
    }
    expect(isStreamMessage({ type: 'snapshot', repo: '/project', events: Array(5001).fill(event()) })).toBe(false);
    expect(isStreamMessage({ type: 'snapshot', repo: '/project', updates_from: 'bad' })).toBe(false);
    expect(isStreamMessage({ type: 'workspaces', workspaces: [{ workspace_id: 'other', shared: true, members: {} }] })).toBe(false);
    expect(isStreamMessage(null)).toBe(false);
  });
  it('keeps source text inert rather than stripping exact saved evidence', () => {
    const diff = '@@ -1 +1 @@\n-plain\n+<img src="https://external.example" onerror="alert(1)">';
    expect(isStreamMessage({ type: 'event', event: { ...event(), diff } })).toBe(true);
  });
  it('rejects messages from foreign and sandboxed frames while preserving host dispatch', () => {
    const current = {} as Window;
    const foreign = {} as Window;
    const origin = 'http://127.0.0.1:4820';
    expect(trustedHostMessage({ source: current, origin }, current, origin)).toBe(true);
    expect(trustedHostMessage({ source: null, origin: '' }, current, origin)).toBe(true);
    expect(trustedHostMessage({ source: foreign, origin }, current, origin)).toBe(false);
    expect(trustedHostMessage({ source: foreign, origin: 'null' }, current, origin)).toBe(false);
    expect(trustedHostMessage({ source: current, origin: 'https://external.example' }, current, origin)).toBe(false);
    expect(trustedHostMessage({ source: null, origin: 'null' }, current, origin)).toBe(false);
  });
});
