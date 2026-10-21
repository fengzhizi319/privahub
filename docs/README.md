# Privahub 文档中心

Privahub 是 SecretPad 后端的 Go 语言重构实现（Gin + GORM），配套 React 前端 `web/`，通过 REST API 对接 Kuscia 控制平面并调度 SecretFlow 执行隐私计算任务。本目录汇集产品、架构、API、运维、测试、安全、用户与迁移验收文档，是 `privahub/docs` 的唯一入口。

## 1. 文档概况

| 项目 | 说明 |
| --- | --- |
| 文档数量 | 22 份 Markdown：15 份主文档、3 份安全文档、4 份安全运行手册 |
| 图片资产 | 157 张 PNG、5 份截图清单 `manifest.json`（user-guide、modeling、visual-system、project-browser、canvas-isolation） |
| 实测数据 | `testing/modeling-latest.json`、`testing/modeling-run-2026-10-15.json` |
| 文档语言 | 产品、用户、运维、测试与迁移文档为中文；威胁模型、加固指南与运行手册为英文 |
| 事实口径 | 代码行为以仓库为准；验收结论以可复现脚本和报告为准；截图与实测数据必须标注环境、日期与边界 |

### 1.1 状态标记

| 状态 | 含义 |
| --- | --- |
| 现行 | 与当前代码或流程保持一致，可直接作为操作依据 |
| 基线 | 描述总体方案，少量统计数字可能滞后；关键事实需与代码交叉核对 |
| 历史 | 记录评估过程或旧结论，不构成当前验收依据 |
| 证据 | 由脚本生成的截图、清单或运行数据，不手工编辑 |

## 2. 文档类型

| 类型 | 回答的问题 | 主要读者 | 更新触发 |
| --- | --- | --- | --- |
| 产品需求（PRD） | 产品为谁解决什么问题，功能优先级与约束是什么 | 产品、前后端、测试 | 需求范围、优先级或里程碑变化 |
| 架构设计 | 系统如何分层，模块如何协作，关键设计取舍是什么 | 后端、前端、运维 | 分层、依赖、数据模型或集成方式变化 |
| API 参考 | 有哪些接口，请求/响应、认证与错误码是什么 | 前端、后端、集成方 | 路由、DTO 或错误码变化 |
| 用户手册 | 用户如何启动、配置并完成一次真实业务操作 | 平台使用者、实施、测试 | 页面、流程、按钮或截图变化 |
| 建模实测指南 | 哪些算法已真实跑通，如何复现与判断结果 | 算法、测试、交付 | 模板、算法参数或实测结果变化 |
| 操作示例 | 如何用 curl 快速调用 API 完成一条业务链路 | 后端、集成方、排障人员 | 接口契约或示例流程变化 |
| 运维部署 | 如何构建、配置、部署、监控、备份和升级 | 运维、SRE、实施 | 配置项、部署方式或运行时依赖变化 |
| 开发指南 | 如何在本地开发、分层扩展接口并完成提交前验证 | 后端、前端、测试 | 分层约定、中间件、测试或构建流程变化 |
| 配置参考 | 每个配置键的默认值、Profile 差异、校验规则与安全影响是什么 | 后端、运维、安全 | 配置键、默认值、校验或 Profile 行为变化 |
| 契约同步说明 | OpenAPI 文档如何生成、校验，前后端契约如何同步 | 后端、前端、测试、安全评审 | 路由、DTO、生成器、漂移门禁或前端基线变化 |
| 测试指南 | 测试如何组织、运行和扩展 | 开发者、测试 | 测试框架、用例或 CI 流程变化 |
| 迁移评估与验收 | 与 Java 版差距在哪，哪些业务已有自动化证据 | 迁移负责人、测试、交付 | 迁移范围、契约或验收证据变化 |
| 安全设计与合规 | 信任边界、威胁、控制措施和残余风险是什么 | 安全、运维、架构 | 新监听端口、新外部集成或新凭据类型 |
| 安全运行手册（Runbook） | 密钥泄露、轮换、事件响应和破窗重置如何操作 | 运维、安全、值班人员 | 安全流程、工具或配置变化 |
| 体验优化说明 | 前端列表、筛选、详情和视觉系统做了哪些调整 | 前端、产品、测试 | 交互、信息架构或视觉规范变化 |
| 实测证据与截图资产 | 某次验证看到了什么，如何校验其真实性 | 测试、审计、交付 | 由脚本重新生成，不手工修改 |

## 3. 快速导航

| 我要做什么 | 建议阅读顺序 |
| --- | --- |
| 第一次使用平台 | [user_guide.md](user_guide.md) → [modeling_user_guide.md](modeling_user_guide.md) |
| 了解产品范围 | [prd.md](prd.md) → [design.md](design.md) |
| 搭建本地开发环境 | [development.md](development.md) §1–2 → [testing.md](testing.md) |
| 开发或联调后端 | [development.md](development.md) → [design.md](design.md) → [api.md](api.md) → [testing.md](testing.md) |
| 新增接口或同步契约 | [development.md](development.md) §3 → [openapi-sync.md](openapi-sync.md) → [api.md](api.md) |
| 查配置项或生产配置 | [configuration.md](configuration.md) → [ops.md](ops.md) → [security/hardening-guide.md](security/hardening-guide.md) |
| 联调前端 | [api.md](api.md) → [display-optimization.md](display-optimization.md) → [user_guide.md](user_guide.md) |
| 部署到生产 | [ops.md](ops.md) → [security/hardening-guide.md](security/hardening-guide.md) |
| 处理安全事件 | [security/threat-model.md](security/threat-model.md) → [security/runbooks/incident-response.md](security/runbooks/incident-response.md) |
| 做迁移验收 | [migration-acceptance.md](migration-acceptance.md) → [migration-gap-assessment.md](migration-gap-assessment.md) → [testing.md](testing.md) |
| 复现手册截图或实测 | [user_guide.md](user_guide.md) §2 → [modeling_user_guide.md](modeling_user_guide.md) §5 |

