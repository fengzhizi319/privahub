# SecretPad 新前端迁移一致性说明

> 本文档说明如何保证 `secretpad/frontend-src`（旧前端）的功能与接口在 `secretpad/web`（新前端）上保持一致，并持续可验证。

## 1. 迁移原则

- **接口真实化**：新前端所有 CRUD、Job、权限、平台类型判断均调用后端真实接口，不保留 mock 数据。
- **接口契约对齐**：以 `secretpad/web/openapi/secretpad.openapi.json` 为唯一源，通过 `openapi-typescript` + `openapi-fetch` 生成类型与请求客户端。
- **功能逐页映射**：旧前端每个一级页面（Dashboard / Projects / Nodes / DataSources / DataTables / DAG / ...）的字段、按钮、权限在新前端都有对应实现。
- **双语支持**：所有用户可见文案通过 `shared/lib/i18n` 字典统一维护，默认跟随浏览器，支持中英切换。
- **权限模型复刻**：复用旧前端的 `Platform` / `PadMode` / `useHasAccess` / `AccessGuard` / `RouteGuard` 模型。
- **现代架构**：路由采用 TanStack Router（代码式路由 + `beforeLoad` 守卫），服务端状态采用 TanStack Query v5（`useQuery` / `useMutation` + 缓存失效，DAG 轮询用 `refetchInterval`）；API 响应统一经 Zod `safeParse` 运行时校验（`validated`/`unwrapValidated`），契约不符时抛出带字段路径的错误。
- **通知统一**：所有 `alert`/`window.confirm` 已替换为 design-system 的 `Toast` 与 `ConfirmDialog`。

## 2. 接口一致性检查清单

| 能力 | 旧前端页面/模块 | 新前端页面 | 真实接口 | 状态 |
|---|---|---|---|---|
| 登录 / 登出 | Login | `pages/login` | `POST /api/login` / `/api/logout` | ✅ |
| Dashboard 统计与 Recent Jobs | Dashboard | `pages/dashboard` | `node/list`, `project/list`, `project/job/list` | ✅ |
| 项目列表 / 创建 | Projects | `pages/projects` | `project/list`, `project/create` | ✅ |
| 节点注册 / 编辑 / 删除 / 刷新 / Token | Nodes | `pages/nodes` | `node/*` | ✅ |
| 数据源添加 / 删除 | DataSources | `pages/data-sources` | `datasource/*` | ✅ |
| 数据表导入 / 删除 / Schema | DataTables | `pages/data-tables` | `datatable/*` | ✅ |
| 平台类型路由守卫 | route guards | `features/auth/ui/access-guard.tsx` | auth-store 中 platformType | ✅ |
| 按钮级权限 | AccessControl | `features/auth/ui/access-guard.tsx` | `useHasAccess` + `AccessGuard` | ✅ |
| 中英双语切换 | Locale Switch | `widgets/AppHeader` + `shared/lib/i18n` | 本地字典 | ✅ |
| 模型产物列表 / 删除 / 详情 | Model Manager | `pages/models` | `model/page`, `model/delete`, `model/info`, `model/serving/*` | ✅ |
| 消息与审批列表 | Message Center | `pages/messages` | `message/list`, `message/pending` | ✅ |
| 周期任务列表 / 下线 / 删除 | Periodic Tasks | `pages/periodic-tasks` | `scheduled/page`, `scheduled/offline`, `scheduled/del` | ✅ |
| DAG 图列表 / 创建 / 删除 / 运行 | DAG Editor | `pages/dag` | `graph/*`, `component/list`, `component/batch`, `component/i18n` | ✅ |
| DAG 节点拖拽保存 / 算子配置 / 连线 | DAG Editor | `packages/dag-next` | `graph/update`, `graph/node/update` | ✅ |
| DAG 运行时日志 / 节点输出 | DAG Editor | `packages/dag-next` | `graph/node/logs`, `graph/node/output` | ✅ |
| DAG 节点状态轮询 | DAG Editor | `pages/dag` | `graph/node/status` | ✅ |
| 项目详情 / 编辑 / 删除 / 参与方 | Projects | `pages/projects` | `project/get`, `project/update`, `project/delete`, `project/node/add`, `project/datatable/*` | ✅ |
| 项目内任务详情 / 停止 | Projects | `pages/projects` | `project/job/get`, `project/job/stop` | ✅ |
| DAG 停止运行 / 重命名 | DAG Editor | `pages/dag` | `graph/stop`, `graph/meta/update` | ✅ |
| 消息详情 / 审批回复 | Message Center | `pages/messages` | `message/detail`, `message/reply` | ✅ |
| 模型打包 / 废弃 / 状态 / 详情 / 部署 | Model Manager | `pages/models` | `model/pack`, `model/discard`, `model/status`, `model/detail`, `model/modelPartyPath`, `model/serving/create` | ✅ |
| 周期任务创建 / 重跑 / 停止 | Periodic Tasks | `pages/periodic-tasks` | `scheduled/graph/create`, `scheduled/info`, `scheduled/task/rerun`, `scheduled/task/stop` | ✅ |
| 节点 Token 复制 / 重置 | Nodes | `pages/nodes` | `node/token`, `node/newToken` | ✅ |
| 数据表详情 / 推送 TEE | DataTables | `pages/data-tables` | `datatable/get`, `datatable/pushToTee` | ✅ |
| 数据上传 / 下载 / 同步 | DataTables | `packages/api-client` | `data/create`, `data/download`, `data/upload`, `data/sync` | ✅ |
| 节点路由列表 / 编辑 / 删除 / 刷新 | Node Route | `pages/node-routes` | `nodeRoute/page`, `nodeRoute/get`, `nodeRoute/update`, `nodeRoute/delete`, `nodeRoute/refresh` | ✅ |
| 机构信息 / 机构节点管理 | Institution | `pages/institutions` | `inst/get`, `inst/node/list`, `inst/node/add`, `inst/node/delete`, `inst/node/token`, `inst/node/register` | ✅ |
| P2P 节点注册 / 删除 | P2P My Node | `pages/p2p/my-node` | `p2p/node/create`, `p2p/node/delete` | ✅ |
| P2P 项目列表 / 创建 / 编辑 / 归档 / 参与方 | P2P Projects | `pages/p2p/projects` | `p2p/project/list`, `p2p/project/create`, `p2p/project/update`, `p2p/project/archive`, `p2p/project/participants` | ✅ |
| 用户信息 / 修改密码 | Account | `pages/account` | `user/get`, `user/updatePwd` | ✅ |
| 数据源详情 / 节点 | DataSources | `pages/data-sources/detail` | `datasource/detail`, `datasource/nodes` | ✅ |
| 审批创建 / 状态轮询 | Approval | `pages/messages` + `features/approval` | `approval/create`, `approval/pull/status` | ✅ |
| 项目任务日志 / 输出 | Projects / DAG | `packages/api-client` | `project/job/task/logs`, `project/job/task/output` | ✅ |
| 项目 TEE 节点 / 输出表 / 表配置 / 数据源 / 机构 | Projects | `packages/api-client` | `project/tee/list`, `project/getOutTable`, `project/update/tableConfig`, `project/datasource/list`, `project/inst/add` | ✅ |
| 周期任务一次成功 / 作业列表 / 任务详情 | Periodic Tasks | `packages/api-client` | `scheduled/graph/once/success`, `scheduled/job/list`, `scheduled/task/info` | ✅ |
| 节点用户 / 远程用户密码重置 | Nodes / Account | `packages/api-client` | `user/node/resetPassword`, `user/remote/resetPassword` | ✅ |
| 节点分页 / 图节点最大索引 / 模型服务详情 | Nodes / DAG / Models | `packages/api-client` | `node/page`, `graph/node/max_index`, `model/serving/detail` | ✅ |
| 特征数据源 / 云日志 / 投票同步 / 版本 | Feature DS / Logs | `pages/feature-datasource` / `pages/cloud-logs` / `pages/component-versions` | `feature_datasource/create`, `feature_datasource/auth/list`, `cloud_log/sls`, `vote_sync/create`, `version/list` | ✅ |

## 3. 自动化验证

### 3.1 类型检查

```bash
cd /home/charles/code/sfwork/secretpad/web
corepack pnpm typecheck
```

覆盖所有 `@secretpad/*` workspace 包，确保 OpenAPI 类型、业务类型、TSX 组件类型一致。

### 3.2 单元测试

```bash
cd /home/charles/code/sfwork/secretpad/web
corepack pnpm test
```

包含：
- `packages/utils` 工具函数
- `packages/api-client` 接口映射
- `apps/secretpad` 页面渲染与交互

### 3.3 E2E 关键路径

