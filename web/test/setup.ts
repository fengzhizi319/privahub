import { afterAll, afterEach } from 'vitest';

import { server } from './mocks/server';

/**
 * Vitest 全局测试环境初始化（setup file）。
 *
 * 职责：
 * 1. 管理 MSW 服务器生命周期，使所有测试共享同一套网络 mock。
 * 2. 该文件通过 vitest.config.ts 的 `test.setupFiles` 引入，对所有测试生效。
 *
 * 关键：为何在「模块顶层」而非 beforeAll 中调用 listen？
 * - openapi-fetch 在 `createClient()` 时（即 api.ts 模块被加载时）就会捕获
 *   当前的 `globalThis.fetch` 引用。若 MSW 在 beforeAll 才启动拦截，
 *   此时 api.ts 已持有「未被 patch 」的旧 fetch，导致请求绕过 MSW 直达真实网络。
 * - vitest 会先执行 setupFiles 的模块顶层代码，再收集/加载测试文件及其依赖，
 *   因此在这里同步调用 listen 可确保 fetch 在任何业务模块加载前就被 MSW 接管。
 */

// 启动 MSW 拦截；对未显式 mock 的请求打印警告（而非直接抛错），
// 便于在开发新测试时发现遗漏的 handler，同时不阻断既有测试。
server.listen({ onUnhandledRequest: 'warn' });

afterEach(() => {
  // 恢复默认 handlers（清除用例内通过 server.use 临时覆盖的处理器）。
  server.resetHandlers();
  // 清空浏览器存储，保证用例间状态隔离。
  localStorage.clear();
});

afterAll(() => {
  server.close();
});
