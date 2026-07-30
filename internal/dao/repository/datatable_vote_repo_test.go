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

func TestVoteRequestRepo_FindByVoteID(t *testing.T) {
	db := setupDatatableVoteTestDB(t)
	repo := NewVoteRequestRepo(db)
	ctx := context.Background()

	repo.Create(ctx, &model.VoteRequestDO{
		VoteID: "v1", Initiator: "alice", Type: "PROJECT",
		Voters: "bob,carol", VoteCounter: "alice", Executors: "alice",
		ApprovedThreshold: 2, Status: 0, Description: "test vote",
	})

	found, err := repo.FindByVoteID(ctx, "v1")
	if err != nil {
		t.Fatalf("FindByVoteID failed: %v", err)
	}
	if found.Initiator != "alice" {
		t.Errorf("expected initiator 'alice', got %q", found.Initiator)
	}

	_, err = repo.FindByVoteID(ctx, "nonexistent")
	if err == nil {
		t.Error("expected error for non-existent vote")
	}
}

func TestVoteRequestRepo_FindByInitiator(t *testing.T) {
	db := setupDatatableVoteTestDB(t)
	repo := NewVoteRequestRepo(db)
	ctx := context.Background()

	repo.Create(ctx, &model.VoteRequestDO{VoteID: "v1", Initiator: "alice", Type: "PROJECT", Voters: "bob", VoteCounter: "alice", Executors: "alice", ApprovedThreshold: 1, Status: 0, Description: "d1"})
	repo.Create(ctx, &model.VoteRequestDO{VoteID: "v2", Initiator: "alice", Type: "NODE_ROUTE", Voters: "carol", VoteCounter: "alice", Executors: "alice", ApprovedThreshold: 1, Status: 1, Description: "d2"})
	repo.Create(ctx, &model.VoteRequestDO{VoteID: "v3", Initiator: "bob", Type: "PROJECT", Voters: "alice", VoteCounter: "bob", Executors: "bob", ApprovedThreshold: 1, Status: 0, Description: "d3"})

	votes, err := repo.FindByInitiator(ctx, "alice")
	if err != nil {
		t.Fatalf("FindByInitiator failed: %v", err)
	}
	if len(votes) != 2 {
		t.Errorf("expected 2 votes from alice, got %d", len(votes))
	}
}

func TestVoteRequestRepo_FindByVoter(t *testing.T) {
	db := setupDatatableVoteTestDB(t)
	repo := NewVoteRequestRepo(db)
	ctx := context.Background()

	repo.Create(ctx, &model.VoteRequestDO{VoteID: "v1", Initiator: "alice", Type: "PROJECT", Voters: "bob,carol", VoteCounter: "alice", Executors: "alice", ApprovedThreshold: 2, Status: 0, Description: "d1"})
	repo.Create(ctx, &model.VoteRequestDO{VoteID: "v2", Initiator: "carol", Type: "PROJECT", Voters: "bob", VoteCounter: "carol", Executors: "carol", ApprovedThreshold: 1, Status: 0, Description: "d2"})
	repo.Create(ctx, &model.VoteRequestDO{VoteID: "v3", Initiator: "alice", Type: "PROJECT", Voters: "carol", VoteCounter: "alice", Executors: "alice", ApprovedThreshold: 1, Status: 0, Description: "d3"})

	votes, err := repo.FindByVoter(ctx, "bob")
	if err != nil {
		t.Fatalf("FindByVoter failed: %v", err)
	}
	if len(votes) != 2 {
		t.Errorf("expected 2 votes where bob is voter, got %d", len(votes))
	}
}

// --- VoteInviteRepo tests ---

func TestVoteInviteRepo_FindByVoteID(t *testing.T) {
	db := setupDatatableVoteTestDB(t)
	repo := NewVoteInviteRepo(db)
	ctx := context.Background()

	repo.Create(ctx, &model.VoteInviteDO{VoteID: "v1", Initiator: "alice", VoteParticipantID: "bob", Type: "PROJECT", Description: "d"})
	repo.Create(ctx, &model.VoteInviteDO{VoteID: "v1", Initiator: "alice", VoteParticipantID: "carol", Type: "PROJECT", Description: "d"})
	repo.Create(ctx, &model.VoteInviteDO{VoteID: "v2", Initiator: "bob", VoteParticipantID: "alice", Type: "PROJECT", Description: "d"})

	invites, err := repo.FindByVoteID(ctx, "v1")
	if err != nil {
		t.Fatalf("FindByVoteID failed: %v", err)
	}
	if len(invites) != 2 {
		t.Errorf("expected 2 invites for v1, got %d", len(invites))
	}
}

func TestVoteInviteRepo_FindByParticipant(t *testing.T) {
	db := setupDatatableVoteTestDB(t)
	repo := NewVoteInviteRepo(db)
	ctx := context.Background()

	repo.Create(ctx, &model.VoteInviteDO{VoteID: "v1", Initiator: "alice", VoteParticipantID: "bob", Type: "PROJECT", Description: "d"})
	repo.Create(ctx, &model.VoteInviteDO{VoteID: "v2", Initiator: "carol", VoteParticipantID: "bob", Type: "NODE_ROUTE", Description: "d"})
	repo.Create(ctx, &model.VoteInviteDO{VoteID: "v1", Initiator: "alice", VoteParticipantID: "carol", Type: "PROJECT", Description: "d"})

	invites, err := repo.FindByParticipant(ctx, "bob")
	if err != nil {
		t.Fatalf("FindByParticipant failed: %v", err)
	}
	if len(invites) != 2 {
		t.Errorf("expected 2 invites for bob, got %d", len(invites))
	}
}

func TestVoteInviteRepo_FindByVoteAndParticipant(t *testing.T) {
	db := setupDatatableVoteTestDB(t)
	repo := NewVoteInviteRepo(db)
	ctx := context.Background()

	repo.Create(ctx, &model.VoteInviteDO{VoteID: "v1", Initiator: "alice", VoteParticipantID: "bob", Type: "PROJECT", Action: "AGREE", Description: "d"})

	found, err := repo.FindByVoteAndParticipant(ctx, "v1", "bob")
	if err != nil {
		t.Fatalf("FindByVoteAndParticipant failed: %v", err)
	}
	if found.Action != "AGREE" {
		t.Errorf("expected action 'AGREE', got %q", found.Action)
	}

	_, err = repo.FindByVoteAndParticipant(ctx, "v1", "nonexistent")
	if err == nil {
		t.Error("expected error for non-existent participant")
	}
}
