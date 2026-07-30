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
		&model.ProjectDatatableDO{},
		&model.DatasourceDO{},
		&model.DatasourceNodeDO{},
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
		repository.NewDatatableRepo(db),
		db,
		nil,
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
		repository.NewDatatableRepo(db),
		db,
		nil,
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
		repository.NewDatatableRepo(db),
		db,
		nil,
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
		repository.NewDatatableRepo(db),
		db,
		nil,
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

func newProjectServiceForTest(db *gorm.DB) *ProjectService {
	return NewProjectService(
		repository.NewProjectRepo(db),
		repository.NewProjectInstRepo(db),
		repository.NewProjectNodeRepo(db),
		repository.NewDatatableRepo(db),
		db,
		nil,
	)
}

func TestFirstNonEmpty(t *testing.T) {
	if got := firstNonEmpty("", "", "x"); got != "x" {
		t.Errorf("firstNonEmpty(\"\",\"\",\"x\") = %q, want \"x\"", got)
	}
	if got := firstNonEmpty("a", "b"); got != "a" {
		t.Errorf("firstNonEmpty(\"a\",\"b\") = %q, want \"a\"", got)
	}
	if got := firstNonEmpty(); got != "" {
		t.Errorf("firstNonEmpty() = %q, want \"\"", got)
	}
}

func TestProjectService_AddDatatable(t *testing.T) {
	db := setupExtendedTestDB(t)
	svc := newProjectServiceForTest(db)

	req := &AddDatatableRequest{
		ProjectID:   "proj-1",
		NodeID:      "alice",
		DatatableID: "dt-1",
		Configs:     []byte(`[{"colName":"id","isLabelKey":true}]`),
	}
	if err := svc.AddDatatable(context.Background(), req); err != nil {
		t.Fatalf("AddDatatable failed: %v", err)
	}

	var dt model.ProjectDatatableDO
	if err := db.Where("project_id = ? AND node_id = ? AND datatable_id = ?", "proj-1", "alice", "dt-1").First(&dt).Error; err != nil {
		t.Fatalf("expected persisted project_datatable row: %v", err)
	}
	if dt.TableConfigs != `[{"colName":"id","isLabelKey":true}]` {
		t.Errorf("unexpected table_configs: %q", dt.TableConfigs)
	}
	if dt.Source != "IMPORTED" {
		t.Errorf("expected source IMPORTED, got %q", dt.Source)
	}

	// Idempotent re-add updates configs without creating a duplicate row.
	req.Configs = []byte(`[{"colName":"name"}]`)
	if err := svc.AddDatatable(context.Background(), req); err != nil {
		t.Fatalf("re-AddDatatable failed: %v", err)
	}
	var count int64
	db.Model(&model.ProjectDatatableDO{}).Where("project_id = ? AND node_id = ? AND datatable_id = ?", "proj-1", "alice", "dt-1").Count(&count)
	if count != 1 {
		t.Errorf("expected exactly 1 row after idempotent re-add, got %d", count)
	}
	var dt2 model.ProjectDatatableDO
	db.Where("project_id = ? AND node_id = ? AND datatable_id = ?", "proj-1", "alice", "dt-1").First(&dt2)
	if dt2.TableConfigs != `[{"colName":"name"}]` {
		t.Errorf("expected updated configs, got %q", dt2.TableConfigs)
	}
}

func TestProjectService_AddDatatable_MissingIDs(t *testing.T) {
	db := setupExtendedTestDB(t)
	svc := newProjectServiceForTest(db)

	err := svc.AddDatatable(context.Background(), &AddDatatableRequest{ProjectID: "proj-1"})
	if err != ErrProjectDatatableInvalid {
		t.Errorf("expected ErrProjectDatatableInvalid, got %v", err)
	}
}

