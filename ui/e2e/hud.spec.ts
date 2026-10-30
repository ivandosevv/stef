import { test, expect, type APIRequestContext, type Page } from '@playwright/test';
import { writeFile, unlink } from 'node:fs/promises';
import { resolve } from 'node:path';

const report = (session: string, node: string, summary: string, why: string, parents: string[] = []) => ({ session_id: session, agent_id: 'test-agent', node_name: node, action: 'modified', summary, why, status: 'ready', parents });
type UpdateMode = 'milestones' | 'files' | 'disabled';
type SavedEvent = { id: string; source: string; kind?: string; node_name: string; action: string; before?: string; after?: string; change_id?: string };

// The mode belongs to the daemon, so reset it rather than relying on browser isolation.
test.beforeEach(async ({ request }) => {
  const response = await request.post('/api/settings', { data: { update_mode: 'files' } });
  expect(response.ok()).toBe(true);
  expect(await response.json()).toEqual({ update_mode: 'files' });
});

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
  const labels = { milestones: 'Major milestones', files: 'Every file', disabled: 'Disabled' };
  await page.getByRole('button', { name: 'Updates', exact: true }).click();
  await page.getByRole('menuitemradio', { name: new RegExp('^' + labels[mode]) }).click();
  await expect(page.getByRole('menu', { name: 'Updates', exact: true })).toHaveCount(0);
  await expect(page.locator('.stef-app')).toHaveAttribute('data-update-mode', mode);
}

test('the minimal live workflow has arrows, brief reasons and an inline suggestion', async ({ page, request }) => {
  const errors: string[] = []; page.on('pageerror', error => errors.push(error.message));
  await page.goto('/?companion=1');
  await expect(page).toHaveTitle('stef');
  await request.post('/api/sessions/activate', { data: { session_id: 'minimal-chat' } });
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
  await expect(page.getByRole('region', { name: 'Why this step' })).toHaveCount(0);
  await expect(page.getByRole('textbox', { name: 'Suggest something to the agent' })).toHaveCount(0);
  await expect(page.getByTestId('workflow-arrow')).toHaveCount(1);
  await expect(page.getByTestId('workflow-arrow').locator('title')).toHaveText('Connection reported by the agent');
  await page.getByRole('button', { name: /Update the route/ }).click();
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
  await request.post('/api/sessions/activate', { data: { session_id: session } });
  await expect(page.getByText('Listening…')).toBeVisible();
  const snapshot = await (await request.get('/api/events')).json();
  const root = resolve(snapshot.repo);
  expect(root.startsWith(resolve('..', '.test-workspace') + '/')).toBe(true);

  await writeFile(resolve(root, 'theme.ts'), 'export const background = "warm";\n');
  const themeSteps = page.locator('[data-testid="workflow-step"][title^="theme.ts"]');
  await expect(themeSteps).toHaveCount(1);
  await themeSteps.first().click();
  await expect(page.getByRole('region', { name: 'Why this step' })).toContainText('Waiting for the agent’s explanation.');
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
  await expect(themeSteps.first()).toHaveAttribute('aria-pressed', 'true');
  await expect(page.getByRole('region', { name: 'Why this step' })).toContainText(explanation);
  await expect(page.getByTestId('workflow-step')).toHaveCount(2);

  await writeFile(resolve(root, 'theme.ts'), 'export const background = "light";\n');
  await expect(themeSteps).toHaveCount(2);
  await expect(page.getByTestId('workflow-step')).toHaveCount(3);
  await expect(page.getByTestId('workflow-arrow')).toHaveCount(2);
  await themeSteps.last().click();
  await expect(page.getByRole('region', { name: 'Why this step' })).toContainText('Waiting for the agent’s explanation.');
  await expect(page.getByRole('region', { name: 'Why this step' })).not.toContainText(explanation);
  await page.reload();
  await expect(page.getByTestId('workflow-step')).toHaveCount(3);
  await expect(page.getByRole('region', { name: 'Why this step' })).toContainText('Waiting for the agent’s explanation.');
  await themeSteps.first().click();
  await expect(page.getByRole('region', { name: 'Why this step' })).toContainText(explanation);
  await page.screenshot({ path: '../artifacts/stef-automatic-file-changes.png', fullPage: true });
  expect(errors).toEqual([]);
});

