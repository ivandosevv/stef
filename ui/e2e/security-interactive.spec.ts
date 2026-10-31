import { test, expect, type Page } from '@playwright/test';

// Saved stream fixtures exercise the browser boundary without reading a project
// or creating file changes. These interactions must never request agent context.
async function savedView(page: Page, diff: string, path = 'source.ts') {
  const session = 'interactive-security-fixture';
  const workspaces = [{ workspace_id: session, name: 'Calm Robin 1001', shared: false, members: [session] }];
  await page.addInitScript(() => localStorage.setItem('stef-design', 'interactive'));
  await page.route('**/api/workspaces', route => route.fulfill({ json: { workspaces, current_workspace_id: session } }));
  await page.routeWebSocket('**/ws**', socket => socket.send(JSON.stringify({
    type: 'snapshot', repo: '/fixtures/stef', session_id: session, update_mode: 'files', updates_from: '0', workspaces,
    events: [
      { id: '1', session_id: session, timestamp: '2026-10-31T18:00:00Z', agent_id: 'filesystem', node_name: path, action: 'modified', summary: 'Saved source', why: '', status: 'ready', source: 'watcher', kind: 'file', diff },
      { id: '2', session_id: session, timestamp: '2026-10-31T18:00:01Z', agent_id: 'fixture-agent', node_name: path, action: 'modified', summary: 'Change the saved value', why: 'Use the new saved value in this exact change.', status: 'ready', source: 'agent', change_id: '1' },
    ],
  })));
  await page.goto('/?session_id=' + session);
  await expect(page.getByTestId('workflow-step')).toHaveCount(1);
  await expect(page.getByTestId('change-visual')).toBeVisible();
}

test('the opt-in lens animates an exact saved mutation locally and preserves Unicode', async ({ page }) => {
  const errors: string[] = [], writes: string[] = [], external: string[] = [];
  page.on('pageerror', error => errors.push(error.message));
  page.on('request', request => {
    if (request.method() !== 'GET') writes.push(request.url());
    if (new URL(request.url()).hostname !== '127.0.0.1') external.push(request.url());
  });
  await savedView(page, '@@ -9 +9 @@\n-const café = "👩‍💻";\n+const café = "👨‍💻";\n');
  await expect(page.getByTestId('change-lens')).toHaveCount(0);
  await page.getByRole('button', { name: 'Select removed line 9', exact: true }).click();
  await page.getByRole('button', { name: 'Compare line', exact: true }).click();
  const lens = page.getByTestId('change-lens');
  await expect(lens).toHaveAttribute('data-side', 'after');
  await expect(lens.getByTestId('comparison-after')).toHaveText('const café = "👨‍💻";');
  await expect(lens.getByTestId('comparison-after').locator('mark')).toHaveText('👨‍💻');
  await expect(lens.getByTestId('comparison-before').locator('mark')).toHaveText('👩‍💻');
  const beforeBounds = await lens.locator('.source-stage').boundingBox();
  await lens.getByRole('button', { name: 'Before', exact: true }).click();
  await expect(lens).toHaveAttribute('data-side', 'before');
  await expect(lens.getByTestId('comparison-before')).toHaveAttribute('aria-hidden', 'false');
  await expect.poll(() => lens.getByTestId('comparison-before').evaluate(node => getComputedStyle(node).opacity)).toBe('1');
  expect(await lens.locator('.source-stage').boundingBox()).toEqual(beforeBounds);
  await lens.getByRole('button', { name: 'After', exact: true }).focus();
  await page.keyboard.press('Enter');
  await expect(lens).toHaveAttribute('data-side', 'after');
  await page.setViewportSize({ width: 390, height: 844 });
  expect(await lens.evaluate(node => node.scrollWidth <= node.clientWidth + 1)).toBe(true);
  await page.screenshot({ path: '../artifacts/stef-safe-change-lens.png', fullPage: true });
  await page.getByRole('button', { name: 'Compare line', exact: true }).click();
  await expect(lens).toHaveCount(0);
  expect(errors).toEqual([]);
  expect(writes).toEqual([]);
  expect(external).toEqual([]);
});

test('saved source stays escaped and a multi-line rewrite compares the captured block', async ({ page }) => {
  const errors: string[] = [], external: string[] = [];
  page.on('pageerror', error => errors.push(error.message));
  page.on('request', request => { if (new URL(request.url()).hostname !== '127.0.0.1') external.push(request.url()); });
  const payload = '<img src="https://stef-invalid.example/probe" onerror="window.stefInjected = true">';
  await savedView(page, '@@ -1,2 +1 @@\n-old one\n-old two\n+' + payload + '\n');
  await page.getByRole('button', { name: 'Select removed line 2', exact: true }).click();
  await page.getByRole('button', { name: 'Compare line', exact: true }).click();
  const lens = page.getByTestId('change-lens');
  await expect(lens.getByTestId('comparison-after')).toHaveText(payload);
  await expect(lens.locator('img, script, iframe')).toHaveCount(0);
  await lens.getByRole('button', { name: 'Before', exact: true }).click();
  await expect(lens.getByTestId('comparison-before')).toHaveText('old one\nold two');
  await expect(lens).toContainText('Lines 1 to 2');
  expect(await page.evaluate(() => Object.hasOwn(window, 'stefInjected'))).toBe(false);
  expect(external).toEqual([]);
  expect(errors).toEqual([]);
});

test('a bounded comparison explains why its captured diff is unavailable', async ({ page }) => {
  const message = 'Diff unavailable: local comparison exceeds the 4000-line safety limit.';
  await savedView(page, message);
  await expect(page.getByTestId('change-visual')).toContainText(message);
});

test('unsafe color names never produce a sample or break the selected change', async ({ page }) => {
  const errors: string[] = []; page.on('pageerror', error => errors.push(error.message));
  await savedView(page, '@@ -1 +1 @@\n-.x { color: constructor; }\n+.x { color: red; }\n', 'style.css');
  await expect(page.getByTestId('style-effect')).toHaveCount(0);
  await expect(page.getByTestId('change-visual')).toContainText('Use the new saved value');
  expect(errors).toEqual([]);
});
