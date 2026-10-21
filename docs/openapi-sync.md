# OpenAPI 契约同步说明

**类型**：契约同步指南与模板 ｜ **读者**：后端、前端、测试、安全评审 ｜ **状态**：现行 ｜ **核对日期**：2026-10-21
**验证方式**：`make openapi-check`、`TestOpenAPIDocumentCoversRoutes`、前端 `pnpm typecheck`

本文说明 Privahub 的 OpenAPI 契约从哪里来、如何生成、如何校验，以及新增或修改接口时后端与前端各需要同步什么。文末提供可直接复制的 PR 模板与变更清单。

## 1. 一句话结论

- **后端**：路由注册和 Handler 的 Go 类型是唯一事实来源。`cmd/openapi-gen` 构建真实 Gin 路由并解析 Handler 源码，生成并提交 `internal/controller/http/apidocs/openapi.json`，再由 `//go:embed` 暴露为 `GET /api/v1alpha1/docs`。
- **前端**：当前使用的 `web/openapi/secretpad.openapi.json` 是 **Java springdoc 导出的历史基线**，不是 Go 生成器产物。它经 `openapi-typescript` 生成 `secretpad.d.ts`，供 `api.ts` / `client.ts` 使用。
- **两条链目前没有自动桥接**：后端文档已纳入 CI 漂移门禁；前端契约仍靠手工维护。把 Go 文档切换为前端源之前，必须先处理路径、operationId、字段命名和手写类型断言差异。
- **不要手改** 任何 `openapi.json` 或 `generated/*.d.ts`；前者由生成器覆盖并被测试比对，后者由 codegen 覆盖。

## 2. 先分清两份 OpenAPI 文档

| 项目 | 后端生成文档 | 前端历史基线 |
| --- | --- | --- |
| 文件 | [`internal/controller/http/apidocs/openapi.json`](../internal/controller/http/apidocs/openapi.json) | [`web/openapi/secretpad.openapi.json`](../web/openapi/secretpad.openapi.json) |
| 生成方式 | `cmd/openapi-gen` 从真实路由与 Go AST 生成 | Java springdoc 导出后提交（`openapi: 3.0.1`） |
| 标题与版本 | `Privahub API` / `v1alpha1` | `Secretpad Restful API` / `0.0.1-SNAPSHOT` |
| 当前规模 | 157 条路径、157 个 operation、214 个 schema | 121 条路径、121 个 operation |
| 服务地址 | `servers: /`（相对当前主机） | `servers: http://127.0.0.1:8080`（仅元数据） |
| 用途 | 运行时 API 文档、路由覆盖测试、安全评审 | 前端 TypeScript 类型与请求客户端 codegen |
| 是否自动同步 | 是，`make openapi-check` 与 CI 强制 | 否，当前无 Go → 前端自动同步 |

两条链的关系：

```text
后端（自动，受门禁约束）
internal/controller/http/router.go + inner_router.go
        │  cmd/openapi-gen（真实路由 + Go AST）
        ▼
internal/controller/http/apidocs/openapi.json（提交）
        │  //go:embed
        ▼
GET /api/v1alpha1/docs

前端（手工基线，尚未桥接）
web/openapi/secretpad.openapi.json（Java springdoc 导出）
        │  pnpm codegen:openapi（openapi-typescript）
        ▼
web/packages/api-client/src/generated/secretpad.d.ts
        │
        ├── api.ts     使用 paths 类型
        └── client.ts  使用 components 类型 + Zod 运行时校验
```

> 统计数字会随路由变化。本文的 157 / 214 / 121 是 2026-10-21 对上述两个文件的实测值；更新接口后请重新统计，不要在多处复制易失数字。

## 3. 后端契约链

### 3.1 事实来源

生成器不维护第二份路由表，信息全部来自代码：

