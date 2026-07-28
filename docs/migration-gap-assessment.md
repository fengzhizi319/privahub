# 迁移版功能差距评估与补齐方案

> 评估对象
> - **迁移版**：前端 `clwork/privahub/web`（privaconsole，vite + React + Tailwind + FSD），后端 `clwork/privahub`（Go + Gin + GORM）
> - **全功能版**：前端 `sfwork/secretpad/frontend-src`（umi + AntD5），后端 `sfwork/secretpad`（Java + Spring Boot）
>
> 评估日期：2026-07-28（首版）；2026-07-28 二次深度评估（修订版）；2026-07-28 三次落地复核（现状版）

---

## 一、背景与目标

`clwork/privahub` 是从 `sfwork/secretpad` 迁移而来的 Go 重构版后端，`clwork/privahub/web` 是配套的全新前端（Feature-Sliced 架构）。迁移版前端在重写时已按照全功能版的业务能力定义了完整的 API 契约（`web/openapi/secretpad.openapi.json` + `packages/api-client`，**统一 camelCase**），但 Go 后端尚未实现其中全部端点，且**已实现端点的请求/响应契约与前端存在系统性不一致**，导致部分页面功能在迁移版中不可用。

本文目标：

1. 系统评估迁移版（前端 + 后端）与全功能版的差距；
2. 给出可执行的补齐方案；
3. 作为后续补齐实现的依据。

---

## 二、评估结论（TL;DR）

### 2.1 首版结论（端点覆盖维度，已基本完成）

| 维度 | 结论 |
| --- | --- |
| **前端页面覆盖** | ✅ 基本完整。`web` 已覆盖全部 24 个页面 + 8 个 feature，与全功能版业务能力一一对应，无缺失页面。 |
| **前端 API 契约** | ✅ 完整。`packages/api-client` 已定义约 90 个端点的调用与 Zod 运行时校验。 |
| **后端 API 实现** | ✅ 16 个缺失端点已补齐（commit `882c3ff`），端点路径覆盖率 100%。 |

### 2.2 修订版结论（契约一致性维度，二次评估新发现）⭐

> **端点「存在」≠ 功能「可用」。二次评估以「前端实际请求/响应契约」为准逐端点核对，发现真正的深层差距是系统性的契约不匹配。**

| 维度 | 结论 | 严重度 |
| --- | --- | --- |
| **请求体字段命名不匹配** | ⚠️ Go 大量 handler 用 `snake_case` + `binding:"required"`（如 `project_id`），而前端统一发 `camelCase`（如 `projectId`）。`encoding/json` 无法匹配二者，导致 **project/model/message/noderoute/p2p/inst 等模块大面积返回 ParamError**。 | 🔴 P0 |
| **响应体字段命名不匹配** | ⚠️ Go 现有 DTO 几乎全部 `snake_case`，而前端 Zod schema / 类型统一 `camelCase`。严格校验端点（22 个 `unwrapValidated`）中必填字段缺失会**抛错**（如 `project/get` 的 `projectId`），其余端点前端取到 **全 undefined（页面空数据）**。 | 🔴 P0 |
| **路径不匹配** | ⚠️ 前端调 `/model/serving/create\|delete\|detail`，Go 注册为 `/serving/*`（缺 `model/` 前缀），模型服务功能不可用。 | 🟠 P1 |
| **graph 模块为何正常** | ✅ `graph_service.go` 采用「双字段兼容」DTO（`project_id` + `projectId`），请求侧可接收 camelCase；是唯一遵循该约定的模块。 | — |

**修订版核心差距 = 系统性 snake_case ↔ camelCase 契约不匹配（请求 + 响应），而非端点缺失。**

最优解：在 Gin 层引入**全局 camelCase 兼容中间件**（请求体双向补键 + 响应体 snake→camel 转换），一次性、无侵入地打通全部现有端点；再补齐 `model/serving/*` 路径别名。

### 2.3 三次评估结论（落地现状，2026-07-28）✅

