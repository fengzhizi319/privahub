# 运维部署文档

## 1. 环境要求

### 1.1 系统要求

| 组件 | 最低版本 | 推荐版本 |
|------|----------|----------|
| Go | 1.21 | 1.22+ |
| GCC | 9.0 | 12.0+ |
| SQLite | 3.35 | 3.40+ |
| MySQL | 5.7 | 8.0+ (可选) |

### 1.2 硬件要求

| 部署规模 | CPU | 内存 | 磁盘 |
|----------|-----|------|------|
| 开发环境 | 2 核 | 4 GB | 20 GB |
| 生产环境 | 4 核 | 8 GB | 100 GB |
| 高并发 | 8 核 | 16 GB | 500 GB |

## 2. 构建

### 2.1 本地构建

```bash
# 克隆项目
git clone https://github.com/fengzhizi319/sfwork.git
cd sfwork/privahub

# 构建
make build

# 或使用 go build
mkdir -p .tmp
TMPDIR=$(pwd)/.tmp CGO_ENABLED=1 go build -o bin/privahub ./cmd/server
```

### 2.2 交叉编译

```bash
# Linux AMD64
GOOS=linux GOARCH=amd64 CGO_ENABLED=1 go build -o bin/privahub-linux ./cmd/server

# Linux ARM64
GOOS=linux GOARCH=arm64 CGO_ENABLED=1 go build -o bin/privahub-linux-arm64 ./cmd/server
```

### 2.3 Docker 构建

```bash
docker build -t privahub:latest -f deployments/docker/Dockerfile .
```

## 3. 配置

### 3.1 配置文件

配置文件位于 `config/` 目录：

| 文件 | 说明 |
|------|------|
| `privahub.yaml` | 基础配置 |
| `privahub-dev.yaml` | 开发环境 |
| `privahub-edge.yaml` | 边缘节点 |
| `components.json` | 组件定义 |

### 3.2 配置项说明

```yaml
server:
  mode: "master"           # master, lite, autonomy
  http_port: 8080          # 主 HTTP 端口
  inner_port: 9001         # 内部端口
  grpc_port: 9090          # gRPC 端口 (预留)
  read_timeout: 10s
  write_timeout: 10s
  shutdown_timeout: 5s

database:
  driver: "sqlite"         # sqlite, mysql
  dsn: "privahub.db?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
  max_open_conns: 50
  max_idle_conns: 10

kuscia:
  namespace: "kuscia-system"
  api_address: "127.0.0.1"
  api_port: 8083
  protocol: "notls"        # tls, notls
  gateway: "127.0.0.1:1080"
  nodes:                   # 多节点配置
    - domain_id: "kuscia-system"
      mode: "master"
      host: "127.0.0.1"
      port: 8083
      protocol: "notls"

observability:
  enable_metrics: true
  metrics_port: 9091
  log_level: "info"        # debug, info, warn, error
  log_format: "console"    # json, console

auth:
  jwt_secret: "change-me-in-production"
  access_token_expiry: "2h"
  refresh_token_expiry: "168h"
```

### 3.3 环境变量覆盖

所有配置项支持环境变量覆盖，前缀 `PRIVAHUB_`：

```bash
export PRIVAHUB_SERVER_HTTP_PORT=9080
export PRIVAHUB_KUSCIA_API_ADDRESS=10.0.0.1
export PRIVAHUB_DATABASE_DRIVER=mysql
export PRIVAHUB_DATABASE_DSN="user:pass@tcp(localhost:3306)/secretpad"
```

### 3.4 Profile 配置

通过 `PRIVAHUB_PROFILE` 环境变量加载 profile 配置：

```bash
# 开发环境
PRIVAHUB_PROFILE=dev ./privahub

# 边缘节点
PRIVAHUB_PROFILE=edge NODE_ID=edge-node ./privahub
```

## 4. 部署

### 4.1 单机部署

```bash
# 1. 构建
make build

# 2. 配置
cp config/privahub.yaml config/secretpad-local.yaml
# 编辑配置...

# 3. 运行
./bin/privahub -c config/secretpad-local.yaml
```

### 4.2 Docker 部署

```bash
# 构建镜像
docker build -t privahub:latest .

# 运行容器
docker run -d \
  --name privahub \
  -p 8080:8080 \
  -p 9001:9001 \
  -v $(pwd)/data:/app/data \
  -v $(pwd)/config:/app/config \
  -e PRIVAHUB_PROFILE=dev \
  privahub:latest
```

### 4.3 Docker Compose 部署

```yaml
version: '3.8'

services:
  privahub:
    build: .
    ports:
      - "8080:8080"
      - "9001:9001"
    volumes:
      - ./data:/app/data
      - ./config:/app/config
    environment:
      - PRIVAHUB_PROFILE=dev
      - PRIVAHUB_KUSCIA_API_ADDRESS=kuscia-master
    depends_on:
      - kuscia-master

  kuscia-master:
    image: secretflow/kuscia:latest
    ports:
      - "18083:8083"
    # ... Kuscia 配置
```

