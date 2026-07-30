package service

import (
	"context"
	"testing"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/internal/dao/repository"
	"github.com/robfig/cron/v3"
	"go.uber.org/zap"
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

// --- Bug 52: ModelService.GetModelPartyPath error propagation ---

func TestModelService_GetModelPartyPath_Success(t *testing.T) {
	db := setupBugfix7TestDB(t)
	svc := NewModelService(db, nil)
	ctx := context.Background()

	// Seed project node + datasource
	db.Create(&model.ProjectNodeDO{ProjectID: "proj-1", NodeID: "alice"})
	db.Create(&model.NodeDO{NodeID: "alice", Name: "Alice Node", ControlNodeID: "alice"})
	db.Create(&model.DatasourceDO{DatasourceID: "ds-1", Name: "test-ds", Type: "OSS", OwnerID: "alice"})

	result, err := svc.GetModelPartyPath(ctx, "proj-1", "node-1", "out-1")
	if err != nil {
		t.Fatalf("GetModelPartyPath failed: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 party, got %d", len(result))
	}
	if result[0].NodeID != "alice" {
		t.Errorf("expected node alice, got %s", result[0].NodeID)
	}
	if result[0].NodeName != "Alice Node" {
		t.Errorf("expected node name 'Alice Node', got %s", result[0].NodeName)
	}
}

// --- Bug 53: ModelService.CreateServing links model pack ---

func TestModelService_CreateServing_LinksModelPack(t *testing.T) {
	db := setupBugfix7TestDB(t)
	svc := NewModelService(db, nil)
	ctx := context.Background()

	// Seed a model pack
	pack := &model.ProjectModelPackDO{
		ProjectID: "proj-1",
		ModelID:   "model-1",
		Initiator: "alice",
		ModelName: "test-model",
		ModelList: `[{"model": "lr"}]`,
	}
	db.Create(pack)

	// Create serving
	vo, err := svc.CreateServing(ctx, &CreateServingRequest{
		ModelID:   "model-1",
		ProjectID: "proj-1",
	})
	if err != nil {
		t.Fatalf("CreateServing failed: %v", err)
	}
	if vo.ServingID == "" {
		t.Fatal("expected non-empty serving ID")
	}

	// Verify model pack was linked
	var updatedPack model.ProjectModelPackDO
	db.Where("model_id = ?", "model-1").First(&updatedPack)
	if updatedPack.ServingID != vo.ServingID {
		t.Errorf("model pack serving_id = %q, want %q", updatedPack.ServingID, vo.ServingID)
	}
}

// --- Bug 54: ScheduledService.ReRunScheduledTask stays RUNNING ---

func TestScheduledService_ReRunScheduledTask_StaysRunning(t *testing.T) {
	db := setupBugfix7TestDB(t)
	log := zap.NewNop()

	// Create a graph service with nil deps (StartGraph will fail gracefully)
	graphSvc := NewGraphService(
		repository.NewGraphRepo(db),
		repository.NewGraphNodeRepo(db),
		repository.NewJobRepo(db),
		repository.NewTaskRepo(db),
		repository.NewTaskLogRepo(db),
		nil, // no kuscia client
		db,
	)

	svc := &ScheduledService{
		db:           db,
		cron:         nil, // not needed for this test
		log:          log,
		graphService: graphSvc,
		entries:      make(map[string]cron.EntryID),
	}

	ctx := context.Background()
	task := &model.ProjectScheduleTaskDO{
		ProjectID:      "proj-1",
		GraphID:        "graph-1",
		ScheduleID:     "sched-1",
		ScheduleTaskID: "task-1",
		Cron:           "0 0 * * * *",
		Status:         model.ScheduledStatusToBeRun,
	}
	db.Create(task)

	// ReRun: graphService is set but StartGraph will fail (no graph found).
	// The task should be marked FAILED (not SUCCESS).
	err := svc.ReRunScheduledTask(ctx, &TaskReRunScheduledRequest{
		ScheduleID:     "sched-1",
		ScheduleTaskID: "task-1",
	})
	if err != nil {
		t.Fatalf("ReRunScheduledTask failed: %v", err)
	}

	var updated model.ProjectScheduleTaskDO
	db.Where("schedule_task_id = ?", "task-1").First(&updated)
	// With graphService set and StartGraph failing, status should be FAILED
	if updated.Status != model.ScheduledStatusFailed {
		t.Errorf("status = %q, want %q", updated.Status, model.ScheduledStatusFailed)
	}
}

func TestScheduledService_ReRunScheduledTask_NoGraphService_MarksSuccess(t *testing.T) {
	db := setupBugfix7TestDB(t)
	log := zap.NewNop()

	svc := &ScheduledService{
		db:           db,
		cron:         nil,
		log:          log,
		graphService: nil, // no graph service → synchronous no-op → SUCCESS
		entries:      make(map[string]cron.EntryID),
	}

	ctx := context.Background()
	task := &model.ProjectScheduleTaskDO{
		ProjectID:      "proj-1",
		GraphID:        "graph-1",
		ScheduleID:     "sched-1",
		ScheduleTaskID: "task-2",
		Cron:           "0 0 * * * *",
		Status:         model.ScheduledStatusToBeRun,
	}
	db.Create(task)

	err := svc.ReRunScheduledTask(ctx, &TaskReRunScheduledRequest{
		ScheduleID:     "sched-1",
		ScheduleTaskID: "task-2",
	})
	if err != nil {
		t.Fatalf("ReRunScheduledTask failed: %v", err)
	}

	var updated model.ProjectScheduleTaskDO
	db.Where("schedule_task_id = ?", "task-2").First(&updated)
	if updated.Status != model.ScheduledStatusSuccess {
		t.Errorf("status = %q, want %q", updated.Status, model.ScheduledStatusSuccess)
	}
}

// --- Bug 55: ScheduledService.GetScheduledOnceSuccess error propagation ---

func TestScheduledService_GetScheduledOnceSuccess_Found(t *testing.T) {
	db := setupBugfix7TestDB(t)
	log := zap.NewNop()
	svc := &ScheduledService{
		db:      db,
		log:     log,
		entries: make(map[string]cron.EntryID),
	}

	ctx := context.Background()
	db.Create(&model.ProjectScheduleTaskDO{
		ProjectID:      "proj-1",
		GraphID:        "graph-1",
		ScheduleID:     "sched-1",
		ScheduleTaskID: "task-ok",
		Cron:           "0 0 * * * *",
		Status:         model.ScheduledStatusSuccess,
	})

	found, err := svc.GetScheduledOnceSuccess(ctx, &ScheduledGraphOnceSuccessRequest{
		ProjectID: "proj-1",
		GraphID:   "graph-1",
	})
	if err != nil {
		t.Fatalf("GetScheduledOnceSuccess failed: %v", err)
	}
	if !found {
		t.Error("expected found=true for existing SUCCESS task")
	}
}

func TestScheduledService_GetScheduledOnceSuccess_NotFound(t *testing.T) {
	db := setupBugfix7TestDB(t)
	log := zap.NewNop()
	svc := &ScheduledService{
		db:      db,
		log:     log,
		entries: make(map[string]cron.EntryID),
	}

	ctx := context.Background()
	found, err := svc.GetScheduledOnceSuccess(ctx, &ScheduledGraphOnceSuccessRequest{
		ProjectID: "proj-x",
		GraphID:   "graph-x",
	})
	if err != nil {
		t.Fatalf("GetScheduledOnceSuccess failed: %v", err)
	}
	if found {
		t.Error("expected found=false for non-existing task")
	}
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

// --- Bug 58: DatasourceService.GetDatasourceDetail error propagation ---

func TestDatasourceService_GetDatasourceDetail_Success(t *testing.T) {
	db := setupBugfix7TestDB(t)
	svc := NewDatasourceService(db, nil)
	ctx := context.Background()

	// Seed datasource + node association
	db.Create(&model.DatasourceDO{
		DatasourceID: "ds-1",
		Name:         "test-ds",
		Type:         "OSS",
		Status:       "Available",
		OwnerID:      "alice",
	})
	db.Create(&model.DatasourceNodeDO{DatasourceID: "ds-1", NodeID: "alice"})
	db.Create(&model.NodeDO{NodeID: "alice", Name: "Alice", ControlNodeID: "alice"})

	detail, err := svc.GetDatasourceDetail(ctx, &DatasourceDetailRequest{DatasourceID: "ds-1"})
	if err != nil {
		t.Fatalf("GetDatasourceDetail failed: %v", err)
	}
	if detail.DatasourceID != "ds-1" {
		t.Errorf("datasource_id = %q, want ds-1", detail.DatasourceID)
	}
	if len(detail.NodeIDs) != 1 || detail.NodeIDs[0] != "alice" {
		t.Errorf("node_ids = %v, want [alice]", detail.NodeIDs)
	}
}

func TestDatasourceService_GetDatasourceDetail_NotFound(t *testing.T) {
	db := setupBugfix7TestDB(t)
	svc := NewDatasourceService(db, nil)
	ctx := context.Background()

	_, err := svc.GetDatasourceDetail(ctx, &DatasourceDetailRequest{DatasourceID: "nonexist"})
	if err != ErrDatasourceNotFound {
		t.Errorf("expected ErrDatasourceNotFound, got %v", err)
	}
}
