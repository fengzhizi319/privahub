# Privahub 开发指南

本文面向参与 Privahub 后端与前端开发的工程师，说明代码分层、扩展步骤、契约约定、测试方式与提交前检查。它回答“改哪里、怎么改、怎么验证”，不重复产品需求与接口清单；接口字段以 [api.md](api.md) 和生成的 OpenAPI 文档为准。

| 项目 | 说明 |
| --- | --- |
| 读者 | 后端、前端、测试、集成方 |
| 状态 | 现行 |
| 更新日期 | 2026-10-21 |
| 验证方式 | `make check`、`go test ./...`、`corepack pnpm typecheck`、文中的命令均可在当前仓库执行 |

## 1. 环境与仓库结构

### 1.1 工具链

| 工具 | 版本/要求 | 用途 |
| --- | --- | --- |
| Go | 1.25（以 `go.mod` 的 `toolchain` 为准） | 后端构建与测试 |
| CGO | 必须启用 | SQLite 驱动 `mattn/go-sqlite3` 需要 C 编译器 |
| Node.js | >= 18 | 前端构建 |
| pnpm | 11.7.0（`packageManager` 固定） | 工作区依赖与脚本 |
| corepack | 随 Node 提供 | 自动使用项目指定的 pnpm |
| Kuscia | 本地 Docker 或非 Docker 栈 | 真实任务、节点与网关联调 |
| golangci-lint | v2.12.x | `make lint` 与 CI |
| pre-commit | 可选 | 提交前快速检查 |

后端位于 `privahub/`，前端位于 `privahub/web/`。工作区根目录的脚本（如 `scripts/dev-start.sh`）负责同时拉起 Kuscia、后端与前端。

### 1.2 目录职责

```text
privahub/
├── cmd/
│   ├── server/            # 主服务入口：配置、迁移、监听器、优雅退出
│   ├── migrator/          # 数据库迁移工具
│   ├── edge-agent/        # EDGE 模式代理
│   ├── audit-verify/      # 审计链校验工具
│   ├── privahub-admin/    # 破窗管理员重置工具
│   └── openapi-gen/       # OpenAPI 文档生成器
├── internal/
│   ├── controller/http/
│   │   ├── router.go      # 主端口路由注册
│   │   ├── inner_router.go# 内部端口与 EDGE 转发路由
│   │   ├── v1/            # Handler：绑定请求、调用 Service、写响应
│   │   ├── middleware/    # 认证、RBAC、数据归属、限流、审计、camelCase
│   │   └── apidocs/       # 生成的 openapi.json 与只读 Handler
│   ├── service/           # 业务逻辑、Kuscia 编排、事务与领域校验
│   ├── dao/
│   │   ├── model/         # GORM 模型
│   │   ├── repository/    # 核心实体的仓储接口与实现
│   │   └── migrations/    # AutoMigrate、数据迁移、种子数据
│   ├── wire/              # 手动依赖注入容器 App 及各领域 wiring
│   ├── bootstrap/         # EDGE/AUTONOMY/TEE 启动初始化
│   ├── audit/             # 防篡改审计链
│   └── graphjob/          # DAG 到 Kuscia 任务的转换与组件配置
├── pkg/                   # 配置、Kuscia 客户端、认证、加密、响应、错误码等
├── config/                # privahub*.yaml、graph.yaml、组件与 i18n
├── deployments/           # Docker、Compose、Kubernetes 清单
├── scripts/               # 迁移验收、系统测试、安全与文档校验
└── docs/                  # 本文档目录
```

前端 `web/` 是 pnpm workspace：

| 路径 | 职责 |
| --- | --- |
| `web/apps/secretpad/` | 主应用（Vite + React Router/TanStack Router、Zustand、TanStack Query） |
| `web/packages/api-client/` | API 客户端、生成的 OpenAPI 类型、Zod 运行时校验 |
| `web/packages/design-system/` | 组件库 |
| `web/packages/dag-next/` | DAG 画布 |
| `web/packages/utils/` | 共享工具 |

### 1.3 本地启动

完整栈（Kuscia + 后端 + 前端）：

