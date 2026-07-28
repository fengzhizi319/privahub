/// <reference types="vite/client" />

/**
 * Vite 客户端环境变量的类型声明。
 *
 * 说明：
 * - Vite 默认仅将 `import.meta.env` 暴露为 `Record<string, any>`，
 *   这里通过扩展 `ImportMetaEnv` 接口为项目自定义的 `VITE_*` 变量
 *   提供精确类型，从而在编译期获得自动补全与类型检查。
 * - 只有以 `VITE_` 前缀开头的变量才会被 Vite 注入到客户端代码中，
 *   这是 Vite 的安全约定（避免泄露服务端敏感配置）。
 */
interface ImportMetaEnv {
  /**
   * Sentry 错误上报的 DSN（Data Source Name）。
   * - 留空或未设置时，Sentry 将自动以 no-op 模式运行（不发送任何数据），
   *   便于本地开发 / 私有化部署在无可观测后端时安全降级。
   * - 生产环境通过构建时环境变量注入，例如：
   *   `VITE_SENTRY_DSN=https://xxx@yyy.ingest.sentry.io/zzz pnpm build`
   */
  readonly VITE_SENTRY_DSN?: string;

  /**
   * Sentry 采样率（0~1），控制性能追踪（tracing）的事件上报比例。
   * 未设置时默认 0.1（10%）。仅影响 tracing，不影响错误事件上报。
   */
  readonly VITE_SENTRY_TRACES_SAMPLE_RATE?: string;

  /**
   * 应用发布版本号，用于 Sentry release 关联与问题归因。
   * 通常由 CI 在构建时注入（如 git commit sha 或语义化版本）。
   */
  readonly VITE_APP_VERSION?: string;
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}
