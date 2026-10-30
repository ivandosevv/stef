import { defineConfig } from '@playwright/test';
export default defineConfig({
  testDir: './e2e',
  fullyParallel: false,
  workers: 1,
  forbidOnly: Boolean(process.env.CI),
  retries: 0,
  reporter: 'list',
  use: { baseURL: 'http://127.0.0.1:4821', viewport: { width: 1440, height: 1080 }, screenshot: 'only-on-failure', trace: 'retain-on-failure' },
  webServer: { command: 'bash ../scripts/e2e-server.sh', url: 'http://127.0.0.1:4821/healthz', reuseExistingServer: false, timeout: 120000 },
});
