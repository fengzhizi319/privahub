package repository

import (
	"context"
	"testing"

	"github.com/fengzhizi319/privahub/internal/dao/model"
)

// --- NodeRepo additional tests ---

func TestNodeRepo_FindByControlNodeID(t *testing.T) {
	db := setupRepoTestDB(t)
	repo := NewNodeRepo(db)
	ctx := context.Background()

	repo.Create(ctx, &model.NodeDO{NodeID: "n1", Name: "N1", Type: "normal", ControlNodeID: "master"})
	repo.Create(ctx, &model.NodeDO{NodeID: "n2", Name: "N2", Type: "normal", ControlNodeID: "master"})
	repo.Create(ctx, &model.NodeDO{NodeID: "n3", Name: "N3", Type: "tee", ControlNodeID: "other"})

	nodes, err := repo.FindByControlNodeID(ctx, "master")
	if err != nil {
		t.Fatalf("FindByControlNodeID failed: %v", err)
	}
	if len(nodes) != 2 {
		t.Errorf("expected 2 nodes controlled by master, got %d", len(nodes))
	}
}

func TestNodeRepo_FindByControlNodeID_Empty(t *testing.T) {
	db := setupRepoTestDB(t)
	repo := NewNodeRepo(db)

	nodes, err := repo.FindByControlNodeID(context.Background(), "nonexistent")
	if err != nil {
		t.Fatalf("FindByControlNodeID failed: %v", err)
	}
	if len(nodes) != 0 {
		t.Errorf("expected 0 nodes, got %d", len(nodes))
	}
}

// --- NodeRouteRepo additional tests ---

func TestNodeRouteRepo_FindByDstNodeID(t *testing.T) {
	db := setupRepoTestDB(t)
	repo := NewNodeRouteRepo(db)
	ctx := context.Background()

	repo.Create(ctx, &model.NodeRouteDO{RouteID: "r1", SrcNodeID: "alice", DstNodeID: "bob"})
	repo.Create(ctx, &model.NodeRouteDO{RouteID: "r2", SrcNodeID: "carol", DstNodeID: "bob"})
	repo.Create(ctx, &model.NodeRouteDO{RouteID: "r3", SrcNodeID: "bob", DstNodeID: "alice"})

	routes, err := repo.FindByDstNodeID(ctx, "bob")
	if err != nil {
		t.Fatalf("FindByDstNodeID failed: %v", err)
	}
	if len(routes) != 2 {
		t.Errorf("expected 2 routes to bob, got %d", len(routes))
	}
}

// --- ProjectRepo additional tests ---

func TestProjectRepo_FindByProjectID(t *testing.T) {
	db := setupRepoTestDB(t)
	repo := NewProjectRepo(db)
	ctx := context.Background()

	repo.Create(ctx, &model.ProjectDO{ProjectID: "proj-1", Name: "Project 1"})

	found, err := repo.FindByProjectID(ctx, "proj-1")
	if err != nil {
		t.Fatalf("FindByProjectID failed: %v", err)
	}
	if found.Name != "Project 1" {
		t.Errorf("expected name 'Project 1', got %q", found.Name)
	}

	_, err = repo.FindByProjectID(ctx, "nonexistent")
	if err == nil {
		t.Error("expected error for non-existent project")
	}
}

func TestProjectRepo_FindByOwnerID(t *testing.T) {
	db := setupRepoTestDB(t)
	repo := NewProjectRepo(db)
	ctx := context.Background()

	repo.Create(ctx, &model.ProjectDO{ProjectID: "p1", Name: "P1", OwnerID: "alice"})
	repo.Create(ctx, &model.ProjectDO{ProjectID: "p2", Name: "P2", OwnerID: "alice"})
	repo.Create(ctx, &model.ProjectDO{ProjectID: "p3", Name: "P3", OwnerID: "bob"})

	projects, err := repo.FindByOwnerID(ctx, "alice")
	if err != nil {
		t.Fatalf("FindByOwnerID failed: %v", err)
	}
	if len(projects) != 2 {
		t.Errorf("expected 2 projects owned by alice, got %d", len(projects))
	}
}

// --- InstRepo tests ---