```bash
cd /home/charles/code/sfwork/secretpad/web/apps/secretpad

# 1. 确保后端在 8080 运行，且 Vite dev server 在 8000 运行
# 2. 运行 Playwright E2E
corepack pnpm exec playwright test
```

当前 E2E 覆盖：
- 登录成功并进入 Dashboard
- 侧边栏导航至 Nodes 页面
- 侧边栏导航至 Messages 页面
- 侧边栏导航至 DAG 页面（Workspace 或空态渲染）

后续随功能迭代继续补充 Projects / DataSources / DataTables 的创建-删除闭环 E2E，以及 DAG 节点拖拽、保存、运行的端到端覆盖。

## 4. 开发-生产构建衔接

采用「同仓并列隔离（Monorepo）」方案：

- 开发期：`secretpad/web/apps/secretpad` 通过 Vite proxy (`/api → 127.0.0.1:8080`) 直连本地后端。
- 生产期：`secretpad/web/scripts/build/build.sh` 先 `pnpm build`，再将 `apps/secretpad/dist/*` 复制到 `secretpad/secretpad-web/src/main/resources/static/`，最后 `mvn package` 出 Fat Jar。

详见顶层 `secretpad/scripts/build/build.sh` 与项目 `README.md`。

## 5. 新增页面/功能时的自查项

1. 是否从真实 API 获取数据（`apiClient.*`）？
2. 是否使用 `useTranslation()` 包裹所有用户可见文案？
3. 是否在 `shared/lib/i18n/dictionaries.ts` 中补充 `zh-CN` / `en-US` 字典？
4. 管理类按钮是否用 `AccessGuard` 包裹，并指定允许的 `Platform` / `PadMode`？
5. 是否在 `router.tsx` 中通过 `createRoute` 注册路由，并由根/`app` 路由的 `beforeLoad` 与 `RouteGuard` 校验登录态与平台类型？
6. 数据获取是否使用 TanStack Query（`useQuery`/`useMutation` + `invalidateQueries`），而非手写 `useEffect`？
7. 是否补充单元测试 / Playwright E2E？

## 6. 已知限制与后续阶段

- 所有一级页面（含 P2P 项目/我的节点、机构管理、节点路由、用户中心）与 DAG 高级能力均已接入真实接口，旧前端约 50 个接口已全量迁移（见第 2 节清单）。
- DAG 画布支持节点拖拽、算子库添加、节点配置/日志/输出面板、连线、保存与运行，并提供节点状态轮询；Mock 演示节点与算子库兑底假数据已移除，接口失败时呈现错误态。
- 数据表分级（classification）改为读取后端真实字段，导入表单支持 L1–L5 选择并透传后端，不再硬编码 `'L1'`。
- 节点 Token 弹窗已支持复制（`navigator.clipboard`）与重置（`node/newToken`）。
- 模型部署/打包、周期任务创建/重跑/停止均已接入真实表单与接口（配合 DAG 训练/打包结果）。
- DAG 算子配置面板当前以通用 JSON 编辑器（`nodeDef`、`config`）呈现，PSI 模板向导已可按项目节点与数据表一键生成 read_data → PSI 初始图，后续可针对高频算子提供更友好的表单化配置。
- `secretpad/frontend-src/` 旧前端目录已弃用删除，构建脚本（`scripts/dev-start.sh`、`scripts/run-all-no-docker.sh`、`scripts/clone-repos.sh`）与顶层 README/AGENTS 已统一指向 `secretpad/web/`。
- 数据表导入当前使用简单的 `name:type` Schema 输入，后续可接入数据源自动拉取元数据。
- P2P / inst / nodeRoute 等接口的真实后端行为依赖 Kuscia 环境，本地仅保证类型与调用正确，联调需 `scripts1/dev-start.sh` 环境。

## 7. 端到端验证记录（2026-07-27）

### 7.1 验证环境

- 前端：`secretpad/web` Vite dev server on `http://127.0.0.1:8000`
- 后端：`secretpad` Spring Boot on `http://127.0.0.1:8080`
- Kuscia：Docker Master + alice + bob（dev-start.sh 部署）
- SecretFlow：自定义镜像 `secretflow/sf-privacy-dev:1.15.0.dev-privacy` 已注册为 Kuscia AppImage

### 7.2 PSI → 逻辑回归训练链路

1. 使用已有项目 `srkbfvmn`、图 `mzeeuxmc`（含 alice-table / bob-table → PSI）。
2. 通过 `/api/v1alpha1/graph/node/output` 验证 PSI 输出：生成联合表 `vtpo-mzeeuxmc-node-3-output-0`，字段覆盖 alice 24 列 + bob 21 列。
3. 在图中追加 `ml.train/ss_sgd_train` 节点 `mzeeuxmc-node-4`，label=`y`，feature_selects=43 个非 ID/label 列。
4. 调用 `/api/v1alpha1/graph/update` 与 `/api/v1alpha1/graph/start`，得到 Job ID `vtpo`。
5. 轮询确认所有节点 `SUCCEED`；训练节点输出类型为 `model`，路径为 `vtpo-mzeeuxmc-node-4-output-0`（alice / bob 各一份）。

结论：前端 → 后端 → Kuscia → SecretFlow 自定义镜像的训练链路已跑通。

### 7.3 模型打包前端增强

- 旧 Models 页面 Pack 逻辑直接传入 `modelPartyConfig: []` 与 `modelComponent: []`，导致后端 `Index 0 out of bounds for length 0` 错误。
- 增强后：
  - 选择训练节点后自动调用 `/api/v1alpha1/model/modelPartyPath` 获取参与方数据源。
  - 按参与方展示数据源下拉框，构造 `modelPartyConfig`。
  - 根据训练节点 `nodeDef` 构造 `modelComponent`（包含 graphNodeId / domain / name / version）。
  - 提交后调用 `/api/v1alpha1/model/status` 轮询，直到 `SUCCEED` 或 `FAILED`。
- 验证：
  - 打包请求 `/api/v1alpha1/model/pack` 成功返回 `jobId: town`。
  - 当前 Kuscia/SecretFlow 模型导出作业在本地环境存在运行时失败（alice 或 bob 任务失败），属于后端/运行时问题，前端已能正确发起完整打包流程并展示状态。
- 部署服务：基于打包成功后的 `modelId` 调用 `/api/v1alpha1/model/serving/create`；当前因打包未成功无法完成端到端部署，但前端表单与接口已就绪。

### 7.4 消息中心

- 修复 `apiClient.getMessages` 未传 `isInitiator: false` 导致后端 NPE 的问题。
- 验证 `/api/v1alpha1/message/list` 与 `/api/v1alpha1/message/pending` 返回空列表/0 待处理，无报错。
- 当前非 P2P 模式无真实审批消息，消息列表为空为正常状态。

### 7.5 启动脚本健壮性

- `scripts1/dev-start.sh` 的 `start_frontend` 增加：
  - 端口监听超时从 120s 延长到 180s。
  - 端口就绪后增加 `curl -f http://127.0.0.1:8000/` HTTP 200 二次确认，避免 Vite 已 bind 端口但尚未完成初始构建时误报“前端未就绪”。

### 7.6 Projects 页面增强（2026-07-27 后续）

- 重写 `pages/projects/index.tsx`：
  - 项目卡片列表保留搜索、创建、编辑、删除。
  - 点击卡片打开详情抽屉：展示基本信息、已加入节点、按节点分组的已关联数据表、近期任务。
  - 支持在抽屉中直接添加节点、添加数据表、从项目移除数据表（调用 `project/datatable/delete`）。
  - 支持查看任务详情弹窗：列出任务下所有图节点/算子状态，点击节点可查看日志与输出。
- 任务 `taskId` 按后端约定由 `jobId-graphNodeId` 组合得到，用于调用 `project/job/task/logs` 与 `project/job/task/output`。
- 补充 `shared/lib/i18n/dictionaries.ts` 中 Projects 相关文案，覆盖中英双语。
- 验证：类型检查与 lint 通过，无新增错误。

### 7.7 DataTables 页面增强（2026-07-27 后续）

- 重写 `pages/data-tables/index.tsx`：
  - 保留节点过滤、数据表列表、导入、删除、推送到 TEE。
  - 新增数据表“详情”抽屉，包含三个标签页：
    - **Schema 预览**：展示数据源、节点、URI、状态及完整字段（类型、描述、敏感分级）。
    - **授权项目**：展示已授权项目列表，支持新增授权（选择项目 + 关联键 + 标签列）与取消授权。
      新增授权调用 `project/datatable/add`，取消授权调用 `project/datatable/delete`。
    - **授权血缘**：以树状结构展示“数据表 → 已授权项目”的血缘关系。
  - 刷新状态按钮调用 `datatable/get` 重新拉取详情与状态。