> 本轮以「代码实际实现」为准逐端点复核，确认前两轮发现的差距**均已落地修复**，迁移版后端与全功能版的差距已从「端点缺失 / 契约不通」收敛为「Kuscia 运行时深度」。

| 维度 | 现状 |
| --- | --- |
| **契约不匹配（修订版 P0）** | ✅ 已修复。全局 camelCase 兼容中间件（`middleware/camelcase.go`）已提交（commit `58483e4`），请求双向补键 + 响应 snake→camel，含单测。 |
| **路径不匹配（`model/serving/*`）** | ✅ 已修复。别名路由已注册。 |
| **16 个缺失端点（首版）** | ✅ 全部注册且实现（见第四节复核表）：DB 直连或 Kuscia＋优雅降级。 |
| **project 模块骨架端点** | ✅ 全部落地为真实 DB 实现：datatable add/delete/tableConfig、tee/list、datasource/list、getOutTable（含服务层单测）。 |
| **死代码清理** | ✅ 移除 `misc_handler.go` 中未路由的重复 `NodeResultList/NodeResultDetail`（真实路由指向 `NodeHandler`）。 |

**当前唯一剩余差距 = Kuscia 运行时深度**：结果列表/详情、pushToTee、图执行产生真实输出等能力依赖活 Kuscia 集群；无 Kuscia 时按设计优雅降级（返回空集合 / 最佳努力记录），页面可打开但无实时计算数据。这属于**部署/运行时能力差距**，非代码缺失，需联调环境验证而非离线补齐。

### 2.4 结构性契约深度审计（2026-07-28）✅

> 中间件只修复「键命名」（snake↔camel），不修复「结构」。为进一步排除中间件覆盖不到的结构性报错风险，对前端全部 **21 个严格校验端点**（`unwrapValidated` → Zod `safeParse`）逐个核对其 schema 的**必填字段**。

**结论：21 个严格校验端点中，仅 `project/get`（`ProjectVOSchema.projectId`）含必填字段**，且已由中间件（响应补 camelCase `projectId`）修复；其余 20 个 schema（`DatatableNodeVO`/`ProjectJobVO`/`ModelPackDetailVO`/`AllNodeResultsListVO`/`NodeResultDetailVO`/`InstTokenVO`/`UserContextDTO`/`DatasourceNodesVO`/`DatasourceDetailAggregateVO`/`PullStatusVO`/`MessageDetailVO`/`SyncDataDTO`/`ProjectParticipantsDetailVO`/`ModelExportPackageResponse` 等）**字段全为 `.optional()`**，`safeParse` 永不因缺字段抛错。

| 核查项 | 结果 |
| --- | --- |
| 严格校验端点数 | 21（`unwrapValidated`）；其余 74 个为宽松 `unwrap`（不报错） |
| 含必填字段的严格端点 | 仅 `project/get`（1 个），已由中间件修复 |
| 剩余结构性抛错风险 | ✅ 无 |
| 图执行 DB 链路 | ✅ `StartGraph` 在 DB 层创建 `ProjectJobDO`（RUNNING）+ 每节点 `ProjectJobTaskDO`，作业记录页有数据；Kuscia 提交失败时降级 |
| `model/serving/*` 别名 | ✅ `create/list/delete/detail` 均已注册指向 `ModelHandler` |

**综上：离线可验证的迁移工作已全部完成**（端点覆盖 100%、契约命名由中间件打通、无结构性抛错、骨架端点均为真实 DB 实现）。后续重点应转向搭建 Kuscia 联调环境，验证实时计算链路（结果产出、pushToTee、图执行真实输出）。

### 2.5 列表响应包装结构修复（2026-07-28）✅

> 进一步审计发现第三类差距：**列表响应包装键不匹配**。前端 `client.ts` 的列表方法多数带 `Array.isArray(payload) ? ... : (payload?.X || payload?.list || [])` 兑底，但有 **6 个强制要求特定包装键**（无裸数组兑底）。逐一核对后发现其中 **4 个 Go handler 返回形状不匹配，导致对应页面空数据**（不报错但无内容，隐蔽性高）。

