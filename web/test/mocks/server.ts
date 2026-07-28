import { setupServer } from 'msw/node';

import { handlers } from './handlers';

/** setupServer 返回值类型（避免手写与 MSW 内部类型不一致）。 */
type MswServer = ReturnType<typeof setupServer>;

/**
 * MSW Node 服务器实例（全局单例）。
 *
 * 说明：
 * - `setupServer` 会在 Node（vitest/jsdom）环境中拦截所有出站 HTTP 请求，
 *   命中 `handlers` 的请求返回 mock 响应，未命中的请求默认报错（便于发现遗漏）。
 * - 该实例在整个测试套件中共享，生命周期由 `test/setup.ts` 统一管理
 *   （beforeAll 启动 / afterEach 重置 / afterAll 关闭）。
 *
 * 为何用 globalThis 缓存：
 * - vitest 的 setup 文件与测试文件可能位于不同的模块图，直接 import 会产生
 *   多个模块实例，导致 setup 中 listen 的服务器与测试中 `server.use()` 操作的
 *   不是同一个，从而使拦截失效。将实例挂到 globalThis 可强制跨模块共享同一实例。
 */
const GLOBAL_KEY = '__secretpad_msw_server__';

const globalStore = globalThis as unknown as Record<string, MswServer | undefined>;

export const server: MswServer =
  globalStore[GLOBAL_KEY] ?? (globalStore[GLOBAL_KEY] = setupServer(...handlers));
