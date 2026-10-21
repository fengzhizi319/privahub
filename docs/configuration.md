# Privahub 配置参考

本文逐项说明 Privahub 的配置来源、加载优先级、全部配置键、默认值、Profile 差异与生产校验规则。排障时先用本文定位键名与默认值，再结合启动日志中的 `config validation failed` 信息修改。

| 项目 | 说明 |
| --- | --- |
| 读者 | 运维、SRE、后端、安全 |
| 状态 | 现行 |
| 更新日期 | 2026-10-21 |
| 代码依据 | `pkg/config/`、`internal/graphjob/settings.go`、`cmd/server/main.go`、`config/*.yaml` |
| 验证方式 | `go test ./pkg/config/`、启动日志、`PRIVAHUB_PROFILE=dev ./bin/privahub` |

## 1. 配置文件与加载顺序

### 1.1 文件位置

| 方式 | 行为 |
| --- | --- |
| `-config /path/privahub.yaml` | 只读取指定文件作为主配置 |
| 未指定 `-config` | 依次在 `./config`、`/etc/privahub`、`.` 查找 `privahub.yaml` |
| Profile 文件 | 在主配置同一组目录中查找 `privahub-<profile>.yaml`，存在则合并 |

Profile 由以下任一方式确定：

1. 主配置中的 `profile: dev` 键。
2. 环境变量 `PRIVAHUB_PROFILE=dev`。

如果 Profile 文件不存在，加载器不会报错，只是不合并覆盖。因此“设置了 `PRIVAHUB_PROFILE=p2p` 但没有 `privahub-p2p.yaml`”仍然会使用基础配置；由于 `p2p` 不在 `IsDev()` 列表中，后续按生产语义校验（见 §1.4）。

### 1.2 优先级（从低到高）

1. 代码内置默认值（`setDefaults`、`setAuditDefaults`、`graphjob.DefaultSettings`）。
2. 主配置文件（`privahub.yaml`）。
3. Profile 文件（`privahub-<profile>.yaml`，按 key 深度合并）。
4. `PRIVAHUB_*` 环境变量。
5. 特殊来源（在反序列化后应用）：
   - `crypto.keys_file` 文件中的密钥；
   - `PRIVAHUB_CRYPTO_KEY_<VERSION>` 单版本密钥；
   - `PRIVAHUB_KUSCIA_NODES_<DOMAIN>_*` 单节点覆盖。

同一层级内，后应用的值覆盖先前的值。例如 `crypto.keys`（文件）< `crypto.keys_file` < `PRIVAHUB_CRYPTO_KEY_V2`。

### 1.3 环境变量命名

- 前缀固定为 `PRIVAHUB_`。
- 键名中的 `.` 替换为 `_`，其余字符保持。
- 示例：`server.http_port` → `PRIVAHUB_SERVER_HTTP_PORT`；`auth.jwt_secret` → `PRIVAHUB_AUTH_JWT_SECRET`；`kuscia.api_address` → `PRIVAHUB_KUSCIA_API_ADDRESS`。
- 标量切片按逗号分隔，例如 `PRIVAHUB_SERVER_TRUSTED_PROXIES=10.0.0.10,10.0.0.11`。
- 时长使用 Go duration 字符串，例如 `30s`、`24h`、`1m30s`。
- 布尔值使用 `true` / `false`。

不是所有键都能用一个环境变量表达：

| 类型 | 可用方式 |
| --- | --- |
| `crypto.keys`（map） | `crypto.keys_file` 或 `PRIVAHUB_CRYPTO_KEY_<VERSION>` |
| `kuscia.nodes`（结构体切片） | 配置文件中声明节点，再用 `PRIVAHUB_KUSCIA_NODES_<DOMAIN>_{HOST,PORT,HTTP_PORT,PROTOCOL}` 覆盖 |
| `server.extra_response_headers`（map） | 只能写在配置文件中 |
| `graph.versions`（map） | 只能写在 `graph.yaml` 中（或整体替换文件） |

`<DOMAIN>` 的规则：`domain_id` 转大写，非 `[A-Z0-9]` 字符替换为 `_`。例如 `kuscia-system` → `KUSCIA_SYSTEM`，`edge-node-1` → `EDGE_NODE_1`。这些覆盖只作用于配置文件中已声明的节点，未知域名会被忽略。

### 1.4 Profile 与 dev 语义

`Config.IsDev()` 仅对以下 Profile 返回 true：

- `dev`
- `test`
- `local`

其余情况（包括未设置 Profile）都按**生产**处理。`p2p`、`edge`、`autonomy` 等 Profile 名称不会自动放宽安全校验；它们是否可选取决于是否满足生产要求。

dev/test/local 的放宽项：

- 可使用内置开发 JWT 密钥与示例 AES 密钥。
- 允许 `auth.enabled=false`。
- 允许 `kuscia.protocol=notls`、`kuscia.insecure_skip_verify=true`。
- 允许主端口明文 HTTP。
- 允许内部端口仅绑定回环且不配置 `inner_auth`。
- `/metrics` 默认 `public`，`/api/v1alpha1/docs` 默认启用。