### 4.4 Kubernetes 部署

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: privahub
spec:
  replicas: 2
  selector:
    matchLabels:
      app: privahub
  template:
    metadata:
      labels:
        app: privahub
    spec:
      containers:
      - name: privahub
        image: privahub:latest
        ports:
        - containerPort: 8080
        - containerPort: 9001
        env:
        - name: PRIVAHUB_PROFILE
          value: "dev"
        resources:
          requests:
            cpu: "500m"
            memory: "512Mi"
          limits:
            cpu: "2000m"
            memory: "2Gi"
        livenessProbe:
          httpGet:
            path: /api/v1alpha1/healthz
            port: 8080
          initialDelaySeconds: 10
          periodSeconds: 30
        readinessProbe:
          httpGet:
            path: /api/v1alpha1/healthz
            port: 8080
          initialDelaySeconds: 5
          periodSeconds: 10
---
apiVersion: v1
kind: Service
metadata:
  name: privahub
spec:
  selector:
    app: privahub
  ports:
  - name: http
    port: 8080
    targetPort: 8080
  - name: inner
    port: 9001
    targetPort: 9001
```

## 5. 监控

### 5.1 健康检查

| 端点 | 端口 | 说明 |
|------|------|------|
| `/api/v1alpha1/healthz` | 8080 | 主服务健康 |
| `/healthz` | 9001 | 内部端口健康 |

### 5.2 Prometheus 指标

指标端点: `http://localhost:8080/metrics`

关键指标:

| 指标 | 类型 | 说明 |
|------|------|------|
| `http_requests_total` | Counter | HTTP 请求总数 |
| `http_request_duration_seconds` | Histogram | 请求延迟分布 |

### 5.3 日志

日志格式:
- `console`: 开发环境，人类可读
- `json`: 生产环境，结构化

日志级别:
- `debug`: 调试信息
- `info`: 一般信息
- `warn`: 警告
- `error`: 错误

## 6. 备份与恢复

### 6.1 SQLite 备份

```bash
# 备份
sqlite3 privahub.db ".backup backups/secretpad-$(date +%Y%m%d).db"

# 恢复
sqlite3 privahub.db ".restore backups/secretpad-20240101.db"
```

### 6.2 MySQL 备份

```bash
# 备份
mysqldump -u user -p secretpad > backups/secretpad-$(date +%Y%m%d).sql

# 恢复
mysql -u user -p secretpad < backups/secretpad-20240101.sql
```

## 7. 故障排查

### 7.1 常见问题

| 问题 | 原因 | 解决方案 |
|------|------|----------|
| 启动失败: port in use | 端口被占用 | 修改配置或停止占用进程 |
| Kuscia 连接失败 | 地址/端口错误 | 检查 kuscia.api_address/api_port |
| 数据库锁定 | SQLite 并发限制 | 增加 busy_timeout 或使用 MySQL |
| JWT 认证失败 | Token 过期 | 重新登录获取 Token |

### 7.2 调试模式

```bash
# 开启 debug 日志
PRIVAHUB_OBSERVABILITY_LOG_LEVEL=debug ./privahub

# 查看请求日志
tail -f logs/secretpad.log | grep "audit"
```

### 7.3 性能分析

```bash
# pprof 端点 (需开启)
go tool pprof http://localhost:8080/debug/pprof/profile

# 查看 goroutine
curl http://localhost:8080/debug/pprof/goroutine?debug=1
```

## 8. 安全加固

### 8.1 生产环境检查清单

- [ ] 修改 `auth.jwt_secret` 为强随机字符串
- [ ] 修改默认管理员密码
- [ ] 配置 `middleware.IPWhitelist` 限制访问来源
- [ ] 启用 HTTPS (通过反向代理)
- [ ] 设置 `log_format: json` 便于日志分析
- [ ] 配置日志轮转
- [ ] 限制 `BodyLimit` 防止大请求攻击
- [ ] 配置 `RateLimiter` 防止暴力破解

### 8.2 Nginx 反向代理配置

```nginx
server {
    listen 443 ssl;
    server_name secretpad.example.com;

    ssl_certificate /etc/ssl/certs/secretpad.crt;
    ssl_certificate_key /etc/ssl/private/secretpad.key;

    location /api/ {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    }

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
    }
}
```

## 9. 升级

### 9.1 滚动升级

```bash
# 1. 构建新版本
make build

# 2. 备份数据库
sqlite3 privahub.db ".backup backups/pre-upgrade.db"

# 3. 停止旧版本
pkill -f privahub

# 4. 启动新版本
./bin/privahub &

# 5. 验证
curl http://localhost:8080/api/v1alpha1/healthz
```

### 9.2 数据库迁移

启动时自动执行 GORM AutoMigrate，无需手动迁移。
