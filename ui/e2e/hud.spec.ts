import { test, expect, type APIRequestContext, type Page, type Locator } from '@playwright/test';
import { writeFile as filesystemWrite, unlink as filesystemUnlink } from 'node:fs/promises';
import { resolve, relative } from 'node:path';

const report = (session: string, node: string, summary: string, why: string, parents: string[] = []) => ({ session_id: session, agent_id: 'test-agent', node_name: node, action: 'modified', summary, why, status: 'ready', parents });
type UpdateMode = 'milestones' | 'files' | 'activity' | 'disabled';
type SavedEvent = { id: string; source: string; kind?: string; node_name: string; action: string; before?: string; after?: string; change_id?: string; diff?: string };

// Reset the current workspace before each independent browser scenario.
test.beforeEach(async ({ request }) => {
  const response = await request.post('/api/settings', { data: { update_mode: 'files' } });
  expect(response.ok()).toBe(true);
  expect(await response.json()).toEqual({ update_mode: 'files' });
});

// Tests supply concrete saved paths as a trusted editor hook would. A root-wide
// file observer cannot identify which private agent wrote a file on its own.
async function claimSavedPath(file: string, content?: string) {
  const response = await fetch('http://127.0.0.1:4821/api/events');
  const snapshot = await response.json();
  const path = relative(snapshot.repo, file);
  if (snapshot.update_mode === 'files') await expect.poll(async () => {
    const observed = await (await fetch('http://127.0.0.1:4821/api/events?session_id=local')).json();
    const revision = (observed.events ?? []).filter((event: SavedEvent) => event.source === 'watcher' && event.kind === 'file' && event.node_name === path).at(-1);
    if (content === undefined) return revision?.action === 'deleted';
    if (!revision || revision.action === 'deleted') return false;
    const captured = revision.after ?? '';
    return captured === content || (captured.endsWith('\n… [truncated]') && content.startsWith(captured.slice(0, -'\n… [truncated]'.length)));
  }, { timeout: 10000 }).toBe(true);
  const reminder = await fetch('http://127.0.0.1:4821/api/explanation-reminder', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ session_id: snapshot.session_id, paths: [relative(snapshot.repo, file)], settle_ms: 300 }) });
  expect(reminder.ok).toBe(true);
}
async function writeFile(file: string, data: string) { await filesystemWrite(file, data); await claimSavedPath(file, data); }
async function unlink(file: string) { await filesystemUnlink(file); await claimSavedPath(file); }
async function selectSession(page: Page, request: APIRequestContext, session: string) {
  const response = await request.post('/api/sessions/activate', { data: { session_id: session } });
  expect(response.ok()).toBe(true);
  const url = new URL(page.url()); url.searchParams.set('session_id', session);
  await page.goto(url.toString());
  await expect(page.getByRole('button', { name: 'Updates', exact: true })).toBeEnabled();
}

async function savedEvents(request: APIRequestContext, session: string): Promise<SavedEvent[]> {
  const response = await request.get('/api/events', { params: { session_id: session } });
  expect(response.ok()).toBe(true);
  return (await response.json()).events ?? [];
}

async function isolatedRoot(request: APIRequestContext): Promise<string> {
  const snapshot = await (await request.get('/api/events')).json();
  const root = resolve(snapshot.repo);
  expect(root.startsWith(resolve('..', '.test-workspace') + '/')).toBe(true);
  return root;
}

async function pickUpdates(page: Page, mode: UpdateMode) {
  const labels = { milestones: 'Major milestones', files: 'Every file', activity: 'All activity', disabled: 'Disabled' };
  await page.getByRole('button', { name: 'Updates', exact: true }).click();
  await page.getByRole('menuitemradio', { name: new RegExp('^' + labels[mode]) }).click();
  await expect(page.getByRole('menu', { name: 'Updates', exact: true })).toHaveCount(0);
  await expect(page.locator('.stef-app')).toHaveAttribute('data-update-mode', mode);
}

async function pickDesign(page: Page, design: 'minimal' | 'interactive') {
  await page.getByRole('button', { name: 'Design', exact: true }).click();
  await page.getByRole('menuitemradio', { name: design === 'minimal' ? 'Minimal' : 'Interactive', exact: true }).click();
  await expect(page.getByRole('menu', { name: 'Design', exact: true })).toHaveCount(0);
  await expect(page.locator('.stef-app')).toHaveAttribute('data-design', design);
}

async function checkIn(request: APIRequestContext, session: string, settle = false) {
  const response = await request.post('/api/check-in', { data: { session_id: session, agent_id: 'test-agent', ...(settle ? { settle_ms: 300 } : {}) } });
  expect(response.ok()).toBe(true);
  return response.json();
}

// A live step may already be selected. Selecting it again would collapse its detail.
async function chooseStep(step: Locator) {
  await expect(step).toBeVisible();
  if (await step.getAttribute('aria-pressed') !== 'true') await step.click();
  await expect(step).toHaveAttribute('aria-pressed', 'true');
}

async function captureSockets(page: Page) {
  await page.addInitScript(() => {
    const Original = window.WebSocket;
    const sockets: WebSocket[] = [];
    window.WebSocket = class extends Original {
      constructor(url: string | URL, protocols?: string | string[]) { super(url, protocols); sockets.push(this); }
    };
    (window as Window & { stefTestSockets?: WebSocket[] }).stefTestSockets = sockets;
  });
}

async function reconnect(page: Page) {
  const count = await page.evaluate(() => (window as Window & { stefTestSockets?: WebSocket[] }).stefTestSockets!.length);
  await page.evaluate(() => (window as Window & { stefTestSockets?: WebSocket[] }).stefTestSockets!.at(-1)!.close());
  await expect.poll(() => page.evaluate(() => (window as Window & { stefTestSockets?: WebSocket[] }).stefTestSockets!.length)).toBe(count + 1);
  await expect(page.getByRole('button', { name: 'Updates', exact: true })).toBeEnabled();
}

async function expectLatestVisible(page: Page) {
  const steps = page.getByTestId('workflow-step');
  await expect(steps.last()).toHaveAttribute('aria-pressed', 'true');
  await expect(page.locator('[data-testid="workflow-step"][aria-pressed="true"]')).toHaveCount(1);
  await expect.poll(() => steps.last().evaluate(element => {
    const rect = element.getBoundingClientRect();
    const viewport = element.closest('.workflow-canvas')!;
    const bounds = viewport.getBoundingClientRect();
    const scrolls = getComputedStyle(viewport).overflowY === 'auto';
    const top = scrolls ? Math.max(0, bounds.top) : 0;
    const bottom = scrolls ? Math.min(window.innerHeight, bounds.bottom) : window.innerHeight;
    return rect.top >= top - 1 && rect.bottom <= bottom + 1;
  })).toBe(true);
  if (await page.locator('.stef-app').getAttribute('data-design') === 'minimal') {
    await expect.poll(() => page.getByRole('region', { name: 'Why this step' }).evaluate(element => {
      const rect = element.getBoundingClientRect();
      return rect.top >= -1 && rect.bottom <= window.innerHeight + 1;
    })).toBe(true);
  }
}

async function wheelOver(page: Page, pane: Locator, delta: number) {
  const box = await pane.boundingBox();
  expect(box).not.toBeNull();
  await page.mouse.move(box!.x + box!.width / 2, box!.y + box!.height / 2);
  await page.mouse.wheel(0, delta);
  // Let the compositor process the wheel before checking unchanged boundary panes.
  await page.evaluate(() => new Promise<void>(resolve => requestAnimationFrame(() => requestAnimationFrame(() => resolve()))));
}

test('the minimal live workflow has arrows, brief reasons and an inline suggestion', async ({ page, request }) => {
  const errors: string[] = []; page.on('pageerror', error => errors.push(error.message));
  await page.goto('/?companion=1');
  await expect(page).toHaveTitle('stef');
  await selectSession(page, request, 'minimal-chat');
  await expect(page.getByText('Listening…')).toBeVisible();
  const snapshot = await (await request.get('/api/events')).json();
  const root = resolve(snapshot.repo);
  expect(root.startsWith(resolve('..', '.test-workspace') + '/')).toBe(true);
  await writeFile(resolve(root, 'main.go'), 'package main\nfunc Run() string { return "live" }\n');
  await expect.poll(async () => ((await (await request.get('/api/events')).json()).events ?? []).length).toBeGreaterThan(0);
  await expect(page.getByTestId('workflow-step')).toHaveCount(1);
  const observed = (await (await request.get('/api/events')).json()).events.find((event: { source: string; kind: string; node_name: string }) => event.source === 'watcher' && event.kind === 'file' && event.node_name === 'main.go');
  expect(observed).toBeTruthy();
  await request.post('/api/events', { data: { ...report('minimal-chat', 'main.go', 'Find the entry point', 'Requests enter here.'), action: observed.action, change_id: observed.id } });
  await request.post('/api/events', { data: report('minimal-chat', 'router.go', 'Update the route', 'Validate requests before handling them.', ['main.go']) });
  await expect(page.getByTestId('workflow-step')).toHaveCount(2);
  await expect(page.getByRole('region', { name: 'Why this step' })).toContainText('Validate requests before handling them.');
  await expect(page.getByRole('textbox', { name: 'Suggest something to the agent' })).toHaveCount(0);
  await expect(page.getByTestId('workflow-arrow')).toHaveCount(1);
  await expect(page.getByTestId('workflow-arrow').locator('title')).toHaveText('Connection reported by the agent');
  await chooseStep(page.getByRole('button', { name: /Update the route/ }));
  const detail = page.getByRole('region', { name: 'Why this step' });
  await expect(detail).toContainText('Validate requests before handling them.');
  await page.getByRole('button', { name: 'Suggest', exact: true }).click();
  await page.getByRole('textbox', { name: 'Suggest something to the agent' }).fill('Please check whitespace-only input.');
  await page.getByRole('button', { name: 'Send suggestion' }).click();
  await expect(detail).toContainText('Queued for the agent’s next check-in.');
  const state = await (await request.post('/api/check-in', { data: { session_id: 'minimal-chat', agent_id: 'test-agent' } })).json();
  expect(state.feedback).toHaveLength(1);
  expect(state.feedback[0].text).toBe('Please check whitespace-only input.');
  await expect(page.getByTestId('workflow-step')).toHaveCount(2);
  await expect(page.getByRole('combobox')).toHaveCount(0);
  await expect(page.getByText('Code diff', { exact: true })).toHaveCount(0);
  await expect(page.getByRole('region', { name: 'Session replay' })).toHaveCount(0);
  await page.screenshot({ path: '../artifacts/stef-minimal-workflow.png', fullPage: true });
  await page.setViewportSize({ width: 390, height: 844 });
  await page.screenshot({ path: '../artifacts/stef-minimal-mobile.png', fullPage: true });
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
  expect(errors).toEqual([]);
});

test('every saved file appears automatically and each revision gets its own explanation', async ({ page, request }) => {
  const errors: string[] = []; page.on('pageerror', error => errors.push(error.message));
  await page.goto('/?companion=1');
  const session = 'automatic-file-changes';
  await selectSession(page, request, session);
  await expect(page.getByText('Listening…')).toBeVisible();
  const snapshot = await (await request.get('/api/events')).json();
  const root = resolve(snapshot.repo);
  expect(root.startsWith(resolve('..', '.test-workspace') + '/')).toBe(true);

  await writeFile(resolve(root, 'theme.ts'), 'export const background = "warm";\n');
  const themeSteps = page.locator('[data-testid="workflow-step"][title^="theme.ts"]');
  await expect(themeSteps).toHaveCount(1);
  await chooseStep(themeSteps.first());
  await expect(page.getByRole('region', { name: 'Why this step' })).toContainText('The agent hasn’t explained this save yet.');
  await writeFile(resolve(root, 'main.ts'), 'import { background } from "./theme";\nconsole.log(background);\n');
  await expect(page.locator('[data-testid="workflow-step"][title^="main.ts"]')).toHaveCount(1);
  await expect(page.getByTestId('workflow-step')).toHaveCount(2);
  await expect(page.getByTestId('workflow-arrow')).toHaveCount(1);

  const changes = (await (await request.get('/api/events')).json()).events;
  const themeChange = changes.find((event: { source: string; kind: string; node_name: string }) => event.source === 'watcher' && event.kind === 'file' && event.node_name === 'theme.ts');
  expect(themeChange).toBeTruthy();
  const explanation = 'Keep palette values separate so appearance changes stay consistent.';
  const result = await request.post('/api/events', { data: { ...report(session, 'theme.ts', 'Define the palette', explanation), action: themeChange.action, change_id: themeChange.id } });
  expect(result.status()).toBe(201);
  await expect(page.locator('[data-testid="workflow-step"][title^="main.ts"]')).toHaveAttribute('aria-pressed', 'true');
  await expect(themeSteps.first()).toHaveAttribute('aria-pressed', 'false');
  await chooseStep(themeSteps.first());
  await expect(page.getByRole('region', { name: 'Why this step' })).toContainText(explanation);
  await expect(page.getByTestId('workflow-step')).toHaveCount(2);

  await writeFile(resolve(root, 'theme.ts'), 'export const background = "light";\n');
  await expect(themeSteps).toHaveCount(2);
  await expect(page.getByTestId('workflow-step')).toHaveCount(3);
  await expect(page.getByTestId('workflow-arrow')).toHaveCount(2);
  await chooseStep(themeSteps.last());
  await expect(page.getByRole('region', { name: 'Why this step' })).toContainText('The agent hasn’t explained this save yet.');
  await expect(page.getByRole('region', { name: 'Why this step' })).not.toContainText(explanation);
  await page.reload();
  await expect(page.getByTestId('workflow-step')).toHaveCount(3);
  await expect(page.getByRole('region', { name: 'Why this step' })).toContainText('The agent hasn’t explained this save yet.');
  await chooseStep(themeSteps.first());
  await expect(page.getByRole('region', { name: 'Why this step' })).toContainText(explanation);
  await page.screenshot({ path: '../artifacts/stef-automatic-file-changes.png', fullPage: true });
  expect(errors).toEqual([]);
});

test('new chats reset, resumed chats restore, and Clear affects only that chat', async ({ page, request }) => {
  await page.goto('/');
  await selectSession(page, request, 'chat-a');
  await request.post('/api/events', { data: report('chat-a', 'a.go', 'Update the handler', 'Keep validation at the boundary.') });
  await expect(page.getByRole('button', { name: /Update the handler/ })).toBeVisible();
  await chooseStep(page.getByRole('button', { name: /Update the handler/ }));
  await selectSession(page, request, 'chat-b');
  await expect(page.getByTestId('workflow-step')).toHaveCount(0);
  await expect(page.getByRole('region', { name: 'Why this step' })).toHaveCount(0);
  await request.post('/api/events', { data: report('chat-b', 'b.go', 'Check the retry limit', 'Retries must stop eventually.') });
  await selectSession(page, request, 'chat-a');
  await expect(page.getByRole('button', { name: /Update the handler/ })).toBeVisible();
  await expect(page.getByRole('region', { name: 'Why this step' })).toContainText('Keep validation at the boundary.');
  await page.getByRole('button', { name: 'Clear this chat’s workflow' }).click();
  await expect(page.getByTestId('workflow-step')).toHaveCount(0);
  await page.reload();
  await expect(page.getByText('Listening…')).toBeVisible();
  await selectSession(page, request, 'chat-b');
  await expect(page.getByRole('button', { name: /Check the retry limit/ })).toBeVisible();
  await request.post('/api/events', { data: report('chat-a', 'fresh.go', 'Start a fresh step', 'Continue after clearing.') });
  await expect(page.getByTestId('workflow-step')).toHaveCount(1);
  await expect(page.getByRole('button', { name: /Check the retry limit/ })).toBeVisible();
  await selectSession(page, request, 'chat-a');
  await expect(page.getByRole('button', { name: /Start a fresh step/ })).toBeVisible();
  await expect(page.getByTestId('workflow-arrow')).toHaveCount(0);
});