## 4. 文档总览

### 4.1 产品与需求

#### [prd.md](prd.md) — 产品需求文档

**类型**：产品需求 ｜ **读者**：产品、前后端、测试 ｜ **状态**：现行基线 ｜ **文件更新**：2026-07-30

内容：

- 产品定位：SecretPad 后端的 Go 重构实现，为前端提供 RESTful API，并通过 Kuscia 执行隐私计算任务。
- 目标用户与场景：平台管理员、数据科学家、数据提供方、运维人员。
- 八组核心功能需求及优先级：用户与认证、节点管理、数据管理、项目管理、训练图/DAG、任务管理、模型服务、平台管理。
- 非功能需求：API 延迟、并发、内存、启动时间、可用性、安全与兼容性目标。
- 技术约束：Go、Gin、GORM、Viper、Zap、手动依赖注入、CGO 与单二进制部署。
- 里程碑 M1–M8、与 Java Spring Boot 版本的实现对比、主要风险与缓解措施。
- 阅读提示：里程碑完成状态是编写时快照，当前完成度以代码、测试和 [迁移验收](migration-acceptance.md) 为准。

### 4.2 架构与设计

#### [design.md](design.md) — 架构设计文档

**类型**：架构设计 ｜ **读者**：后端、前端、运维 ｜ **状态**：现行基线 ｜ **文件更新**：2026-07-30

内容：

- 系统分层：Client → Gin HTTP → Service → DAO / Kuscia Client / 外部服务。
- 目录结构与包职责：`cmd/`、`internal/controller/http`、`internal/service`、`internal/dao`、`pkg/`、`config/`。
- 手动依赖注入容器 `wire.App`、统一响应格式、中间件链与优雅降级策略。
- 核心数据模型与种子数据：默认机构、节点、管理员用户与 RBAC 角色。
- 端口规划、多 Profile 配置、环境变量覆盖规则。
- 安全设计（JWT、RBAC、限流、审计、SSRF 与路径遍历防护）与可观测性（日志、Prometheus、健康检查）。
- 时效说明：文中 143 路由、22 Service、17 Handler 为迁移期统计；当前主监听器注册 156 条路由（其中 155 条 `/api`），内部监听器 5 条（其中 3 条 `/api`），OpenAPI 去重后为 157 个 operation，另有 27 个 Service 结构、19 个 Handler。结构描述仍适用，具体路由以 [router.go](../internal/controller/http/router.go) 与 [openapi.json](../internal/controller/http/apidocs/openapi.json) 为准。

### 4.3 API 参考

#### [api.md](api.md) — API 接口文档

**类型**：API 参考 ｜ **读者**：前端、后端、集成方 ｜ **状态**：现行（契约以代码为准） ｜ **文件更新**：2026-07-30

内容：

- Base URL、JWT Bearer 认证、统一响应 envelope 与错误码。
- 认证、项目、DAG、任务、节点、数据表、数据源、模型、模型服务、投票、用户、定时任务、组件、机构、审批、消息、数据上传下载、节点路由等模块的接口清单与示例。
- 内部 9001 端口接口：投票同步、节点用户密码重置、数据同步、SSE 与健康检查。
- 公共接口：`/healthz`、`/env`、`/metrics`。
- 阅读提示：本文按模块摘录，完整路由以 [router.go](../internal/controller/http/router.go) 与生成文档 [internal/controller/http/apidocs/openapi.json](../internal/controller/http/apidocs/openapi.json) 为准（在线地址 `GET /api/v1alpha1/docs`）；前端 codegen 使用的 [web/openapi/secretpad.openapi.json](../web/openapi/secretpad.openapi.json) 是 Java 历史基线，不是 Go 路由产物。新增或修改接口时必须同步更新本文件，并按 [OpenAPI 契约同步说明](openapi-sync.md) 重新生成文档。

#### [example.md](example.md) — 使用示例

**类型**：操作示例 ｜ **读者**：后端、集成方、排障人员 ｜ **状态**：参考示例 ｜ **文件更新**：2026-07-30

内容：

- 构建与启动、健康检查与 Token 获取。
- 节点注册与路由、数据源注册、数据表注册与查询。
- 项目创建、参与节点与数据表绑定。
- DAG 创建、图结构更新、启动/停止、节点状态与日志。
- 任务查询、组件查询、用户创建与密码重置。
- 完整工作流 shell 脚本，以及未认证、参数错误、资源不存在三类错误示例。
- 内部端口调用示例。
- 阅读提示：示例用于快速串联流程；字段命名和路径若与当前实现不一致，以 [api.md](api.md) 与路由注册为准。

### 4.4 用户手册与建模指南

#### [user_guide.md](user_guide.md) — Privahub 使用手册

**类型**：用户手册 ｜ **读者**：平台使用者、实施、测试 ｜ **状态**：现行 ｜ **证据日期**：2026-10-15

内容：