func TestProjectService_DeleteDatatable(t *testing.T) {
	db := setupExtendedTestDB(t)
	svc := newProjectServiceForTest(db)

	_ = svc.AddDatatable(context.Background(), &AddDatatableRequest{
		ProjectID: "proj-1", NodeID: "alice", DatatableID: "dt-1",
	})

	err := svc.DeleteDatatable(context.Background(), &ProjDeleteDatatableRequest{
		ProjectID: "proj-1", NodeID: "alice", DatatableID: "dt-1",
	})
	if err != nil {
		t.Fatalf("DeleteDatatable failed: %v", err)
	}

	var count int64
	db.Model(&model.ProjectDatatableDO{}).Where("project_id = ? AND node_id = ? AND datatable_id = ?", "proj-1", "alice", "dt-1").Count(&count)
	if count != 0 {
		t.Errorf("expected 0 rows after deletion, got %d", count)
	}

	// Idempotent: deleting a non-existent association succeeds.
	if err := svc.DeleteDatatable(context.Background(), &ProjDeleteDatatableRequest{
		ProjectID: "x", NodeID: "y", DatatableID: "z",
	}); err != nil {
		t.Errorf("idempotent delete should succeed, got %v", err)
	}
}

func TestProjectService_UpdateTableConfig(t *testing.T) {
	db := setupExtendedTestDB(t)
	svc := newProjectServiceForTest(db)

	// Upsert: config update on a missing association creates the row.
	req := &AddDatatableRequest{
		ProjectID: "proj-1", NodeID: "alice", DatatableID: "dt-1",
		Configs: []byte(`[{"colName":"a"}]`),
	}
	if err := svc.UpdateTableConfig(context.Background(), req); err != nil {
		t.Fatalf("UpdateTableConfig (upsert) failed: %v", err)
	}
	var count int64
	db.Model(&model.ProjectDatatableDO{}).Where("project_id = ? AND node_id = ? AND datatable_id = ?", "proj-1", "alice", "dt-1").Count(&count)
	if count != 1 {
		t.Fatalf("expected 1 row after upsert, got %d", count)
	}

	// Update existing association's configs.
	req.Configs = []byte(`[{"colName":"b"}]`)
	if err := svc.UpdateTableConfig(context.Background(), req); err != nil {
		t.Fatalf("UpdateTableConfig (update) failed: %v", err)
	}
	var dt model.ProjectDatatableDO
	db.Where("project_id = ? AND node_id = ? AND datatable_id = ?", "proj-1", "alice", "dt-1").First(&dt)
	if dt.TableConfigs != `[{"colName":"b"}]` {
		t.Errorf("expected updated configs, got %q", dt.TableConfigs)
	}
}

func TestNodeService_ListTeeNodes(t *testing.T) {
	db := setupExtendedTestDB(t)
	svc := NewNodeService(repository.NewNodeRepo(db), repository.NewNodeRouteRepo(db), nil)

	// mode 0 = mpc-only (excluded), mode 1 = tee-only, mode 2 = mpc&tee (both included)
	for _, n := range []CreateNodeRequest{
		{NodeID: "mpc-node", Name: "mpc", Mode: 0},
		{NodeID: "tee-node", Name: "tee", Mode: 1},
		{NodeID: "hybrid-node", Name: "hybrid", Mode: 2},
	} {
		if _, err := svc.CreateNode(context.Background(), &n); err != nil {
			t.Fatalf("CreateNode(%s) failed: %v", n.NodeID, err)
		}
	}

	teeNodes, err := svc.ListTeeNodes(context.Background())
	if err != nil {
		t.Fatalf("ListTeeNodes failed: %v", err)
	}
	if len(teeNodes) != 2 {
		t.Fatalf("expected 2 tee-capable nodes, got %d", len(teeNodes))
	}
	ids := map[string]bool{}
	for _, n := range teeNodes {
		ids[n.NodeID] = true
	}
	if !ids["tee-node"] || !ids["hybrid-node"] {
		t.Errorf("expected tee-node and hybrid-node, got %v", ids)
	}
	if ids["mpc-node"] {
		t.Errorf("mpc-only node must not be tee-capable")
	}
}

