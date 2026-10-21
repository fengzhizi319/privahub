import { test, expect } from '@playwright/test';
import { login, openFromSidebar, useEnglish } from './helpers';

/**
 * P0 critical path: login (cookie session) + landing page smoke test.
 * Requires backend on port 8080 (proxied by Vite) and default dev account.
 */
test.describe('SecretPad P0 Login & Dashboard', () => {
  test.beforeEach(async ({ page }) => {
    await useEnglish(page);
  });

  test('logs in with default dev account and opens the dashboard', async ({ page }) => {
    await login(page);

    // Landing page: Getting Started guide (first login) or dashboard.
    await expect(page.locator('h1')).toContainText(/Getting Started|Console Dashboard/);

    // Sidebar navigation is visible and translated (no raw i18n keys).
    const sidebar = page.getByRole('link', { name: /Dashboard$/ });
    await expect(sidebar).toBeVisible();
    await expect(page.getByRole('link', { name: /Projects$/ })).toBeVisible();
    await expect(page.getByRole('link', { name: /(^|\s)Nodes$/ })).toBeVisible();
    await expect(page.getByRole('link', { name: /Privacy Scenes$/ })).toBeVisible();
    await expect(page.getByRole('link', { name: /sidebar\./ })).toHaveCount(0);

    await openFromSidebar(page, 'Dashboard');
    await expect(page).toHaveURL(/\/dashboard/);
    await expect(page.locator('h1')).toContainText('Console Dashboard');
  });

  test('navigates to Nodes page after login', async ({ page }) => {
    await login(page);
    await openFromSidebar(page, 'Nodes');
    await expect(page.locator('h2').first()).toContainText('Kuscia Node Cluster');
  });

  test('navigates to Messages page after login', async ({ page }) => {
    await login(page);
    await openFromSidebar(page, 'Messages');
    await expect(page.locator('h2')).toContainText('Message & Approval Center');
  });

  test('keeps the session across a reload and rejects anonymous access after logout', async ({
    page,
  }) => {
    await login(page);
    await page.goto('/dashboard');
    await page.reload();
    await expect(page.locator('h1')).toContainText('Console Dashboard');

    await page.context().clearCookies();
    await page.goto('/dashboard');
    await expect(page).toHaveURL(/\/login/);
  });
});