```bash
# 工作区根目录
bash scripts/dev-start.sh
bash scripts/dev-status.sh
```

只启动后端（Docker Kuscia 已就绪）：

```bash
cd privahub
make build
PRIVAHUB_PROFILE=dev ./bin/privahub -config ./config/privahub.yaml
```

只启动前端：

```bash
cd privahub/web
corepack enable
corepack pnpm install
corepack pnpm --filter @secretpad/app dev
```

关键地址：前端 `http://localhost:8000`，后端 `http://localhost:8080`，健康检查 `GET /api/v1alpha1/healthz`。`dev` Profile 使用 Docker 映射的 Kuscia 端口（gRPC 18083、HTTP 18082、网关 18080），开发账号为 `admin / 12345678`，仅限本地。配置细节见 [configuration.md](configuration.md)。

## 2. 启动流程与分层

### 2.1 `cmd/server` 启动顺序

`cmd/server/main.go` 的顺序体现了后端的初始化契约：

1. `config.Load` 读取配置并执行 fail-closed 校验；生产缺少密钥、TLS 或内部鉴权会直接退出。
2. 初始化 Zap 日志。
3. 构建静态加密 keyring（`crypto.Keyring`）并设为进程默认；未配置密钥时在 dev/test 打印警告。
4. `auth.ConfigurePasswordKDF` 设置密码 KDF 工作因子。
5. `dao.NewDB` 打开 SQLite/MySQL。
6. `migration.AutoMigrate` 建表并执行数据迁移。
7. `migration.EncryptSecretsAtRest` 加密存量明文并轮换到 active key。
8. `migration.SeedDataWithOptions` 初始化管理员、RBAC 与演示数据（仅 CENTER 的演示数据）。
9. `wire.NewApp` 构造 Service、Handler、Kuscia 客户端、后台任务与审计。
10. 构建主路由与内部路由，挂载 EDGE 转发目标。
11. `VerifyRouteCoverage` 对每条 `/api` 路由执行默认拒绝检查：未分类的路由会让进程启动失败。
12. 启动主端口与内部端口；收到 SIGINT/SIGTERM 后先停后台服务，再优雅关闭两个监听器。

新增启动期逻辑时，先判断它属于“配置校验”“迁移”“wiring”还是“后台服务”，放到对应阶段，不要在 `main` 里直接写业务。

### 2.2 分层规则

```text
Client
  │ HTTP/JSON
  ▼
middleware 链（CaseKeysResponse → TraceID → Recovery → Metrics → CORS → 安全头 → BlockInnerPaths → EDGE 转发 → 审计 → 全局限流 → BodyLimit → CaseKeysRequest）
  ▼
Handler（internal/controller/http/v1）
  │ 绑定 DTO、调用 Service、映射错误码
  ▼
Service（internal/service）
  │ 业务规则、事务、Kuscia 调用、审计事件
  ▼
DAO / GORM（internal/dao）      Kuscia Client（pkg/kuscia）      外部服务（OSS/ODPS/MySQL/SLS）
```

约定：

- **Handler 不写业务规则**：只做参数绑定、调用 Service、把错误翻译为错误码。复杂错误映射可复用 `failDataErr` 一类辅助函数。
- **Service 不依赖 Gin**：入参是 `context.Context` 与请求结构体，返回 VO/错误；需要用户身份时由 Handler 从会话中取出并放入请求或上下文。
- **DAO 不依赖 HTTP**：只处理持久化。核心实体提供 `repository` 接口；部分较新的 Service 直接持有 `*gorm.DB`，修改时跟随所在包既有风格，不要为统一而大规模重构。
- **依赖通过 `wire.App` 显式构造**：新增 Service 时在 `internal/wire` 中创建并挂到 `App`，Handler 只接收自己需要的 Service 指针。
- **跨层错误用 `error` 传递**：Service 返回 `service.BizError`（带 Java 错误码）或 `errcode.ErrorCode`，由 Handler 统一转为响应 envelope。

### 2.3 中间件顺序

主端口的关键顺序不可随意调整：

