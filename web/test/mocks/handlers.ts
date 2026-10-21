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
  // 用户上下文（user/get）：应用加载时刷新 platformType / deployMode。
  http.post(`${BASE}/api/v1alpha1/user/get`, () =>
    ok({
      name: 'admin',
      ownerId: 'kuscia-system',
      platformType: 'CENTER',
      platformNodeId: 'kuscia-system',
      ownerType: 'CENTER',
      deployMode: 'ALL-IN-ONE',
    })
  ),

  // 待处理消息数（message/pending）：Header 角标。
  http.post(`${BASE}/api/v1alpha1/message/pending`, () => ok(3)),

  // 登录：返回 token 与用户上下文，验证 token 落盘逻辑。
  http.post(`${BASE}/api/v1alpha1/user/login`, async ({ request }) => {
    const body = (await request.json()) as { name?: string; password?: string; passwordHash?: string };
    const pass = body.password || body.passwordHash;
    if (body.name === 'admin' && pass === 'correct-hash') {
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

  // 登录：浏览器以 X-Privahub-Session: cookie 请求时，后端只下发 HttpOnly Cookie，
  // body 中不含 token（与 Go 后端一致）。
  http.post(`${BASE}/api/login`, async ({ request }) => {
    const body = (await request.json()) as { name?: string; passwordHash?: string };
    const cookieMode = request.headers.get('X-Privahub-Session') === 'cookie';
    if (body.name === 'admin' && body.passwordHash === 'correct-hash') {
      return ok({
        token: cookieMode ? '' : 'msw-token-123',
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
  // Java PageResponse：{ pageTotal, pageSize, total, data: ProjectJobSummaryVO[] }。
  http.post(`${BASE}/api/v1alpha1/project/job/list`, () =>
    ok({ data: [], pageSize: 10, pageTotal: 0, total: 0 })
  ),

  // ---------------- DAG / graph（Java 契约形状） ----------------
  // component/list → Map<app, CompListVO{name, desc, version, comps[]}>。
  http.post(`${BASE}/api/v1alpha1/component/list`, () =>
    ok({
      secretflow: {
        name: 'secretflow',
        desc: 'SecretFlow',
        version: '1.0.0',
        comps: [
          { domain: 'data_prep', name: 'psi', version: '1.0.0', desc: 'PSI between two parties.' },
          { domain: 'ml.train', name: 'ss_glm_train', version: '1.0.0', desc: 'SS-GLM training.' },
        ],
      },
      trustedflow: {
        name: 'trustedflow',
        desc: 'TrustedFlow',
        version: '0.1.0',
        comps: [{ domain: 'data_prep', name: 'psi', version: '0.1.0', desc: 'TEE PSI.' }],
      },
    })
  ),
  // component/batch → 有序 ComponentDef[]（SF proto JSON，无 code_name）。
  http.post(`${BASE}/api/v1alpha1/component/batch`, () =>
    ok([
      {
        domain: 'data_prep',
        name: 'psi',
        version: '1.0.0',
        attrs: [{ name: 'receiver_parties', type: 'AT_PARTY', atomic: { listMaxLengthInclusive: '2' } }],
        inputs: [
          { name: 'input_ds1', types: ['sf.table.individual'], attrs: [{ name: 'keys', colMinCntInclusive: '1', colMaxCntInclusive: '1' }] },
          { name: 'input_ds2', types: ['sf.table.individual'], attrs: [{ name: 'keys', colMinCntInclusive: '1', colMaxCntInclusive: '1' }] },
        ],
        outputs: [{ name: 'psi_output', types: ['sf.table.vertical'] }, { name: 'report', types: ['sf.report'] }],
      },
    ])
  ),
  // component/i18n → Map<app, Map<"domain/name:version", Map<原文, 译文>>>。
  http.post(`${BASE}/api/v1alpha1/component/i18n`, () =>
    ok({ secretflow: { 'data_prep/psi:1.0.0': { psi: '隐私求交', receiver_parties: '结果接收方' } }, trustedflow: {} })
  ),
  http.post(`${BASE}/api/v1alpha1/graph/detail`, () =>
    ok({
      projectId: 'p1',
      graphId: 'g1',
      name: 'Graph 1',
      nodes: [
        {
          graphNodeId: 'g1-node-1',
          codeName: 'data_prep/psi',
          label: '隐私求交',
          x: 10,
          y: 20,
          inputs: ['', ''],
          outputs: ['g1-node-1-output-0', 'g1-node-1-output-1'],
          nodeDef: { domain: 'data_prep', name: 'psi', version: '1.0.0' },
          status: 'STAGING',
        },
      ],
      edges: [],
      maxParallelism: 1,
      dataSourceConfig: [{ editEnable: true, nodeId: 'alice', nodeName: 'alice', dataSourceName: 'default', dataSourceId: 'default-data-source' }],
    })
  ),
  // graph/node/status：没有任务的节点为 STAGING。
  http.post(`${BASE}/api/v1alpha1/graph/node/status`, () =>
    ok({ finished: true, nodes: [{ graphNodeId: 'g1-node-1', status: 'STAGING', progress: 0, parties: [] }] })
  ),
  http.post(`${BASE}/api/v1alpha1/graph/node/output`, () =>
    ok({
      type: 'table',
      codeName: 'data_prep/psi',
      jobId: 'job1',
      taskId: 'job1-g1-node-1',
      graphID: 'g1',
      gmtCreate: '2026-09-01T00:00:00Z',
      meta: {
        headers: [{ name: 'metas', type: 'AT_STRING' }],
        rows: [
          { path: 'job1-g1-node-1-output-0', nodeId: 'alice', nodeName: 'alice', type: 'embedded', fields: 'id,age', fieldTypes: 'str,int', tableId: 't1', dsId: 'default-data-source', datasourceType: 'LOCAL' },
        ],
      },
      tabs: null,
    })
  ),
  http.post(`${BASE}/api/v1alpha1/graph/node/max_index`, () => ok({ maxIndex: 32 })),
  http.post(`${BASE}/api/v1alpha1/graph/start`, () => ok({ jobId: 'job1' })),
  http.post(`${BASE}/api/v1alpha1/project/get`, () =>
    ok({
      projectId: 'p1',
      projectName: 'Demo',
      computeMode: 'MPC',
      nodes: [{ nodeId: 'alice', nodeName: 'alice', nodeType: 'embedded', datatables: [{ datatableId: 't-alice', datatableName: 'alice.csv' }] }],
    })
  ),
  http.post(`${BASE}/api/v1alpha1/project/datatable/get`, () =>
    ok({
      datatableId: 't-alice',
      datatableName: 'alice.csv',
      nodeId: 'alice',
      nodeName: 'alice',
      configs: [{ colName: 'id', colType: 'str', isAssociateKey: true }],
      datatableVO: { datatableId: 't-alice', schema: [{ featureName: 'ignored', featureType: 'str' }] },
    })
  ),

  // 登出：仅返回成功，验证本地会话标记清理。
  http.post(`${BASE}/api/logout`, () => ok(null)),
  http.post(`${BASE}/api/v1alpha1/user/logout`, () => ok('admin')),

  // 刷新：用 HttpOnly refresh Cookie 轮换会话（cookie 模式下 body 不含 token）。
  http.post(`${BASE}/api/user/refresh`, () => ok({ token_type: 'Cookie' })),

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
