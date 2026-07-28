import { test, expect } from '@playwright/test';

/**
 * P0 critical path: login + landing page smoke test.
 * Requires backend on port 8080 (proxied by Vite) and default dev account.
 */
test.describe('SecretPad P0 Login & Dashboard', () => {
  test.beforeEach(async ({ page }) => {
    // Pin locale to English for deterministic label/text matching.
    await page.context().addInitScript(() => {
      localStorage.setItem('secretpad-locale', 'en-US');
    });
  });

  test('logs in with default dev account and lands on dashboard', async ({ page }) => {
    await page.goto('/');

    // Login page renders
    await expect(page.locator('h2')).toContainText('SecretPad');

    // Fill credentials
    await page.getByLabel('Username').fill('admin');
    await page.getByLabel('Password').fill('12345678');
    await page.getByRole('button', { name: /Sign In/i }).click();

    // After login, dashboard is visible
    await expect(page.locator('h1')).toContainText('Console Dashboard');

    // Sidebar navigation is visible
    await expect(page.getByRole('button', { name: '📊 Dashboard' })).toBeVisible();
    await expect(page.getByRole('button', { name: '📁 Projects' })).toBeVisible();
    await expect(page.getByRole('button', { name: '🖥️ Nodes' })).toBeVisible();
  });

  test('navigates to Nodes page after login', async ({ page }) => {
    await page.goto('/');
    await page.getByLabel('Username').fill('admin');
    await page.getByLabel('Password').fill('12345678');
    await page.getByRole('button', { name: /Sign In/i }).click();

    // Wait for dashboard
    await expect(page.locator('h1')).toContainText('Console Dashboard');

    // Click Nodes in sidebar
    await page.getByRole('button', { name: /Nodes/i }).click();
    await expect(page.locator('h2')).toContainText('Kuscia Node Cluster');
  });

  test('navigates to Messages page after login', async ({ page }) => {
    await page.goto('/');
    await page.getByLabel('Username').fill('admin');
    await page.getByLabel('Password').fill('12345678');
    await page.getByRole('button', { name: /Sign In/i }).click();

    // Wait for dashboard
    await expect(page.locator('h1')).toContainText('Console Dashboard');

    // Click Messages in sidebar
    await page.getByRole('button', { name: /Messages/i }).click();
    await expect(page.locator('h2')).toContainText('Message & Approval Center');
  });
});