1. `CaseKeysResponse` 在最外层，保证所有 `/api` JSON 响应都带 camelCase 孪生键。
2. `BodyLimit` 在 `CaseKeysRequest` 之前，避免先读取未限流的请求体。
3. 认证、RBAC、数据归属、用户限流只挂在需要保护的路由组上。
4. `/metrics`、`/api/v1alpha1/healthz`、`/api/v1alpha1/env` 等公共端点不经过认证组。
5. 内部端口使用 `InnerGuardMiddleware`（校验 `kuscia-origin-source`）、`InnerAuthMiddleware`（token/CIDR）与同一套 RBAC/数据归属中间件。

## 3. 新增一个 API 的完整步骤

以下以新增 `POST /api/v1alpha1/example/do` 为例。每一步都对应仓库中的真实模式。

### 3.1 先确定契约

在动手前写清楚：

| 项目 | 示例 |
| --- | --- |
| 路径与方法 | `POST /api/v1alpha1/example/do` |
| 认证 | 登录用户（默认），还是公共/管理员/内部端口 |
| 请求字段 | `projectId`、`name`，哪些必填 |
| 响应数据 | `{"id":"...","createdAt":"..."}` |
| 错误码 | 参数错误、资源不存在、权限不足、系统错误 |
| 归属校验 | 请求体中的 `projectId` / `nodeId` / `ownerId` 由哪个中间件校验 |
| 部署模式 | CENTER / EDGE / AUTONOMY 是否都提供，是否需要加入 `sync.edge_include` |

前端以 camelCase 为契约；后端 DTO 可以使用 snake_case json tag，camelCase 兼容中间件会补齐孪生键。新代码优先让字段命名与前端一致，减少歧义。

### 3.2 在 Service 中定义请求、响应与方法

```go
// internal/service/example_service.go
type ExampleRequest struct {
    ProjectID string `json:"projectId" binding:"required"`
    Name      string `json:"name" binding:"required"`
}

type ExampleVO struct {
    ID        string `json:"id"`
    CreatedAt string `json:"createdAt"`
}

func (s *ExampleService) Do(ctx context.Context, req *ExampleRequest) (*ExampleVO, error) {
    if err := s.checkProject(ctx, req.ProjectID); err != nil {
        return nil, err
    }
    // ... 业务逻辑、事务、Kuscia 调用
    return &ExampleVO{ID: "example-1", CreatedAt: time.Now().UTC().Format(time.RFC3339)}, nil
}
```

要求：

- `binding:"required"` 会进入生成的 OpenAPI `required` 列表，也是 Handler 参数校验的第一道门。
- 需要 Java 兼容错误码时返回 `service.NewBizError(errcode.ProjectNotExists, msg)` 或直接返回 `*errcode.ErrorCode`；不要新造与 Java 冲突的数字段。
- 涉及多表写入时在 Service 内使用 `db.WithContext(ctx).Transaction(...)`，不要在 Handler 中开事务。
- 调用 Kuscia 时传入请求 `context` 并遵守现有超时；失败要区分“远端业务失败”和“连接失败”。

### 3.3 添加 Handler

```go
// internal/controller/http/v1/example_handler.go
type ExampleHandler struct {
    exampleService *service.ExampleService
}

func NewExampleHandler(exampleService *service.ExampleService) *ExampleHandler {
    return &ExampleHandler{exampleService: exampleService}
}

// Do handles POST /example/do.
// Request: {projectId, name}
// Response: {id, createdAt}
func (h *ExampleHandler) Do(c *gin.Context) {
    var req service.ExampleRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        response.FailWithMsg(c, errcode.ParamError, err.Error())
        return
    }
    req.ProjectID = verifiedID(c, "projectId", req.ProjectID)
    vo, err := h.exampleService.Do(c.Request.Context(), &req)
    if err != nil {
        failDataErr(c, err, errcode.SystemError)
        return
    }
    response.OK(c, vo)
}
```

`verifiedID` 用于数据归属中间件已校验过资源 ID 的场景：非 CENTER 调用方返回中间件验证后的 ID，避免“校验 A、使用 B”的参数混淆。没有归属校验的字段不要调用它。

### 3.4 在 `wire.App` 中装配

在 `internal/wire/wire.go` 中：

