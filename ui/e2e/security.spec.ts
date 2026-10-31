import { expect, test } from '@playwright/test';

test('embedded workflow rejects foreign and malformed messages without issuing AI requests', async ({ page }) => {
  const errors: string[] = [];
  const mutations: string[] = [];
  page.on('pageerror', error => errors.push(error.message));
  page.on('request', request => { if (request.method() !== 'GET') mutations.push(request.url()); });
  await page.addInitScript(() => {
    const host = window as Window & { stefTestReady?: boolean; acquireVsCodeApi?: () => { postMessage(message: unknown): void } };
    host.acquireVsCodeApi = () => ({ postMessage(message) { if ((message as { type?: string }).type === 'stef.ready') host.stefTestReady = true; } });
  });
  await page.goto('/?session_id=browser-security-chat');
  await page.waitForFunction(() => (window as Window & { stefTestReady?: boolean }).stefTestReady === true);
  await page.evaluate(() => {
    const event = { id: '1', timestamp: '2026-10-31T10:00:00Z', session_id: 'browser-security-chat', agent_id: 'test-agent', node_name: 'safe.ts', source: 'agent', action: 'modified', status: 'ready', summary: 'Validate event boundary', why: 'Keep saved data safe.' };
    const snapshot = { type: 'snapshot', repo: '/sandbox', session_id: 'browser-security-chat', update_mode: 'files', updates_from: '0', events: [event] };
    window.dispatchEvent(new MessageEvent('message', { data: { type: 'stef.stream', message: snapshot }, source: window, origin: 'https://untrusted.example' }));
    window.dispatchEvent(new MessageEvent('message', { data: { type: 'stef.stream', message: { ...snapshot, events: [{ ...event, id: 'broken' }] } } }));
    window.dispatchEvent(new MessageEvent('message', { data: { type: 'stef.connection', connected: 'yes' } }));
  });
  await expect(page.getByTestId('workflow-step')).toHaveCount(0);
  await page.evaluate(() => {
    const event = { id: '1', timestamp: '2026-10-31T10:00:00Z', session_id: 'browser-security-chat', agent_id: 'test-agent', node_name: 'safe.ts', source: 'agent', action: 'modified', status: 'ready', summary: 'Validate event boundary', why: 'Keep saved data safe.' };
    window.dispatchEvent(new MessageEvent('message', { data: { type: 'stef.stream', message: { type: 'snapshot', repo: '/sandbox', session_id: 'browser-security-chat', update_mode: 'files', updates_from: '0', events: [event] } } }));
    window.dispatchEvent(new MessageEvent('message', { data: { type: 'stef.connection', connected: true } }));
  });
  await expect(page.getByTestId('workflow-step')).toHaveCount(1);
  await expect(page.getByRole('button', { name: 'Updates', exact: true })).toBeEnabled();
  expect(errors).toEqual([]);
  expect(mutations).toEqual([]);
});