func TestProjectService_ListProjectDatasources(t *testing.T) {
	db := setupExtendedTestDB(t)
	svc := newProjectServiceForTest(db)

	vo, err := svc.CreateProject(context.Background(), &CreateProjectRequest{
		Name: "p1", NodeIDs: []string{"alice"},
	}, "admin")
	if err != nil {
		t.Fatalf("CreateProject failed: %v", err)
	}

	// Node name source.
	if err := db.Create(&model.NodeDO{NodeID: "alice", Name: "Alice Node", ControlNodeID: "alice", Type: "normal", MasterNodeID: "master"}).Error; err != nil {
		t.Fatalf("create node failed: %v", err)
	}
	// A datasource bound to node alice.
	if err := db.Create(&model.DatasourceDO{DatasourceID: "ds1", Name: "DS One", Type: "OSS", Status: "Available", OwnerID: "admin"}).Error; err != nil {
		t.Fatalf("create datasource failed: %v", err)
	}
	if err := db.Create(&model.DatasourceNodeDO{DatasourceID: "ds1", NodeID: "alice"}).Error; err != nil {
		t.Fatalf("create datasource-node failed: %v", err)
	}

	result, err := svc.ListProjectDatasources(context.Background(), vo.ProjectID)
	if err != nil {
		t.Fatalf("ListProjectDatasources failed: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 node grouping, got %d", len(result))
	}
	if result[0].NodeID != "alice" {
		t.Errorf("expected nodeId alice, got %q", result[0].NodeID)
	}
	if result[0].NodeName != "Alice Node" {
		t.Errorf("expected nodeName 'Alice Node', got %q", result[0].NodeName)
	}
	if len(result[0].DataSources) != 1 {
		t.Fatalf("expected 1 datasource, got %d", len(result[0].DataSources))
	}
	ds := result[0].DataSources[0]
	if ds.DataSourceID != "ds1" || ds.DataSourceName != "DS One" || ds.Type != "OSS" {
		t.Errorf("unexpected datasource item: %+v", ds)
	}
}

func TestProjectService_GetProjectOutTable(t *testing.T) {
	db := setupExtendedTestDB(t)
	svc := newProjectServiceForTest(db)

	vo, err := svc.CreateProject(context.Background(), &CreateProjectRequest{
		Name: "out-proj", NodeIDs: []string{"alice"},
	}, "admin")
	if err != nil {
		t.Fatalf("CreateProject failed: %v", err)
	}
	pid := vo.ProjectID

	// One graph, one job.
	if err := db.Create(&model.ProjectGraphDO{ProjectID: pid, GraphID: "g1", Name: "graph-1"}).Error; err != nil {
		t.Fatalf("create graph failed: %v", err)
	}
	if err := db.Create(&model.ProjectJobDO{ProjectID: pid, JobID: "j1", Name: "job-1", Status: "Succeed", GraphID: "g1"}).Error; err != nil {
		t.Fatalf("create job failed: %v", err)
	}
	// Graph node with declared outputs and one without.
	if err := db.Create(&model.ProjectGraphNodeDO{ProjectID: pid, GraphID: "g1", GraphNodeID: "n1", CodeName: "data_prep", Outputs: `["out_table_a","out_table_b"]`}).Error; err != nil {
		t.Fatalf("create graph node n1 failed: %v", err)
	}
	if err := db.Create(&model.ProjectGraphNodeDO{ProjectID: pid, GraphID: "g1", GraphNodeID: "n2", CodeName: "read_data", Outputs: ``}).Error; err != nil {
		t.Fatalf("create graph node n2 failed: %v", err)
	}

	out, err := svc.GetProjectOutTable(context.Background(), pid, "")
	if err != nil {
		t.Fatalf("GetProjectOutTable failed: %v", err)
	}
	if out.ProjectID != pid || out.ProjectName != "out-proj" {
		t.Errorf("unexpected project meta: %+v", out)
	}
	if out.GraphCount != 1 || out.JobCount != 1 {
		t.Errorf("expected graphCount=1 jobCount=1, got %d/%d", out.GraphCount, out.JobCount)
	}
	if len(out.Nodes) != 1 {
		t.Fatalf("expected 1 output-bearing node, got %d", len(out.Nodes))
	}
	if out.Nodes[0].GraphNodeID != "n1" || len(out.Nodes[0].Outputs) != 2 {
		t.Errorf("unexpected node output: %+v", out.Nodes[0])
	}

	// Filtering by a non-existent graph yields no nodes.
	out2, err := svc.GetProjectOutTable(context.Background(), pid, "no-such-graph")
	if err != nil {
		t.Fatalf("GetProjectOutTable (filtered) failed: %v", err)
	}
	if len(out2.Nodes) != 0 {
		t.Errorf("expected 0 nodes for unknown graph, got %d", len(out2.Nodes))
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
	if detail.ServingID != vo.ServingID {
		t.Errorf("expected serving ID %q, got %q", vo.ServingID, detail.ServingID)
	}
	if len(detail.ServingDetails) != 2 {
		t.Errorf("expected 2 serving detail parties, got %d", len(detail.ServingDetails))
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
