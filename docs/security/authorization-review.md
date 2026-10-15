# 认证与权限复核（2026-10-15）

## Scope Key 与当前系统的关系

当前 Privahub **没有入站 Scope Key 配置或签发接口**，不存在 `scope_keys` 可供调整。
主 API 使用服务器端会话、JWT、接口资源码和节点/项目归属；Kuscia Token 是出站凭据，
不能作为 Privahub API 的入站授权凭据。不要将两类 Token 混配。

本次参考 `/home/charles/code/PrivShield/services/service-hub` 及其共享认证实现：

- `pkg/auth/scope_clamp.go`：凭据不能因持有者改权而静默扩权，撤权必须影响旧凭据。
- `pkg/auth/key_contract.go`：凭据权限契约只收窄访问，不能替代其他授权层。
- `services/service-hub/internal/taskaccess/access.go`：接口许可与数据范围分别裁决。

采用这些原则修复现有会话链路，不引入另一套可绕过会话授权的 API Key 通道。
没有修改参考项目，也没有复制其依赖或读取其密钥内容。

## 已修复问题

| 问题 | 改进 |
| --- | --- |
| 角色缓存仅以用户名为键，不区分数据库；TTL 延迟撤权，进程间失效通知不一致 | ADMIN/AUDITOR 检查直接查询当前数据库，查询失败拒绝；按已索引的 user_key 查询，不保留正向授权缓存 |
| 修改角色/节点/所有者后，旧会话仍保存登录时的权限快照 | 在同一事务内修改账号/关系、增加授权版本并删除所有 access/refresh 会话；新权限需重新登录 |
| 并发登录可能在撤权删除会话后才保存旧权限会话 | 账号与会话均记录 `authorization_version`，访问及刷新都校验版本；晚到的旧登录结果不能使用 |
| 所有者在事务外更新；全行保存可能覆盖并发密码修改 | 所有者、关系、版本和会话删除原子提交；只更新指定账号字段 |
| 角色查询/更新混淆同名人类账号与虚拟节点 | 只操作 USER/EDGE_USER 的 ROLE 记录，保留 NODE 记录；EDGE 账号写入 EDGE_USER，与登录查询一致 |
| AUTONOMY 的路径白名单跳过归属检查，下游未必复查 | 移除整条路由放行；项目/节点仍须有明确成员关系；非空旧配置启动报错 |
| 归档项目只要存在任意成员，就允许无关租户访问 | 归档不扩大权限，仍须匹配 project_node/project_inst 成员关系 |
| 多认证头可能选择不同身份；非法 Bearer 可能回落到 Cookie | 拒绝冲突、重复、空值、非法方案；相同令牌跨旧/新头兼容；认证头校验和提取使用一致解析规则 |
| 删除账号未删除刷新会话记录 | 删除账号时同步清理其所有会话，避免残留凭据 |

`InvalidateUserCache` 与 `RBACConfig.CacheDuration` 暂保留兼容，已不缓存特权角色。

## 升级与兼容性

GORM AutoMigrate 为 `user_accounts` 和 `user_tokens` 增加
`authorization_version BIGINT NOT NULL DEFAULT 0`（实际类型由数据库驱动决定）。
旧配置如含 `auth.data_resource_whitelist`，须删除该项，改为显式项目/节点成员授权；空配置不受影响。

两侧授权版本初始值均为 0，已有会话可继续使用；管理员下一次改权会使目标用户所有会话立即失效。
访问/刷新令牌格式及前端响应字段不变。升级需具备数据库加列权限，并按现有流程备份。

本次维持已有 CENTER、EDGE 平台接口策略和 ADMIN 审计可读策略；**没有**宣称这些策略
已经等同于 PrivShield 的三权分立，也没有把宽权限套用到新的机器凭据上。
未来增加 Scope Key 时应独立设计：签发权限上限、精确 METHOD/path 契约、数据范围、
受众、期限/吊销、摘要存储、凭据轮转和工作负载证书绑定；不得默认赋予 CENTER 身份。

## 回归验证

```bash
CGO_ENABLED=1 go test ./...
CGO_ENABLED=1 go test -race ./internal/service/... ./internal/controller/http/...
CGO_ENABLED=1 go vet ./...
python3 scripts/run_system_e2e.py --browser
```

新增回归覆盖跨数据库同名账号、实时角色撤销、存储失败拒绝、同名 NODE 隔离、
归档项目越权、P2P 成员正反向访问与旧白名单失效、冲突认证头、改权事务回滚、并发登录晚到的授权版本、旧数据库升级。
系统脚本新增真实 HTTP 流程：创建临时 ADMIN → 首次改密 → 管理接口允许 →
撤销 ADMIN → 旧 access/refresh 均拒绝 → 重新登录身份有效但管理接口拒绝 → 删除临时账号。

该机制保证改权提交后的新认证读取拒绝旧版本；已通过授权、正在执行的请求不会被强行中断。

本轮实测：Go 全量测试、服务/路由/config race、vet 均通过；隔离 HTTP 权限撤销闭环、
10 项 Chromium 测试及真实双节点 PSI 通过。本机系统报告为
`/tmp/privahub-migration-2hkr3n5a/report.json`（临时产物可按上文命令重建）。
