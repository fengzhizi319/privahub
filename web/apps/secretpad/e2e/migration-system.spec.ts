import fs from 'node:fs';
import path from 'node:path';
import { test, expect, type Page } from '@playwright/test';
import { login, useEnglish } from './helpers';

/** Real backend, cookie/CSRF session and disposable database provided by
 * scripts/run_system_e2e.py. No intercepted API responses or persisted tokens. */
async function command(page: Page, endpoint: string, body: unknown = {}) {
  const csrf = (await page.context().cookies()).find((cookie) => cookie.name === 'privahub_csrf')?.value;
  const response = await page.request.post(`/api/v1alpha1/${endpoint}`, {
    headers: { 'X-CSRF-Token': csrf || '', Origin: new URL(page.url()).origin }, data: body,
  });
  expect(response.ok(), endpoint).toBe(true);
  const envelope = await response.json();
  expect(envelope.status.code, `${endpoint}: ${envelope.status.msg}`).toBe(0);
  return envelope.data;
}

async function screenshot(page: Page, name: string) {
  const directory = process.env.E2E_ARTIFACT_DIR;
  if (!directory) return;
  fs.mkdirSync(directory, { recursive: true });
  await page.screenshot({ path: path.join(directory, `${name}.png`), fullPage: true });
}

test.describe('Migration system flows', () => {
  test.beforeEach(async ({ page }) => {
    await page.context().addInitScript(() => {
      for (const key of ['ProjectListTour', 'CreateProjectTour', 'DatatableAuthTour', 'DAGGuideTourOne', 'DAGGuideTourTwo', 'RecordGuideTour']) localStorage.setItem(key, '1');
    });
    await useEnglish(page);
    await login(page);
  });

  test('project → graph → save → reload → delete, through a real session', async ({ page }) => {
    const created = await command(page, 'project/create', {
      name: 'Clinical research collaboration', description: 'A secure workspace for collaborative analysis.', computeMode: 'MPC',
    });
    const projectId = created.projectId;
    let graphId: string | undefined;
    try {
      await page.goto('/projects');
      await expect(page.getByRole('heading', { name: 'Clinical research collaboration' })).toBeVisible();
      await screenshot(page, 'projects-desktop');
      const graph = await command(page, 'graph/create', { projectId, name: 'Cohort analysis' });
      graphId = graph.graphId;
      await page.goto(`/dag?projectId=${projectId}&graphId=${graphId}`);
      await expect(page.getByText(/^\d+ nodes · \d+ edges$/)).toBeVisible();
      await expect(page.getByRole('button', { name: /Save/ })).toBeVisible();
      await page.getByRole('button', { name: /Save/ }).click();
      await page.reload();
      await expect(page.getByText(/^\d+ nodes · \d+ edges$/)).toBeVisible();
      await screenshot(page, 'dag-desktop');
    } finally {
      if (graphId) await command(page, 'graph/delete', { projectId, graphId });
      await command(page, 'project/delete', { projectId });
    }
    await page.goto('/projects');
    await expect(page.getByRole('heading', { name: 'Clinical research collaboration' })).toHaveCount(0);
  });

  test('every CENTER business page renders without a JavaScript crash', async ({ page }) => {
    const crashes: string[] = [];
    page.on('pageerror', (error) => crashes.push(error.message));
    for (const route of ['/dashboard', '/projects', '/nodes', '/data-sources', '/data-tables',
      '/graphs', '/job-records', '/models', '/results', '/periodic-tasks', '/messages',
      '/node-routes', '/institutions', '/cloud-logs', '/feature-datasource',
      '/component-versions', '/privacy-scenes', '/account', '/audit']) {
      await page.goto(route);
      await expect(page.locator('main')).toBeVisible();
      await expect(page.locator('main')).not.toBeEmpty();
      await expect(page.getByText('Page failed to load', { exact: true })).toHaveCount(0);
    }
    expect(crashes).toEqual([]);
  });

  test('mobile navigation is keyboard accessible and content fits the viewport', async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 });
    await page.goto('/projects');
    await page.getByRole('button', { name: 'Open navigation' }).click();
    const navigation = page.getByRole('dialog', { name: 'Main navigation' });
    await expect(navigation).toBeVisible();
    await screenshot(page, 'navigation-mobile');
    await page.keyboard.press('Escape');
    await expect(navigation).not.toBeVisible();
    await page.getByRole('button', { name: 'Open navigation' }).click();
    await navigation.getByRole('link', { name: /Messages$/ }).click();
    await expect(navigation).not.toBeVisible();
    await expect(page).toHaveURL(/\/messages/);
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
    await screenshot(page, 'messages-mobile');
    await page.setViewportSize({ width: 1440, height: 1000 });
    await page.goto('/projects');
    await page.getByTitle('Toggle Theme').click();
    await expect(page.locator('html')).toHaveClass(/dark/);
    const card = page.locator('[data-tour="project-list"] > div').first();
    await expect(card).toHaveCSS('background-color', 'rgb(17, 24, 39)');
    await screenshot(page, 'projects-dark');
  });
});
