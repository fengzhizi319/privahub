package service

import (
	"context"
	"sync"
	"testing"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/internal/dao/repository"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupBugfix2TestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	err = db.AutoMigrate(
		&model.UserAccountsDO{},
		&model.SysUserPermissionRelDO{},
		&model.SysUserNodeRelDO{},
		&model.FeatureTableDO{},
		&model.ProjectFeatureTableDO{},
		&model.ProjectScheduleTaskDO{},
		&model.NodeDO{},
		&model.UserTokensDO{},
	)
	if err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}
	if !db.Migrator().HasTable(&model.SysUserPermissionRelDO{}) {
		_ = db.AutoMigrate(&model.SysUserPermissionRelDO{})
	}
	// Node users require an existing node (Java NodeUserServiceImpl).
	seedNodeUserTestNodes(db)
	return db
}

// --- Bug 20: NodeUserService.Create atomicity ---

func TestNodeUserService_Create_Atomic(t *testing.T) {
	db := setupBugfix2TestDB(t)
	svc := NewNodeUserService(db)

	ctx := context.Background()
	err := svc.Create(ctx, &NodeUserCreateRequest{
		NodeID:   "alice",
		UserName: "testuser",
		Password: "secret123",
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Verify user account was created
	var user model.UserAccountsDO
	if err := db.Where("name = ? AND owner_type = ? AND owner_id = ?", "testuser", "EDGE", "alice").
		First(&user).Error; err != nil {
		t.Fatalf("user account not found: %v", err)
	}

	// Verify user-node relationship was created
	var rel model.SysUserNodeRelDO
	if err := db.Where("user_id = ? AND node_id = ?", "testuser", "alice").
		First(&rel).Error; err != nil {
		t.Fatalf("user-node relation not found: %v", err)
	}
}

func TestNodeUserService_Create_DuplicateRejected(t *testing.T) {
	db := setupBugfix2TestDB(t)
	svc := NewNodeUserService(db)

	ctx := context.Background()
	req := &NodeUserCreateRequest{
		NodeID:   "bob",
		UserName: "dupuser",
		Password: "pass1",
	}
	if err := svc.Create(ctx, req); err != nil {
		t.Fatalf("first Create failed: %v", err)
	}

	// Second create with same user+node should fail
	err := svc.Create(ctx, &NodeUserCreateRequest{
		NodeID:   "bob",
		UserName: "dupuser",
		Password: "pass2",
	})
	if err != ErrNodeUserExists {
		t.Fatalf("expected ErrNodeUserExists, got: %v", err)
	}
}

// --- Bug 21: UserService.UpdateUser atomicity ---

func TestUserService_UpdateUser_TransactionalRolesAndNodes(t *testing.T) {
	db := setupBugfix2TestDB(t)

	userRepo := repository.NewUserAccountsRepo(db)
	permRepo := repository.NewSysUserPermissionRepo(db)
	nodeRepo := repository.NewSysUserNodeRepo(db)
	svc := NewUserService(userRepo, permRepo, nodeRepo, db)

	ctx := context.Background()

	// Create initial user
	_, err := svc.CreateUser(ctx, &CreateUserRequest{
		Name:      "alice",
		Password:  "password",
		RoleCodes: []string{"ADMIN"},
		NodeIDs:   []string{"node1"},
	})
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	// Update roles and nodes
	err = svc.UpdateUser(ctx, &UpdateUserRequest{
		Name:      "alice",
		RoleCodes: []string{"OPERATOR", "VIEWER"},
		NodeIDs:   []string{"node2", "node3"},
	})
	if err != nil {
		t.Fatalf("UpdateUser failed: %v", err)
	}

	// Verify old roles are gone, new roles exist
	perms, err := permRepo.FindByUserKey(ctx, "alice")
	if err != nil {
		t.Fatalf("FindByUserKey failed: %v", err)
	}
	if len(perms) != 2 {
		t.Fatalf("expected 2 permissions, got %d", len(perms))
	}
	roleSet := make(map[string]bool)
	for _, p := range perms {
		roleSet[p.TargetCode] = true
	}
	if !roleSet["OPERATOR"] || !roleSet["VIEWER"] {
		t.Errorf("unexpected roles: %v", roleSet)
	}

	// Verify old nodes are gone, new nodes exist
	nodes, err := nodeRepo.FindByUserID(ctx, "alice")
	if err != nil {
		t.Fatalf("FindByUserID failed: %v", err)
	}
	if len(nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(nodes))
	}
	nodeSet := make(map[string]bool)
	for _, n := range nodes {
		nodeSet[n.NodeID] = true
	}
	if !nodeSet["node2"] || !nodeSet["node3"] {
		t.Errorf("unexpected nodes: %v", nodeSet)
	}
}

// --- Bug 23: SseServer data race (run with -race) ---

func TestSseServer_ConcurrentSendNoRace(t *testing.T) {
	sse := NewSseServer(nil)
	defer sse.Stop()

	// This test primarily verifies no data race occurs when running with -race flag.
	// We exercise the mutex-protected LastActive field via concurrent Ping calls.
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				sse.Ping()
				_ = sse.ActiveConnections()
			}
		}()
	}
	wg.Wait()
}

// --- Bug 24: FeatureTableService.CreateFeatureTable error propagation ---

// --- Bug 25: RBAC least privilege (tested via middleware package) ---
// The RBAC fix is in the middleware package; here we verify the service-level
// user creation correctly assigns roles so RBAC can enforce them.