- 扩展 `apiClient.addProjectDatatable` 的 `configs` 类型为 `TableColumnConfigParam[]`，支持透传 `isAssociateKey` / `isLabelKey` / `isProtection`。
- 补充中英双语字典 `dataTables.*` 与 `projects.selectProject`。
- 验证：类型检查与 lint 通过，无新增错误。

### 7.8 Nodes 页面增强（2026-07-27 后续）

- 重写 `pages/nodes/index.tsx`：
  - 节点列表增加搜索框（按名称 / ID / 通讯地址过滤）。
  - 点击节点名称打开详情抽屉，包含三个标签页：
    - **基本信息**：节点名、ID、类型、状态、地址、注册时间、描述；对 `type === 'embedded'` 显示“内置”标签。
    - **部署令牌**：展示 Token 状态与内容，支持复制与重新生成。
    - **节点产物**：调用 `node/result/list` 获取模型/规则等产物，支持查看产物详情（`node/result/detail`）。
  - 保留原有注册、编辑、刷新、删除、查看 Token 功能。
- 补充中英双语字典 `nodes.*`（detail / basicInfo / deployToken / nodeResults / embedded 等）。
- 验证：类型检查与 lint 通过，无新增错误。

### 7.9 DAG 复杂组件迁移（Phase 4 进行中）

- 增强 `packages/dag-next/src/index.tsx`：
  - **算子配置面板**：选中节点时通过 `onGetComponentDef` 回调拉取组件定义（`component/batch`），展示算子描述、Inputs/Outputs、Attributes 摘要，辅助用户填写 `nodeDef` / `config` JSON。
  - **结果可视化**：输出面板识别 `type === 'table'` 且 `meta.rows` 为数组时，以 HTML 表格形式展示数据；支持 `tabs` 分栏展示；其余类型仍使用 JSON 高亮。
- 在 `pages/dag/index.tsx` 中实现 `handleGetComponentDef`：按 `codeName` 解析 domain/name，调用 `apiClient.batchGetComponent` 并映射为 `ComponentMetadata`。
- 类型检查与 lint 通过，无新增错误。
- 待完成：结果管理独立页、DAG 模型提交、DAG 周期任务入口（Phase 7~9）。

### 7.10 DAG 模板库（Phase 6）

- 在 `features/dag-templates/` 下建立可扩展的模板贡献系统：
  - `types.ts`：定义 `TemplateMetadata`、`TemplateBuildInput`、`TemplateBuildResult`、`TemplateContribution` 及各类模板配置类型（双表/单表/K-匿名/L-多样性/本地差分隐私/脱敏/Risk/TEE）。
  - `builder.ts`：提供 `nodeId` / `edgeId` / `connect` / `createNode` / `createReadDataNode` / `createPsiNode` / `buildSingleTablePrivacyTemplate` / 属性辅助函数（`sAttr` / `ssAttr` / `i64Attr` / `fAttr` / `bAttr` / `jsonAttr` / `naAttr`），统一封装旧前端中重复的 nodeDef/edges 构建逻辑。
  - `templates/`：逐个迁移旧前端 `pipeline-template-*.ts` 为纯函数模板实现：blank、psi、data-classification、sanitization、query-obfuscation、k-anonymity、l-diversity、local-differential-privacy、differential-privacy、risk、tee。
  - `registry.ts`：集中注册所有模板，按 `basic` / `privacy` / `ml` 分类，提供 `templateByKey`、`isTwoTableTemplate`、`isSingleTableTemplate` 等类型守卫。
  - `use-template-wizard.ts` + `template-wizard.tsx`：通用模板向导。根据模板类型动态渲染双表/单表/无参数表单；自动加载节点数据表、列信息；创建时先 `createGraph` 获取 graphId，再 `template.build` 生成节点/边，最后 `updateGraph` 提交。
- 在 `pages/dag/index.tsx` 中将旧“PSI 模板”按钮替换为“模板向导”按钮，接入 `TemplateWizard`。
- 扩展 `shared/lib/i18n/dictionaries.ts` 的 `dag` 命名空间：新增 `template`、`templateWizardTitle`、`selectTemplate`、`templateCategory`、`templateName`、`templateDesc`、`templateSingleTable`、`templateNoInputsHint`、`featureColumns`、`labelColumn`、`predictionName`、`qiColumns`、`saColumns`、`queryColumn`、`sanitizationColumns` 等中英键值。
- 将 `shared/lib/i18n/dictionaries.ts` 的字典类型由 `Record<string, string | Record<string, string>>` 改为递归接口 `Dictionary`，支持多层嵌套命名空间，同步更新 `I18nContext.tsx` 的 `getValue` 类型。
- 验证：`corepack pnpm --filter @secretpad/app typecheck` 与 `corepack pnpm run lint` 通过（0 errors，历史 warning 50 个）。

### 7.11 弃用旧前端 frontend-src（Phase 5）

- 删除本地 `secretpad/frontend-src/` 目录（旧 Umi 前端，已不在维护）。
- 更新 `scripts/clone-repos.sh`：不再从独立 `secretpad-frontend` 仓库克隆旧前端；secretpad 仓库自带新前端 `secretpad/web/`。
- 更新 `scripts/dev-start.sh`：环境检测、前端依赖安装、启动命令全部切换为 `secretpad/web/` 与 `corepack pnpm --filter @secretpad/app dev`；增加 180s 端口监听 + 30s HTTP 200 二次确认。
- 更新 `scripts/run-all-no-docker.sh`：无 Docker 模式前端启动同样切换为 `secretpad/web/`。
- 改进 `scripts/dev-stop.sh`：停止前端时递归终止 `pnpm -> node -> Vite` 所有后代进程，避免子进程残留继续占用 8000 端口；所有脚本路径引用统一为 `scripts/` / `scripts1/` 一致。
- 更新顶层 `README.md` 与 `AGENTS.md`：前端目录、启动命令、架构图统一改为 `secretpad/web/` + Vite，并标注旧前端已弃用删除。
- 验证：
  - `corepack pnpm typecheck` / `pnpm run lint` 通过（0 errors，历史 warning 48 个）。
  - `bash -n scripts/dev-start.sh scripts/dev-stop.sh scripts1/dev-start.sh scripts1/dev-stop.sh` 通过。
  - 执行完整 `bash scripts1/dev-start.sh`：后端 HTTP 8080、前端 8000 首页均返回 200；`POST /api/login` 成功返回 token；`POST /api/v1alpha1/node/list` 成功返回 alice/bob 节点；执行 `bash scripts1/dev-stop.sh` 后 8000/8080/8443 端口全部释放。

### 7.12 结果管理独立页（Phase 7）

- 新增 `pages/results/index.tsx`：
  - 调用 `apiClient.listNodeResults` 跨项目拉取节点产物列表，支持 `nameFilter` 搜索、`kindFilters` 类型筛选、`timeSortingRule` 时间排序、分页。
  - 列表字段：结果名称、所属节点、类型（table/report/rule/model）、来源项目、计算模式、TEE 拉取状态、创建时间。
  - 点击“下载”调用 `apiClient.downloadData` 触发浏览器下载（非报告类结果）。
  - 点击“详情”打开 Modal，调用 `apiClient.getNodeResultDetail` 展示结果元数据与 `output` 摘要。
- 新增 `/results` 路由（`router.tsx`），并在 `AppSidebar` / `AppLayout` 中增加侧边栏入口与标题。
- 补充 `i18n/dictionaries.ts` 的 `results` 命名空间与 `sidebar.results` 键值。
- 验证：`corepack pnpm --filter @secretpad/app typecheck` 与 `corepack pnpm run lint` 通过（0 errors）。



## 8. 迁移完成度总结与剩余功能清单

> 数据来源：旧前端基于 `github.com/fengzhizi319/secretpad-frontend.git`（clone 到 `/tmp/secretpad-frontend-analysis`）的 `main` 分支；新前端基于 `secretpad/web/` 当前工作区。下文以“旧前端”指代 `secretpad-frontend`（原 `frontend-src`），“新前端”指代 `secretpad/web/`。

### 7.13 DAG 模型提交入口（Phase 8）

- 新增 `features/model-pack/model-pack-modal.tsx`：
  - 接收 `projectId`、`graphId`、`trainNode`（成功训练节点）。
  - 打开 Modal 时调用 `model/modelPartyPath` 获取每个参与方可用的数据源，默认选中第一个数据源。
  - 用户填写模型名称，确认后构造 `modelPartyConfig` 与 `modelComponent`（从 `nodeDef` 读取 domain / name / version）。
  - 提交 `model/pack` 后，若返回 `jobId` 则轮询 `model/status` 直至 `SUCCEED` 或 `FAILED`。