- 阅读路线、截图环境与验证边界；区分 CENTER、EDGE、AUTONOMY 部署上下文和 ADMIN、AUDITOR 服务端角色。
- 开发栈启动、健康检查、日志、重启与关闭；Kuscia master/alice/bob 端口映射。
- 登录、首次改密、总览、工作台、新手引导与隐私场景入口。
- 节点、节点视角、我的节点、节点路由与双向往路验证。
- 数据源、数据表、字段 Schema、数据授权、特征数据源。
- 项目、图管理、DAG 编排、组件配置、真实两方 PSI 运行与结果验证。
- 运行记录、周期任务、结果管理、模型打包与 Serving 发布。
- 消息与审批、用户中心、权限边界、云端日志、组件版本与安全审计。
- P2P/AUTONOMY 与 EDGE 模式、多语言、主题和移动端操作。
- 按顺序完成一次真实计算的检查表、常见问题与 2026-10-15 验证记录。
- 附录：自定义/预定义画布、项目分类与筛选、主页菜单布局、统一列表与详情。
- 全文引用 60 张真实 Chromium 截图，路由、SHA256 和平台信息见 [截图清单](images/user-guide/manifest.json)；复现命令见 §2.2。
- 边界：截图数据为开发演示数据；SLS、OSS/ODPS/MySQL、TEE、在线特征服务和真实 Serving 推理仍需各自部署与凭据，页面存在不代表外部链路已验收。

#### [modeling_user_guide.md](modeling_user_guide.md) — 联合建模预定义画布与全流程实测

**类型**：建模实测指南 ｜ **读者**：算法、测试、交付 ｜ **状态**：现行 ｜ **实测日期**：2026-10-15

内容：

- 自定义画布与预定义画布的独立入口、模板预览和副本隔离。
- 通过项目向导或 DAG 模板向导选择算法，配置双方数据表、关联键、特征列、标签列与预测列。
- 八类纵向安全联合建模算法的实测状态、耗时、测试样本、AUC、任务 ID 与截图：SS-SGD、SSGLM、SecureBoost、SS-XGB、高斯朴素贝叶斯、KNN、KMeans、高斯过程分类。
- 12 项本地隐私用例：分类分级、脱敏、K-匿名、L-多样性、本地差分隐私、DP 求和/计数/均值/直方图、单条与批量查询混淆。
- 模板向导字段加载、标签泄漏、长表单滚动、E2E 状态判断等缺陷的修复记录。
- GPC 在 4 GiB Lite 节点上 OOM 的失败样本、默认迭代调整和重跑结果。
- 复现命令 `scripts/modeling-test.sh`、证据清单和适用范围；不将纵向建模结果表述为已实现横向联邦学习。
- 截图与 SHA256 见 [建模截图清单](images/modeling/manifest.json)，运行数据见 [实测记录](testing/modeling-latest.json)。

### 4.5 运维与部署

#### [ops.md](ops.md) — 运维部署文档

**类型**：运维部署 ｜ **读者**：运维、SRE、实施 ｜ **状态**：现行 ｜ **文件更新**：2026-09-24

内容：

- 系统与硬件要求；本地构建、交叉编译与 Docker 构建。
- 配置文件（`privahub.yaml`、`privahub-dev.yaml`、`privahub-edge.yaml`、`components.json`）与主要配置项。
- 环境变量覆盖规则、Profile 选择与生产启动的 fail-closed 行为。
- 单机、Docker、Docker Compose、Kubernetes 四种部署方式。
- 健康检查、Prometheus 指标访问控制、日志格式与级别。
- SQLite/MySQL 备份恢复、常见故障排查、调试与性能分析。
- 生产安全加固清单、Nginx 反向代理示例、滚动升级与数据库迁移。
- 阅读提示：本文覆盖基础运维；生产上线前应逐项执行 [安全加固指南](security/hardening-guide.md) 的检查表。

### 4.6 测试与验收

#### [testing.md](testing.md) — 测试指南

**类型**：测试指南 ｜ **读者**：开发者、测试 ｜ **状态**：现行 ｜ **文件更新**：2026-10-15

内容：

- 全量、指定包、指定函数和覆盖率测试命令。
- 测试文件分布：Service、中间件、Kuscia Client。
- 测试模式：SQLite 内存数据库、`httptest` HTTP 测试、`httptest.Server` 模拟 Kuscia。
- 按 Service 分组的用例清单：Auth、Project、Graph、Job、Node、Datatable、Vote、User、FeatureTable、GraphDatasource、EdgeDataSync、PartitionRule、OSS、SSE、Env、DataDirectory 等。
- 测试命名、表驱动写法、辅助函数与最佳实践。
- GitHub Actions 与本地 pre-commit 示例。
- 阅读提示：测试函数数量随代码变化，本文件不固化静态数字；迁移验收与真实计算验证见 [migration-acceptance.md](migration-acceptance.md)。

#### [migration-acceptance.md](migration-acceptance.md) — 业务迁移验收与系统测试

**类型**：迁移验收 ｜ **读者**：迁移负责人、测试、交付 ｜ **状态**：现行验收入口 ｜ **证据日期**：2026-10-15

内容：

- 独立 Java 契约基线：25 个控制器、121 个业务路由，保存于 `internal/controller/http/testdata/legacy_routes.json`。
- 路由覆盖测试的边界：只证明路由与监听器边界，不证明响应语义或业务闭环。
- 业务验证矩阵：登录/会话/权限、项目/画布、数据、组件、作业/结果、定时任务、模型、审批/P2P、TEE/机构/路由、前端。
- 一键门禁 `scripts/test_migration.sh` 与隔离系统测试 `scripts/run_system_e2e.py` 的运行方式、环境变量和产物管理。
- 本轮修复范围：模型归属、定时任务租户隔离、同步游标原子 upsert、前端响应兼容、控制台视觉与可访问性。
- 2026-10-15 实测结果与仍需外部环境验收的链路。
- 阅读提示：路由覆盖率、单元测试数量和页面截图都不能替代矩阵右列的外部链路验收。