**生产环境不要把 `PRIVAHUB_PROFILE` 设为 `dev`、`test` 或 `local`。**

## 2. 启动校验与失败行为

`config.Load` 在返回前调用 `Validate`，任何错误都会让进程以退出码 1 结束。常见拒绝条件：

| 条件 | 错误要点 |
| --- | --- |
| `server.mode` 非法 | 只能是 master/center、lite/edge、autonomy/p2p、test |
| `database.driver` 非法 | 只能是 sqlite、mysql |
| `server.http_port` 越界 | 1–65535 |
| `auth.data_resource_whitelist` 非空 | 已废弃，必须改用显式项目/节点成员关系 |
| 生产 `auth.jwt_secret` 为空 | 必须设置随机密钥（>= 16 字符） |
| 生产 `auth.jwt_secret` 是示例值 | 内置 dev 密钥、`change-me-in-production` 等会被拒绝 |
| 生产 `crypto.aes_key` 是样例值 | 必须替换为 32 字节随机值 |
| 生产 `auth.enabled=false` | 仅 dev/test 允许关闭认证 |
| 生产主端口明文 | 需要 `server.tls.enabled=true`，或 `server.behind_tls_proxy=true` + `server.trusted_proxies` |
| 生产 `kuscia.protocol` 非 TLS | 必须为 `tls` 或 `mtls`；节点级 `protocol` 同样检查 |
| 生产 `kuscia.insecure_skip_verify=true` | 生产直接拒绝；配置 `kuscia.ca_file` 并关闭该开关 |
| 生产没有静态加密密钥 | 需要 `crypto.keys` / `crypto.aes_key` / `keys_file` / 环境变量之一 |
| 生产内部端口无鉴权 | 即使绑定回环也必须配置 `server.inner_auth` |
| 非回环内部端口无鉴权 | 任何 Profile 都拒绝 |
| `server.inner_auth.token` 少于 16 字符 | 直接拒绝 |
| `server.inner_auth.tls` 三件套不完整 | `cert_file`、`key_file`、`client_ca_file` 必须同时提供 |
| `server.trusted_proxies` 含非法 IP/CIDR | 拒绝 |
| `observability.metrics_access` 非法 | 只能是 public、restricted、auth、disabled |
| `audit.retention_days` 介于 1–1094 | 生产要求 0（永久）或 >= 1095（3 年） |
| 设置了审计保留但 `archive_dir` 为空 | 拒绝 |
| EDGE 使用非回环的明文网关 | 生产拒绝，需 `https://` 或回环地址 |
| `server.tls` 配置不完整 | 启用 TLS 时证书/密钥必填；TLCP 需要四份 SM2 证书 |

启动后还会打印非致命警告（`SecurityWarnings` / `AuditWarnings`）：

- `auth.enabled=false`（仅 dev）。
- `observability.metrics_access=public` 且非 dev。
- `crypto.keys_file` 权限不是 0600/0400。
- 未配置 `audit.anchor.file/url`：审计链只自洽，数据库写入者可重写。
- dev 未配置加密密钥：敏感列明文存储。

## 3. `server` — HTTP 服务与安全边界

| 键 | 默认值 | 说明 |
| --- | --- | --- |
| `mode` | `master` | 平台模式：`master`/`center`、`lite`/`edge`、`autonomy`/`p2p`、`test`；大小写不敏感 |
| `http_port` | `8080` | 主 HTTP 端口；`/metrics` 与 `/api/v1alpha1/docs` 也在此端口 |
| `inner_port` | `9001` | 内部节点通信端口 |
| `grpc_port` | `9090` | 预留；当前主服务不监听 gRPC |
| `read_timeout` | `10s` | HTTP 读取超时 |
| `write_timeout` | `10s` | HTTP 写入超时；SSE 处理器会按事件重置写截止时间 |
| `shutdown_timeout` | `5s` | 优雅退出超时 |
| `read_header_timeout` | `5s` | 防 slowloris |
| `idle_timeout` | `120s` | Keep-Alive 空闲超时 |
| `max_header_bytes` | `1048576` | 最大请求头字节数 |
| `node_id` | 空 | 本平台节点 ID；为空时使用 `kuscia.namespace` |
| `deploy_mode` | `ALL-IN-ONE` | `MPC`、`TEE`、`ALL-IN-ONE`；影响版本列表与 TEE 初始化 |
| `inner_bind_address` | `127.0.0.1` | 内部端口监听地址；跨容器访问时设为 `0.0.0.0` 并配置 `inner_auth` |
| `center_platform_service` | `secretpad.master.svc` | EDGE 通过网关访问中心时使用的 Host |
| `inner_port_paths` | 见下 | 只在内部端口提供、主端口返回 404 的路径 |
| `cors_allowed_origins` | `http://localhost:8000`、`http://127.0.0.1:8000` | CORS 允许来源；`*` 表示任意 |
| `extra_response_headers` | 内置 CSP | 每个响应追加的头；可覆盖 CSP |
| `max_body_size` | `33554432`（32 MiB） | 默认请求体上限；<=0 表示不限 |
| `upload_max_size` | `2147483648`（2 GiB） | 上传路由上限；显式 `-1` 才表示不限，`0` 回落到默认 |
| `upload_paths` | `/api/v1alpha1/data/upload`、`/api/v1alpha1/inst/node/register` | 使用 `upload_max_size` 的路径 |
| `api_docs.enabled` | dev/test：`true`；其他：`false` | 是否注册 `GET /api/v1alpha1/docs` |
| `api_docs.public` | `false` | `false` 时该端点要求登录会话 |