test('appearance changes the whole workflow and survives reload, chat switching and Clear', async ({ page, request }) => {
  const errors: string[] = []; page.on('pageerror', error => errors.push(error.message));
  await page.emulateMedia({ colorScheme: 'dark' });
  await page.goto('/?companion=1');
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark');
  await selectSession(page, request, 'theme-chat-a');
  await request.post('/api/events', { data: report('theme-chat-a', 'entry.ts', 'Find the palette', 'The workflow shares its page colors.') });
  await request.post('/api/events', { data: report('theme-chat-a', 'palette.ts', 'Check readability', 'Keep labels and arrows visible in every appearance.', ['entry.ts']) });
  const selected = page.getByRole('button', { name: 'Check readability', exact: true });
  await chooseStep(selected);
  await page.getByRole('button', { name: 'Suggest', exact: true }).click();
  const input = page.getByRole('textbox', { name: 'Suggest something to the agent' });
  await input.fill('Please keep the workflow readable.');

  const appearance = page.getByRole('button', { name: /appearance/i });
  await appearance.click();
  await expect(page.getByRole('menuitemradio', { name: 'Dark', exact: true })).toHaveAttribute('aria-checked', 'true');
  await page.keyboard.press('Escape');
  await expect(page.getByRole('menu')).toHaveCount(0);
  await expect(appearance).toBeFocused();
  await appearance.click();
  await page.getByRole('main', { name: 'Workflow' }).click({ position: { x: 10, y: 10 } });
  await expect(page.getByRole('menu')).toHaveCount(0);

  const contrast = (foreground: string, background: string): number => {
    const luminance = (color: string) => {
      const channels = (color.match(/[\d.]+/g) ?? []).slice(0, 3).map(value => {
        const channel = Number(value) / 255;
        return channel <= 0.04045 ? channel / 12.92 : ((channel + 0.055) / 1.055) ** 2.4;
      });
      expect(channels).toHaveLength(3);
      return channels[0] * 0.2126 + channels[1] * 0.7152 + channels[2] * 0.0722;
    };
    const a = luminance(foreground), b = luminance(background);
    return (Math.max(a, b) + 0.05) / (Math.min(a, b) + 0.05);
  };
  const palettes = [];
  for (const [name, theme] of [['Dark', 'dark'], ['Light', 'light'], ['Eye comfort', 'comfort']]) {
    await appearance.click();
    await page.getByRole('menuitemradio', { name, exact: true }).click();
    await expect(page.getByRole('menu')).toHaveCount(0);
    await expect(appearance).toBeFocused();
    await expect(page.locator('html')).toHaveAttribute('data-theme', theme);
    await expect(selected).toHaveAttribute('aria-pressed', 'true');
    await expect(page.getByTestId('workflow-step')).toHaveCount(2);
    await expect(page.getByTestId('workflow-arrow')).toHaveCount(1);
    await expect(input).toHaveValue('Please keep the workflow readable.');
    const colors = await page.evaluate(() => {
      const css = (selector: string, property: string) => getComputedStyle(document.querySelector(selector)!).getPropertyValue(property);
      return {
        background: css('html', 'background-color'), foreground: css('html', 'color'),
        label: css('.step:not(.selected)', 'color'),
        selectedForeground: css('.step.selected', 'color'), selectedBackground: css('.step.selected', 'background-color'),
        reason: css('.step-detail p', 'color'), muted: css('.step-detail .file', 'color'),
        inputForeground: css('.step-detail input', 'color'), inputBackground: css('.step-detail input', 'background-color'),
        arrow: css('.flow-arrow', 'stroke'), arrowhead: css('#workflow-arrow path', 'stroke'),
        scheme: css('html', 'color-scheme'),
      };
    });
    expect(colors.scheme).toBe(theme === 'dark' ? 'dark' : 'light');
    expect(colors.arrowhead).toBe(colors.arrow);
    for (const text of [colors.foreground, colors.label, colors.reason, colors.muted]) {
      expect(contrast(text, colors.background), name + ' text contrast').toBeGreaterThanOrEqual(4.5);
    }
    expect(contrast(colors.selectedForeground, colors.selectedBackground), name + ' selected text contrast').toBeGreaterThanOrEqual(4.5);
    expect(contrast(colors.inputForeground, colors.inputBackground), name + ' input contrast').toBeGreaterThanOrEqual(4.5);
    expect(contrast(colors.arrow, colors.background), name + ' arrow contrast').toBeGreaterThanOrEqual(3);
    palettes.push(colors);
    if (theme !== 'dark') {
      await page.setViewportSize({ width: 390, height: 844 });
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
      await page.screenshot({ path: '../artifacts/stef-theme-' + theme + '.png', fullPage: true });
    }
  }
  for (const property of ['background', 'foreground', 'label', 'arrow'] as const) {
    expect(new Set(palettes.map(palette => palette[property])).size, property + ' changes for each theme').toBe(3);
  }
  await expect.poll(() => page.evaluate(() => localStorage.getItem('stef-theme'))).toBe('comfort');
  await page.reload();
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'comfort');
  await expect(selected).toHaveAttribute('aria-pressed', 'true');
  await expect(page.getByTestId('workflow-step')).toHaveCount(2);
  await expect(page.getByRole('region', { name: 'Why this step' })).toContainText('Keep labels and arrows visible in every appearance.');
  await selectSession(page, request, 'theme-chat-b');
  await expect(page.getByText('Listening…')).toBeVisible();
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'comfort');
  await request.post('/api/events', { data: report('theme-chat-b', 'fresh.ts', 'Try a new chat', 'Appearance belongs to the view, not this chat.') });
  await expect(page.getByTestId('workflow-step')).toHaveCount(1);
  await page.getByRole('button', { name: 'Clear this chat’s workflow' }).click();
  await expect(page.getByText('Listening…')).toBeVisible();
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'comfort');
  await selectSession(page, request, 'theme-chat-a');
  await expect(selected).toHaveAttribute('aria-pressed', 'true');
  await expect(page.getByTestId('workflow-step')).toHaveCount(2);
  await appearance.click();
  await page.getByRole('menuitemradio', { name: 'System', exact: true }).click();
  await expect.poll(() => page.evaluate(() => localStorage.getItem('stef-theme'))).toBe('system');
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark');
  await page.emulateMedia({ colorScheme: 'light' });
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'light');
  await expect(selected).toHaveAttribute('aria-pressed', 'true');
  await expect(page.getByTestId('workflow-step')).toHaveCount(2);
  await page.reload();
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'light');
  await appearance.click();
  await expect(page.getByRole('menuitemradio', { name: 'System', exact: true })).toHaveAttribute('aria-checked', 'true');
  await page.keyboard.press('Escape');
  expect(errors).toEqual([]);
});

test('mode changes start a future reporting window while saved history remains available on demand', async ({ page, request }) => {
  const session = 'updates-milestones';
  await page.goto('/?companion=1');
  await selectSession(page, request, session);
  const root = await isolatedRoot(request);
  const file = resolve(root, 'updates-original.ts');
  await writeFile(file, 'export const mode = "files";\n');
  const fileSteps = page.locator('[data-testid="workflow-step"][title^="updates-original.ts"]');
  await expect(fileSteps).toHaveCount(1);
  const original = (await savedEvents(request, session)).find(event => event.source === 'watcher' && event.kind === 'file' && event.node_name === 'updates-original.ts')!;
  expect(original).toBeTruthy();
  const explanation = 'Store the preference once so every companion follows the same setting.';
  const explained = await request.post('/api/events', { data: { ...report(session, original.node_name, 'Store update preference', explanation), action: original.action, change_id: original.id } });
  expect((await explained.json()).accepted).toBe(true);
  await request.post('/api/events', { data: report(session, '@workflow', 'Choose update frequency', 'Let developers choose how much of the workflow they want to follow.') });
  await request.post('/api/activity', { data: { session_id: session, agent_id: 'test-agent', phase: 'reading', node_name: 'updates-context.ts', summary: 'Read context' } });
  await expect(page.getByTestId('workflow-step')).toHaveCount(3);
  await chooseStep(fileSteps);
  await page.getByRole('button', { name: 'Suggest', exact: true }).click();
  await page.getByRole('textbox', { name: 'Suggest something to the agent' }).fill('Please keep the preference shared.');

  await pickUpdates(page, 'milestones');
  await expect(page.getByTestId('workflow-step')).toHaveCount(0);
  await expect(page.getByRole('button', { name: 'Choose update frequency', exact: true })).toHaveCount(0);
  await expect(fileSteps).toHaveCount(0);
  await expect(page.getByRole('button', { name: 'Read context', exact: true })).toHaveCount(0);
  await expect(page.getByRole('region', { name: 'Why this step' })).toHaveCount(0);
  await writeFile(file, 'export const mode = "milestones";\n');
  await writeFile(resolve(root, 'updates-milestones-only.ts'), 'export const omitted = true;\n');
  const milestoneState = await (await request.post('/api/check-in', { data: { session_id: session, agent_id: 'test-agent', settle_ms: 300 } })).json();
  expect(milestoneState.update_mode).toBe('milestones');
  expect(milestoneState.pending_changes).toEqual([]);
  expect(milestoneState.pending_count).toBe(0);
  expect(milestoneState.events).toHaveLength(0);
  expect(typeof milestoneState.updates_from).toBe('string');
  expect(BigInt(milestoneState.updates_from)).toBeGreaterThan(BigInt(original.id));
  const ignored = await request.post('/api/events', { data: { ...report(session, original.node_name, 'File detail omitted', 'Milestone mode does not add per-file explanations.'), action: original.action, change_id: original.id } });
  expect((await ignored.json()).accepted).toBe(false);
  await request.post('/api/activity', { data: { session_id: session, agent_id: 'test-agent', phase: 'editing', node_name: 'updates-hook.ts' } });
  // An implementation milestone is valid even when its action is "modified".
  const milestone = await request.post('/api/events', { data: report(session, 'updates-settings.ts', 'Connect update preferences', 'Apply the same setting to automatic capture and agent reports.') });
  expect((await milestone.json()).accepted).toBe(true);
  await expect(page.getByTestId('workflow-step')).toHaveCount(1);
  await expect(page.getByTestId('workflow-arrow')).toHaveCount(0);
  await expectLatestVisible(page);
  await expect(page.getByRole('region', { name: 'Why this step' })).toContainText('Apply the same setting to automatic capture and agent reports.');
  await expect(page.getByRole('textbox', { name: 'Suggest something to the agent' })).toHaveCount(0);

  await pickUpdates(page, 'files');
  await expect(page.getByTestId('workflow-step')).toHaveCount(0);
  const resumed = await checkIn(request, session);
  expect(resumed.pending_changes).toEqual([]);
  expect(resumed.pending_count).toBe(0);
  expect(resumed.events).toEqual([]);
  expect(BigInt(resumed.updates_from)).toBeGreaterThan(BigInt(milestoneState.updates_from));
  await page.getByRole('button', { name: 'Show saved history', exact: true }).click();
  await expect(page.getByText(/^Saved history —/)).toBeVisible();
  await expect(page.getByTestId('workflow-step')).toHaveCount(4);
  await chooseStep(fileSteps);
  await expect(page.getByRole('region', { name: 'Why this step' })).toContainText(explanation);
  await expect(page.getByRole('textbox', { name: 'Suggest something to the agent' })).toHaveCount(0);
  await writeFile(file, 'export const mode = "resumed";\n');
  await expect(fileSteps).toHaveCount(2);
  const events = await savedEvents(request, session);
  const revisions = events.filter(event => event.source === 'watcher' && event.kind === 'file' && event.node_name === original.node_name);
  expect(revisions).toHaveLength(2);
  expect(revisions[1].before).toContain('"milestones"');
  expect(revisions[1].after).toContain('"resumed"');
  expect(events.some(event => event.node_name === 'updates-milestones-only.ts' || event.node_name === 'updates-hook.ts')).toBe(false);
  await chooseStep(fileSteps.last());
  await expect(page.getByRole('region', { name: 'Why this step' })).toContainText('The agent hasn’t explained this save yet.');
  await expect(page.getByRole('region', { name: 'Why this step' })).not.toContainText(explanation);
  await page.getByRole('button', { name: 'Hide saved history', exact: true }).click();
  await expect(fileSteps).toHaveCount(1);
  await expect(page.getByTestId('workflow-step')).toHaveCount(1);
});

