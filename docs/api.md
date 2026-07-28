# API 接口文档

## 概述

- **Base URL**: `http://localhost:8080/api/v1alpha1`
- **认证方式**: JWT Bearer Token (除登录外所有接口)
- **内容类型**: `application/json`
- **路由总数**: 143

## 统一响应格式

### 成功响应

```json
{
  "status": {
    "code": 0,
    "msg": "success"
  },
  "data": { ... }
}
```

### 错误响应

```json
{
  "status": {
    "code": 202011500,
    "msg": "error message"
  },
  "data": null
}
```

## 错误码

| 错误码 | 说明 |
|--------|------|
| 0 | 成功 |
| 202011400 | 参数错误 |
| 202011401 | 未认证 |
| 202011403 | 无权限 |
| 202011404 | 资源不存在 |
| 202011500 | 系统错误 |

---

## 认证接口

### POST /user/login

用户登录，获取 JWT Token。

**请求体**:
```json
{
  "name": "admin",
  "password": "12345678"
}
```

**响应**:
```json
{
  "status": {"code": 0, "msg": "success"},
  "data": {
    "token": "eyJhbGciOiJIUzI1NiIs...",
    "expire_time": "2024-01-01T12:00:00Z"
  }
}
```

### GET /user/current

获取当前登录用户信息。

**Headers**: `Authorization: Bearer <token>`

---

## 项目接口

### POST /project/create

创建项目。

**请求体**:
```json
{
  "name": "项目名称",
  "description": "项目描述",
  "compute_mode": "MPC"
}
```

### POST /project/list

获取项目列表。

**请求体**:
```json
{
  "page": 1,
  "page_size": 10
}
```

### POST /project/get

获取项目详情。

**请求体**:
```json
{
  "project_id": "proj-xxx"
}
```

### POST /project/update

更新项目。

### POST /project/delete

删除项目。

### POST /project/node/add

添加项目参与节点。

**请求体**:
```json
{
  "project_id": "proj-xxx",
  "node_id": "alice"
}
```

### POST /project/datatable/add

添加项目数据表。

### POST /project/datatable/delete

删除项目数据表。

---

## DAG 图接口

### POST /graph/create

创建 DAG 图。

**请求体**:
```json
{
  "project_id": "proj-xxx",
  "name": "图名称"
}
```

### POST /graph/list

获取图列表。

### POST /graph/detail

获取图详情（含节点和边）。

**请求体**:
```json
{
  "graph_id": "graph-xxx"
}
```

### POST /graph/update

全量更新图结构。

**请求体**:
```json
{
  "graph_id": "graph-xxx",
  "nodes": [...],
  "edges": [...]
}
```

### POST /graph/node/update

更新单个节点配置。

**请求体**:
```json
{
  "graph_id": "graph-xxx",
  "node_id": "node-xxx",
  "component": "data_prep/csv_data_import",
  "params": {...}
}
```

### POST /graph/start

启动图执行。

**请求体**:
```json
{
  "graph_id": "graph-xxx"
}
```

### POST /graph/stop

停止图执行。

### POST /graph/node/status

获取节点执行状态。

### POST /graph/node/output

获取节点输出数据。

### POST /graph/node/logs

获取节点执行日志。

---

## 任务接口

### POST /project/job/create

创建任务。

### POST /project/job/list

获取任务列表。

### POST /project/job/detail

获取任务详情。

### POST /project/job/stop

停止任务。

### POST /project/job/task/log

获取子任务日志。

---

## 节点接口

### POST /node/create

创建节点（注册到 Kuscia Domain）。

**请求体**:
```json
{
  "node_id": "alice",
  "name": "Alice 节点",
  "mode": "lite"
}
```

### POST /node/list

获取节点列表。

### POST /node/get

获取节点详情。

### POST /node/update

更新节点。

### POST /node/delete

删除节点。

### POST /node/token

获取节点 Token。

### POST /node/route/create

创建节点路由。

### POST /node/route/list

获取节点路由列表。

### POST /node/route/delete

删除节点路由。

---

## 数据表接口

### POST /datatable/register

注册数据表。

**请求体**:
```json
{
  "name": "数据表名称",
  "datasource_id": "ds-xxx",
  "relative_uri": "data/train.csv",
  "node_id": "alice",
  "columns": [
    {"name": "id", "type": "string"},
    {"name": "age", "type": "int"}
  ]
}
```

### POST /datatable/list

获取数据表列表。

### POST /datatable/detail

获取数据表详情。

### POST /datatable/delete

删除数据表。

### POST /datatable/grant

授权数据表给其他节点。

**请求体**:
```json
{
  "datatable_id": "dt-xxx",
  "grant_node_id": "bob"
}
```

### POST /datatable/fed/create

创建联邦表。

---

