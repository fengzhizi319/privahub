import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';

// mock @sentry/react，避免测试真正发起网络上报，同时可断言调用参数。
vi.mock('@sentry/react', () => ({
  init: vi.fn(),
  captureException: vi.fn(),
  browserTracingIntegration: vi.fn(() => ({ name: 'BrowserTracing' })),
  ErrorBoundary: vi.fn(),
}));

import * as Sentry from '@sentry/react';

import { initSentry, captureException } from './sentry';

/**
 * sentry.ts 单元测试。
 *
 * 重点验证「安全降级」契约：
 * - 未配置 DSN 时以 enabled=false 初始化并返回 false（不发送任何数据）。
 * - 配置 DSN 时正确透传 release / tracesSampleRate，并返回 true。
 * - 采样率解析对非法值 / 越界值的容错。
 */

beforeEach(() => {
  vi.clearAllMocks();
});

afterEach(() => {
  // 清理每个用例注入的环境变量，避免相互污染。
  delete (import.meta.env as Record<string, unknown>).VITE_SENTRY_DSN;
  delete (import.meta.env as Record<string, unknown>).VITE_SENTRY_TRACES_SAMPLE_RATE;
  delete (import.meta.env as Record<string, unknown>).VITE_APP_VERSION;
});

describe('initSentry', () => {
  it('未配置 DSN 时禁用上报并返回 false', () => {
    const enabled = initSentry();

    expect(enabled).toBe(false);
    expect(Sentry.init).toHaveBeenCalledWith({ enabled: false });
  });

  it('配置 DSN 时启用上报并返回 true', () => {
    (import.meta.env as Record<string, unknown>).VITE_SENTRY_DSN =
      'https://public@example.ingest.sentry.io/1';

    const enabled = initSentry();

    expect(enabled).toBe(true);
    expect(Sentry.init).toHaveBeenCalledWith(
      expect.objectContaining({
        dsn: 'https://public@example.ingest.sentry.io/1',
        // 未显式配置采样率时回退到默认 0.1。
        tracesSampleRate: 0.1,
      })
    );
  });

  it('透传 release 版本号与自定义采样率', () => {
    const env = import.meta.env as Record<string, unknown>;
    env.VITE_SENTRY_DSN = 'https://public@example.ingest.sentry.io/1';
    env.VITE_APP_VERSION = 'v1.2.3';
    env.VITE_SENTRY_TRACES_SAMPLE_RATE = '0.5';

    initSentry();

    expect(Sentry.init).toHaveBeenCalledWith(
      expect.objectContaining({ release: 'v1.2.3', tracesSampleRate: 0.5 })
    );
  });

  it('非法采样率回退到默认值，越界值被 clamp 到 [0,1]', () => {
    const env = import.meta.env as Record<string, unknown>;
    env.VITE_SENTRY_DSN = 'https://public@example.ingest.sentry.io/1';

    // 非法字符串 → 默认 0.1。
    env.VITE_SENTRY_TRACES_SAMPLE_RATE = 'not-a-number';
    initSentry();
    expect(Sentry.init).toHaveBeenLastCalledWith(
      expect.objectContaining({ tracesSampleRate: 0.1 })
    );

    // 大于 1 → clamp 到 1。
    env.VITE_SENTRY_TRACES_SAMPLE_RATE = '5';
    initSentry();
    expect(Sentry.init).toHaveBeenLastCalledWith(
      expect.objectContaining({ tracesSampleRate: 1 })
    );

    // 小于 0 → clamp 到 0。
    env.VITE_SENTRY_TRACES_SAMPLE_RATE = '-3';
    initSentry();
    expect(Sentry.init).toHaveBeenLastCalledWith(
      expect.objectContaining({ tracesSampleRate: 0 })
    );
  });
});

describe('captureException', () => {
  it('将错误与上下文透传给 Sentry.captureException', () => {
    const err = new Error('boom');
    captureException(err, { scope: 'test' });

    expect(Sentry.captureException).toHaveBeenCalledWith(err, {
      extra: { scope: 'test' },
    });
  });
});
