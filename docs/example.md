# 使用示例

## 1. 快速开始

### 1.1 启动服务

```bash
# 构建
cd privahub
mkdir -p .tmp
TMPDIR=$(pwd)/.tmp CGO_ENABLED=1 go build -o bin/privahub ./cmd/server

# 使用开发配置启动
SECRETPAD_PROFILE=dev ./bin/privahub

# 或直接使用默认配置
./bin/privahub
```

### 1.2 验证服务

```bash
# 健康检查
curl http://localhost:8080/health
# {"status":"ok"}

# 获取环境信息
curl http://localhost:8080/api/v1alpha1/env
```

## 2. 认证流程

### 2.1 登录获取 Token

```bash
# 登录
TOKEN=$(curl -s -X POST http://localhost:8080/api/v1alpha1/user/login \
  -H "Content-Type: application/json" \
  -d '{"name":"admin","password":"12345678"}' | jq -r '.data.token')

echo "Token: $TOKEN"
```

**响应示例：**
```json
{
  "status": {"code": 0, "msg": "success"},
  "data": {
    "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
    "user": {
      "name": "admin",
      "platformType": "AUTONOMY"
    }
  }
}
```

### 2.2 使用 Token 调用接口

```bash
# 所有后续请求需携带 Token
curl -X POST http://localhost:8080/api/v1alpha1/node/list \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{}'
```

### 2.3 Token 刷新

```bash
curl -X POST http://localhost:8080/api/v1alpha1/user/refreshToken \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{}'
```

## 3. 节点管理

### 3.1 查看节点列表

```bash
curl -s -X POST http://localhost:8080/api/v1alpha1/node/list \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{}' | jq .
```

**响应示例：**
```json
{
  "status": {"code": 0, "msg": "success"},
  "data": [
    {
      "nodeId": "kuscia-system",
      "name": "kuscia-system",
      "mode": "master",
      "status": "Ready",
      "instId": "kuscia-system"
    },
    {
      "nodeId": "alice",
      "name": "alice",
      "mode": "lite",
      "status": "Ready",
      "instId": "alice"
    }
  ]
}
```

### 3.2 注册新节点

```bash
curl -X POST http://localhost:8080/api/v1alpha1/node/create \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{
    "nodeId": "carol",
    "name": "carol-node",
    "mode": "lite",
    "addr": "192.168.1.100:8083",
    "protocol": "notls"
  }'
```

### 3.3 查看节点路由

```bash
curl -X POST http://localhost:8080/api/v1alpha1/node/route/list \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"nodeId": "alice"}'
```

## 4. 数据管理

### 4.1 注册数据源

```bash
curl -X POST http://localhost:8080/api/v1alpha1/datasource/create \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{
    "name": "alice-local-data",
    "type": "localfs",
    "path": "/home/kuscia/data",
    "nodeId": "alice"
  }'
```

### 4.2 注册数据表

```bash
curl -X POST http://localhost:8080/api/v1alpha1/datatable/create \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{
    "name": "train_data",
    "datasourceId": "ds-alice-001",
    "nodeId": "alice",
    "relativeUri": "train_data.csv",
    "columns": [
      {"name": "id", "type": "string"},
      {"name": "age", "type": "int"},
      {"name": "income", "type": "float"},
      {"name": "label", "type": "int"}
    ]
  }'
```

### 4.3 查看数据表列表

```bash
curl -X POST http://localhost:8080/api/v1alpha1/datatable/list \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"nodeId": "alice"}'
```

## 5. 项目管理

### 5.1 创建项目

```bash
curl -X POST http://localhost:8080/api/v1alpha1/project/create \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{
    "name": "信用评分联合建模",
    "description": "alice 和 bob 联合进行信用评分模型训练",
    "computeMode": "SECURE_AGGREGATE",
    "computeFunc": "DAG"
  }'
```

**响应示例：**
```json
{
  "status": {"code": 0, "msg": "success"},
  "data": {
    "projectId": "proj-20260727-abcdef"
  }
}
```

### 5.2 添加参与节点

