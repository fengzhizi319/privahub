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

func TestGraphService_StartGraph_Success(t *testing.T) {
	db := setupBugfix8TestDB(t)
	graphRepo := repository.NewGraphRepo(db)
	graphNodeRepo := repository.NewGraphNodeRepo(db)
	jobRepo := repository.NewJobRepo(db)
	taskRepo := repository.NewTaskRepo(db)
	taskLogRepo := repository.NewTaskLogRepo(db)

	svc := NewGraphService(graphRepo, graphNodeRepo, jobRepo, taskRepo, taskLogRepo, nil, db)
	ctx := context.Background()

	// Seed graph
	db.Create(&model.ProjectGraphDO{
		ProjectID: "proj-1",
		GraphID:   "graph-1",
		Name:      "test-graph",
		Edges:     "[]",
	})
	// Seed graph nodes
	db.Create(&model.ProjectGraphNodeDO{
		ProjectID:   "proj-1",
		GraphID:     "graph-1",
		GraphNodeID: "node-1",
		CodeName:    "read_data/datatable",
		Label:       "Read Data",
	})
	db.Create(&model.ProjectGraphNodeDO{
		ProjectID:   "proj-1",
		GraphID:     "graph-1",
		GraphNodeID: "node-2",
		CodeName:    "data_prep/psi",
		Label:       "PSI",
	})

	result, err := svc.StartGraph(ctx, &StartGraphRequest{
		ProjectID: "proj-1",
		GraphID:   "graph-1",
	})
	if err != nil {
		t.Fatalf("StartGraph failed: %v", err)
	}
	if result.JobID == "" {
		t.Fatal("expected non-empty job ID")
	}
	if result.Status != "RUNNING" {
		t.Fatalf("expected RUNNING status, got: %s", result.Status)
	}

	// Verify tasks were created
	var tasks []model.ProjectJobTaskDO
	db.Where("job_id = ?", result.JobID).Find(&tasks)
	if len(tasks) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(tasks))
	}
}

func TestGraphService_StartGraph_GraphNotFound(t *testing.T) {
	db := setupBugfix8TestDB(t)
	graphRepo := repository.NewGraphRepo(db)
	graphNodeRepo := repository.NewGraphNodeRepo(db)
	jobRepo := repository.NewJobRepo(db)
	taskRepo := repository.NewTaskRepo(db)
	taskLogRepo := repository.NewTaskLogRepo(db)

	svc := NewGraphService(graphRepo, graphNodeRepo, jobRepo, taskRepo, taskLogRepo, nil, db)
	ctx := context.Background()

	_, err := svc.StartGraph(ctx, &StartGraphRequest{
		ProjectID: "proj-1",
		GraphID:   "nonexistent",
	})
	if err != ErrGraphNotFound {
		t.Fatalf("expected ErrGraphNotFound, got: %v", err)
	}
}

// --- Bug 61: GraphService.StopGraph task error propagation ---

func TestGraphService_StopGraph_Success(t *testing.T) {
	db := setupBugfix8TestDB(t)
	graphRepo := repository.NewGraphRepo(db)
	graphNodeRepo := repository.NewGraphNodeRepo(db)
	jobRepo := repository.NewJobRepo(db)
	taskRepo := repository.NewTaskRepo(db)
	taskLogRepo := repository.NewTaskLogRepo(db)

	svc := NewGraphService(graphRepo, graphNodeRepo, jobRepo, taskRepo, taskLogRepo, nil, db)
	ctx := context.Background()

	// Seed job and tasks
	db.Create(&model.ProjectJobDO{
		ProjectID: "proj-1",
		JobID:     "job-1",
		Name:      "test-job",
		Status:    "RUNNING",
		GraphID:   "graph-1",
	})
	db.Create(&model.ProjectJobTaskDO{
		ProjectID:   "proj-1",
		JobID:       "job-1",
		TaskID:      "task-1",
		GraphNodeID: "node-1",
		Status:      "RUNNING",
	})
	db.Create(&model.ProjectJobTaskDO{
		ProjectID:   "proj-1",
		JobID:       "job-1",
		TaskID:      "task-2",
		GraphNodeID: "node-2",
		Status:      "PENDING",
	})
	db.Create(&model.ProjectJobTaskDO{
		ProjectID:   "proj-1",
		JobID:       "job-1",
		TaskID:      "task-3",
		GraphNodeID: "node-3",
		Status:      "SUCCEEDED",
	})

	err := svc.StopGraph(ctx, &StopGraphRequest{
		ProjectID: "proj-1",
		JobID:     "job-1",
	})
	if err != nil {
		t.Fatalf("StopGraph failed: %v", err)
	}

	// Verify job is stopped
	var job model.ProjectJobDO
	db.Where("job_id = ?", "job-1").First(&job)
	if job.Status != "STOPPED" {
		t.Fatalf("expected job STOPPED, got: %s", job.Status)
	}

	// Verify RUNNING/PENDING tasks are stopped, SUCCEEDED unchanged
	var tasks []model.ProjectJobTaskDO
	db.Where("job_id = ?", "job-1").Find(&tasks)
	statusMap := make(map[string]string)
	for _, task := range tasks {
		statusMap[task.TaskID] = task.Status
	}
	if statusMap["task-1"] != "STOPPED" {
		t.Fatalf("expected task-1 STOPPED, got: %s", statusMap["task-1"])
	}
	if statusMap["task-2"] != "STOPPED" {
		t.Fatalf("expected task-2 STOPPED, got: %s", statusMap["task-2"])
	}
	if statusMap["task-3"] != "SUCCEEDED" {
		t.Fatalf("expected task-3 SUCCEEDED unchanged, got: %s", statusMap["task-3"])
	}
}

func TestGraphService_StopGraph_JobNotFound(t *testing.T) {
	db := setupBugfix8TestDB(t)
	graphRepo := repository.NewGraphRepo(db)
	graphNodeRepo := repository.NewGraphNodeRepo(db)
	jobRepo := repository.NewJobRepo(db)
	taskRepo := repository.NewTaskRepo(db)
	taskLogRepo := repository.NewTaskLogRepo(db)

	svc := NewGraphService(graphRepo, graphNodeRepo, jobRepo, taskRepo, taskLogRepo, nil, db)
	ctx := context.Background()

	err := svc.StopGraph(ctx, &StopGraphRequest{
		ProjectID: "proj-1",
		JobID:     "nonexistent",
	})
	if err != ErrJobNotFound {
		t.Fatalf("expected ErrJobNotFound, got: %v", err)
	}
}
