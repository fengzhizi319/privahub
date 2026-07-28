package service

import (
	"context"
	"testing"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/internal/dao/repository"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupExtendedTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	err = db.AutoMigrate(
		&model.UserAccountsDO{},
		&model.UserTokensDO{},
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
		&model.InstDO{},
		&model.ProjectModelServingDO{},
		&model.ProjectModelPackDO{},
	)
	if err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}
	return db
}

// --- ProjectService Tests ---

func TestProjectService_CreateAndGet(t *testing.T) {
	db := setupExtendedTestDB(t)

	svc := NewProjectService(
		repository.NewProjectRepo(db),
		repository.NewProjectInstRepo(db),
		repository.NewProjectNodeRepo(db),
	)

	vo, err := svc.CreateProject(context.Background(), &CreateProjectRequest{
		Name:        "test-project",
		Description: "a test project",
		ComputeMode: "mpc",
		NodeIDs:     []string{"alice", "bob"},
	}, "admin")
	if err != nil {
		t.Fatalf("CreateProject failed: %v", err)
	}
	if vo.ProjectID == "" {
		t.Error("expected non-empty project ID")
	}
	if vo.Name != "test-project" {
		t.Errorf("expected name 'test-project', got %q", vo.Name)
	}

	// Get project
	got, err := svc.GetProject(context.Background(), vo.ProjectID)
	if err != nil {
		t.Fatalf("GetProject failed: %v", err)
	}
	if got.Name != "test-project" {
		t.Errorf("expected name 'test-project', got %q", got.Name)
	}
}

func TestProjectService_ListProjects(t *testing.T) {
	db := setupExtendedTestDB(t)

	svc := NewProjectService(
		repository.NewProjectRepo(db),
		repository.NewProjectInstRepo(db),
		repository.NewProjectNodeRepo(db),
	)

	// Create 3 projects
	for i := 0; i < 3; i++ {
		_, err := svc.CreateProject(context.Background(), &CreateProjectRequest{
			Name: "project-" + string(rune('a'+i)),
		}, "admin")
		if err != nil {
			t.Fatalf("CreateProject %d failed: %v", i, err)
		}
	}

	resp, err := svc.ListProjects(context.Background(), 1, 10, "")
	if err != nil {
		t.Fatalf("ListProjects failed: %v", err)
	}
	if resp.Total != 3 {
		t.Errorf("expected 3 projects, got %d", resp.Total)
	}
}

func TestProjectService_DeleteProject(t *testing.T) {
	db := setupExtendedTestDB(t)

	svc := NewProjectService(
		repository.NewProjectRepo(db),
		repository.NewProjectInstRepo(db),
		repository.NewProjectNodeRepo(db),
	)

	vo, _ := svc.CreateProject(context.Background(), &CreateProjectRequest{
		Name: "to-delete",
	}, "admin")

	err := svc.DeleteProject(context.Background(), vo.ProjectID)
	if err != nil {
		t.Fatalf("DeleteProject failed: %v", err)
	}

	_, err = svc.GetProject(context.Background(), vo.ProjectID)
	if err == nil {
		t.Error("expected error after deletion")
	}
}

func TestProjectService_AddNode(t *testing.T) {
	db := setupExtendedTestDB(t)

	svc := NewProjectService(
		repository.NewProjectRepo(db),
		repository.NewProjectInstRepo(db),
		repository.NewProjectNodeRepo(db),
	)

	vo, _ := svc.CreateProject(context.Background(), &CreateProjectRequest{
		Name: "node-test",
	}, "admin")

	err := svc.AddNode(context.Background(), vo.ProjectID, "alice")
	if err != nil {
		t.Fatalf("AddNode failed: %v", err)
	}

	// Verify node association
	var count int64
	db.Model(&model.ProjectNodeDO{}).Where("project_id = ? AND node_id = ?", vo.ProjectID, "alice").Count(&count)
	if count != 1 {
		t.Errorf("expected 1 project-node association, got %d", count)
	}
}

// --- GraphService Tests ---

func TestGraphService_CreateAndList(t *testing.T) {
	db := setupExtendedTestDB(t)

	svc := NewGraphService(
		repository.NewGraphRepo(db),
		repository.NewGraphNodeRepo(db),
		repository.NewJobRepo(db),
		repository.NewTaskRepo(db),
		repository.NewTaskLogRepo(db),
		nil,
	)

	vo, err := svc.CreateGraph(context.Background(), &CreateGraphRequest{
		ProjectID: "proj-1",
		Name:      "test-graph",
	})
	if err != nil {
		t.Fatalf("CreateGraph failed: %v", err)
	}
	if vo.GraphID == "" {
		t.Error("expected non-empty graph ID")
	}

	graphs, err := svc.ListGraph(context.Background(), &ListGraphRequest{
		ProjectID: "proj-1",
	})
	if err != nil {
		t.Fatalf("ListGraph failed: %v", err)
	}
	if len(graphs) != 1 {
		t.Errorf("expected 1 graph, got %d", len(graphs))
	}
}