```bash
curl -X POST http://localhost:8080/api/v1alpha1/project/node/add \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{
    "projectId": "proj-20260727-abcdef",
    "nodeId": "alice"
  }'

curl -X POST http://localhost:8080/api/v1alpha1/project/node/add \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{
    "projectId": "proj-20260727-abcdef",
    "nodeId": "bob"
  }'
```

### 5.3 绑定数据表

```bash
curl -X POST http://localhost:8080/api/v1alpha1/project/datatable/add \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{
    "projectId": "proj-20260727-abcdef",
    "nodeId": "alice",
    "datatableId": "dt-alice-train"
  }'
```

### 5.4 查看项目列表

```bash
curl -X POST http://localhost:8080/api/v1alpha1/project/list \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"page": 1, "pageSize": 10}'
```

## 6. 训练图 (DAG)

### 6.1 创建训练图

```bash
curl -X POST http://localhost:8080/api/v1alpha1/graph/create \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{
    "projectId": "proj-20260727-abcdef",
    "name": "lr-training-pipeline"
  }'
```

### 6.2 更新图结构

```bash
curl -X POST http://localhost:8080/api/v1alpha1/graph/update \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{
    "projectId": "proj-20260727-abcdef",
    "graphId": "graph-001",
    "nodes": [
      {
        "nodeId": "node-read-alice",
        "component": "data_prep/data_read",
        "params": {"datatableId": "dt-alice-train"}
      },
      {
        "nodeId": "node-read-bob",
        "component": "data_prep/data_read",
        "params": {"datatableId": "dt-bob-train"}
      },
      {
        "nodeId": "node-lr",
        "component": "ml.train/lr",
        "params": {"epochs": 10, "learningRate": 0.01}
      }
    ],
    "edges": [
      {"source": "node-read-alice", "target": "node-lr"},
      {"source": "node-read-bob", "target": "node-lr"}
    ]
  }'
```

### 6.3 启动训练

```bash
curl -X POST http://localhost:8080/api/v1alpha1/graph/start \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{
    "projectId": "proj-20260727-abcdef",
    "graphId": "graph-001"
  }'
```

### 6.4 查看节点状态

```bash
curl -X POST http://localhost:8080/api/v1alpha1/graph/node/status \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{
    "projectId": "proj-20260727-abcdef",
    "graphId": "graph-001"
  }'
```

**响应示例：**
```json
{
  "status": {"code": 0, "msg": "success"},
  "data": {
    "nodes": [
      {"nodeId": "node-read-alice", "status": "SUCCEED"},
      {"nodeId": "node-read-bob", "status": "SUCCEED"},
      {"nodeId": "node-lr", "status": "RUNNING"}
    ]
  }
}
```

### 6.5 停止训练

```bash
curl -X POST http://localhost:8080/api/v1alpha1/graph/stop \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{
    "projectId": "proj-20260727-abcdef",
    "graphId": "graph-001"
  }'
```

## 7. 任务管理

### 7.1 查看任务列表

```bash
curl -X POST http://localhost:8080/api/v1alpha1/project/job/list \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"projectId": "proj-20260727-abcdef"}'
```

### 7.2 查看任务详情

```bash
curl -X POST http://localhost:8080/api/v1alpha1/project/job/get \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{
    "projectId": "proj-20260727-abcdef",
    "jobId": "job-20260727-xyz"
  }'
```

## 8. 组件管理

### 8.1 获取组件列表

```bash
curl -X POST http://localhost:8080/api/v1alpha1/component/list \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"mode": "ALL"}'
```

### 8.2 获取组件详情

```bash
curl -X POST http://localhost:8080/api/v1alpha1/component/get \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{
    "componentId": "ml.train/lr",
    "version": "1.0.0"
  }'
```

## 9. 用户管理

### 9.1 创建用户

```bash
curl -X POST http://localhost:8080/api/v1alpha1/user/create \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{
    "name": "operator1",
    "password": "SecurePass123!",
    "platformType": "AUTONOMY"
  }'
```

### 9.2 重置密码