#### [migration-gap-assessment.md](migration-gap-assessment.md) — 迁移版功能差距评估与补齐方案

**类型**：历史评估 ｜ **读者**：迁移负责人、后端、前端 ｜ **状态**：历史参考 ｜ **评估日期**：2026-07-28；补充复核 2026-10-15

内容：

- 迁移版与全功能版（Java + 旧前端）的对比目标和评估方法。
- 首版端点覆盖评估、修订版契约一致性评估、三次落地复核。
- snake_case ↔ camelCase 请求/响应不匹配、`model/serving/*` 路径别名、16 个缺失端点、列表包装结构、标量响应、严格数组校验、图节点字段类型、组件批量接口等缺陷的定位与修复。
- 全局 camelCase 兼容中间件、路径别名与 DTO 对齐方案。
- 2026-10-15 周期任务租户隔离复核：计划、执行实例、详情、停止与重跑的归属校验。
- 阅读提示：本文记录历史评估过程，其中“离线可验证工作已完成”等结论不代表全部业务迁移验收通过；当前结论以 [migration-acceptance.md](migration-acceptance.md) 为准。

#### [testing/modeling-latest.json](testing/modeling-latest.json) 与 [testing/modeling-run-2026-10-15.json](testing/modeling-run-2026-10-15.json) — 建模实测数据

**类型**：实测证据 ｜ **读者**：测试、算法、审计 ｜ **状态**：证据 ｜ **生成日期**：2026-10-15

内容：

- `modeling-latest.json`：最近一次联合建模与本地隐私用例的运行结果，供文档生成和复查使用。
- `modeling-run-2026-10-15.json`：带日期的运行快照，便于追溯当轮证据。
- 两个文件由 `scripts/modeling-test.sh` 及其辅助脚本生成，不手工编辑；字段和截图对应关系见 [modeling_user_guide.md](modeling_user_guide.md)。

### 4.7 安全与合规

#### [security/threat-model.md](security/threat-model.md) — Threat Model（STRIDE）

**类型**：安全设计 ｜ **读者**：安全、架构、运维 ｜ **状态**：现行 living document ｜ **最近复核**：2026-09

内容：

- 系统信任边界 TB1–TB8：浏览器、Kuscia 网关与内部端口、数据库、KusciaAPI、跨机构、外部数据源、TEE、审计锚定。
- 资产与参与者：凭据、会话、密钥、节点证书、数据源凭据、项目/作业元数据、审计证据。
- 按组件拆分的 STRIDE 威胁与控制：SPA、主 API、内部端口、Kuscia、EDGE/P2P 同步、数据源 SSRF、TEE、审计链、构建与供应链。
- 已知缺口与后续事项。
- 维护要求：新增监听端口、外部集成或凭据类型时，必须在同一变更中更新本文。

#### [security/hardening-guide.md](security/hardening-guide.md) — Production Hardening Guide

**类型**：安全加固检查表 ｜ **读者**：运维、安全、发布负责人 ｜ **状态**：现行 ｜ **文件更新**：2026-10-15

内容：

- fail-closed 启动原则与 Profile 风险：生产环境不得使用 `dev`/`test`/`local`。
- 进程与容器：非 root、只读根文件系统、capabilities、seccomp、可写卷与 Pod Security。
- 密钥注入：JWT、静态加密密钥、内部端口 token、指标 token、审计锚定 token、Kuscia 凭据、初始管理员密码。
- 传输安全：主端口 TLS / TLS 代理、可信代理、Kuscia TLS/mTLS、EDGE 网关。
- 内部端口：绑定地址、token/CIDR/mTLS 与禁止公网暴露。
- HTTP 表面：指标、API 文档、CORS、Body/Upload 限制、登录锁定、密码 KDF、会话时长、CSRF。
- 数据平面、审计保留与锚定、运维备份与运行手册。
- 开发门禁 `make check`、`make sec`、pre-commit 与 fail-closed 检查。

#### [security/authorization-review.md](security/authorization-review.md) — 认证与权限复核

**类型**：安全复核 ｜ **读者**：后端、安全、测试 ｜ **状态**：现行 ｜ **复核日期**：2026-10-15

内容：

- 当前系统没有入站 Scope Key 配置；Kuscia Token 是出站凭据，不能作为 Privahub API 的入站授权凭据。
- 已修复问题：角色缓存撤权延迟、授权版本、并发登录晚到会话、所有者事务原子性、同名人类账号与虚拟节点隔离、AUTONOMY 路径白名单、归档项目越权、冲突认证头、删除账号未清理会话。
- `authorization_version` 的数据库升级与兼容性；旧配置 `auth.data_resource_whitelist` 的迁移要求。
- 未来引入 Scope Key 时应独立设计签发上限、METHOD/path 契约、数据范围、受众、期限/吊销、摘要存储与轮转。
- 回归验证命令与跨数据库、撤权、并发登录等测试覆盖。

#### [security/runbooks/credential-leak.md](security/runbooks/credential-leak.md) — 凭据泄露处置

**类型**：安全运行手册 ｜ **读者**：运维、安全、值班人员 ｜ **状态**：现行 ｜ **文件更新**：2026-09-24

内容：

- 触发条件与“先轮换、后调查”原则；从 Git 历史删除密钥不等于修复。
- 按凭据类型分类影响范围与轮换方式：JWT、静态加密密钥、内部端口 token、节点/Kuscia 私钥、指标/锚定 token、用户密码、数据源凭据、CI/仓库 token。
- 遏制、调查、清理与复盘流程；审计链优先验真；公开泄露按已被利用处理。

