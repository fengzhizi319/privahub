package repository

import (
	"context"
	"testing"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupDatatableVoteTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	err = db.AutoMigrate(
		&model.ProjectDatatableDO{},
		&model.ProjectFedTableDO{},
		&model.VoteRequestDO{},
		&model.VoteInviteDO{},
	)
	if err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}
	return db
}

// --- DatatableRepo tests ---

func TestDatatableRepo_FindByProjectID(t *testing.T) {
	db := setupDatatableVoteTestDB(t)
	repo := NewDatatableRepo(db)
	ctx := context.Background()

	repo.Create(ctx, &model.ProjectDatatableDO{ProjectID: "p1", NodeID: "alice", DatatableID: "dt1", TableConfigs: "{}", Source: "CREATED"})
	repo.Create(ctx, &model.ProjectDatatableDO{ProjectID: "p1", NodeID: "bob", DatatableID: "dt2", TableConfigs: "{}", Source: "IMPORTED"})
	repo.Create(ctx, &model.ProjectDatatableDO{ProjectID: "p2", NodeID: "alice", DatatableID: "dt3", TableConfigs: "{}", Source: "CREATED"})

	dts, err := repo.FindByProjectID(ctx, "p1")
	if err != nil {
		t.Fatalf("FindByProjectID failed: %v", err)
	}
	if len(dts) != 2 {
		t.Errorf("expected 2 datatables for p1, got %d", len(dts))
	}
}

func TestDatatableRepo_FindByNodeID(t *testing.T) {
	db := setupDatatableVoteTestDB(t)
	repo := NewDatatableRepo(db)
	ctx := context.Background()

	repo.Create(ctx, &model.ProjectDatatableDO{ProjectID: "p1", NodeID: "alice", DatatableID: "dt1", TableConfigs: "{}", Source: "CREATED"})
	repo.Create(ctx, &model.ProjectDatatableDO{ProjectID: "p2", NodeID: "alice", DatatableID: "dt2", TableConfigs: "{}", Source: "CREATED"})
	repo.Create(ctx, &model.ProjectDatatableDO{ProjectID: "p1", NodeID: "bob", DatatableID: "dt3", TableConfigs: "{}", Source: "CREATED"})

	dts, err := repo.FindByNodeID(ctx, "alice")
	if err != nil {
		t.Fatalf("FindByNodeID failed: %v", err)
	}
	if len(dts) != 2 {
		t.Errorf("expected 2 datatables for alice, got %d", len(dts))
	}
}

func TestDatatableRepo_FindByProjectAndNodeID(t *testing.T) {
	db := setupDatatableVoteTestDB(t)
	repo := NewDatatableRepo(db)
	ctx := context.Background()

	repo.Create(ctx, &model.ProjectDatatableDO{ProjectID: "p1", NodeID: "alice", DatatableID: "dt1", TableConfigs: "{}", Source: "CREATED"})
	repo.Create(ctx, &model.ProjectDatatableDO{ProjectID: "p1", NodeID: "bob", DatatableID: "dt2", TableConfigs: "{}", Source: "CREATED"})

	dts, err := repo.FindByProjectAndNodeID(ctx, "p1", "alice")
	if err != nil {
		t.Fatalf("FindByProjectAndNodeID failed: %v", err)
	}
	if len(dts) != 1 {
		t.Errorf("expected 1 datatable for p1/alice, got %d", len(dts))
	}
}

func TestDatatableRepo_FindByProjectNodeDatatable(t *testing.T) {
	db := setupDatatableVoteTestDB(t)
	repo := NewDatatableRepo(db)
	ctx := context.Background()

	repo.Create(ctx, &model.ProjectDatatableDO{ProjectID: "p1", NodeID: "alice", DatatableID: "dt1", TableConfigs: `{"cols":["a"]}`, Source: "CREATED"})

	found, err := repo.FindByProjectNodeDatatable(ctx, "p1", "alice", "dt1")
	if err != nil {
		t.Fatalf("FindByProjectNodeDatatable failed: %v", err)
	}
	if found.TableConfigs != `{"cols":["a"]}` {
		t.Errorf("unexpected table_configs: %q", found.TableConfigs)
	}

	_, err = repo.FindByProjectNodeDatatable(ctx, "p1", "alice", "nonexistent")
	if err == nil {
		t.Error("expected error for non-existent datatable")
	}
}