| 信息 | 来源 |
| --- | --- |
| 路径与方法 | `NewRouter` / `NewInnerRouter` 实际注册的路由（仅 `/api/` 前缀） |
| 请求结构 | Handler 中第一次 `ShouldBindJSON` / `ShouldBind` / 名称含 `bind` 的绑定调用所传的 `&req` |
| 响应结构 | Handler 中第一次 `response.OK(c, v)` 的 `v` |
| 字段名 | Go struct 的 `json` tag；无 tag 时用 `form` tag；再退回字段名 |
| 必填字段 | `binding:"required"` |
| 摘要 | Handler 函数 Go doc 注释的第一段（仅首行写入 `summary`） |
| 安全元数据 | `middleware.PublicPaths`、`AdminOnlyPaths`、`AuditorPaths`、`APIResourceCodes` |
| 内部端口标记 | 路由来自 `NewInnerRouter` |

因此，**改路由或 DTO 就必须重新生成**；只改实现逻辑、不改签名时通常不需要。

### 3.2 生成命令

```bash
cd privahub

# 写入提交路径（Makefile 目标）
make openapi

# 等价命令
CGO_ENABLED=1 go run ./cmd/openapi-gen

# 输出到临时文件，不改工作区
CGO_ENABLED=1 go run ./cmd/openapi-gen -out /tmp/privahub-openapi.json
```

生成需要 CGO（SQLite 驱动）。生成器会：

1. 设置 `PRIVAHUB_PROFILE=test` 并加载配置。
2. 使用临时目录中的 SQLite 数据库、节点池文件、初始密码文件与审计 spool，不影响本地数据。
3. 执行 `AutoMigrate`，构建 `wire.App`，再构建主端口与内部端口两个真实路由。
4. 收集所有 `/api/` 路由并排序。
5. 用 `golang.org/x/tools/go/packages` 加载 `./internal/controller/http/...`，按 Handler 名称定位函数体并推导请求/响应。
6. 以 2 空格缩进写出 JSON，并在标准输出打印 operation 数量。

### 3.3 派生规则

| OpenAPI 字段 | 生成规则 | 备注 |
| --- | --- | --- |
| `paths` / method | 主端口 + 内部端口所有 `/api/` 路由 | 非 `/api/` 前缀的 `/healthz`（内部端口）、`/sync`、`/metrics` 不在文档内；`/api/v1alpha1/healthz` 会包含 |
| `operationId` | `<method>_<去掉 /api/ 并替换分隔符的路径>`；内部端口加 `inner_` 前缀 | 路径重命名会改变 operationId |
| `tags` | `/api/v1alpha1/` 后第一段；`p2p/*` 保留两级 | 用于文档分组 |
| `summary` | Handler Go doc 第一段的第一行 | 无注释则缺失 |
| `requestBody` | 第一次绑定调用的 `&req` 类型 | **仅 POST/PUT 生成**；找不到时为 `{"type":"object"}` |
| `requestBody` 媒体类型 | Handler 调用 `FormFile` / `MultipartForm` / `PostForm` 时为 `multipart/form-data` | 其余为 `application/json` |
| `responses.200` | `response.OK(c, v)` 的 `v` 包进统一 envelope | 业务错误仍是 HTTP 200，`status.code != 0` |
| `data` 字段 | `response.OKEmpty` 不产生 `data` | 只描述成功形态 |
| `components.schemas` | 命名 struct 转为 `包名.类型名`，泛型追加类型参数 | 结构体引用使用 `$ref` |
| `required` | `binding:"required"` 的字段 | 按字母排序 |
| 全局 `security` | `userToken`（`User-Token` 请求头） | 登录会话仍以 Cookie 为主，此处只描述契约 |
| 公共接口 | `middleware.PublicPaths` → `security: []` | 例如 `/api/login`、`/api/v1alpha1/user/login`、`/env` |
| 内部接口 | → `x-inner-port: true`，`security: [{kusciaOrigin: []}]` | `kuscia-origin-source` 请求头 |
| 管理员接口 | `AdminOnlyPaths` → `x-admin-only: true` | 不改变 `security` 列表 |
| 审计员接口 | `AuditorPaths` → `x-roles: [AUDITOR, ADMIN]` | 不改变 `security` 列表 |
| API 资源码 | `middleware.APIResourceCodes[path]` → `x-api-resource` | 路径级，不区分方法 |
| 处理函数 | → `x-handler` | 完整 Go 函数名，便于定位 |
| 主/内网同路径 | 只保留一个 operation，并加 `x-also-inner-port: true` | 见 §3.4 已知偏差 |