test('Disabled stops recording and delivery; re-enabling exposes future work and optional saved history', async ({ page, request }) => {
  const session = 'updates-disabled';
  await page.goto('/?companion=1');
  await selectSession(page, request, session);
  const root = await isolatedRoot(request);
  const file = resolve(root, 'updates-disabled-revision.ts');
  await writeFile(file, 'export const state = "enabled";\n');
  const fileSteps = page.locator('[data-testid="workflow-step"][title^="updates-disabled-revision.ts"]');
  await expect(fileSteps).toHaveCount(1);
  const original = (await savedEvents(request, session)).find(event => event.source === 'watcher' && event.kind === 'file' && event.node_name === 'updates-disabled-revision.ts')!;
  expect(original).toBeTruthy();
  await request.post('/api/events', { data: report(session, '@workflow', 'Keep the saved workflow', 'Disabling updates should preserve the work already explained.') });
  await chooseStep(page.getByRole('button', { name: 'Keep the saved workflow', exact: true }));
  await page.getByRole('button', { name: 'Suggest', exact: true }).click();
  await page.getByRole('textbox', { name: 'Suggest something to the agent' }).fill('Deliver this when updates are enabled.');
  await page.getByRole('button', { name: 'Send suggestion' }).click();
  await expect(page.getByRole('region', { name: 'Why this step' })).toContainText('Queued for the agent’s next check-in.');
  const history = await savedEvents(request, session);

  await pickUpdates(page, 'disabled');
  await expect(page.getByText('Updates disabled', { exact: true })).toBeVisible();
  await expect(page.getByTestId('workflow-step')).toHaveCount(0);
  await expect(page.getByRole('button', { name: 'Updates', exact: true })).toBeEnabled();
  await expect(page.getByRole('button', { name: 'Clear this chat’s workflow' })).toBeEnabled();
  await writeFile(file, 'export const state = "disabled";\n');
  await writeFile(resolve(root, 'updates-disabled-only.ts'), 'export const skipped = true;\n');
  const suppressed = await request.post('/api/events', { data: report(session, 'updates-ignored.ts', 'Ignored while disabled', 'The daemon must suppress recording as well as the visible workflow.') });
  const suppressedReceipt = await suppressed.json();
  expect(suppressedReceipt.accepted).toBe(false);
  expect(suppressedReceipt.update_mode).toBe('disabled');
  expect(suppressedReceipt.event.id).toBe('');
  await request.post('/api/activity', { data: { session_id: session, agent_id: 'test-agent', phase: 'editing', node_name: 'updates-disabled-hook.ts' } });
  const batch = await request.post('/api/changes', { data: { session_id: session, agent_id: 'test-agent', changes: [{ change_id: original.id, summary: 'Omitted explanation', why: 'Disabled mode does not request per-file explanations.' }] } });
  const batchReceipt = await batch.json();
  expect(batchReceipt.accepted).toBe(false);
  expect(batchReceipt.ids).toEqual([]);
  expect(batchReceipt.update_mode).toBe('disabled');
  const state = await (await request.post('/api/check-in', { data: { session_id: session, agent_id: 'test-agent', settle_ms: 300 } })).json();
  expect(state.update_mode).toBe('disabled');
  expect(state.events).toEqual([]);
  expect(state.pending_changes).toEqual([]);
  expect(state.pending_count).toBe(0);
  expect(state.feedback ?? []).toEqual([]);
  expect(await savedEvents(request, session)).toEqual(history);

  await page.reload();
  await expect(page.getByText('Updates disabled', { exact: true })).toBeVisible();
  await pickUpdates(page, 'files');
  await expect(page.getByTestId('workflow-step')).toHaveCount(0);
  const resumed = await checkIn(request, session);
  expect(resumed.pending_changes).toEqual([]);
  expect(resumed.pending_count).toBe(0);
  expect(resumed.events).toEqual([]);
  expect(resumed.feedback).toHaveLength(1);
  expect(resumed.feedback[0].text).toBe('Deliver this when updates are enabled.');
  await page.getByRole('button', { name: 'Show saved history', exact: true }).click();
  await expect(page.getByTestId('workflow-step')).toHaveCount(2);
  await expect(page.getByRole('button', { name: 'Keep the saved workflow', exact: true })).toHaveAttribute('aria-pressed', 'true');
  await expect(page.getByRole('region', { name: 'Why this step' })).toContainText('Disabling updates should preserve the work already explained.');
  expect((await checkIn(request, session)).pending_changes).toEqual([]);
  await writeFile(file, 'export const state = "resumed";\n');
  await expect(fileSteps).toHaveCount(2);
  const events = await savedEvents(request, session);
  const revisions = events.filter(event => event.source === 'watcher' && event.kind === 'file' && event.node_name === original.node_name);
  expect(revisions).toHaveLength(2);
  expect(revisions[1].before).toContain('"disabled"');
  expect(revisions[1].after).toContain('"resumed"');
  expect(events.some(event => ['updates-disabled-only.ts', 'updates-disabled-hook.ts', 'updates-ignored.ts'].includes(event.node_name))).toBe(false);
});

test('Updates is shared by views of the same workspace and stays independent in other chats', async ({ page, context, request }) => {
  await page.goto('/?companion=1');
  await selectSession(page, request, 'updates-chat-a');
  await request.post('/api/events', { data: report('updates-chat-a', '@workflow', 'Review update settings', 'Each workspace chooses how much future work to record.') });
  await expect(page.getByTestId('workflow-step')).toHaveCount(1);
  const second = await context.newPage();
  await second.goto('/?companion=1&session_id=updates-chat-a');
  await expect(second.getByTestId('workflow-step')).toHaveCount(1);
  await pickUpdates(page, 'milestones');
  await expect(second.locator('.stef-app')).toHaveAttribute('data-update-mode', 'milestones');
  expect(await (await request.get('/api/settings', { params: { session_id: 'updates-chat-a' } })).json()).toEqual({ update_mode: 'milestones' });
  await second.reload();
  await expect(second.locator('.stef-app')).toHaveAttribute('data-update-mode', 'milestones');
  await page.getByRole('button', { name: /appearance/i }).click();
  await page.getByRole('menuitemradio', { name: 'Light', exact: true }).click();
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'light');
  await selectSession(page, request, 'updates-chat-b');
  await expect(page.getByText('Listening…')).toBeVisible();
  await expect(page.locator('.stef-app')).toHaveAttribute('data-update-mode', 'files');
  await expect(second.locator('.stef-app')).toHaveAttribute('data-update-mode', 'milestones');
  await request.post('/api/events', { data: report('updates-chat-b', '@workflow', 'Start a separate chat', 'Each chat keeps its own workflow history and update setting.') });
  await expect(page.getByTestId('workflow-step')).toHaveCount(1);
  await page.getByRole('button', { name: 'Clear this chat’s workflow' }).click();
  await expect(page.getByText('Listening…')).toBeVisible();
  await selectSession(page, request, 'updates-chat-a');
  await page.getByRole('button', { name: 'Show saved history', exact: true }).click();
  await expect(page.getByRole('button', { name: 'Review update settings', exact: true })).toHaveAttribute('aria-pressed', 'true');
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'light');
  await pickUpdates(second, 'disabled');
  await expect(page.locator('.stef-app')).toHaveAttribute('data-update-mode', 'disabled');
  await pickUpdates(page, 'files');
  await expect(second.locator('.stef-app')).toHaveAttribute('data-update-mode', 'files');
  await expect(second.getByTestId('workflow-step')).toHaveCount(0);
  await second.getByRole('button', { name: 'Show saved history', exact: true }).click();
  await expect(second.getByRole('button', { name: 'Review update settings', exact: true })).toBeVisible();
  await second.close();
});

test('the compact Updates menu explains token cost and handles keyboard, narrow layouts and save failures', async ({ page, request }) => {
  const errors: string[] = []; page.on('pageerror', error => errors.push(error.message));
  await page.goto('/?companion=1');
  await selectSession(page, request, 'updates-menu');
  await request.post('/api/events', { data: report('updates-menu', '@workflow', 'Keep controls simple', 'Update frequency should take one small menu rather than a dashboard.') });
  const selected = page.getByRole('button', { name: 'Keep controls simple', exact: true });
  await chooseStep(selected);
  const trigger = page.getByRole('button', { name: 'Updates', exact: true });
  const menu = page.getByRole('menu', { name: 'Updates', exact: true });
  await trigger.click();
  await expect(menu.getByRole('menuitemradio')).toHaveCount(4);
  await expect(menu.getByText('New key steps only, with brief explanations.', { exact: true })).toBeVisible();
  await expect(menu.getByText('New saves from now on. Per-file explanations may use more tokens.', { exact: true })).toBeVisible();
  await expect(menu.getByText('Stop new updates. Keep saved history.', { exact: true })).toBeVisible();
  const files = menu.getByRole('menuitemradio', { name: /^Every file/ });
  await expect(files).toBeFocused();
  await expect(files).toHaveAttribute('aria-checked', 'true');
  await page.keyboard.press('Home');
  await expect(menu.getByRole('menuitemradio', { name: /^Major milestones/ })).toBeFocused();
  await page.keyboard.press('End');
  await expect(menu.getByRole('menuitemradio', { name: /^Disabled/ })).toBeFocused();
  await page.keyboard.press('ArrowUp');
  await expect(menu.getByRole('menuitemradio', { name: /^All activity/ })).toBeFocused();
  await page.keyboard.press('ArrowUp');
  await expect(files).toBeFocused();
  await page.keyboard.press('Escape');
  await expect(menu).toHaveCount(0);
  await expect(trigger).toBeFocused();
  await expect(selected).toHaveAttribute('aria-pressed', 'true');
  await trigger.click();
  await page.getByRole('main', { name: 'Workflow' }).click({ position: { x: 5, y: 5 } });
  await expect(menu).toHaveCount(0);

  await page.setViewportSize({ width: 320, height: 640 });
  await trigger.click();
  const bounds = await menu.boundingBox();
  expect(bounds).not.toBeNull();
  expect(bounds!.x).toBeGreaterThanOrEqual(0);
  expect(bounds!.x + bounds!.width).toBeLessThanOrEqual(320);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
  await page.screenshot({ path: '../artifacts/stef-updates-menu.png', fullPage: true });

  await page.route('**/api/settings*', async route => {
    if (route.request().method() === 'POST') await route.fulfill({ status: 500, body: 'Setting could not be saved.' });
    else await route.continue();
  });
  await menu.getByRole('menuitemradio', { name: /^Major milestones/ }).click();
  await expect(page.getByRole('status')).toContainText('Could not change updates.');
  await expect(page.locator('.stef-app')).toHaveAttribute('data-update-mode', 'files');
  await expect(files).toHaveAttribute('aria-checked', 'true');
  await expect(menu.getByRole('menuitemradio', { name: /^Major milestones/ })).toBeEnabled();
  expect(await (await request.get('/api/settings')).json()).toEqual({ update_mode: 'files' });
  await page.unroute('**/api/settings*');
  await menu.getByRole('menuitemradio', { name: /^Major milestones/ }).click();
  await expect(menu).toHaveCount(0);
  await expect(page.locator('.stef-app')).toHaveAttribute('data-update-mode', 'milestones');
  await expect(page.getByRole('status')).toHaveCount(0);
  expect(errors).toEqual([]);
});

test('Design is local, keeps selection and suggestion drafts, and survives reload and chat changes', async ({ page, request }) => {
  const errors: string[] = []; page.on('pageerror', error => errors.push(error.message));
  const mutations: string[] = [];
  page.on('request', outgoing => {
    const path = new URL(outgoing.url()).pathname;
    if (outgoing.method() !== 'GET' && ['/api/settings', '/api/events', '/api/changes', '/api/feedback'].includes(path)) mutations.push(path);
  });
  await page.goto('/?companion=1');
  await expect(page.locator('.stef-app')).toHaveAttribute('data-design', 'minimal');
  const session = 'design-chat-a';
  await selectSession(page, request, session);
  await request.post('/api/events', { data: { ...report(session, 'entry.ts', 'Explore the entry', 'Find where this task begins.'), action: 'scouted' } });
  await request.post('/api/events', { data: report(session, 'design.ts', 'Connect the local design', 'Render the same saved work in a richer local view.', ['entry.ts']) });
  const selected = page.getByRole('button', { name: 'Connect the local design', exact: true });
  await chooseStep(selected);
  await page.getByRole('button', { name: 'Suggest', exact: true }).click();
  const input = page.getByRole('textbox', { name: 'Suggest something to the agent' });
  await input.fill('Keep the saved reason intact.');

  const design = page.getByRole('button', { name: 'Design', exact: true });
  const menu = page.getByRole('menu', { name: 'Design', exact: true });
  await design.click();
  await expect(menu.getByRole('menuitemradio')).toHaveCount(2);
  await expect(menu.getByRole('menuitemradio', { name: 'Minimal', exact: true })).toHaveAttribute('aria-checked', 'true');
  await expect(menu.getByRole('menuitemradio', { name: 'Minimal', exact: true })).toBeFocused();
  await page.keyboard.press('End');
  await expect(menu.getByRole('menuitemradio', { name: 'Interactive', exact: true })).toBeFocused();
  await page.keyboard.press('Home');
  await expect(menu.getByRole('menuitemradio', { name: 'Minimal', exact: true })).toBeFocused();
  await page.keyboard.press('Escape');
  await expect(menu).toHaveCount(0);
  await expect(design).toBeFocused();

  await pickDesign(page, 'interactive');
  await expect(selected).toHaveAttribute('aria-pressed', 'true');
  await expect(page.getByRole('region', { name: 'Selected change' })).toContainText('Render the same saved work in a richer local view.');
  await expect(page.getByTestId('workflow-step')).toHaveCount(2);
  await expect(page.getByTestId('workflow-arrow')).toHaveCount(1);
  await expect.poll(() => page.evaluate(() => localStorage.getItem('stef-design'))).toBe('interactive');
  expect(await (await request.get('/api/settings')).json()).toEqual({ update_mode: 'files' });
  await pickDesign(page, 'minimal');
  await expect(input).toHaveValue('Keep the saved reason intact.');
  await expect(selected).toHaveAttribute('aria-pressed', 'true');
  await expect(page.getByRole('region', { name: 'Selected change' })).toHaveCount(0);
  await pickDesign(page, 'interactive');
  await page.getByRole('button', { name: /appearance/i }).click();
  await page.getByRole('menuitemradio', { name: 'Eye comfort', exact: true }).click();
  await page.reload();
  await expect(page.locator('.stef-app')).toHaveAttribute('data-design', 'interactive');
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'comfort');
  await expect(selected).toHaveAttribute('aria-pressed', 'true');
  await expect(page.getByRole('region', { name: 'Selected change' })).toContainText('Render the same saved work in a richer local view.');
  await selectSession(page, request, 'design-chat-b');
  await expect(page.getByTestId('workflow-step')).toHaveCount(0);
  await expect(page.locator('.stef-app')).toHaveAttribute('data-design', 'interactive');
  await expect(page.getByRole('region', { name: 'Selected change' })).toHaveCount(0);
  await selectSession(page, request, session);
  await expect(selected).toHaveAttribute('aria-pressed', 'true');
  const panel = page.getByRole('region', { name: 'Selected change' });
  await expect(panel).toContainText('Render the same saved work in a richer local view.');
  await expect(panel).toContainText('Reported path');
  await panel.getByRole('button', { name: 'entry.ts', exact: true }).click();
  await expect(page.getByRole('button', { name: 'Explore the entry', exact: true })).toHaveAttribute('aria-pressed', 'true');
  await expect(panel).toContainText('Explore');
  await expect(panel).toContainText('Find where this task begins.');
  await page.getByRole('button', { name: 'Clear this chat’s workflow' }).click();
  await expect(page.getByTestId('workflow-step')).toHaveCount(0);
  await expect(page.locator('.stef-app')).toHaveAttribute('data-design', 'interactive');
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'comfort');
  expect(mutations).toEqual([]);
  expect(errors).toEqual([]);
});

