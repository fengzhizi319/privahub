package repository

import (
	"context"
	"testing"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupRepoTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	err = db.AutoMigrate(
		&model.InstDO{},
		&model.NodeDO{},
		&model.NodeRouteDO{},
		&model.ProjectDO{},
		&model.ProjectInstDO{},
		&model.ProjectNodeDO{},
	)
	if err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}
	return db
}

func TestBaseRepo_Create(t *testing.T) {
	db := setupRepoTestDB(t)
	repo := NewBaseRepo[model.InstDO](db)

	inst := &model.InstDO{InstID: "test-inst", Name: "Test Institution"}
	err := repo.Create(context.Background(), inst)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if inst.ID == 0 {
		t.Error("expected non-zero ID after create")
	}
}

func TestBaseRepo_BatchCreate(t *testing.T) {
	db := setupRepoTestDB(t)
	repo := NewBaseRepo[model.InstDO](db)

	entities := []model.InstDO{
		{InstID: "inst-1", Name: "Inst 1"},
		{InstID: "inst-2", Name: "Inst 2"},
		{InstID: "inst-3", Name: "Inst 3"},
	}
	err := repo.BatchCreate(context.Background(), entities)
	if err != nil {
		t.Fatalf("BatchCreate failed: %v", err)
	}

	all, _ := repo.FindAll(context.Background())
	if len(all) != 3 {
		t.Errorf("expected 3 entities, got %d", len(all))
	}
}

func TestBaseRepo_BatchCreate_Empty(t *testing.T) {
	db := setupRepoTestDB(t)
	repo := NewBaseRepo[model.InstDO](db)

	err := repo.BatchCreate(context.Background(), []model.InstDO{})
	if err != nil {
		t.Errorf("BatchCreate with empty slice should not fail: %v", err)
	}
}

func TestBaseRepo_FindByID(t *testing.T) {
	db := setupRepoTestDB(t)
	repo := NewBaseRepo[model.InstDO](db)

	inst := &model.InstDO{InstID: "find-me", Name: "Find Me"}
	repo.Create(context.Background(), inst)

	found, err := repo.FindByID(context.Background(), inst.ID)
	if err != nil {
		t.Fatalf("FindByID failed: %v", err)
	}
	if found.InstID != "find-me" {
		t.Errorf("expected inst_id 'find-me', got %q", found.InstID)
	}
}

func TestBaseRepo_FindByID_NotFound(t *testing.T) {
	db := setupRepoTestDB(t)
	repo := NewBaseRepo[model.InstDO](db)

	_, err := repo.FindByID(context.Background(), 99999)
	if err == nil {
		t.Error("expected error for non-existent ID")
	}
}

func TestBaseRepo_Update(t *testing.T) {
	db := setupRepoTestDB(t)
	repo := NewBaseRepo[model.InstDO](db)

	inst := &model.InstDO{InstID: "update-me", Name: "Old Name"}
	repo.Create(context.Background(), inst)

	inst.Name = "New Name"
	err := repo.Update(context.Background(), inst)
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	found, _ := repo.FindByID(context.Background(), inst.ID)
	if found.Name != "New Name" {
		t.Errorf("expected name 'New Name', got %q", found.Name)
	}
}

func TestBaseRepo_Delete(t *testing.T) {
	db := setupRepoTestDB(t)
	repo := NewBaseRepo[model.InstDO](db)

	inst := &model.InstDO{InstID: "delete-me", Name: "Delete Me"}
	repo.Create(context.Background(), inst)

	err := repo.Delete(context.Background(), inst.ID)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	_, err = repo.FindByID(context.Background(), inst.ID)
	if err == nil {
		t.Error("expected error after delete")
	}
}

func TestBaseRepo_FindAll(t *testing.T) {
	db := setupRepoTestDB(t)
	repo := NewBaseRepo[model.InstDO](db)

	repo.Create(context.Background(), &model.InstDO{InstID: "all-1", Name: "All 1"})
	repo.Create(context.Background(), &model.InstDO{InstID: "all-2", Name: "All 2"})

	all, err := repo.FindAll(context.Background())
	if err != nil {
		t.Fatalf("FindAll failed: %v", err)
	}
	if len(all) != 2 {
		t.Errorf("expected 2 entities, got %d", len(all))
	}
}

func TestNodeRepo_FindByNodeID(t *testing.T) {
	db := setupRepoTestDB(t)
	repo := NewNodeRepo(db)

	node := &model.NodeDO{NodeID: "node-1", Name: "Node 1", Type: "normal"}
	repo.Create(context.Background(), node)

	found, err := repo.FindByNodeID(context.Background(), "node-1")
	if err != nil {
		t.Fatalf("FindByNodeID failed: %v", err)
	}
	if found.Name != "Node 1" {
		t.Errorf("expected name 'Node 1', got %q", found.Name)
	}
}

func TestNodeRepo_FindByNodeID_NotFound(t *testing.T) {
	db := setupRepoTestDB(t)
	repo := NewNodeRepo(db)

	_, err := repo.FindByNodeID(context.Background(), "nonexistent")
	if err == nil {
		t.Error("expected error for non-existent node")
	}
}