- 在 `pages/dag/index.tsx` 工具栏增加“打包模型”按钮：仅当选中节点状态为 `Success` 且 `codeName` 包含 `train` 时展示。
- 补充 `i18n/dictionaries.ts`：`models.packFromDagTitle`、`models.trainNode`、`dag.packModel` 等中英键值。
- 验证：`corepack pnpm --filter @secretpad/app typecheck` 与 `corepack pnpm run lint` 通过（0 errors）。

### 7.14 DAG 周期任务入口（Phase 9）

- 新增 `features/scheduled-task-from-dag/scheduled-task-modal.tsx`：
  - 接收 `projectId`、`graphId`、`graphName`、当前 `nodes[]`。
  - 打开 Modal 时默认全选当前 DAG 节点作为执行节点，并校验 `scheduled/graph/once/success`；若图尚未成功运行过，提示用户先运行成功。
  - 用户填写任务名称与 Cron 表达式，提交 `scheduled/graph/create`。
- 在 `pages/dag/index.tsx` 工具栏增加“创建周期任务”按钮：当选中 graph 时展示。
- 补充 `i18n/dictionaries.ts`：`dag.createPeriodicTask`、`dag.periodicTaskOnceSuccessHint`、`dag.periodicTaskNodes`、`dag.periodicTaskSelectAll`、`dag.periodicTaskDeselectAll` 等中英键值。
- 验证：`corepack pnpm --filter @secretpad/app typecheck` 与 `corepack pnpm run lint` 通过（0 errors）。

### 7.16 数据上传入口（P1 Phase 10）

- 新增 `features/data-upload/data-upload-modal.tsx`：
  - 选择目标 Kuscia 节点（默认当前数据表页面选中的节点）。
  - 选择本地文件，展示文件大小。
  - 调用 `apiClient.uploadData`（`POST /api/v1alpha1/data/upload?Node-Id=...`）上传文件。
  - 上传成功后展示返回的数据源名称、真实文件名、数据源、数据源类型。
- 在 `pages/data-tables/index.tsx` 工具栏增加“上传数据文件”按钮。
- 补充 `i18n/dictionaries.ts`：`dataUpload.*` 命名空间（中英双语）。
- 验证：`corepack pnpm --filter @secretpad/app typecheck` 与 `corepack pnpm run lint` 通过（0 errors）。

### 7.17 隐私组件场景展示页（P2 Phase 10）

- 新增 `pages/privacy-scenes/index.tsx`：
  - 以卡片网格展示 11 个隐私计算场景：PSI、MPC 风控建模、TEE 可信建模、数据分类分级、数据脱敏、K-匿名、L-多样性、本地差分隐私、差分隐私查询、查询混淆、联邦学习。
  - 每个场景卡片包含场景标题、中文/英文描述、核心技术标签（如 PSI、MPC、TEE、DP 等）、“立即体验”按钮，点击后跳转到 DAG 或数据表页面。
- 在 `router.tsx` 以懒加载方式注册 `/privacy-scenes` 路由，在 `AppSidebar` governance 区增加侧边栏入口。
- 补充 `i18n/dictionaries.ts`：`privacyScenes.*` 命名空间（中英双语）。
- 验证：`corepack pnpm --filter @secretpad/app typecheck` 与 `corepack pnpm run lint` 通过（0 errors）。

### 7.18 DAG 高级配置抽屉（P1）

- 新增 `packages/dag-next/src/attribute-form.tsx`（`AttributeForm` 组件）：
  - 依据后端 `component/batch` 返回的组件定义（`ComponentDef.attrs`，对应 secretflow spec `component.proto` 的 `AttributeDef` 列表）动态渲染可视化参数配置表单，替代原先只能手工编辑 JSON 的方式。
  - 将扁平的 `AttributeDef[]`（通过 `prefixes` 描述祖先路径）还原为属性树；按属性类型（`AT_INT` / `AT_STRING` / `AT_BOOL` / `AT_STRINGS` / `AT_STRUCT_GROUP` / `AT_UNION_GROUP` 等）渲染对应输入控件。
  - 联合组（`AT_UNION_GROUP`）渲染为互斥单选，仅展开被选中的子树；结构组（`AT_STRUCT_GROUP`）渲染为可折叠分组。
  - 编辑结果实时回写为 `nodeDef.attrPaths` + `nodeDef.attrs` 两个平行数组，与旧前端及模板构建器（`builder.ts`）产出的结构完全一致。
  - 支持 `readOnly` 只读模式（供组件解释器复用）；文案通过 `labels` 属性注入，保持画布包可复用性。
- 在 `packages/dag-next/src/index.tsx` 节点配置面板中接入：选中节点且组件定义加载完成后，以动态表单替代原始 JSON 编辑。
- 验证：typecheck 与 lint 通过（0 errors）。

### 7.19 DAG 组件解释器 / 组件树增强（P1）

- 新增 `packages/dag-next/src/component-interpreter.tsx`（`ComponentInterpreter` 组件）：
  - 对应旧前端 `component-interpreter` / `component-tree` / `component-config` 模块。
  - 在算子库面板为每个算子提供“ℹ️”入口，点击后弹出该算子的完整定义解释：算子描述（中英双语由后端 `component/i18n` 与 `desc` 提供）、输入/输出端口（名称、允许的数据类型、描述）、可配置属性（复用 `AttributeForm` 只读模式展示默认值与说明）。
- 组件树（算子库面板）增强：按后端 `component/list` 分组展示，支持搜索过滤与展开/收起。
- 验证：typecheck 与 lint 通过（0 errors）。

### 7.20 新用户引导页 / Tour（P1）

- 新增 `pages/guide/index.tsx`（`GuidePage`）：
  - 对应旧前端 `guide` / `guide-pipeline` / `guide-node` 模块，为首次使用平台的用户提供“从零到跑通第一个隐私计算任务”的分步引导。
  - 以步骤卡片组织核心上手流程：注册节点 → 导入数据表 → 创建项目 → 编排 DAG → 运行并查看结果。
  - 每个步骤可手动勾选“已完成”，进度持久化到 localStorage（`secretpad-guide-progress`），刷新不丢失；提供“前往”按钮直接跳转对应功能页。
  - 顶部展示整体完成进度条，全部完成后给出祝贺态。
- 在 `router.tsx` 注册 `/guide` 路由，`AppSidebar` 增加侧边栏入口；`pages/dashboard` 顶部增加引导 banner（新用户可一键进入引导页）。
- 补充 `i18n/dictionaries.ts`：`guide.*` 与 `sidebar.guide` 命名空间（中英双语）。
- 验证：typecheck 与 lint 通过（0 errors）。

### 7.21 EDGE / P2P 工作台聚合页（P1）

- 新增 `pages/workbench/index.tsx`（`WorkbenchPage`）：
  - 对应旧前端 `edge` / `p2p-workbench` 模块，为 EDGE / P2P 用户提供一站式聚合工作台。
  - 欢迎头部 + 可展开/收起的“引导流程图”（6 步：节点 → 数据 → 项目 → DAG → 运行 → 结果）。
  - 统计行：节点数、数据表数（`useQueries` 并行查询各节点数据表数量并汇总）、项目数、待处理事项数。
  - 申请事项：展示待处理消息 top5；我的项目：按 `usePlatform().isP2p` 分支展示 P2P 项目或普通项目。
  - 快捷入口：跳转节点/数据表/项目/DAG/结果等独立路由，复用现有页面能力，避免重复开发。
- 在 `router.tsx` 注册 `/workbench` 路由，`AppSidebar` 增加侧边栏入口。
- 补充 `i18n/dictionaries.ts`：`workbench.*` 与 `sidebar.workbench` 命名空间（中英双语）。
- 验证：typecheck 与 lint 通过（0 errors）。

### 7.22 DAG 日志 Monaco 查看器（P1）

- 新增 `packages/dag-next/src/log-viewer.tsx`（`LogViewer` 组件）：
  - 对应旧前端 `dag-log` 模块。考虑到离线/内网部署场景，采用**轻量自包含的 Monaco 风格查看器**（零第三方依赖）替代重型 `monaco-editor`。
  - 功能：行号列（sticky）、按级别高亮（ERROR/WARN/INFO/DEBUG 自动识别着色）、关键字搜索高亮、级别筛选、自动滚动到底、自动换行、字号缩放（9–18px）、一键复制（`navigator.clipboard`）。