test('Interactive shows exact local revisions and requests more agent context only from Explain more', async ({ page, request }) => {
  const errors: string[] = []; page.on('pageerror', error => errors.push(error.message));
  const feedbackRequests: unknown[] = [];
  page.on('request', outgoing => { if (new URL(outgoing.url()).pathname === '/api/feedback' && outgoing.method() === 'POST') feedbackRequests.push(outgoing.postDataJSON()); });
  await page.goto('/?companion=1');
  const session = 'interactive-exact-revision';
  await selectSession(page, request, session);
  const root = await isolatedRoot(request);
  const file = resolve(root, 'interactive-source.ts');
  const fileSteps = page.locator('[data-testid="workflow-step"][title^="interactive-source.ts"]');
  const before = 'export const mode = "before";';
  const after = `export const markup = '<img src=x onerror="window.stefVisualInjected=true">';`;
  const baseline = [before, ...Array.from({ length: 38 }, (_, index) => '// context line ' + (index + 2)), 'export const tail = "before";'];
  const replacement = [after, ...baseline.slice(1, -1), 'export const tail = "after";'];
  await writeFile(file, baseline.join('\n') + '\n');
  await expect(fileSteps).toHaveCount(1);
  await writeFile(file, replacement.join('\n') + '\n');
  await expect(fileSteps).toHaveCount(2);
  const revision = (await savedEvents(request, session)).filter(event => event.source === 'watcher' && event.node_name === 'interactive-source.ts').at(-1)!;
  const reason = 'Keep this exact revision visible so a developer can compare the previous value with the new escaped source without reading unrelated changes or asking another model to reconstruct what happened.';
  await request.post('/api/events', { data: { ...report(session, revision.node_name, 'Keep exact source visible', reason), action: revision.action, change_id: revision.id } });
  await request.post('/api/events', { data: { ...report(session, 'unrelated-agent.ts', 'Separate agent work', 'This later reporter does not own the selected revision.'), agent_id: 'unrelated-agent' } });
  await chooseStep(fileSteps.last());
  await pickDesign(page, 'interactive');
  const panel = page.getByRole('region', { name: 'Selected change', exact: true });
  await expect(panel).toBeVisible();
  await expect(panel).toContainText(reason);
  await expect(panel).toContainText(before);
  await expect(panel).toContainText(after);
  await expect(panel.getByText('Hunk 1 of 2', { exact: true })).toBeVisible();
  await expect(panel.getByRole('button', { name: 'Previous hunk', exact: true })).toBeDisabled();
  await panel.getByRole('button', { name: 'Next hunk', exact: true }).click();
  await expect(panel.getByText('Hunk 2 of 2', { exact: true })).toBeVisible();
  await expect(panel.locator('[data-kind="removal"] .diff-code')).toContainText(['export const tail = "before";']);
  await expect(panel.locator('[data-kind="addition"] .diff-code')).toContainText(['export const tail = "after";']);
  await expect(panel.getByRole('button', { name: 'Next hunk', exact: true })).toBeDisabled();
  await panel.getByRole('button', { name: 'Previous hunk', exact: true }).focus();
  await page.keyboard.press('Enter');
  await expect(panel.getByText('Hunk 1 of 2', { exact: true })).toBeVisible();
  await expect(panel.locator('img')).toHaveCount(0);
  expect(await page.evaluate(() => (window as typeof window & { stefVisualInjected?: boolean }).stefVisualInjected)).toBeUndefined();
  expect(feedbackRequests).toEqual([]);
  expect((await checkIn(request, session)).feedback ?? []).toEqual([]);
  await panel.getByRole('button', { name: 'Show diff', exact: true }).click();
  const dialog = page.getByRole('dialog', { name: 'File diff', exact: true });
  await expect(dialog).toContainText(before);
  await expect(dialog).toContainText(after);
  await dialog.getByRole('button', { name: 'Close diff', exact: true }).click();
  expect(feedbackRequests).toEqual([]);

  const queued = page.waitForResponse(response => new URL(response.url()).pathname === '/api/feedback' && response.request().method() === 'POST');
  await panel.getByRole('button', { name: 'Explain more', exact: true }).click();
  expect((await queued).ok()).toBe(true);
  await expect(panel).toContainText(/next check-in/);
  expect(feedbackRequests).toHaveLength(1);
  expect((feedbackRequests[0] as { target_agent: string }).target_agent).toBe('test-agent');
  const unrelatedState = await (await request.post('/api/check-in', { data: { session_id: session, agent_id: 'unrelated-agent' } })).json();
  expect(unrelatedState.feedback ?? []).toEqual([]);
  const state = await checkIn(request, session);
  expect(state.feedback).toHaveLength(1);
  expect(state.feedback[0].node_name).toBe(revision.node_name);
  expect(state.feedback[0].text).toContain(revision.id);
  await page.getByRole('button', { name: 'Follow latest', exact: true }).click();
  await writeFile(file, 'export const mode = "later unrelated revision";\n');
  await expect(fileSteps).toHaveCount(3);
  await expect(fileSteps.last()).toHaveAttribute('aria-pressed', 'true');
  const expanded = reason + ' The new markup stays literal text in the preview, and the older value remains the removal for this saved revision.';
  await request.post('/api/events', { data: { ...report(session, revision.node_name, 'Keep exact source visible', expanded), action: revision.action, context_id: revision.id } });
  await expect(fileSteps.last()).toHaveAttribute('aria-pressed', 'true');
  await chooseStep(fileSteps.nth(1));
  await expect(panel).toContainText(expanded);
  await expect(panel).not.toContainText('later unrelated revision');
  expect(feedbackRequests).toHaveLength(1);

  await page.emulateMedia({ reducedMotion: 'reduce' });
  for (const [label, theme] of [['Light', 'light'], ['Eye comfort', 'comfort']]) {
    await page.getByRole('button', { name: /appearance/i }).click();
    await page.getByRole('menuitemradio', { name: label, exact: true }).click();
    await expect(page.locator('html')).toHaveAttribute('data-theme', theme);
    await page.setViewportSize({ width: 320, height: 640 });
    await expect(panel).toContainText(expanded);
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
    const bounds = await panel.boundingBox();
    expect(bounds).not.toBeNull();
    expect(bounds!.x).toBeGreaterThanOrEqual(0);
    expect(bounds!.x + bounds!.width).toBeLessThanOrEqual(320);
    await page.screenshot({ path: '../artifacts/stef-interactive-' + theme + '.png', fullPage: true });
  }
  expect(errors).toEqual([]);
});


for (const width of [860, 360]) {
  test('Interactive contains wheel scrolling in each pane at ' + width + 'px', async ({ page, request }) => {
    const errors: string[] = []; page.on('pageerror', error => errors.push(error.message));
    const feedbackRequests: unknown[] = [];
    page.on('request', outgoing => { if (new URL(outgoing.url()).pathname === '/api/feedback' && outgoing.method() === 'POST') feedbackRequests.push(outgoing.postDataJSON()); });
    await page.setViewportSize({ width, height: width === 360 ? 740 : 640 });
    await page.emulateMedia({ reducedMotion: 'reduce' });
    await page.goto('/?companion=1');
    const session = 'interactive-pane-scroll-' + width;
    await selectSession(page, request, session);
    await pickDesign(page, 'interactive');
    const root = await isolatedRoot(request);
    await Promise.all(Array.from({ length: 24 }, (_, index) => writeFile(resolve(root, 'scroll-' + width + '-' + index + '.ts'), 'export const filler = true;\n')));
    await expect(page.getByTestId('workflow-step')).toHaveCount(24);
    const name = 'scroll-long-' + width + '.ts';
    await writeFile(resolve(root, name), Array.from({ length: 90 }, (_, index) => 'export const line' + index + ' = ' + index + ';').join('\n') + '\n');
    await expect(page.getByTestId('workflow-step')).toHaveCount(25);
    const fileStep = page.locator('[data-testid="workflow-step"][title^="' + name + '"]');
    const revision = (await savedEvents(request, session)).find(event => event.source === 'watcher' && event.node_name === name)!;
    await request.post('/api/events', { data: { ...report(session, name, 'Keep this save inspectable', 'Keep exact saved source available to inspect.'), action: revision.action, change_id: revision.id } });
    const left = page.locator('.workflow-canvas');
    const right = page.locator('.interactive-detail');
    const code = right.getByRole('region', { name: 'Code diff', exact: true });
    await expect.poll(() => left.evaluate(node => node.scrollHeight - node.clientHeight)).toBeGreaterThan(400);
    await expect.poll(() => right.evaluate(node => node.scrollHeight - node.clientHeight)).toBeGreaterThan(400);
    expect(await code.evaluate(node => node.scrollHeight > node.clientHeight + 1)).toBe(false);
    await left.evaluate(node => node.scrollTop = 100);
    await right.evaluate(node => node.scrollTop = 100);
    await wheelOver(page, right, 180);
    await expect.poll(() => right.evaluate(node => node.scrollTop)).toBeGreaterThan(100);
    expect(await left.evaluate(node => node.scrollTop)).toBe(100);
    const afterRight = await right.evaluate(node => node.scrollTop);
    await wheelOver(page, left, 180);
    await expect.poll(() => left.evaluate(node => node.scrollTop)).toBeGreaterThan(100);
    expect(await right.evaluate(node => node.scrollTop)).toBe(afterRight);

    for (const side of [right, left]) {
      const other = side === right ? left : right;
      for (const direction of [-1, 1]) {
        await side.evaluate((node, end) => node.scrollTop = end ? node.scrollHeight : 0, direction === 1);
        const boundary = await side.evaluate(node => node.scrollTop);
        const otherTop = await other.evaluate(node => node.scrollTop);
        await wheelOver(page, side, direction * 500);
        await expect.poll(() => side.evaluate(node => node.scrollTop)).toBe(boundary);
        expect(await other.evaluate(node => node.scrollTop)).toBe(otherTop);
        expect(await page.evaluate(() => window.scrollY)).toBe(0);
      }
    }
    await right.evaluate(node => node.scrollTop = 150);
    await request.post('/api/events', { data: { ...report(session, name, 'Keep this save inspectable', 'Keep this saved source available to inspect.'), action: revision.action, context_id: revision.id } });
    await expect(right).toContainText('Keep this saved source available to inspect.');
    expect(await right.evaluate(node => node.scrollTop)).toBe(150);
    await chooseStep(page.getByTestId('workflow-step').first());
    await expect.poll(() => right.evaluate(node => node.scrollTop)).toBe(0);
    await chooseStep(fileStep);
    await expect.poll(() => right.evaluate(node => node.scrollTop)).toBe(0);
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth && document.documentElement.scrollHeight <= innerHeight + 1)).toBe(true);
    expect(feedbackRequests).toEqual([]);
    expect((await checkIn(request, session)).feedback ?? []).toEqual([]);
    expect(errors).toEqual([]);
  });
}

test('Interactive keyboard and touch scrolling stay in the selected pane', async ({ page, request }) => {
  await page.setViewportSize({ width: 360, height: 740 });
  await page.emulateMedia({ reducedMotion: 'reduce' });
  await page.goto('/?companion=1');
  const session = 'interactive-pane-input';
  await selectSession(page, request, session);
  await pickDesign(page, 'interactive');
  const root = await isolatedRoot(request);
  await Promise.all(Array.from({ length: 20 }, (_, index) => writeFile(resolve(root, 'input-scroll-' + index + '.ts'), 'export const filler = true;\n')));
  await expect(page.getByTestId('workflow-step')).toHaveCount(20);
  await writeFile(resolve(root, 'input-scroll-long.ts'), Array.from({ length: 90 }, (_, index) => 'export const line' + index + ' = ' + index + ';').join('\n') + '\n');
  await expect(page.getByTestId('workflow-step')).toHaveCount(21);
  const left = page.locator('.workflow-canvas');
  const right = page.locator('.interactive-detail');
  await left.evaluate(node => node.scrollTop = 100);
  await right.evaluate(node => node.scrollTop = 0);
  await right.focus();
  await page.keyboard.press('PageDown');
  await expect.poll(() => right.evaluate(node => node.scrollTop)).toBeGreaterThan(0);
  expect(await left.evaluate(node => node.scrollTop)).toBe(100);
  // Native PageDown can keep animating after the first observed movement.
  const settle = async (pane: Locator) => pane.evaluate(node => new Promise<void>(resolve => {
    let previous = node.scrollTop, still = 0;
    const frame = () => {
      const current = node.scrollTop;
      still = current === previous ? still + 1 : 0;
      previous = current;
      if (still >= 6) resolve(); else requestAnimationFrame(frame);
    };
    requestAnimationFrame(frame);
  }));
  await settle(right);
  const rightTop = await right.evaluate(node => node.scrollTop);
  await left.focus();
  await page.keyboard.press('PageDown');
  await expect.poll(() => left.evaluate(node => node.scrollTop)).toBeGreaterThan(100);
  await settle(left);
  expect(await right.evaluate(node => node.scrollTop)).toBe(rightTop);
  await right.evaluate(node => node.scrollTop = 100);
  const leftTop = await left.evaluate(node => node.scrollTop);
  const touch = await page.context().newCDPSession(page);
  await touch.send('Emulation.setTouchEmulationEnabled', { enabled: true });
  const box = await right.boundingBox();
  expect(box).not.toBeNull();
  const x = box!.x + box!.width / 2;
  const start = box!.y + box!.height * .8;
  await touch.send('Input.dispatchTouchEvent', { type: 'touchStart', touchPoints: [{ x, y: start }] });
  for (let amount = 1; amount <= 6; amount++) {
    await touch.send('Input.dispatchTouchEvent', { type: 'touchMove', touchPoints: [{ x, y: start - amount * 22 }] });
  }
  await touch.send('Input.dispatchTouchEvent', { type: 'touchEnd', touchPoints: [] });
  await expect.poll(() => right.evaluate(node => node.scrollTop)).toBeGreaterThan(100);
  expect(await left.evaluate(node => node.scrollTop)).toBe(leftTop);
  expect(await page.evaluate(() => window.scrollY)).toBe(0);
  await touch.detach();
});

test('Interactive change exploration stays local and drafts an exact line suggestion', async ({ page, request }) => {
  const errors: string[] = []; page.on('pageerror', error => errors.push(error.message));
  const feedbackRequests: unknown[] = [];
  page.on('request', outgoing => { if (new URL(outgoing.url()).pathname === '/api/feedback' && outgoing.method() === 'POST') feedbackRequests.push(outgoing.postDataJSON()); });
  await page.goto('/?companion=1');
  const session = 'interactive-explore-change';
  await selectSession(page, request, session);
  await pickDesign(page, 'interactive');
  const root = await isolatedRoot(request);
  const name = 'explore-line.ts';
  const file = resolve(root, name);
  const before = ['export const head = "old";', ...Array.from({ length: 38 }, (_, index) => '// context ' + (index + 2)), 'export const tail = "old";'];
  const after = ['export const head = "new";', ...before.slice(1, -1), 'export const tail = "new";'];
  await writeFile(file, before.join('\n') + '\n');
  const steps = page.locator('[data-testid="workflow-step"][title^="' + name + '"]');
  await expect(steps).toHaveCount(1);
  await writeFile(file, after.join('\n') + '\n');
  await expect(steps).toHaveCount(2);
  const revision = (await savedEvents(request, session)).filter(event => event.source === 'watcher' && event.node_name === name).at(-1)!;
  const why = 'Keep the head and tail changes understandable in their exact saved revision.';
  await request.post('/api/events', { data: { ...report(session, name, 'Explain both changed sections', why), action: revision.action, change_id: revision.id } });
  const panel = page.getByRole('region', { name: 'Selected change', exact: true });
  const map = panel.getByRole('region', { name: 'Change map', exact: true });
  await expect(map.getByTestId('hunk-map')).toHaveCount(2);
  const second = map.getByRole('button', { name: /^Explore hunk 2:/ });
  await second.focus();
  await page.keyboard.press('Enter');
  await expect(panel.getByText('Hunk 2 of 2', { exact: true })).toBeVisible();
  await expect(panel.locator('[data-kind="addition"] .diff-code')).toContainText(['export const tail = "new";']);
  await panel.getByText('Filter source', { exact: true }).click();
  await panel.getByRole('button', { name: 'Show added lines', exact: true }).click();
  await expect(panel.locator('[data-kind="removal"], [data-kind="context"]')).toHaveCount(0);
  await expect(panel.locator('[data-kind="addition"]')).toHaveCount(1);
  const line = panel.getByRole('button', { name: 'Select added line 40', exact: true });
  await line.focus();
  await page.keyboard.press('Enter');
  await expect(line).toHaveAttribute('aria-pressed', 'true');
  await expect(panel.getByRole('button', { name: 'Suggest about this line', exact: true })).toBeVisible();
  await writeFile(resolve(root, 'explore-arriving.ts'), 'export const arriving = true;\n');
  await expect(page.getByTestId('workflow-step')).toHaveCount(3);
  await expect(steps.last()).toHaveAttribute('aria-pressed', 'true');
  await expect(page.getByRole('button', { name: 'Follow latest', exact: true })).toBeEnabled();
  await panel.getByRole('button', { name: 'Suggest about this line', exact: true }).click();
  const draft = page.getByRole('textbox', { name: 'Suggest something to the agent' });
  await expect(draft).toBeFocused();
  const text = await draft.inputValue();
  expect(text).toContain(name);
  expect(text).toContain(revision.id);
  expect(text).toContain('40');
  expect(text).toContain('export const tail = "new";');
  expect(text.length).toBeLessThanOrEqual(500);
  await expect(page.getByRole('button', { name: 'Follow latest', exact: true })).toBeDisabled();
  expect(feedbackRequests).toEqual([]);
  expect((await checkIn(request, session)).feedback ?? []).toEqual([]);
  await page.keyboard.press('Escape');
  await expect(draft).toHaveCount(0);
  await page.getByRole('button', { name: 'Follow latest', exact: true }).click();
  await expectLatestVisible(page);
  expect(feedbackRequests).toEqual([]);
  expect(errors).toEqual([]);
});

