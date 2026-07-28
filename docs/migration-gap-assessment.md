# 迁移版功能差距评估与补齐方案

> 评估对象
> - **迁移版**：前端 `clwork/privahub/web`（privaconsole，vite + React + Tailwind + FSD），后端 `clwork/privahub`（Go + Gin + GORM）
> - **全功能版**：前端 `sfwork/secretpad/frontend-src`（umi + AntD5），后端 `sfwork/secretpad`（Java + Spring Boot）
>
> 评估日期：2026-07-28

---

## 一、背景与目标

`clwork/privahub` 是从 `sfwork/secretpad` 迁移而来的 Go 重构版后端，`clwork/privahub/web` 是配套的全新前端（Feature-Sliced 架构）。迁移版前端在重写时已按照全功能版的业务能力定义了完整的 API 契约（`web/openapi/secretpad.openapi.json` + `packages/api-client`），但 Go 后端尚未实现其中全部端点，导致部分页面功能在迁移版中不可用。

本文目标：

1. 系统评估迁移版（前端 + 后端）与全功能版的差距；
2. 给出可执行的补齐方案；
3. 作为后续补齐实现的依据。

---

## 二、评估结论（TL;DR）

| 维度 | 结论 |
| --- | --- |
| **前端页面覆盖** | ✅ 基本完整。`web` 已覆盖全部 24 个页面 + 8 个 feature，与全功能版业务能力一一对应，无缺失页面。 |
| **前端 API 契约** | ✅ 完整。`packages/api-client` 已定义约 90 个端点的调用与 Zod 运行时校验。 |
| **后端 API 实现** | ⚠️ **存在 16 个端点缺失**。前端已调用、OpenAPI 已定义，但 Go 后端未注册/未实现，是功能不可用的根因。 |
| **补齐重点** | 🔧 **后端**。实现 16 个缺失端点即可打通迁移版全部功能；前端无需新增页面。 |

**核心差距 = 后端 16 个缺失的 API 端点。**

---

## 三、评估方法

1. 提取全功能版后端全部 Controller 的 `@*Mapping` 端点清单（26 个 Controller）；
2. 提取迁移版后端 `internal/controller/http/router.go` 已注册路由清单；
3. 提取迁移版前端 `packages/api-client/src/client.ts` 实际调用的端点清单（约 90 个）；
4. 三方交叉比对：以**前端实际调用**为准，筛出「前端要调、后端没有」的端点；
5. 对每个缺失端点，核对前端 Zod schema / OpenAPI 契约，确认请求/响应结构，并确认其在页面中的真实使用情况。

---

## 四、后端差距明细（16 个缺失端点）

以下端点均满足：**迁移版前端 `client.ts` 已调用** + **OpenAPI 已定义契约** + **Go 后端 `router.go` 未注册**。按模块分组：

### 4.1 结果管理模块（Node Result，2 个）

支撑页面：`pages/results`（结果管理）、结果详情。

| 端点 | 前端方法 | 用途 | 使用文件数 |
| --- | --- | --- | --- |
| `POST /node/result/list` | `listNodeResults` | 分页列出节点产出的结果（支持节点/类型/名称过滤、时间排序） | 2 |
| `POST /node/result/detail` | `getNodeResultDetail` | 结果详情（含图详情、列信息、输出预览、数据源） | 2 |

- 响应契约：`AllNodeResultsListVO { nodeAllResultsVOList[], totalNodeResultNums }`、`NodeResultDetailVO { nodeResultsVO, graphDetailVO, tableColumnVOList, output, datasource }`。

### 4.2 数据源模块（Datasource，1 个）

支撑页面：`pages/data-sources`（数据源详情）。

| 端点 | 前端方法 | 用途 | 使用文件数 |
| --- | --- | --- | --- |
| `POST /datasource/nodes` | `getDataSourceNodes` | 查询某数据源关联的节点列表 | 1 |

- 响应契约：`DatasourceNodesVO { nodes: [{nodeId, nodeName, status}] }`。

### 4.3 数据表模块（Datatable，3 个）

支撑页面：`pages/data-tables`、数据上传、项目数据表配置。

| 端点 | 前端方法 | 用途 | 使用文件数 | 备注 |
| --- | --- | --- | --- | --- |
| `POST /datatable/create` | `createDataTable` | 注册数据表 | 1 | Go 现有 `/datatable/register`，需补 `/create` 别名/实现 |
| `POST /datatable/get` | `getDataTable` | 查询数据表详情 | 5 | Go 现有 `/datatable/detail`，需补 `/get` |
| `POST /datatable/pushToTee` | `pushDatatableToTee` | 推送数据表至 TEE 节点 | 1 | 缺失 |

