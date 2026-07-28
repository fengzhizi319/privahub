import { http, HttpResponse } from 'msw';

/**
 * MSW（Mock Service Worker）请求处理器集合。
 *
 * 作用：
 * - 在「网络层」拦截 HTTP 请求并返回预设响应，从而让集成测试能够
 *   驱动真实的 `api.ts`（openapi-fetch）+ `client.ts` 全链路，
 *   而不是像单元测试那样用 `vi.mock` 替换掉整个 api 模块。
 * - 这样可以覆盖：请求头注入（User-Token/Trace-Id）、响应解包、
 *   Zod 运行时校验、401 重定向等真实行为。
 *
 * 约定：
 * - SecretPad 后端统一返回 `{ status: { code, msg }, data }` 包裹结构，
 *   `code === 0` 表示成功。下列处理器均遵循该约定。
 * - 每个 handler 都返回最小可用数据集，聚焦于验证客户端逻辑而非数据完整性。
 * - **使用绝对 URL**：MSW 在 Node（vitest）环境中没有浏览器 location，
 *   相对路径无法正确解析，故统一基于 BASE（与 .env.test 中的
 *   VITE_API_BASE_URL 保持一致）拼接为绝对地址进行匹配。
 */

/** 与 `.env.test` 中 VITE_API_BASE_URL 保持一致的请求基地址。 */
const BASE = 'http://localhost';

/** 构造一个标准的成功响应包裹体。 */
const ok = (data: unknown) =>
  HttpResponse.json({ status: { code: 0, msg: 'success' }, data });

/** 构造一个业务失败响应（非零 code），用于验证错误分支。 */
const fail = (code: number, msg: string) =>
  HttpResponse.json({ status: { code, msg }, data: null });

export const handlers = [
  // 登录：返回 token 与用户上下文，验证 token 落盘逻辑。
  http.post(`${BASE}/api/login`, async ({ request }) => {
    const body = (await request.json()) as { name?: string; passwordHash?: string };
    if (body.name === 'admin' && body.passwordHash === 'correct-hash') {
      return ok({
        token: 'msw-token-123',
        name: 'admin',
        ownerId: 'kuscia-system',
        platformType: 'CENTER',
        platformNodeId: 'kuscia-system',
        ownerType: 'CENTER',
      });
    }
    return fail(202011601, 'invalid username or password');
  }),

  // 节点列表：返回单个 embedded 节点，验证字段归一化。
  http.post(`${BASE}/api/v1alpha1/node/list`, () =>
    ok([
      {
        nodeId: 'alice',
        nodeName: 'Alice Node',
        nodeStatus: 'Ready',
        type: 'embedded',
        netAddress: '127.0.0.1:28080',
        gmtCreate: '2026-07-26T11:02:33+08:00',
      },
    ])
  ),

  // CENTER 模式项目列表：返回最小可用项目集，验证 App 默认渲染仪表盘时的请求链路。
  http.post(`${BASE}/api/v1alpha1/project/list`, () =>
    ok([
      {
        projectId: 'p-center-1',
        projectName: 'Center Proj',
        gmtCreate: '2026-07-26T11:02:33+08:00',
        jobCount: 0,
      },
    ])
  ),

  // 项目任务列表：仪表盘 getJobs() 会逐项目拉取任务。
  // 响应为分页包裹结构 { data: [...], pageSize, pageTotal }，这里返回空列表。
  http.post(`${BASE}/api/v1alpha1/project/job/list`, () =>
    ok({ data: [], pageSize: 10, pageTotal: 0 })
  ),

  // 登出：仅返回成功，验证本地 token 清理。
  http.post(`${BASE}/api/logout`, () => ok(null)),

  // P2P 项目列表：返回合法 ProjectVO，验证 Zod 校验通过路径。
  http.post(`${BASE}/api/v1alpha1/p2p/project/list`, () =>
    ok([
      {
        projectId: 'p1',
        projectName: 'P2P Proj',
        computeMode: 'MPC',
      },
    ])
  ),
];

/**
 * 专门用于「错误路径」测试的处理器集合。
 * 在具体的测试用例中通过 `server.use(...errorHandlers.xxx)` 临时覆盖默认行为。
 */
export const errorHandlers = {
  /** 让项目列表返回缺失必填字段的非法数据，触发 Zod 校验失败。 */
  invalidProjectList: http.post(`${BASE}/api/v1alpha1/p2p/project/list`, () =>
    ok([{ projectName: 'no-id' }])
  ),

  /** 返回 401，验证 api.ts 的 onResponse 拦截器清理 token 的行为。 */
  unauthorized: http.post(
    `${BASE}/api/v1alpha1/node/list`,
    () => new HttpResponse(null, { status: 401 })
  ),
};