test('style samples render exact small dimensions and real layout gaps', async ({ page, request }) => {
  await page.emulateMedia({ reducedMotion: 'reduce' });
  await page.goto('/?companion=1');
  const session = 'interactive-small-style-effect';
  await selectSession(page, request, session);
  await pickDesign(page, 'interactive');
  const root = await isolatedRoot(request);
  const file = resolve(root, 'small-style-effect.css');
  await writeFile(file, '.sample { width: 4px; height: 4px; gap: 4px; }\n');
  await expect(page.getByTestId('workflow-step')).toHaveCount(1);
  await writeFile(file, '.sample { width: 12px; height: 12px; gap: 12px; }\n');
  await expect(page.getByTestId('workflow-step')).toHaveCount(2);
  const effect = page.getByRole('region', { name: 'Style effect preview', exact: true });
  const property = effect.getByRole('combobox', { name: 'Preview style property', exact: true });
  const target = effect.getByTestId('style-effect-target');
  for (const name of ['width', 'height', 'gap']) {
    await property.selectOption({ label: name });
    for (const [label, size] of [['Before style', 4], ['After style', 12]] as const) {
      await effect.getByRole('button', { name: label, exact: true }).click();
      await expect.poll(() => target.evaluate((node, dimension) => {
        if (dimension !== 'gap') return node.getBoundingClientRect()[dimension as 'width' | 'height'];
        const children = node.querySelectorAll('span');
        return children[1].getBoundingClientRect().left - children[0].getBoundingClientRect().right;
      }, name)).toBe(size);
    }
  }
  expect((await checkIn(request, session)).feedback ?? []).toEqual([]);
});

test('Interactive previews a safe exact style effect with working comparison controls', async ({ page, request }) => {
  const errors: string[] = []; page.on('pageerror', error => errors.push(error.message));
  const feedbackRequests: unknown[] = [], externalRequests: string[] = [];
  page.on('request', outgoing => {
    const url = new URL(outgoing.url());
    if (url.pathname === '/api/feedback' && outgoing.method() === 'POST') feedbackRequests.push(outgoing.postDataJSON());
    if (url.hostname !== '127.0.0.1') externalRequests.push(outgoing.url());
  });
  await page.setViewportSize({ width: 860, height: 640 });
  await page.emulateMedia({ reducedMotion: 'reduce' });
  await page.goto('/?companion=1');
  const session = 'interactive-style-effect';
  await selectSession(page, request, session);
  await pickDesign(page, 'interactive');
  const root = await isolatedRoot(request);
  const name = 'style-effect.css';
  const file = resolve(root, name);
  await writeFile(file, '.sample { color: #000000; padding: 8px; border-radius: 4px; }\n');
  const steps = page.locator('[data-testid="workflow-step"][title^="' + name + '"]');
  await expect(steps).toHaveCount(1);
  await writeFile(file, '.sample { color: #ffffff; padding: 24px; border-radius: 16px; }\n');
  await expect(steps).toHaveCount(2);
  const panel = page.getByRole('region', { name: 'Selected change', exact: true });
  const effect = panel.getByRole('region', { name: 'Style effect preview', exact: true });
  await expect(effect).toContainText('Style sample');
  await expect(effect).toContainText('This sample previews the changed style, not the running app.');
  const property = effect.getByRole('combobox', { name: 'Preview style property', exact: true });
  const options = await property.locator('option').evaluateAll(elements => elements.map(element => ({ label: element.textContent ?? '', value: (element as HTMLOptionElement).value })));
  const padding = options.find(option => option.label.includes('padding'));
  expect(padding).toBeTruthy();
  await property.selectOption(padding!.value);
  const target = effect.getByTestId('style-effect-target');
  await effect.getByRole('button', { name: 'Before style', exact: true }).click();
  await expect.poll(() => target.evaluate(node => getComputedStyle(node).paddingLeft)).toBe('8px');
  await effect.getByRole('button', { name: 'After style', exact: true }).click();
  await expect.poll(() => target.evaluate(node => getComputedStyle(node).paddingLeft)).toBe('24px');
  const position = effect.getByRole('slider', { name: 'Style effect position', exact: true });
  await position.focus();
  await page.keyboard.press('Home');
  await expect(position).toHaveValue('0');
  await page.keyboard.press('End');
  await expect(position).toHaveValue('100');
  for (let count = 0; count < 50; count++) await page.keyboard.press('ArrowLeft');
  await expect(position).toHaveValue('50');
  await expect.poll(() => target.evaluate(node => parseFloat(getComputedStyle(node).paddingLeft))).toBe(16);
  await page.setViewportSize({ width: 360, height: 740 });
  expect(await effect.evaluate(node => node.scrollWidth <= node.clientWidth + 1)).toBe(true);
  await expect(page.getByRole('button', { name: 'Follow latest', exact: true })).toBeEnabled();
  expect(feedbackRequests).toEqual([]);
  expect(externalRequests).toEqual([]);
  await page.getByRole('button', { name: 'Follow latest', exact: true }).click();
  await writeFile(resolve(root, 'effect-unsafe.css'), '.sample { padding: 8px; }\n');
  await expect(page.getByTestId('workflow-step')).toHaveCount(3);
  await writeFile(resolve(root, 'effect-unsafe.css'), '.sample { padding: url(https://stef-preview-invalid.example/leak); }\n');
  await expect(page.getByTestId('workflow-step')).toHaveCount(4);
  await expect(panel.getByTestId('style-effect')).toHaveCount(0);
  await writeFile(resolve(root, 'effect-logic.ts'), 'export const enabled = false;\n');
  await expect(page.getByTestId('workflow-step')).toHaveCount(5);
  await expect(panel.getByTestId('style-effect')).toHaveCount(0);
  expect(externalRequests).toEqual([]);
  expect(feedbackRequests).toEqual([]);
  expect((await checkIn(request, session)).feedback ?? []).toEqual([]);
  expect(errors).toEqual([]);
});

for (const design of ['minimal', 'interactive'] as const) {
  test(design + ' follows rapid saved-file changes and preserves history across reload and reconnect', async ({ page, request }) => {
    const errors: string[] = []; page.on('pageerror', error => errors.push(error.message));
    const feedbackRequests: unknown[] = [];
    page.on('request', outgoing => { if (new URL(outgoing.url()).pathname === '/api/feedback' && outgoing.method() === 'POST') feedbackRequests.push(outgoing.postDataJSON()); });
    await captureSockets(page);
    await page.setViewportSize({ width: 860, height: 640 });
    await page.goto('/?companion=1');
    const session = 'follow-live-' + design;
    await selectSession(page, request, session);
    await pickDesign(page, design);
    const root = await isolatedRoot(request);
    const firstName = 'follow-' + design + '-first.ts';
    await writeFile(resolve(root, firstName), 'export const first = true;\n');
    await expect(page.getByTestId('workflow-step')).toHaveCount(1);
    const first = (await savedEvents(request, session)).find(event => event.source === 'watcher' && event.kind === 'file' && event.node_name === firstName)!;
    await Promise.all(Array.from({ length: 16 }, (_, index) => writeFile(resolve(root, 'follow-' + design + '-' + index + '.ts'), 'export const value = ' + index + ';\n')));
    await expect(page.getByTestId('workflow-step')).toHaveCount(17);
    await expectLatestVisible(page);
    const latest = (await savedEvents(request, session)).filter(event => event.source === 'watcher' && event.kind === 'file').at(-1)!;
    const latestStep = page.getByTestId('workflow-step').last();
    const latestTitle = await latestStep.getAttribute('title');
    await request.post('/api/events', { data: { ...report(session, firstName, 'Explain the first save', 'Keep the starting value separate from the later changes.'), action: first.action, change_id: first.id } });
    await expect(latestStep).toHaveAttribute('title', latestTitle!);
    await expectLatestVisible(page);
    const why = 'Keep this newest value visible while the other files finish saving.';
    await request.post('/api/events', { data: { ...report(session, latest.node_name, 'Explain the latest save', why), action: latest.action, change_id: latest.id } });
    await expect(page.getByRole('region', { name: design === 'minimal' ? 'Why this step' : 'Selected change', exact: true })).toContainText(why);
    await expectLatestVisible(page);

    const firstStep = page.locator('[data-testid="workflow-step"][title^="' + firstName + '"]');
    await chooseStep(firstStep);
    await page.reload();
    await expect(page.getByTestId('workflow-step')).toHaveCount(17);
    await expect(firstStep).toHaveAttribute('aria-pressed', 'true');
    await reconnect(page);
    await expect(firstStep).toHaveAttribute('aria-pressed', 'true');
    // A reconnect must keep the user's inspection paused even when a new tail arrives.
    const socketCount = await page.evaluate(() => (window as Window & { stefTestSockets?: WebSocket[] }).stefTestSockets!.length);
    await page.evaluate(() => (window as Window & { stefTestSockets?: WebSocket[] }).stefTestSockets!.at(-1)!.close());
    await expect(page.getByRole('button', { name: 'Updates', exact: true })).toBeDisabled();
    const offlineName = 'follow-' + design + '-offline.ts';
    await writeFile(resolve(root, offlineName), 'export const reconnect = true;\n');
    await expect.poll(async () => (await savedEvents(request, session)).some(event => event.node_name === offlineName && event.kind === 'file')).toBe(true);
    await expect.poll(() => page.evaluate(() => (window as Window & { stefTestSockets?: WebSocket[] }).stefTestSockets!.length)).toBe(socketCount + 1);
    await expect(page.getByTestId('workflow-step')).toHaveCount(18);
    await expect(firstStep).toHaveAttribute('aria-pressed', 'true');
    await page.getByRole('button', { name: 'Follow latest', exact: true }).click();
    await expectLatestVisible(page);

    await page.setViewportSize({ width: 360, height: 640 });
    await writeFile(resolve(root, 'follow-' + design + '-narrow.ts'), 'export const narrow = true;\n');
    await expect(page.getByTestId('workflow-step')).toHaveCount(19);
    await expectLatestVisible(page);
    if (design === 'interactive') expect(await page.locator('.workflow-canvas').evaluate(element => element.scrollTop)).toBeGreaterThan(0);
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
    expect(feedbackRequests).toEqual([]);
    expect((await checkIn(request, session)).feedback ?? []).toEqual([]);
    expect(errors).toEqual([]);
  });

  test(design + ' starts following new work after an Updates boundary discards a hidden draft', async ({ page, request }) => {
    const errors: string[] = []; page.on('pageerror', error => errors.push(error.message));
    const feedbackRequests: unknown[] = [];
    page.on('request', outgoing => { if (new URL(outgoing.url()).pathname === '/api/feedback' && outgoing.method() === 'POST') feedbackRequests.push(outgoing.postDataJSON()); });
    await page.goto('/?companion=1');
    const session = 'follow-draft-boundary-' + design;
    await selectSession(page, request, session);
    await pickDesign(page, design);
    const root = await isolatedRoot(request);
    const file = resolve(root, 'draft-boundary-' + design + '.ts');
    await writeFile(file, 'export const window = "files";\n');
    await expect(page.getByTestId('workflow-step')).toHaveCount(1);
    await page.getByRole('button', { name: 'Suggest', exact: true }).click();
    const input = page.getByRole('textbox', { name: 'Suggest something to the agent' });
    await input.fill('A draft belonging to the previous reporting window.');
    await pickUpdates(page, 'milestones');
    await expect(page.getByTestId('workflow-step')).toHaveCount(0);
    await expect(input).toHaveCount(0);
    const why = 'Keep only the newly reported milestone visible in this recording window.';
    const milestone = await request.post('/api/events', { data: report(session, '@workflow', 'Follow the new milestone', why) });
    expect((await milestone.json()).accepted).toBe(true);
    await expect(page.getByTestId('workflow-step')).toHaveCount(1);
    await expectLatestVisible(page);
    await expect(page.getByRole('region', { name: design === 'minimal' ? 'Why this step' : 'Selected change', exact: true })).toContainText(why);
    await page.getByRole('button', { name: 'Suggest', exact: true }).click();
    await input.fill('Another unfinished draft before disabling updates.');
    await pickUpdates(page, 'disabled');
    await expect(page.getByText('Updates disabled', { exact: true })).toBeVisible();
    await expect(input).toHaveCount(0);
    await pickUpdates(page, 'files');
    await expect(page.getByTestId('workflow-step')).toHaveCount(0);
    await writeFile(file, 'export const window = "resumed";\n');
    await expect(page.getByTestId('workflow-step')).toHaveCount(1);
    await expectLatestVisible(page);
    await expect(input).toHaveCount(0);
    await page.getByRole('button', { name: 'Show saved history', exact: true }).click();
    await chooseStep(page.getByTestId('workflow-step').first());
    await page.getByRole('button', { name: 'Suggest', exact: true }).click();
    await input.fill('A draft on saved history that I will hide.');
    await page.getByRole('button', { name: 'Hide saved history', exact: true }).click();
    await expect(input).toHaveCount(0);
    await page.getByRole('button', { name: 'Follow latest', exact: true }).click();
    await writeFile(resolve(root, 'draft-hidden-history-' + design + '.ts'), 'export const current = true;\n');
    await expect(page.getByTestId('workflow-step')).toHaveCount(2);
    await expectLatestVisible(page);
    expect(feedbackRequests).toEqual([]);
    expect((await checkIn(request, session)).feedback ?? []).toEqual([]);
    expect(errors).toEqual([]);
  });

  test(design + ' defers following while a historical diff or suggestion is in use', async ({ page, request }) => {
    const errors: string[] = []; page.on('pageerror', error => errors.push(error.message));
    const feedbackRequests: unknown[] = [];
    page.on('request', outgoing => { if (new URL(outgoing.url()).pathname === '/api/feedback' && outgoing.method() === 'POST') feedbackRequests.push(outgoing.postDataJSON()); });
    await page.setViewportSize({ width: 860, height: 640 });
    await page.goto('/?companion=1');
    const session = 'follow-inspection-' + design;
    await selectSession(page, request, session);
    await pickDesign(page, design);
    const root = await isolatedRoot(request);
    const name = 'inspection-' + design + '.ts';
    const file = resolve(root, name);
    const fileSteps = page.locator('[data-testid="workflow-step"][title^="' + name + '"]');
    const original = 'export const revision = "original";';
    await writeFile(file, original + '\n');
    await expect(fileSteps).toHaveCount(1);
    const created = (await savedEvents(request, session)).find(event => event.source === 'watcher' && event.kind === 'file' && event.node_name === name)!;
    await Promise.all(Array.from({ length: 10 }, (_, index) => writeFile(resolve(root, 'inspection-' + design + '-filler-' + index + '.ts'), 'export const filler = true;\n')));
    await expect(page.getByTestId('workflow-step')).toHaveCount(11);
    await chooseStep(fileSteps.first());
    await page.getByRole('button', { name: 'Show diff', exact: true }).click();
    const dialog = page.getByRole('dialog', { name: 'File diff', exact: true });
    const close = dialog.getByRole('button', { name: 'Close diff', exact: true });
    await expect(close).toBeFocused();
    await writeFile(file, 'export const revision = "newer";\n');
    await expect(fileSteps).toHaveCount(2);
    await expect(dialog.locator('[data-kind="addition"] .diff-code')).toHaveText([original]);
    await expect(dialog.locator('[data-kind="removal"] .diff-code')).toHaveCount(0);
    await expect(fileSteps.first()).toHaveAttribute('aria-pressed', 'true');
    await expect(close).toBeFocused();
    await close.click();
    await expect(fileSteps.first()).toHaveAttribute('aria-pressed', 'true');
    await expect(page.getByRole('button', { name: 'Show diff', exact: true })).toBeFocused();
    await page.getByRole('button', { name: 'Follow latest', exact: true }).click();
    await expectLatestVisible(page);

    await chooseStep(fileSteps.first());
    await page.getByRole('button', { name: 'Suggest', exact: true }).click();
    const input = page.getByRole('textbox', { name: 'Suggest something to the agent' });
    await input.fill('Keep this original revision in mind.');
    await writeFile(resolve(root, 'inspection-' + design + '-draft.ts'), 'export const draft = true;\n');
    await expect(page.getByTestId('workflow-step')).toHaveCount(13);
    await expect(fileSteps.first()).toHaveAttribute('aria-pressed', 'true');
    await expect(input).toHaveValue('Keep this original revision in mind.');
    await expect(input).toBeFocused();
    await request.post('/api/events', { data: { ...report(session, name, 'Explain the original save', 'Keep a known starting value for this exact comparison.'), action: created.action, change_id: created.id } });
    await expect(input).toBeFocused();
    await expect(fileSteps.first()).toHaveAttribute('aria-pressed', 'true');
    await input.fill('');
    await expect(fileSteps.first()).toHaveAttribute('aria-pressed', 'true');
    await input.blur();
    await expect(fileSteps.first()).toHaveAttribute('aria-pressed', 'true');
    await page.getByRole('button', { name: 'Follow latest', exact: true }).click();
    await expectLatestVisible(page);
    await chooseStep(fileSteps.first());
    await page.getByRole('button', { name: 'Suggest', exact: true }).click();
    await input.fill('Discard this draft when I press Escape.');
    await writeFile(resolve(root, 'inspection-' + design + '-escape.ts'), 'export const escape = true;\n');
    await expect(page.getByTestId('workflow-step')).toHaveCount(14);
    await expect(fileSteps.first()).toHaveAttribute('aria-pressed', 'true');
    await expect(input).toBeFocused();
    await page.keyboard.press('Escape');
    await expect(input).toHaveCount(0);
    await page.getByRole('button', { name: 'Follow latest', exact: true }).click();
    await expectLatestVisible(page);
    expect(feedbackRequests).toEqual([]);
    expect((await checkIn(request, session)).feedback ?? []).toEqual([]);
    expect(errors).toEqual([]);
  });
}

