package service

import (
	"context"
	"testing"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// setupBugfix8TestDB creates an in-memory SQLite database for bug fix 59-61 tests.
func setupBugfix8TestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	if err := db.AutoMigrate(
		&model.UserAccountsDO{},
		&model.SysUserNodeRelDO{},
		&model.SysUserPermissionRelDO{},
		&model.NodeDO{},
		&model.ProjectGraphDO{},
		&model.ProjectGraphNodeDO{},
		&model.ProjectJobDO{},
		&model.ProjectJobTaskDO{},
		&model.ProjectJobTaskLogDO{},
	); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}
	return db
}

// --- Bug 59: NodeUserService.Create Count error propagation ---

func TestNodeUserService_Create_Success(t *testing.T) {
	db := setupBugfix8TestDB(t)
	// Java NodeUserServiceImpl: the node must exist.
	db.Create(&model.NodeDO{NodeID: "alice", Name: "alice", ControlNodeID: "alice"})
	db.Create(&model.NodeDO{NodeID: "bob", Name: "bob", ControlNodeID: "bob"})
	svc := NewNodeUserService(db)
	ctx := context.Background()

	err := svc.Create(ctx, &NodeUserCreateRequest{
		NodeID:   "alice",
		UserName: "testuser",
		Password: "secret123",
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	// Verify user was created
	var count int64
	db.Model(&model.UserAccountsDO{}).Where("name = ? AND owner_type = ? AND owner_id = ?", "testuser", "EDGE", "alice").Count(&count)
	if count != 1 {
		t.Fatalf("expected 1 user, got %d", count)
	}

	// Verify relationship was created
	var relCount int64
	db.Model(&model.SysUserNodeRelDO{}).Where("user_id = ? AND node_id = ?", "testuser", "alice").Count(&relCount)
	if relCount != 1 {
		t.Fatalf("expected 1 relationship, got %d", relCount)
	}
}

func TestNodeUserService_Create_DuplicateReturnsError(t *testing.T) {
	db := setupBugfix8TestDB(t)
	// Java NodeUserServiceImpl: the node must exist.
	db.Create(&model.NodeDO{NodeID: "alice", Name: "alice", ControlNodeID: "alice"})
	db.Create(&model.NodeDO{NodeID: "bob", Name: "bob", ControlNodeID: "bob"})
	svc := NewNodeUserService(db)
	ctx := context.Background()

	// Create first user
	err := svc.Create(ctx, &NodeUserCreateRequest{
		NodeID:   "alice",
		UserName: "testuser",
		Password: "secret123",
	})
	if err != nil {
		t.Fatalf("first create failed: %v", err)
	}

	// Attempt duplicate
	err = svc.Create(ctx, &NodeUserCreateRequest{
		NodeID:   "alice",
		UserName: "testuser",
		Password: "other456",
	})
	if err != ErrNodeUserExists {
		t.Fatalf("expected ErrNodeUserExists, got: %v", err)
	}
}

func TestNodeUserService_Create_AltFieldsFallback(t *testing.T) {
	db := setupBugfix8TestDB(t)
	// Java NodeUserServiceImpl: the node must exist.
	db.Create(&model.NodeDO{NodeID: "alice", Name: "alice", ControlNodeID: "alice"})
	db.Create(&model.NodeDO{NodeID: "bob", Name: "bob", ControlNodeID: "bob"})
	svc := NewNodeUserService(db)
	ctx := context.Background()

	// Use Alt fields (camelCase JSON)
	err := svc.Create(ctx, &NodeUserCreateRequest{
		NodeIDAlt:   "bob",
		UserNameAlt: "altuser",
		Password:    "pass789",
	})
	if err != nil {
		t.Fatalf("expected no error with alt fields, got: %v", err)
	}

	var user model.UserAccountsDO
	if err := db.Where("name = ? AND owner_id = ?", "altuser", "bob").First(&user).Error; err != nil {
		t.Fatalf("user not found with alt fields: %v", err)
	}
}

// --- Bug 60: GraphService.StartGraph FindByGraphID error propagation ---

// --- Bug 61: GraphService.StopGraph task error propagation ---
