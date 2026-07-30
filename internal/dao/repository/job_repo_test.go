package repository

import (
	"context"
	"testing"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupJobTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	err = db.AutoMigrate(
		&model.ProjectJobDO{},
		&model.ProjectJobTaskDO{},
		&model.ProjectJobTaskLogDO{},
		&model.ProjectGraphDO{},
		&model.ProjectGraphNodeDO{},
	)
	if err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}
	return db
}

// --- JobRepo tests ---

func TestJobRepo_FindByJobID(t *testing.T) {
	db := setupJobTestDB(t)
	repo := NewJobRepo(db)
	ctx := context.Background()

	repo.Create(ctx, &model.ProjectJobDO{ProjectID: "p1", JobID: "job-1", Name: "Job 1", Status: "Running"})

	found, err := repo.FindByJobID(ctx, "job-1")
	if err != nil {
		t.Fatalf("FindByJobID failed: %v", err)
	}
	if found.Name != "Job 1" {
		t.Errorf("expected name 'Job 1', got %q", found.Name)
	}
}

func TestJobRepo_FindByJobID_NotFound(t *testing.T) {
	db := setupJobTestDB(t)
	repo := NewJobRepo(db)

	_, err := repo.FindByJobID(context.Background(), "nonexistent")
	if err == nil {
		t.Error("expected error for non-existent job")
	}
}

func TestJobRepo_FindByProjectID(t *testing.T) {
	db := setupJobTestDB(t)
	repo := NewJobRepo(db)
	ctx := context.Background()

	repo.Create(ctx, &model.ProjectJobDO{ProjectID: "p1", JobID: "j1", Name: "J1", Status: "Succeeded"})
	repo.Create(ctx, &model.ProjectJobDO{ProjectID: "p1", JobID: "j2", Name: "J2", Status: "Running"})
	repo.Create(ctx, &model.ProjectJobDO{ProjectID: "p2", JobID: "j3", Name: "J3", Status: "Failed"})

	jobs, err := repo.FindByProjectID(ctx, "p1")
	if err != nil {
		t.Fatalf("FindByProjectID failed: %v", err)
	}
	if len(jobs) != 2 {
		t.Errorf("expected 2 jobs for p1, got %d", len(jobs))
	}
}

func TestJobRepo_FindByProjectAndJobID(t *testing.T) {
	db := setupJobTestDB(t)
	repo := NewJobRepo(db)
	ctx := context.Background()

	repo.Create(ctx, &model.ProjectJobDO{ProjectID: "p1", JobID: "j1", Name: "J1", Status: "Running"})

	found, err := repo.FindByProjectAndJobID(ctx, "p1", "j1")
	if err != nil {
		t.Fatalf("FindByProjectAndJobID failed: %v", err)
	}
	if found.JobID != "j1" {
		t.Errorf("expected job_id 'j1', got %q", found.JobID)
	}

	// Wrong project
	_, err = repo.FindByProjectAndJobID(ctx, "p2", "j1")
	if err == nil {
		t.Error("expected error for wrong project")
	}
}

func TestJobRepo_UpdateStatus(t *testing.T) {
	db := setupJobTestDB(t)
	repo := NewJobRepo(db)
	ctx := context.Background()

	repo.Create(ctx, &model.ProjectJobDO{ProjectID: "p1", JobID: "j1", Name: "J1", Status: "Running"})

	err := repo.UpdateStatus(ctx, "j1", "Failed", "timeout exceeded")
	if err != nil {
		t.Fatalf("UpdateStatus failed: %v", err)
	}

	found, _ := repo.FindByJobID(ctx, "j1")
	if found.Status != "Failed" {
		t.Errorf("expected status 'Failed', got %q", found.Status)
	}
	if found.ErrMsg != "timeout exceeded" {
		t.Errorf("expected err_msg 'timeout exceeded', got %q", found.ErrMsg)
	}
}

func TestJobRepo_UpdateStatus_NoErrMsg(t *testing.T) {
	db := setupJobTestDB(t)
	repo := NewJobRepo(db)
	ctx := context.Background()

	repo.Create(ctx, &model.ProjectJobDO{ProjectID: "p1", JobID: "j1", Name: "J1", Status: "Running", ErrMsg: "old error"})

	err := repo.UpdateStatus(ctx, "j1", "Succeeded", "")
	if err != nil {
		t.Fatalf("UpdateStatus failed: %v", err)
	}

	found, _ := repo.FindByJobID(ctx, "j1")
	if found.Status != "Succeeded" {
		t.Errorf("expected status 'Succeeded', got %q", found.Status)
	}
	// ErrMsg should remain unchanged when empty string is passed
	if found.ErrMsg != "old error" {
		t.Errorf("expected err_msg to remain 'old error', got %q", found.ErrMsg)
	}
}

// --- TaskRepo tests ---

