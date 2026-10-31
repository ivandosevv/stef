import { test, expect, type APIRequestContext, type Page } from '@playwright/test';
import { writeFile } from 'node:fs/promises';
import { resolve } from 'node:path';

type Mode = 'milestones' | 'files' | 'activity' | 'disabled';
type SavedEvent = { id: string; node_name: string; action: string; source: string; kind?: string; phase?: string; recording_mode?: Mode; why: string; summary: string; diff?: string };

async function start(page: Page, request: APIRequestContext, session: string) {
  expect((await request.post('/api/settings', { data: { update_mode: 'files' } })).ok()).toBe(true);
  expect((await request.post('/api/sessions/activate', { data: { session_id: session } })).ok()).toBe(true);
  await page.goto('/?companion=1&session_id=' + session);
  await expect(page.getByRole('button', { name: 'Updates', exact: true })).toBeEnabled();
}
async function pick(page: Page, mode: Mode) {
  const labels: Record<Mode, string> = { milestones: 'Major milestones', files: 'Every file', activity: 'All activity', disabled: 'Disabled' };
  await page.getByRole('button', { name: 'Updates', exact: true }).click();
  await page.getByRole('menuitemradio', { name: labels[mode], exact: true }).click();
  await expect(page.locator('.stef-app')).toHaveAttribute('data-update-mode', mode);
}
async function activity(request: APIRequestContext, session: string, phase: string, tool = 'exec_command') {
  const response = await request.post('/api/activity', { data: { session_id: session, agent_id: 'activity-agent', phase, tool } });
  expect(response.ok()).toBe(true);
}
async function report(request: APIRequestContext, session: string, action: string, summary: string, why: string, extra: object = {}) {
  const response = await request.post('/api/events', { data: { session_id: session, agent_id: 'activity-agent', node_name: '@workflow', action, summary, why, status: 'ready', ...extra } });
  expect(response.ok()).toBe(true);
  return response.json();
}
async function events(request: APIRequestContext, session: string): Promise<SavedEvent[]> {
  const response = await request.get('/api/events', { params: { session_id: session } });
  expect(response.ok()).toBe(true);
  return (await response.json()).events ?? [];
}

test('All activity records distinct operations, decisions and exact saved revisions', async ({ page, request }) => {
  const session = 'activity-distinct';
  const errors: string[] = []; page.on('pageerror', error => errors.push(error.message));
  await start(page, request, session);
  await page.getByRole('button', { name: 'Updates', exact: true }).click();
  const menu = page.getByRole('menu', { name: 'Updates', exact: true });
  await expect(menu.getByRole('menuitemradio')).toHaveCount(4);
  await expect(menu).toContainText('Private thinking is not exposed.');
  await page.keyboard.press('End');
  await expect(page.getByRole('menuitemradio', { name: 'Disabled', exact: true })).toBeFocused();
  await page.keyboard.press('ArrowUp');
  await expect(page.getByRole('menuitemradio', { name: 'All activity', exact: true })).toBeFocused();
  await page.keyboard.press('Enter');
  await expect(page.locator('.stef-app')).toHaveAttribute('data-update-mode', 'activity');

  for (const phase of ['started', 'searching', 'terminal', 'tool_done', 'terminal', 'completed']) await activity(request, session, phase);
  await report(request, session, 'decided', 'Validate before reading', 'Reject unsafe paths before opening project source.');
  await expect(page.getByTestId('workflow-step')).toHaveCount(7);
  await expect(page.getByTestId('workflow-arrow')).toHaveCount(6);
  const saved = await events(request, session);
  expect(saved.every(event => event.recording_mode === 'activity')).toBe(true);
  expect(saved.filter(event => event.phase === 'terminal')).toHaveLength(2);

  const snapshot = await (await request.get('/api/events', { params: { session_id: session } })).json();
  const root = resolve(snapshot.repo);
  expect(root.startsWith(resolve('..', '.test-workspace') + '/')).toBe(true);
  await writeFile(resolve(root, 'activity-example.ts'), `export const validated = true;
`);
  await expect.poll(async () => (await events(request, 'local')).some(event => event.source === 'watcher' && event.kind === 'file' && event.node_name === 'activity-example.ts')).toBe(true);
  const receipt = await (await request.post('/api/check-in', { data: { session_id: session, agent_id: 'activity-agent', settle_ms: 300, changed_paths: ['activity-example.ts'] } })).json();
  expect(receipt.update_mode).toBe('activity');
  expect(receipt.pending_count).toBe(1);
  const change = receipt.pending_changes[0];
  await report(request, session, change.action, 'Keep a small fixture', 'Capture a saved revision while observing the wider task.', { node_name: change.node_name, change_id: change.change_id });
  await expect(page.getByTestId('workflow-step')).toHaveCount(8);
  await expect(page.getByRole('region', { name: 'Why this step' })).toContainText('Capture a saved revision while observing the wider task.');
  await page.getByRole('button', { name: 'Show diff', exact: true }).click();
  await expect(page.getByRole('dialog', { name: 'File diff', exact: true })).toContainText('export const validated = true;');
  await page.keyboard.press('Escape');

  const terminal = page.getByRole('button', { name: 'Running terminal command · exec_command', exact: true }).last();
  await terminal.click();
  await expect(page.getByRole('region', { name: 'Why this step' })).toContainText('Observed activity. The agent has not shared an explanation.');
  await expect(page.getByRole('button', { name: 'Request explanation', exact: true })).toHaveCount(0);
  await page.reload();
  await expect(terminal).toHaveAttribute('aria-pressed', 'true');
  await expect(page.getByTestId('workflow-step')).toHaveCount(8);
  await page.screenshot({ path: '../artifacts/stef-all-activity.png', fullPage: true });
  expect(errors).toEqual([]);
});