1. 给 `App` 增加 `ExampleService *service.ExampleService` 与 `ExampleHandler *v1.ExampleHandler` 字段。
2. 在 `NewApp` 中构造 Service（传入需要的 DB、Kuscia 客户端、日志等）。
3. 构造 Handler。
4. 如果有后台循环或需要优雅退出，把 Service 挂到 `Shutdown` 流程。

不要用全局单例保存请求级状态；进程级配置通过 `cfg` 传入。

### 3.5 注册路由

在 `internal/controller/http/router.go` 选择正确的路由组：

| 组 | 适用场景 | 中间件 |
| --- | --- | --- |
| `protected` | 普通登录用户 API | Auth + RBAC + DataResource + UserRateLimit |
| `aux` | 数据、节点用户、特征表、EDGE 数据同步 | 同上 |
| `p2p` | `/api/v1alpha1/p2p/*` | Auth + RBAC + DataResource |
| `authGroup` | `/api/login`、`/api/logout`、`/api/user/refresh` | 按端点单独配置 |
| 内部路由 | 节点间 vote/data/SSE、EDGE 转发目标 | InnerGuard + InnerAuth + RBAC + DataResource |

示例：

```go
func registerExampleRoutes(rg *gin.RouterGroup, h *v1.ExampleHandler) {
    rg.POST("/example/do", h.Do)
}
```

然后在 `NewRouter` 的 `protected` 组中调用 `registerExampleRoutes(protected, app.ExampleHandler)`。只有确实需要无认证的端点才放到公共组，并在 `PublicPaths` 中登记。

### 3.6 完成授权分类（否则服务启动失败）

`middleware.VerifyRouteCoverage` 对每条 `/api` 路由执行默认拒绝检查，必须同时满足：

1. **接口 RBAC 分类**（以下之一；审计角色路由另加 `AuditorPaths`）：
   - `middleware.APIResourceCodes[path]`：对应 Java `@ApiResource` 的 `ApiResourceCode`。
   - `middleware.AdminOnlyPaths[path] = true`：仅 ADMIN / CENTER_ADMIN。
   - `middleware.NoInterfaceCheckPaths[path] = "原因"`：Java 端确实没有接口级注解，必须写清原因。
   - `middleware.PublicPaths[path] = true`：公共端点。
   - 审计角色路由使用 `middleware.AuditorPaths`，与 RBAC 分类配合。
2. **数据归属分类**（二选一）：
   - `middleware.DataResourceRoutes[path] = projectRule() / nodeRule("...")`：请求体中必须携带对应 ID。
   - `middleware.DataResourceExempt[path] = "原因"`：不需要请求体资源校验，原因不能为空。

注意：

- 同一条路径不能同时出现在 `DataResourceRoutes` 与 `DataResourceExempt`。
- 不能重新引入 `auth.data_resource_whitelist`；配置校验会拒绝非空值。需要跨项目访问时授予明确的项目/节点成员关系。
- 新增管理员端点时同时更新 `AdminOnlyPaths` 和 `DataResourceExempt`（如果它不携带资源 ID）。
- 内部端口路由也必须分类；`MountEdgeForwardTargets` 只挂载主路由中已存在的 POST 处理器。

相关回归测试：

```bash
go test ./internal/controller/http/ -run 'TestRouteInterfaceResourceCoverage|TestVerifyRouteCoverage_RealRouters'
```

### 3.7 重新生成 OpenAPI 与更新文档

```bash
cd privahub
make openapi          # 重新生成 internal/controller/http/apidocs/openapi.json
make openapi-check    # 校验提交的文档与路由一致
CGO_ENABLED=1 go test ./internal/controller/http/ -run TestOpenAPIDocumentCoversRoutes
```

同步更新：

- [api.md](api.md)：模块接口清单、字段与错误码。
- [example.md](example.md)：如果新增了可串联的业务链路。
- 前端 `web/packages/api-client`：类型、客户端方法与 Zod schema；流程见 [openapi-sync.md](openapi-sync.md)。

### 3.8 测试

最少覆盖三层：