| 端点 | 前端读取 | 修复前 Go 返回 | 修复后 | 影响页面 |
| --- | --- | --- | --- | --- |
| `node/page` | `payload.list` + `payload.total` | `{nodes}` | `{list,nodes,total}` | 节点管理 |
| `model/page` | `payload.modelPacks` 或 `payload.list` | 裸数组 | `{modelPacks,list}` | 模型管理 |
| `scheduled/page` | `payload.list` | 裸数组 | `{list}` | 周期任务 |
| `message/list` | `payload.messages` 或 `payload.list` | `{data,total,...}` | `{messages,list,total,...}` | 消息中心 |

另两个强制包装端点 `scheduled/task/page`、`scheduled/job/list` 原本即返回 `{list}`，无需修复。此类差距中间件无法覆盖（只改键名不改包装结构），需逐个 handler 对齐。修复后上述四个页面可正常渲染列表。

### 2.6 标量/单值响应结构修复（2026-07-28）✅

同类审计发现返回「单一标量」的端点也有结构不匹配（前端 `unwrap` 在 `data` 为 null/undefined 时会直接报错 `API returned empty data`）：

- `message/pending`：前端 `Number(unwrap(data))` 期望 `data` 为裸数字，Go 原返回 `{pending_count}` 对象 → `Number(对象)` 为 NaN → 待处理消息计数恒为 0。修复：直接返回裸数字 `count`。
- `graph/node/max_index`：前端读 `payload.maxIndex`，Go 原返回 `OKEmpty`（无 `data` 字段）→ `unwrap` 报错，DAG 画布添加节点流程中断。修复：`RefreshNodeMaxIndex` 改为返回 `(int, error)`，handler 返回 `{max_index}`（中间件补 camelCase `maxIndex`）。

### 2.7 严格数组校验端点修复（2026-07-28）✅

前端部分方法用 `validated(z.array(...), unwrap(data))` 严格校验响应为**裸数组**（无包装兜底，且 `unwrap` 在 `data` 为空时报错）。逐一核对后发现：

- `p2p/project/archive`：前端 `archiveP2pProject` 期望返回刷新后的 `ProjectVO[]`，Go 原返回 `OKEmpty` → `unwrap` 报错，P2P 项目归档操作失败。修复：抽取 `buildP2pProjectList` 辅助方法，归档后重查并返回裸项目数组（`ProjectList` 同步复用）。
- `model/modelPartyPath`：前端 `getModelPartyPath` 传 `{projectId, graphNodeId, graphNodeOutPutId}` 并期望参与方裸数组 `[{nodeId, nodeName, dataSources}]`；Go 原误读 `{modelId}` 且返回对象 `{model_id, parties}` → 请求参数不匹配 + `z.array` 校验失败，模型打包参与方路径加载失败。修复：handler 改收正确请求体，`ModelService` 新增 `GetModelPartyPath`（按项目节点 + 各节点数据源构建），返回裸数组。数据源项同时输出 `dataSourceId`/`datasourceId` 以兼容前端读取。
- `p2p/project/list`：复查确认 Go 已返回裸数组，与 `z.array(ProjectVOSchema)` 匹配，无需修复。

### 2.8 图节点字段类型修复（DAG 画布，2026-07-28）✅

`graph/detail` 返回的 `GraphNodeVO` 将 `inputs`/`outputs`/`node_def` 以 **JSON 字符串**输出（数据库 text 列原样返回），但前端 `GraphNodeDetail` 契约要求 `inputs`/`outputs` 为 `string[]`、`nodeDef` 为对象。中间件只改键名不解析 JSON，导致 DAG 画布读取 `node.outputs?.[0]` 取到字符串首字符 `[`、`node.outputs?.length` 取到字符串长度，边连接与输出处理错误。

修复：`GraphNodeVO.Inputs/Outputs` 改为 `[]string`（复用 job 服务已有的 `splitOutputs` 解析 JSON 数组），`NodeDef` 改为 `json.RawMessage`（新增 `rawJSONMessage` 辅助函数，空/非法转 null）。同包的 `JobGraphNodeVO` 早已正确处理（`Outputs []string` + `splitOutputs`），本次使 graph 路径与其对齐。`go build`/`vet`/`test` 全过。