test('recording windows preserve activity provenance and explicit historical explanations', async ({ page, request }) => {
  const session = 'activity-history';
  await start(page, request, session);
  await report(request, session, 'modified', 'Start with the file view', 'Explain the key step without collecting extra decisions.');
  await activity(request, session, 'terminal', 'old-file-mode-tool');
  await expect(page.getByTestId('workflow-step')).toHaveCount(1);
  await pick(page, 'activity');
  await expect(page.getByTestId('workflow-step')).toHaveCount(0);
  await activity(request, session, 'searching', 'Search');
  await activity(request, session, 'terminal', 'Bash');
  await report(request, session, 'decided', 'Use a local boundary', 'Keep workflow observation on the user’s machine.');
  await expect(page.getByTestId('workflow-step')).toHaveCount(3);
  await pick(page, 'milestones');
  await report(request, session, 'modified', 'Verify the boundary', 'Check recording scope before accepting incoming activity.');
  await expect(page.getByTestId('workflow-step')).toHaveCount(1);
  await pick(page, 'files');
  await expect(page.getByTestId('workflow-step')).toHaveCount(0);
  await page.getByRole('button', { name: 'Show saved history', exact: true }).click();
  await expect(page.getByTestId('workflow-step')).toHaveCount(5);
  await expect(page.getByText('Running terminal command · old-file-mode-tool', { exact: true })).toHaveCount(0);

  const mutations: string[] = [];
  page.on('request', outgoing => {
    if (outgoing.method() !== 'GET' && new URL(outgoing.url()).pathname.startsWith('/api/')) mutations.push(new URL(outgoing.url()).pathname);
  });
  await page.getByRole('button', { name: 'Design', exact: true }).click();
  await page.getByRole('menuitemradio', { name: 'Interactive', exact: true }).click();
  await page.getByRole('button', { name: 'Running terminal command · Bash', exact: true }).click();
  const visual = page.getByTestId('change-visual');
  await expect(visual).toHaveAttribute('data-visual-kind', 'activity');
  await expect(visual.locator('.action')).toHaveText('Terminal');
  await expect(visual).toContainText('Observed activity. The agent has not shared an explanation.');
  expect(mutations).toEqual([]);

  await visual.getByRole('button', { name: 'Explain more', exact: true }).click();
  await expect(visual).toContainText('Asked. The agent can respond at its next check-in.');
  expect(mutations).toEqual(['/api/feedback']);
  const retained = (await events(request, session)).find(event => event.phase === 'terminal' && event.recording_mode === 'activity')!;
  await report(request, session, retained.action, 'Check the local transport', 'The terminal test verifies that observed activity stays inside this private workspace.', { node_name: retained.node_name, context_id: retained.id });
  await expect(visual).toContainText('The terminal test verifies that observed activity stays inside this private workspace.');
  await expect(page.getByTestId('workflow-step')).toHaveCount(5);
  await page.reload();
  await expect(page.getByTestId('workflow-step')).toHaveCount(5);
  await expect(visual).toContainText('The terminal test verifies that observed activity stays inside this private workspace.');
});