#### [security/runbooks/key-rotation.md](security/runbooks/key-rotation.md) — 密钥轮换

**类型**：安全运行手册 ｜ **读者**：运维、安全 ｜ **状态**：现行 ｜ **文件更新**：2026-09-24

内容：

- 轮换周期与停机影响：JWT、静态加密密钥、内部 token/证书、指标/锚定 token、TLS 证书、Kuscia 证书。
- JWT 密钥的常规灰度轮换与泄露时的紧急轮换。
- 静态加密密钥的版本化 envelope、`crypto.keys` / `crypto.keys_file` / 环境变量注入、重加密与旧版本移除。
- 内部端口 token 与 mTLS 证书的双侧协调。
- 轮换后的冒烟验证、旧凭据失效确认与审计记录。

#### [security/runbooks/incident-response.md](security/runbooks/incident-response.md) — 安全事件响应

**类型**：安全运行手册 ｜ **读者**：安全、运维、管理层 ｜ **状态**：现行 ｜ **文件更新**：2026-09-24

内容：

- 事件分级（SEV1–SEV3）与角色分工：事件负责人、运维、调查、沟通。
- 检测与宣告、证据保全、隔离、凭据轮换、账户禁用与跨机构协调。
- 使用 `audit-verify` 先验证审计链完整性，再重建登录、权限、节点、路由、数据下载等时间线。
- 清除、恢复、通知（合同、PIPL/GDPR）与无责复盘要求。

#### [security/runbooks/break-glass-admin-reset.md](security/runbooks/break-glass-admin-reset.md) — 破窗管理员重置

**类型**：安全运行手册 ｜ **读者**：运维、安全、值班人员 ｜ **状态**：现行 ｜ **文件更新**：2026-09-24

内容：

- 适用场景、双人审批、备份与事件记录要求。
- 方案 A：由另一管理员通过重置密码 API 处理，强制首次改密并撤销会话。
- 方案 B：使用 `privahub-admin reset-password` 直接操作数据库，包含一次性密码生成、KDF 存储、会话撤销、审计事件与参数说明。
- Kubernetes Job 与 Docker Compose 两种执行方式、输出处理与密码不留存要求。
- 工具不可用时的降级流程与事后审计、JWT 轮换要求。

### 4.8 前端体验与视觉系统

#### [display-optimization.md](display-optimization.md) — 页面信息展示优化

**类型**：体验优化说明 ｜ **读者**：前端、产品、测试 ｜ **状态**：现行 ｜ **验证日期**：2026-10-15

内容：

- 主页导航分组、项目分组折叠与多字段搜索、三种项目视图及偏好记忆。
- 数据表、模型、结果、运行记录共用的 `TableView`：行高、列显隐、列宽、固定表头、当前页排序与恢复默认。
- `FilterBar` 与运行记录的服务端关键词/状态/时间筛选、分页总数一致性、通配符与注入字符串处理。
- 状态标签映射、行操作收纳、总览/工作台信息优先级。
- 项目、模型、结果统一详情抽屉与标签页；DAG 配置校验摘要与字段定位。
- 空状态、加载骨架、暗色主题对比度、窄屏表格内部滚动。
- 范围边界与验证脚本（`verify-visual-system.mjs`、`check-visual-system.py`）及截图清单。

#### [images/](images/) — 截图与清单资产

**类型**：实测证据 ｜ **读者**：测试、交付、审计 ｜ **状态**：证据

内容：

- [images/user-guide/](images/user-guide/)：60 张使用手册截图与 `manifest.json`，记录路由、平台、标题、按钮、SHA256、视口与真实 API 标记。
- [images/modeling/](images/modeling/)：八类建模与 12 项本地隐私用例的画布、产物、详情截图与清单。
- [images/visual-system/](images/visual-system/)：总览、工作台、数据表、运行记录、项目详情、DAG 配置、暗色与移动端截图。
- [images/project-browser/](images/project-browser/)：项目分组、紧凑列表、隐私场景筛选与移动端截图。
- [images/canvas-isolation/](images/canvas-isolation/)：预定义画布导航、单模板预览与算法画布截图。
- 所有截图由真实 Chromium 访问本机服务生成；清单中的 SHA256 用于校验文件未被替换。图片与清单由脚本重生成，不手工编辑。

### 4.9 开发、配置与契约同步

#### [development.md](development.md) — 开发指南

**类型**：开发指南 ｜ **读者**：后端、前端、测试 ｜ **状态**：现行 ｜ **核对日期**：2026-10-21

内容：

- 工具链要求、仓库目录职责、本地启动方式与 Profile/端口选择。
- `cmd/server` 启动顺序：配置加载 → 日志 → 密钥环 → 密码 KDF → 数据库 → 迁移 → 种子数据 → `wire.App` → 路由 → 路由覆盖校验 → 监听与优雅关闭。
- 分层规则：Handler 只做绑定与响应，业务在 Service，数据访问在 DAO；手动依赖注入 `wire.App` 的装配方式。
- 中间件顺序及其影响：TraceID、Recovery、可信代理、BodyLimit、会话、RBAC、数据权限、限流、camelCase。
- 新增一个 API 的完整步骤：先定契约、Service 请求/响应类型、Handler、`wire.App`、路由注册、RBAC 与数据权限分类、OpenAPI 重新生成、测试。
- 统一响应 envelope、camelCase ↔ snake_case 规则、错误码使用方式，以及 OpenAPI 生成器对 Handler 写法的约束。
- 认证、RBAC、数据归属校验与内部端口请求的虚拟用户语义。
- GORM 模型、迁移与种子数据约定；测试类型、脚手架、命令与编写要求。
- 代码风格、提交前检查清单、常见扩展场景（新增配置项、后台任务、命令行工具）与联调排障。
- 阅读提示：授权分类缺失会让服务启动失败；新增路由必须同时完成 §3 的检查项，不能只注册 Handler。

