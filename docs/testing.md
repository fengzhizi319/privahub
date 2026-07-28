# 测试指南

## 1. 测试概览

| 指标 | 数值 |
|------|------|
| 测试文件 | 5 |
| 测试用例 | 55 |
| 覆盖包 | internal/service, pkg/kuscia, internal/controller/http/middleware |

## 2. 运行测试

### 2.1 全部测试

```bash
# 使用 Makefile
make test

# 或使用 go test
TMPDIR=$(pwd)/.tmp CGO_ENABLED=1 go test ./... -v
```

### 2.2 指定包测试

```bash
# Service 层测试
go test ./internal/service/... -v

# Kuscia 客户端测试
go test ./pkg/kuscia/... -v

# 中间件测试
go test ./internal/controller/http/middleware/... -v
```

### 2.3 指定测试函数

```bash
# 运行单个测试
go test ./internal/service/... -run TestAuthService -v

# 运行匹配的测试
go test ./internal/service/... -run "Test.*Service" -v
```

### 2.4 覆盖率报告

```bash
# 生成覆盖率
go test ./... -coverprofile=coverage.out

# 查看覆盖率
go tool cover -html=coverage.out -o coverage.html

# 命令行查看
go tool cover -func=coverage.out
```

## 3. 测试文件说明

### 3.1 Service 层测试

| 文件 | 说明 |
|------|------|
| `service_test.go` | 核心 Service 测试 (Auth/Project/Graph/Job) |
| `service_ext_test.go` | 扩展 Service 测试 (Node/Datatable/Vote) |
| `service_new_test.go` | 新增 Service 测试 (Env/DataDir/SSE/...) |

### 3.2 中间件测试

| 文件 | 说明 |
|------|------|
| `middleware_test.go` | AuditLog/IPWhitelist/RateLimiter/BodyLimit 测试 |

### 3.3 Kuscia 客户端测试

| 文件 | 说明 |
|------|------|
| `client_test.go` | Kuscia HTTP 客户端测试 |

## 4. 测试模式

### 4.1 内存数据库测试

Service 层测试使用 SQLite 内存数据库：

```go
func setupTestDB(t *testing.T) *gorm.DB {
    db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
    if err != nil {
        t.Fatalf("failed to open test db: %v", err)
    }
    // Auto migrate
    db.AutoMigrate(&model.UserDO{}, &model.ProjectDO{}, ...)
    return db
}
```

### 4.2 HTTP 测试

中间件测试使用 `httptest`：

```go
func TestRateLimiter_AllowsBurst(t *testing.T) {
    r := gin.New()
    r.Use(RateLimiter(5, 1))
    r.GET("/test", func(c *gin.Context) {
        c.JSON(200, gin.H{"ok": true})
    })

    w := httptest.NewRecorder()
    req, _ := http.NewRequest("GET", "/test", nil)
    req.RemoteAddr = "10.0.0.1:12345"
    r.ServeHTTP(w, req)

    if w.Code != 200 {
        t.Fatalf("expected 200, got %d", w.Code)
    }
}
```

### 4.3 Mock Kuscia 客户端

Kuscia 客户端测试使用 httptest.Server：

```go
func TestKusciaClient_Ping(t *testing.T) {
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "application/json")
        json.NewEncoder(w).Encode(map[string]interface{}{
            "status": map[string]interface{}{"code": 0, "msg": "success"},
        })
    }))
    defer server.Close()

    client := kuscia.NewClient(server.URL, "notls")
    err := client.Ping(context.Background())
    if err != nil {
        t.Fatalf("ping failed: %v", err)
    }
}
```

## 5. 测试用例清单

### 5.1 Auth Service

- `TestAuthService_LoginSuccess` - 登录成功
- `TestAuthService_LoginWrongPassword` - 密码错误
- `TestAuthService_LoginUserNotFound` - 用户不存在
- `TestAuthService_TokenValidate` - Token 验证

### 5.2 Project Service

- `TestProjectService_CreateAndList` - 创建和列表
- `TestProjectService_GetDetail` - 获取详情
- `TestProjectService_Update` - 更新项目
- `TestProjectService_Delete` - 删除项目

### 5.3 Graph Service

- `TestGraphService_CreateAndList` - 创建和列表
- `TestGraphService_UpdateNodes` - 更新节点
- `TestGraphService_Delete` - 删除图

### 5.4 Job Service

- `TestJobService_CreateAndList` - 创建和列表
- `TestJobService_GetDetail` - 获取详情
- `TestJobService_Stop` - 停止任务

### 5.5 Node Service

- `TestNodeService_CreateAndList` - 创建和列表
- `TestNodeService_GetDetail` - 获取详情
- `TestNodeService_Delete` - 删除节点

### 5.6 Datatable Service

- `TestDatatableService_RegisterAndList` - 注册和列表
- `TestDatatableService_GetDetail` - 获取详情
- `TestDatatableService_Delete` - 删除数据表

### 5.7 Vote Service

- `TestVoteService_CreateAndList` - 创建和列表
- `TestVoteService_Reply` - 回复投票

### 5.8 User Service

- `TestUserService_CreateAndList` - 创建和列表
- `TestUserService_ResetPassword` - 重置密码

### 5.9 Feature Table Service