test('All activity rejects raw thinking, prompts and output and Disabled records no new activity', async ({ page, request }) => {
  const session = 'activity-privacy';
  await start(page, request, session);
  await pick(page, 'activity');
  for (const field of ['thinking', 'analysis', 'prompt', 'arguments', 'stdout']) {
    const response = await request.post('/api/activity', { data: { session_id: session, agent_id: 'activity-agent', phase: 'terminal', tool: 'Bash', [field]: 'private-text-that-must-not-be-collected' } });
    expect(response.status()).toBe(400);
  }
  expect((await request.post('/api/activity', { data: { session_id: session, agent_id: 'activity-agent', phase: 'thinking' } })).status()).toBe(400);
  await activity(request, session, 'terminal', 'Bash');
  await expect(page.getByTestId('workflow-step')).toHaveCount(1);
  expect(JSON.stringify(await events(request, session))).not.toContain('private-text-that-must-not-be-collected');
  await pick(page, 'disabled');
  const before = await events(request, session);
  await activity(request, session, 'searching', 'Search');
  expect((await report(request, session, 'decided', 'Do not record this decision', 'This workspace has disabled new recording.')).accepted).toBe(false);
  expect(await events(request, session)).toEqual(before);
  await expect(page.getByTestId('workflow-step')).toHaveCount(0);
  await expect(page.getByText('Updates disabled', { exact: true })).toBeVisible();
  await pick(page, 'activity');
  await expect(page.getByTestId('workflow-step')).toHaveCount(0);
  await page.getByRole('button', { name: 'Show saved history', exact: true }).click();
  await expect(page.getByTestId('workflow-step')).toHaveCount(1);
});

test('returning to All activity opens a new window without revealing hidden file-mode tools', async ({ page, request }) => {
  const session = 'activity-return-window';
  await start(page, request, session);
  await pick(page, 'activity');
  await activity(request, session, 'terminal', 'Bash');
  await expect(page.getByTestId('workflow-step')).toHaveCount(1);
  const first = (await events(request, session))[0];
  await pick(page, 'files');
  await activity(request, session, 'terminal', 'hidden-middle-tool');
  await expect(page.getByTestId('workflow-step')).toHaveCount(0);
  await pick(page, 'activity');
  await expect(page.getByTestId('workflow-step')).toHaveCount(0);
  await activity(request, session, 'terminal', 'Bash');
  await expect(page.getByTestId('workflow-step')).toHaveCount(1);
  const recorded = await events(request, session);
  const latest = recorded.at(-1)! as SavedEvent & { recording_from: string };
  expect(latest.id).not.toBe(first.id);
  expect(latest.recording_from).not.toBe((first as SavedEvent & { recording_from: string }).recording_from);
  await page.getByRole('button', { name: 'Show saved history', exact: true }).click();
  await expect(page.getByTestId('workflow-step')).toHaveCount(2);
  await expect(page.getByText('Running terminal command · hidden-middle-tool', { exact: true })).toHaveCount(0);
  const firstStep = page.getByRole('button', { name: 'Running terminal command · Bash', exact: true }).first();
  await firstStep.click();
  await expect(firstStep).toHaveAttribute('aria-pressed', 'true');
  await page.reload();
  await expect(page.getByTestId('workflow-step')).toHaveCount(2);
  await expect(firstStep).toHaveAttribute('aria-pressed', 'true');
});