常用扩展字段速查：

| 扩展字段 | 含义 | 消费方 |
| --- | --- | --- |
| `x-handler` | 实际处理函数 | 评审、排障 |
| `x-api-resource` | RBAC 接口资源码 | 安全评审、权限核对 |
| `x-inner-port` | 仅内部端口注册 | 网关、安全评审 |
| `x-also-inner-port` | 主端口和内部端口都注册 | 网关、安全评审 |
| `x-admin-only` | 仅 ADMIN / CENTER_ADMIN | 前端、评审 |
| `x-roles` | 允许的角色列表 | 前端、评审 |

### 3.4 生成器的已知边界

生成器是“源码推导”，不是完整契约描述工具。以下内容 **不会** 出现在文档中，评审时不能假设文档没写就不存在：

1. **无 path / query / header 参数**。查询参数、路径参数、分页参数、请求头（除安全方案）都不会生成。
2. **无 4xx/5xx 响应**。认证失败、权限不足、限流、参数错误、资源不存在等 HTTP 状态码未描述。
3. **每个 Handler 只取第一组请求/响应类型**。同一接口不同分支返回不同结构时，只记录第一次 `response.OK` 的类型。
4. **间接绑定/响应会丢失**。若绑定或响应经 Helper 包装且名称不含 `bind`，或响应不是直接调用 `response.OK`，schema 会退化为空对象。
5. **`ShouldBindQuery` 也会被当作请求体来源**。对 POST/PUT 若使用 query 绑定，生成的 `requestBody` 可能不符合实际；此类接口需人工核对。
6. **自定义序列化不展开**。`json.RawMessage`、接口字段、自定义 `MarshalJSON` 只能得到 `{}` 或宽泛对象。
7. **无示例、默认值、枚举**。`binding` 中的范围/枚举约束不会转成 OpenAPI 约束。
8. **只分析 `internal/controller/http/...` 包**。Handler 必须位于该目录树内，生成器才能定位函数体。
9. **同路径同方法的双端口注册存在偏差**。生成器注释意图保留主端口版本，但排序不保证主端口优先；当前 `/api/v1alpha1/user/node/resetPassword` 保留的是内部端口版本（带 `x-inner-port`、`kusciaOrigin` 和 `post_inner_...` operationId），同时标记 `x-also-inner-port`。修改该逻辑前，请以实际生成的 JSON 为准。
10. **运行环境有要求**。生成过程会建库、迁移、构造完整应用；配置校验失败、CGO 缺失或 Kuscia 客户端初始化异常都会导致生成失败，而不是退化为静态扫描。

这些边界意味着：OpenAPI 文档适合做路由清单、请求/响应类型参考和漂移检测，**不能替代** [`api.md`](api.md)、业务测试或安全评审。

### 3.5 服务与访问控制

- 文档通过 `//go:embed openapi.json` 编译进二进制，由 [`apidocs.Handler`](../internal/controller/http/apidocs/apidocs.go) 直接返回。
- 路由为 `GET /api/v1alpha1/docs`，注册逻辑见 [`registerAPIDocs`](../internal/controller/http/router.go)。
- `server.api_docs.enabled`：dev/test 默认 `true`，其他 Profile 默认 `false`；关闭时路由不注册（404）。
- `server.api_docs.public`：默认 `false`，需要登录会话；设为 `true` 才允许匿名访问。
- `/api/v1alpha1/docs` 在 camelCase 中间件的排除列表中，文档中的属性名保持 Go `json` tag 原样，不做 camelCase 改写。
- 文档自身也在路由表中，因此会出现在生成的 `paths` 里；它被归类为“无需接口权限检查”，但仍受 `api_docs` 配置与登录要求约束。

## 4. 后端漂移门禁

### 4.1 四道门禁