---

## 三、评估方法

### 3.1 首版（端点覆盖维度）

1. 提取全功能版后端全部 Controller 的 `@*Mapping` 端点清单（26 个 Controller）；
2. 提取迁移版后端 `internal/controller/http/router.go` 已注册路由清单；
3. 提取迁移版前端 `packages/api-client/src/client.ts` 实际调用的端点清单（约 90 个）；
4. 三方交叉比对：以**前端实际调用**为准，筛出「前端要调、后端没有」的端点；
5. 对每个缺失端点，核对前端 Zod schema / OpenAPI 契约，确认请求/响应结构，并确认其在页面中的真实使用情况。

### 3.2 修订版（契约一致性维度）⭐

首版只回答了「端点存不存在」，修订版进一步回答「端点能不能用」：

1. **路径精确 diff**：将前端 `client.ts` 的 123 个调用路径（含 `/api/v1alpha1` 前缀）逐一映射为 Go 注册后缀，与 `router.go` 的 166 条路由做集合差，找出路径不匹配项；
2. **请求契约核对**：逐个核对前端请求 body 字段（camelCase）与 Go handler `ShouldBindJSON` 的 struct tag，重点关注 `binding:"required"` 的 snake_case 字段（前端不发 → ParamError）；
3. **响应契约核对**：逐个核对前端 22 个 `unwrapValidated`（Zod `safeParse` 严格校验）端点的响应 DTO 字段命名，区分「必填字段缺失→抛错」与「全 optional→空数据」两类影响；
4. **机制验证**：确认 Go `encoding/json` 对 `projectId` vs `project_id` **不匹配**（非纯大小写差异），Zod `z.object` 默认非 strict（未知字段忽略、可选字段可缺、**必填字段缺失报错**）。

---

## 三’、契约不匹配明细（修订版核心发现）⭐

### A. 路径不匹配（1 组，3 端点）

| 前端调用路径 | Go 注册路径 | 影响 |
| --- | --- | --- |
| `POST /api/v1alpha1/model/serving/create` | `POST /api/v1alpha1/serving/create` | 模型服务创建不可用（404） |
| `POST /api/v1alpha1/model/serving/delete` | `POST /api/v1alpha1/serving/delete` | 模型服务删除不可用（404） |
| `POST /api/v1alpha1/model/serving/detail` | `POST /api/v1alpha1/serving/detail` | 模型服务详情不可用（404） |

> 注：`/api/login`、`/api/logout` 仅为前端登录的**降级备用路径**，主路径 `/api/v1alpha1/user/login|logout` Go 已实现，故不构成差距。

### B. 请求体 snake_case required 绑定（前端 camelCase → ParamError）

`grep 'json:"[a-z]+_.." binding:"required"'` 统计，分布于 11 个 handler 文件：

| 字段 | 出现次数 | 涉及模块 |
| --- | --- | --- |
| `node_id` | 16 | node / project / misc / p2p |
| `project_id` | 12 | project / job / model / scheduled |
| `vote_id` | 4 | message / vote |
| `schedule_task_id` | 4 | scheduled |
| `router_id` | 4 | noderoute |
| `inst_id` | 4 | project / misc |
| `datatable_id` / `model_id` / `datasource_id` / `src_node_id` / `dst_node_id` / `table_name` / `result_id` / `refresh_token` | 各 1–2 | 各模块 |

典型例：`project/get` handler 绑定 `json:"project_id" binding:"required"`，前端发 `{projectId}` → Go 收到空 `project_id` → **ParamError**，项目详情页完全不可用。同类：`project/delete`、`project/node/add`、`project/inst/add`、`project/tee/list`、`project/getOutTable`、`project/datasource/list`、`project/update/tableConfig` 等。

### C. 响应体 snake_case（前端 camelCase 取空 / 严格校验报错）

前端 22 个 `unwrapValidated` 严格校验端点中，Go 响应为 snake_case 的：