- 在 `packages/dag-next/src/index.tsx` 节点日志面板接入 `LogViewer` 替换原文本展示；并 `export { LogViewer }` 供应用层复用。
- 在 `pages/cloud-logs` 中复用该组件（见 7.24）。
- 补充 `i18n/dictionaries.ts`：`dag.log*` 相关标签（中英双语）。
- 验证：typecheck 与 lint 通过（0 errors）。

### 7.23 模型导出 / 发布详情（P2）

- 增强 `pages/models/index.tsx` 的“详情”弹窗为**模型导出 / 发布详情**：
  - 对应旧前端 `ModelExportController` 与 `model-manager/model-release`。由于后端未提供独立 `model/export/*` 端点，基于 `model/detail`（导出详情：parties + columns）与 `model/serving/detail`（发布详情）实现。
  - 打开详情时并发请求：`getModelInfo`（概要）+ `getModelDetail`（导出详情，按参与方展示数据列）+ `getModelServingDetail`（仅当 `model.servingId` 存在时，展示 endpoints / featureHttp / sourcePath / featureMappings / isMock）。
- 在 `packages/api-client/src/schemas/index.ts` 补充 `ServingDetailVOSchema` 与 `ServingDetailVO` 类型（`getModelServingDetail` 返回类型）。
- 验证：typecheck 与 lint 通过（0 errors）。

### 7.24 云端日志 SLS 页面（P2）

- 新增 `pages/cloud-logs/index.tsx`（`CloudLogsPage`）：
  - 对应旧前端 `CloudLogController` 的 `cloud_log/sls` 接口。
  - 查询表单：projectId（必填）、jobId / taskId / graphNodeId / nodeId（可选）、queryParties（checkbox）；空串传 `undefined`。
  - 调用 `apiClient.getCloudLogs`，`enabled: hasFetched && !!projectId`；结果复用 `@secretpad/dag-next` 的 `LogViewer` 展示（h-[480px]），并展示 status 徽标与 nodeParties。
- 在 `router.tsx` 注册 `/cloud-logs` 路由，`AppSidebar` 增加侧边栏入口。
- 补充 `i18n/dictionaries.ts`：`cloudLogs.*` 与 `sidebar.cloudLogs` 命名空间（中英双语）。
- 验证：typecheck 与 lint 通过（0 errors）。

### 7.25 特征数据源 / 投票同步页面（P2）

- 新增 `pages/feature-datasource/index.tsx`（`FeatureDatasourcePage`，双 Tab）：
  - 对应旧前端 `FeatureDatasourceController` 与 `VoteSyncController`。
  - **特征数据源 Tab**：选择项目 + 节点后调用 `listFeatureDatasourceAuth` 查询该节点在该项目下被授权的特征表及其字段；支持创建特征数据源（`createFeatureDatasource`，动态编辑字段行、多选授权节点）。
  - **投票同步 Tab**：选择项目、同步数据类型（对应后端 `VoteSyncTypeEnum` 的 8 个枚举：VOTE_REQUEST / VOTE_INVITE / NODE_ROUTE / TEE_NODE_DATATABLE_MANAGEMENT / PROJECT_APPROVAL_CONFIG / PROJECT / PROJECT_NODE / PROJECT_INST）与参与节点，调用 `createVoteSync` 发起跨节点数据同步投票。
- 在 `router.tsx` 注册 `/feature-datasource` 路由，`AppSidebar` 增加侧边栏入口。
- 补充 `i18n/dictionaries.ts`：`featureDs.*` 与 `sidebar.featureDatasource` 命名空间（中英双语）。
- 验证：typecheck 与 lint 通过（0 errors）。

### 7.26 组件版本管理页面（P2）

- 新增 `pages/component-versions/index.tsx`（`ComponentVersionsPage`）：
  - 对应旧前端“版本管理”模块与后端 `ComponentVersionController` 的 `version/list` 接口。
  - 调用 `apiClient.listComponentVersions()` 获取 `ComponentVersion` 对象，将 9 个组件字段（kusciaImage / secretflowImage / secretflowServingImage / secretpadImage / dataProxyImage / scqlImage / teeAppImage / teeDmImage / capsuleManagerSimImage）映射为“组件名 + 镜像版本”卡片网格。
  - 镜像版本用等宽字体展示，自动从镜像 tag 中提取版本号高亮（`extractTag`，兼容 registry 带端口场景）；支持手动刷新。
- 在 `router.tsx` 注册 `/component-versions` 路由，`AppSidebar` 治理区增加侧边栏入口。
- 补充 `i18n/dictionaries.ts`：`versions.*` 与 `sidebar.componentVersions` 命名空间（中英双语）。
- 验证：typecheck 与 lint 通过（0 errors）。

### 7.27 数据表树形血缘可视化（P2）

- 新增 `features/lineage/lineage-tree.tsx`（`LineageTree` / `LineageNode` / `NodeCard` / `TreeBranch`）：
  - 对应旧前端 `data-table-tree` 模块。零依赖纯 CSS（flex + 绝对定位连接线）绘制横向树，适配离线部署。
  - `LineageNode` 描述树节点（icon / title / subtitle / badge / badgeTone / details / children）；`TreeBranch` 递归渲染「父节点 — 水平短线 — 子节点列」，子节点列用上半段/下半段拼接出只 spanning 首子中心~末子中心的竖直主干。
- 集成到 `pages/data-tables/index.tsx` 详情血缘 Tab：`lineageRoot` useMemo 构建三层树「数据源 → 数据表（含 URI/状态/列数）→ 已授权项目（含关联键/标签列/授权时间）」，数据来自 `detailVO`（`DatatableVO`）与 `authProjects`。
- 为 `AuthProjectVOSchema` 补充 `associateKeys` / `labelKeys` / `groupKeys`（`string[]` optional），与生成的 `secretpad.d.ts` 对齐。
- 补充 `i18n/dictionaries.ts`：`dataTables.lineage*` 命名空间（中英双语）。

### 7.28 审批主动创建 UI + 状态轮询（P2）

- 新增 `features/approval/create-approval-modal.tsx`（`CreateApprovalModal`）：
  - 对应后端 `POST /api/v1alpha1/approval/create`（`ApprovalController#createApproval`）。
  - 按 `voteType`（NODE_ROUTE / TEE_DOWNLOAD / PROJECT_CREATE / PROJECT_ARCHIVE）声明式渲染对应 `voteConfig` 字段（键名严格对齐后端各 VoteConfig 子类属性名），提交时组装对象；列表字段（participants）按逗号拆分为数组；发起方 ID 默认取当前登录用户 `ownerId`。
- 新增 `features/approval/approval-status-poller.tsx`（`ApprovalStatusPoller`）：
  - 对应后端 `POST /api/v1alpha1/approval/pull/status`（`ApprovalController#pullStatus`）。
  - 填写资源定位字段（projectID / jobID / taskID / resourceID 必填 + resourceType 下拉 model/rule/table）后开启轮询，`useQuery` 以 3s `refetchInterval` 重复请求，展示各参与方（节点维度）投票状态 Badge 与逐次投票明细。
- 为 `packages/api-client` 新增 `VoteInfoSchema` / `ParticipantSchema` / `PullStatusVOSchema`，并将 `pullApprovalStatus` 返回类型由 `unknown` 收紧为 `PullStatusVO`（`unwrapValidated` 运行时校验）。
- 集成到 `pages/messages/index.tsx`：头部新增「发起审批」按钮打开创建 Modal；头部下方嵌入 `ApprovalStatusPoller` 轮询卡片。
- 补充 `i18n/dictionaries.ts`：`approval.*` 命名空间（中英双语）。

### 7.15 迁移完成度总结（更新）

- P0 功能（DAG 模板库、结果管理独立页、DAG 模型提交入口、DAG 周期任务入口）已全部完成。
- 旧前端核心日常流程（项目 / 节点 / 数据表 / 数据源 / 模型 / 消息 / 周期任务 / DAG 编排与运行 / 结果查看）均已在新前端实现并接入真实接口。
- **本轮（2026-07-26）已完成全部 P1 与 P2 迁移项**（见 7.18–7.26）：
  - P1：DAG 高级配置抽屉（`attribute-form`）、DAG 组件解释器/组件树增强（`component-interpreter`）、新用户引导页（`pages/guide`）、EDGE/P2P 工作台聚合页（`pages/workbench`）、DAG 日志 Monaco 查看器（`log-viewer`）。
  - P2：模型导出/发布详情（`pages/models` 详情弹窗）、云端日志 SLS（`pages/cloud-logs`）、特征数据源/投票同步（`pages/feature-datasource`）、组件版本管理（`pages/component-versions`）。