func TestUserService_CreateUser_RoleAssignment(t *testing.T) {
	db := setupBugfix2TestDB(t)

	userRepo := repository.NewUserAccountsRepo(db)
	permRepo := repository.NewSysUserPermissionRepo(db)
	nodeRepo := repository.NewSysUserNodeRepo(db)
	svc := NewUserService(userRepo, permRepo, nodeRepo, db)

	ctx := context.Background()
	vo, err := svc.CreateUser(ctx, &CreateUserRequest{
		Name:      "operator1",
		Password:  "pass123",
		RoleCodes: []string{"OPERATOR"},
		NodeIDs:   []string{"alice"},
	})
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	if vo.Name != "operator1" {
		t.Errorf("expected name operator1, got %s", vo.Name)
	}
	if len(vo.RoleCodes) != 1 || vo.RoleCodes[0] != "OPERATOR" {
		t.Errorf("expected [OPERATOR], got %v", vo.RoleCodes)
	}
	if len(vo.NodeIDs) != 1 || vo.NodeIDs[0] != "alice" {
		t.Errorf("expected [alice], got %v", vo.NodeIDs)
	}

	// Verify permission record exists in DB (needed for RBAC)
	var perm model.SysUserPermissionRelDO
	if err := db.Where("user_key = ? AND target_code = ?", "operator1", "OPERATOR").
		First(&perm).Error; err != nil {
		t.Fatalf("permission record not found: %v", err)
	}
}

// --- Additional: NodeUserService ListByNodeId ---

func TestNodeUserService_ListByNodeId(t *testing.T) {
	db := setupBugfix2TestDB(t)
	svc := NewNodeUserService(db)

	ctx := context.Background()
	// Create two users for same node
	for _, name := range []string{"user1", "user2"} {
		err := svc.Create(ctx, &NodeUserCreateRequest{
			NodeID:   "alice",
			UserName: name,
			Password: "pass",
		})
		if err != nil {
			t.Fatalf("Create(%s) failed: %v", name, err)
		}
	}

	list, err := svc.ListByNodeId(ctx, &NodeUserListRequest{NodeID: "alice"})
	if err != nil {
		t.Fatalf("ListByNodeId failed: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 users, got %d", len(list))
	}
}

// --- Additional: EnvService concurrency safety ---

func TestEnvService_ConcurrentAccess(t *testing.T) {
	svc := NewEnvService("center", "node1", "inst1", nil)

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = svc.GetPlatformType()
			_ = svc.IsCenter()
			_ = svc.GetPlatformNodeId()
			_ = svc.IsNodeInCurrentInst("node1")
			_ = svc.FindLocalNodeId([]string{"node1", "node2"})
			svc.SetEmbeddedNodes([]string{"node2"})
		}()
	}
	wg.Wait()

	if !svc.IsCenter() {
		t.Error("expected center platform")
	}
}

// --- Additional: GraphDatasourceService ---

func TestGraphDatasourceService_BindAndGet(t *testing.T) {
	db := setupBugfix2TestDB(t)
	// Need to add the ProjectGraphDomainDatasourceDO table
	db.AutoMigrate(&model.ProjectGraphDomainDatasourceDO{})

	svc := NewGraphDatasourceService(db)
	ctx := context.Background()

	// Bind
	err := svc.Bind(ctx, &GraphDatasourceBindRequest{
		ProjectID:    "proj1",
		GraphID:      "graph1",
		DomainID:     "alice",
		DatasourceID: "ds-001",
	})
	if err != nil {
		t.Fatalf("Bind failed: %v", err)
	}

	// Get
	vo, err := svc.Get(ctx, "proj1", "graph1", "alice")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if vo.DatasourceID != "ds-001" {
		t.Errorf("expected ds-001, got %s", vo.DatasourceID)
	}

	// Update binding
	err = svc.Bind(ctx, &GraphDatasourceBindRequest{
		ProjectID:    "proj1",
		GraphID:      "graph1",
		DomainID:     "alice",
		DatasourceID: "ds-002",
	})
	if err != nil {
		t.Fatalf("Bind update failed: %v", err)
	}
	vo, _ = svc.Get(ctx, "proj1", "graph1", "alice")
	if vo.DatasourceID != "ds-002" {
		t.Errorf("expected ds-002 after update, got %s", vo.DatasourceID)
	}

	// Unbind
	if err := svc.Unbind(ctx, "proj1", "graph1", "alice"); err != nil {
		t.Fatalf("Unbind failed: %v", err)
	}
	_, err = svc.Get(ctx, "proj1", "graph1", "alice")
	if err != ErrGraphDatasourceNotFound {
		t.Errorf("expected ErrGraphDatasourceNotFound after unbind, got %v", err)
	}
}

// --- Additional: EnvService FindLocalNodeId ---

func TestEnvService_FindLocalNodeId_Edge(t *testing.T) {
	svc := NewEnvService("autonomy", "node-a", "inst1", nil)
	svc.SetEmbeddedNodes([]string{"node-b"})

	// Should find node-a (platform node)
	if got := svc.FindLocalNodeId([]string{"node-x", "node-a"}); got != "node-a" {
		t.Errorf("expected node-a, got %q", got)
	}
	// Should find node-b (embedded)
	if got := svc.FindLocalNodeId([]string{"node-b", "node-c"}); got != "node-b" {
		t.Errorf("expected node-b, got %q", got)
	}
	// No local node found
	if got := svc.FindLocalNodeId([]string{"node-x", "node-y"}); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}