`inner_port_paths` 默认值：

```text
/api/v1alpha1/vote_sync/create
/api/v1alpha1/user/node/resetPassword
/sync
/api/v1alpha1/data/sync
/api/v1alpha1/sync
```

### 3.1 主端口 TLS

| 键 | 默认值 | 说明 |
| --- | --- | --- |
| `tls.enabled` | `false` | 主端口启用 HTTPS/TLCP |
| `tls.cert_file` | 空 | 标准 TLS 服务端证书 |
| `tls.key_file` | 空 | 标准 TLS 私钥 |
| `tls.client_ca_file` | 空 | 可选：要求客户端证书（mTLS） |
| `tls.min_version` | `1.2` | 仅允许 `1.2` 或 `1.3` |
| `tls.stack` | `tls` | `tls`、`tlcp`（国密 GB/T 38636）、`both`（同端口自动识别） |
| `tls.tlcp.sign_cert_file` | 空 | TLCP 签名证书 |
| `tls.tlcp.sign_key_file` | 空 | TLCP 签名私钥 |
| `tls.tlcp.enc_cert_file` | 空 | TLCP 加密证书 |
| `tls.tlcp.enc_key_file` | 空 | TLCP 加密私钥 |
| `tls.tlcp.port` | `0` | 预留键；当前监听器不单独使用该端口 |
| `behind_tls_proxy` | `false` | 声明由反向代理终止 TLS；启用 HSTS 与 Secure Cookie |
| `trusted_proxies` | 空 | 可信代理 IP/CIDR；只有这些来源的 `X-Forwarded-*` 被信任 |
| `trusted_platform` | 空 | 可选 CDN 客户端 IP 头（如 `CF-Connecting-IP`）；仅在平台会剥离客户端伪造头时使用 |

生产判断：`tls.enabled=true` **或**（`behind_tls_proxy=true` 且 `trusted_proxies` 非空）。`server.tls.stack=both` 允许同一端口同时接受 TLS 与 TLCP。

### 3.2 内部端口鉴权

| 键 | 默认值 | 说明 |
| --- | --- | --- |
| `inner_auth.token` | 空 | 共享密钥（>= 16 字符），请求头 `X-Privahub-Inner-Token` |
| `inner_auth.token_file` | 空 | 从文件读取 token；与 `token` 二选一，`token` 优先 |
| `inner_auth.allowed_cidrs` | 空 | 直接对端 IP/CIDR 白名单，例如 Kuscia 网关网段 |
| `inner_auth.tls.cert_file` | 空 | 内部端口 mTLS 服务端证书 |
| `inner_auth.tls.key_file` | 空 | 内部端口 mTLS 私钥 |
| `inner_auth.tls.client_ca_file` | 空 | 客户端 CA；三者必须同时配置 |
| `inner_auth.tls.gateway_names` | 空 | 允许代表任意来源的网关证书 CN/DNS SAN；其他客户端证书的 CN/SAN 必须等于 `kuscia-origin-source` |

已配置的机制会**同时生效**（token、CIDR、mTLS 都检查）。生产环境即使内部端口只绑定 `127.0.0.1` 也必须至少配置一种。

### 3.3 限流

| 键 | 默认值 | 说明 |
| --- | --- | --- |
| `rate_limit.user_permits` | `5` | 每个用户在窗口内的许可数；<=0 关闭 |
| `rate_limit.user_window` | `60s` | 用户限流窗口 |
| `rate_limit.user_burst` | `1` | 用户令牌桶容量；dev 为 `10` |
| `rate_limit.user_paths` | 见下 | 共用同一用户桶的敏感写路径 |
| `rate_limit.login_per_minute` | `20` | 每客户端 IP 的登录尝试上限；<=0 关闭 |
| `rate_limit.login_burst` | `1` | 登录桶容量；dev 为 `20` |
| `rate_limit.global_burst` | `100` | 每 IP 全局桶容量 |
| `rate_limit.global_rps` | `20` | 每 IP 全局平均速率 |

`user_paths` 默认值：

```text
/api/v1alpha1/inst/node/newToken
/api/v1alpha1/datasource/create
/api/v1alpha1/datatable/create
/api/v1alpha1/feature_datasource/create
```

## 4. `database` — 数据库