test('new chats reset, resumed chats restore, and Clear affects only that chat', async ({ page, request }) => {
  await page.goto('/');
  await request.post('/api/sessions/activate', { data: { session_id: 'chat-a' } });
  await request.post('/api/events', { data: report('chat-a', 'a.go', 'Update the handler', 'Keep validation at the boundary.') });
  await expect(page.getByRole('button', { name: /Update the handler/ })).toBeVisible();
  await page.getByRole('button', { name: /Update the handler/ }).click();
  await request.post('/api/sessions/activate', { data: { session_id: 'chat-b' } });
  await expect(page.getByTestId('workflow-step')).toHaveCount(0);
  await expect(page.getByRole('region', { name: 'Why this step' })).toHaveCount(0);
  await request.post('/api/events', { data: report('chat-b', 'b.go', 'Check the retry limit', 'Retries must stop eventually.') });
  await request.post('/api/sessions/activate', { data: { session_id: 'chat-a' } });
  await expect(page.getByRole('button', { name: /Update the handler/ })).toBeVisible();
  await expect(page.getByRole('region', { name: 'Why this step' })).toContainText('Keep validation at the boundary.');
  await page.getByRole('button', { name: 'Clear this chat’s workflow' }).click();
  await expect(page.getByTestId('workflow-step')).toHaveCount(0);
  await page.reload();
  await expect(page.getByText('Listening…')).toBeVisible();
  await request.post('/api/sessions/activate', { data: { session_id: 'chat-b' } });
  await expect(page.getByRole('button', { name: /Check the retry limit/ })).toBeVisible();
  await request.post('/api/events', { data: report('chat-a', 'fresh.go', 'Start a fresh step', 'Continue after clearing.') });
  await expect(page.getByTestId('workflow-step')).toHaveCount(1);
  await expect(page.getByRole('button', { name: /Check the retry limit/ })).toBeVisible();
  await request.post('/api/sessions/activate', { data: { session_id: 'chat-a' } });
  await expect(page.getByRole('button', { name: /Start a fresh step/ })).toBeVisible();
  await expect(page.getByTestId('workflow-arrow')).toHaveCount(0);
});

test('appearance changes the whole workflow and survives reload, chat switching and Clear', async ({ page, request }) => {
  const errors: string[] = []; page.on('pageerror', error => errors.push(error.message));
  await page.emulateMedia({ colorScheme: 'dark' });
  await page.goto('/?companion=1');
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark');
  await request.post('/api/sessions/activate', { data: { session_id: 'theme-chat-a' } });
  await request.post('/api/events', { data: report('theme-chat-a', 'entry.ts', 'Find the palette', 'The workflow shares its page colors.') });
  await request.post('/api/events', { data: report('theme-chat-a', 'palette.ts', 'Check readability', 'Keep labels and arrows visible in every appearance.', ['entry.ts']) });
  const selected = page.getByRole('button', { name: 'Check readability', exact: true });
  await selected.click();
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
  await request.post('/api/sessions/activate', { data: { session_id: 'theme-chat-b' } });
  await expect(page.getByText('Listening…')).toBeVisible();
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'comfort');
  await request.post('/api/events', { data: report('theme-chat-b', 'fresh.ts', 'Try a new chat', 'Appearance belongs to the view, not this chat.') });
  await expect(page.getByTestId('workflow-step')).toHaveCount(1);
  await page.getByRole('button', { name: 'Clear this chat’s workflow' }).click();
  await expect(page.getByText('Listening…')).toBeVisible();
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'comfort');
  await request.post('/api/sessions/activate', { data: { session_id: 'theme-chat-a' } });
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

