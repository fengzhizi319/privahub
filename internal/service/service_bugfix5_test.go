package service

import (
	"context"
	"testing"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/internal/dao/repository"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// setupBugfix5TestDB creates an in-memory SQLite database for bug fix 37-43 tests.
func setupBugfix5TestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	if err := db.AutoMigrate(
		&model.FeatureTableDO{},
		&model.ProjectFeatureTableDO{},
		&model.UserAccountsDO{},
		&model.SysUserPermissionRelDO{},
		&model.SysUserNodeRelDO{},
		&model.ProjectDO{},
		&model.ProjectInstDO{},
		&model.ProjectNodeDO{},
		&model.ProjectDatatableDO{},
		&model.EdgeDataSyncLogDO{},
	); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}
	return db
}

// --- Bug 37: FeatureTableService.CreateFeatureTable atomicity ---

func TestFeatureTableService_CreateFeatureTable_Atomic(t *testing.T) {
	db := setupBugfix5TestDB(t)
	svc := NewFeatureTableService(db)

	err := svc.CreateFeatureTable(context.Background(), &CreateFeatureDatasourceRequest{
		NodeID:           "alice",
		FeatureTableName: "test_feature",
		Type:             "HTTP",
		URL:              "http://example.com",
		Columns:          "col1,col2",
		ProjectID:        "proj-001",
	})
	if err != nil {
		t.Fatalf("CreateFeatureTable failed: %v", err)
	}

	// Verify feature table was persisted
	var ft model.FeatureTableDO
	if err := db.Where("feature_table_name = ?", "test_feature").First(&ft).Error; err != nil {
		t.Fatalf("feature table not found in DB: %v", err)
	}
	if ft.Status != "Available" {
		t.Errorf("expected status Available, got %q", ft.Status)
	}

	// Verify project association was persisted atomically
	var pft model.ProjectFeatureTableDO
	if err := db.Where("project_id = ? AND feature_table_id = ?", "proj-001", ft.FeatureTableID).First(&pft).Error; err != nil {
		t.Fatalf("project feature table association not found: %v", err)
	}
	if pft.NodeID != "alice" {
		t.Errorf("expected node_id alice, got %q", pft.NodeID)
	}
}

func TestFeatureTableService_CreateFeatureTable_NoProject(t *testing.T) {
	db := setupBugfix5TestDB(t)
	svc := NewFeatureTableService(db)

	err := svc.CreateFeatureTable(context.Background(), &CreateFeatureDatasourceRequest{
		NodeID:           "bob",
		FeatureTableName: "standalone_feature",
	})
	if err != nil {
		t.Fatalf("CreateFeatureTable failed: %v", err)
	}

	// Verify feature table was persisted
	var count int64
	db.Model(&model.FeatureTableDO{}).Where("feature_table_name = ?", "standalone_feature").Count(&count)
	if count != 1 {
		t.Fatalf("expected 1 feature table, got %d", count)
	}

	// Verify no project association was created
	db.Model(&model.ProjectFeatureTableDO{}).Count(&count)
	if count != 0 {
		t.Fatalf("expected 0 project associations, got %d", count)
	}
}

// --- Bug 38: UserService.CreateUser atomicity ---

func TestUserService_CreateUser_Atomic(t *testing.T) {
	db := setupBugfix5TestDB(t)
	svc := NewUserService(
		repository.NewUserAccountsRepo(db),
		repository.NewSysUserPermissionRepo(db),
		repository.NewSysUserNodeRepo(db),
		db,
	)

	vo, err := svc.CreateUser(context.Background(), &CreateUserRequest{
		Name:      "testuser",
		Password:  "password123",
		RoleCodes: []string{"ADMIN", "USER"},
		NodeIDs:   []string{"alice", "bob"},
	})
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}
	if vo.Name != "testuser" {
		t.Errorf("expected name testuser, got %q", vo.Name)
	}

	// Verify user was persisted
	var user model.UserAccountsDO
	if err := db.Where("name = ?", "testuser").First(&user).Error; err != nil {
		t.Fatalf("user not found in DB: %v", err)
	}

	// Verify roles were persisted atomically
	var perms []model.SysUserPermissionRelDO
	db.Where("user_key = ?", "testuser").Find(&perms)
	if len(perms) != 2 {
		t.Fatalf("expected 2 permissions, got %d", len(perms))
	}

	// Verify nodes were persisted atomically
	var nodes []model.SysUserNodeRelDO
	db.Where("user_id = ?", "testuser").Find(&nodes)
	if len(nodes) != 2 {
		t.Fatalf("expected 2 node relations, got %d", len(nodes))
	}
}