func TestNodeRepo_FindByType(t *testing.T) {
	db := setupRepoTestDB(t)
	repo := NewNodeRepo(db)

	repo.Create(context.Background(), &model.NodeDO{NodeID: "n1", Name: "N1", Type: "normal"})
	repo.Create(context.Background(), &model.NodeDO{NodeID: "n2", Name: "N2", Type: "tee"})
	repo.Create(context.Background(), &model.NodeDO{NodeID: "n3", Name: "N3", Type: "normal"})

	nodes, err := repo.FindByType(context.Background(), "normal")
	if err != nil {
		t.Fatalf("FindByType failed: %v", err)
	}
	if len(nodes) != 2 {
		t.Errorf("expected 2 normal nodes, got %d", len(nodes))
	}
}

func TestNodeRouteRepo_FindByPair(t *testing.T) {
	db := setupRepoTestDB(t)
	repo := NewNodeRouteRepo(db)

	route := &model.NodeRouteDO{
		RouteID:   "route-1",
		SrcNodeID: "alice",
		DstNodeID: "bob",
	}
	repo.Create(context.Background(), route)

	found, err := repo.FindByPair(context.Background(), "alice", "bob")
	if err != nil {
		t.Fatalf("FindByPair failed: %v", err)
	}
	if found.RouteID != "route-1" {
		t.Errorf("expected route_id 'route-1', got %q", found.RouteID)
	}

	// Reverse pair should not exist
	_, err = repo.FindByPair(context.Background(), "bob", "alice")
	if err == nil {
		t.Error("expected error for reverse pair")
	}
}

func TestNodeRouteRepo_FindBySrcNodeID(t *testing.T) {
	db := setupRepoTestDB(t)
	repo := NewNodeRouteRepo(db)

	repo.Create(context.Background(), &model.NodeRouteDO{RouteID: "r1", SrcNodeID: "alice", DstNodeID: "bob"})
	repo.Create(context.Background(), &model.NodeRouteDO{RouteID: "r2", SrcNodeID: "alice", DstNodeID: "carol"})
	repo.Create(context.Background(), &model.NodeRouteDO{RouteID: "r3", SrcNodeID: "bob", DstNodeID: "alice"})

	routes, err := repo.FindBySrcNodeID(context.Background(), "alice")
	if err != nil {
		t.Fatalf("FindBySrcNodeID failed: %v", err)
	}
	if len(routes) != 2 {
		t.Errorf("expected 2 routes from alice, got %d", len(routes))
	}
}

func TestProjectRepo_PageQuery(t *testing.T) {
	db := setupRepoTestDB(t)
	repo := NewProjectRepo(db)

	// Create 5 projects
	for i := 1; i <= 5; i++ {
		repo.Create(context.Background(), &model.ProjectDO{
			ProjectID: "proj-" + string(rune('0'+i)),
			Name:      "Project " + string(rune('0'+i)),
		})
	}

	// Page 1, size 2
	projects, total, err := repo.PageQuery(context.Background(), 1, 2, "")
	if err != nil {
		t.Fatalf("PageQuery failed: %v", err)
	}
	if total != 5 {
		t.Errorf("expected total 5, got %d", total)
	}
	if len(projects) != 2 {
		t.Errorf("expected 2 projects on page 1, got %d", len(projects))
	}

	// Page 3, size 2 (should have 1 project)
	projects, total, err = repo.PageQuery(context.Background(), 3, 2, "")
	if err != nil {
		t.Fatalf("PageQuery failed: %v", err)
	}
	if total != 5 {
		t.Errorf("expected total 5, got %d", total)
	}
	if len(projects) != 1 {
		t.Errorf("expected 1 project on page 3, got %d", len(projects))
	}
}

func TestProjectRepo_PageQuery_WithNameFilter(t *testing.T) {
	db := setupRepoTestDB(t)
	repo := NewProjectRepo(db)

	repo.Create(context.Background(), &model.ProjectDO{ProjectID: "p1", Name: "Alpha Project"})
	repo.Create(context.Background(), &model.ProjectDO{ProjectID: "p2", Name: "Beta Project"})
	repo.Create(context.Background(), &model.ProjectDO{ProjectID: "p3", Name: "Alpha Test"})

	projects, total, err := repo.PageQuery(context.Background(), 1, 10, "Alpha")
	if err != nil {
		t.Fatalf("PageQuery with name filter failed: %v", err)
	}
	if total != 2 {
		t.Errorf("expected 2 projects matching 'Alpha', got %d", total)
	}
	if len(projects) != 2 {
		t.Errorf("expected 2 projects, got %d", len(projects))
	}
}

func TestProjectInstRepo_DeleteByProjectID(t *testing.T) {
	db := setupRepoTestDB(t)
	repo := NewProjectInstRepo(db)

	repo.Create(context.Background(), &model.ProjectInstDO{ProjectID: "proj-1", InstID: "inst-1"})
	repo.Create(context.Background(), &model.ProjectInstDO{ProjectID: "proj-1", InstID: "inst-2"})
	repo.Create(context.Background(), &model.ProjectInstDO{ProjectID: "proj-2", InstID: "inst-1"})

	err := repo.DeleteByProjectID(context.Background(), "proj-1")
	if err != nil {
		t.Fatalf("DeleteByProjectID failed: %v", err)
	}

	// proj-1 should have no insts
	insts, _ := repo.FindByProjectID(context.Background(), "proj-1")
	if len(insts) != 0 {
		t.Errorf("expected 0 insts for proj-1, got %d", len(insts))
	}

	// proj-2 should still have 1 inst
	insts, _ = repo.FindByProjectID(context.Background(), "proj-2")
	if len(insts) != 1 {
		t.Errorf("expected 1 inst for proj-2, got %d", len(insts))
	}
}