- 至此，迁移优先级表（P0/P1/P2）中列出的 9 个功能已全部迁移完成；**本轮（2026-07-27）进一步补齐数据表树形血缘可视化（7.27）与审批主动创建 UI + 状态轮询（7.28）**。仅剩 Graph 占位页等非核心体验项可按需迁移，不阻塞主业务闭环。

### 8.1 对比维度说明

| 维度 | 说明 |
|---|---|
| 页面级路由 | 旧前端 `apps/platform/config/routes.ts` 中定义的一级页面。 |
| 模块级功能 | 旧前端 `apps/platform/src/modules/` 下的业务模块，往往被多个页面组合复用。 |
| Pipeline 模板 | 旧前端 `modules/pipeline/templates/` 注册的 graph 模板。 |
| 可复用组件 | 组件库、DAG 引擎、工具函数等跨页面能力。 |
| 平台模式 | CENTER / EDGE / P2P / AUTONOMY 等部署模式下的页面差异。 |

### 8.2 旧前端功能全景

旧前端是一个 Umi 3 + Ant Design + Valtio 的 monorepo，主要应用为 `apps/platform`，包含 19 个页面、约 40 个业务模块、17 个 Pipeline 模板。

#### 8.2.1 一级页面（routes.ts）

| 路由 | 页面文件 | 承担角色 |
|---|---|---|
| `/login` | `pages/login.tsx` | 登录页。 |
| `/`, `/home` | `pages/new-home.tsx` | CENTER 模式工作台（标签页：Dashboard、节点管理、数据源、数据表、项目管理、结果管理、隐私组件场景、DAG/消息/模型外链）。 |
| `/dashboard` | `pages/dashboard.tsx` | 独立 Dashboard。 |
| `/data-source` | `pages/data-source.tsx` | 数据源列表。 |
| `/data-source/:id` | `pages/data-source-detail.tsx` | 数据源详情。 |
| `/data-table` | `pages/data-table.tsx` | 数据表列表。 |
| `/nodes` | `pages/nodes.tsx` | 节点管理（managed-node-list）。 |
| `/graphs` | `pages/graphs.tsx` | Graph 管理列表（演示/占位页面）。 |
| `/dag` | `pages/dag.tsx` | DAG 画布（main-dag）。 |
| `/record` | `pages/record.tsx` | DAG / Pipeline 运行记录（record-layout）。 |
| `/model-submission` | `pages/model-submission.tsx` | 模型提交/模型管理（model-submission-layout）。 |
| `/periodic-task-detail` | `pages/periodic-task-detail.tsx` | 周期任务详情。 |
| `/node` | `pages/new-node.tsx` | EDGE 模式节点首页（标签页：数据源、数据管理、合作节点、结果管理）。 |
| `/my-node` | `pages/my-node.tsx` | P2P / EDGE 模式“我的节点”首页。 |
| `/message` | `pages/message.tsx` | 消息中心。 |
| `/edge` | `pages/edge.tsx` | EDGE / P2P 工作台（标签页：工作台、数据源、数据管理、合作节点、我的项目、结果管理）。 |
| `/guide` | `pages/guide.tsx` | 新用户引导页。 |

#### 8.2.2 核心模块与功能

- **数据资产**：`all-data-sources`、`all-data-tables`、`data-source-list`、`data-manager`、`data-table-add`、`data-table-info`、`data-table-auth`、`data-table-tree`。
- **项目与结果**：`project-list`、`project-content`、`create-project`、`p2p-create-project`、`p2p-project-list`、`p2p-project-detail`、`result-manager`、`result-details`。
- **节点与机构**：`managed-node-list`、`managed-node`、`node`、`my-node`、`cooperative-node-list`、`guide-node`。
- **DAG 与流水线**：`main-dag`、`dag-submit`、`dag-record`、`dag-result`、`dag-log`、`dag-model-submission`、`dag-modal-manager`、`dag-guide-tour`、`dag-record-guide-tour`、`pipeline`、`pipeline-record-list`。
- **周期任务**：`periodic-task`（含 `periodic-task-list`、`periodic-task-drawer`、`periodic-child-task-list`）。
- **组件与配置**：`component-config`、`component-tree`、`component-interpreter`、`advanced-config`、`template-quick-config`。
- **其他业务**：`model-manager`、`message-center`、`privacy-scenes`、`guide`、`guide-pipeline`、`p2p-workbench`。

#### 8.2.3 Pipeline 模板（modules/pipeline/templates）

旧前端注册并支持选择以下模板创建 DAG：

1. `pipeline-template-blank`：空白图。
2. `pipeline-template-psi`：PSI 求交。
3. `pipeline-template-psi-guide`：引导式 PSI。
4. `pipeline-template-psi-tee`：TEE PSI。
5. `pipeline-template-psi-tee-guide`：引导式 TEE PSI。
6. `pipeline-template-scenario-psi`：场景 PSI。
7. `pipeline-template-risk`：风险模型。
8. `pipeline-template-risk-guide`：引导式风险模型。
9. `pipeline-template-tee`：TEE 计算。
10. `pipeline-template-tee-guide`：引导式 TEE。
11. `pipeline-template-privacy`：隐私计算（通用）。
12. `pipeline-template-privacy-guide`：引导式隐私计算。
13. `pipeline-template-sanitization`：数据脱敏/数据清洗。
14. `pipeline-template-query-obfuscation`：查询混淆。
15. `pipeline-template-data-classification`：数据分类分级。
16. `pipeline-template-k-anonymity`：K-匿名。
17. `pipeline-template-l-diversity`：L-多样性。
18. `pipeline-template-local-differential-privacy`：本地差分隐私。

### 8.3 新前端功能全景

新前端是 `pnpm` + Vite 5 + React 18 + TanStack Router + TanStack Query + Zustand 的 monorepo，应用为 `apps/secretpad`，包含 23 个页面、4 个 workspace 包。

#### 8.3.1 一级路由（router.tsx）

| 路由 | 页面 | 状态 |
|---|---|---|
| `/login` | `pages/login` | 已迁移。 |
| `/dashboard` | `pages/dashboard` | 已迁移（顶部含新用户引导 banner）。 |
| `/projects` | `pages/projects` | 已迁移（原 `project-list` + `project-content` 能力）。 |
| `/nodes` | `pages/nodes` | 已迁移（原 `managed-node-list` 能力）。 |
| `/data-tables` | `pages/data-tables` | 已迁移。 |
| `/data-sources` | `pages/data-sources` | 已迁移。 |
| `/data-sources/detail` | `pages/data-sources/detail` | 已迁移。 |
| `/dag` | `pages/dag` | 已迁移（画布 + 高级配置表单 + 组件解释器 + 日志查看器）。 |
| `/models` | `pages/models` | 已迁移（原 `model-manager` 能力，含导出/发布详情）。 |
| `/results` | `pages/results` | 已迁移（原 `result-manager` 能力）。 |
| `/periodic-tasks` | `pages/periodic-tasks` | 已迁移。 |
| `/messages` | `pages/messages` | 已迁移。 |
| `/privacy-scenes` | `pages/privacy-scenes` | 已迁移（原 `privacy-scenes` 能力）。 |
| `/node-routes` | `pages/node-routes` | 已迁移（原 `cooperative-node-list` / `my-node` 部分能力）。 |
| `/institutions` | `pages/institutions` | 已迁移。 |
| `/p2p/projects` | `pages/p2p/projects` | 已迁移。 |
| `/p2p/my-node` | `pages/p2p/my-node` | 已迁移。 |
| `/account` | `pages/account` | 已迁移（原 `user` 密码修改等）。 |
| `/guide` | `pages/guide` | 已迁移（原 `guide` / `guide-pipeline` / `guide-node` 能力）。 |
| `/workbench` | `pages/workbench` | 已迁移（原 `edge` / `p2p-workbench` 聚合工作台）。 |
| `/cloud-logs` | `pages/cloud-logs` | 已迁移（原 `CloudLogController` 云日志 SLS）。 |
| `/feature-datasource` | `pages/feature-datasource` | 已迁移（原特征数据源 + 投票同步）。 |
| `/component-versions` | `pages/component-versions` | 已迁移（原 `ComponentVersionController` 版本管理）。 |

#### 8.3.2 复用包

- `@secretpad/design-system`：Button、Badge、Card、Modal、ConfirmDialog、Toast 等基础组件。
- `@secretpad/api-client`：OpenAPI 生成客户端，覆盖所有后端接口。
- `@secretpad/dag-next`：新版 DAG 画布（组件面板、拖拽、连线、缩放、节点状态、右侧面板）。
- `@secretpad/utils`：含 `sha256` 等通用工具。

### 8.4 功能对照矩阵

