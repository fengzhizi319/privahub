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

// setupBugfix7TestDB creates an in-memory SQLite database for bug fix 52-58 tests.
func setupBugfix7TestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	if err := db.AutoMigrate(
		&model.ProjectModelPackDO{},
		&model.ProjectModelServingDO{},
		&model.ProjectNodeDO{},
		&model.NodeDO{},
		&model.DatasourceDO{},
		&model.DatasourceNodeDO{},
		&model.NodeRouteDO{},
		&model.ProjectScheduleTaskDO{},
		&model.ProjectJobDO{},
		&model.ProjectJobTaskDO{},
	); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}
	return db
}

// --- Bug 56: NodeService.DeleteRoute atomicity ---

func TestNodeService_DeleteRoute_Atomic(t *testing.T) {
	db := setupBugfix7TestDB(t)

	nodeRepo := repository.NewNodeRepo(db)
	routeRepo := repository.NewNodeRouteRepo(db)
	svc := NewNodeService(nodeRepo, routeRepo, nil, db)
	ctx := context.Background()

	// Create bidirectional routes
	db.Create(&model.NodeRouteDO{RouteID: "r1", SrcNodeID: "alice", DstNodeID: "bob"})
	db.Create(&model.NodeRouteDO{RouteID: "r1", SrcNodeID: "bob", DstNodeID: "alice"})

	err := svc.DeleteRoute(ctx, "alice", "bob")
	if err != nil {
		t.Fatalf("DeleteRoute failed: %v", err)
	}

	// Verify both directions are deleted
	var count int64
	db.Model(&model.NodeRouteDO{}).Where(
		"(src_node_id = ? AND dst_node_id = ?) OR (src_node_id = ? AND dst_node_id = ?)",
		"alice", "bob", "bob", "alice",
	).Count(&count)
	if count != 0 {
		t.Errorf("expected 0 routes after deletion, got %d", count)
	}
}

func TestNodeService_DeleteRoute_NotFound(t *testing.T) {
	db := setupBugfix7TestDB(t)

	nodeRepo := repository.NewNodeRepo(db)
	routeRepo := repository.NewNodeRouteRepo(db)
	svc := NewNodeService(nodeRepo, routeRepo, nil, db)
	ctx := context.Background()

	err := svc.DeleteRoute(ctx, "alice", "bob")
	if err != ErrRouteNotFound {
		t.Errorf("expected ErrRouteNotFound, got %v", err)
	}
}

// Bug 58 datasource detail tests moved to datasource_service_test.go (Kuscia-backed).