test('Interactive wraps exact Git-style additions and removals without horizontal scrolling', async ({ page, request }) => {
  const errors: string[] = []; page.on('pageerror', error => errors.push(error.message));
  const feedbackRequests: unknown[] = [];
  page.on('request', outgoing => { if (new URL(outgoing.url()).pathname === '/api/feedback' && outgoing.method() === 'POST') feedbackRequests.push(outgoing.postDataJSON()); });
  await page.goto('/?companion=1');
  const session = 'interactive-unified-wrapping';
  await selectSession(page, request, session);
  const root = await isolatedRoot(request);
  const name = 'unified-wrapped.ts';
  const file = resolve(root, name);
  const fileSteps = page.locator('[data-testid="workflow-step"][title^="' + name + '"]');
  const before = '\t  export const markup = "before";  ';
  const markup = '\t  export const markup = \'<img src=x onerror="window.stefWrappedInjected=true">\';  ';
  const longLine = '\t  export const description = "' + 'Exact source characters stay visible. '.repeat(24) + '";  ';
  await writeFile(file, before);
  await expect(fileSteps).toHaveCount(1);
  await writeFile(file, markup + '\n' + longLine);
  await expect(fileSteps).toHaveCount(2);
  const revision = (await savedEvents(request, session)).filter(event => event.source === 'watcher' && event.kind === 'file' && event.node_name === name).at(-1)!;
  const reason = 'Wrap the saved source so the full change can be read in the companion without sideways scrolling.';
  await request.post('/api/events', { data: { ...report(session, name, 'Keep the full change readable', reason), action: revision.action, change_id: revision.id } });
  await pickDesign(page, 'interactive');
  const panel = page.getByRole('region', { name: 'Selected change', exact: true });
  const code = panel.getByRole('region', { name: 'Code diff', exact: true });
  const additions = code.locator('[data-kind="addition"]');
  const removals = code.locator('[data-kind="removal"]');
  for (const [width, label, theme] of [[860, 'Light', 'light'], [360, 'Eye comfort', 'comfort']] as const) {
    await page.setViewportSize({ width, height: 740 });
    await page.getByRole('button', { name: /appearance/i }).click();
    await page.getByRole('menuitemradio', { name: label, exact: true }).click();
    await expect(page.locator('html')).toHaveAttribute('data-theme', theme);
    await expect(panel).toContainText(reason);
    await expect(code).toBeVisible();
    await expect(removals.locator('.diff-code')).toHaveJSProperty('textContent', before);
    expect(await additions.locator('.diff-code').evaluateAll(elements => elements.map(element => element.textContent))).toEqual([markup, longLine]);
    await expect(removals.locator('.diff-prefix')).toHaveText('-');
    await expect(additions.locator('.diff-prefix')).toHaveText(['+', '+']);
    await expect(removals.locator('.line-number:nth-child(1)')).toHaveText('1');
    await expect(removals.locator('.line-number:nth-child(2)')).toHaveText('');
    await expect(additions.locator('.line-number:nth-child(1)')).toHaveText(['', '']);
    await expect(additions.locator('.line-number:nth-child(2)')).toHaveText(['1', '2']);
    await expect(code.locator('[data-kind="hunk"]')).toContainText('@@ -1 +1,2 @@');
    await expect(code.locator('[data-kind="meta"]')).toContainText(['\\ No newline at end of file']);
    await expect(panel.getByRole('region', { name: 'Before code', exact: true })).toHaveCount(0);
    await expect(panel.getByRole('region', { name: 'After code', exact: true })).toHaveCount(0);
    await expect(panel.getByRole('button', { name: 'Next hunk', exact: true })).toHaveCount(0);
    const sourceStyle = await additions.locator('.diff-code').last().evaluate(element => ({ height: element.getBoundingClientRect().height, lineHeight: parseFloat(getComputedStyle(element).lineHeight), tabSize: getComputedStyle(element).tabSize }));
    expect(sourceStyle.height).toBeGreaterThan(sourceStyle.lineHeight * 2);
    expect(sourceStyle.tabSize).toBe('4');
    expect(await code.evaluate(element => [...element.querySelectorAll('*'), element].filter(node => node instanceof HTMLElement && node.clientWidth > 0 && node.scrollWidth > node.clientWidth + 1).map(node => node.className))).toEqual([]);
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
    await expect(panel.locator('img')).toHaveCount(0);
    expect(await page.evaluate(() => (window as typeof window & { stefWrappedInjected?: boolean }).stefWrappedInjected)).toBeUndefined();
    const colors = await code.evaluate(element => ['addition', 'removal'].map(kind => {
      const row = element.querySelector('[data-kind="' + kind + '"]')!;
      const style = getComputedStyle(row);
      return { foreground: style.color, background: style.backgroundColor };
    }));
    expect(colors[0]).not.toEqual(colors[1]);
    await panel.getByRole('button', { name: 'Show diff', exact: true }).click();
    const dialog = page.getByRole('dialog', { name: 'File diff', exact: true });
    expect(await dialog.locator('[data-kind="addition"] .diff-code').evaluateAll(elements => elements.map(element => element.textContent))).toEqual([markup, longLine]);
    await expect(dialog.locator('[data-kind="removal"] .diff-code')).toHaveJSProperty('textContent', before);
    await dialog.getByRole('button', { name: 'Close diff', exact: true }).click();
  }

  await expect(page.getByRole('button', { name: 'Follow latest', exact: true })).toBeEnabled();
  await page.getByRole('button', { name: 'Follow latest', exact: true }).click();
  const clippedName = 'unified-truncated.txt';
  await writeFile(resolve(root, clippedName), Array.from({ length: 400 }, (_, index) => 'captured ' + index + ': ' + 'source '.repeat(18)).join('\n') + '\n');
  await expect(page.locator('[data-testid="workflow-step"][title^="' + clippedName + '"]')).toHaveCount(1);
  await expect(panel.getByTestId('visual-truncation')).toContainText('Preview truncated. Only the captured portion is available.');
  await expect(panel.locator('.diff-code')).not.toContainText(['… [truncated]']);
  const emptyName = 'unified-empty.txt';
  const emptyFile = resolve(root, emptyName);
  await writeFile(emptyFile, '');
  await expect(page.locator('[data-testid="workflow-step"][title^="' + emptyName + '"]')).toHaveCount(1);
  await expect(panel).toContainText('Empty file created.');
  await expect(panel.locator('[data-kind="addition"], [data-kind="removal"]')).toHaveCount(0);
  await unlink(emptyFile);
  await expect(page.locator('[data-testid="workflow-step"][title^="' + emptyName + '"]')).toHaveCount(2);
  await expect(panel).toContainText('Empty file deleted.');
  await expect(panel.locator('[data-kind="addition"], [data-kind="removal"]')).toHaveCount(0);
  expect(feedbackRequests).toEqual([]);
  expect((await checkIn(request, session)).feedback ?? []).toEqual([]);
  expect(errors).toEqual([]);
});

for (const recovery of [{ design: 'minimal', reply: 'changes' }, { design: 'interactive', reply: 'context' }] as const) {
  test(recovery.design + ' requests a missing explanation once and resolves an exact ' + recovery.reply + ' reply', async ({ page, request }) => {
    const errors: string[] = []; page.on('pageerror', error => errors.push(error.message));
    const feedbackRequests: { session_id: string; text: string; node_name: string; target_agent: string }[] = [];
    page.on('request', outgoing => { if (new URL(outgoing.url()).pathname === '/api/feedback' && outgoing.method() === 'POST') feedbackRequests.push(outgoing.postDataJSON()); });
    await page.goto('/?companion=1');
    const session = 'missing-reason-' + recovery.design;
    await selectSession(page, request, session);
    const root = await isolatedRoot(request);
    const name = 'missing-reason-' + recovery.design + '.ts';
    const file = resolve(root, name);
    const fileSteps = page.locator('[data-testid="workflow-step"][title^="' + name + '"]');
    const before = 'export const version = "before";';
    const after = 'export const version = "after";';
    await writeFile(file, before + '\n');
    await expect(fileSteps).toHaveCount(1);
    const first = (await savedEvents(request, session)).find(event => event.source === 'watcher' && event.node_name === name)!;
    await request.post('/api/events', { data: { ...report(session, name, 'Start the fixture', 'Keep a known previous value for comparison.'), action: first.action, change_id: first.id } });
    await writeFile(file, after + '\n');
    await expect(fileSteps).toHaveCount(2);
    const revision = (await savedEvents(request, session)).filter(event => event.source === 'watcher' && event.node_name === name).at(-1)!;
    await request.post('/api/events', { data: { ...report(session, 'unrelated-' + recovery.design + '.ts', 'Separate agent work', 'This later reporter does not own the pending saved file.'), agent_id: 'unrelated-agent' } });
    await chooseStep(fileSteps.last());
    await pickDesign(page, recovery.design);
    const panel = () => page.getByRole('region', { name: recovery.design === 'minimal' ? 'Why this step' : 'Selected change', exact: true });
    await expect(panel()).toContainText('The agent hasn’t explained this save yet.');
    await expect(panel().getByRole('button', { name: 'Request explanation', exact: true })).toBeEnabled();
    await chooseStep(fileSteps.first());
    await chooseStep(fileSteps.last());
    await pickDesign(page, recovery.design === 'minimal' ? 'interactive' : 'minimal');
    await pickDesign(page, recovery.design);
    await panel().getByRole('button', { name: 'Show diff', exact: true }).click();
    const dialog = page.getByRole('dialog', { name: 'File diff', exact: true });
    await expect(dialog.locator('[data-kind="removal"] .diff-code')).toHaveText([before]);
    await expect(dialog.locator('[data-kind="addition"] .diff-code')).toHaveText([after]);
    await dialog.getByRole('button', { name: 'Close diff', exact: true }).click();
    expect(feedbackRequests).toEqual([]);

    const response = page.waitForResponse(outgoing => new URL(outgoing.url()).pathname === '/api/feedback' && outgoing.request().method() === 'POST');
    await panel().getByRole('button', { name: 'Request explanation', exact: true }).click();
    expect((await response).ok()).toBe(true);
    await expect(panel().getByRole('button', { name: 'Request queued', exact: true })).toBeDisabled();
    await expect(panel()).toContainText('next check-in');
    expect(feedbackRequests).toHaveLength(1);
    expect(feedbackRequests[0].session_id).toBe(session);
    expect(feedbackRequests[0].node_name).toBe(name);
    expect(feedbackRequests[0].target_agent).toBe('*');
    expect(feedbackRequests[0].text).toContain('Only answer if this change is attributable to your task');
    expect(feedbackRequests[0].text).toContain('context_id="' + revision.id + '"');
    expect(feedbackRequests[0].text).toContain('action="' + revision.action + '"');
    expect(feedbackRequests[0].text).toContain('a brief user-facing why');
    const state = await checkIn(request, session);
    expect(state.feedback).toHaveLength(1);
    await chooseStep(fileSteps.first());
    await chooseStep(fileSteps.nth(1));
    await expect(panel().getByRole('button', { name: 'Request queued', exact: true })).toBeDisabled();
    await pickDesign(page, recovery.design === 'minimal' ? 'interactive' : 'minimal');
    await expect(page.getByRole('button', { name: 'Request queued', exact: true })).toBeDisabled();
    await pickDesign(page, recovery.design);
    expect(feedbackRequests).toHaveLength(1);

    await writeFile(file, 'export const version = "later";\n');
    await expect(fileSteps).toHaveCount(3);
    await expect(fileSteps.nth(1)).toHaveAttribute('aria-pressed', 'true');
    await expect(page.getByRole('button', { name: 'Follow latest', exact: true })).toBeEnabled();
    const later = (await savedEvents(request, session)).filter(event => event.source === 'watcher' && event.node_name === name).at(-1)!;
    const unrelated = await request.post('/api/events', { data: { ...report(session, name, 'Explain the previous save', 'This reply belongs to the previous revision.'), action: first.action, context_id: first.id } });
    expect(unrelated.ok()).toBe(true);
    await expect(panel().getByRole('button', { name: 'Request queued', exact: true })).toBeDisabled();
    const why = 'Update the displayed value while preserving the exact previous value in the saved diff.';
    const reply = recovery.reply === 'changes'
      ? await request.post('/api/changes', { data: { session_id: session, agent_id: 'test-agent', changes: [{ change_id: revision.id, summary: 'Update the displayed value', why }] } })
      : await request.post('/api/events', { data: { ...report(session, name, 'Update the displayed value', why), action: revision.action, context_id: revision.id } });
    expect(reply.ok()).toBe(true);
    await expect(fileSteps.nth(1)).toHaveAttribute('aria-pressed', 'true');
    await expect(panel()).toContainText(why);
    await expect(panel()).toContainText('The agent added an explanation.');
    await expect(page.getByRole('button', { name: 'Request queued', exact: true })).toHaveCount(0);
    if (recovery.design === 'minimal') await expect(page.getByRole('button', { name: 'Request explanation', exact: true })).toHaveCount(0);
    else await expect(panel().getByRole('button', { name: 'Explain more', exact: true })).toBeEnabled();
    await panel().getByRole('button', { name: 'Show diff', exact: true }).click();
    await expect(dialog.locator('[data-kind="removal"] .diff-code')).toHaveText([before]);
    await expect(dialog.locator('[data-kind="addition"] .diff-code')).toHaveText([after]);
    await expect(dialog).not.toContainText('"later"');
    await dialog.getByRole('button', { name: 'Close diff', exact: true }).click();
    await chooseStep(fileSteps.last());
    await expect(panel()).toContainText('The agent hasn’t explained this save yet.');
    await expect(panel().getByRole('button', { name: 'Request explanation', exact: true })).toBeEnabled();
    expect((await checkIn(request, session)).pending_changes.map((change: { change_id: string }) => change.change_id)).toEqual([later.id]);
    expect(feedbackRequests).toHaveLength(1);
    expect(errors).toEqual([]);
  });
}