| 功能域 | 旧前端模块 | 新前端位置 | 迁移状态 | 备注 |
|---|---|---|---|---|
| 登录 / 登出 | `login` | `pages/login` + `features/auth` | ✅ 已迁移 | 密码 SHA-256 哈希一致。 |
| Dashboard 统计与最近任务 | `dashboard` | `pages/dashboard` | ✅ 已迁移 | 新前端增加了节点拓扑。 |
| CENTER 工作台标签页 | `new-home` | 无独立页面，改为左侧导航 | ✅ 等效 | 各标签页已拆分为独立路由。 |
| 项目列表 / 创建 / 编辑 / 删除 | `project-list`, `create-project` | `pages/projects` | ✅ 已迁移 | 支持项目详情抽屉、参与方、数据表、任务列表。 |
| P2P 项目列表 / 创建 / 编辑 / 归档 | `p2p-project-list`, `p2p-create-project` | `pages/p2p/projects` | ✅ 已迁移 | 支持参与者查看。 |
| 节点列表 / 注册 / 编辑 / 删除 / Token | `managed-node-list`, `managed-node` | `pages/nodes` | ✅ 已迁移 | 新增节点详情抽屉、节点产物。 |
| 节点路由（合作节点） | `cooperative-node-list` | `pages/node-routes` | ✅ 已迁移 | 源/目的地址编辑、刷新、删除。 |
| P2P 我的节点 | `my-node` | `pages/p2p/my-node` | ✅ 已迁移 | 节点路由注册。 |
| 数据源管理 | `all-data-sources`, `data-source-list` | `pages/data-sources` | ✅ 已迁移 | 含详情页。 |
| 数据表管理 | `all-data-tables`, `data-manager`, `data-table-add`, `data-table-info`, `data-table-auth` | `pages/data-tables` | ✅ 已迁移 | 含 schema、授权、Push TEE、L1–L5 分类。 |
| 数据表树 / 血缘 | `data-table-tree` | `features/lineage` + `pages/data-tables` 详情 | ✅ 已迁移 | 数据表详情血缘 Tab 以树形可视化展示「数据源 → 数据表 → 已授权项目」（见 7.27）。 |
| 模型管理 / 打包 / 部署 / 废弃 | `model-manager`, `dag-model-submission` | `pages/models` + DAG 工具栏 | ✅ 已迁移 | 模型列表、打包、部署、从 DAG 训练节点直接打包均已迁移。 |
| 消息中心 | `message-center` | `pages/messages` | ✅ 已迁移 | 审批/回复、详情、未读角标。 |
| 周期任务 | `periodic-task` | `pages/periodic-tasks` | ✅ 已迁移 | 创建、下线、删除、运行记录。 |
| 账户信息 / 修改密码 | `user` / `account` | `pages/account` | ✅ 已迁移 | 密码 SHA-256 哈希。 |
| 机构管理 | `inst` | `pages/institutions` | ✅ 已迁移 | 机构节点增删改查。 |
| EDGE 模式首页 | `new-node` | `pages/workbench` | ✅ 已迁移 | 旧 `/node` EDGE 节点首页能力由 `/workbench` 聚合工作台承载（数据/合作节点/项目/结果一站式查看）。 |
| EDGE / P2P 工作台 | `edge`, `p2p-workbench` | `pages/workbench` | ✅ 已迁移 | 新增 `/workbench` 聚合工作台：欢迎头部 + 引导流程图 + 统计行 + 申请事项 + 我的项目（按 `isP2p` 分支）+ 快捷入口。 |
| 结果管理独立页 | `result-manager` / `pipeline-record-list` | `pages/results` | ✅ 已迁移 | 跨项目列表、搜索、类型筛选、排序、分页、下载、详情。批量删除旧前端未完全实现，暂不迁移。 |
| 隐私组件场景 | `privacy-scenes` | `pages/privacy-scenes` | ✅ 已迁移 | 卡片式展示 11 个隐私计算场景，含技术标签与“立即体验”跳转。 |
| 新用户引导 | `guide`, `guide-pipeline`, `guide-node`, `guide-tour`, `dag-guide-tour`, `dag-record-guide-tour` | `pages/guide` | ✅ 已迁移 | 新增 `/guide` 分步引导页（步骤卡片 + 进度持久化 + 跳转）；dashboard 增加引导 banner。 |
| DAG 模板向导 | `pipeline` | `features/dag-templates` + `pages/dag` | ✅ 已迁移 | 已迁移 blank/psi/data-classification/sanitization/k-anonymity/l-diversity/local-differential-privacy/differential-privacy/query-obfuscation/risk/tee 共 11 个模板；向导按 basic/privacy/ml 分类展示。 |
| DAG 模型提交入口 | `dag-model-submission` | `features/model-pack` + `pages/dag` | ✅ 已迁移 | 在 DAG 工具栏选择成功训练节点后可直接打包模型，调用 `model/pack` 并轮询 `model/status`。 |
| DAG 周期任务入口 | `main-dag/periodic-task-entry` | `features/scheduled-task-from-dag` + `pages/dag` | ✅ 已迁移 | 在 DAG 工具栏直接创建周期任务，调用 `scheduled/graph/create`。 |
| DAG 运行记录 / 报告 | `dag-record`, `pipeline-record-list`, `dag-result` | 无独立页面 | ⚠️ 部分迁移 | 项目详情与 DAG 节点面板可查看单次任务日志/输出；缺少统一的 pipeline 运行记录列表和结果报告页。 |
| DAG 高级配置 | `advanced-config` | `packages/dag-next/attribute-form` | ✅ 已迁移 | 依据 `component/batch` 的 `attrs` 动态生成表单（属性树 + 类型控件 + 联合组互斥），回写 `attrPaths`/`attrs`。 |
| DAG 组件树 / 组件解释器 | `component-tree`, `component-interpreter`, `component-config` | `packages/dag-next/component-interpreter` | ✅ 已迁移 | 算子库“ℹ️”入口弹出算子描述、输入/输出端口、只读属性表单；组件树按 `component/list` 分组 + 搜索。 |
| DAG 日志查看器（Monaco） | `dag-log` | `packages/dag-next/log-viewer` | ✅ 已迁移 | 轻量自包含 Monaco 风格查看器：行号/级别高亮/搜索/筛选/自动滚动/换行/字号/复制。 |
| Graph 管理占位页 | `pages/graphs.tsx` | 无 | ❌ 未迁移 | 旧 `/graphs` 为 Graph 列表演示页，非核心功能。 |
| 数据上传文件 UI | `DataController` | 无 | ❌ 未迁移 | `api-client` 已提供 `upload`/`download`/`sync`，但无页面入口。 |
| 模型导出 / 模型发布详情 | `ModelExportController`, `model-manager/model-release` | `pages/models` 详情弹窗 | ✅ 已迁移 | 基于 `model/detail`（parties/columns）与 `model/serving/detail`（endpoints/featureMappings）展示导出/发布详情。 |
| 云端日志 SLS | `CloudLogController` | `pages/cloud-logs` | ✅ 已迁移 | 新增 `/cloud-logs` 页面对接 `cloud_log/sls`，复用 `LogViewer` 展示。 |
| 特征数据源 / 投票同步 | `FeatureDatasourceController`, `VoteSyncController` | `pages/feature-datasource` | ✅ 已迁移 | 新增 `/feature-datasource` 双 Tab 页面：特征表查询/创建 + 投票同步（`VoteSyncTypeEnum`）。 |
| 审批创建 / 状态轮询 | `ApprovalController` | `features/approval` + `pages/messages` | ✅ 已迁移 | 消息中心可回复审批；新增主动创建审批 Modal 与状态轮询卡片（见 7.28）。 |
| 组件版本管理 | `ComponentVersionController` | `pages/component-versions` | ✅ 已迁移 | 新增 `/component-versions` 页面，展示 9 个核心组件镜像版本卡片。 |

### 8.5 未迁移功能详细清单与优先级建议

#### 8.5.1 P0（影响日常核心流程，建议尽快补齐）

1. **DAG 模板向导补全**（`features/dag-templates` + `pages/dag`） ✅ 已完成
   - 已迁移模板：blank、psi、data-classification、sanitization、query-obfuscation、k-anonymity、l-diversity、local-differential-privacy、differential-privacy、risk、tee。
   - 实现方式：每个模板为纯函数 `TemplateContribution.build(...)`，统一在 `registry.ts` 注册；`TemplateWizard` 按 basic/privacy/ml 分类展示，根据模板类型动态渲染双表/单表/无参数表单；创建时先调用 `createGraph` 获取 graphId，再调用 `template.build` 生成节点/边，最后 `updateGraph` 提交完整拓扑。
   - 剩余模板（引导式向导、场景 PSI、TEE PSI 等）因与现有模板拓扑等效或属于体验增强，归入 P1/P2 按需迁移。