func TestInstRepo_FindByInstID(t *testing.T) {
	db := setupRepoTestDB(t)
	repo := NewInstRepo(db)
	ctx := context.Background()

	repo.Create(ctx, &model.InstDO{InstID: "inst-1", Name: "Institution 1"})

	found, err := repo.FindByInstID(ctx, "inst-1")
	if err != nil {
		t.Fatalf("FindByInstID failed: %v", err)
	}
	if found.Name != "Institution 1" {
		t.Errorf("expected name 'Institution 1', got %q", found.Name)
	}

	_, err = repo.FindByInstID(ctx, "nonexistent")
	if err == nil {
		t.Error("expected error for non-existent inst")
	}
}

// --- ProjectInstRepo additional tests ---

func TestProjectInstRepo_FindByInstID(t *testing.T) {
	db := setupRepoTestDB(t)
	repo := NewProjectInstRepo(db)
	ctx := context.Background()

	repo.Create(ctx, &model.ProjectInstDO{ProjectID: "p1", InstID: "inst-1"})
	repo.Create(ctx, &model.ProjectInstDO{ProjectID: "p2", InstID: "inst-1"})
	repo.Create(ctx, &model.ProjectInstDO{ProjectID: "p1", InstID: "inst-2"})

	insts, err := repo.FindByInstID(ctx, "inst-1")
	if err != nil {
		t.Fatalf("FindByInstID failed: %v", err)
	}
	if len(insts) != 2 {
		t.Errorf("expected 2 projects for inst-1, got %d", len(insts))
	}
}

// --- ProjectNodeRepo tests ---

func TestProjectNodeRepo_FindByProjectID(t *testing.T) {
	db := setupRepoTestDB(t)
	repo := NewProjectNodeRepo(db)
	ctx := context.Background()

	repo.Create(ctx, &model.ProjectNodeDO{ProjectID: "p1", NodeID: "alice"})
	repo.Create(ctx, &model.ProjectNodeDO{ProjectID: "p1", NodeID: "bob"})
	repo.Create(ctx, &model.ProjectNodeDO{ProjectID: "p2", NodeID: "alice"})

	nodes, err := repo.FindByProjectID(ctx, "p1")
	if err != nil {
		t.Fatalf("FindByProjectID failed: %v", err)
	}
	if len(nodes) != 2 {
		t.Errorf("expected 2 nodes for p1, got %d", len(nodes))
	}
}

func TestProjectNodeRepo_FindByNodeID(t *testing.T) {
	db := setupRepoTestDB(t)
	repo := NewProjectNodeRepo(db)
	ctx := context.Background()

	repo.Create(ctx, &model.ProjectNodeDO{ProjectID: "p1", NodeID: "alice"})
	repo.Create(ctx, &model.ProjectNodeDO{ProjectID: "p2", NodeID: "alice"})
	repo.Create(ctx, &model.ProjectNodeDO{ProjectID: "p1", NodeID: "bob"})

	nodes, err := repo.FindByNodeID(ctx, "alice")
	if err != nil {
		t.Fatalf("FindByNodeID failed: %v", err)
	}
	if len(nodes) != 2 {
		t.Errorf("expected 2 projects for alice, got %d", len(nodes))
	}
}

func TestProjectNodeRepo_DeleteByProjectID(t *testing.T) {
	db := setupRepoTestDB(t)
	repo := NewProjectNodeRepo(db)
	ctx := context.Background()

	repo.Create(ctx, &model.ProjectNodeDO{ProjectID: "p1", NodeID: "alice"})
	repo.Create(ctx, &model.ProjectNodeDO{ProjectID: "p1", NodeID: "bob"})
	repo.Create(ctx, &model.ProjectNodeDO{ProjectID: "p2", NodeID: "alice"})

	err := repo.DeleteByProjectID(ctx, "p1")
	if err != nil {
		t.Fatalf("DeleteByProjectID failed: %v", err)
	}

	nodes, _ := repo.FindByProjectID(ctx, "p1")
	if len(nodes) != 0 {
		t.Errorf("expected 0 nodes for p1 after delete, got %d", len(nodes))
	}

	nodes, _ = repo.FindByProjectID(ctx, "p2")
	if len(nodes) != 1 {
		t.Errorf("expected 1 node for p2, got %d", len(nodes))
	}
}
