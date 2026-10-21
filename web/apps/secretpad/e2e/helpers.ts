import { expect, type Page } from '@playwright/test';

export const E2E_USER = process.env.E2E_USER || 'admin';
export const E2E_PASSWORD = process.env.E2E_PASSWORD || '12345678';

/** Pins the UI locale to English for deterministic label/text matching. */
export async function useEnglish(page: Page): Promise<void> {
  await page.context().addInitScript(() => {
    localStorage.setItem('secretpad-locale', 'en-US');
  });
}

/**
 * Browser login (cookie session + CSRF): the backend sets the HttpOnly
 * privahub_session cookie and the readable privahub_csrf cookie; no token is
 * kept in page storage. CENTER admins land on the Getting Started guide on
 * first login and on the dashboard afterwards.
 */
export async function login(page: Page): Promise<void> {
  await page.goto('/');
  await expect(page).toHaveURL(/\/login/);
  await expect(page.locator('h2')).toContainText('SecretPad');
  await page.getByLabel('Username').fill(E2E_USER);
  await page.getByLabel('Password').fill(E2E_PASSWORD);
  await page.getByRole('button', { name: /Sign In/i }).click();
  await expect(page).toHaveURL(/\/(guide|dashboard|workbench)(\?|$)/);

  const cookies = await page.context().cookies();
  const session = cookies.find((c) => c.name === 'privahub_session');
  expect(session, 'session cookie').toBeTruthy();
  expect(session?.httpOnly, 'session cookie is HttpOnly').toBe(true);
  expect(cookies.some((c) => c.name === 'privahub_csrf'), 'CSRF cookie').toBe(true);
}

/** Opens a page from the sidebar (labels are "<icon> <name>"). */
export async function openFromSidebar(page: Page, name: string): Promise<void> {
  await page.getByRole('link', { name: new RegExp(`(^|\\s)${name}$`) }).click();
}