#### [configuration.md](configuration.md) — 配置参考

**类型**：配置参考 ｜ **读者**：后端、运维、安全 ｜ **状态**：现行 ｜ **核对日期**：2026-10-21

内容：

- 配置文件搜索顺序、默认文件位置、Profile 合并顺序与 `PRIVAHUB_*` 环境变量映射规则。
- Profile 与 dev 语义：空 Profile 视为生产；`dev` / `test` / `local` 之外全部按生产校验。
- 启动校验与 fail-closed 行为：JWT、AES 密钥、TLS / TLS 代理、Kuscia 传输、内部端口鉴权、指标访问、审计保留期。
- 全量配置键表：`server`（TLS、内部端口鉴权、限流）、`database`、`crypto`、`kuscia`（含 `nodes[]` 与 EDGE 网关）、`observability`、`auth`、`data`、`sync`、`audit`，以及独立的 `graph.yaml`。
- 默认值与动态默认值：KDF 迭代次数、账号锁定阈值、上传大小、CORS、指标访问模式、审计保留期等。
- 仓库内 Profile 对照、生产配置示例、常见配置错误与修改配置项的流程。
- 保留或未生效键的说明（如 `server.grpc_port`、`observability.metrics_port`、`kuscia.kubeconfig`），避免按名称误判行为。
- 阅读提示：EDGE Profile 中的 `${...}` 占位符是字面量，Viper 不做 shell 展开；生产必须通过环境变量或密钥文件注入真实值。

#### [openapi-sync.md](openapi-sync.md) — OpenAPI 契约同步说明

**类型**：契约同步指南与模板 ｜ **读者**：后端、前端、测试、安全评审 ｜ **状态**：现行 ｜ **核对日期**：2026-10-21

内容：

- 两份 OpenAPI 文档的区分：Go 生成的 `internal/controller/http/apidocs/openapi.json` 与前端 Java 历史基线 `web/openapi/secretpad.openapi.json`。
- 后端契约链：路由与 Handler 类型 → `cmd/openapi-gen` → 提交的 JSON → `//go:embed` → `GET /api/v1alpha1/docs`。
- 生成器派生规则：请求体、响应、schema、`required`、`security`、`x-handler`、`x-api-resource`、`x-inner-port` 等元数据来源。
- 生成器边界：不生成 path/query 参数、4xx/5xx 响应、示例与枚举；间接绑定/响应会丢失类型。
- 漂移门禁：`make openapi`、`make openapi-check`、CI `openapi` Job 与 `TestOpenAPIDocumentCoversRoutes` 的双向覆盖。
- 前端契约链：`pnpm codegen:openapi`、生成的 `secretpad.d.ts`、`api.ts` / `client.ts` 的 `RawJson` 断言与 Zod 运行时校验。
- 前端已知差距、切换到 Go 文档的建议步骤、新增/修改/删除接口的同步清单。
- 可直接复制的 PR 模板、评审责任、故障排查与命令速查。
- 阅读提示：生成文件不得手改；后端文档通过门禁不代表业务已验收，前端类型也不等于完整契约。

## 5. 常见任务与对应文档

| 任务 | 先读 | 再查 | 验证方式 |
| --- | --- | --- | --- |
| 启动本地开发栈 | [development.md](development.md) §1.3、[user_guide.md](user_guide.md) §2 | `scripts/dev-start.sh --help` | `bash scripts/dev-status.sh` |
| 新增或修改 API | [development.md](development.md) §3、[api.md](api.md) | [openapi-sync.md](openapi-sync.md) | `make openapi-check`、`go test ./internal/controller/http/...` |
| 修改或排查配置 | [configuration.md](configuration.md) | [ops.md](ops.md) §3 | 启动校验日志、[configuration.md](configuration.md) §15 |
| 同步 OpenAPI 或前端类型 | [openapi-sync.md](openapi-sync.md) | [development.md](development.md) §3.7 | `make openapi-check`、`cd web && corepack pnpm typecheck` |
| 排查任务失败 | [user_guide.md](user_guide.md) §9–10、§20 | [ops.md](ops.md) §7 | `bash scripts/dev-logs.sh backend -f` |
| 生产上线 | [ops.md](ops.md) | [hardening-guide.md](security/hardening-guide.md) | `make check`、加固检查表逐项确认 |
| 安全审计或事件响应 | [threat-model.md](security/threat-model.md) | [incident-response.md](security/runbooks/incident-response.md) | `audit-verify`、审计链校验 |
| 复现使用手册截图 | [user_guide.md](user_guide.md) §2.2 | `scripts/user-guide.sh --help` | `python3 scripts/check-user-guide.py` |
| 复现建模实测 | [modeling_user_guide.md](modeling_user_guide.md) §5 | `scripts/modeling-test.sh --help` | `python3 scripts/check-modeling.py` |
| 做迁移验收 | [migration-acceptance.md](migration-acceptance.md) | [testing.md](testing.md) | `cd privahub && bash scripts/test_migration.sh --kuscia` |
| 调整列表、筛选或详情 | [display-optimization.md](display-optimization.md) | [api.md](api.md) | 前端测试、`verify-visual-system.mjs` |

## 6. 文档维护约定

### 6.1 更新映射