test('missing-explanation requests reset on reporting windows, chat changes and shared Clear while ordinary reconnects preserve them', async ({ page, request, context }) => {
  const feedbackRequests: unknown[] = [];
  page.on('request', outgoing => { if (new URL(outgoing.url()).pathname === '/api/feedback' && outgoing.method() === 'POST') feedbackRequests.push(outgoing.postDataJSON()); });
  await page.addInitScript(() => {
    const Original = window.WebSocket;
    const sockets: WebSocket[] = [];
    window.WebSocket = class extends Original {
      constructor(url: string | URL, protocols?: string | string[]) { super(url, protocols); sockets.push(this); }
    };
    (window as Window & { stefTestSockets?: WebSocket[] }).stefTestSockets = sockets;
  });
  await page.goto('/?companion=1');
  const session = 'missing-reason-boundaries';
  await selectSession(page, request, session);
  const root = await isolatedRoot(request);
  const name = 'missing-reason-boundaries.ts';
  const file = resolve(root, name);
  const fileSteps = page.locator('[data-testid="workflow-step"][title^="' + name + '"]');
  await writeFile(file, 'export const pending = true;\n');
  await expect(fileSteps).toHaveCount(1);
  const old = (await savedEvents(request, session)).find(event => event.source === 'watcher' && event.node_name === name)!;
  await chooseStep(fileSteps);
  await page.getByRole('button', { name: 'Request explanation', exact: true }).click();
  await expect(page.getByRole('button', { name: 'Request queued', exact: true })).toBeDisabled();
  expect(feedbackRequests).toHaveLength(1);
  await page.evaluate(() => (window as Window & { stefTestSockets?: WebSocket[] }).stefTestSockets!.at(-1)!.close());
  await expect.poll(() => page.evaluate(() => (window as Window & { stefTestSockets?: WebSocket[] }).stefTestSockets!.length)).toBe(2);
  await expect(page.getByRole('button', { name: 'Updates', exact: true })).toBeEnabled();
  await expect(page.getByRole('button', { name: 'Request queued', exact: true })).toBeDisabled();
  expect(feedbackRequests).toHaveLength(1);
  await pickUpdates(page, 'disabled');
  await expect(page.getByRole('button', { name: 'Request queued', exact: true })).toHaveCount(0);
  expect((await checkIn(request, session)).feedback ?? []).toEqual([]);
  await pickUpdates(page, 'milestones');
  await page.getByRole('button', { name: 'Show saved history', exact: true }).click();
  await expect(fileSteps).toHaveCount(1);
  if ((await fileSteps.getAttribute('aria-pressed')) !== 'true') await chooseStep(fileSteps);
  await expect(page.getByRole('button', { name: 'Request explanation', exact: true })).toBeEnabled();
  await pickDesign(page, 'interactive');
  await page.getByRole('button', { name: 'Show diff', exact: true }).click();
  await page.getByRole('button', { name: 'Close diff', exact: true }).click();
  await pickDesign(page, 'minimal');
  await page.getByRole('button', { name: 'Hide saved history', exact: true }).click();
  await page.getByRole('button', { name: 'Show saved history', exact: true }).click();
  expect(feedbackRequests).toHaveLength(1);
  expect((await checkIn(request, session)).pending_changes).toEqual([]);
  await page.getByRole('button', { name: 'Request explanation', exact: true }).click();
  await expect(page.getByRole('button', { name: 'Request queued', exact: true })).toBeDisabled();
  expect(feedbackRequests).toHaveLength(2);
  const requested = await checkIn(request, session);
  expect(requested.feedback).toHaveLength(1);
  const explained = await request.post('/api/events', { data: { ...report(session, name, 'Explain requested history', 'Keep this saved revision available for an explicitly requested explanation.'), action: old.action, context_id: old.id } });
  expect(explained.ok()).toBe(true);
  await expect(page.getByRole('region', { name: 'Why this step', exact: true })).toContainText('The agent added an explanation.');
  expect((await checkIn(request, session)).pending_changes).toEqual([]);

  await pickUpdates(page, 'files');
  await writeFile(file, 'export const pending = false;\n');
  await expect(fileSteps).toHaveCount(1);
  await chooseStep(fileSteps);
  await page.getByRole('button', { name: 'Request explanation', exact: true }).click();
  await expect(page.getByRole('button', { name: 'Request queued', exact: true })).toBeDisabled();
  await selectSession(page, request, 'missing-reason-other-chat');
  await expect(page.getByTestId('workflow-step')).toHaveCount(0);
  await selectSession(page, request, session);
  await expect(fileSteps).toHaveCount(1);
  await expect(page.getByRole('button', { name: 'Request explanation', exact: true })).toBeEnabled();
  await page.getByRole('button', { name: 'Request explanation', exact: true }).click();
  await expect(page.getByRole('button', { name: 'Request queued', exact: true })).toBeDisabled();
  const otherView = await context.newPage();
  await otherView.goto('/?companion=1');
  await expect(otherView.getByTestId('workflow-step')).toHaveCount(1);
  await otherView.getByRole('button', { name: 'Clear this chat’s workflow', exact: true }).click();
  await expect(page.getByTestId('workflow-step')).toHaveCount(0);
  await expect(page.getByRole('button', { name: 'Request queued', exact: true })).toHaveCount(0);
  await otherView.close();
  await writeFile(file, 'export const pending = "after clear";\n');
  await expect(fileSteps).toHaveCount(1);
  await chooseStep(fileSteps);
  await expect(page.getByRole('button', { name: 'Request explanation', exact: true })).toBeEnabled();
  await expect(page.getByRole('status')).toHaveCount(0);
  expect(feedbackRequests).toHaveLength(4);
});

test('old unexplained files never re-enter pending reporting after milestones or Disabled', async ({ page, request }) => {
  await page.goto('/?companion=1');
  const root = await isolatedRoot(request);
  for (const mode of ['milestones', 'disabled'] as const) {
    const session = 'future-only-' + mode;
    await selectSession(page, request, session);
    const name = 'future-only-' + mode + '.ts';
    const file = resolve(root, name);
    const fileSteps = page.locator('[data-testid="workflow-step"][title^="' + name + '"]');
    await writeFile(file, 'export const state = "old unexplained";\n');
    await expect(fileSteps).toHaveCount(1);
    const old = (await savedEvents(request, session)).find(event => event.source === 'watcher' && event.node_name === name)!;
    expect((await checkIn(request, session)).pending_changes.map((change: { change_id: string }) => change.change_id)).toEqual([old.id]);
    await pickUpdates(page, mode);
    const suppressed = await checkIn(request, session);
    expect(typeof suppressed.updates_from).toBe('string');
    expect(BigInt(suppressed.updates_from)).toBeGreaterThan(BigInt(old.id));
    await writeFile(file, 'export const state = "suppressed baseline";\n');
    await writeFile(resolve(root, 'hidden-' + mode + '.ts'), 'export const hidden = true;\n');
    expect((await checkIn(request, session, true)).pending_changes).toEqual([]);
    await pickUpdates(page, 'files');
    await expect(fileSteps).toHaveCount(0);
    await page.reload();
    await expect(fileSteps).toHaveCount(0);
    const resumed = await checkIn(request, session);
    expect(resumed.pending_changes).toEqual([]);
    expect(resumed.pending_count).toBe(0);
    expect(resumed.events).toEqual([]);
    const noop = await request.post('/api/settings', { data: { session_id: session, update_mode: 'files' } });
    expect(await noop.json()).toEqual({ update_mode: 'files' });
    expect((await checkIn(request, session)).updates_from).toBe(resumed.updates_from);
    await page.getByRole('button', { name: 'Show saved history', exact: true }).click();
    await expect(fileSteps).toHaveCount(1);
    await chooseStep(fileSteps);
    await expect(page.getByRole('region', { name: 'Why this step' })).toContainText('The agent hasn’t explained this save yet.');
    expect((await checkIn(request, session)).pending_changes).toEqual([]);
    const batch = await request.post('/api/changes', { data: { session_id: session, agent_id: 'test-agent', changes: [{ change_id: old.id, summary: 'Do not explain old backlog', why: 'Automatic reporting applies only to the current window.' }] } });
    expect(batch.status()).toBe(400);
    expect(await batch.text()).toContain('predates the current reporting window');
    expect((await checkIn(request, session)).pending_count).toBe(0);
    const single = await request.post('/api/events', { data: { ...report(session, name, 'Do not explain old backlog', 'Automatic reporting applies only to the current window.'), action: old.action, change_id: old.id } });
    expect(single.status()).toBe(400);
    expect(await single.text()).toContain('predates the current reporting window');
    await expect(page.getByRole('region', { name: 'Why this step' })).toContainText('The agent hasn’t explained this save yet.');
    await page.getByRole('button', { name: 'Request explanation', exact: true }).click();
    const manual = await request.post('/api/events', { data: { ...report(session, name, 'Explain saved history on request', 'This older revision can still receive an explicitly requested explanation.'), action: old.action, context_id: old.id } });
    expect(manual.ok()).toBe(true);
    expect((await manual.json()).accepted).toBe(true);
    await expect(page.getByRole('region', { name: 'Why this step' })).toContainText('This older revision can still receive an explicitly requested explanation.');
    expect((await checkIn(request, session)).pending_count).toBe(0);
    await page.getByRole('button', { name: 'Hide saved history', exact: true }).click();
    await writeFile(file, 'export const state = "new eligible revision";\n');
    await expect(fileSteps).toHaveCount(1);
    const future = await checkIn(request, session, true);
    expect(future.pending_changes).toHaveLength(1);
    expect(BigInt(future.pending_changes[0].change_id)).toBeGreaterThanOrEqual(BigInt(resumed.updates_from));
    const revisions = (await savedEvents(request, session)).filter(event => event.source === 'watcher' && event.node_name === name);
    expect(revisions).toHaveLength(2);
    expect(revisions[1].before).toContain('suppressed baseline');
    expect(revisions[1].after).toContain('new eligible revision');
    expect((await savedEvents(request, session)).some(event => event.node_name === 'hidden-' + mode + '.ts')).toBe(false);
  }
});

