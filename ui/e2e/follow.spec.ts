import { test, expect, type APIRequestContext, type Page } from '@playwright/test';

type Design = 'minimal' | 'interactive';

async function start(page: Page, request: APIRequestContext, session: string) {
  expect((await request.post('/api/settings', { data: { update_mode: 'files' } })).ok()).toBe(true);
  expect((await request.post('/api/sessions/activate', { data: { session_id: session } })).ok()).toBe(true);
  await page.goto('/?companion=1&session_id=' + session);
  await expect(page.getByRole('button', { name: 'Updates', exact: true })).toBeEnabled();
}

async function report(request: APIRequestContext, session: string, node: string, summary: string, why: string, contextId?: string): Promise<string> {
  const response = await request.post('/api/events', { data: {
    session_id: session, agent_id: 'follow-test-agent', node_name: node, action: 'modified',
    summary, why, status: 'ready', ...(contextId ? { context_id: contextId } : {}),
  } });
  expect(response.ok()).toBe(true);
  const receipt = await response.json();
  expect(receipt.accepted).toBe(true);
  return receipt.event.id;
}

async function design(page: Page, value: Design) {
  await page.getByRole('button', { name: 'Design', exact: true }).click();
  await page.getByRole('menuitemradio', { name: value === 'minimal' ? 'Minimal' : 'Interactive', exact: true }).click();
  await expect(page.locator('.stef-app')).toHaveAttribute('data-design', value);
}

async function captureSockets(page: Page) {
  await page.addInitScript(() => {
    const Original = window.WebSocket;
    const sockets: WebSocket[] = [];
    window.WebSocket = class extends Original {
      constructor(url: string | URL, protocols?: string | string[]) { super(url, protocols); sockets.push(this); }
    };
    (window as Window & { stefFollowSockets?: WebSocket[] }).stefFollowSockets = sockets;
  });
}

async function reconnect(page: Page) {
  const count = await page.evaluate(() => (window as Window & { stefFollowSockets?: WebSocket[] }).stefFollowSockets!.length);
  await page.evaluate(() => (window as Window & { stefFollowSockets?: WebSocket[] }).stefFollowSockets!.at(-1)!.close());
  await expect.poll(() => page.evaluate(() => (window as Window & { stefFollowSockets?: WebSocket[] }).stefFollowSockets!.length)).toBe(count + 1);
  await expect(page.getByRole('button', { name: 'Updates', exact: true })).toBeEnabled();
}