- `TestFeatureTableService_CreateAndList` - 创建和列表
- `TestFeatureTableService_ProjectList` - 项目列表

### 5.10 Graph Datasource Service

- `TestGraphDatasourceService_BindAndList` - 绑定和列表
- `TestGraphDatasourceService_Unbind` - 解绑

### 5.11 Edge Data Sync Service

- `TestEdgeDataSyncService_UpsertAndGet` - 更新和获取

### 5.12 Partition Rule Service

- `TestPartitionRuleService_MaxPtReplacement` - MaxPt 替换
- `TestPartitionRuleService_DateReplacement` - 日期替换
- `TestPartitionRuleService_NonODPSReturnsEmpty` - 非 ODPS 返回空
- `TestPartitionRuleService_ParenthesesRejected` - 括号拒绝
- `TestPartitionRuleService_InvalidColumn` - 无效列名

### 5.13 OSS Service

- `TestOssService_ValidateEndpoint` - Endpoint 校验
- `TestOssService_EmptyParams` - 空参数

### 5.14 SSE Server

- `TestSseServer_OpenCloseAndCount` - 连接管理

### 5.15 Env Service

- `TestEnvService_PlatformDetection` - 平台检测
- `TestEnvService_EmbeddedNodes` - 嵌入节点
- `TestEnvService_FindLocalNodeId` - 查找本地节点
- `TestNormalizePlatformType` - 平台类型标准化

### 5.16 Data Directory Service

- `TestDataDirectoryService_ListFilesEmpty` - 空目录列表
- `TestDataDirectoryService_PathTraversalBlocked` - 路径遍历防护
- `TestDataDirectoryService_EnsureNodeDir` - 创建节点目录

### 5.17 Middleware

- `TestAuditLog_WriteOperation` - 写操作审计
- `TestAuditLog_ReadOperationSkipped` - 读操作跳过
- `TestIPWhitelist_Allowed` - IP 允许
- `TestIPWhitelist_Denied` - IP 拒绝
- `TestIPWhitelist_EmptyAllowsAll` - 空白名单允许所有
- `TestIPWhitelist_SingleIP` - 单 IP 匹配
- `TestRateLimiter_AllowsBurst` - 突发允许
- `TestRateLimiter_DifferentIPsIndependent` - IP 独立限流
- `TestBodyLimit_RejectsLargeBody` - 大请求拒绝
- `TestBodyLimit_AllowsSmallBody` - 小请求允许

### 5.18 Kuscia Client

- `TestKusciaClient_Ping` - 连通性测试
- `TestKusciaClient_CreateDomain` - 创建 Domain
- `TestKusciaClient_QueryJob` - 查询 Job

## 6. 编写测试指南

### 6.1 测试命名规范

```go
func Test<ServiceName>_<MethodName>_<Scenario>(t *testing.T)
```

示例:
- `TestAuthService_Login_Success`
- `TestProjectService_Create_DuplicateName`

### 6.2 表驱动测试

```go
func TestNormalizePlatformType(t *testing.T) {
    cases := map[string]PlatformType{
        "center":   PlatformCenter,
        "master":   PlatformCenter,
        "autonomy": PlatformAutonomy,
        "lite":     PlatformLite,
        "edge":     PlatformP2PEdge,
    }
    for input, expected := range cases {
        if got := NormalizePlatformType(input); got != expected {
            t.Errorf("NormalizePlatformType(%q) = %q, want %q", input, got, expected)
        }
    }
}
```

### 6.3 测试辅助函数

```go
func setupTestDB(t *testing.T) *gorm.DB {
    t.Helper()
    db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
    if err != nil {
        t.Fatalf("failed to open test db: %v", err)
    }
    return db
}

func createTestUser(t *testing.T, db *gorm.DB, name string) *model.UserDO {
    t.Helper()
    user := &model.UserDO{Name: name, Password: "hashed"}
    if err := db.Create(user).Error; err != nil {
        t.Fatalf("failed to create test user: %v", err)
    }
    return user
}
```

## 7. CI 集成

### 7.1 GitHub Actions

```yaml
name: Test

on: [push, pull_request]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
    - uses: actions/checkout@v4
    - uses: actions/setup-go@v5
      with:
        go-version: '1.22'
    - name: Run tests
      run: |
        cd privahub
        CGO_ENABLED=1 go test ./... -v -coverprofile=coverage.out
    - name: Upload coverage
      uses: codecov/codecov-action@v4
      with:
        file: privahub/coverage.out
```

### 7.2 本地 Pre-commit Hook

```bash
#!/bin/bash
# .git/hooks/pre-commit

cd privahub
echo "Running tests..."
CGO_ENABLED=1 go test ./... -short
if [ $? -ne 0 ]; then
    echo "Tests failed. Commit aborted."
    exit 1
fi
```

## 8. 测试最佳实践

1. **独立性**: 每个测试独立运行，不依赖其他测试
2. **可重复**: 测试结果可重复，不依赖外部状态
3. **快速**: 单元测试应在秒级完成
4. **覆盖边界**: 测试正常路径和异常路径
5. **清理资源**: 使用 `t.Cleanup()` 清理临时资源
6. **并行安全**: 使用 `t.Parallel()` 时注意资源共享
