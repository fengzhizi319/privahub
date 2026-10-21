import * as Sentry from '@sentry/react';

/**
 * Sentry 错误监控与性能追踪初始化模块。
 *
 * 设计目标：
 * 1. **安全降级**：未配置 DSN 时（本地开发 / 私有化部署无可观测后端），
 *    Sentry 自动以 no-op 模式运行，不发送任何数据、不产生运行时开销。
 * 2. **构建期注入**：DSN、采样率、版本号均通过 `VITE_*` 环境变量在
 *    构建时注入，避免在源码中硬编码任何敏感凭据。
 * 3. **与 React 深度集成**：导出 `SentryErrorBoundary`，可替代或包裹
 *    应用自有的 ErrorBoundary，自动捕获渲染期异常并附带组件栈。
 */

/**
 * 解析采样率环境变量为数值。
 * - 非法 / 缺失时回退到默认值 0.1（10%），保证 tracing 始终有界。
 * - 取值范围被 clamp 到 [0, 1]，防止配置错误导致全量或负采样。
 *
 * @param raw 环境变量原始字符串（可能为 undefined）
 * @param fallback 缺省采样率
 * @returns 归一化后的采样率数值
 */
function parseSampleRate(raw: string | undefined, fallback: number): number {
  if (!raw) return fallback;
  const value = Number.parseFloat(raw);
  if (Number.isNaN(value)) return fallback;
  return Math.min(1, Math.max(0, value));
}

/**
 * 初始化 Sentry。
 *
 * 应在应用入口（main.tsx）尽早调用，且必须在 React 渲染之前完成，
 * 以确保首屏渲染异常也能被捕获上报。
 *
 * @returns 是否真正启用了上报（即配置了有效 DSN）。
 */
export function initSentry(): boolean {
  const dsn = import.meta.env.VITE_SENTRY_DSN;

  // 未配置 DSN：显式以 enabled=false 初始化，保持 SDK 处于已知的禁用态，
  // 这样即便业务代码误调用 Sentry.captureException 也不会抛错。
  if (!dsn) {
    Sentry.init({ enabled: false });
    return false;
  }

  Sentry.init({
    dsn,
    // 应用版本号，用于在 Sentry 后台按 release 归因问题、做版本回归对比。
    release: import.meta.env.VITE_APP_VERSION || undefined,
    // 性能追踪采样率；错误事件不受此影响，始终全量上报。
    tracesSampleRate: parseSampleRate(
      import.meta.env.VITE_SENTRY_TRACES_SAMPLE_RATE,
      0.1
    ),
    // 浏览器性能追踪集成（路由切换 / 资源加载等自动埋点）。
    integrations: [Sentry.browserTracingIntegration()],
    // 仅在构建产物中开启调试日志，开发期保持安静。
    debug: false,
  });

  return true;
}

/**
 * 主动上报一条异常（用于命令式代码 / 非渲染期的 try-catch 场景）。
 * 未启用 Sentry 时为安全的空操作。
 *
 * @param error 需要上报的错误对象或消息
 * @param context 附加的结构化上下文，会作为 tags/extra 一并上报
 */
export function captureException(
  error: unknown,
  context?: Record<string, unknown>
): void {
  Sentry.captureException(error, { extra: context });
}

/**
 * 由 Sentry 提供的 React 错误边界。
 * 在应用顶层包裹后，渲染期异常会自动上报并展示 fallback。
 */
export const SentryErrorBoundary = Sentry.ErrorBoundary;
