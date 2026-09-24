package service

import (
	"context"
	"testing"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/internal/dao/repository"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupBugfixTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	err = db.AutoMigrate(
		&model.ProjectDO{},
		&model.ProjectInstDO{},
		&model.ProjectNodeDO{},
		&model.ProjectGraphDO{},
		&model.ProjectGraphNodeDO{},
		&model.ProjectJobDO{},
		&model.ProjectJobTaskDO{},
		&model.ProjectJobTaskLogDO{},
		&model.NodeDO{},
		&model.NodeRouteDO{},
		&model.ProjectDatatableDO{},
		&model.VoteRequestDO{},
		&model.VoteInviteDO{},
	)
	if err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}
	return db
}

// --- Bug 17: FullUpdateGraph atomicity ---

// --- Bug 18: CreateRoute orphan prevention ---

func TestNodeService_CreateRoute_Success(t *testing.T) {
	db := setupBugfixTestDB(t)

	svc := NewNodeService(
		repository.NewNodeRepo(db),
		repository.NewNodeRouteRepo(db),
		nil,
		db,
	)

	err := svc.CreateRoute(context.Background(), &CreateRouteRequest{
		SrcNodeID:     "alice",
		DstNodeID:     "bob",
		SrcNetAddress: "alice:8080",
		DstNetAddress: "bob:8080",
	})
	if err != nil {
		t.Fatalf("CreateRoute failed: %v", err)
	}

	// Verify forward route exists
	routes, err := svc.ListRoutes(context.Background(), "alice")
	if err != nil {
		t.Fatalf("ListRoutes failed: %v", err)
	}
	if len(routes) != 1 {
		t.Fatalf("expected 1 forward route, got %d", len(routes))
	}
	if routes[0].DstNodeID != "bob" {
		t.Errorf("expected dst 'bob', got %q", routes[0].DstNodeID)
	}

	// Verify reverse route exists
	reverseRoutes, _ := svc.ListRoutes(context.Background(), "bob")
	if len(reverseRoutes) != 1 {
		t.Fatalf("expected 1 reverse route, got %d", len(reverseRoutes))
	}
	if reverseRoutes[0].DstNodeID != "alice" {
		t.Errorf("expected reverse dst 'alice', got %q", reverseRoutes[0].DstNodeID)
	}
}

func TestNodeService_CreateRoute_DuplicateRejected(t *testing.T) {
	db := setupBugfixTestDB(t)

	svc := NewNodeService(
		repository.NewNodeRepo(db),
		repository.NewNodeRouteRepo(db),
		nil,
		db,
	)

	_ = svc.CreateRoute(context.Background(), &CreateRouteRequest{
		SrcNodeID: "alice",
		DstNodeID: "bob",
	})

	// Duplicate should be rejected
	err := svc.CreateRoute(context.Background(), &CreateRouteRequest{
		SrcNodeID: "alice",
		DstNodeID: "bob",
	})
	if err != ErrRouteAlreadyExists {
		t.Errorf("expected ErrRouteAlreadyExists, got %v", err)
	}
}

// --- Bug 19: DeleteProject cascade cleanup ---