func TestTaskRepo_FindByJobID(t *testing.T) {
	db := setupJobTestDB(t)
	repo := NewTaskRepo(db)
	ctx := context.Background()

	repo.Create(ctx, &model.ProjectJobTaskDO{ProjectID: "p1", JobID: "j1", TaskID: "t1", Parties: "alice,bob", Status: "Running"})
	repo.Create(ctx, &model.ProjectJobTaskDO{ProjectID: "p1", JobID: "j1", TaskID: "t2", Parties: "alice", Status: "Succeeded"})
	repo.Create(ctx, &model.ProjectJobTaskDO{ProjectID: "p1", JobID: "j2", TaskID: "t3", Parties: "bob", Status: "Pending"})

	tasks, err := repo.FindByJobID(ctx, "j1")
	if err != nil {
		t.Fatalf("FindByJobID failed: %v", err)
	}
	if len(tasks) != 2 {
		t.Errorf("expected 2 tasks for j1, got %d", len(tasks))
	}
}

func TestTaskRepo_FindByProjectAndJobID(t *testing.T) {
	db := setupJobTestDB(t)
	repo := NewTaskRepo(db)
	ctx := context.Background()

	repo.Create(ctx, &model.ProjectJobTaskDO{ProjectID: "p1", JobID: "j1", TaskID: "t1", Parties: "alice", Status: "Running"})
	repo.Create(ctx, &model.ProjectJobTaskDO{ProjectID: "p2", JobID: "j1", TaskID: "t2", Parties: "bob", Status: "Running"})

	tasks, err := repo.FindByProjectAndJobID(ctx, "p1", "j1")
	if err != nil {
		t.Fatalf("FindByProjectAndJobID failed: %v", err)
	}
	if len(tasks) != 1 {
		t.Errorf("expected 1 task for p1/j1, got %d", len(tasks))
	}
}

func TestTaskRepo_FindByTaskID(t *testing.T) {
	db := setupJobTestDB(t)
	repo := NewTaskRepo(db)
	ctx := context.Background()

	repo.Create(ctx, &model.ProjectJobTaskDO{ProjectID: "p1", JobID: "j1", TaskID: "t1", Parties: "alice", Status: "Running"})

	found, err := repo.FindByTaskID(ctx, "t1")
	if err != nil {
		t.Fatalf("FindByTaskID failed: %v", err)
	}
	if found.Parties != "alice" {
		t.Errorf("expected parties 'alice', got %q", found.Parties)
	}

	_, err = repo.FindByTaskID(ctx, "nonexistent")
	if err == nil {
		t.Error("expected error for non-existent task")
	}
}

func TestTaskRepo_UpdateStatus(t *testing.T) {
	db := setupJobTestDB(t)
	repo := NewTaskRepo(db)
	ctx := context.Background()

	repo.Create(ctx, &model.ProjectJobTaskDO{ProjectID: "p1", JobID: "j1", TaskID: "t1", Parties: "alice", Status: "Running"})

	err := repo.UpdateStatus(ctx, "t1", "Failed", "OOM killed")
	if err != nil {
		t.Fatalf("UpdateStatus failed: %v", err)
	}

	found, _ := repo.FindByTaskID(ctx, "t1")
	if found.Status != "Failed" {
		t.Errorf("expected status 'Failed', got %q", found.Status)
	}
	if found.ErrMsg != "OOM killed" {
		t.Errorf("expected err_msg 'OOM killed', got %q", found.ErrMsg)
	}
}

// --- TaskLogRepo tests ---

func TestTaskLogRepo_CreateAndFind(t *testing.T) {
	db := setupJobTestDB(t)
	repo := NewTaskLogRepo(db)
	ctx := context.Background()

	repo.Create(ctx, &model.ProjectJobTaskLogDO{ProjectID: "p1", JobID: "j1", TaskID: "t1", Log: "line 1"})
	repo.Create(ctx, &model.ProjectJobTaskLogDO{ProjectID: "p1", JobID: "j1", TaskID: "t1", Log: "line 2"})
	repo.Create(ctx, &model.ProjectJobTaskLogDO{ProjectID: "p1", JobID: "j1", TaskID: "t2", Log: "other task"})

	logs, err := repo.FindByTaskID(ctx, "p1", "j1", "t1")
	if err != nil {
		t.Fatalf("FindByTaskID failed: %v", err)
	}
	if len(logs) != 2 {
		t.Errorf("expected 2 logs for t1, got %d", len(logs))
	}
}

func TestTaskLogRepo_FindByTaskID_Empty(t *testing.T) {
	db := setupJobTestDB(t)
	repo := NewTaskLogRepo(db)

	logs, err := repo.FindByTaskID(context.Background(), "p1", "j1", "nonexistent")
	if err != nil {
		t.Fatalf("FindByTaskID failed: %v", err)
	}
	if len(logs) != 0 {
		t.Errorf("expected 0 logs, got %d", len(logs))
	}
}

// --- GraphRepo tests ---