| 门禁 | 命令 / 位置 | 检查内容 |
| --- | --- | --- |
| 本地 Makefile | `make openapi-check` | 重新生成到临时文件并与提交文件 `diff -u` |
| 脚本 | [`scripts/check_openapi_drift.sh`](../scripts/check_openapi_drift.sh) | 生成失败、输出为空、内容不一致均失败 |
| 单元测试 | `TestOpenAPIDocumentCoversRoutes`（[`router_test.go`](../internal/controller/http/router_test.go)） | 双向比对：注册路由必须在文档中，文档中的路由必须已注册 |
| CI | [`.github/workflows/ci.yml`](../.github/workflows/ci.yml) 的 `openapi` Job | 运行 `make openapi-check`，再原地生成并要求 `git diff --exit-code` |

`make check` 也包含 `openapi-check`（见 [`Makefile`](../Makefile)）。CI 还会显式执行一次原地生成并检查工作区是否干净，防止脚本层面的漏检。

### 4.2 漂移脚本行为

脚本要点：

1. 校验 `internal/controller/http/apidocs/openapi.json` 存在。
2. 在 `mktemp -d` 目录生成新文档，**不会修改工作区**。
3. 用 `diff -u` 比较，失败时打印前 60 行差异。
4. 失败提示固定为：

   ```text
   fix: CGO_ENABLED=1 go run ./cmd/openapi-gen && commit internal/controller/http/apidocs/openapi.json
   ```

### 4.3 失败处理流程

1. 先看 diff：是新增/删除路由、DTO 字段变化、注释变化，还是安全分类变化。
2. **变更是有意的**：执行 `make openapi`，把 `openapi.json` 与代码放在同一个提交中；在 PR 中说明契约影响。
3. **变更不是有意的**：修正路由、DTO 或分类表，再重新生成；不要通过改 JSON “修好”测试。
4. **只有注释变化导致 `summary` 变化**：也属于契约变化，重新生成并提交。
5. **生成器本身有问题**：先补测试或最小复现，再改 [`cmd/openapi-gen/main.go`](../cmd/openapi-gen/main.go)，并重新生成全部文档。
6. **不要** 手工编辑 JSON、只提交部分 diff，或用 `git checkout` 丢弃他人刚生成的变更而不核对。

### 4.4 为什么不能手改

- 文件是 `//go:embed` 的编译输入，手改会直接改变线上 `/api/v1alpha1/docs`。
- `TestOpenAPIDocumentCoversRoutes` 会双向比对，CI 的 `git diff --exit-code` 也会失败。
- 手工修改通常只改“看到的字段”，却遗漏 `components`、`required`、`security` 或 operationId，造成更难发现的契约漂移。

## 5. 前端契约链

### 5.1 当前基线

[`web/openapi/secretpad.openapi.json`](../web/openapi/secretpad.openapi.json) 是 Java SecretPad 的 springdoc 导出：

- `openapi: 3.0.1`，标题 `Secretpad Restful API`，版本 `0.0.1-SNAPSHOT`。
- 121 条路径，包含 `/api/login`、`/api/v1alpha1/vote_sync/create` 等 Java 时期路径。
- `servers` 指向 `http://127.0.0.1:8080`，只用于展示，不参与类型生成。
- 该文件是前端 [`web/docs/frontend-migration-consistency.md`](../web/docs/frontend-migration-consistency.md) 声明的“唯一源”，但它与 Go 路由不是同一份契约。

### 5.2 生成命令

```bash
cd privahub/web
corepack enable
corepack pnpm install

# 读取 web/openapi/secretpad.openapi.json
# 写入 packages/api-client/src/generated/secretpad.d.ts
corepack pnpm codegen:openapi
```

脚本 [`scripts/codegen-openapi.ts`](../web/scripts/codegen-openapi.ts) 的行为：

1. 检查 `openapi/secretpad.openapi.json` 是否存在。
2. 调用 `openapi-typescript <spec> -o <out> --alphabetize`。
3. 生成文件提交到仓库，供 TypeScript 编译使用。

**过期提示**：脚本找不到 spec 时提示 `curl http://127.0.0.1:8080/v3/api-docs`。Go 后端没有 `/v3/api-docs` 路由，该提示只适用于 Java 后端；Go 环境请使用 `CGO_ENABLED=1 go run ./cmd/openapi-gen` 或 `/api/v1alpha1/docs`。

### 5.3 前端如何消费