test('file diffs keep exact additions, removals and AI explanations for each saved revision', async ({ page, request }) => {
  const errors: string[] = []; page.on('pageerror', error => errors.push(error.message));
  const session = 'file-diff-revisions';
  await page.goto('/?companion=1');
  await selectSession(page, request, session);
  const root = await isolatedRoot(request);
  const file = resolve(root, 'diff-revisions.txt');
  const fileSteps = page.locator('[data-testid="workflow-step"][title^="diff-revisions.txt"]');
  const dialog = page.getByRole('dialog', { name: 'File diff', exact: true });
  const showDiff = page.getByRole('button', { name: 'Show diff', exact: true });
  const added = dialog.locator('[data-testid="diff-line"][data-kind="addition"] .diff-code');
  const removed = dialog.locator('[data-testid="diff-line"][data-kind="removal"] .diff-code');
  const before = ['const mode = "before";', '---old marker', '+++old marker'];
  const after = ['const mode = "after";', '---new marker', '+++new marker'];

  await writeFile(file, before.join('\n') + '\n');
  await expect(fileSteps).toHaveCount(1);
  const created = (await savedEvents(request, session)).find(event => event.source === 'watcher' && event.kind === 'file' && event.node_name === 'diff-revisions.txt')!;
  expect(created.action).toBe('created');
  await expect(dialog).toHaveCount(0);
  await chooseStep(fileSteps.first());
  await showDiff.click();
  await expect(dialog).toBeVisible();
  await expect(dialog).toContainText('diff-revisions.txt');
  await expect(added).toHaveText(before);
  await expect(removed).toHaveCount(0);
  await expect(dialog.locator('[data-testid="diff-line"][data-kind="addition"] .line-number:nth-child(1)')).toHaveText(['', '', '']);
  await expect(dialog.locator('[data-testid="diff-line"][data-kind="addition"] .line-number:nth-child(2)')).toHaveText(['1', '2', '3']);
  await expect(dialog.getByRole('region', { name: 'Why this change' })).toContainText('The agent hasn’t explained this save yet.');

  await writeFile(file, after.join('\n') + '\n');
  await expect(fileSteps).toHaveCount(2);
  await expect(added).toHaveText(before);
  const creationReason = 'Add a small text fixture to verify exact saved-file differences.';
  await request.post('/api/events', { data: { ...report(session, created.node_name, 'Add the diff fixture', creationReason), action: created.action, change_id: created.id } });
  await expect(dialog.getByRole('region', { name: 'Why this change' })).toContainText(creationReason);
  await expect(added).toHaveText(before);
  await page.getByRole('button', { name: 'Close diff', exact: true }).click();
  await expect(dialog).toHaveCount(0);
  await expect(fileSteps.first()).toHaveAttribute('aria-pressed', 'true');
  await expect(showDiff).toBeFocused();
  await page.getByRole('button', { name: 'Follow latest', exact: true }).click();
  await expect(fileSteps.last()).toHaveAttribute('aria-pressed', 'true');

  const modified = (await savedEvents(request, session)).filter(event => event.source === 'watcher' && event.kind === 'file' && event.node_name === created.node_name).at(-1)!;
  expect(modified.action).toBe('modified');
  await chooseStep(fileSteps.last());
  await showDiff.click();
  await expect(removed).toHaveText(before);
  await expect(added).toHaveText(after);
  await expect(dialog.locator('[data-testid="diff-line"][data-kind="removal"] .line-number:nth-child(1)')).toHaveText(['1', '2', '3']);
  await expect(dialog.locator('[data-testid="diff-line"][data-kind="removal"] .line-number:nth-child(2)')).toHaveText(['', '', '']);
  await expect(dialog.locator('[data-testid="diff-line"][data-kind="addition"] .line-number:nth-child(2)')).toHaveText(['1', '2', '3']);
  await expect(dialog.locator('[data-testid="diff-line"][data-kind="hunk"]')).toContainText('@@ -1,3 +1,3 @@');
  await expect(dialog.getByRole('region', { name: 'Why this change' })).not.toContainText(creationReason);
  const modificationReason = 'Replace the sample lines so added and removed content can be reviewed together.';
  await request.post('/api/events', { data: { ...report(session, modified.node_name, 'Replace the sample lines', modificationReason), action: modified.action, change_id: modified.id } });
  await expect(dialog.getByRole('region', { name: 'Why this change' })).toContainText(modificationReason);
  await page.keyboard.press('Escape');
  await expect(dialog).toHaveCount(0);
  await expect(fileSteps.last()).toHaveAttribute('aria-pressed', 'true');
  await expect(showDiff).toBeFocused();

  await page.reload();
  await expect(fileSteps).toHaveCount(2);
  await expect(fileSteps.last()).toHaveAttribute('aria-pressed', 'true');
  await showDiff.click();
  await expect(removed).toHaveText(before);
  await expect(added).toHaveText(after);
  await expect(dialog.getByRole('region', { name: 'Why this change' })).toContainText(modificationReason);
  await page.getByRole('button', { name: 'Close diff', exact: true }).click();
  await chooseStep(fileSteps.first());
  await showDiff.click();
  await expect(added).toHaveText(before);
  await expect(removed).toHaveCount(0);
  await expect(dialog.getByRole('region', { name: 'Why this change' })).toContainText(creationReason);
  await page.getByRole('button', { name: 'Close diff', exact: true }).click();

  await unlink(file);
  await expect(fileSteps).toHaveCount(3);
  const deleted = (await savedEvents(request, session)).filter(event => event.source === 'watcher' && event.kind === 'file' && event.node_name === created.node_name).at(-1)!;
  expect(deleted.action).toBe('deleted');
  const deletionReason = 'Remove the fixture after checking that deletion preserves its exact prior contents.';
  await request.post('/api/events', { data: { ...report(session, deleted.node_name, 'Remove the diff fixture', deletionReason), action: deleted.action, change_id: deleted.id } });
  await chooseStep(fileSteps.last());
  await showDiff.click();
  await expect(removed).toHaveText(after);
  await expect(added).toHaveCount(0);
  await expect(dialog.getByRole('region', { name: 'Why this change' })).toContainText(deletionReason);
  await page.screenshot({ path: '../artifacts/stef-file-diff-deleted.png', fullPage: true });
  // A mode changed in another view closes this dialog without reopening it later.
  for (const mode of ['milestones', 'disabled']) {
    await request.post('/api/settings', { data: { session_id: session, update_mode: mode } });
    await expect(page.locator('.stef-app')).toHaveAttribute('data-update-mode', mode);
    await expect(dialog).toHaveCount(0);
    await request.post('/api/settings', { data: { session_id: session, update_mode: 'files' } });
    await expect(page.getByTestId('workflow-step')).toHaveCount(0);
    await page.getByRole('button', { name: 'Show saved history', exact: true }).click();
    await expect(fileSteps.last()).toHaveAttribute('aria-pressed', 'true');
    await expect(dialog).toHaveCount(0);
    if (mode === 'milestones') await showDiff.click();
  }
  expect(errors).toEqual([]);
});

test('diff source stays escaped and readable in narrow light and eye comfort views', async ({ page, request }) => {
  const errors: string[] = []; page.on('pageerror', error => errors.push(error.message));
  const session = 'file-diff-escaped';
  await page.goto('/?companion=1');
  await selectSession(page, request, session);
  const root = await isolatedRoot(request);
  const file = resolve(root, 'diff-source.ts');
  const fileSteps = page.locator('[data-testid="workflow-step"][title^="diff-source.ts"]');
  const source = 'export const markup = \'<img src=x onerror="window.stefDiffInjected=true">\';';
  const longLine = 'export const message = "' + 'Keep source text exact. '.repeat(18) + '";';
  await writeFile(file, 'export const markup = "before";\n');
  await expect(fileSteps).toHaveCount(1);
  await writeFile(file, source + '\n' + longLine + '\n');
  await expect(fileSteps).toHaveCount(2);
  await chooseStep(fileSteps.last());
  const palettes: string[] = [];
  for (const [label, theme] of [['Light', 'light'], ['Eye comfort', 'comfort']]) {
    await page.getByRole('button', { name: /appearance/i }).click();
    await page.getByRole('menuitemradio', { name: label, exact: true }).click();
    await expect(page.locator('html')).toHaveAttribute('data-theme', theme);
    await page.setViewportSize({ width: 320, height: 640 });
    await page.getByRole('button', { name: 'Show diff', exact: true }).click();
    const dialog = page.getByRole('dialog', { name: 'File diff', exact: true });
    await expect(dialog).toBeVisible();
    await expect(dialog.locator('[data-testid="diff-line"][data-kind="addition"] .diff-code')).toHaveText([source, longLine]);
    await expect(dialog.locator('img')).toHaveCount(0);
    expect(await page.evaluate(() => (window as typeof window & { stefDiffInjected?: boolean }).stefDiffInjected)).toBeUndefined();
    const bounds = await dialog.boundingBox();
    expect(bounds).not.toBeNull();
    expect(bounds!.x).toBeGreaterThanOrEqual(0);
    expect(bounds!.x + bounds!.width).toBeLessThanOrEqual(320);
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
    const contrast = await dialog.evaluate(element => {
      const luminance = (color: string) => {
        const channels = (color.match(/[\d.]+/g) ?? []).slice(0, 3).map(value => {
          const channel = Number(value) / 255;
          return channel <= 0.04045 ? channel / 12.92 : ((channel + 0.055) / 1.055) ** 2.4;
        });
        return channels[0] * 0.2126 + channels[1] * 0.7152 + channels[2] * 0.0722;
      };
      return ['addition', 'removal'].map(kind => {
        const row = element.querySelector('[data-testid="diff-line"][data-kind="' + kind + '"]')!;
        const foreground = luminance(getComputedStyle(row).color);
        const background = luminance(getComputedStyle(row).backgroundColor);
        return (Math.max(foreground, background) + 0.05) / (Math.min(foreground, background) + 0.05);
      });
    });
    for (const ratio of contrast) expect(ratio, label + ' diff text contrast').toBeGreaterThanOrEqual(4.5);
    palettes.push(await dialog.evaluate(element => getComputedStyle(element).backgroundColor));
    await page.screenshot({ path: '../artifacts/stef-file-diff-' + theme + '.png', fullPage: true });
    await dialog.getByRole('button', { name: 'Close diff', exact: true }).click();
    await expect(fileSteps.last()).toHaveAttribute('aria-pressed', 'true');
  }
  expect(new Set(palettes).size).toBe(2);
  expect(errors).toEqual([]);
});

test('empty file revisions have no invented lines and milestones without captured code have no diff control', async ({ page, request }) => {
  const session = 'file-diff-empty';
  await page.goto('/?companion=1');
  await selectSession(page, request, session);
  const root = await isolatedRoot(request);
  const file = resolve(root, 'diff-empty.txt');
  const fileSteps = page.locator('[data-testid="workflow-step"][title^="diff-empty.txt"]');
  const dialog = page.getByRole('dialog', { name: 'File diff', exact: true });
  await writeFile(file, '');
  await expect(fileSteps).toHaveCount(1);
  await chooseStep(fileSteps.first());
  await page.getByRole('button', { name: 'Show diff', exact: true }).click();
  await expect(dialog).toContainText('Empty file created.');
  await expect(dialog.locator('[data-testid="diff-line"][data-kind="addition"], [data-testid="diff-line"][data-kind="removal"]')).toHaveCount(0);
  await dialog.getByRole('button', { name: 'Close diff', exact: true }).click();
  await unlink(file);
  await expect(fileSteps).toHaveCount(2);
  await chooseStep(fileSteps.last());
  await page.getByRole('button', { name: 'Show diff', exact: true }).click();
  await expect(dialog).toContainText('Empty file deleted.');
  await expect(dialog.locator('[data-testid="diff-line"][data-kind="addition"], [data-testid="diff-line"][data-kind="removal"]')).toHaveCount(0);
  await dialog.getByRole('button', { name: 'Close diff', exact: true }).click();
  await request.post('/api/events', { data: report(session, '@workflow', 'Review the workflow', 'A planning milestone does not invent a saved-file diff.') });
  await chooseStep(page.getByRole('button', { name: 'Review the workflow', exact: true }));
  await expect(page.getByRole('button', { name: 'Show diff', exact: true })).toHaveCount(0);
});

test('the optional embedded host still receives file-open actions without a browser socket', async ({ page }) => {
  let sockets = 0; page.on('websocket', () => sockets++);
  await page.addInitScript(() => {
    const target = window as typeof window & { acquireVsCodeApi: () => { postMessage: (message: unknown) => void }; stefMessages: unknown[] };
    target.stefMessages = [];
    target.acquireVsCodeApi = () => ({ postMessage(message: unknown) {
      target.stefMessages.push(message);
      if ((message as { type?: string }).type === 'stef.ready') setTimeout(() => {
        window.dispatchEvent(new MessageEvent('message', { data: { type: 'stef.stream', message: { type: 'snapshot', repo: '/personal/test', session_id: 'embedded', events: [{ id: '1', timestamp: new Date().toISOString(), agent_id: 'test', node_name: 'main.go', action: 'scouted', summary: 'Find the entry point', why: 'Requests enter here.', status: 'ready', source: 'agent' }] } } }));
        window.dispatchEvent(new MessageEvent('message', { data: { type: 'stef.connection', connected: true } }));
      }, 0);
    } });
  });
  await page.goto('/?session_id=embedded');
  await chooseStep(page.getByTestId('workflow-step'));
  await page.getByRole('button', { name: 'Open main.go' }).click();
  await expect.poll(() => page.evaluate(() => (window as typeof window & { stefMessages: { type: string }[] }).stefMessages.some(message => message.type === 'stef.openFile'))).toBe(true);
  await pickDesign(page, 'interactive');
  await page.getByRole('button', { name: 'Open main.go' }).click();
  await expect.poll(() => page.evaluate(() => (window as typeof window & { stefMessages: { type: string; node?: string }[] }).stefMessages.filter(message => message.type === 'stef.openFile' && message.node === 'main.go').length)).toBe(2);
  expect(sockets).toBe(0);
});


for (const design of ['minimal', 'interactive'] as const) {
  test(design + ' reuses public output without losing file reasons or exact diffs', async ({ page, request }) => {
    const errors: string[] = [], writes: string[] = [];
    page.on('pageerror', error => errors.push(error.message));
    const session = 'reused-public-output-' + design;
    await page.goto('/?companion=1');
    await selectSession(page, request, session);
    await pickDesign(page, design);
    const root = await isolatedRoot(request);
    const names = ['preference', 'startup', 'test'].map(name => 'reuse-' + design + '-' + name + '.ts');
    const content = names.map((_, index) => 'export const saved' + index + ' = true;\n');
    for (let i = 0; i < names.length; i++) await writeFile(resolve(root, names[i]), content[i]);
    await expect(page.getByTestId('workflow-step')).toHaveCount(3);
    const revisions = (await savedEvents(request, session)).filter(event => event.source === 'watcher' && names.includes(event.node_name));
    expect(revisions).toHaveLength(3);
    const common = 'Apply the saved preference before rendering.';
    const testReason = 'Verify startup restores the saved preference.';
    const response = await request.post('/api/changes', { data: {
      session_id: session, agent_id: 'test-agent', summary: 'Restore saved preference', why: common,
      changes: revisions.map(event => ({ change_id: event.id, ...(event.node_name === names[2] ? { why: testReason } : {}) })),
    } });
    expect(response.status()).toBe(201);
    const receipt = await response.json();
    expect(receipt.accepted).toBe(true);
    expect(receipt.pending_count).toBe(0);
    expect(JSON.stringify(receipt)).not.toContain(common);
    await expect(page.getByTestId('workflow-step')).toHaveCount(3);
    page.on('request', outgoing => {
      if (outgoing.method() !== 'GET' && new URL(outgoing.url()).pathname.startsWith('/api/')) writes.push(new URL(outgoing.url()).pathname);
    });
    for (let i = 0; i < names.length; i++) {
      const step = page.locator('[data-testid="workflow-step"][title^="' + names[i] + '"]');
      await chooseStep(step);
      const reason = i === 2 ? testReason : common;
      await expect(page.getByRole('region', { name: design === 'minimal' ? 'Why this step' : 'Selected change', exact: true })).toContainText(reason);
      await expect(page.getByRole('button', { name: 'Request explanation', exact: true })).toHaveCount(0);
      await page.getByRole('button', { name: 'Show diff', exact: true }).click();
      const dialog = page.getByRole('dialog', { name: 'File diff', exact: true });
      await expect(dialog.getByRole('region', { name: 'Why this change' })).toContainText(reason);
      await expect(dialog.locator('[data-testid="diff-line"][data-kind="addition"] .diff-code')).toHaveText(content[i].trim());
      await page.getByRole('button', { name: 'Close diff', exact: true }).click();
    }
    expect(writes).toEqual([]);
    expect(errors).toEqual([]);
  });
}