test('milestones filter saved file noise, keep explanations and resume file capture without a backlog', async ({ page, request }) => {
  const session = 'updates-milestones';
  await page.goto('/?companion=1');
  await request.post('/api/sessions/activate', { data: { session_id: session } });
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
  await fileSteps.click();
  await page.getByRole('button', { name: 'Suggest', exact: true }).click();
  await page.getByRole('textbox', { name: 'Suggest something to the agent' }).fill('Please keep the preference shared.');

  await pickUpdates(page, 'milestones');
  await expect(page.getByTestId('workflow-step')).toHaveCount(1);
  await expect(page.getByRole('button', { name: 'Choose update frequency', exact: true })).toBeVisible();
  await expect(fileSteps).toHaveCount(0);
  await expect(page.getByRole('button', { name: 'Read context', exact: true })).toHaveCount(0);
  await expect(page.getByRole('region', { name: 'Why this step' })).toHaveCount(0);
  await writeFile(file, 'export const mode = "milestones";\n');
  await writeFile(resolve(root, 'updates-milestones-only.ts'), 'export const omitted = true;\n');
  const milestoneState = await (await request.post('/api/check-in', { data: { session_id: session, agent_id: 'test-agent', settle_ms: 300 } })).json();
  expect(milestoneState.update_mode).toBe('milestones');
  expect(milestoneState.pending_changes).toEqual([]);
  expect(milestoneState.pending_count).toBe(0);
  expect(milestoneState.events).toHaveLength(1);
  const ignored = await request.post('/api/events', { data: { ...report(session, original.node_name, 'File detail omitted', 'Milestone mode does not add per-file explanations.'), action: original.action, change_id: original.id } });
  expect((await ignored.json()).accepted).toBe(false);
  await request.post('/api/activity', { data: { session_id: session, agent_id: 'test-agent', phase: 'editing', node_name: 'updates-hook.ts' } });
  // An implementation milestone is valid even when its action is "modified".
  const milestone = await request.post('/api/events', { data: report(session, 'updates-settings.ts', 'Connect update preferences', 'Apply the same setting to automatic capture and agent reports.') });
  expect((await milestone.json()).accepted).toBe(true);
  await expect(page.getByTestId('workflow-step')).toHaveCount(2);
  await expect(page.getByTestId('workflow-arrow')).toHaveCount(1);

  await pickUpdates(page, 'files');
  await expect(page.getByTestId('workflow-step')).toHaveCount(4);
  await expect(fileSteps).toHaveAttribute('aria-pressed', 'true');
  await expect(page.getByRole('region', { name: 'Why this step' })).toContainText(explanation);
  await expect(page.getByRole('textbox', { name: 'Suggest something to the agent' })).toHaveValue('Please keep the preference shared.');
  await writeFile(file, 'export const mode = "resumed";\n');
  await expect(fileSteps).toHaveCount(2);
  const events = await savedEvents(request, session);
  const revisions = events.filter(event => event.source === 'watcher' && event.kind === 'file' && event.node_name === original.node_name);
  expect(revisions).toHaveLength(2);
  expect(revisions[1].before).toContain('"milestones"');
  expect(revisions[1].after).toContain('"resumed"');
  expect(events.some(event => event.node_name === 'updates-milestones-only.ts' || event.node_name === 'updates-hook.ts')).toBe(false);
  await fileSteps.last().click();
  await expect(page.getByRole('region', { name: 'Why this step' })).toContainText('Waiting for the agent’s explanation.');
  await expect(page.getByRole('region', { name: 'Why this step' })).not.toContainText(explanation);
});

