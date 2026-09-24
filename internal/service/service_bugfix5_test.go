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
		Password:  "Str0ng-Pass#1",
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
		Password:  "Str0ng-Pass#2",
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

// --- Bug 41: ProjectService.DeleteProject atomicity ---

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