## 数据源接口

### POST /datasource/create

创建数据源。

**请求体**:
```json
{
  "name": "本地数据源",
  "type": "LOCAL_FS",
  "path": "/data"
}
```

### POST /datasource/list

获取数据源列表。

### POST /datasource/detail

获取数据源详情。

### POST /datasource/delete

删除数据源。

### POST /datasource/test

测试数据源连通性。

---

## 模型接口

### POST /model/list

获取模型列表。

### POST /model/detail

获取模型详情。

### POST /model/delete

删除模型。

### POST /model/export

导出模型。

### POST /model/pack

打包模型。

### POST /model/status

获取打包状态。

---

## 模型服务接口

### POST /serving/create

创建模型服务。

### POST /serving/list

获取服务列表。

### POST /serving/delete

删除服务。

### POST /serving/detail

获取服务详情。

---

## 投票接口

### POST /vote/create

创建投票。

**请求体**:
```json
{
  "type": "PROJECT_SERVING_APPROVAL",
  "voters": ["alice", "bob"],
  "resource_id": "res-xxx"
}
```

### POST /vote/list

获取投票列表。

### POST /vote/detail

获取投票详情。

### POST /vote/reply

回复投票。

**请求体**:
```json
{
  "vote_id": "vote-xxx",
  "action": "AGREE"
}
```

---

## 用户接口

### POST /user/create

创建用户。

### POST /user/list

获取用户列表。

### POST /user/get

获取用户详情。

### POST /user/update

更新用户。

### POST /user/delete

删除用户。

### POST /user/reset-password

重置密码。

### POST /user/updatePwd

修改密码。

---

## 定时任务接口

### POST /scheduled/create

创建定时任务。

**请求体**:
```json
{
  "project_id": "proj-xxx",
  "graph_id": "graph-xxx",
  "cron": "0 0 2 * * ?",
  "name": "每日凌晨2点执行"
}
```

### POST /scheduled/list

获取定时任务列表。

### POST /scheduled/delete

删除定时任务。

### POST /scheduled/pause

暂停定时任务。

### POST /scheduled/resume

恢复定时任务。

### POST /scheduled/offline

下线定时任务。

---

## 组件接口

### POST /component/list

获取组件列表。

### POST /component/version

获取组件版本。

### POST /component/i18n

获取组件国际化配置。

### POST /component/batch

批量获取组件信息。

---

## 机构接口

### POST /inst/create

创建机构。

### POST /inst/list

获取机构列表。

### POST /inst/get

获取机构详情。

### POST /inst/node/list

获取机构节点列表。

### POST /inst/node/add

添加机构节点。

### POST /inst/node/token

获取机构节点 Token。

### POST /inst/node/delete

删除机构节点。

---

## 审批接口

### POST /approval/create

创建审批。

### POST /approval/pull/status

拉取审批状态。

---

## 消息接口

### POST /message/list

获取消息列表。

### POST /message/detail

获取消息详情。

### POST /message/reply

回复消息。

### POST /message/pending

获取待处理消息。

---

## 数据接口

### POST /data/upload

上传数据文件。

**Content-Type**: `multipart/form-data`

**表单字段**:
- `file`: CSV 文件
- `node_id`: 节点 ID

### POST /data/download

下载数据文件。

### POST /data/sync

数据同步。

---

## 节点路由接口

### POST /nodeRoute/page

分页获取节点路由。

### POST /nodeRoute/get

获取节点路由详情。

### POST /nodeRoute/update

更新节点路由。

### POST /nodeRoute/listNode

获取可路由节点列表。

### POST /nodeRoute/refresh

刷新节点路由状态。

### POST /nodeRoute/delete

删除节点路由。

---

## 内部接口 (9001 端口)

以下接口在内部端口 9001 上提供，无需 JWT 认证：

### POST /api/v1alpha1/vote_sync/create

投票同步创建。

### POST /api/v1alpha1/user/node/resetPassword

节点用户密码重置。

### POST /api/v1alpha1/data/sync

数据同步。

### GET /sync

SSE 实时数据同步端点。

### GET /healthz

健康检查。

---

## 公共接口

### GET /api/v1alpha1/healthz

健康检查。

**响应**:
```json
{
  "status": {"code": 0, "msg": "success"},
  "data": {
    "status": "healthy",
    "service": "privahub"
  }
}
```

### GET /api/v1alpha1/env

获取环境信息。

**响应**:
```json
{
  "status": {"code": 0, "msg": "success"},
  "data": {
    "platform_type": "center",
    "node_id": "kuscia-system",
    "inst_id": "",
    "deploy_mode": "center",
    "is_center": true,
    "is_autonomy": false,
    "is_p2p_edge": false
  }
}
```

### GET /metrics

Prometheus 指标端点。