1. **Service 测试**：业务规则、事务、错误码、Kuscia 失败分支。
2. **Handler/路由测试**：认证、参数绑定、camelCase 兼容、归属校验、响应结构。
3. **契约测试**：新路由出现在 `apidocs/openapi.json`，且 RBAC/数据归属分类完整。

```bash
CGO_ENABLED=1 go test ./internal/service/ -run TestExample -v
CGO_ENABLED=1 go test ./internal/controller/http/ -run TestExample -v
CGO_ENABLED=1 go test ./...
```

## 4. 请求、响应与命名约定

### 4.1 统一响应 envelope

所有业务响应（包括业务错误）使用 HTTP 200 + 统一 JSON：

```json
{
  "status": { "code": 0, "msg": "success" },
  "data": { "id": "example-1" }
}
```

| 场景 | 写法 |
| --- | --- |
| 成功且有数据 | `response.OK(c, vo)` |
| 成功且无数据 | `response.OKEmpty(c)` |
| 预定义错误 | `response.Fail(c, errcode.ProjectNotExists)` |
| 带模板参数 | `response.FailArgs(c, ec, args...)` |
| 自定义消息 | `response.FailWithMsg(c, errcode.ParamError, err.Error())` |
| 指定 HTTP 状态 | `response.FailHTTP(c, http.StatusForbidden, ec)`；中间件用 `response.AbortHTTP` |

例外：受保护路由的认证失败会同时返回 HTTP 401，前端据此判断会话过期；其余业务错误保持 HTTP 200，通过 `status.code` 区分。错误消息按 `Accept-Language` 本地化（`zh` / `en`），默认中文。

### 4.2 camelCase 与 snake_case

历史 Go DTO 多为 snake_case，而迁移版前端统一使用 camelCase。后端通过两个中间件兼容：

- `CaseKeysResponse` 为每个 `/api` JSON 响应**追加** camelCase 孪生键，不替换原键。
- `CaseKeysRequest` 为 JSON 请求体**追加** snake_case / camelCase 孪生键，Handler 绑定任一命名都能收到值。

约定：

- 新 DTO 的 json tag 选择一种命名并保持包内一致；前端可见字段优先 camelCase。
- 不要依赖中间件修复结构差异（数组包装、标量 vs 对象、类型变化），这些必须在 DTO 层对齐。
- `CaseKeysRequest` 只处理 JSON；multipart、表单和查询参数需要显式兼容。
- `/api/v1alpha1/docs` 在中间件的排除列表中，OpenAPI 文档中的属性名保持 Go json tag 原样。

### 4.3 错误码

错误码定义在 `pkg/errcode`：

- `java_codes.go`：与 Java `secretpad-common` 对齐的数字码与 i18n key。
- `codes.go`：`define`、`alias`、`New` 与兼容别名。
- `auth_ext.go`：认证扩展码。

新增错误码时：

1. 优先复用已有 Java 码；确需新增时使用未被占用的数字段。
2. 提供中英文消息，或在 Java i18n 表中补充 key。
3. 在 Service 返回 `*errcode.ErrorCode` 或 `BizError`，Handler 不再拼接数字。
4. 更新 [api.md](api.md) 的错误码说明与前端处理（如需要）。

### 4.4 OpenAPI 生成对代码的约束

`cmd/openapi-gen` 通过静态分析推导契约：

- 请求体：Handler 中第一个 `ShouldBindJSON` / `ShouldBind` / `bind(c, &req)` 的 `&req` 类型。
- 响应数据：第一个 `response.OK(c, v)` 的 `v` 类型。
- 仅 POST/PUT 生成 `requestBody`；`FormFile` 等标记为 `multipart/form-data`。
- 不推导查询参数、路径参数、响应头；GET 端点只记录路径与处理器。

因此：

- 请求/响应 DTO 必须是可导出的命名类型或结构体，避免 `gin.H` 作为唯一响应。
- 一个 Handler 只保留一个主要的 `response.OK` 调用；分支响应尽量返回同一 VO 类型。
- 需要准确文档时，把字段定义放在 DTO 上，不要依赖内联匿名结构体。

## 5. 认证、授权与数据归属

### 5.1 认证