| 文件 | 使用方式 | 风险点 |
| --- | --- | --- |
| [`packages/api-client/src/api.ts`](../web/packages/api-client/src/api.ts) | `import type { paths } from './generated/secretpad'`，供 `openapi-fetch` 推导 path/method | 基线中不存在的 Go-only 路由需要类型断言 |
| [`packages/api-client/src/client.ts`](../web/packages/api-client/src/client.ts) | 使用 `components['schemas'][...]`，并用 `openapi-fetch` 发起请求 | 大量 `RawJson = any` 与 `as unknown as` 绕过静态检查 |
| [`packages/api-client/src/schemas/index.ts`](../web/packages/api-client/src/schemas/index.ts) | Zod schema | 运行时真正的字段校验在这里，而不是生成的 `.d.ts` |
| `client.ts` 的 `validated` / `unwrapValidated` | `safeParse` 失败时抛出带字段路径的错误 | 新增字段若未更新 Zod schema，运行时仍可能判失败 |

因此：

- **生成的 TypeScript 类型不等于完整契约**。`RawJson = any` 的路径没有静态保障。
- **Zod 是当前最有效的运行时守卫**。修改响应结构时，必须同步 `schemas/index.ts` 与相关测试。
- **Go 增加接口不会自动出现在前端类型中**。如果前端要调用新接口，需要手工更新基线或改用 Go 文档生成，并补充 Zod schema。

### 5.4 已确认的差距

1. 前端基线不是从 Go 路由生成的，路径集合与字段可能落后于后端。
2. Go-only 路由（例如部分 alias、节点路由、P2P 扩展）在基线中不存在，客户端靠断言调用。
3. Java 基线中的部分路径/字段可能在 Go 实现中语义不同或不存在，反向核对同样必要。
4. 前端没有“生成物与 Go 文档一致”的 CI 门禁；后端 `openapi-check` 不会检查 `web/openapi/secretpad.openapi.json`。
5. codegen 缺失文件的提示仍指向 Java 的 `/v3/api-docs`。
6. 前端生成的 `.d.ts` 由 `--alphabetize` 排序；手工编辑会在下次生成时被覆盖。

### 5.5 切换到 Go 文档的建议步骤（尚未执行）

若计划让前端直接消费 Go 生成的文档，建议单独提交、分步验证，不要直接覆盖旧文件：

1. 确认后端 `make openapi-check` 通过，路由与 DTO 已冻结到本次迁移范围。
2. 导出到新文件，保留旧基线便于回滚：

   ```bash
   cd privahub
   CGO_ENABLED=1 go run ./cmd/openapi-gen -out web/openapi/privahub.openapi.json
   ```

3. 对比 operation 集合，先处理缺失/多余路径：

   ```bash
   cd privahub
   python3 - <<'PY'
   import json

   def ops(path):
       doc = json.load(open(path))
       out = set()
       for p, item in doc["paths"].items():
           for m in item:
               if not m.startswith("x-"):
                   out.add(f"{m.upper()} {p}")
       return out

   old = ops("web/openapi/secretpad.openapi.json")
   new = ops("web/openapi/privahub.openapi.json")
   print("only in old:", len(old - new))
   print("only in new:", len(new - old))
   PY
   ```

4. 先把 codegen 输出到临时文件并运行 `pnpm typecheck`，统计需要移除的类型断言与需要补的 Zod schema。
5. 更新 `scripts/codegen-openapi.ts` 的输入路径与错误提示，保留别名兼容期。
6. 为前端增加“生成的 `.d.ts` 与 spec 一致”的 CI 检查，再删除旧基线。
7. 同步更新 [`web/docs/frontend-migration-consistency.md`](../web/docs/frontend-migration-consistency.md) 与本文。

注意：Go 文档的 operationId、schema 命名（`service.LoginRequest` 等）、安全方案（`User-Token`）与 Java 基线不同。直接替换会影响 `paths` 类型、`components` 引用和手写断言，不能视为纯文件替换。

## 6. 同步工作流

### 6.1 新增接口

按顺序完成，缺一项都可能让门禁或联调失败：