| 端点 | Zod schema | 影响类型 |
| --- | --- | --- |
| `project/get` | `ProjectVOSchema`（`projectId` **必填**） | 🔴 报错（页面崩溃） |
| `project/datatable/get` | `DatatableNodeVOSchema`（全 optional） | 🟡 空数据 |
| `project/job/get` | `ProjectJobVOSchema`（全 optional） | 🟡 空数据 |
| `message/detail` | `MessageDetailVOSchema` | 🟡 空数据 |
| `model/pack` | `ModelExportPackageResponseSchema` | 🟡 空数据 |
| `model/detail` | `ModelPackDetailVOSchema` | 🟡 空数据 |
| `data/sync` | `SyncDataDTOSchema` | 🟡 空数据 |
| `inst/node/add\|token\|newToken` | `InstTokenVOSchema` | 🟡 空数据 |
| `p2p/project/participants` | `ProjectParticipantsDetailVOSchema` | 🟡 空数据 |
| `user/get` | `UserContextDTOSchema` | 🟡 空数据 |
| `datasource/detail` | `DatasourceDetailAggregateVOSchema` | 🟡 空数据 |
| `approval/pull/status` | `PullStatusVOSchema` | 🟡 空数据 |

> 上一轮已补齐的 5 个端点（`node/result/list|detail`、`datatable/get`、`datasource/nodes`、`scheduled/info|task/info`）已用 camelCase 「Compat」DTO，不受此问题影响。

此外，非严格校验但前端直接读 camelCase 字段的端点（如 `graph/detail` 返回 `graph_id/project_id/nodes[].graph_node_id`，前端读 `graphId/projectId/graphNodeId`）同样取空，影响 DAG 画布渲染。

### D. 骨架实现（逻辑深度差距）

部分 handler 仅绑定请求后直接返回空值，无实际业务逻辑。本轮已将其中**数据表关联三端点**落地为真实的 DB 实现（基于 `project_datatable` 表 / `ProjectDatatableDO`，幂等 upsert，含服务层单测）：

- ✅ `project/datatable/add`：写入项目-节点-数据表关联，列配置（`configs`）持久化到 `table_configs`
- ✅ `project/datatable/delete`：按 (project,node,datatable) 删除关联，幂等
- ✅ `project/update/tableConfig`：更新列配置，缺失时自动 upsert
- ✅ `project/tee/list`：返回全局 TEE 能力节点（`NodeDO.Mode` 为 1/2），修正原 `project_id required` 导致前端空体请求必错的问题
- ✅ `project/datasource/list`：按项目节点聚合数据源（`datasource_node` 关联），返回 `[{nodeId,nodeName,dataSources[]}]`
- ✅ `project/getOutTable`：聚合项目元信息 + 图/作业计数 + 各图节点声明输出表（解析 `project_graph_node.outputs` JSON，与节点输出 fallback 同源），支持 `graphId` 过滤，返回 `ProjectOutputVO`

至此，`project/*` 模块的所有骨架端点均已落地为真实 DB 实现（含服务层单测）。项目模块剩余的**逻辑深度差距**集中在依赖 Kuscia 运行时的图执行/作业链路（见下节），属于与全功能版的运行时能力差距，优先级低于契约不匹配（契约不通时页面直接不可用）。

---

## 四、后端差距明细（16 个端点：已全部注册并实现）✅

> **三次复核更新**：以下 16 个端点首版评估时为「前端已调用 + OpenAPI 已定义 + Go `router.go` 未注册」。现状：**全部已在 `router.go` 注册并有真实实现**（DB 直连或 Kuscia＋优雅降级），不再是「缺失端点」。下表保留原始分类以供追溯，并标注当前实现方式。

