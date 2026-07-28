import { defineConfig } from 'vitest/config';
import path from 'path';

export default defineConfig({
  esbuild: {
    jsx: 'automatic',
  },
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './apps/secretpad/src'),
    },
  },
  test: {
    globals: true,
    environment: 'jsdom',
    // 将 jsdom 的文档 URL 固定为 http://localhost/，
    // 使 MSW 的相对路径 handler（如 '/api/login'）与请求 URL 能基于同一 origin 正确匹配。
    environmentOptions: {
      jsdom: {
        url: 'http://localhost/',
      },
    },
    // 全局初始化文件：启动 MSW 网络拦截、管理用例间状态隔离。
    setupFiles: ['./test/setup.ts'],
    include: ['apps/**/*.{test,spec}.{ts,tsx}', 'packages/**/*.{test,spec}.{ts,tsx}'],
    exclude: ['**/node_modules/**', '**/dist/**', '**/.{git,cache}/**', '**/e2e/**'],
    coverage: {
      // 使用 v8 覆盖率提供器（与 CI 中 @vitest/coverage-v8 对应）。
      provider: 'v8',
      // 报告输出格式：终端表格 + HTML（便于本地查看）+ lcov（供 CI 集成）。
      reporter: ['text', 'html', 'lcov'],
      // 仅统计源码目录，排除测试文件、mock、生成代码与配置。
      include: ['apps/secretpad/src/**', 'packages/*/src/**'],
      exclude: [
        '**/*.test.{ts,tsx}',
        '**/*.spec.{ts,tsx}',
        '**/generated/**',
        '**/mocks/**',
        '**/*.d.ts',
        '**/index.ts',
      ],
      // 覆盖率阈值门禁（按 glob 分层设定）：
      // - 仅对「核心逻辑层」（工具库 / 共享 lib / API 核心拦截器）设定高阈值，
      //   这些是纯逻辑、易测试、且回归风险最高的代码。
      // - UI 页面（pages/widgets/dag-next）仍会被统计并输出报告，但不设硬门禁，
      //   其质量由后续的 Playwright E2E 保障，避免为凑覆盖率而写脆性的页面单测。
      // - 低于阈值时测试以非零退出码失败，从而在 CI 中阻断核心覆盖率回退。
      thresholds: {
        // 通用工具库：纯函数，要求最高覆盖。
        'packages/utils/src/**': {
          statements: 90,
          branches: 85,
          functions: 90,
          lines: 90,
        },
        // 应用共享 lib（i18n / platform / observability 等核心基础设施）。
        'apps/secretpad/src/shared/lib/**': {
          statements: 80,
          branches: 70,
          functions: 70,
          lines: 80,
        },
        // API 核心拦截器（请求头注入 / 401 处理 / base URL 解析）。
        'packages/api-client/src/api.ts': {
          statements: 75,
          branches: 50,
          functions: 100,
          lines: 75,
        },
      },
    },
  },
});