func TestUserService_CreateUser_Duplicate(t *testing.T) {
	db := setupBugfix5TestDB(t)
	svc := NewUserService(
		repository.NewUserAccountsRepo(db),
		repository.NewSysUserPermissionRepo(db),
		repository.NewSysUserNodeRepo(db),
		db,
	)

	_, err := svc.CreateUser(context.Background(), &CreateUserRequest{
		Name:     "dupuser",
		Password: "pass1",
	})
	if err != nil {
		t.Fatalf("first CreateUser failed: %v", err)
	}

	_, err = svc.CreateUser(context.Background(), &CreateUserRequest{
		Name:     "dupuser",
		Password: "pass2",
	})
	if err != ErrUserAlreadyExists {
		t.Fatalf("expected ErrUserAlreadyExists, got %v", err)
	}
}

// --- Bug 39: UserService.DeleteUser atomicity ---

func TestUserService_DeleteUser_Atomic(t *testing.T) {
	db := setupBugfix5TestDB(t)
	svc := NewUserService(
		repository.NewUserAccountsRepo(db),
		repository.NewSysUserPermissionRepo(db),
		repository.NewSysUserNodeRepo(db),
		db,
	)

	// Create user with roles and nodes
	_, err := svc.CreateUser(context.Background(), &CreateUserRequest{
		Name:      "deleteuser",
		Password:  "password",
		RoleCodes: []string{"ADMIN"},
		NodeIDs:   []string{"alice"},
	})
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	// Delete user
	if err := svc.DeleteUser(context.Background(), "deleteuser"); err != nil {
		t.Fatalf("DeleteUser failed: %v", err)
	}

	// Verify user was deleted
	var count int64
	db.Model(&model.UserAccountsDO{}).Where("name = ?", "deleteuser").Count(&count)
	if count != 0 {
		t.Fatal("user should be deleted")
	}

	// Verify permissions were deleted atomically
	db.Model(&model.SysUserPermissionRelDO{}).Where("user_key = ?", "deleteuser").Count(&count)
	if count != 0 {
		t.Fatal("permissions should be deleted")
	}

	// Verify nodes were deleted atomically
	db.Model(&model.SysUserNodeRelDO{}).Where("user_id = ?", "deleteuser").Count(&count)
	if count != 0 {
		t.Fatal("node relations should be deleted")
	}
}

func TestUserService_DeleteUser_NotFound(t *testing.T) {
	db := setupBugfix5TestDB(t)
	svc := NewUserService(
		repository.NewUserAccountsRepo(db),
		repository.NewSysUserPermissionRepo(db),
		repository.NewSysUserNodeRepo(db),
		db,
	)

	err := svc.DeleteUser(context.Background(), "nonexistent")
	if err != ErrUserNotFound {
		t.Fatalf("expected ErrUserNotFound, got %v", err)
	}
}

// --- Bug 40: ProjectService.CreateProject atomicity ---

func TestProjectService_CreateProject_Atomic(t *testing.T) {
	db := setupBugfix5TestDB(t)
	svc := NewProjectService(
		repository.NewProjectRepo(db),
		repository.NewProjectInstRepo(db),
		repository.NewProjectNodeRepo(db),
		repository.NewDatatableRepo(db),
		db,
		nil, // kusciaClient
	)

	vo, err := svc.CreateProject(context.Background(), &CreateProjectRequest{
		Name:        "Test Project",
		Description: "A test project",
		InstID:      "inst-001",
		NodeIDs:     []string{"alice", "bob"},
	}, "admin")
	if err != nil {
		t.Fatalf("CreateProject failed: %v", err)
	}
	if vo.ProjectID == "" {
		t.Fatal("expected non-empty project ID")
	}

	// Verify project was persisted
	var project model.ProjectDO
	if err := db.Where("project_id = ?", vo.ProjectID).First(&project).Error; err != nil {
		t.Fatalf("project not found in DB: %v", err)
	}
	if project.Name != "Test Project" {
		t.Errorf("expected name 'Test Project', got %q", project.Name)
	}

	// Verify inst association was persisted atomically
	var inst model.ProjectInstDO
	if err := db.Where("project_id = ?", vo.ProjectID).First(&inst).Error; err != nil {
		t.Fatalf("project inst association not found: %v", err)
	}
	if inst.InstID != "inst-001" {
		t.Errorf("expected inst_id inst-001, got %q", inst.InstID)
	}

	// Verify node associations were persisted atomically
	var nodes []model.ProjectNodeDO
	db.Where("project_id = ?", vo.ProjectID).Find(&nodes)
	if len(nodes) != 2 {
		t.Fatalf("expected 2 node associations, got %d", len(nodes))
	}
}

// --- Bug 41: ProjectService.DeleteProject atomicity ---