1. **路由**：在 [`router.go`](../internal/controller/http/router.go) 或 [`inner_router.go`](../internal/controller/http/inner_router.go) 注册；确认路径符合既有命名。
2. **Handler**：放在 `internal/controller/http/...` 下，写清 Go doc 第一段（会成为 `summary`）。
3. **请求类型**：使用命名 struct，补 `json` tag 与 `binding:"required"`；不要用匿名 `gin.H` 作为请求体。
4. **响应类型**：直接调用 `response.OK(c, v)`，`v` 使用命名 struct；空响应使用 `response.OKEmpty`。
5. **权限分类**：在 [`middleware/rbac.go`](../internal/controller/http/middleware/rbac.go) 登记 `APIResourceCodes`、`PublicPaths` / `AdminOnlyPaths` / `AuditorPaths` / `NoInterfaceCheckPaths`。
6. **数据权限**：在 [`middleware/data_resource.go`](../internal/controller/http/middleware/data_resource.go) 加入 `DataResourceRoutes`，或登记到 `DataResourceExempt` 并写明非空理由。
7. **重新生成**：`make openapi`。
8. **检查元数据**：确认 `security`、`x-api-resource`、`x-inner-port` / `x-admin-only` 是否符合预期。
9. **测试**：`CGO_ENABLED=1 go test ./internal/controller/http/...`，至少覆盖路由、鉴权与 Handler 行为。
10. **文档**：更新 [`api.md`](api.md)；若示例流程变化，同步 [`example.md`](example.md)。
11. **前端**（如前端调用）：更新 `web/openapi/secretpad.openapi.json` 或按 §5.5 切换源，重新 codegen，补 Zod schema 与客户端方法。
12. **提交**：代码、测试、`openapi.json`、前端生成物与文档放在同一变更中。

### 6.2 修改请求或响应 DTO

- 改字段名、类型、必填性、`json` tag：重新生成后端文档。
- 新增字段：确认前端 Zod schema 是否允许（`.strict()` 会拒绝未知字段；普通 object 通常会忽略）。
- 删除/重命名字段：按破坏性变更处理，检查前端 `client.ts`、`schemas/index.ts` 与测试。
- 只改字段注释：不会进入 OpenAPI；如需要描述，写 Handler doc 或扩展生成器。

### 6.3 删除或重命名接口

1. 确认没有网关、前端、EDGE 同步或外部集成仍在使用。
2. 删除路由、Handler、权限分类与数据权限登记。
3. 重新生成，确认 `paths` 与 operationId 的删除 diff。
4. 前端删除对应方法与 Zod schema；若保留兼容期，使用 alias 路由并在 `api.md` 标注弃用。
5. 在 PR 中明确“破坏性变更”与回滚方案。

### 6.4 只改配置、错误码或中间件

| 变更 | 是否需要重新生成 OpenAPI | 还需要更新 |
| --- | --- | --- |
| `server.api_docs.*` | 否 | [`configuration.md`](configuration.md)、访问控制说明 |
| 业务错误码 | 否（文档只描述 200 envelope） | `pkg/errcode`、[`api.md`](api.md)、前端错误处理 |
| `APIResourceCodes` / 角色分类 | 是（`x-api-resource` / `x-admin-only` / `x-roles`） | 权限测试、安全评审 |
| `PublicPaths` | 是（`security` 变化） | 威胁模型、加固指南 |
| `DataResourceRoutes` | 否（不进入文档） | 数据权限测试、授权复核 |
| camelCase 排除列表 | 否（文档属性名不变） | 中间件测试、前端字段处理 |

## 7. PR 模板

将以下内容复制到 PR 描述，按实际变更删减：