test('Disabled stops recording and agent delivery while saved workflow returns when enabled', async ({ page, request }) => {
  const session = 'updates-disabled';
  await page.goto('/?companion=1');
  await request.post('/api/sessions/activate', { data: { session_id: session } });
  const root = await isolatedRoot(request);
  const file = resolve(root, 'updates-disabled-revision.ts');
  await writeFile(file, 'export const state = "enabled";\n');
  const fileSteps = page.locator('[data-testid="workflow-step"][title^="updates-disabled-revision.ts"]');
  await expect(fileSteps).toHaveCount(1);
  const original = (await savedEvents(request, session)).find(event => event.source === 'watcher' && event.kind === 'file' && event.node_name === 'updates-disabled-revision.ts')!;
  expect(original).toBeTruthy();
  await request.post('/api/events', { data: report(session, '@workflow', 'Keep the saved workflow', 'Disabling updates should preserve the work already explained.') });
  await page.getByRole('button', { name: 'Keep the saved workflow', exact: true }).click();
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
  await expect(page.getByTestId('workflow-step')).toHaveCount(2);
  await expect(page.getByRole('button', { name: 'Keep the saved workflow', exact: true })).toHaveAttribute('aria-pressed', 'true');
  await expect(page.getByRole('region', { name: 'Why this step' })).toContainText('Disabling updates should preserve the work already explained.');
  const resumed = await (await request.post('/api/check-in', { data: { session_id: session, agent_id: 'test-agent' } })).json();
  expect(resumed.feedback).toHaveLength(1);
  expect(resumed.feedback[0].text).toBe('Deliver this when updates are enabled.');
  await writeFile(file, 'export const state = "resumed";\n');
  await expect(fileSteps).toHaveCount(2);
  const events = await savedEvents(request, session);
  const revisions = events.filter(event => event.source === 'watcher' && event.kind === 'file' && event.node_name === original.node_name);
  expect(revisions).toHaveLength(2);
  expect(revisions[1].before).toContain('"disabled"');
  expect(revisions[1].after).toContain('"resumed"');
  expect(events.some(event => ['updates-disabled-only.ts', 'updates-disabled-hook.ts', 'updates-ignored.ts'].includes(event.node_name))).toBe(false);
});

test('Updates is shared between views and survives reload, appearance, chat switching and Clear', async ({ page, context, request }) => {
  await page.goto('/?companion=1');
  await request.post('/api/sessions/activate', { data: { session_id: 'updates-chat-a' } });
  await request.post('/api/events', { data: report('updates-chat-a', '@workflow', 'Review update settings', 'The preference belongs to this project rather than an individual chat.') });
  await expect(page.getByTestId('workflow-step')).toHaveCount(1);
  const second = await context.newPage();
  await second.goto('/?companion=1');
  await expect(second.getByTestId('workflow-step')).toHaveCount(1);
  await pickUpdates(page, 'milestones');
  await expect(second.locator('.stef-app')).toHaveAttribute('data-update-mode', 'milestones');
  expect(await (await request.get('/api/settings')).json()).toEqual({ update_mode: 'milestones' });
  await second.reload();
  await expect(second.locator('.stef-app')).toHaveAttribute('data-update-mode', 'milestones');
  await page.getByRole('button', { name: /appearance/i }).click();
  await page.getByRole('menuitemradio', { name: 'Light', exact: true }).click();
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'light');
  await expect(page.locator('.stef-app')).toHaveAttribute('data-update-mode', 'milestones');
  await request.post('/api/sessions/activate', { data: { session_id: 'updates-chat-b' } });
  await expect(page.getByText('Listening…')).toBeVisible();
  await expect(second.getByText('Listening…')).toBeVisible();
  await expect(page.locator('.stef-app')).toHaveAttribute('data-update-mode', 'milestones');
  await request.post('/api/events', { data: report('updates-chat-b', '@workflow', 'Start a separate chat', 'Each chat still keeps its own workflow history.') });
  await expect(page.getByTestId('workflow-step')).toHaveCount(1);
  await page.getByRole('button', { name: 'Clear this chat’s workflow' }).click();
  await expect(page.getByText('Listening…')).toBeVisible();
  await expect(second.getByText('Listening…')).toBeVisible();
  expect(await (await request.get('/api/settings')).json()).toEqual({ update_mode: 'milestones' });
  await request.post('/api/sessions/activate', { data: { session_id: 'updates-chat-a' } });
  await expect(page.getByRole('button', { name: 'Review update settings', exact: true })).toBeVisible();
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'light');
  await pickUpdates(second, 'disabled');
  await expect(page.locator('.stef-app')).toHaveAttribute('data-update-mode', 'disabled');
  await expect(page.getByText('Updates disabled', { exact: true })).toBeVisible();
  await pickUpdates(page, 'files');
  await expect(second.locator('.stef-app')).toHaveAttribute('data-update-mode', 'files');
  await expect(second.getByRole('button', { name: 'Review update settings', exact: true })).toBeVisible();
  await second.close();
});

