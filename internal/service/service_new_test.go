package service

import (
	"context"
	"os"
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
	)
	if err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}
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

	_ = svc.Create(ctx, &NodeUserCreateRequest{
		NodeID:   "bob",
		UserName: "edge_user2",
		Password: "old_pass",
	})

	// Reset password
	err := svc.ResetPassword(ctx, &ResetNodeUserPwdRequest{
		NodeID:   "bob",
		UserName: "edge_user2",
		Password: "new_pass",
	})
	if err != nil {
		t.Fatalf("ResetPassword failed: %v", err)
	}

	// Reset non-existent user
	err = svc.ResetPassword(ctx, &ResetNodeUserPwdRequest{
		NodeID:   "bob",
		UserName: "nonexistent",
		Password: "x",
	})
	if err != ErrNodeUserNotFound {
		t.Fatalf("expected ErrNodeUserNotFound, got: %v", err)
	}
}

// --- FeatureTableService Tests ---

func TestFeatureTableService_CreateAndList(t *testing.T) {
	db := setupNewTestDB(t)
	svc := NewFeatureTableService(db)
	ctx := context.Background()

	err := svc.CreateFeatureTable(ctx, &CreateFeatureDatasourceRequest{
		NodeID:           "alice",
		FeatureTableName: "feature_http",
		URL:              "http://example.com/api",
		Columns:          `[{"name":"col1","type":"string"}]`,
	})
	if err != nil {
		t.Fatalf("CreateFeatureTable failed: %v", err)
	}

	list, err := svc.FeatureDatasourceList(ctx, "alice")
	if err != nil {
		t.Fatalf("FeatureDatasourceList failed: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 feature table, got %d", len(list))
	}
	if list[0].FeatureTableName != "feature_http" {
		t.Fatalf("expected feature_http, got %s", list[0].FeatureTableName)
	}
	if list[0].Status != "Available" {
		t.Fatalf("expected Available, got %s", list[0].Status)
	}
}

func TestFeatureTableService_ProjectList(t *testing.T) {
	db := setupNewTestDB(t)
	svc := NewFeatureTableService(db)
	ctx := context.Background()

	// Create with project association
	err := svc.CreateFeatureTable(ctx, &CreateFeatureDatasourceRequest{
		NodeID:           "alice",
		FeatureTableName: "proj_feature",
		URL:              "http://example.com/proj",
		Columns:          `[{"name":"x","type":"int"}]`,
		ProjectID:        "proj-001",
	})
	if err != nil {
		t.Fatalf("CreateFeatureTable failed: %v", err)
	}

	// Project list should return it
	list, err := svc.ProjectFeatureTableList(ctx, "alice", "proj-001")
	if err != nil {
		t.Fatalf("ProjectFeatureTableList failed: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1, got %d", len(list))
	}

	// Different project should return empty
	list2, err := svc.ProjectFeatureTableList(ctx, "alice", "proj-999")
	if err != nil {
		t.Fatalf("ProjectFeatureTableList failed: %v", err)
	}
	if len(list2) != 0 {
		t.Fatalf("expected 0, got %d", len(list2))
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
	expected := "dt=MAX_PT('my_table')"
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

// --- OssService Tests ---

func TestOssService_ValidateEndpoint(t *testing.T) {
	tests := []struct {
		endpoint string
		wantErr  bool
	}{
		{"s3.amazonaws.com", false},
		{"minio.example.com:9000", false},
		{"localhost", true},
		{"127.0.0.1", true},
		{"169.254.169.254", true},
		{"10.0.0.1", true},
		{"192.168.1.1", true},
		{"metadata.google.internal", true},
	}

	for _, tt := range tests {
		err := validateEndpoint(tt.endpoint)
		if (err != nil) != tt.wantErr {
			t.Errorf("validateEndpoint(%q) error = %v, wantErr %v", tt.endpoint, err, tt.wantErr)
		}
	}
}

func TestOssService_EmptyParams(t *testing.T) {
	svc := NewOssService()
	ctx := context.Background()

	err := svc.CheckBucketExists(ctx, &OssConfig{}, "bucket")
	if err == nil {
		t.Fatal("expected error for empty endpoint")
	}

	_, err = svc.CheckObjectExists(ctx, &OssConfig{Endpoint: "s3.example.com"}, "", "key")
	if err == nil {
		t.Fatal("expected error for empty bucket")
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
	svc := NewEnvService("autonomy", "node-a", "inst-1", nil)
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
	if svc.IsNodeInCurrentInst("node-x") {
		t.Fatal("expected node-x NOT in current inst")
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
		"edge":     PlatformP2PEdge,
		"p2p_edge": PlatformP2PEdge,
		"unknown":  PlatformCenter,
	}
	for input, expected := range cases {
		if got := NormalizePlatformType(input); got != expected {
			t.Errorf("NormalizePlatformType(%q) = %q, want %q", input, got, expected)
		}
	}
}

// --- DataDirectoryService Tests ---

func TestDataDirectoryService_ListFilesEmpty(t *testing.T) {
	svc := NewDataDirectoryService("/tmp/nonexistent-dir-test")
	files, err := svc.ListFiles(context.Background(), "node-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(files) != 0 {
		t.Fatalf("expected 0 files, got %d", len(files))
	}
}

func TestDataDirectoryService_PathTraversalBlocked(t *testing.T) {
	svc := NewDataDirectoryService("/tmp")

	if svc.FileExists("..", "etc/passwd") {
		t.Fatal("path traversal in nodeID should be blocked")
	}
	if svc.FileExists("node1", "../../etc/passwd") {
		t.Fatal("path traversal in fileName should be blocked")
	}
	if svc.GetFilePath("node1", "../secret.txt") != "" {
		t.Fatal("GetFilePath should return empty for traversal")
	}
}

func TestDataDirectoryService_EnsureNodeDir(t *testing.T) {
	tmpDir := t.TempDir()
	svc := NewDataDirectoryService(tmpDir)

	dir, err := svc.EnsureNodeDir("test-node")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dir == "" {
		t.Fatal("expected non-empty dir path")
	}

	// Verify directory exists
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("directory should exist: %v", err)
	}
	if !info.IsDir() {
		t.Fatal("expected a directory")
	}
}