| 键 | 默认值 | 说明 |
| --- | --- | --- |
| `driver` | `sqlite` | `sqlite` 或 `mysql` |
| `dsn` | `privahub.db?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)` | 连接串；MySQL 使用 DSN 格式（含用户名、密码、库名、`parseTime=true`） |
| `max_open_conns` | `50` | 最大打开连接数 |
| `max_idle_conns` | `10` | 最大空闲连接数 |
| `migrate_dir` | 空 | 预留键；当前迁移由代码内置执行 |

SQLite 注意事项：

- 单写者模型；破窗重置、备份时先停服务。
- 建议保持 WAL 与 `busy_timeout`，共享目录（NFS 等）不受支持。
- 数据库文件及其 `-wal` / `-shm` 必须与静态加密密钥分别备份；没有密钥时加密列不可恢复。

MySQL 注意事项：

- 生产建议 `parseTime=true` 与合适的 `loc`。
- 迁移仍由服务启动时执行；升级前先备份。
- 多副本部署时只允许一个实例执行迁移，或先运行 `cmd/migrator`。

## 5. `crypto` — 静态加密

| 键 | 默认值 | 说明 |
| --- | --- | --- |
| `aes_key` | 空 | 旧版单密钥；作为版本 `v1` 注册。`privahub.yaml` 示例文件填入样例值，生产使用该样例会被拒绝 |
| `keys` | 空 | 版本到密钥的映射，例如 `{v1: "<old>", v2: "<new>"}`；>= 32 hex 字符或 >= 16 原始字节 |
| `active_version` | 空 | 新密文使用的版本；多个版本且没有 `v1` 时必须显式设置 |
| `keys_file` | 空 | YAML/JSON 映射文件；建议 0600/0400，组/其他可读会告警 |

密钥注入优先级（低到高）：

1. `crypto.keys`（配置文件）。
2. `crypto.keys_file` 文件。
3. `PRIVAHUB_CRYPTO_KEY_<VERSION>`（版本名转小写；首字符为字母或数字，后续允许字母、数字、`.`、`_`、`-`，最长 32 字符）。

约束与运维：

- 新密文格式为 `enc:<version>:...`，使用 SM4-GCM + HKDF-SM3。
- 旧版本密钥必须保留到所有存量行完成重加密；启动时 `EncryptSecretsAtRest` 会重加密非 active 版本的行。
- 丢失全部已配置版本会导致节点/机构 token、数据源凭据、私钥等列不可读。
- 轮换流程见 [key-rotation.md](security/runbooks/key-rotation.md)。

示例：

```yaml
crypto:
  keys:
    v1: "3f1c...旧密钥..."
    v2: "9ab0...新密钥..."
  active_version: v2
```

或使用 Secret 挂载：

```bash
PRIVAHUB_CRYPTO_KEYS_FILE=/run/secrets/privahub/crypto-keys.yaml
PRIVAHUB_CRYPTO_ACTIVE_VERSION=v2
```

## 6. `kuscia` — 控制平面连接

| 键 | 默认值 | 说明 |
| --- | --- | --- |
| `kubeconfig` | 空 | 预留键；当前客户端不使用 |
| `namespace` | `kuscia-system` | 本节点所在的 Kuscia Domain/Namespace，也是默认 `node_id` |
| `api_address` | `127.0.0.1` | KusciaAPI 地址 |
| `api_port` | `8083` | KusciaAPI gRPC 端口；节点注册与展示使用，实际 API 调用走 `http_port` |
| `http_port` | `8082` | KusciaAPI HTTP 端口（Privahub 实际调用） |
| `protocol` | `notls` | `notls`、`tls`、`mtls`；生产仅允许后两者 |
| `gateway` | 空 | 跨域网关地址（Envoy），EDGE 转发与节点路由使用 |
| `cert_file` | 空 | KusciaAPI 客户端证书（mTLS） |
| `key_file` | 空 | 客户端私钥 |
| `ca_file` | 空 | 服务端 CA |
| `token` | 空 | Token 头值或文件路径 |
| `insecure_skip_verify` | `false` | 仅 dev/test 允许 true |
| `timeout` | `30s` | KusciaAPI 请求超时 |
| `http_proxy` | 空 | 空=直连并忽略 `HTTP(S)_PROXY`；`env`=遵循环境代理；其他值=代理 URL |
| `certs_dir` | `./config/certs/` | 机构节点注册上传证书的存放目录 |
| `pool_file` | `./config/kuscia/nodes.json` | 运行时注册的 KusciaAPI 端点持久化文件 |
| `data_proxy` | `false` | 强制 DomainDataSource `access_directly=false`，经 DataProxy 访问数据 |
| `nodes` | 空 | 多节点列表；每项见下表 |

### 6.1 `kuscia.nodes[]`