- 响应契约：`DatatableNodeVO { datatableVO, nodeName, nodeId }`。

### 4.4 周期任务模块（Scheduled，9 个）

支撑页面：`pages/periodic-tasks`（周期任务）、`features/scheduled-task-from-dag`（从 DAG 创建周期任务）。

| 端点 | 前端方法 | 用途 | 使用文件数 | 备注 |
| --- | --- | --- | --- | --- |
| `POST /scheduled/graph/create` | `createScheduledGraph` | 基于图创建周期任务（含 Cron 配置） | 2 | Go 现有 `/scheduled/create`（入参为 cron 字符串），需补结构化 `/graph/create` |
| `POST /scheduled/id` | `getScheduledId` | 按 project+graph 查询 scheduleId | 0 | 缺失 |
| `POST /scheduled/info` | `getScheduledInfo` | 周期任务详情（返回 ProjectJobVO） | 0 | 缺失 |
| `POST /scheduled/task/page` | `getScheduledTaskPage` | 周期任务的执行实例分页 | 1 | 缺失 |
| `POST /scheduled/task/rerun` | `rerunScheduledTask` | 重跑某次执行实例 | 1 | 缺失 |
| `POST /scheduled/task/stop` | `stopScheduledTask` | 停止某次执行实例 | 1 | 缺失 |
| `POST /scheduled/graph/once/success` | `getScheduledOnceSuccess` | 判断图是否至少成功运行过一次 | 1 | 缺失 |
| `POST /scheduled/job/list` | `getScheduledJobs` | 某执行实例下的 job 列表 | 0 | 缺失 |
| `POST /scheduled/task/info` | `getScheduledTaskInfo` | 某执行实例详情（返回 ProjectJobVO） | 0 | 缺失 |

- 关键契约：`Cron { startTime, endTime, scheduleCycle, scheduleDate, scheduleTime }`、`TaskPageScheduledVO { scheduleTaskId, scheduleTaskExpectStartTime, scheduleTaskStartTime, scheduleTaskEndTime, scheduleTaskStatus, allReRun }`。
- 注：`getScheduledId / getScheduledInfo / getScheduledJobs / getScheduledTaskInfo` 当前在页面中暂无直接调用（0 文件），但属于 OpenAPI 契约的一部分，且为周期任务详情页的必备能力，一并补齐以保证模块完整。

### 4.5 机构模块（Inst，1 个）

支撑页面：`pages/institutions`（机构/节点注册）。

| 端点 | 前端方法 | 用途 | 使用文件数 | 备注 |
| --- | --- | --- | --- | --- |
| `POST /inst/node/register` | `registerInstNode` | 注册机构节点（multipart：cert/key/token 文件 + json_data 查询参数） | 0 | 缺失，multipart 上传 |

---

## 五、前端差距分析

对 `web/apps/secretpad/src/pages` 与 `sfwork/secretpad/frontend-src/apps/platform/src/pages + modules` 逐项比对：

| 全功能版能力 | 迁移版对应 | 状态 |
| --- | --- | --- |
| 登录 / 账户 / 修改密码 | `pages/login`、`pages/account`、`features/auth` | ✅ |
| 工作台 / 仪表盘 | `pages/workbench`、`pages/dashboard` | ✅ |
| 项目管理 / 项目详情 | `pages/projects` | ✅ |
| DAG 画布 / 引导模板 | `pages/dag`、`features/dag-templates`、`packages/dag-next` | ✅ |
| 运行记录 / 任务详情 | `pages/job-records`、`features/job-detail` | ✅ |
| 图管理 | `pages/graphs` | ✅ |
| 结果管理 | `pages/results` | ✅（依赖后端 4.1） |
| 数据源 / 数据表 / 数据上传 | `pages/data-sources`、`pages/data-tables`、`features/data-upload` | ✅（依赖后端 4.2/4.3） |
| 特征数据源 | `pages/feature-datasource` | ✅ |
| 节点 / 机构 / 节点路由 | `pages/nodes`、`pages/institutions`、`pages/node-routes` | ✅（依赖后端 4.5） |
| 消息中心 / 审批 | `pages/messages`、`features/approval` | ✅ |
| 模型管理 / 模型打包 / 血缘 | `pages/models`、`features/model-pack`、`features/lineage` | ✅ |
| 周期任务 | `pages/periodic-tasks`、`features/scheduled-task-from-dag` | ✅（依赖后端 4.4） |
| 隐私组件场景 / 组件版本 | `pages/privacy-scenes`、`pages/component-versions` | ✅ |
| 云日志 | `pages/cloud-logs` | ✅ |
| P2P（我的节点 / 项目） | `pages/p2p/*` | ✅ |
| 新手引导 | `pages/guide` | ✅ |