test('the compact Updates menu explains token cost and handles keyboard, narrow layouts and save failures', async ({ page, request }) => {
  const errors: string[] = []; page.on('pageerror', error => errors.push(error.message));
  await page.goto('/?companion=1');
  await request.post('/api/sessions/activate', { data: { session_id: 'updates-menu' } });
  await request.post('/api/events', { data: report('updates-menu', '@workflow', 'Keep controls simple', 'Update frequency should take one small menu rather than a dashboard.') });
  const selected = page.getByRole('button', { name: 'Keep controls simple', exact: true });
  await selected.click();
  const trigger = page.getByRole('button', { name: 'Updates', exact: true });
  const menu = page.getByRole('menu', { name: 'Updates', exact: true });
  await trigger.click();
  await expect(menu.getByRole('menuitemradio')).toHaveCount(3);
  await expect(menu.getByText('Brief explanations at key steps.', { exact: true })).toBeVisible();
  await expect(menu.getByText('Per-file explanations may use more tokens.', { exact: true })).toBeVisible();
  await expect(menu.getByText('Stop new updates. Keep saved history.', { exact: true })).toBeVisible();
  const files = menu.getByRole('menuitemradio', { name: /^Every file/ });
  await expect(files).toBeFocused();
  await expect(files).toHaveAttribute('aria-checked', 'true');
  await page.keyboard.press('Home');
  await expect(menu.getByRole('menuitemradio', { name: /^Major milestones/ })).toBeFocused();
  await page.keyboard.press('End');
  await expect(menu.getByRole('menuitemradio', { name: /^Disabled/ })).toBeFocused();
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

  await page.route('**/api/settings', async route => {
    if (route.request().method() === 'POST') await route.fulfill({ status: 500, body: 'Setting could not be saved.' });
    else await route.continue();
  });
  await menu.getByRole('menuitemradio', { name: /^Major milestones/ }).click();
  await expect(page.getByRole('status')).toContainText('Could not change updates.');
  await expect(page.locator('.stef-app')).toHaveAttribute('data-update-mode', 'files');
  await expect(files).toHaveAttribute('aria-checked', 'true');
  await expect(menu.getByRole('menuitemradio', { name: /^Major milestones/ })).toBeEnabled();
  expect(await (await request.get('/api/settings')).json()).toEqual({ update_mode: 'files' });
  await page.unroute('**/api/settings');
  await menu.getByRole('menuitemradio', { name: /^Major milestones/ }).click();
  await expect(menu).toHaveCount(0);
  await expect(page.locator('.stef-app')).toHaveAttribute('data-update-mode', 'milestones');
  await expect(page.getByRole('status')).toHaveCount(0);
  expect(errors).toEqual([]);
});