```markdown
## 变更类型

- [ ] 新增接口
- [ ] 修改请求/响应 DTO
- [ ] 删除/重命名接口
- [ ] 仅实现或修复（无契约变化）
- [ ] 仅配置/错误码/中间件分类

## 契约影响

- 新增路径：
- 删除路径：
- 变更字段（请求/响应）：
- 权限或安全元数据变化（security / x-api-resource / x-admin-only / x-inner-port）：
- 破坏性变更：是 / 否
- 兼容期与回滚方案：

## 生成物

- [ ] `internal/controller/http/apidocs/openapi.json` 已由 `make openapi` 重新生成
- [ ] 前端 `secretpad.d.ts` 已由 `pnpm codegen:openapi` 重新生成（如涉及前端契约）
- [ ] 未手工编辑任何生成文件

## 验证

- [ ] `make openapi-check`
- [ ] `CGO_ENABLED=1 go test ./internal/controller/http/...`
- [ ] `corepack pnpm typecheck`（web）
- [ ] `corepack pnpm test`（web，如涉及客户端或 Zod schema）
- [ ] 手动/自动化验证关键接口（附命令或截图）

## 文档

- [ ] `docs/api.md` 已更新
- [ ] `docs/example.md` 已更新（如流程变化）
- [ ] `docs/development.md` / `docs/configuration.md` 已更新（如规则或配置变化）
- [ ] `docs/openapi-sync.md` 的统计数字或已知差距已复核（如适用）
```

## 8. 评审责任与维护约定

| 角色 | 责任 |
| --- | --- |
| 后端作者 | 路由/DTO 变更时同一提交内重新生成 `openapi.json`；保证权限与数据权限分类完整 |
| 后端评审 | 核对 diff 是否只包含预期路径/字段；检查 `security`、`x-api-resource`、`x-inner-port` |
| 前端作者 | 更新前端基线或生成物、Zod schema、客户端方法与测试 |
| 前端评审 | 确认 `RawJson` 断言没有扩大、Zod schema 与实际响应一致 |
| 安全评审 | 新增路径、公共路径、内部端口、管理员接口、数据资源映射必须逐项确认 |
| 文档维护者 | 同步 `api.md`、`example.md` 与本文；统计数字只在核对后更新 |

维护规则：

1. 生成文件与代码同提交，不把 `openapi.json` 留到“下个 PR”。
2. 不把“文档里有这个路径”当作“接口已验收”；业务验收仍需测试或真实调用证据。
3. 新增 `x-*` 元数据时，先在生成器中实现并测试，再写入本文表格。
4. 前端基线切换属于独立变更，必须保留回滚路径。
5. 每次修改生成器，都要重新生成全部文档并检查无关 diff。

## 9. 故障排查

| 现象 | 常见原因 | 处理 |
| --- | --- | --- |
| `openapi-gen: load ...` | Handler 包编译失败或不在 `internal/controller/http/...` | 先修复编译错误；确认 Handler 位置 |
| `openapi-gen` 建库/迁移失败 | CGO 未开启、临时目录不可写、配置校验失败 | 使用 `CGO_ENABLED=1`；检查 `TMPDIR` 与 test profile 配置 |
| `openapi-check` 报 diff | 路由/DTO/注释/分类变化后未重新生成 | 核对 diff，执行 `make openapi` 并提交 |
| 新增路由未出现在文档 | 路径不以 `/api/` 开头，或注册在生成器未构建的 Engine 上 | 检查路径前缀与注册位置 |
| `requestBody` 只有 `{"type":"object"}` | 绑定经 Helper 包装、名称不含 `bind`，或使用匿名类型 | 改为直接 `ShouldBindJSON(&req)` + 命名 struct，或扩展生成器 |
| 响应缺少 `data` | Handler 使用 `response.OKEmpty`，或响应不是直接 `response.OK` | 确认是否符合预期；需要 schema 时改为直接 `response.OK(c, v)` |
| `summary` 为空 | Handler 没有 Go doc 注释 | 补第一段注释并重新生成 |
| `/api/v1alpha1/docs` 返回 404 | `server.api_docs.enabled=false` | dev/test 默认开启；生产需显式开启 |
| `/api/v1alpha1/docs` 返回 401 | `server.api_docs.public=false`，未登录 | 登录后访问，或仅在受控环境设 `public=true` |
| 前端 codegen 提示 spec 不存在 | 基线文件被移动/删除 | 恢复文件；Go 环境不要按提示访问 `/v3/api-docs` |
| 前端 `pnpm typecheck` 大量报错 | 生成物与客户端断言不匹配 | 先对比 operation 集合，再逐个修复类型断言与 Zod schema |
| 同一路径出现两个 operationId | 生成器按路径合并，实际只保留一个 | 以 `paths[path]` 下的 method 为准；必要时检查 `x-also-inner-port` |

