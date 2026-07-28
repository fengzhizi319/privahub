import { test, expect } from '@playwright/test';

/**
 * P0 critical path: DAG canvas smoke test.
 * Requires backend on port 8080 and default dev account.
 */
test.describe('SecretPad P0 DAG Canvas', () => {
  test.beforeEach(async ({ page }) => {
    await page.context().addInitScript(() => {
      localStorage.setItem('secretpad-locale', 'en-US');
    });
  });

  test('navigates to DAG page after login', async ({ page }) => {
    await page.goto('/');
    await page.getByLabel('Username').fill('admin');
    await page.getByLabel('Password').fill('12345678');
    await page.getByRole('button', { name: /Sign In/i }).click();

    await expect(page.locator('h1')).toContainText('Console Dashboard');

    await page.getByRole('button', { name: /DAG Canvas/i }).click();
    await expect(page.locator('h2')).toContainText('DAG Pipeline Editor');
  });

  test('renders DAG canvas or empty state', async ({ page }) => {
    await page.goto('/');
    await page.getByLabel('Username').fill('admin');
    await page.getByLabel('Password').fill('12345678');
    await page.getByRole('button', { name: /Sign In/i }).click();

    await expect(page.locator('h1')).toContainText('Console Dashboard');
    await page.getByRole('button', { name: /DAG Canvas/i }).click();

    // Either the workspace renders with operator library, or the empty state card is shown.
    await expect(
      page.getByText('Operator Library').or(page.getByText('No DAG graph in current project'))
    ).toBeVisible();
  });
});