func TestProjectService_DeleteProject_Atomic(t *testing.T) {
	db := setupBugfix5TestDB(t)
	svc := NewProjectService(
		repository.NewProjectRepo(db),
		repository.NewProjectInstRepo(db),
		repository.NewProjectNodeRepo(db),
		repository.NewDatatableRepo(db),
		db,
		nil,
	)

	// Create project with associations
	vo, err := svc.CreateProject(context.Background(), &CreateProjectRequest{
		Name:    "Delete Me",
		InstID:  "inst-del",
		NodeIDs: []string{"alice"},
	}, "admin")
	if err != nil {
		t.Fatalf("CreateProject failed: %v", err)
	}

	// Add a datatable association
	db.Create(&model.ProjectDatatableDO{
		ProjectID:   vo.ProjectID,
		NodeID:      "alice",
		DatatableID: "dt-001",
	})

	// Delete project
	if err := svc.DeleteProject(context.Background(), vo.ProjectID); err != nil {
		t.Fatalf("DeleteProject failed: %v", err)
	}

	// Verify project was deleted
	var count int64
	db.Model(&model.ProjectDO{}).Where("project_id = ?", vo.ProjectID).Count(&count)
	if count != 0 {
		t.Fatal("project should be deleted")
	}

	// Verify inst association was deleted atomically
	db.Model(&model.ProjectInstDO{}).Where("project_id = ?", vo.ProjectID).Count(&count)
	if count != 0 {
		t.Fatal("project inst association should be deleted")
	}

	// Verify node associations were deleted atomically
	db.Model(&model.ProjectNodeDO{}).Where("project_id = ?", vo.ProjectID).Count(&count)
	if count != 0 {
		t.Fatal("project node associations should be deleted")
	}

	// Verify datatable associations were deleted atomically
	db.Model(&model.ProjectDatatableDO{}).Where("project_id = ?", vo.ProjectID).Count(&count)
	if count != 0 {
		t.Fatal("project datatable associations should be deleted")
	}
}

func TestProjectService_DeleteProject_NotFound(t *testing.T) {
	db := setupBugfix5TestDB(t)
	svc := NewProjectService(
		repository.NewProjectRepo(db),
		repository.NewProjectInstRepo(db),
		repository.NewProjectNodeRepo(db),
		repository.NewDatatableRepo(db),
		db,
		nil,
	)

	err := svc.DeleteProject(context.Background(), "nonexistent")
	if err != ErrProjectNotFound {
		t.Fatalf("expected ErrProjectNotFound, got %v", err)
	}
}

// --- Bug 43: EdgeDataSyncService.UpsertSyncLog race condition fix ---

func TestEdgeDataSyncService_UpsertSyncLog_Create(t *testing.T) {
	db := setupBugfix5TestDB(t)
	svc := NewEdgeDataSyncService(db)

	err := svc.UpsertSyncLog(context.Background(), "test_table")
	if err != nil {
		t.Fatalf("UpsertSyncLog (create) failed: %v", err)
	}

	// Verify log was created
	var log model.EdgeDataSyncLogDO
	if err := db.Where("table_name = ?", "test_table").First(&log).Error; err != nil {
		t.Fatalf("sync log not found: %v", err)
	}
	if log.SyncTableName != "test_table" {
		t.Errorf("expected table_name test_table, got %q", log.SyncTableName)
	}
	if log.LastUpdateTime == "" {
		t.Error("expected non-empty last_update_time")
	}
}

func TestEdgeDataSyncService_UpsertSyncLog_Update(t *testing.T) {
	db := setupBugfix5TestDB(t)
	svc := NewEdgeDataSyncService(db)

	// Create initial log
	if err := svc.UpsertSyncLog(context.Background(), "update_table"); err != nil {
		t.Fatalf("UpsertSyncLog (create) failed: %v", err)
	}

	// Get initial time
	var log1 model.EdgeDataSyncLogDO
	db.Where("table_name = ?", "update_table").First(&log1)

	// Update log
	if err := svc.UpsertSyncLog(context.Background(), "update_table"); err != nil {
		t.Fatalf("UpsertSyncLog (update) failed: %v", err)
	}

	// Verify only one record exists
	var count int64
	db.Model(&model.EdgeDataSyncLogDO{}).Where("table_name = ?", "update_table").Count(&count)
	if count != 1 {
		t.Fatalf("expected 1 sync log, got %d", count)
	}

	// Verify time was updated
	var log2 model.EdgeDataSyncLogDO
	db.Where("table_name = ?", "update_table").First(&log2)
	if log2.LastUpdateTime < log1.LastUpdateTime {
		t.Error("last_update_time should not decrease")
	}
}

func TestEdgeDataSyncService_GetLastSyncTime(t *testing.T) {
	db := setupBugfix5TestDB(t)
	svc := NewEdgeDataSyncService(db)

	// Create log
	if err := svc.UpsertSyncLog(context.Background(), "time_table"); err != nil {
		t.Fatalf("UpsertSyncLog failed: %v", err)
	}

	// Get last sync time
	syncTime, err := svc.GetLastSyncTime(context.Background(), "time_table")
	if err != nil {
		t.Fatalf("GetLastSyncTime failed: %v", err)
	}
	if syncTime == "" {
		t.Error("expected non-empty sync time")
	}
}

func TestEdgeDataSyncService_GetLastSyncTime_NotFound(t *testing.T) {
	db := setupBugfix5TestDB(t)
	svc := NewEdgeDataSyncService(db)

	_, err := svc.GetLastSyncTime(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent table")
	}
}