```bash
curl -X POST http://localhost:8080/api/v1alpha1/user/resetPassword \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{
    "name": "operator1",
    "newPassword": "NewSecurePass456!"
  }'
```

## 10. 完整工作流脚本

以下脚本演示完整的隐私计算工作流：

```bash
#!/bin/bash
set -e

BASE_URL="http://localhost:8080/api/v1alpha1"

echo "=== 1. 登录 ==="
TOKEN=$(curl -s -X POST "$BASE_URL/user/login" \
  -H "Content-Type: application/json" \
  -d '{"name":"admin","password":"12345678"}' | jq -r '.data.token')
echo "Token acquired: ${TOKEN:0:20}..."

AUTH="Authorization: Bearer $TOKEN"
CT="Content-Type: application/json"

echo ""
echo "=== 2. 查看节点 ==="
curl -s -X POST "$BASE_URL/node/list" -H "$CT" -H "$AUTH" -d '{}' | jq '.data[].nodeId'

echo ""
echo "=== 3. 创建项目 ==="
PROJECT_ID=$(curl -s -X POST "$BASE_URL/project/create" \
  -H "$CT" -H "$AUTH" \
  -d '{"name":"demo-project","computeMode":"SECURE_AGGREGATE","computeFunc":"DAG"}' \
  | jq -r '.data.projectId')
echo "Project: $PROJECT_ID"

echo ""
echo "=== 4. 添加节点 ==="
curl -s -X POST "$BASE_URL/project/node/add" \
  -H "$CT" -H "$AUTH" \
  -d "{\"projectId\":\"$PROJECT_ID\",\"nodeId\":\"alice\"}" | jq '.status'
curl -s -X POST "$BASE_URL/project/node/add" \
  -H "$CT" -H "$AUTH" \
  -d "{\"projectId\":\"$PROJECT_ID\",\"nodeId\":\"bob\"}" | jq '.status'

echo ""
echo "=== 5. 创建训练图 ==="
GRAPH_ID=$(curl -s -X POST "$BASE_URL/graph/create" \
  -H "$CT" -H "$AUTH" \
  -d "{\"projectId\":\"$PROJECT_ID\",\"name\":\"demo-graph\"}" \
  | jq -r '.data.graphId')
echo "Graph: $GRAPH_ID"

echo ""
echo "=== 6. 获取组件列表 ==="
curl -s -X POST "$BASE_URL/component/list" \
  -H "$CT" -H "$AUTH" -d '{"mode":"ALL"}' | jq '.data | length'

echo ""
echo "=== Done! ==="
```

## 11. 错误处理示例

### 11.1 未认证

```bash
curl -s -X POST http://localhost:8080/api/v1alpha1/node/list \
  -H "Content-Type: application/json" \
  -d '{}'
```

**响应：**
```json
{
  "status": {"code": 202011401, "msg": "unauthorized"},
  "data": null
}
```

### 11.2 参数错误

```bash
curl -s -X POST http://localhost:8080/api/v1alpha1/project/get \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{}'
```

**响应：**
```json
{
  "status": {"code": 202011400, "msg": "projectId is required"},
  "data": null
}
```

### 11.3 资源不存在

```bash
curl -s -X POST http://localhost:8080/api/v1alpha1/project/get \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"projectId": "non-existent"}'
```

**响应：**
```json
{
  "status": {"code": 202011404, "msg": "project not found"},
  "data": null
}
```

## 12. Inner Port API (集群内部)

Inner Port (9001) 用于集群节点间免认证通信：

```bash
# 投票同步
curl -X POST http://localhost:9001/vote_sync/create \
  -H "Content-Type: application/json" \
  -d '{"voteID": "vote-001", "type": "NODE_DELETE", "voters": ["alice","bob"]}'

# 用户同步
curl -X POST http://localhost:9001/user/node/resetPassword \
  -H "Content-Type: application/json" \
  -d '{"name": "admin", "newPassword": "synced-pass"}'

# SSE 数据同步（长连接）
curl -N http://localhost:9001/sync
```