| 模块 | 端点数 | 当前实现方式 | 状态 |
| --- | --- | --- | --- |
| 4.1 结果管理 | 2 | `NodeService.ListNodeResults/GetNodeResultDetail`：Kuscia DomainData 查询＋不可用时跳过/降级 | ✅ |
| 4.2 数据源 | 1 | `DatasourceService.GetDatasourceNodes`：`datasource_node` 关联 DB 查询 | ✅ |
| 4.3 数据表 | 3 | `DatatableService.CreateDatatableCompat/GetDatatableCompat`（DB）＋`PushDatatableToTee`（DB 记录＋Kuscia 最佳努力授权） | ✅ |
| 4.4 周期任务 | 9 | `ScheduledService`：cron 引擎＋`ProjectScheduleTaskDO` DB 实现，图执行依赖 Kuscia 时降级 | ✅ |
| 4.5 机构注册 | 1 | `MiscHandler.RegisterInstNode`：multipart 上传＋机构节点注册 | ✅ |

以下保留首版的原始分组明细（仅供追溯）：

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

### 6.0 修订版方案（契约一致性，本轮重点）⭐

针对「三’」的系统性契约不匹配，采用**全局中间件**一次性无侵入修复，避免逐个 handler 改造（11 个文件、数十个 DTO）的高风险与高成本：

**方案：`internal/controller/http/middleware/camelcase.go`**

1. **请求体双向补键**（`CamelSnakeRequest`）：仅对 `Content-Type: application/json` 的请求，递归遍历 body JSON，为每个 key 补充其「另一种命名」孪生键（camelCase↔snake_case），写回 `c.Request.Body`。
   - 前端发 `{projectId}` → 补 `{project_id}`，snake_case 绑定可接收；
   - 前端发 `{scheduleId}` → 补 `{schedule_id}`，同时原 camelCase 键保留，camelCase 绑定（上一轮 16 端点）仍可接收；
   - **加法式**，不删除原键，故不破坏任何现有端点。
2. **响应体 snake→camel 转换**（`CamelSnakeResponse`）：包装 `gin.ResponseWriter`，在 `Write` 时拦截 JSON，递归将所有 snake_case key 转为 camelCase（保留原键以防其他消费方）。跳过 SSE（`/sync`）、非 JSON 响应。
   - Go 返回 `{project_id}` → 补 `{projectId}`，前端 `ProjectVOSchema.projectId`（必填）满足，严格校验通过；
   - 已是 camelCase 的键（无下划线）不变，上一轮 16 端点不受影响。
3. **跳过规则**：multipart（`inst/node/register`）、SSE（`/sync`）、静态资源、`/metrics`、`/healthz` 不走转换。

**路径别名**：`router.go` 为 `model/serving/create|delete|detail` 增加带 `model/` 前缀的别名路由，指向现有 `ModelHandler.CreateServing|DeleteServing|ServingDetail`。

**优势**：一次实现、全局生效、加法式无破坏、后续新端点自动兼容。风险点（嵌套/`json.RawMessage` 递归转换、大 body 性能）在实现中针对性处理并加单测。

### 6.1 首版总体策略（端点补齐，已完成）

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
2. **中间件单测**（修订版新增）：
   - 请求补键：`{projectId}` → 同时含 `project_id`；嵌套对象/数组递归补键；已是 camelCase 的键不重复；
   - 响应转换：`{project_id}` → 同时含 `projectId`；嵌套/`json.RawMessage` 递归；非 JSON / SSE 跳过；
   - 加法式不破坏：原键始终保留。
3. **契约自检**：对每个新端点，人工核对 Go 响应 JSON 字段与前端 Zod schema 字段一一对应（camelCase）；
4. **前端**：`pnpm typecheck` / `pnpm lint` / `pnpm build` / `pnpm test` 通过（前端无代码改动时应保持原样通过）；
5. **路由核对**：确认 `router.go` 中 16 个路由 + `model/serving/*` 3 个别名均已注册且指向正确 handler。

---

## 八、风险与假设

- **假设** 迁移版前端与后端部署在同一域（前端请求 `/api/v1alpha1/*` 直达 Go 后端），与现有路由前缀一致；
- **假设** `ProjectScheduleTaskDO` 及结果相关 DO 已存在或可通过 GORM AutoMigrate 平滑扩展；
- **风险**：周期任务的「执行实例（task）」模型若 Go 侧尚无独立表，需要新增表结构，工作量集中在 4.4；
- **风险**：`inst/node/register` 的证书/密钥文件处理涉及安全，需与现有机构注册流程保持一致。
