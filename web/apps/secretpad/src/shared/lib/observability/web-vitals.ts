import { onCLS, onINP, onLCP, type Metric } from 'web-vitals';

import { captureException } from './sentry';

/**
 * Web Vitals 核心性能指标采集与上报模块。
 *
 * 采集的指标（Core Web Vitals + 关键补充）：
 * - **CLS**（Cumulative Layout Shift）：累计布局偏移，衡量视觉稳定性。
 * - **INP**（Interaction to Next Paint）：交互到下一次绘制，衡量响应性
 *   （已取代 FID 成为 Google 核心指标）。
 * - **LCP**（Largest Contentful Paint）：最大内容绘制，衡量加载性能。
 *
 * 上报策略：
 * 1. 开发环境（DEV）：以 `console.info` 输出到控制台，便于本地调优，
 *    不产生任何网络请求。
 * 2. 生产环境：通过 `navigator.sendBeacon` 上报到 `/api/v1alpha1/v1/metrics`
 *    （如后端提供该端点）；同时把「较差」的指标作为 breadcrumb/异常
 *    线索交给 Sentry，便于与错误关联分析。
 * 3. 所有上报均为「尽力而为」，任何失败都被静默吞掉，绝不影响业务。
 */

/**
 * 各指标的「良好 / 需改进」阈值（毫秒或无量纲），参考 web.dev 标准。
 * 超过 needs-improvement 阈值的指标会被视为「较差」并额外标记上报。
 */
const POOR_THRESHOLDS: Record<string, number> = {
  CLS: 0.25, // 无单位，>0.25 视为较差
  INP: 500, // 毫秒，>500ms 视为较差
  LCP: 4000, // 毫秒，>4s 视为较差
};

/**
 * 将单条指标上报到后端（尽力而为）。
 * 使用 sendBeacon 以保证在页面卸载时也能可靠送达，且不阻塞主线程。
 *
 * @param metric web-vitals 提供的指标对象
 */
function reportToBackend(metric: Metric): void {
  // 仅在浏览器支持 sendBeacon 时尝试上报，否则直接放弃（降级）。
  if (typeof navigator === 'undefined' || !navigator.sendBeacon) return;

  const payload = JSON.stringify({
    name: metric.name,
    value: metric.value,
    rating: metric.rating, // 'good' | 'needs-improvement' | 'poor'
    delta: metric.delta,
    id: metric.id,
    navigationType: metric.navigationType,
  });

  try {
    // 端点可由后端按需实现；不存在时 sendBeacon 仅静默失败，无副作用。
    navigator.sendBeacon(
      '/api/v1alpha1/v1/metrics',
      new Blob([payload], { type: 'application/json' })
    );
  } catch {
    // 上报失败不应影响用户体验，静默忽略。
  }
}

/**
 * 统一处理每一条采集到的指标：控制台输出 + 生产上报 + 较差指标告警。
 *
 * @param metric web-vitals 指标对象
 */
function handleMetric(metric: Metric): void {
  // 开发期可视化，方便本地性能调优。
  if (import.meta.env.DEV) {
    console.info(
      `[WebVitals] ${metric.name} = ${metric.value.toFixed(2)} (${metric.rating})`
    );
  }

  // 生产环境上报到后端。
  if (import.meta.env.PROD) {
    reportToBackend(metric);
  }

  // 对「较差」的指标额外记录一条异常线索，便于在 Sentry 中关联分析。
  const threshold = POOR_THRESHOLDS[metric.name];
  if (threshold !== undefined && metric.value > threshold) {
    captureException(
      new Error(`[WebVitals] Poor ${metric.name}: ${metric.value.toFixed(2)}`),
      { metricName: metric.name, rating: metric.rating, value: metric.value }
    );
  }
}

/**
 * 注册所有 Web Vitals 采集器。
 * 应在应用入口调用一次；web-vitals 内部会在指标「最终确定」时回调，
 * 因此无需手动处理页面卸载时机。
 */
export function reportWebVitals(): void {
  onCLS(handleMetric);
  onINP(handleMetric);
  onLCP(handleMetric);
}