func TestGraphRepo_FindByProjectAndGraphID(t *testing.T) {
	db := setupJobTestDB(t)
	repo := NewGraphRepo(db)
	ctx := context.Background()

	repo.Create(ctx, &model.ProjectGraphDO{ProjectID: "p1", GraphID: "g1", Name: "Graph 1"})
	repo.Create(ctx, &model.ProjectGraphDO{ProjectID: "p1", GraphID: "g2", Name: "Graph 2"})

	found, err := repo.FindByProjectAndGraphID(ctx, "p1", "g1")
	if err != nil {
		t.Fatalf("FindByProjectAndGraphID failed: %v", err)
	}
	if found.Name != "Graph 1" {
		t.Errorf("expected name 'Graph 1', got %q", found.Name)
	}

	_, err = repo.FindByProjectAndGraphID(ctx, "p1", "nonexistent")
	if err == nil {
		t.Error("expected error for non-existent graph")
	}
}

func TestGraphRepo_FindByProjectID(t *testing.T) {
	db := setupJobTestDB(t)
	repo := NewGraphRepo(db)
	ctx := context.Background()

	repo.Create(ctx, &model.ProjectGraphDO{ProjectID: "p1", GraphID: "g1", Name: "G1"})
	repo.Create(ctx, &model.ProjectGraphDO{ProjectID: "p1", GraphID: "g2", Name: "G2"})
	repo.Create(ctx, &model.ProjectGraphDO{ProjectID: "p2", GraphID: "g3", Name: "G3"})

	graphs, err := repo.FindByProjectID(ctx, "p1")
	if err != nil {
		t.Fatalf("FindByProjectID failed: %v", err)
	}
	if len(graphs) != 2 {
		t.Errorf("expected 2 graphs for p1, got %d", len(graphs))
	}
}

// --- GraphNodeRepo tests ---

func TestGraphNodeRepo_FindByGraphID(t *testing.T) {
	db := setupJobTestDB(t)
	repo := NewGraphNodeRepo(db)
	ctx := context.Background()

	repo.Create(ctx, &model.ProjectGraphNodeDO{ProjectID: "p1", GraphID: "g1", GraphNodeID: "n1", CodeName: "read_table"})
	repo.Create(ctx, &model.ProjectGraphNodeDO{ProjectID: "p1", GraphID: "g1", GraphNodeID: "n2", CodeName: "train"})
	repo.Create(ctx, &model.ProjectGraphNodeDO{ProjectID: "p1", GraphID: "g2", GraphNodeID: "n3", CodeName: "predict"})

	nodes, err := repo.FindByGraphID(ctx, "p1", "g1")
	if err != nil {
		t.Fatalf("FindByGraphID failed: %v", err)
	}
	if len(nodes) != 2 {
		t.Errorf("expected 2 nodes for g1, got %d", len(nodes))
	}
}

func TestGraphNodeRepo_FindByGraphNodeID(t *testing.T) {
	db := setupJobTestDB(t)
	repo := NewGraphNodeRepo(db)
	ctx := context.Background()

	repo.Create(ctx, &model.ProjectGraphNodeDO{ProjectID: "p1", GraphID: "g1", GraphNodeID: "n1", CodeName: "read_table", Label: "Read"})

	found, err := repo.FindByGraphNodeID(ctx, "p1", "g1", "n1")
	if err != nil {
		t.Fatalf("FindByGraphNodeID failed: %v", err)
	}
	if found.Label != "Read" {
		t.Errorf("expected label 'Read', got %q", found.Label)
	}

	_, err = repo.FindByGraphNodeID(ctx, "p1", "g1", "nonexistent")
	if err == nil {
		t.Error("expected error for non-existent node")
	}
}

func TestGraphNodeRepo_DeleteByGraphID(t *testing.T) {
	db := setupJobTestDB(t)
	repo := NewGraphNodeRepo(db)
	ctx := context.Background()

	repo.Create(ctx, &model.ProjectGraphNodeDO{ProjectID: "p1", GraphID: "g1", GraphNodeID: "n1"})
	repo.Create(ctx, &model.ProjectGraphNodeDO{ProjectID: "p1", GraphID: "g1", GraphNodeID: "n2"})
	repo.Create(ctx, &model.ProjectGraphNodeDO{ProjectID: "p1", GraphID: "g2", GraphNodeID: "n3"})

	err := repo.DeleteByGraphID(ctx, "p1", "g1")
	if err != nil {
		t.Fatalf("DeleteByGraphID failed: %v", err)
	}

	nodes, _ := repo.FindByGraphID(ctx, "p1", "g1")
	if len(nodes) != 0 {
		t.Errorf("expected 0 nodes after delete, got %d", len(nodes))
	}

	// g2 should be unaffected
	nodes, _ = repo.FindByGraphID(ctx, "p1", "g2")
	if len(nodes) != 1 {
		t.Errorf("expected 1 node for g2, got %d", len(nodes))
	}
}