test('file diffs keep exact additions, removals and AI explanations for each saved revision', async ({ page, request }) => {
  const errors: string[] = []; page.on('pageerror', error => errors.push(error.message));
  const session = 'file-diff-revisions';
  await page.goto('/?companion=1');
  await request.post('/api/sessions/activate', { data: { session_id: session } });
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
  await fileSteps.first().click();
  await showDiff.click();
  await expect(dialog).toBeVisible();
  await expect(dialog).toContainText('diff-revisions.txt');
  await expect(added).toHaveText(before);
  await expect(removed).toHaveCount(0);
  await expect(dialog.locator('[data-testid="diff-line"][data-kind="addition"] .line-number:nth-child(1)')).toHaveText(['', '', '']);
  await expect(dialog.locator('[data-testid="diff-line"][data-kind="addition"] .line-number:nth-child(2)')).toHaveText(['1', '2', '3']);
  await expect(dialog.getByRole('region', { name: 'Why this change' })).toContainText('Waiting for the agent’s explanation.');

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

  const modified = (await savedEvents(request, session)).filter(event => event.source === 'watcher' && event.kind === 'file' && event.node_name === created.node_name).at(-1)!;
  expect(modified.action).toBe('modified');
  await fileSteps.last().click();
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
  await fileSteps.first().click();
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
  await fileSteps.last().click();
  await showDiff.click();
  await expect(removed).toHaveText(after);
  await expect(added).toHaveCount(0);
  await expect(dialog.getByRole('region', { name: 'Why this change' })).toContainText(deletionReason);
  await page.screenshot({ path: '../artifacts/stef-file-diff-deleted.png', fullPage: true });
  // A mode changed in another view closes this dialog without reopening it later.
  for (const mode of ['milestones', 'disabled']) {
    await request.post('/api/settings', { data: { update_mode: mode } });
    await expect(page.locator('.stef-app')).toHaveAttribute('data-update-mode', mode);
    await expect(dialog).toHaveCount(0);
    await request.post('/api/settings', { data: { update_mode: 'files' } });
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
  await request.post('/api/sessions/activate', { data: { session_id: session } });
  const root = await isolatedRoot(request);
  const file = resolve(root, 'diff-source.ts');
  const fileSteps = page.locator('[data-testid="workflow-step"][title^="diff-source.ts"]');
  const source = 'export const markup = \'<img src=x onerror="window.stefDiffInjected=true">\';';
  const longLine = 'export const message = "' + 'Keep source text exact. '.repeat(18) + '";';
  await writeFile(file, 'export const markup = "before";\n');
  await expect(fileSteps).toHaveCount(1);
  await writeFile(file, source + '\n' + longLine + '\n');
  await expect(fileSteps).toHaveCount(2);
  await fileSteps.last().click();
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
  await request.post('/api/sessions/activate', { data: { session_id: session } });
  const root = await isolatedRoot(request);
  const file = resolve(root, 'diff-empty.txt');
  const fileSteps = page.locator('[data-testid="workflow-step"][title^="diff-empty.txt"]');
  const dialog = page.getByRole('dialog', { name: 'File diff', exact: true });
  await writeFile(file, '');
  await expect(fileSteps).toHaveCount(1);
  await fileSteps.first().click();
  await page.getByRole('button', { name: 'Show diff', exact: true }).click();
  await expect(dialog).toContainText('Empty file created.');
  await expect(dialog.locator('[data-testid="diff-line"][data-kind="addition"], [data-testid="diff-line"][data-kind="removal"]')).toHaveCount(0);
  await dialog.getByRole('button', { name: 'Close diff', exact: true }).click();
  await unlink(file);
  await expect(fileSteps).toHaveCount(2);
  await fileSteps.last().click();
  await page.getByRole('button', { name: 'Show diff', exact: true }).click();
  await expect(dialog).toContainText('Empty file deleted.');
  await expect(dialog.locator('[data-testid="diff-line"][data-kind="addition"], [data-testid="diff-line"][data-kind="removal"]')).toHaveCount(0);
  await dialog.getByRole('button', { name: 'Close diff', exact: true }).click();
  await request.post('/api/events', { data: report(session, '@workflow', 'Review the workflow', 'A planning milestone does not invent a saved-file diff.') });
  await page.getByRole('button', { name: 'Review the workflow', exact: true }).click();
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
  await page.goto('/');
  await page.getByTestId('workflow-step').click();
  await page.getByRole('button', { name: 'Open main.go' }).click();
  await expect.poll(() => page.evaluate(() => (window as typeof window & { stefMessages: { type: string }[] }).stefMessages.some(message => message.type === 'stef.openFile'))).toBe(true);
  expect(sockets).toBe(0);
});
