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

// setupBugfix4TestDB creates an in-memory SQLite database for bug fix 32-36 tests.
func setupBugfix4TestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	if err := db.AutoMigrate(
		&model.VoteRequestDO{},
		&model.VoteInviteDO{},
		&model.NodeDO{},
		&model.NodeRouteDO{},
	); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}
	return db
}

// --- Bug 36: NodeService.DeleteNode route cleanup ---

func TestNodeService_DeleteNode_CleansRoutes(t *testing.T) {
	db := setupBugfix4TestDB(t)
	svc := NewNodeService(
		repository.NewNodeRepo(db),
		repository.NewNodeRouteRepo(db),
		nil,
		db,
	)

	// Create nodes
	if err := db.AutoMigrate(&model.ProjectNodeDO{}, &model.SysUserPermissionRelDO{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	for _, id := range []string{"alice", "bob", "charlie"} {
		db.Create(&model.NodeDO{NodeID: id, Name: id, ControlNodeID: id, Type: "normal"})
	}

	// Create routes: alice<->bob, alice<->charlie
	_ = svc.CreateRoute(context.Background(), &CreateRouteRequest{SrcNodeID: "alice", DstNodeID: "bob"})
	_ = svc.CreateRoute(context.Background(), &CreateRouteRequest{SrcNodeID: "alice", DstNodeID: "charlie"})

	// Verify routes exist
	var routeCount int64
	db.Model(&model.NodeRouteDO{}).Count(&routeCount)
	if routeCount != 4 { // 2 forward + 2 reverse
		t.Fatalf("expected 4 routes before delete, got %d", routeCount)
	}

	// Delete alice — should remove all routes involving alice
	if err := svc.DeleteNode(context.Background(), "alice"); err != nil {
		t.Fatalf("DeleteNode failed: %v", err)
	}

	// Verify no orphaned routes remain
	db.Model(&model.NodeRouteDO{}).Where("src_node_id = ? OR dst_node_id = ?", "alice", "alice").Count(&routeCount)
	if routeCount != 0 {
		t.Fatalf("expected 0 routes involving alice after delete, got %d", routeCount)
	}

	// Verify bob<->charlie routes are unaffected (there are none in this test,
	// but verify the total remaining is 0 since all routes involved alice)
	db.Model(&model.NodeRouteDO{}).Count(&routeCount)
	if routeCount != 0 {
		t.Fatalf("expected 0 total routes (all involved alice), got %d", routeCount)
	}
}

func TestNodeService_DeleteNode_NotFound(t *testing.T) {
	db := setupBugfix4TestDB(t)
	svc := NewNodeService(
		repository.NewNodeRepo(db),
		repository.NewNodeRouteRepo(db),
		nil,
		db,
	)

	err := svc.DeleteNode(context.Background(), "nonexistent")
	if err != ErrNodeNotFound {
		t.Fatalf("expected ErrNodeNotFound, got %v", err)
	}
}