## 10. 附录

### 10.1 命令速查

```bash
# 后端（在 privahub 根目录）：重新生成并检查
cd privahub
make openapi
make openapi-check

# 后端：只跑文档相关测试
CGO_ENABLED=1 go test ./internal/controller/http/ \
  -run 'TestOpenAPIDocumentCoversRoutes|TestAPIDocsEndpoint' -v

# 后端：查看运行时文档（需登录，或 api_docs.public=true）
curl -s http://127.0.0.1:8080/api/v1alpha1/docs | head

# 前端（在 privahub/web 目录）：重新生成类型并验证
cd web
corepack pnpm codegen:openapi
corepack pnpm typecheck
corepack pnpm test

# 统计后端文档规模（在 privahub 根目录）
cd ..
python3 - <<'PY'
import json
d = json.load(open("internal/controller/http/apidocs/openapi.json"))
ops = [(m, p) for p, item in d["paths"].items() for m in item if not m.startswith("x-")]
print("paths:", len(d["paths"]), "operations:", len(ops), "schemas:", len(d["components"]["schemas"]))
PY
```

### 10.2 文件地图

| 文件 | 作用 |
| --- | --- |
| [`internal/controller/http/router.go`](../internal/controller/http/router.go) | 主端口路由；`registerAPIDocs` |
| [`internal/controller/http/inner_router.go`](../internal/controller/http/inner_router.go) | 内部端口路由；EDGE 转发目标 |
| [`internal/controller/http/middleware/rbac.go`](../internal/controller/http/middleware/rbac.go) | 接口资源码、公共/管理员/审计员/免检查分类 |
| [`internal/controller/http/middleware/data_resource.go`](../internal/controller/http/middleware/data_resource.go) | 数据资源路由与豁免 |
| [`cmd/openapi-gen/main.go`](../cmd/openapi-gen/main.go) | 生成器 |
| [`internal/controller/http/apidocs/apidocs.go`](../internal/controller/http/apidocs/apidocs.go) | embed 与 HTTP Handler |
| [`internal/controller/http/apidocs/openapi.json`](../internal/controller/http/apidocs/openapi.json) | 后端提交的生成文档 |
| [`scripts/check_openapi_drift.sh`](../scripts/check_openapi_drift.sh) | 漂移检查脚本 |
| [`internal/controller/http/router_test.go`](../internal/controller/http/router_test.go) | 路由与文档双向覆盖测试 |
| [`web/openapi/secretpad.openapi.json`](../web/openapi/secretpad.openapi.json) | 前端历史基线 |
| [`web/scripts/codegen-openapi.ts`](../web/scripts/codegen-openapi.ts) | 前端 codegen 脚本 |
| [`web/packages/api-client/src/generated/secretpad.d.ts`](../web/packages/api-client/src/generated/secretpad.d.ts) | 生成的 TypeScript 类型 |
| [`web/packages/api-client/src/api.ts`](../web/packages/api-client/src/api.ts) | `paths` 类型消费方 |
| [`web/packages/api-client/src/client.ts`](../web/packages/api-client/src/client.ts) | `components` 类型、请求方法与 `RawJson` 断言 |
| [`web/packages/api-client/src/schemas/index.ts`](../web/packages/api-client/src/schemas/index.ts) | Zod 运行时校验 |

### 10.3 相关文档

- [`api.md`](api.md)：按模块的接口说明与错误码。
- [`example.md`](example.md)：curl 业务链路示例。
- [`development.md`](development.md)：新增端点的开发流程与测试约定。
- [`configuration.md`](configuration.md)：`server.api_docs` 等配置项。
- [`../web/docs/frontend-migration-consistency.md`](../web/docs/frontend-migration-consistency.md)：前端契约基线的历史声明。

### 10.4 变更记录

| 日期 | 说明 |
| --- | --- |
| 2026-10-21 | 初版：梳理后端生成链、漂移门禁、前端历史基线、切换建议与 PR 模板 |