**结论：前端无缺失页面/功能，不需要新增前端代码。** 前端对缺失端点的调用已经写好，只要后端实现即可生效。前端侧的工作仅为：联调验证 + 必要的错误提示兜底（可选）。

---

## 六、补齐实施方案

### 6.1 总体策略

- **契约优先**：以 `web/openapi/secretpad.openapi.json` 与 `packages/api-client/src/schemas/index.ts` 中的 Zod schema 为唯一请求/响应契约，Go 实现的入参/出参字段名（camelCase）与之严格对齐，保证前端 `unwrapValidated` 运行时校验通过。
- **复用优先**：优先复用 Go 后端已有的 service / repository / kuscia client，不重复造轮子。
  - `datatable/create`、`datatable/get` 复用现有 `DatatableService`（`register`/`detail` 的等价逻辑），以别名路由 + 适配 DTO 实现；
  - `scheduled/*` 在现有 `ScheduledService`（cron 引擎 + `ProjectScheduleTaskDO`）上扩展；
  - `node/result/*`、`datasource/nodes` 复用 kuscia `QueryDomainData` / 数据源仓储；
  - `inst/node/register` 复用机构节点注册逻辑，接收 multipart。
- **非致命降级**：对依赖 Kuscia 实时查询的端点（result/list、result/detail、datasource/nodes），Kuscia 不可用时返回空集合而非报错，保证页面可打开。

### 6.2 后端改动点（按文件）

| 文件 | 改动 |
| --- | --- |
| `internal/controller/http/router.go` | 注册 16 个新路由（含 `/datatable/create`、`/datatable/get` 别名） |
| `internal/controller/http/v1/datatable_handler.go` | 新增 `Create`/`Get`/`PushToTee` handler |
| `internal/controller/http/v1/node_handler.go` | 新增 `ResultList`/`ResultDetail` handler |
| `internal/controller/http/v1/datasource_handler.go` | 新增 `Nodes` handler |
| `internal/controller/http/v1/scheduled_handler.go` | 新增 9 个周期任务 handler |
| `internal/controller/http/v1/misc_handler.go` | 新增 `RegisterInstNode`（multipart）handler |
| `internal/service/scheduled_service.go` | 扩展：结构化 Cron 创建、task 分页/重跑/停止/详情、job 列表、once-success |
| `internal/service/datatable_service.go` | 扩展：create/get/pushToTee（或复用 register/detail） |
| `internal/service/node_service.go`（或新增 result 逻辑） | 结果列表/详情（kuscia domaindata 查询 + 降级） |
| `internal/service/datasource_service.go` | 数据源关联节点查询 |
| `internal/dao/model/*` | 如缺少周期任务执行实例表字段，补充 DO 字段/迁移 |

### 6.3 前端改动点

- 无新增页面/组件。
- 联调验证 16 个端点对应页面的可用性；如个别端点返回结构与 Zod schema 有出入，优先改后端对齐契约。

### 6.4 实施顺序（自底向上，逐个可验证）

1. **数据源/数据表**（4.2 + 4.3，4 个）：逻辑最简单，先打通，验证「契约对齐」流程；
2. **结果管理**（4.1，2 个）：kuscia 查询 + 降级；
3. **周期任务**（4.4，9 个）：工作量最大，扩展现有 cron service；
4. **机构注册**（4.5，1 个）：multipart，最后补齐。

---

## 七、验证计划

1. **后端**：`go build ./...`、`go vet ./...`、`gofmt -l`、`go test ./...` 全部通过；
2. **契约自检**：对每个新端点，人工核对 Go 响应 JSON 字段与前端 Zod schema 字段一一对应（camelCase）；
3. **前端**：`pnpm typecheck` / `pnpm lint` / `pnpm build` 通过（前端无代码改动时应保持原样通过）；
4. **路由核对**：确认 `router.go` 中 16 个路由均已注册且指向正确 handler。

---

## 八、风险与假设

- **假设** 迁移版前端与后端部署在同一域（前端请求 `/api/v1alpha1/*` 直达 Go 后端），与现有路由前缀一致；
- **假设** `ProjectScheduleTaskDO` 及结果相关 DO 已存在或可通过 GORM AutoMigrate 平滑扩展；
- **风险**：周期任务的「执行实例（task）」模型若 Go 侧尚无独立表，需要新增表结构，工作量集中在 4.4；
- **风险**：`inst/node/register` 的证书/密钥文件处理涉及安全，需与现有机构注册流程保持一致。
