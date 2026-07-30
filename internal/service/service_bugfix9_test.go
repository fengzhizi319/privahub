package service

// Regression tests for Bug73–Bug79 (round 17).
//
// Bug73: ListNodeStatus ignored FindByGraphID error in no-job path.
// Bug74: ListNodeStatus ignored FindByProjectAndJobID error for tasks.
// Bug75: latestJobIDForGraph ignored FindByProjectID error.
// Bug76: GetNodeLogs ignored FindByJobID error.
// Bug77: RefreshNodeMaxIndex ignored FindByGraphID error.
// Bug78: wire.go passed nil logger to NewSseServer.
// Bug79: wire.go Shutdown() did not stop SseServer heartbeat goroutine.

import (
	"context"
	"testing"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/internal/dao/repository"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// setupBug73TestDB creates an in-memory SQLite DB with graph-related tables.
func setupBug73TestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&model.ProjectGraphDO{},
		&model.ProjectGraphNodeDO{},
		&model.ProjectJobDO{},
		&model.ProjectJobTaskDO{},
		&model.ProjectJobTaskLogDO{},
	))
	return db
}

func newBug73GraphService(db *gorm.DB) *GraphService {
	return NewGraphService(
		repository.NewGraphRepo(db),
		repository.NewGraphNodeRepo(db),
		repository.NewJobRepo(db),
		repository.NewTaskRepo(db),
		repository.NewTaskLogRepo(db),
		nil,
		db,
	)
}

// closeDB closes the underlying sql.DB to simulate database failure.
func closeDB(t *testing.T, db *gorm.DB) {
	t.Helper()
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())
}

// --- Bug73: ListNodeStatus must return error when DB fails in no-job path ---

func TestBug73_ListNodeStatus_DBError_NoJob(t *testing.T) {
	db := setupBug73TestDB(t)
	svc := newBug73GraphService(db)
	ctx := context.Background()

	// Seed graph + nodes but no jobs
	db.Create(&model.ProjectGraphDO{ProjectID: "p1", GraphID: "g1", Name: "G"})
	db.Create(&model.ProjectGraphNodeDO{ProjectID: "p1", GraphID: "g1", GraphNodeID: "n1", CodeName: "x"})

	// Close DB to simulate failure
	closeDB(t, db)

	// Without Bug73 fix, this returned fake IDLE status; now must return error.
	_, err := svc.ListNodeStatus(ctx, &ListGraphNodeStatusRequest{
		ProjectID: "p1",
		GraphID:   "g1",
	})
	assert.Error(t, err, "Bug73: ListNodeStatus should propagate DB error in no-job path")
}

// --- Bug74: ListNodeStatus must return error when task query fails ---

func TestBug74_ListNodeStatus_DBError_TaskQuery(t *testing.T) {
	db := setupBug73TestDB(t)
	svc := newBug73GraphService(db)
	ctx := context.Background()

	// Seed graph + job so we enter the task-query path
	db.Create(&model.ProjectGraphDO{ProjectID: "p1", GraphID: "g1", Name: "G"})
	db.Create(&model.ProjectJobDO{ProjectID: "p1", JobID: "j1", GraphID: "g1", Status: "RUNNING"})

	// Close DB after seeding job but before task query
	closeDB(t, db)

	_, err := svc.ListNodeStatus(ctx, &ListGraphNodeStatusRequest{
		ProjectID: "p1",
		GraphID:   "g1",
		JobID:     "j1",
	})
	assert.Error(t, err, "Bug74: ListNodeStatus should propagate DB error on task query")
}

// --- Bug75: latestJobIDForGraph must return error on DB failure ---

func TestBug75_LatestJobIDForGraph_DBError(t *testing.T) {
	db := setupBug73TestDB(t)
	svc := newBug73GraphService(db)
	ctx := context.Background()

	closeDB(t, db)

	jobID, err := svc.latestJobIDForGraph(ctx, "p1", "g1")
	assert.Error(t, err, "Bug75: latestJobIDForGraph should return error on DB failure")
	assert.Empty(t, jobID)
}

func TestBug75_LatestJobIDForGraph_NoJob(t *testing.T) {
	db := setupBug73TestDB(t)
	svc := newBug73GraphService(db)
	ctx := context.Background()

	// No jobs seeded — should return empty string without error
	jobID, err := svc.latestJobIDForGraph(ctx, "p1", "g1")
	assert.NoError(t, err)
	assert.Empty(t, jobID)
}

func TestBug75_LatestJobIDForGraph_Found(t *testing.T) {
	db := setupBug73TestDB(t)
	svc := newBug73GraphService(db)
	ctx := context.Background()

	db.Create(&model.ProjectJobDO{ProjectID: "p1", JobID: "j1", GraphID: "g1", Status: "SUCCEEDED"})
	db.Create(&model.ProjectJobDO{ProjectID: "p1", JobID: "j2", GraphID: "g2", Status: "RUNNING"})

	jobID, err := svc.latestJobIDForGraph(ctx, "p1", "g1")
	assert.NoError(t, err)
	assert.Equal(t, "j1", jobID)
}

// --- Bug76: GetNodeLogs must return error when task lookup fails ---

func TestBug76_GetNodeLogs_DBError(t *testing.T) {
	db := setupBug73TestDB(t)
	svc := newBug73GraphService(db)
	ctx := context.Background()

	// Seed graph node so FindByGraphNodeID succeeds initially
	db.Create(&model.ProjectGraphNodeDO{
		ProjectID: "p1", GraphID: "g1", GraphNodeID: "n1", CodeName: "x",
	})

	closeDB(t, db)

	// With JobID set but no TaskID, the code enters FindByJobID path
	_, err := svc.GetNodeLogs(ctx, &GraphNodeLogsRequest{
		ProjectID:   "p1",
		GraphID:     "g1",
		GraphNodeID: "n1",
		JobID:       "j1",
	})
	assert.Error(t, err, "Bug76: GetNodeLogs should propagate DB error from FindByJobID")
}

// --- Bug77: RefreshNodeMaxIndex must return error when node query fails ---

func TestBug77_RefreshNodeMaxIndex_DBError(t *testing.T) {
	db := setupBug73TestDB(t)
	svc := newBug73GraphService(db)
	ctx := context.Background()

	db.Create(&model.ProjectGraphDO{ProjectID: "p1", GraphID: "g1", Name: "G", NodeMaxIndex: 0})

	closeDB(t, db)

	_, err := svc.RefreshNodeMaxIndex(ctx, &RefreshNodeMaxIndexRequest{
		ProjectID: "p1",
		GraphID:   "g1",
	})
	assert.Error(t, err, "Bug77: RefreshNodeMaxIndex should propagate DB error from FindByGraphID")
}

// --- Bug78 + Bug79: SseServer logger and shutdown ---

func TestBug78_SseServer_WithLogger(t *testing.T) {
	// Bug78: NewSseServer should accept a non-nil logger without panic.
	log := zap.NewNop()
	sse := NewSseServer(log)
	require.NotNil(t, sse)
	assert.Equal(t, 0, sse.ActiveConnections())
	sse.Stop()
}

func TestBug79_SseServer_StopIdempotent(t *testing.T) {
	// Bug79: Stop must be safe to call multiple times (stopOnce guard).
	sse := NewSseServer(zap.NewNop())
	sse.Stop()
	sse.Stop() // second call must not panic
	assert.Equal(t, 0, sse.ActiveConnections())
}
