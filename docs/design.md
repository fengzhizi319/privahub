# 架构设计文档

## 1. 系统概述

SecretPad-Go 是隐私计算平台 SecretPad 后端的 Go 语言实现，完整迁移自 Java Spring Boot 版本。系统采用分层架构设计，提供 RESTful API 供前端调用，并通过 Kuscia 控制平面执行隐私计算任务。

## 2. 架构分层

```
┌─────────────────────────────────────────────────────────────┐
│                      Client (Frontend)                       │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                   HTTP Layer (Gin)                           │
│  ┌─────────────────────────────────────────────────────┐   │
│  │              Middleware Chain (10)                   │   │
│  │  TraceID → Recovery → Metrics → CORS → Security     │   │
│  │  → AuditLog → RateLimiter → BodyLimit → JWTAuth     │   │
│  └─────────────────────────────────────────────────────┘   │
│  ┌─────────────────────────────────────────────────────┐   │
│  │              Router (143 routes)                     │   │
│  │  /api/v1alpha1/* → 17 Handlers                      │   │
│  └─────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                   Service Layer (22)                         │
│  Auth │ Project │ Graph │ Job │ Node │ Datatable │ ...      │
└─────────────────────────────────────────────────────────────┘
                              │
              ┌───────────────┼───────────────┐
              ▼               ▼               ▼
┌──────────────────┐ ┌──────────────┐ ┌──────────────────┐
│   DAO Layer      │ │ Kuscia Client│ │   External       │
│   (GORM)         │ │ (HTTP/JSON)  │ │   Services       │
│                  │ │              │ │                  │
│ • Models (12)    │ │ • Domain     │ │ • SSE Server     │
│ • Repositories   │ │ • DomainData │ │ • Cron Engine    │
│ • Migrations     │ │ • Job        │ │ • Data Directory │
│ • Seed Data      │ │ • Serving    │ │                  │
└──────────────────┘ │ • Route      │ └──────────────────┘
                     │ • Certificate│
                     └──────────────┘
```

## 3. 目录结构

```
privahub/
├── cmd/                          # 可执行入口
│   ├── server/main.go            # 主服务 (HTTP + Inner Port)
│   ├── edge-agent/main.go        # 边缘代理
│   └── migrator/main.go          # 数据库迁移工具
│
├── internal/                     # 私有业务代码
│   ├── controller/http/
│   │   ├── router.go             # 主路由注册
│   │   ├── inner_router.go       # 内部端口路由 (9001)
│   │   ├── middleware/           # 中间件
│   │   │   ├── middleware.go     # TraceID/Recovery/Metrics/CORS/...
│   │   │   ├── auth.go           # JWT 认证
│   │   │   └── rbac.go           # 权限控制
│   │   └── v1/                   # Handler 层
│   │       ├── auth_handler.go
│   │       ├── project_handler.go
│   │       ├── graph_handler.go
│   │       ├── job_handler.go
│   │       ├── node_handler.go
│   │       ├── datatable_handler.go
│   │       ├── datasource_handler.go
│   │       ├── model_handler.go
│   │       ├── vote_handler.go
│   │       ├── user_handler.go
│   │       ├── scheduled_handler.go
│   │       ├── data_handler.go
│   │       ├── p2p_handler.go
│   │       ├── noderoute_handler.go
│   │       ├── approval_handler.go
│   │       ├── message_handler.go
│   │       └── misc_handler.go
│   │
│   ├── service/                  # 业务逻辑层
│   │   ├── auth_service.go
│   │   ├── project_service.go
│   │   ├── graph_service.go
│   │   ├── job_service.go
│   │   ├── job_sync_service.go   # 后台任务状态同步
│   │   ├── node_service.go
│   │   ├── datatable_service.go
│   │   ├── datasource_service.go
│   │   ├── model_service.go
│   │   ├── vote_service.go
│   │   ├── user_service.go
│   │   ├── scheduled_service.go  # Cron 引擎
│   │   ├── serving_sync_service.go
│   │   ├── edgedatasync_service.go
│   │   ├── env_service.go        # 平台拓扑感知
│   │   ├── datadir_service.go    # 数据目录扫描
│   │   ├── featuretable_service.go
│   │   ├── graphdatasource_service.go
│   │   ├── nodeuser_service.go
│   │   ├── oss_service.go        # S3 连通性检测
│   │   ├── partition_rule_service.go
│   │   └── sse_server.go         # SSE 实时同步
│   │
│   ├── dao/                      # 数据访问层
│   │   ├── db.go                 # 数据库连接
│   │   ├── model/                # GORM 模型
│   │   │   ├── core.go           # Project/Graph/Component
│   │   │   ├── job.go            # Job/Task
│   │   │   ├── datatable.go      # Datatable/FedTable
│   │   │   ├── datasource.go
│   │   │   ├── user.go
│   │   │   ├── vote.go
│   │   │   ├── serving.go
│   │   │   ├── scheduled.go
│   │   │   ├── graph.go
│   │   │   └── misc.go
│   │   ├── repository/           # 仓储接口
│   │   │   ├── interfaces.go
│   │   │   ├── base_repo.go
│   │   │   ├── core_repo.go
│   │   │   ├── job_repo.go
│   │   │   └── user_repo.go
│   │   └── migrations/
│   │       └── migrate.go        # AutoMigrate + SeedData
│   │
│   └── wire/
│       └── wire.go               # 依赖注入容器 (App struct)
│
├── pkg/                          # 公共库
│   ├── config/config.go          # Viper 配置
│   ├── kuscia/                   # Kuscia HTTP 客户端
│   │   ├── client.go
│   │   ├── domain.go
│   │   ├── job.go
│   │   ├── serving.go
│   │   ├── route.go
│   │   └── certificate.go
│   ├── auth/jwt.go               # JWT 管理器
│   ├── errcode/codes.go          # 错误码
│   ├── response/response.go      # 统一响应
│   ├── logger/zap.go             # 日志
│   └── metrics/prometheus.go     # 指标
│
├── config/                       # 配置文件
│   ├── secretpad.yaml
│   ├── secretpad-dev.yaml
│   ├── secretpad-edge.yaml
│   └── components.json           # 组件定义
│
└── deployments/docker/
    └── Dockerfile
```