| 键 | 默认值 | 说明 |
| --- | --- | --- |
| `domain_id` | 必填 | Kuscia Domain ID，同时用于环境变量覆盖键 |
| `mode` | 空 | `master`、`lite`、`autonomy` |
| `host` | 空 | KusciaAPI 主机 |
| `port` | 0 | gRPC 端口（信息性） |
| `http_port` | 0 | 该节点的 HTTP 端口；0 回落到 `kuscia.http_port` |
| `protocol` | 空 | 该节点协议；生产必须为 tls/mtls |
| `cert_file` / `key_file` / `ca_file` | 空 | 节点级客户端证书与 CA |
| `token` | 空 | 节点级 Token 或文件路径 |

节点环境变量示例：

```bash
PRIVAHUB_KUSCIA_NODES_KUSCIA_SYSTEM_HOST=10.0.0.10
PRIVAHUB_KUSCIA_NODES_KUSCIA_SYSTEM_HTTP_PORT=8082
PRIVAHUB_KUSCIA_NODES_ALICE_PROTOCOL=mtls
```

端口值必须是 1–65535，否则启动失败。

### 6.2 EDGE 网关

EDGE（`server.mode=lite/edge`）通过 `kuscia.gateway` 把用户写请求转发到中心。生产要求网关为 `https://` 或回环地址；明文远程网关会被拒绝。网关证书与 token 的轮换见 [key-rotation.md](security/runbooks/key-rotation.md) §3。

## 7. `observability` — 日志与指标

| 键 | 默认值 | 说明 |
| --- | --- | --- |
| `enable_metrics` | `true` | 是否注册 `/metrics` |
| `metrics_port` | `9091` | 预留键；当前 `/metrics` 在主端口 `http_port` 提供，不单独监听 9091 |
| `log_level` | `info` | `debug`、`info`、`warn`、`error` |
| `log_format` | `json` | `json` 或 `console`；生产建议 json |
| `metrics_access` | dev/test：`public`；其他：`restricted` | `public`、`restricted`、`auth`、`disabled` |
| `metrics_allowed_cidrs` | 空 | `restricted` 模式下允许的来源 CIDR（回环始终允许） |
| `metrics_token` | 空 | `restricted` 模式下 `Authorization: Bearer <token>` |

`metrics_access` 语义：

| 值 | 行为 |
| --- | --- |
| `public` | 不限制；生产会打印告警 |
| `restricted` | 仅回环、`metrics_allowed_cidrs` 或正确 Bearer token |
| `auth` | 需要有效登录会话 |
| `disabled` | 不注册 `/metrics` |

## 8. `auth` — 认证、会话与密码

| 键 | 默认值 | 说明 |
| --- | --- | --- |
| `jwt_secret` | dev：内置开发密钥；生产：空 | JWT 签名密钥；生产必填、>= 16 字符且不能是示例值 |
| `jwt_previous_secrets` | 空 | 轮换期间仍可验签的旧密钥列表；令牌头带 `kid` |
| `access_token_expiry` | `2h` | 访问令牌有效期 |
| `refresh_token_expiry` | `168h`（7 天） | 刷新令牌有效期，也是会话绝对上限 |
| `session_idle_timeout` | `24h` | 会话滑动空闲超时 |
| `enabled` | `true` | `false` 关闭认证并注入平台管理员；仅 dev/test |
| `max_attempts` | `5` | 同一账号 + 客户端 IP 的登录失败上限 |
| `lock_minutes` | `30` | 上述锁定时间 |
| `account_max_attempts` | `0` → `4 × max_attempts`（默认 20） | 账号在所有 IP 上的失败上限；防止单 IP 锁定管理员 |
| `reset_password_max_attempts` | `5` | 修改密码接口的失败上限 |
| `reset_password_lock_minutes` | `60` | 修改密码锁定时间 |
| `password_kdf_iterations` | `0` → `1000000`，最低 `200000` | SM3 KDF 工作因子；过低会被提升，过高有上限 |
| `admin_name` | `admin` | 初始管理员账号 |
| `admin_password` | 空 | 初始管理员密码；生产未设置时生成随机一次性密码 |
| `initial_password_file` | `./config/initial-admin-password` | 一次性密码文件（0600），首次登录强制改密 |
| `cookie_secure` | 空（自动） | 浏览器会话 Cookie 的 Secure 标志；自动=主端口 TLS/代理开启时 Secure |
| `data_resource_whitelist` | 空 | **已废弃**；非空值会导致启动失败 |

密码与会话要点：

- 密码使用 `sm3kdf:v1` 存储；旧哈希在迁移时自动包裹。
- 修改密码、管理员重置、破窗重置都会提升密码版本并撤销全部会话。
- 刷新令牌一次性使用；检测到重放会撤销会话。
- 初始管理员密码只在首次启动生成一次；文件已存在时不会覆盖。

## 9. `data` — 数据管理