func TestGraphService_DeleteGraph(t *testing.T) {
	db := setupExtendedTestDB(t)

	svc := NewGraphService(
		repository.NewGraphRepo(db),
		repository.NewGraphNodeRepo(db),
		repository.NewJobRepo(db),
		repository.NewTaskRepo(db),
		repository.NewTaskLogRepo(db),
		nil,
	)

	vo, _ := svc.CreateGraph(context.Background(), &CreateGraphRequest{
		ProjectID: "proj-1",
		Name:      "to-delete",
	})

	err := svc.DeleteGraph(context.Background(), &DeleteGraphRequest{
		ProjectID: "proj-1",
		GraphID:   vo.GraphID,
	})
	if err != nil {
		t.Fatalf("DeleteGraph failed: %v", err)
	}

	graphs, _ := svc.ListGraph(context.Background(), &ListGraphRequest{ProjectID: "proj-1"})
	if len(graphs) != 0 {
		t.Errorf("expected 0 graphs after deletion, got %d", len(graphs))
	}
}

func TestGraphService_UpdateMeta(t *testing.T) {
	db := setupExtendedTestDB(t)

	svc := NewGraphService(
		repository.NewGraphRepo(db),
		repository.NewGraphNodeRepo(db),
		repository.NewJobRepo(db),
		repository.NewTaskRepo(db),
		repository.NewTaskLogRepo(db),
		nil,
	)

	vo, _ := svc.CreateGraph(context.Background(), &CreateGraphRequest{
		ProjectID: "proj-1",
		Name:      "original",
	})

	err := svc.UpdateGraphMeta(context.Background(), &UpdateGraphMetaRequest{
		ProjectID: "proj-1",
		GraphID:   vo.GraphID,
		Name:      "renamed",
	})
	if err != nil {
		t.Fatalf("UpdateGraphMeta failed: %v", err)
	}

	detail, err := svc.GetGraphDetail(context.Background(), &GetGraphRequest{
		ProjectID: "proj-1",
		GraphID:   vo.GraphID,
	})
	if err != nil {
		t.Fatalf("GetGraphDetail failed: %v", err)
	}
	if detail.Name != "renamed" {
		t.Errorf("expected name 'renamed', got %q", detail.Name)
	}
}

// --- ModelService Tests ---

func TestModelService_ServingCRUD(t *testing.T) {
	db := setupExtendedTestDB(t)

	svc := NewModelService(db, nil)

	// Create serving
	vo, err := svc.CreateServing(context.Background(), &CreateServingRequest{
		ProjectID:          "proj-1",
		Initiator:          "alice",
		ServingInputConfig: `{"model_id": "m1"}`,
		Parties:            "alice,bob",
	})
	if err != nil {
		t.Fatalf("CreateServing failed: %v", err)
	}
	if vo.ServingID == "" {
		t.Error("expected non-empty serving ID")
	}

	// List servings
	list, err := svc.ListServings(context.Background(), &ServingListRequest{ProjectID: "proj-1"})
	if err != nil {
		t.Fatalf("ListServings failed: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("expected 1 serving, got %d", len(list))
	}

	// Get detail
	detail, err := svc.GetServingDetail(context.Background(), &ServingDetailRequest{ServingID: vo.ServingID})
	if err != nil {
		t.Fatalf("GetServingDetail failed: %v", err)
	}
	if detail.Initiator != "alice" {
		t.Errorf("expected initiator 'alice', got %q", detail.Initiator)
	}

	// Delete
	err = svc.DeleteServing(context.Background(), &DeleteServingRequest{ServingID: vo.ServingID})
	if err != nil {
		t.Fatalf("DeleteServing failed: %v", err)
	}

	_, err = svc.GetServingDetail(context.Background(), &ServingDetailRequest{ServingID: vo.ServingID})
	if err != ErrServingNotFound {
		t.Errorf("expected ErrServingNotFound, got %v", err)
	}
}

// --- mapServingState Tests ---

func TestMapServingState(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Pending", "pending"},
		{"Progressing", "progressing"},
		{"PartialAvailable", "partial_available"},
		{"Available", "available"},
		{"Failed", "failed"},
		{"Unknown", "Unknown"},
	}
	for _, tt := range tests {
		got := mapServingState(tt.input)
		if got != tt.expected {
			t.Errorf("mapServingState(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}
