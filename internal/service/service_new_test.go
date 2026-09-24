package service

import (
	"context"
	"testing"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupNewTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	err = db.AutoMigrate(
		&model.UserAccountsDO{},
		&model.SysUserNodeRelDO{},
		&model.FeatureTableDO{},
		&model.ProjectFeatureTableDO{},
		&model.ProjectGraphDomainDatasourceDO{},
		&model.EdgeDataSyncLogDO{},
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

// --- NodeUserService Tests ---

func TestNodeUserService_CreateAndList(t *testing.T) {
	db := setupNewTestDB(t)
	svc := NewNodeUserService(db)
	ctx := context.Background()

	// Create a node user
	err := svc.Create(ctx, &NodeUserCreateRequest{
		NodeID:   "alice",
		UserName: "edge_user1",
		Password: "password123",
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Duplicate should fail
	err = svc.Create(ctx, &NodeUserCreateRequest{
		NodeID:   "alice",
		UserName: "edge_user1",
		Password: "password456",
	})
	if err != ErrNodeUserExists {
		t.Fatalf("expected ErrNodeUserExists, got: %v", err)
	}

	// List users for node
	users, err := svc.ListByNodeId(ctx, &NodeUserListRequest{NodeID: "alice"})
	if err != nil {
		t.Fatalf("ListByNodeId failed: %v", err)
	}
	if len(users) != 1 {
		t.Fatalf("expected 1 user, got %d", len(users))
	}
	if users[0].Name != "edge_user1" {
		t.Fatalf("expected edge_user1, got %s", users[0].Name)
	}
}

func TestNodeUserService_ResetPassword(t *testing.T) {
	db := setupNewTestDB(t)
	svc := NewNodeUserService(db)
	ctx := context.Background()

	if err := svc.Create(ctx, &NodeUserCreateRequest{
		NodeID:       "bob",
		Name:         "edge_user2",
		PasswordHash: HashPassword("old_pass"),
	}); err != nil {
		t.Fatal(err)
	}

	// Wrong old password is rejected (Java: "wrong password").
	err := svc.ResetPassword(ctx, &ResetNodeUserPwdRequest{
		NodeID: "bob", Name: "edge_user2",
		PasswordHash: HashPassword("bad"), NewPasswordHash: HashPassword("new_pass"),
	})
	if err != ErrNodeUserWrongPwd {
		t.Fatalf("expected ErrNodeUserWrongPwd, got %v", err)
	}
	// Account not belonging to nodeId is rejected.
	err = svc.ResetPassword(ctx, &ResetNodeUserPwdRequest{
		NodeID: "alice", Name: "edge_user2",
		PasswordHash: HashPassword("old_pass"), NewPasswordHash: HashPassword("new_pass"),
	})
	if err != ErrNodeUserNotFound {
		t.Fatalf("expected ErrNodeUserNotFound for foreign node, got %v", err)
	}
	// Correct reset.
	if err := svc.ResetPassword(ctx, &ResetNodeUserPwdRequest{
		NodeID: "bob", Name: "edge_user2",
		PasswordHash: HashPassword("old_pass"), NewPasswordHash: HashPassword("new_pass"),
	}); err != nil {
		t.Fatalf("ResetPassword failed: %v", err)
	}
	var u model.UserAccountsDO
	db.Where("name = ?", "edge_user2").First(&u)
	if u.PasswordHash != HashPassword("new_pass") {
		t.Error("password not updated")
	}

	// Reset non-existent user
	err = svc.ResetPassword(ctx, &ResetNodeUserPwdRequest{
		NodeID: "bob", Name: "nonexistent", PasswordHash: "x", NewPasswordHash: "y",
	})
	if err != ErrNodeUserNotFound {
		t.Fatalf("expected ErrNodeUserNotFound, got: %v", err)
	}
}

// --- GraphDatasourceService Tests ---

func TestGraphDatasourceService_BindAndList(t *testing.T) {
	db := setupNewTestDB(t)
	svc := NewGraphDatasourceService(db)
	ctx := context.Background()

	// Bind
	err := svc.Bind(ctx, &GraphDatasourceBindRequest{
		ProjectID:    "proj-1",
		GraphID:      "graph-1",
		DomainID:     "alice",
		DatasourceID: "ds-001",
	})
	if err != nil {
		t.Fatalf("Bind failed: %v", err)
	}

	// Get
	vo, err := svc.Get(ctx, "proj-1", "graph-1", "alice")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if vo.DatasourceID != "ds-001" {
		t.Fatalf("expected ds-001, got %s", vo.DatasourceID)
	}

	// Update binding (re-bind with different datasource)
	err = svc.Bind(ctx, &GraphDatasourceBindRequest{
		ProjectID:    "proj-1",
		GraphID:      "graph-1",
		DomainID:     "alice",
		DatasourceID: "ds-002",
	})
	if err != nil {
		t.Fatalf("Re-bind failed: %v", err)
	}
	vo2, _ := svc.Get(ctx, "proj-1", "graph-1", "alice")
	if vo2.DatasourceID != "ds-002" {
		t.Fatalf("expected ds-002 after update, got %s", vo2.DatasourceID)
	}

	// List by project
	list, err := svc.ListByProject(ctx, "proj-1")
	if err != nil {
		t.Fatalf("ListByProject failed: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1, got %d", len(list))
	}
}

func TestGraphDatasourceService_Unbind(t *testing.T) {
	db := setupNewTestDB(t)
	svc := NewGraphDatasourceService(db)
	ctx := context.Background()

	_ = svc.Bind(ctx, &GraphDatasourceBindRequest{
		ProjectID:    "proj-2",
		GraphID:      "graph-2",
		DomainID:     "bob",
		DatasourceID: "ds-003",
	})

	// Unbind
	err := svc.Unbind(ctx, "proj-2", "graph-2", "bob")
	if err != nil {
		t.Fatalf("Unbind failed: %v", err)
	}

	// Get should fail now
	_, err = svc.Get(ctx, "proj-2", "graph-2", "bob")
	if err != ErrGraphDatasourceNotFound {
		t.Fatalf("expected ErrGraphDatasourceNotFound, got: %v", err)
	}

	// Unbind non-existent
	err = svc.Unbind(ctx, "proj-2", "graph-2", "bob")
	if err != ErrGraphDatasourceNotFound {
		t.Fatalf("expected ErrGraphDatasourceNotFound on double unbind, got: %v", err)
	}
}

// --- EdgeDataSyncService Tests ---

func TestEdgeDataSyncService_UpsertAndGet(t *testing.T) {
	db := setupNewTestDB(t)
	svc := NewEdgeDataSyncService(db)
	ctx := context.Background()

	// Upsert new
	err := svc.UpsertSyncLog(ctx, "project_graph")
	if err != nil {
		t.Fatalf("UpsertSyncLog failed: %v", err)
	}

	// Get logs
	logs, err := svc.GetSyncLogs(ctx)
	if err != nil {
		t.Fatalf("GetSyncLogs failed: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("expected 1 log, got %d", len(logs))
	}
	if logs[0].TableName != "project_graph" {
		t.Fatalf("expected project_graph, got %s", logs[0].TableName)
	}

	// Upsert again (update)
	err = svc.UpsertSyncLog(ctx, "project_graph")
	if err != nil {
		t.Fatalf("UpsertSyncLog update failed: %v", err)
	}
	logs2, _ := svc.GetSyncLogs(ctx)
	if len(logs2) != 1 {
		t.Fatalf("expected still 1 log after upsert, got %d", len(logs2))
	}

	// GetLastSyncTime
	ts, err := svc.GetLastSyncTime(ctx, "project_graph")
	if err != nil {
		t.Fatalf("GetLastSyncTime failed: %v", err)
	}
	if ts == "" {
		t.Fatal("expected non-empty timestamp")
	}
}

// --- PartitionRuleService Tests ---

func TestPartitionRuleService_MaxPtReplacement(t *testing.T) {
	svc := NewPartitionRuleService()

	result, err := svc.ReadPartitionRuleAnalysis(
		"my_table",
		DataSourceTypeODPS,
		"dt=maxpt",
		"20240910",
		map[string]bool{"dt": true},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := "dt=max_pt('my_table')"
	if result != expected {
		t.Fatalf("expected %q, got %q", expected, result)
	}
}

func TestPartitionRuleService_DateReplacement(t *testing.T) {
	svc := NewPartitionRuleService()

	result, err := svc.ReadPartitionRuleAnalysis(
		"test_table",
		DataSourceTypeODPS,
		"dt=${yyyymmdd} and a=${yyyymmdd-1} or b=${yyyymmdd+2}",
		"20240910",
		map[string]bool{"dt": true, "a": true, "b": true},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := "dt='20240910' and a='20240909' or b='20240912'"
	if result != expected {
		t.Fatalf("expected %q, got %q", expected, result)
	}
}

func TestPartitionRuleService_NonODPSReturnsEmpty(t *testing.T) {
	svc := NewPartitionRuleService()

	result, err := svc.ReadPartitionRuleAnalysis(
		"test_table",
		DataSourceTypeOSS,
		"dt=${yyyymmdd}",
		"20240910",
		nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "" {
		t.Fatalf("expected empty for OSS, got %q", result)
	}
}

func TestPartitionRuleService_ParenthesesRejected(t *testing.T) {
	svc := NewPartitionRuleService()

	_, err := svc.ReadPartitionRuleAnalysis(
		"test_table",
		DataSourceTypeODPS,
		"dt in (${yyyymmdd-1},${yyyymmdd+1})",
		"20240910",
		map[string]bool{"dt": true},
	)
	if err == nil {
		t.Fatal("expected error for parentheses, got nil")
	}
}

func TestPartitionRuleService_InvalidColumn(t *testing.T) {
	svc := NewPartitionRuleService()

	_, err := svc.ReadPartitionRuleAnalysis(
		"test_table",
		DataSourceTypeODPS,
		"unknown_col=${yyyymmdd}",
		"20240910",
		map[string]bool{"dt": true, "a": true},
	)
	if err == nil {
		t.Fatal("expected error for invalid column, got nil")
	}
}

// --- SseServer Tests ---

func TestSseServer_OpenCloseAndCount(t *testing.T) {
	svc := NewSseServer(nil)

	if svc.ActiveConnections() != 0 {
		t.Fatalf("expected 0 connections, got %d", svc.ActiveConnections())
	}

	// Send to non-existent node should return false
	if svc.Send("nonexistent", &SyncDataDTO{TableName: "t"}) {
		t.Fatal("expected Send to non-existent node to return false")
	}

	// Broadcast with no connections should return 0
	count := svc.Broadcast(&SyncDataDTO{TableName: "t"})
	if count != 0 {
		t.Fatalf("expected 0 broadcast recipients, got %d", count)
	}

	// Close non-existent should return false
	if svc.Close("nonexistent") {
		t.Fatal("expected Close non-existent to return false")
	}
}

// --- EnvService Tests ---

func TestEnvService_PlatformDetection(t *testing.T) {
	svc := NewEnvService("center", "node-1", "inst-1", nil)

	if !svc.IsCenter() {
		t.Fatal("expected IsCenter() = true")
	}
	if svc.IsAutonomy() {
		t.Fatal("expected IsAutonomy() = false")
	}
	if svc.IsP2pEdge() {
		t.Fatal("expected IsP2pEdge() = false")
	}
	if svc.GetPlatformNodeId() != "node-1" {
		t.Fatalf("expected node-1, got %s", svc.GetPlatformNodeId())
	}
	if !svc.IsCurrentNodeEnvironment("node-1") {
		t.Fatal("expected IsCurrentNodeEnvironment(node-1) = true")
	}
	if svc.IsCurrentNodeEnvironment("node-2") {
		t.Fatal("expected IsCurrentNodeEnvironment(node-2) = false")
	}
}

func TestEnvService_EmbeddedNodes(t *testing.T) {
	// Java: embedded nodes only exist on the CENTER platform.
	svc := NewEnvService("center", "node-a", "inst-1", nil)
	svc.SetEmbeddedNodes([]string{"node-b", "node-c"})

	if !svc.IsEmbeddedNode("node-b") {
		t.Fatal("expected node-b to be embedded")
	}
	if svc.IsEmbeddedNode("node-x") {
		t.Fatal("expected node-x to NOT be embedded")
	}
	if !svc.IsNodeInCurrentInst("node-b") {
		t.Fatal("expected node-b in current inst")
	}
	auto := NewEnvService("autonomy", "node-a", "inst-1", nil)
	auto.SetEmbeddedNodes([]string{"node-b"})
	if auto.IsNodeInCurrentInst("node-x") || !auto.IsNodeInCurrentInst("node-b") {
		t.Fatal("autonomy inst membership wrong")
	}
}

func TestEnvService_FindLocalNodeId(t *testing.T) {
	svc := NewEnvService("autonomy", "node-a", "inst-1", nil)
	svc.SetEmbeddedNodes([]string{"node-b"})

	local := svc.FindLocalNodeId([]string{"node-x", "node-b", "node-y"})
	if local != "node-b" {
		t.Fatalf("expected node-b, got %s", local)
	}

	// Center mode fallback
	centerSvc := NewEnvService("center", "master", "inst-1", nil)
	local2 := centerSvc.FindLocalNodeId([]string{"alice", "bob"})
	if local2 != "alice" {
		t.Fatalf("expected alice (first node in center mode), got %s", local2)
	}
}

func TestNormalizePlatformType(t *testing.T) {
	cases := map[string]PlatformType{
		"center":   PlatformCenter,
		"master":   PlatformCenter,
		"autonomy": PlatformAutonomy,
		"lite":     PlatformLite,
		"edge":     PlatformEdge,
		"p2p_edge": PlatformP2PEdge,
		"unknown":  PlatformCenter,
	}
	for input, expected := range cases {
		if got := NormalizePlatformType(input); got != expected {
			t.Errorf("NormalizePlatformType(%q) = %q, want %q", input, got, expected)
		}
	}
}
