import { defineConfig, devices } from '@playwright/test';

/**
 * Playwright E2E configuration for SecretPad new frontend.
 * Runs against the local Vite dev server (default http://127.0.0.1:8000).
 * Make sure the backend is running on port 8080 and `pnpm dev` is started
 * before executing E2E tests, or use the `webServer` auto-launch block below.
 */
export default defineConfig({
  testDir: './e2e',
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 2 : 0,
  workers: process.env.CI ? 1 : undefined,
  reporter: 'list',
  use: {
    baseURL: process.env.E2E_BASE_URL || 'http://127.0.0.1:8000',
    trace: 'on-first-retry',
    screenshot: 'only-on-failure',
  },
  projects: [
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'] },
    },
  ],
  // Auto-start the Vite dev server for E2E if E2E_START_SERVER is set.
  // In CI you may prefer to start the server externally.
  webServer: process.env.E2E_START_SERVER
    ? {
        command: 'pnpm dev',
        url: 'http://127.0.0.1:8000',
        reuseExistingServer: !process.env.CI,
        timeout: 120000,
      }
    : undefined,
});
