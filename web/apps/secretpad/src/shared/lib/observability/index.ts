/**
 * 可观测性（Observability）模块统一出口。
 *
 * 聚合错误监控（Sentry）与性能指标（Web Vitals）两类能力，
 * 供应用入口（main.tsx）按需引入，避免业务代码直接依赖具体 SDK。
 */
export {
  initSentry,
  captureException,
  SentryErrorBoundary,
} from './sentry';
export { reportWebVitals } from './web-vitals';
