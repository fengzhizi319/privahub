import { describe, it, expect, vi, beforeEach } from 'vitest';
import type { Metric } from 'web-vitals';

// mock ./sentry，隔离对 Sentry 的依赖，仅断言较差指标是否触发上报。
vi.mock('./sentry', () => ({
  captureException: vi.fn(),
}));

// mock web-vitals，捕获各指标的回调函数，便于在测试中手动触发。
vi.mock('web-vitals', () => ({
  onCLS: vi.fn(),
  onINP: vi.fn(),
  onLCP: vi.fn(),
}));

import { onCLS, onINP, onLCP } from 'web-vitals';

import { reportWebVitals } from './web-vitals';
import { captureException } from './sentry';

/**
 * web-vitals.ts 单元测试。
 *
 * 通过 mock web-vitals 捕获注册的回调，再手动以不同数值触发，验证：
 * - reportWebVitals 正确注册了 CLS / INP / LCP 三个采集器；
 * - 「较差」指标（超过阈值）会触发 Sentry 上报；
 * - 「良好」指标不会触发上报。
 */

/** 构造一个最小可用的 Metric 对象。 */
const makeMetric = (name: string, value: number): Metric =>
  ({
    name,
    value,
    rating: 'good',
    delta: value,
    id: `${name}-id`,
    navigationType: 'navigate',
  }) as Metric;

/** 取出某个 onXXX mock 注册的第一个回调（类型为指标处理函数）。 */
const callbackOf = (spy: unknown) =>
  vi.mocked(spy as (cb: (metric: Metric) => void) => void).mock.calls[0][0];

beforeEach(() => {
  vi.clearAllMocks();
});

describe('reportWebVitals', () => {
  it('注册 CLS / INP / LCP 三个采集器', () => {
    reportWebVitals();

    expect(onCLS).toHaveBeenCalledTimes(1);
    expect(onINP).toHaveBeenCalledTimes(1);
    expect(onLCP).toHaveBeenCalledTimes(1);
  });

  it('较差的 LCP（>4000ms）触发 Sentry 上报', () => {
    reportWebVitals();
    const onLcp = callbackOf(onLCP);

    onLcp(makeMetric('LCP', 5000));

    expect(captureException).toHaveBeenCalledTimes(1);
    expect(captureException).toHaveBeenCalledWith(
      expect.objectContaining({ message: expect.stringContaining('Poor LCP') }),
      expect.objectContaining({ metricName: 'LCP', value: 5000 })
    );
  });

  it('良好的 LCP（<=4000ms）不触发上报', () => {
    reportWebVitals();
    const onLcp = callbackOf(onLCP);

    onLcp(makeMetric('LCP', 2000));

    expect(captureException).not.toHaveBeenCalled();
  });

  it('较差的 CLS（>0.25）触发上报', () => {
    reportWebVitals();
    const onCls = callbackOf(onCLS);

    onCls(makeMetric('CLS', 0.4));

    expect(captureException).toHaveBeenCalledTimes(1);
  });

  it('较差的 INP（>500ms）触发上报', () => {
    reportWebVitals();
    const onInp = callbackOf(onINP);

    onInp(makeMetric('INP', 600));

    expect(captureException).toHaveBeenCalledTimes(1);
  });
});