- 登录端点：`POST /api/login` 与 `POST /api/v1alpha1/user/login`；刷新：`POST /api/user/refresh`。
- 浏览器会话使用 HttpOnly Cookie + CSRF 头；API 客户端也可使用 `User-Token`。两种方式由认证中间件统一解析。
- 访问令牌短期有效（默认 2h），刷新令牌一次性使用并检测重放；会话还有滑动空闲超时。
- `auth.enabled=false` 仅允许 dev/test；生产配置校验会拒绝。
- 密码使用 SM3 KDF 存储；修改密码、管理员重置、破窗重置都会撤销会话。

### 5.2 RBAC

- 角色与资源关系存储在 `sys_role`、`sys_role_resource_rel`、`sys_user_permission_rel`、`sys_user_node_rel`。
- 中间件对每条请求读取权威数据库，不使用会延迟撤权的进程内缓存。
- `ADMIN` / `CENTER_ADMIN` 可访问管理员端点；`AUDITOR` 可访问审计端点（与 ADMIN 一起）。
- 新增端点必须选择 RBAC 分类，不能留空。

### 5.3 数据归属

`DataResourceGuard` 根据 `DataResourceRoutes` 从请求体读取资源 ID，调用 `service.DataResourceAuth` 校验当前用户是否有权访问该项目/节点/机构。校验通过后，Handler 用 `verifiedID(c, field, bound)` 取回已验证 ID。

规则：

- 请求体字段名必须与规则中的 `Field` 一致（中间件同时识别 camelCase 与 snake_case）。
- 一个请求携带多个资源 ID 时，用 `DataResourceRule.Extra` 声明；可选字段用 `optional(...)`。
- 无法在请求体中表达归属的端点，必须在 `DataResourceExempt` 写明理由，并在 Service 内自行校验。

### 5.4 内部端口

内部端口信任 `kuscia-origin-source` 头，因此：

- 默认绑定 `127.0.0.1`；非回环绑定必须配置 `server.inner_auth`（token / CIDR / mTLS）。
- 生产即使绑定回环也要求 `inner_auth`，防止同机进程伪造来源。
- 内部路由以节点虚拟用户执行 RBAC 与数据归属检查，不要绕过中间件直接调用 Handler。

## 6. 数据访问与迁移

### 6.1 模型

模型位于 `internal/dao/model`，按领域拆分文件（core、graph、job、datatable、datasource、user、audit 等）。新增表时：

1. 在对应文件定义 `XxxDO`，使用 GORM tag 指定列名、索引、唯一约束。
2. 在 `internal/dao/migrations/migrate.go` 的 `AllModels()` 注册模型。
3. 如需回填或转换旧数据，新增 `migrations/xxx_migrate.go` 并在 `AutoMigrate` 中调用；迁移必须幂等。
4. 涉及敏感列时，确认 `EncryptSecretsAtRest` 能识别并加密；新列加入对应的迁移/轮换逻辑。
5. 为迁移写测试（SQLite 临时库即可），覆盖“空库执行”和“重复执行”。

### 6.2 查询与事务

- 所有数据库调用带 `WithContext(ctx)`，使用请求上下文或后台任务上下文。
- 动态条件使用 GORM 参数绑定；禁止拼接用户输入。`make sqli-check` 会扫描高危模式。
- 多表写入使用事务；事务内不要调用可能长时间阻塞的外部服务。
- 列表接口返回统一分页结构（`infos`/`list` + `pageSize`/`pageNum`/`total`/`totalPage`，以对应模块现有 VO 为准）。
- 软删除字段和租户/归属字段必须与现有查询保持一致，避免跨租户读取。

### 6.3 种子数据

`SeedDataWithOptions` 根据平台类型初始化：

- CENTER：机构、内嵌 alice/bob 节点、演示项目/数据表/图、默认数据源。
- EDGE / AUTONOMY：本节点与管理员、RBAC，不写 CENTER 演示数据。
- 管理员密码：dev 使用 `admin/12345678`；生产未显式设置时生成一次性密码写入 `auth.initial_password_file`（0600），首次登录强制改密。

演示数据只用于开发与文档截图，不要在生产依赖其存在。

