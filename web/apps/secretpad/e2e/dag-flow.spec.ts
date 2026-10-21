import { test, expect } from '@playwright/test';
import { login, openFromSidebar, useEnglish } from './helpers';

/**
 * P0 critical path: DAG canvas smoke test.
 * Requires backend on port 8080 and default dev account.
 */
test.describe('SecretPad P0 DAG Canvas', () => {
  test.beforeEach(async ({ page }) => {
    await useEnglish(page);
  });

  test('navigates to DAG page after login', async ({ page }) => {
    await login(page);

    await openFromSidebar(page, 'DAG Canvas');
    await expect(page.locator('h2')).toContainText('DAG Pipeline Editor');
  });

  test('renders DAG canvas or empty state', async ({ page }) => {
    await login(page);
    await openFromSidebar(page, 'DAG Canvas');

    // Either the workspace renders (top-bar node/edge counter), or the empty state card is shown.
    await expect(
      page
        .getByText(/^\d+ nodes · \d+ edges$/)
        .or(page.getByText('No DAG graph in current project'))
    ).toBeVisible();
  });

  /**
   * 核心回归场景：从左侧组件库拖一个算子到画布构建任务。
   * 曾因 (1) 画布被渲染为只读回退（draggable=false）、
   * (2) 拖入节点被父组件重渲染立即重置，而线上失效。
   */
  test('drags an operator from the library onto the canvas', async ({ page }) => {
    await login(page);
    await openFromSidebar(page, 'DAG Canvas');

    // 等待页面进入「有图」或「无图」任一状态。
    const counter = page.getByText(/^\d+ nodes · \d+ edges$/);
    const noGraph = page.getByText('No DAG graph in current project');
    await expect(counter.or(noGraph)).toBeVisible();
    // 图列表加载期间也会短暂显示空态卡片，等待其稳定后再决定是否真的需要建图，
    // 否则建图触发的列表刷新会在拖拽中途切换选中图，造成计数断言抖动。
    await page.waitForTimeout(1000);
    if ((await noGraph.isVisible()) && !(await counter.isVisible())) {
      await page.getByRole('button', { name: 'New Graph' }).click();
      await page
        .getByPlaceholder(/Risk Model Joint Training DAG/i)
        .fill(`E2E Drag Graph ${Date.now()}`);
      await page.getByRole('button', { name: 'Create', exact: true }).click();
    }

    // 可编辑画布检查点：组件条目可拖拽（readOnly 回退时为 draggable=false），
    // 且保存按钮存在（readOnly 回退不渲染 Save）。
    const firstOperator = page.locator('[draggable="true"]').first();
    await expect(firstOperator).toBeVisible();
    await expect(page.getByRole('button', { name: /Save/ })).toBeVisible();

    const beforeText = (await counter.textContent()) || '0 nodes';
    const before = parseInt(beforeText.match(/^(\d+) nodes/)?.[1] || '0', 10);

    // HTML5 拖放：组件库 → 画布
    const canvas = page.locator('svg.pointer-events-none').locator('..');
    await firstOperator.dragTo(canvas, { targetPosition: { x: 320, y: 220 } });

    // 节点数 +1
    await expect(counter).toHaveText(new RegExp(`^${before + 1} nodes ·`));

    // 节点须稳定存在：等待页面后续重渲染后不被重置回服务端状态
    await page.waitForTimeout(800);
    await expect(counter).toHaveText(new RegExp(`^${before + 1} nodes ·`));
  });
});