| 变更内容 | 必须同步的文档 |
| --- | --- |
| 路由、请求/响应 DTO、错误码 | [api.md](api.md)、[design.md](design.md)、[openapi.json](../internal/controller/http/apidocs/openapi.json)、[openapi-sync.md](openapi-sync.md)；前端调用变化还需同步 `web/openapi/secretpad.openapi.json` 与 Zod schema |
| 配置项、默认值、Profile 行为 | [configuration.md](configuration.md)、[ops.md](ops.md)、[security/hardening-guide.md](security/hardening-guide.md) |
| 分层、中间件、测试脚手架 | [development.md](development.md)、[testing.md](testing.md) |
| OpenAPI 生成器、漂移门禁、前端 codegen | [openapi-sync.md](openapi-sync.md)、[openapi.json](../internal/controller/http/apidocs/openapi.json)、`web/packages/api-client/src/generated/secretpad.d.ts` |
| 认证、授权、审计、密钥或信任边界 | [security/authorization-review.md](security/authorization-review.md)、[threat-model.md](security/threat-model.md)、[hardening-guide.md](security/hardening-guide.md) |
| 页面流程、按钮、文案、截图 | [user_guide.md](user_guide.md)、[display-optimization.md](display-optimization.md)、`images/*/manifest.json` |
| 算法模板、组件参数、实测结果 | [modeling_user_guide.md](modeling_user_guide.md)、`testing/modeling-*.json` |
| 测试框架、用例、门禁 | [testing.md](testing.md)、[migration-acceptance.md](migration-acceptance.md) |
| 迁移范围与验收结论 | [migration-acceptance.md](migration-acceptance.md)；历史过程更新到 [migration-gap-assessment.md](migration-gap-assessment.md) |

### 6.2 事实与证据规则

1. 不把“路由存在”“Zod 校验通过”“页面能打开”写成“业务已验收”；验收结论必须对应可复现脚本、报告或截图。
2. 区分部署上下文（CENTER/EDGE/AUTONOMY）与服务端角色（ADMIN/AUDITOR），不以菜单可见代替后端授权校验。
3. 截图必须来自真实浏览器与真实接口，注明视口、平台、日期；开发演示数据不能表述为生产数据。
4. 外部依赖（SLS、OSS/ODPS/MySQL、TEE、Serving、在线特征服务）未部署时，明确写“未验收”，不以空页面或空结果推断可用。
5. 静态统计数字注明统计日期与命令；测试数量、路由数量变化后，优先修订索引与概览，不在多处复制易失数字。
6. 威胁模型、加固指南与运行手册按英文维护，具体复核结论与面向用户的文档按中文维护；新增文档必须在本索引登记类型、读者、状态与维护触发条件。

### 6.3 校验命令

在工作区根目录执行文档与实测校验：

```bash
# 使用手册：链接、PNG、SHA256、前端路由覆盖
python3 scripts/check-user-guide.py

# 建模手册：图片、清单与实测数据
python3 scripts/check-modeling.py

# 视觉系统：截图清单与 SHA256
python3 scripts/check-visual-system.py

# 真实两方 PSI 与建模实测（需要完整本地栈）
MODELING_PYTHON=/path/to/python bash scripts/modeling-test.sh --no-start
```

在 `privahub` 根目录执行迁移验收与基础回归：

```bash
# 迁移验收：Go 全量测试、race、vet、前端测试、隔离系统测试
bash scripts/test_migration.sh --legacy-source /path/to/secretpad

# 后端回归
CGO_ENABLED=1 go test ./... && CGO_ENABLED=1 go vet ./...

# 前端回归
cd web && corepack pnpm run typecheck && corepack pnpm run lint && corepack pnpm test
```

文档校验失败时不要以旧截图、旧报告或历史结论代替本轮结果；失败输出应保留在 `logs/` 或临时报告目录中，按测试数据管理。

## 7. 项目速览

### 7.1 技术栈

| 层 | 技术 |
| --- | --- |
| Web 框架 | Gin |
| ORM | GORM（SQLite / MySQL） |
| 配置 | Viper（YAML + `PRIVAHUB_*` 环境变量） |
| 日志 | Zap（console / json） |
| 定时任务 | robfig/cron |
| 监控 | Prometheus client |
| 认证 | JWT + HttpOnly 会话 Cookie + CSRF |
| 前端 | React 18、Vite 5、TypeScript、Tailwind CSS、Zustand、pnpm workspace |
| 编排与计算 | Kuscia、SecretFlow、DataMesh |

### 7.2 代码规模

统计日期 2026-10-21，命令见本节末尾。数字会随代码变化，引用时请注明日期。

| 指标 | 当前值 |
| --- | --- |
| 非测试 Go 文件 / 行数 | 215 / 51,331 |
| 全部 Go 文件 / 行数（含测试） | 360 / 76,614 |
| 主端口 API 路由 / 内部端口 API 路由 | 155 / 3（OpenAPI 去重后 157 个 operation） |
| Service 结构 / Handler 结构 | 27 / 19 |
| 测试函数 | 692 |
| 配置 Profile | 3（base / dev / edge） |

```bash
cd privahub
find . -name '*.go' -not -name '*_test.go' | wc -l
find . -name '*.go' -not -name '*_test.go' -print0 | xargs -0 cat | wc -l
python3 - <<'PY'
import json
d = json.load(open('internal/controller/http/apidocs/openapi.json'))
ops = [(m, p, o) for p, item in d['paths'].items() for m, o in item.items() if not m.startswith('x-')]
inner_only = [x for x in ops if x[2].get('x-inner-port') and not x[2].get('x-also-inner-port')]
inner = [x for x in ops if x[2].get('x-inner-port')]
print('operations', len(ops))
print('main api', len(ops) - len(inner_only))
print('inner api', len(inner))
PY
grep -rhE '^func Test' --include='*_test.go' . | wc -l
```