## 4. 核心设计模式

### 4.1 依赖注入

采用手动依赖注入，通过 `wire.App` 结构体聚合所有依赖：

```go
type App struct {
    DB           *gorm.DB
    Config       *config.Config
    JWTManager   *auth.JWTManager
    KusciaClient *kuscia.Client
    
    // Services
    AuthService  *service.AuthService
    ProjectService *service.ProjectService
    // ... 22 services
    
    // Handlers
    AuthHandler  *v1.AuthHandler
    ProjectHandler *v1.ProjectHandler
    // ... 17 handlers
}
```

### 4.2 统一响应格式

所有 API 返回统一格式：

```json
{
  "status": {
    "code": 0,
    "msg": "success"
  },
  "data": { ... }
}
```

错误响应：

```json
{
  "status": {
    "code": 202011500,
    "msg": "system error"
  },
  "data": null
}
```

### 4.3 中间件链

```
Request → TraceID → Recovery → Metrics → CORS → SecurityHeaders
        → AuditLog → RateLimiter → BodyLimit → [JWTAuth] → Handler
```

### 4.4 优雅降级

Kuscia 不可达时，本地操作仍然成功：

```go
// 创建项目时同步到 Kuscia
if err := kusciaClient.CreateDomain(ctx, req); err != nil {
    log.Warn("Kuscia sync failed, continuing locally", zap.Error(err))
    // 不返回错误，本地创建成功
}
```

## 5. 数据模型

### 5.1 核心实体

| 实体 | 表名 | 说明 |
|------|------|------|
| ProjectDO | project | 项目 |
| ProjectGraphDO | project_graph | DAG 图 |
| ProjectJobDO | project_job | 任务 |
| ProjectTaskDO | project_task | 子任务 |
| DatatableDO | datatable | 数据表 |
| DatasourceDO | datasource | 数据源 |
| UserDO | user | 用户 |
| VoteDO | vote | 投票 |
| ServingDO | serving | 模型服务 |
| ScheduleTaskDO | schedule_task | 定时任务 |

### 5.2 种子数据

启动时自动初始化：

- 默认机构 (inst)
- 默认节点 (kuscia-system, alice, bob)
- 管理员用户 (admin / 12345678)
- RBAC 角色权限

## 6. 端口规划

| 端口 | 用途 | 认证 |
|------|------|------|
| 8080 | 主 HTTP 服务 | JWT |
| 9001 | 内部端口 (集群通信) | 无 |
| 9090 | gRPC (预留) | - |
| 9091 | Prometheus 指标 | 无 |

## 7. 配置管理

### 7.1 多 Profile 支持

```bash
# 基础配置
./privahub

# 开发环境
SECRETPAD_PROFILE=dev ./privahub

# 边缘节点
SECRETPAD_PROFILE=edge ./privahub
```

### 7.2 环境变量覆盖

所有配置项支持环境变量覆盖，前缀 `SECRETPAD_`：

```bash
SECRETPAD_SERVER_HTTP_PORT=9080
SECRETPAD_KUSCIA_API_ADDRESS=10.0.0.1
SECRETPAD_DATABASE_DRIVER=mysql
```

## 8. 安全设计

| 机制 | 实现 |
|------|------|
| 认证 | JWT (HS256, 2h 过期) |
| 授权 | RBAC 中间件 |
| 限流 | 令牌桶 (100 burst, 20/s) |
| 体限制 | 32 MB |
| IP 白名单 | CIDR 匹配 |
| 审计日志 | 写操作记录 |
| SSRF 防护 | OSS endpoint 校验 |
| 路径遍历 | 数据目录 safePath |

## 9. 可观测性

### 9.1 日志

- 结构化日志 (Zap)
- TraceID 贯穿请求链路
- 审计日志独立记录

### 9.2 指标

- `http_requests_total` - 请求计数
- `http_request_duration_seconds` - 请求延迟
- Prometheus 格式暴露于 `/metrics`

### 9.3 健康检查

- `GET /api/v1alpha1/healthz` - 主服务健康
- `GET /healthz` (9001) - 内部端口健康
