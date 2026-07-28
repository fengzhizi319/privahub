# Privahub

SecretPad 后端的 Go 语言重构实现，基于 Gin + GORM 构建，完整迁移自 Java Spring Boot 版本。

## 项目概览

| 指标 | 数值 |
|------|------|
| Go 文件 | 84 |
| 代码行数 | 14,686 |
| 注册路由 | 143 |
| Service 文件 | 22 |
| Kuscia 方法 | 25 |
| 测试用例 | 55 |
| 配置 Profile | 3 (base/dev/edge) |

## 技术栈

- **Web 框架**: Gin v1.10
- **ORM**: GORM v1.25 (SQLite/MySQL)
- **配置管理**: Viper v1.19
- **日志**: Zap v1.27
- **定时任务**: robfig/cron v3
- **监控**: Prometheus client
- **认证**: JWT (golang-jwt/v5)

## 快速开始

### 构建

```bash
# 构建主服务
make build

# 或使用 go build
CGO_ENABLED=1 go build -o bin/privahub ./cmd/server
```

### 运行

```bash
# 默认配置
./bin/privahub

# 开发环境 profile
PRIVAHUB_PROFILE=dev ./bin/privahub

# 边缘节点 profile
PRIVAHUB_PROFILE=edge NODE_ID=edge-node ./bin/privahub
```

### 测试

```bash
# 运行全部测试
make test

# 或使用 go test
CGO_ENABLED=1 go test ./... -v
```

## 项目结构

```
privahub/
├── cmd/
│   ├── server/          # 主服务入口
│   ├── edge-agent/      # 边缘代理
│   └── migrator/        # 数据库迁移工具
├── internal/
│   ├── controller/http/ # HTTP 路由 + Handler
│   │   ├── v1/          # 17 个业务 Handler
│   │   └── middleware/  # 10 个中间件
│   ├── service/         # 22 个业务 Service
│   ├── dao/             # 数据访问层
│   │   ├── model/       # GORM 模型
│   │   ├── repository/  # 仓储接口
│   │   └── migrations/  # 迁移 + 种子数据
│   └── wire/            # 依赖注入容器
├── pkg/
│   ├── config/          # Viper 配置管理
│   ├── kuscia/          # Kuscia HTTP 客户端
│   ├── auth/            # JWT 认证
│   ├── errcode/         # 错误码定义
│   ├── response/        # 统一响应格式
│   ├── logger/          # Zap 日志
│   └── metrics/         # Prometheus 指标
├── config/              # 配置文件
│   ├── privahub.yaml       # 基础配置
│   ├── privahub-dev.yaml   # 开发环境
│   └── privahub-edge.yaml  # 边缘节点
├── docs/                # 文档
└── deployments/docker/  # Docker 部署
```

## 核心功能

### 已迁移服务 (27 Java → 22 Go + 5 内联)

| 服务 | 说明 |
|------|------|
| AuthService | 用户认证、JWT 令牌管理 |
| ProjectService | 项目管理、成员管理 |
| GraphService | DAG 图管理、组件配置 |
| JobService | 任务管理、状态同步 |
| NodeService | 节点管理、Kuscia Domain |
| DatatableService | 数据表管理 |
| DatasourceService | 数据源管理 |
| ModelService | 模型管理、导出 |
| VoteService | 投票管理、P2P 同步 |
| ScheduledService | 定时任务 (Cron 引擎) |
| EnvService | 平台拓扑感知 |
| SseServer | SSE 实时数据同步 |
| ... | 共 22 个独立 Service |

### 中间件 (10 个)

| 中间件 | 说明 |
|--------|------|
| TraceID | 请求追踪 ID |
| Recovery | Panic 恢复 |
| Metrics | Prometheus 指标 |
| CORS | 跨域配置 |
| SecurityHeaders | 安全响应头 |
| AuditLog | 写操作审计日志 |
| IPWhitelist | IP 白名单 (CIDR) |
| RateLimiter | 令牌桶限流 |
| BodyLimit | 请求体大小限制 |
| JWTAuth | JWT 认证 |

### Kuscia 集成 (25 个方法)

- Domain 管理: Create/Delete/Query
- DomainData 管理: Create/Delete/Query/List/Grant
- DomainRoute 管理: Create/Delete/Query/BatchStatus
- Job 管理: Create/Delete/Query/Stop/BatchStatus
- Serving 管理: Create/Delete/Query/Update/BatchStatus
- 证书管理: GenerateKeyCerts

## 配置说明

### 环境变量

| 变量 | 说明 | 默认值 |
|------|------|--------|
| `PRIVAHUB_PROFILE` | 配置 profile | (空) |
| `PRIVAHUB_SERVER_HTTP_PORT` | HTTP 端口 | 8080 |
| `PRIVAHUB_SERVER_INNER_PORT` | 内部端口 | 9001 |
| `PRIVAHUB_KUSCIA_API_ADDRESS` | Kuscia 地址 | 127.0.0.1 |
| `PRIVAHUB_KUSCIA_API_PORT` | Kuscia 端口 | 8083 |

### 配置文件

支持多 Profile 配置，通过 `PRIVAHUB_PROFILE` 环境变量指定：

- `privahub.yaml` - 基础配置
- `privahub-dev.yaml` - 开发环境 (Docker Kuscia 端口映射)
- `privahub-edge.yaml` - 边缘节点 (lite 模式)

## 文档索引

| 文档 | 说明 |
|------|------|
| [design.md](design.md) | 架构设计文档 |
| [api.md](api.md) | API 接口文档 |
| [ops.md](ops.md) | 运维部署文档 |
| [testing.md](testing.md) | 测试指南 |
| [prd.md](prd.md) | 产品需求文档 |
| [example.md](example.md) | 使用示例 |

## 许可证

Apache License 2.0