### 7.3 目录结构

```text
privahub/
├── cmd/
│   ├── server/          # 主服务
│   ├── edge-agent/      # 边缘代理
│   ├── migrator/        # 数据库迁移工具
│   ├── openapi-gen/     # OpenAPI 3 生成器
│   └── privahub-admin/  # 破窗管理员工具
├── internal/
│   ├── controller/http/ # 路由、19 个 Handler、中间件
│   ├── service/         # 27 个业务 Service
│   ├── dao/             # GORM 模型、仓储、迁移与种子数据
│   └── wire/            # 依赖注入容器
├── pkg/                 # 配置、Kuscia 客户端、认证、审计、加密、指标等
├── config/              # privahub.yaml / dev / edge
├── deployments/         # Docker、Kubernetes、Compose
├── scripts/             # 迁移、系统 E2E、文档与安全校验脚本
└── docs/                # 本目录
```

### 7.4 构建、运行与测试

```bash
# 构建（SQLite 驱动需要 CGO）
cd privahub
make build
# 或
mkdir -p .tmp
TMPDIR=$(pwd)/.tmp CGO_ENABLED=1 go build -o bin/privahub ./cmd/server

# 运行
./bin/privahub
PRIVAHUB_PROFILE=dev ./bin/privahub

# 测试
make test
CGO_ENABLED=1 go test ./...
CGO_ENABLED=1 go test -race ./internal/service/... ./internal/controller/http/...
```

### 7.5 配置与端口

完整配置键、默认值、校验规则与 Profile 差异见 [configuration.md](configuration.md)。

| Profile | 用途 |
| --- | --- |
| 未设置 | 生产语义，启动时校验密钥、内部鉴权与安全开关，不满足则拒绝启动 |
| `dev` | 本地开发，使用 Docker 映射的 Kuscia 端口与开发默认值 |
| `edge` | 边缘节点（lite 模式） |

主要环境变量：

| 变量 | 说明 |
| --- | --- |
| `PRIVAHUB_PROFILE` | 配置 Profile；生产不要设置为 dev/test/local |
| `PRIVAHUB_AUTH_JWT_SECRET` | JWT 密钥，生产必填且非示例值 |
| `PRIVAHUB_CRYPTO_AES_KEY` / `PRIVAHUB_CRYPTO_KEYS_FILE` | 静态加密密钥或密钥文件 |
| `PRIVAHUB_SERVER_HTTP_PORT` / `PRIVAHUB_SERVER_INNER_PORT` | 主 HTTP / 内部端口 |
| `PRIVAHUB_KUSCIA_API_ADDRESS` / `PRIVAHUB_KUSCIA_API_PORT` | KusciaAPI 地址与端口 |
| `PRIVAHUB_KUSCIA_HTTP_PORT` / `PRIVAHUB_KUSCIA_GATEWAY` | KusciaAPI HTTP 端口 / 网关地址 |
| `PRIVAHUB_DATA_DIR` | 上传数据落盘根目录 |

| 端口 | 用途 | 认证 |
| --- | --- | --- |
| 8080 | 主 HTTP 服务（含 `/metrics`） | JWT / 会话；指标由 `metrics_access` 控制 |
| 9001 | 内部端口（节点间通信） | 默认仅回环；生产需 `server.inner_auth` |

默认开发账号为 `admin / 12345678`，仅用于本地 `dev` 环境；生产首次启动使用生成的一次性管理员密码，并强制首次改密。

### 7.6 核心能力

- 认证与用户：登录、刷新、改密、用户管理、RBAC、会话撤销、审计。
- 节点与路由：Kuscia Domain、节点注册、Token、DomainRoute、合作节点。
- 数据：数据源、数据表、字段 Schema、授权、特征表、数据上传下载、数据目录。
- 项目与画布：项目、成员、DAG 图、组件配置、模板、图执行与状态同步。
- 任务与结果：任务列表/详情/停止、日志、输出、结果管理、周期任务、重跑。
- 模型与服务：模型打包、导出、Serving 发布与状态同步、TEE 拉取。
- 平台：机构、审批、消息、P2P/EDGE 同步、审计、组件版本、环境拓扑。

### 7.7 许可证

Apache License 2.0。

## 8. 文档缺口与补充建议

2026-10-21 已补充 [development.md](development.md)、[configuration.md](configuration.md) 与 [openapi-sync.md](openapi-sync.md)，原三项高优先级缺口关闭。以下类型目前仍没有独立文档，建议后续按优先级补充，并在本索引登记：

| 缺口 | 建议内容 | 优先级 |
| --- | --- | --- |
| 错误码字典 | `pkg/errcode` 全量错误码、HTTP 状态映射、前端处理建议 | 中 |
| 数据模型字典 | 表、字段、索引、迁移历史、租户/归属字段与保留策略 | 中 |
| 术语表 | CENTER/EDGE/AUTONOMY、ADMIN/AUDITOR、Kuscia、DomainData、PSI、MPC、TEE 等 | 中 |
| 版本兼容矩阵 | Kuscia、SecretFlow、Serving、前端与数据库的兼容范围 | 中 |
| 变更记录 | 对外 API、配置、数据库结构与验收结论的变更历史 | 低 |

新增文档时应遵守本目录的类型划分：产品需求、架构设计、API 参考、操作指南、运维部署、测试验收、安全设计、运行手册或证据资产；并在文首注明读者、状态、更新日期与验证方式。