func TestDatatableRepo_FindAll(t *testing.T) {
	db := setupDatatableVoteTestDB(t)
	repo := NewDatatableRepo(db)
	ctx := context.Background()

	repo.Create(ctx, &model.ProjectDatatableDO{ProjectID: "p1", NodeID: "alice", DatatableID: "dt1", TableConfigs: "{}", Source: "CREATED"})
	repo.Create(ctx, &model.ProjectDatatableDO{ProjectID: "p2", NodeID: "bob", DatatableID: "dt2", TableConfigs: "{}", Source: "CREATED"})

	all, err := repo.FindAll(ctx)
	if err != nil {
		t.Fatalf("FindAll failed: %v", err)
	}
	if len(all) != 2 {
		t.Errorf("expected 2 datatables, got %d", len(all))
	}
}

func TestDatatableRepo_DeleteByProjectID(t *testing.T) {
	db := setupDatatableVoteTestDB(t)
	repo := NewDatatableRepo(db)
	ctx := context.Background()

	repo.Create(ctx, &model.ProjectDatatableDO{ProjectID: "p1", NodeID: "alice", DatatableID: "dt1", TableConfigs: "{}", Source: "CREATED"})
	repo.Create(ctx, &model.ProjectDatatableDO{ProjectID: "p1", NodeID: "bob", DatatableID: "dt2", TableConfigs: "{}", Source: "CREATED"})
	repo.Create(ctx, &model.ProjectDatatableDO{ProjectID: "p2", NodeID: "alice", DatatableID: "dt3", TableConfigs: "{}", Source: "CREATED"})

	err := repo.DeleteByProjectID(ctx, "p1")
	if err != nil {
		t.Fatalf("DeleteByProjectID failed: %v", err)
	}

	dts, _ := repo.FindByProjectID(ctx, "p1")
	if len(dts) != 0 {
		t.Errorf("expected 0 datatables for p1 after delete, got %d", len(dts))
	}

	dts, _ = repo.FindByProjectID(ctx, "p2")
	if len(dts) != 1 {
		t.Errorf("expected 1 datatable for p2, got %d", len(dts))
	}
}

// --- FedTableRepo tests ---

func TestFedTableRepo_FindByProjectID(t *testing.T) {
	db := setupDatatableVoteTestDB(t)
	repo := NewFedTableRepo(db)
	ctx := context.Background()

	repo.Create(ctx, &model.ProjectFedTableDO{ProjectID: "p1", FedTableID: "ft1", Joins: "[]"})
	repo.Create(ctx, &model.ProjectFedTableDO{ProjectID: "p1", FedTableID: "ft2", Joins: "[]"})
	repo.Create(ctx, &model.ProjectFedTableDO{ProjectID: "p2", FedTableID: "ft3", Joins: "[]"})

	tables, err := repo.FindByProjectID(ctx, "p1")
	if err != nil {
		t.Fatalf("FindByProjectID failed: %v", err)
	}
	if len(tables) != 2 {
		t.Errorf("expected 2 fed tables for p1, got %d", len(tables))
	}
}

func TestFedTableRepo_FindByFedTableID(t *testing.T) {
	db := setupDatatableVoteTestDB(t)
	repo := NewFedTableRepo(db)
	ctx := context.Background()

	repo.Create(ctx, &model.ProjectFedTableDO{ProjectID: "p1", FedTableID: "ft1", Joins: `[{"nodeId":"alice"}]`})

	found, err := repo.FindByFedTableID(ctx, "ft1")
	if err != nil {
		t.Fatalf("FindByFedTableID failed: %v", err)
	}
	if found.ProjectID != "p1" {
		t.Errorf("expected project_id 'p1', got %q", found.ProjectID)
	}

	_, err = repo.FindByFedTableID(ctx, "nonexistent")
	if err == nil {
		t.Error("expected error for non-existent fed table")
	}
}

// --- VoteRequestRepo tests ---

// --- VoteInviteRepo tests ---