## 7. 测试

### 7.1 测试类型与位置

| 类型 | 位置 | 方式 |
| --- | --- | --- |
| Service 单元测试 | `internal/service/*_test.go` | 临时 SQLite + `httptest.Server` 模拟 Kuscia/OSS |
| Handler/路由测试 | `internal/controller/http/**/*_test.go` | `httptest` + 完整 `wire.App` |
| 中间件测试 | `internal/controller/http/middleware/*_test.go` | 构造 Gin 上下文与伪造请求 |
| 配置测试 | `pkg/config/*_test.go` | 直接调用 `Validate` / 子校验函数 |
| 迁移测试 | `internal/dao/migrations/*_test.go` | 临时库执行并断言结果 |
| 契约测试 | `router_test.go` | 路由覆盖、RBAC 分类、OpenAPI 一致性 |
| 前端测试 | `web/**/*.test.ts(x)` | Vitest + Testing Library + MSW |

### 7.2 后端测试脚手架

`internal/controller/http/router_test.go` 的 `newTestApp` / `newTestAppWith` 会：

1. 设置 `PRIVAHUB_PROFILE=test`。
2. 加载配置并改写为临时 SQLite 文件。
3. 执行 `migration.AutoMigrate`。
4. 构造 `wire.NewApp`，并在测试结束时关闭后台服务与数据库。

新增路由测试优先复用该脚手架；需要修改配置时使用 `newTestAppWith` 的 mutate 回调。Service 测试通常直接构造 Service 并注入临时 `*gorm.DB`，外部依赖用 `httptest.Server` 或接口 fake。

### 7.3 命令

```bash
cd privahub

# 全量（race + 覆盖率）
make test

# 指定包/用例
CGO_ENABLED=1 go test ./internal/service/ -run TestDatasource -v
CGO_ENABLED=1 go test ./internal/controller/http/... -count=1

# 静态检查与安全门禁
make check
make sec
make fuzz

# 迁移验收与隔离系统测试（需要 Kuscia 环境）
bash scripts/test_migration.sh --legacy-source /path/to/secretpad
```

前端：

```bash
cd privahub/web
corepack pnpm run typecheck
corepack pnpm run lint
corepack pnpm test
```

### 7.4 测试编写要求

- 表驱动用例使用 `t.Run`，断言用 `testify/require`（前置条件）与 `assert`（可继续执行的断言）。
- 不依赖测试执行顺序；每个用例自建数据库/临时目录并在 `t.Cleanup` 清理。
- 失败路径至少覆盖：参数错误、资源不存在、权限不足、外部依赖失败。
- 契约变更必须同时更新测试与 OpenAPI；不要只改文档。
- 不把“路由存在”或“页面能打开”当作业务验收；真实 Kuscia/SecretFlow 链路按 [migration-acceptance.md](migration-acceptance.md) 的矩阵验证。

## 8. 代码风格与提交

### 8.1 Go

- 提交前运行 `make fmt`（`go fmt` + `golangci-lint fmt`）。
- 导入分组使用 `local-prefixes: github.com/fengzhizi319/privahub`。
- 每个文件保留 Apache-2.0 许可头（与仓库现有文件一致）。
- 错误使用 `%w` 包装；返回给用户的错误走 `errcode`/`BizError`，不要把内部错误原文直接暴露给前端。
- 日志使用 Zap 结构化字段；不要记录密码、token、Cookie、Authorization、密钥或完整请求体。
- 注释只解释“为什么”，不重复代码表面行为；导出类型/函数按现有风格写简短注释。
- 避免无必要的抽象；三行重复代码优于为假设需求提前设计接口。

### 8.2 前端

- Prettier：`printWidth 88`、单引号、尾逗号。
- ESLint 配置在 `web/eslint.config.js`；提交时 lint-staged 自动修复。
- 服务端状态使用 TanStack Query；全局客户端状态使用 Zustand。
- 所有用户可见文案通过 i18n 字典；新增页面同时补中英文。
- API 响应必须经过 Zod 校验（`unwrapValidated`），契约不符时抛出带字段路径的错误。
- 提交信息遵循 Conventional Commits（Husky + commitlint）。