| 键 | 默认值 | 说明 |
| --- | --- | --- |
| `dir` | 空 → `$PRIVAHUB_DATA_DIR` → `./data` | 上传 CSV 根目录；实际路径为 `${dir}/<nodeId>/` |
| `ip_block.enable` | `true` | 对用户提供的 OSS/ODPS/MySQL/HTTP 端点启用 SSRF 黑名单 |
| `ip_block.list` | 内置列表 | 自定义 CIDR 列表；为空使用默认列表 |
| `tee.domain_id` | `tee` | TEE 节点 Domain ID |
| `tee.capsule_manager` | `capsule-manager.#.svc` | TEE Capsule Manager 服务名模板；`#` 替换为 TEE 域 |
| `tee.max_parallelism` | 0 → `1` | TEE 图任务默认并行度 |
| `tee.cert_dir` | `./config/certs` | TEE 图任务签名证书目录 |
| `cloud_log.sls.ak` | 空 | SLS AccessKey |
| `cloud_log.sls.sk` | 空 | SLS SecretKey |
| `cloud_log.sls.host` | 空 | SLS 端点 |
| `cloud_log.sls.project` | 空 | SLS Project |

SSRF 规则：

- `ip_block.enable=true` 时，默认阻断回环、私网、链路本地、云元数据、CGNAT、组播、保留地址等。
- 即使自定义 `list`，未指定地址、链路本地/元数据、组播与广播地址仍始终阻断。
- DNS 解析在连接时进行，防止 DNS rebinding。
- dev Profile 将 `ip_block.enable` 设为 `false`，便于访问本机 MinIO/MySQL。

SLS 四项全部配置后才启用云端日志；缺一项视为未配置。

## 10. `sync` — 数据同步与 EDGE/P2P

`sync` 的大部分键面向 EDGE/AUTONOMY 与 CENTER 之间的数据同步。为空时按平台类型使用内置默认列表。

| 键 | 默认值 | 说明 |
| --- | --- | --- |
| `tables` | 空 | 同步表名单；空=平台默认 |
| `center` | 空 → `true` | CENTER 生产者开关 |
| `p2p` | 空 → `true` | AUTONOMY 生产者开关 |
| `edge_enabled` | 空 → `true` | EDGE SSE 客户端开关 |
| `edge_reconnect_interval` | `5s` | EDGE 重连间隔 |
| `edge_idle_timeout` | `60s` | 超过该时间未收到任何消息（含 ping）则断开 SSE |
| `dispatch_interval` | `3s` | 变更派发周期 |
| `ping_interval` | `15s` | SSE 心跳周期 |
| `max_retries` | `3` | 失败变更重试上限 |
| `p2p_queue_dir` | `./config/sync/` | P2P 待发送变更的持久化目录 |
| `http_timeout` | `10s` | 节点间同步 HTTP 超时 |
| `edge_forward` | 内置列表 | EDGE 代理到中心的写端点 |
| `edge_include` | 内置列表 | EDGE 本地服务的 `/api/v1alpha1` 端点；其他路径返回 404 |
| `edge_include_extra` | 内置列表 | 追加到 `edge_include` 的 Go 别名（登录/登出/current/env/healthz） |
| `disabled_routes` | 空 | 返回 404 的路径列表；默认不启用 |
| `disable_java_p2p_routes` | `false` | AUTONOMY 下应用 Java P2P 禁用列表 |
| `inst_name` | 空 → `DEFAULT_INST` | AUTONOMY 初始化机构名 |
| `tee.enabled` | 空 → `true` | CENTER TEE 节点初始化开关 |
| `tee.address` | 空 | TEE 节点 KusciaAPI 地址 |
| `tee.port` | 0 → `kuscia.http_port` | TEE 节点 HTTP 端口 |
| `tee.protocol` | 空 | TEE 节点协议 |
| `tee.cert_dir` | `config/certs/tee` | TEE 节点证书目录 |

EDGE 注意：`privahub-edge.yaml` 中的 `${NODE_ID}`、`${KUSCIA_API_ADDRESS:...}` 等是**字面量**，Viper 不会做 shell 展开。必须用环境变量覆盖实际键，例如：

```bash
PRIVAHUB_KUSCIA_NAMESPACE=edge-node-1
PRIVAHUB_KUSCIA_API_ADDRESS=10.0.0.21
PRIVAHUB_KUSCIA_NODES_EDGE_NODE_1_HOST=10.0.0.21
PRIVAHUB_KUSCIA_NODES_EDGE_NODE_1_PROTOCOL=tls
```

`JWT_SECRET` 也不会展开；请直接设置 `PRIVAHUB_AUTH_JWT_SECRET`，否则 `${JWT_SECRET:change-me-in-production}` 会命中示例密钥列表并被生产校验拒绝。

## 11. `audit` — 防篡改审计

| 键 | 默认值 | 说明 |
| --- | --- | --- |
| `queue_size` | `4096` | 内存事件队列容量 |
| `spool_dir` | `./data/audit/spool` | 数据库写失败时的 fsync 本地暂存目录；启动/周期性重放 |
| `spool_max_bytes` | `268435456`（256 MiB） | 暂存上限；超出丢弃并计数 |
| `retention_days` | `0` | `0`=永久；生产最小 `1095`（3 年） |
| `archive_dir` | `./data/audit/archive` | 过期记录导出为哈希链段后删除 |
| `retention_interval` | `24h` | 归档周期 |
| `anchor.file` | 空 | 追加写 JSON Lines 的外部锚定文件，建议 WORM/另一主机 |
| `anchor.url` | 空 | 接收 `{time,seq,hash,node,algorithm}` POST 的服务 |
| `anchor.token` | 空 | 发送给 `anchor.url` 的 Bearer token |
| `anchor.interval` | `1h` | 锚定周期 |

