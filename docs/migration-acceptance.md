# 业务迁移验收与系统测试

本文记录可重复验证的迁移证据；路由覆盖不等于所有部署模式及外部系统已完成验收。
保留 Go 服务分层、GORM、数据库调度锁和新前端架构，不移植 Spring/Quartz/旧 UI 框架。

## 独立契约基线

从旧 Java Controller 提取 **25 个控制器、121 个业务路由**，保存在
`internal/controller/http/testdata/legacy_routes.json`。`TestMigration_LegacyRouteCoverage`
核对 Go 主/内部监听器，并验证内部同步和节点密码重置接口在主监听器返回 404。
这项检查只证明路由及监听器边界，不证明响应语义或业务闭环。

```bash
python3 scripts/migration_inventory.py --source /home/charles/code/sfwork/secretpad --check
```

## 业务验证矩阵

| 业务 | 已具备的自动化证据 | 尚需外部环境验收 |
| --- | --- | --- |
| 登录、会话、权限 | Go 安全/路由测试；浏览器真实 HttpOnly Cookie、CSRF、刷新及退出 | 生产 TLS/反向代理配置 |
| 项目、成员、画布 | HTTP 创建、节点关联、图更新往返、禁止删除非空项目、清理；浏览器保存并重载 | 多机构跨域协作 |
| 数据源、数据表、特征表 | 既有服务测试；HTTP 特征表创建、列表取 ID、查询、授权、解除、删除 | OSS/ODPS/MySQL 各供应商真实读写 |
| 组件、版本、国际化 | HTTP 列表/批量/版本/i18n；客户端兼容 Java/Go 数组响应 | 不同算法镜像版本组合 |
| 作业、结果 | 图渲染/状态同步测试；真实 alice/bob PSI 成功及 table/report 输出检查 | 全算法矩阵、故障恢复、长时间压力 |
| 定时任务 | 服务测试覆盖租户隔离、任务归属、排序、运行/停止状态；HTTP 查询 | 多实例长时间调度与重启恢复 |
| 模型、在线服务 | 模型测试及项目归属、空配置、重复参与方拒绝；HTTP 非法发布拒绝 | 真实 Serving 镜像部署/推理/下线 |
| 审批、消息、P2P、同步 | 既有服务/路由测试；HTTP 消息及非法审批拒绝；同步游标原子 upsert | 双端部署审批、P2P 协商及断网重试 |
| TEE、机构、节点路由 | 既有服务/路由测试；主端口内部接口隔离 | TEE 硬件、多机构证书/网络真实链路 |
| 前端 | Vitest；真实 Chromium 登录/DAG/项目闭环、19 个 CENTER 页面无崩溃、移动导航/键盘/主题 | 全角色操作验收及人工无障碍审查 |

## 一键门禁

在 privahub 根目录执行：

```bash
bash scripts/test_migration.sh --legacy-source /home/charles/code/sfwork/secretpad
# 另加真实 Kuscia PSI（需要含 pandas 的 Python）：
MIGRATION_PYTHON=/home/charles/miniconda3/envs/sf310/bin/python \
  bash scripts/test_migration.sh --kuscia
```

包含 Go 全量测试、核心服务/路由 race、vet、前端类型/lint/单元测试/构建、
隔离 HTTP 与浏览器系统测试。`--no-browser` 仅用于无 Chromium 环境，不代表浏览器验收。

单独重跑系统测试：

```bash
python3 scripts/run_system_e2e.py --browser
MIGRATION_PYTHON=/path/to/python python3 scripts/run_system_e2e.py --browser --kuscia
```

每次创建权限 0700 的临时目录、随机密码、独立 SQLite/数据目录和动态端口，
退出时清理进程组，保留 `report.json`、各阶段日志和 `screenshots/`。指定
`--output /new/path` 可固定产物位置，已存在目录会拒绝覆盖。失败截图/浏览器
错误上下文可能包含测试输入；产物应按测试数据管理，不公开发布整个目录。

默认 HTTP 测试不连接 Kuscia。`--kuscia` 使用当前本地 Docker 映射：master
28082/28083、alice 28182/28183、bob 28282/28283，网关 28080。
可通过 `MIGRATION_KUSCIA_HTTP_PORT`、`MIGRATION_KUSCIA_GRPC_PORT`、
`MIGRATION_KUSCIA_GATEWAY`、`MIGRATION_ALICE_HTTP_PORT`/`GRPC_PORT`、
`MIGRATION_BOB_HTTP_PORT`/`GRPC_PORT` 覆盖。此模式会在实际 Kuscia 创建测试作业，
应使用开发集群。PSI 使用已有 demo 表 alice-table/bob-table，不覆盖输入数据。

## 本次修复范围

- 模型详情、信息及发布绑定已授权项目；空参与方/重复节点在服务边界拒绝。
- 定时任务列表、详情、操作校验租户及任务归属；白名单排序避免 SQL 注入；
  RUNNING/STOPPING 都阻止下线；列表运行状态批量查询，消除逐行计数。
- 同步日志游标使用唯一键原子 upsert，消除读后写竞争窗口。
- 前端 void 成功响应兼容 data 缺失/null，仍拒绝业务错误和非法 envelope；
  组件批量数组转换为前端索引，保持旧接口响应契约。
- 控制台统一深色导航、SVG 图标、移动模态导航、跳过导航和焦点样式；
  卡片及表单使用语义主题令牌，优化项目列表与顶部响应式布局。

完整业务迁移的最终签收仍以矩阵右列为准，不能用已有单测或路由数字替代外部链路验收。

## 2026-10-15 实测结果

一键总门禁通过：Java 基线检查、Go 全量测试/race/vet、前端类型/lint、
60 个测试文件 408 项用例、生产构建、10 项 Chromium 测试、隔离 HTTP 闭环、
真实双节点 PSI（SUCCEED，table/report 输出均验证）。另以 race 连续运行三次
多连接同步游标并发回归，均通过。

本机报告：`/tmp/privahub-migration-i1hlt9eq/report.json`；总门禁日志：
`/tmp/privahub-migration-final-gate.log`；截图在报告目录 `screenshots/`。
临时产物可能被系统清理，应按上述命令重建，不能作为永久验收凭据。
这些结果不替代矩阵中列出的外部链路验收。