### 8.3 提交前检查清单

```text
[ ] make fmt
[ ] make check            # gate/fmt/vet/lint/sqli/redact/image/k8s/openapi/test/vuln/secrets
[ ] 新增或修改路由后已 make openapi
[ ] 新增路由已在 APIResourceCodes / AdminOnlyPaths / NoInterfaceCheckPaths / PublicPaths 中分类
[ ] 新增路由已在 DataResourceRoutes / DataResourceExempt 中分类
[ ] 后端测试覆盖成功、参数错误、权限与外部失败分支
[ ] 前端 typecheck / lint / test 通过（如涉及前端）
[ ] api.md / example.md / openapi-sync 相关文档已同步
[ ] 没有把密钥、token、真实凭据写入代码、配置样例或文档
```

## 9. 常见扩展场景

### 9.1 新增配置项

1. 在 `pkg/config` 对应结构体添加字段与 `mapstructure` tag。
2. 在 `setDefaults` 设置默认值；需要环境变量覆盖的标量字段会被 `bindEnvKeys` 自动绑定。
3. 在 `Validate` / `validateSecurity` / `validateAudit` 中加入约束；安全相关项默认拒绝不安全值。
4. 非致命问题加入 `SecurityWarnings` 或对应 `Warnings`。
5. 补充 `pkg/config/*_test.go`。
6. 更新 [configuration.md](configuration.md)，必要时更新 [ops.md](ops.md) 与加固指南。

Map、结构体切片（如 `crypto.keys`、`kuscia.nodes`）不能用一个环境变量表达，需要像现有实现一样提供专用环境变量或文件机制。

### 9.2 新增后台任务

- 在 Service 中实现 `Start(ctx)` / `Stop()` 或等价的循环，使用 `context` 退出。
- 在 `wire.NewApp` 中启动，在 `App.Shutdown` 中停止；确保关闭顺序在 HTTP 服务器退出前。
- 任务日志带稳定字段（任务类型、对象 ID），不要输出敏感数据。
- 增加可观测性：失败计数、最后成功时间或 Prometheus 指标。

### 9.3 新增命令行工具

- 在 `cmd/<tool>/main.go` 提供 `-config` 或 `-driver/-dsn`，复用 `pkg/config` 与 `dao.NewDB`。
- 退出码约定：0 成功、1 运行错误、2 用法错误。
- 直接改数据库的工具必须使用事务、写审计事件（如适用），并在文档中记录操作步骤。
- 参考 `cmd/privahub-admin` 与 [破窗重置手册](security/runbooks/break-glass-admin-reset.md)。

### 9.4 排查联调问题

| 现象 | 优先检查 |
| --- | --- |
| 启动即退出 | 配置校验错误、JWT/AES/TLS/inner_auth 是否满足生产要求 |
| 401 / 会话立即失效 | Cookie Secure/CSRF、`jwt_secret` 是否轮换、密码版本是否变化 |
| 403 / 资源不存在 | RBAC 分类、`DataResourceRoutes`、用户与项目/节点成员关系 |
| 路由 404 | 是否注册在正确路由组、EDGE `sync.edge_include` 是否包含该路径 |
| OpenAPI 漂移 | 运行 `make openapi` 并提交生成文件 |
| Kuscia 调用失败 | `kuscia.*` 地址/端口/协议、证书、网关、节点 token |
| 上传失败 | `server.upload_max_size`、`data.dir` 权限与磁盘空间 |

日志与栈状态：

```bash
bash scripts/dev-status.sh
bash scripts/dev-logs.sh backend -f
```

## 10. 相关文档

- [design.md](design.md)：分层与模块设计。
- [api.md](api.md)：接口清单与字段。
- [configuration.md](configuration.md)：全量配置项与默认值。
- [openapi-sync.md](openapi-sync.md)：OpenAPI 生成、漂移门禁与前端类型同步。
- [testing.md](testing.md)：测试组织与用例说明。
- [security/authorization-review.md](security/authorization-review.md)：认证与权限复核细节。
- [ops.md](ops.md)：构建、部署与排障。