for (const view of ['minimal', 'interactive'] as const) {
  test(view + ' follows by default, pauses for reading and resumes only through Follow latest', async ({ page, request }) => {
    const errors: string[] = [], writes: string[] = [];
    page.on('pageerror', error => errors.push(error.message));
    page.on('request', outgoing => {
      if (outgoing.method() !== 'GET' && new URL(outgoing.url()).pathname.startsWith('/api/')) writes.push(new URL(outgoing.url()).pathname);
    });
    await captureSockets(page);
    const session = 'manual-follow-' + view;
    await start(page, request, session);
    await design(page, view);
    const firstId = await report(request, session, 'follow-first.ts', 'Review first change', 'Keep the first route readable while newer work arrives.');
    await expect(page.getByRole('button', { name: 'Review first change', exact: true })).toHaveAttribute('aria-pressed', 'true');
    await page.goto('about:blank');
    await report(request, session, 'follow-second.ts', 'Review second change', 'Show the next change when the developer is following.');
    await page.goto('/?companion=1&session_id=' + session);
    await expect(page.getByRole('button', { name: 'Follow latest', exact: true })).toHaveCount(0);
    await expect(page.getByRole('button', { name: 'Review second change', exact: true })).toHaveAttribute('aria-pressed', 'true');
    const updates = page.getByRole('button', { name: 'Updates', exact: true });
    await updates.focus();
    await report(request, session, 'follow-third.ts', 'Review third change', 'Keep tracking the task without moving keyboard focus.');
    await expect(page.getByRole('button', { name: 'Review third change', exact: true })).toHaveAttribute('aria-pressed', 'true');
    await expect(updates).toBeFocused();
    const first = page.getByRole('button', { name: 'Review first change', exact: true });
    const follow = page.getByRole('button', { name: 'Follow latest', exact: true });
    await first.click();
    await expect(first).toHaveAttribute('aria-pressed', 'true');
    // The control is already useful before any newer work is waiting.
    await expect(follow).toBeEnabled();
    await report(request, session, 'follow-fourth.ts', 'Review fourth change', 'Keep recording while the developer reads an earlier step.');
    await report(request, session, 'follow-fifth.ts', 'Review fifth change', 'Retain the selected explanation until following resumes.');
    await expect(page.getByTestId('workflow-step')).toHaveCount(5);
    await expect(first).toHaveAttribute('aria-pressed', 'true');
    const reason = 'This retained route stays selected while its requested explanation becomes clearer.';
    await report(request, session, 'follow-first.ts', 'Clarify the first change', reason, firstId);
    await expect(first).toHaveAttribute('aria-pressed', 'true');
    await expect(page.getByRole('region', { name: view === 'minimal' ? 'Why this step' : 'Selected change', exact: true })).toContainText(reason);
    await page.reload();
    await expect(first).toHaveAttribute('aria-pressed', 'true');
    await expect(follow).toBeEnabled();
    await reconnect(page);
    await expect(first).toHaveAttribute('aria-pressed', 'true');
    await design(page, view === 'minimal' ? 'interactive' : 'minimal');
    await expect(first).toHaveAttribute('aria-pressed', 'true');
    await expect(follow).toBeEnabled();
    await page.setViewportSize({ width: 320, height: 640 });
    await expect(follow).toBeVisible();
    const bounds = await follow.boundingBox();
    expect(bounds!.x).toBeGreaterThanOrEqual(0);
    expect(bounds!.x + bounds!.width).toBeLessThanOrEqual(320);
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
    await follow.click();
    const latest = page.getByRole('button', { name: 'Review fifth change', exact: true });
    await expect(latest).toHaveAttribute('aria-pressed', 'true');
    await expect(latest).not.toBeFocused();
    await expect(follow).toHaveCount(0);
    await updates.focus();
    await report(request, session, 'follow-sixth.ts', 'Review sixth change', 'Continue following new work after the developer opts in.');
    await expect(page.getByRole('button', { name: 'Review sixth change', exact: true })).toHaveAttribute('aria-pressed', 'true');
    await expect(updates).toBeFocused();
    const sixth = page.getByRole('button', { name: 'Review sixth change', exact: true });
    await sixth.click();
    await expect(sixth).toHaveAttribute('aria-pressed', 'true');
    await report(request, session, 'follow-seventh.ts', 'Review seventh change', 'Clicking the automatic selection pauses it without collapsing the explanation.');
    await expect(sixth).toHaveAttribute('aria-pressed', 'true');
    await expect(follow).toBeEnabled();
    await follow.click();
    await expect(page.getByRole('button', { name: 'Review seventh change', exact: true })).toHaveAttribute('aria-pressed', 'true');
    // The old page cannot update its saved automatic selection while closed.
    const context = page.context();
    await page.close();
    await report(request, session, 'follow-eighth.ts', 'Track a reopened view', 'A following view catches up to work recorded while it was closed.');
    const reopened = await context.newPage();
    reopened.on('pageerror', error => errors.push(error.message));
    reopened.on('request', outgoing => {
      if (outgoing.method() !== 'GET' && new URL(outgoing.url()).pathname.startsWith('/api/')) writes.push(new URL(outgoing.url()).pathname);
    });
    await reopened.goto('/?companion=1&session_id=' + session);
    await expect(reopened.getByRole('button', { name: 'Track a reopened view', exact: true })).toHaveAttribute('aria-pressed', 'true');
    await expect(reopened.getByRole('button', { name: 'Review seventh change', exact: true })).toHaveAttribute('aria-pressed', 'false');
    await expect(reopened.getByRole('button', { name: 'Follow latest', exact: true })).toHaveCount(0);
    expect(writes).toEqual([]);
    expect(errors).toEqual([]);
  });

  test(view + ' scopes a paused view to its workspace and resets following for Clear and a new recording window', async ({ page, request }) => {
    const session = 'follow-boundaries-' + view;
    await start(page, request, session);
    await design(page, view);
    await report(request, session, 'boundary-first.ts', 'Keep the original step', 'Keep inspection local to the selected workspace.');
    await report(request, session, 'boundary-second.ts', 'Add another step', 'Give the developer an earlier step to inspect.');
    const first = page.getByRole('button', { name: 'Keep the original step', exact: true });
    const follow = page.getByRole('button', { name: 'Follow latest', exact: true });
    await first.click();
    await expect(follow).toBeEnabled();
    const other = session + '-other';
    await start(page, request, other);
    await report(request, other, 'other.ts', 'Track the other workspace', 'A new workspace starts following its own incoming work.');
    await expect(page.getByRole('button', { name: 'Track the other workspace', exact: true })).toHaveAttribute('aria-pressed', 'true');
    await expect(follow).toHaveCount(0);
    await page.goto('/?companion=1&session_id=' + session);
    await expect(first).toHaveAttribute('aria-pressed', 'true');
    await expect(follow).toBeEnabled();
    await report(request, session, 'boundary-third.ts', 'Keep recording new work', 'Returning to a paused workspace restores its inspection.');
    await expect(first).toHaveAttribute('aria-pressed', 'true');
    await page.getByRole('button', { name: 'Clear this chat’s workflow', exact: true }).click();
    await expect(page.getByTestId('workflow-step')).toHaveCount(0);
    await expect(follow).toHaveCount(0);
    await report(request, session, 'clear-first.ts', 'Follow after Clear', 'Clearing a workflow starts its new work with following enabled.');
    await expect(page.getByRole('button', { name: 'Follow after Clear', exact: true })).toHaveAttribute('aria-pressed', 'true');
    await report(request, session, 'clear-second.ts', 'Continue after Clear', 'New work continues to select itself in the cleared workflow.');
    await page.getByRole('button', { name: 'Follow after Clear', exact: true }).click();
    await expect(follow).toBeEnabled();
    await page.getByRole('button', { name: 'Updates', exact: true }).click();
    await page.getByRole('menuitemradio', { name: 'Major milestones', exact: true }).click();
    await expect(page.getByTestId('workflow-step')).toHaveCount(0);
    await expect(follow).toHaveCount(0);
    await report(request, session, '@workflow', 'Follow the new window', 'A new reporting window begins with following enabled.');
    await expect(page.getByRole('button', { name: 'Follow the new window', exact: true })).toHaveAttribute('aria-pressed', 'true');
    await page.reload();
    await report(request, session, 'window-next.ts', 'Continue in this window', 'The new window keeps following after a normal reload.');
    await expect(page.getByRole('button', { name: 'Continue in this window', exact: true })).toHaveAttribute('aria-pressed', 'true');
    await expect(follow).toHaveCount(0);
  });

  test(view + ' catches up to a new reporting window after disconnect and can follow an empty current window', async ({ page, request }) => {
    const errors: string[] = [], writes: string[] = [];
    page.on('pageerror', error => errors.push(error.message));
    page.on('request', outgoing => {
      if (outgoing.method() !== 'GET' && new URL(outgoing.url()).pathname.startsWith('/api/')) writes.push(new URL(outgoing.url()).pathname);
    });
    await captureSockets(page);
    const session = 'follow-offline-window-' + view;
    await start(page, request, session);
    await design(page, view);
    await report(request, session, 'offline-first.ts', 'Keep the old selection', 'Let the developer inspect the earlier recording window.');
    await report(request, session, 'offline-second.ts', 'Add the second step', 'Keep another recorded step in this original window.');
    const first = page.getByRole('button', { name: 'Keep the old selection', exact: true });
    const follow = page.getByRole('button', { name: 'Follow latest', exact: true });
    await first.click();
    await expect(follow).toBeEnabled();
    await page.context().setOffline(true);
    await page.evaluate(() => (window as Window & { stefFollowSockets?: WebSocket[] }).stefFollowSockets!.at(-1)!.close());
    await expect(page.getByRole('button', { name: 'Updates', exact: true })).toBeDisabled();
    // The independent API fixture remains online while the browser misses both
    // the empty boundary snapshot and the later current-window report.
    expect((await request.post('/api/settings', { data: { session_id: session, update_mode: 'milestones' } })).ok()).toBe(true);
    await report(request, session, '@workflow', 'Follow the changed window', 'The new window already contains a milestone when the view reconnects.');
    await page.context().setOffline(false);
    const milestone = page.getByRole('button', { name: 'Follow the changed window', exact: true });
    await expect(page.getByRole('button', { name: 'Updates', exact: true })).toBeEnabled();
    await expect(page.locator('.stef-app')).toHaveAttribute('data-update-mode', 'milestones');
    await expect(page.getByTestId('workflow-step')).toHaveCount(1);
    await expect(milestone).toHaveAttribute('aria-pressed', 'true');
    await expect(milestone).not.toBeFocused();
    await expect(follow).toHaveCount(0);
    expect((await request.post('/api/settings', { data: { session_id: session, update_mode: 'files' } })).ok()).toBe(true);
    await expect(page.locator('.stef-app')).toHaveAttribute('data-update-mode', 'files');
    await expect(page.getByTestId('workflow-step')).toHaveCount(0);
    await page.getByRole('button', { name: 'Show saved history', exact: true }).click();
    await first.click();
    await page.getByRole('button', { name: 'Hide saved history', exact: true }).click();
    await expect(page.getByTestId('workflow-step')).toHaveCount(0);
    await expect(follow).toBeEnabled();
    await follow.click();
    await expect(follow).toHaveCount(0);
    await report(request, session, 'empty-window-next.ts', 'Follow future work', 'Following can resume before the current window has any steps.');
    await expect(page.getByRole('button', { name: 'Follow future work', exact: true })).toHaveAttribute('aria-pressed', 'true');
    expect(writes).toEqual([]);
    expect(errors).toEqual([]);
  });
}