生产建议至少配置 `anchor.file` 或 `anchor.url`；否则拥有数据库写权限的攻击者可以重写整条链而不被发现。审计链校验使用 `cmd/audit-verify`，见 [incident-response.md](security/runbooks/incident-response.md)。

## 12. `graph.yaml` — 图执行与组件

`config/graph.yaml` 不由 `pkg/config` 加载，而是由 `internal/graphjob` 读取；路径可用 `PRIVAHUB_GRAPH_CONFIG` 覆盖。文件缺失或解析失败时**静默回退到内置默认值**，不会阻止启动。

| 键 | 默认值 | 说明 |
| --- | --- | --- |
| `graph.app_image` | `secretflow-image` | SecretFlow 任务的 Kuscia AppImage |
| `graph.scql_image` | `scql-image` | SCQL 任务 AppImage |
| `graph.serving_image` | `sf-serving-image` | Serving 发布 AppImage |
| `graph.max_parallelism` | `1` | 图未指定时的默认并行度 |
| `graph.health_check` | `true` | 运行前检查节点与路由就绪 |
| `graph.devices[]` | SPU + HEU | SecretFlow 集群设备列表，含 `name`、`type`、`config` |
| `graph.cross_silo_comm_backend` | `brpc_link` | 跨域通信后端 |
| `graph.component_dir` | `config/components` | 每应用一份 `CompListDef` JSON |
| `graph.i18n_dir` | `config/i18n` | 组件翻译文件 |
| `graph.legacy_component_file` | `config/components.json` | 旧版扁平组件文件的回退 |
| `graph.hide` | 8 个 IO/模型组件 | 在 UI 中隐藏的 `{app}/{domain}/{name}:{version}` |
| `graph.deploy_mode` | `ALL-IN-ONE` | 与 `server.deploy_mode` 配合决定 `version/list` |
| `graph.versions` | 见文件 | 各镜像版本键值；键名保持 camelCase |

修改 `graph.yaml` 后需要重启服务；它不参与 `make openapi`，也不进入配置校验。组件定义变更后按 [modeling_user_guide.md](modeling_user_guide.md) 的流程重新验证模板。

## 13. 仓库内 Profile 对照

| 项目 | 基础 `privahub.yaml` | `privahub-dev.yaml` | `privahub-edge.yaml` |
| --- | --- | --- | --- |
| 语义 | 未设置 Profile 时按生产校验 | dev | edge |
| `server.mode` | master | master | lite |
| 数据库 | SQLite `privahub.db` | SQLite `data/privahub-dev.db` | SQLite `privahub-edge.db` |
| Kuscia 地址 | 127.0.0.1:8083/8082 | 127.0.0.1:18083/18082，网关 18080 | 字面占位符，需环境变量覆盖 |
| `kuscia.protocol` | notls（生产拒绝） | notls | tls |
| 日志 | info / console | debug / console | info / json |
| JWT | 示例密钥（生产拒绝） | dev 密钥 | 字面占位符（生产拒绝） |
| API 文档 | 默认关闭 | 启用 | 默认关闭 |
| IP 黑名单 | 默认启用 | 关闭 | 默认启用 |
| 同步 | 平台默认 | 平台默认 | `edge_enabled=true`，5s 重连 |

`PRIVAHUB_PROFILE=test` 没有对应的 YAML 文件，使用基础配置 + dev 语义；测试代码会进一步改写数据库与路径。

## 14. 生产配置示例

推荐把非敏感结构放在 ConfigMap/配置文件中，把密钥全部通过 Secret 注入环境变量。

```yaml
server:
  mode: master
  http_port: 8080
  inner_bind_address: 0.0.0.0
  behind_tls_proxy: false
  tls:
    enabled: true
    cert_file: /etc/privahub/tls/server.crt
    key_file: /etc/privahub/tls/server.key
    min_version: "1.2"
  trusted_proxies: []
  api_docs:
    enabled: false
  inner_auth:
    token_file: /run/secrets/privahub/inner-token

database:
  driver: mysql
  dsn: "privahub:${DB_PASSWORD}@tcp(mysql:3306)/privahub?parseTime=true&charset=utf8mb4"
  max_open_conns: 50
  max_idle_conns: 10

crypto:
  keys_file: /run/secrets/privahub/crypto-keys.yaml
  active_version: v2

kuscia:
  namespace: kuscia-system
  api_address: kuscia-master
  http_port: 8082
  protocol: mtls
  cert_file: /run/secrets/privahub/kuscia-client.crt
  key_file: /run/secrets/privahub/kuscia-client.key
  ca_file: /run/secrets/privahub/kuscia-ca.crt
  gateway: kuscia-gateway:1080

observability:
  log_level: info
  log_format: json
  metrics_access: restricted
  metrics_allowed_cidrs: ["10.0.0.0/8"]

auth:
  access_token_expiry: 2h
  refresh_token_expiry: 168h
  session_idle_timeout: 24h
  max_attempts: 5
  lock_minutes: 30
  admin_name: admin
  initial_password_file: /var/lib/privahub/initial-admin-password

data:
  dir: /app/data
  ip_block:
    enable: true

audit:
  retention_days: 1095
  archive_dir: /app/audit/archive
  anchor:
    file: /app/audit/anchor.jsonl
    interval: 1h
```