for (const design of ['minimal', 'interactive'] as const) {
  test(design + ' keeps short decision reasons visible while actions advance', async ({ page, request }) => {
    const session = 'visible-decision-' + design;
    const errors: string[] = [], writes: string[] = [];
    page.on('pageerror', error => errors.push(error.message));
    await start(page, request, session);
    await pick(page, 'activity');
    await page.getByRole('button', { name: 'Design', exact: true }).click();
    await page.getByRole('menuitemradio', { name: design === 'minimal' ? 'Minimal' : 'Interactive', exact: true }).click();
    const reason = 'I’m checking startup because the saved preference resets before the interface mounts.';
    const decision = await report(request, session, 'decided', 'Check startup first', reason, { node_name: 'startup.ts' });
    const button = page.getByRole('button', { name: 'Check startup first', exact: true });
    await expect(button).toHaveAttribute('aria-pressed', 'true');
    const row = page.locator('foreignObject').filter({ has: button });
    await expect(row.getByText(reason, { exact: true })).toBeVisible();
    await activity(request, session, 'reading', 'Read');
    await expect(page.getByTestId('workflow-step')).toHaveCount(2);
    await expect(button).toHaveAttribute('aria-pressed', 'false');
    await expect(row.getByText(reason, { exact: true })).toBeVisible();
    await expect(page.getByTestId('workflow-arrow')).toHaveCount(1);
    const data = await events(request, session);
    expect(data[0].id).toBe(decision.event.id);
    expect(data[0].source).toBe('agent');
    expect(data[0].why).toBe(reason);
    expect(data[1].source).toBe('hook');
    page.on('request', outgoing => {
      if (outgoing.method() !== 'GET' && new URL(outgoing.url()).pathname.startsWith('/api/')) writes.push(new URL(outgoing.url()).pathname);
    });
    await page.setViewportSize({ width: 320, height: 640 });
    await expect(row.getByText(reason, { exact: true })).toBeVisible();
    await expect.poll(() => row.evaluate(element => {
      const bounds = element.getBoundingClientRect();
      const last = element.querySelector('p')?.getBoundingClientRect();
      return Boolean(last && last.bottom <= bounds.bottom + 1 && last.left >= bounds.left - 1 && last.right <= bounds.right + 1);
    })).toBe(true);
    const reasonBounds = await row.getByText(reason, { exact: true }).boundingBox();
    const nextBounds = await page.getByTestId('workflow-step').last().boundingBox();
    expect(reasonBounds!.y + reasonBounds!.height).toBeLessThan(nextBounds!.y);
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
    await button.click();
    await expect(row.getByText(reason, { exact: true })).toHaveCount(1);
    await expect(button).toHaveAttribute('aria-pressed', 'true');
    await activity(request, session, 'tool_done', 'Read');
    await expect(button).toHaveAttribute('aria-pressed', 'true');
    await expect(row.getByText(reason, { exact: true })).toBeVisible();
    await page.reload();
    await expect(button).toHaveAttribute('aria-pressed', 'true');
    await expect(row.getByText(reason, { exact: true })).toBeVisible();
    const identifier = 'bindWorkerToPrivateOrExplicitlySharedWorkspaceBeforeAcceptingAgentActivity';
    const longReason = 'I’m checking ' + identifier + ' because new private sessions must bind before any agent activity enters the workspace.';
    await report(request, session, 'decided', 'Inspect session binding', longReason, { node_name: 'binding.ts' });
    const binding = page.getByRole('button', { name: 'Inspect session binding', exact: true });
    const bindingRow = page.locator('foreignObject').filter({ has: binding });
    await expect(bindingRow.getByText(longReason, { exact: true })).toBeVisible();
    await binding.click();
    await expect(bindingRow.getByText(longReason, { exact: true })).toHaveCount(1);
    await expect.poll(() => bindingRow.evaluate(element => {
      const text = element.querySelector('p')!;
      const bounds = element.getBoundingClientRect(), prose = text.getBoundingClientRect();
      return text.scrollWidth <= text.clientWidth + 1 && prose.left >= bounds.left - 1 && prose.right <= bounds.right + 1 && prose.bottom <= bounds.bottom + 1;
    })).toBe(true);
    expect(writes).toEqual([]);
    expect(errors).toEqual([]);
  });
}