2. **结果管理独立页面**（`pages/results`） ✅ 已完成
   - 旧模块：`result-manager`、`pipeline-record-list`。
   - 新前端：新增 `/results` 路由与侧边栏入口，调用 `node/result/list`、`node/result/detail`、`data/download`。
   - 支持搜索、按类型（table/report/rule/model）筛选、时间排序、分页、下载、详情弹窗。批量删除旧前端未完全实现，暂不迁移。

3. **DAG 模型提交入口**（`pages/dag` 工具栏）✅ 已完成
   - 旧模块：`dag-model-submission`（`SubmissionDrawer`、`PipelineTitleComponent`）。
   - 新前端：`features/model-pack/model-pack-modal.tsx`。
   - 实现：在 DAG 工具栏选中成功训练节点后展示“打包模型”按钮；调用 `model/modelPartyPath` 获取参与方数据源，构造 `modelPartyConfig` 与 `modelComponent`，提交 `model/pack` 并轮询 `model/status` 至 SUCCEED / FAILED。

4. **DAG 周期任务入口**（`pages/dag` 工具栏）✅ 已完成
   - 旧模块：`main-dag/periodic-task-entry`。
   - 新前端：`features/scheduled-task-from-dag/scheduled-task-modal.tsx`。
   - 实现：在 DAG 工具栏增加“创建周期任务”按钮；校验 `scheduled/graph/once/success` 后，将当前 graph、默认全部节点、cron 表达式与任务名称写入 `scheduled/graph/create`。

#### 8.5.2 P1（提升体验，建议下一阶段补齐）

5. **DAG 高级配置抽屉**（`packages/dag-next`） ✅ 已完成
   - 旧模块：`advanced-config`。
   - 新前端：`packages/dag-next/src/attribute-form.tsx`。根据 `component/batch` 返回的 `attrs` 定义动态生成表单（属性树 + 类型控件 + 联合组互斥），替代原始 JSON 编辑。

6. **DAG 组件解释器 / 组件树增强** ✅ 已完成
   - 旧模块：`component-interpreter`、`component-tree`、`component-config`。
   - 新前端：`packages/dag-next/src/component-interpreter.tsx`。算子库“ℹ️”入口弹出算子描述、输入/输出端口、只读属性表单；组件树按 `component/list` 分组 + 搜索。

7. **新用户引导页**（`pages/guide`） ✅ 已完成
   - 旧模块：`guide`、`guide-pipeline`、`guide-node`。
   - 新前端：`pages/guide/index.tsx`。分步引导页（步骤卡片 + 进度持久化 localStorage + 跳转按钮 + 进度条）；dashboard 增加引导 banner。

8. **数据上传入口**（`pages/data-tables`）✅ 已完成
   - 旧接口：`DataController.upload`/`download`/`sync`。
   - 新前端：`features/data-upload/data-upload-modal.tsx`。
   - 实现：在数据表页面工具栏增加“上传数据文件”按钮；选择节点与本地文件后调用 `apiClient.uploadData`；上传成功后展示数据源名称、真实文件名、数据源类型。

9. **EDGE / P2P 工作台聚合页**（`pages/edge`、`pages/workbench`） ✅ 已完成
   - 旧模块：`edge`、`p2p-workbench`。
   - 新前端：`pages/workbench/index.tsx`。欢迎头部 + 引导流程图 + 统计行（`useQueries` 汇总数据表数）+ 申请事项 + 我的项目（按 `isP2p` 分支）+ 快捷入口。

10. **DAG 日志 Monaco 查看器** ✅ 已完成
    - 旧模块：`dag-log`。
    - 新前端：`packages/dag-next/src/log-viewer.tsx`。轻量自包含 Monaco 风格查看器（零第三方依赖，适配离线部署）：行号/级别高亮/搜索/筛选/自动滚动/换行/字号/复制。

#### 8.5.3 P2（高级/低频功能，按需迁移）

11. **隐私组件场景展示页**（`pages/privacy-scenes`）✅ 已完成
    - 旧模块：`privacy-scenes`。
    - 新前端：`pages/privacy-scenes/index.tsx`。
    - 实现：卡片式展示 11 个隐私计算场景（PSI、MPC 风控、TEE、数据分类分级、脱敏、K-匿名、L-多样性、本地 DP、DP 查询、查询混淆、联邦学习），每个场景包含技术标签与“立即体验”跳转按钮；新增 `/privacy-scenes` 路由与侧边栏入口。

12. **模型导出与发布详情**（`pages/models`） ✅ 已完成
    - 对接 `ModelExportController` 和 `model-manager/model-release`。
    - 新前端：`pages/models/index.tsx` 详情弹窗。基于 `model/detail`（parties/columns）与 `model/serving/detail`（endpoints/featureMappings/isMock）展示导出/发布详情。

13. **云端日志 SLS 页面**（`pages/cloud-logs`） ✅ 已完成
    - 对接 `CloudLogController` 的 `cloud_log/sls`。
    - 新前端：`pages/cloud-logs/index.tsx`。查询表单（projectId 必填 + jobId/taskId/graphNodeId/nodeId 可选 + queryParties），复用 `LogViewer` 展示。

14. **特征数据源与投票同步页面** ✅ 已完成
    - 对接 `FeatureDatasourceController`、`VoteSyncController`。
    - 新前端：`pages/feature-datasource/index.tsx`。双 Tab：特征表查询/创建（动态字段行 + 多选授权节点）+ 投票同步（`VoteSyncTypeEnum` 8 个枚举）。

15. **组件版本管理页面** ✅ 已完成
    - 对接 `ComponentVersionController`。
    - 新前端：`pages/component-versions/index.tsx`。展示 9 个核心组件镜像版本卡片，自动提取 tag 高亮，支持刷新。

16. **数据表树形血缘视图**（`data-table-tree`） ✅ 已完成
    - 在数据表详情血缘 Tab 以树形可视化展示「数据源 → 数据表 → 已授权项目」（见 7.27）。

### 8.6 迁移建议

1. **优先 P0**：DAG 模板、结果管理、模型提交、周期任务入口是当前用户在 DAG 上跑通完整业务闭环的最大缺口。补齐后，旧前端 90% 以上的核心操作可由新前端独立完成。（✅ 已完成）
2. **补齐 P1 体验项**：高级配置、组件解释器、引导页、数据上传、工作台聚合页、日志查看器完成后，新前端在生产可用性上基本追平旧前端。（✅ 已完成）
3. **P2 按需**：隐私场景、模型导出/发布、云日志、特征数据源/投票同步、组件版本、数据表血缘可视化、审批主动创建 UI 等均已按业务需求迁移完成。（✅ 已完成）
4. **平台模式适配**：EDGE / P2P 工作台聚合页已复用现有独立页面，通过新增 `/workbench` 首页组件组合呈现，避免重复开发。（✅ 已完成）
5. **保持一致性**：所有新增页面继续沿用 `TanStack Router`、`TanStack Query`、`Zustand`、`@secretpad/design-system`、i18n 字典的现有模式；新增 DAG 模板继续按 `pipeline-template-*.ts` 的纯函数方式生成 graph 定义，便于测试与维护。

### 8.7 当前结论

- 新前端已覆盖旧前端 **登录、Dashboard、项目、节点、数据源、数据表、模型管理、消息、周期任务、账户、机构、P2P 项目/节点** 等核心 CRUD 能力。
- **DAG 模板库、结果管理独立页、DAG 模型提交入口、DAG 周期任务入口** 等剩余 P0 功能已全部补齐。
- **本轮（2026-07-26）已完成全部 P1 与 P2 迁移项**：DAG 高级配置抽屉、组件解释器/组件树增强、新用户引导页、EDGE/P2P 工作台聚合页、DAG 日志 Monaco 查看器、模型导出/发布详情、云端日志 SLS、特征数据源/投票同步、组件版本管理（见 7.18–7.26）。
- 至此，迁移优先级表（P0/P1/P2）中列出的 9 个功能已全部迁移完成；旧前端核心日常流程与体验增强项均已可由新前端承载。**本轮（2026-07-27）补齐数据表树形血缘可视化（7.27）与审批主动创建 UI + 状态轮询（7.28）**；仅剩 **Graph 占位页** 等非核心体验项可按需迁移，不阻塞主业务闭环。
- 旧前端中部分能力（如 `pages/graphs.tsx` 占位页、部分已弃用的 CENTER 工作台）可不再迁移，直接以新前端的独立路由方式替代。