配套环境变量（示例，值由 Secret 提供）：

```bash
PRIVAHUB_AUTH_JWT_SECRET=<openssl rand -hex 32>
PRIVAHUB_CRYPTO_KEYS_FILE=/run/secrets/privahub/crypto-keys.yaml
PRIVAHUB_CRYPTO_ACTIVE_VERSION=v2
PRIVAHUB_SERVER_INNER_AUTH_TOKEN=<openssl rand -hex 32>
PRIVAHUB_DATABASE_DSN='privahub:<password>@tcp(mysql:3306)/privahub?parseTime=true&charset=utf8mb4'
```

上例中的 `${DB_PASSWORD}` 写法仅为可读性；Viper 不做变量展开，实际部署应通过环境变量覆盖 `PRIVAHUB_DATABASE_DSN` 或由 Secret 模板生成完整 DSN。

启动前自检：

```bash
# dev 语义快速验证配置结构（不要用于生产）
PRIVAHUB_PROFILE=dev ./bin/privahub -config ./config/privahub.yaml

# 生产语义：缺少必填项会立即退出并打印具体键名
./bin/privahub -config /etc/privahub/privahub.yaml
```

## 15. 常见配置问题

| 现象 | 原因与处理 |
| --- | --- |
| `auth.jwt_secret is not configured` | 生产未设置 `PRIVAHUB_AUTH_JWT_SECRET` |
| `auth.jwt_secret is a well-known sample` | 使用了 dev 密钥或 `change-me-in-production` |
| `crypto.aes_key is the sample key` | 仍在使用 `12345678901234567890123456789012` |
| `no at-rest encryption key` | 未配置 `crypto.keys` / `aes_key` / `keys_file` / 环境变量 |
| `the main port serves plain HTTP` | 未启用 TLS，也未声明 `behind_tls_proxy` + `trusted_proxies` |
| `server.inner_auth is required in production` | 内部端口缺少 token/CIDR/mTLS |
| `server.inner_bind_address=... exposes the inner port` | 非回环绑定但没有任何 `inner_auth` |
| `kuscia.protocol="notls" is not allowed` | 生产必须 tls/mtls |
| `audit.retention_days=365 is below ...` | 生产只能 0 或 >= 1095 |
| `crypto.keys_file ... has permissions 0644` | 密钥文件权限过宽，改为 0600/0400 |
| `PRIVAHUB_KUSCIA_NODES_X_PORT` 报端口错误 | 环境变量值不是 1–65535 |
| Edge 启动后仍连 127.0.0.1 | `${...}` 未被展开；用 `PRIVAHUB_*` 覆盖实际键 |
| `/metrics` 404 | `observability.enable_metrics=false` 或 `metrics_access=disabled` |
| `/api/v1alpha1/docs` 404 | `server.api_docs.enabled=false`（生产默认） |
| `/api/v1alpha1/docs` 401 | `server.api_docs.public=false`，需要登录会话 |

配置文件语法错误会由 Viper 返回 `failed to read config file`；环境变量拼写错误通常不会报错，而是静默使用默认值。排查时用启动日志中的 `profile`、`mode`、端口与显式配置值交叉确认。

## 16. 修改配置项的流程

1. 在 `pkg/config` 对应结构体添加字段与 `mapstructure` tag。
2. 在 `setDefaults` 设置默认值；标量字段会被自动绑定到 `PRIVAHUB_*`。
3. 在 `Validate` 或子校验函数中定义接受范围；安全相关默认拒绝。
4. 非致命问题加入 `SecurityWarnings` / `AuditWarnings`。
5. 补充 `pkg/config/*_test.go`，覆盖默认值、环境变量覆盖与拒绝分支。
6. 更新本文、[ops.md](ops.md)、[development.md](development.md) 与加固指南（涉及安全时）。
7. 如果配置影响部署清单，同步更新 `deployments/` 与 `make k8s-check` 相关文件。

## 17. 相关文档

- [development.md](development.md)：代码分层与新增配置项的开发流程。
- [ops.md](ops.md)：部署、备份、升级与排障。
- [security/hardening-guide.md](security/hardening-guide.md)：生产安全基线。
- [security/runbooks/key-rotation.md](security/runbooks/key-rotation.md)：JWT 与静态加密密钥轮换。
- [openapi-sync.md](openapi-sync.md)：`server.api_docs` 与契约同步。
