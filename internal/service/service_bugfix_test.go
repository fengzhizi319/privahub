package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/internal/dao/repository"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupBugfixTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	err = db.AutoMigrate(
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
		&model.ProjectDatatableDO{},
		&model.VoteRequestDO{},
		&model.VoteInviteDO{},
	)
	if err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}
	return db
}

// --- Bug 16: VoteService JSON injection prevention ---

func TestVoteService_CreateVote_SpecialCharacters(t *testing.T) {
	db := setupBugfixTestDB(t)

	svc := NewVoteService(
		repository.NewVoteRequestRepo(db),
		repository.NewVoteInviteRepo(db),
		db,
	)

	// Voters with JSON special characters that would break manual concatenation
	voters := []string{`alice"inject`, `bob\backslash`, "charlie\nnewline"}
	vo, err := svc.CreateVote(context.Background(), &CreateVoteRequest{
		Initiator: "admin",
		Type:      "PROJECT_CREATE",
		Voters:    voters,
		Executors: []string{`executor"with"quotes`},
	})
	if err != nil {
		t.Fatalf("CreateVote failed: %v", err)
	}

	// Verify the voters JSON is valid and round-trips correctly
	var parsedVoters []string
	if err := json.Unmarshal([]byte(vo.Voters), &parsedVoters); err != nil {
		t.Fatalf("Voters JSON is invalid: %v (raw: %s)", err, vo.Voters)
	}
	if len(parsedVoters) != 3 {
		t.Fatalf("expected 3 voters, got %d", len(parsedVoters))
	}
	for i, v := range voters {
		if parsedVoters[i] != v {
			t.Errorf("voter[%d] = %q, want %q", i, parsedVoters[i], v)
		}
	}

	// Verify executors JSON is valid
	var parsedExecutors []string
	if err := json.Unmarshal([]byte(vo.Executors), &parsedExecutors); err != nil {
		t.Fatalf("Executors JSON is invalid: %v (raw: %s)", err, vo.Executors)
	}
	if len(parsedExecutors) != 1 || parsedExecutors[0] != `executor"with"quotes` {
		t.Errorf("unexpected executors: %v", parsedExecutors)
	}
}

func TestVoteService_CreateVote_EmptyArrays(t *testing.T) {
	db := setupBugfixTestDB(t)

	svc := NewVoteService(
		repository.NewVoteRequestRepo(db),
		repository.NewVoteInviteRepo(db),
		db,
	)

	vo, err := svc.CreateVote(context.Background(), &CreateVoteRequest{
		Initiator: "admin",
		Type:      "TEST",
		Voters:    []string{},
		Executors: nil,
	})
	if err != nil {
		t.Fatalf("CreateVote with empty arrays failed: %v", err)
	}

	// Empty slice marshals to "[]", nil marshals to "null"
	if vo.Voters != "[]" {
		t.Errorf("expected voters '[]', got %q", vo.Voters)
	}
	if vo.Executors != "null" {
		t.Errorf("expected executors 'null', got %q", vo.Executors)
	}
}

// --- Bug 17: FullUpdateGraph atomicity ---

func TestGraphService_FullUpdateGraph_Atomic(t *testing.T) {
	db := setupBugfixTestDB(t)

	svc := NewGraphService(
		repository.NewGraphRepo(db),
		repository.NewGraphNodeRepo(db),
		repository.NewJobRepo(db),
		repository.NewTaskRepo(db),
		repository.NewTaskLogRepo(db),
		nil,
		db,
	)

	// Create a graph with initial nodes
	graph, err := svc.CreateGraph(context.Background(), &CreateGraphRequest{
		ProjectID: "proj-atomic",
		Name:      "atomic-test",
	})
	if err != nil {
		t.Fatalf("CreateGraph failed: %v", err)
	}

	// Full update with valid nodes
	err = svc.FullUpdateGraph(context.Background(), &FullUpdateGraphRequest{
		ProjectID: "proj-atomic",
		GraphID:   graph.GraphID,
		Edges:     json.RawMessage(`[{"source":"n1","target":"n2"}]`),
		Nodes: []GraphNodeReq{
			{GraphNodeID: "n1", CodeName: "read_data/datatable", Label: "Node 1", X: 10, Y: 20},
			{GraphNodeID: "n2", CodeName: "lr/train", Label: "Node 2", X: 30, Y: 40},
		},
	})
	if err != nil {
		t.Fatalf("FullUpdateGraph failed: %v", err)
	}

	// Verify nodes were persisted
	detail, err := svc.GetGraphDetail(context.Background(), &GetGraphRequest{
		ProjectID: "proj-atomic",
		GraphID:   graph.GraphID,
	})
	if err != nil {
		t.Fatalf("GetGraphDetail failed: %v", err)
	}
	if len(detail.Nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(detail.Nodes))
	}
	if detail.Nodes[0].GraphNodeID != "n1" || detail.Nodes[1].GraphNodeID != "n2" {
		t.Errorf("unexpected node IDs: %v, %v", detail.Nodes[0].GraphNodeID, detail.Nodes[1].GraphNodeID)
	}

	// Second update replaces all nodes atomically
	err = svc.FullUpdateGraph(context.Background(), &FullUpdateGraphRequest{
		ProjectID: "proj-atomic",
		GraphID:   graph.GraphID,
		Edges:     json.RawMessage(`[]`),
		Nodes: []GraphNodeReq{
			{GraphNodeID: "n3", CodeName: "psi/psi", Label: "Node 3", X: 50, Y: 60},
		},
	})
	if err != nil {
		t.Fatalf("Second FullUpdateGraph failed: %v", err)
	}

	detail, _ = svc.GetGraphDetail(context.Background(), &GetGraphRequest{
		ProjectID: "proj-atomic",
		GraphID:   graph.GraphID,
	})
	if len(detail.Nodes) != 1 {
		t.Fatalf("expected 1 node after replacement, got %d", len(detail.Nodes))
	}
	if detail.Nodes[0].GraphNodeID != "n3" {
		t.Errorf("expected node n3, got %q", detail.Nodes[0].GraphNodeID)
	}
}

// --- Bug 18: CreateRoute orphan prevention ---

func TestNodeService_CreateRoute_Success(t *testing.T) {
	db := setupBugfixTestDB(t)

	svc := NewNodeService(
		repository.NewNodeRepo(db),
		repository.NewNodeRouteRepo(db),
		nil,
		db,
	)

	err := svc.CreateRoute(context.Background(), &CreateRouteRequest{
		SrcNodeID:     "alice",
		DstNodeID:     "bob",
		SrcNetAddress: "alice:8080",
		DstNetAddress: "bob:8080",
	})
	if err != nil {
		t.Fatalf("CreateRoute failed: %v", err)
	}

	// Verify forward route exists
	routes, err := svc.ListRoutes(context.Background(), "alice")
	if err != nil {
		t.Fatalf("ListRoutes failed: %v", err)
	}
	if len(routes) != 1 {
		t.Fatalf("expected 1 forward route, got %d", len(routes))
	}
	if routes[0].DstNodeID != "bob" {
		t.Errorf("expected dst 'bob', got %q", routes[0].DstNodeID)
	}

	// Verify reverse route exists
	reverseRoutes, _ := svc.ListRoutes(context.Background(), "bob")
	if len(reverseRoutes) != 1 {
		t.Fatalf("expected 1 reverse route, got %d", len(reverseRoutes))
	}
	if reverseRoutes[0].DstNodeID != "alice" {
		t.Errorf("expected reverse dst 'alice', got %q", reverseRoutes[0].DstNodeID)
	}
}

func TestNodeService_CreateRoute_DuplicateRejected(t *testing.T) {
	db := setupBugfixTestDB(t)

	svc := NewNodeService(
		repository.NewNodeRepo(db),
		repository.NewNodeRouteRepo(db),
		nil,
		db,
	)

	_ = svc.CreateRoute(context.Background(), &CreateRouteRequest{
		SrcNodeID: "alice",
		DstNodeID: "bob",
	})

	// Duplicate should be rejected
	err := svc.CreateRoute(context.Background(), &CreateRouteRequest{
		SrcNodeID: "alice",
		DstNodeID: "bob",
	})
	if err != ErrRouteAlreadyExists {
		t.Errorf("expected ErrRouteAlreadyExists, got %v", err)
	}
}

// --- Bug 19: DeleteProject cascade cleanup ---

func TestProjectService_DeleteProject_CascadeCleanup(t *testing.T) {
	db := setupBugfixTestDB(t)

	svc := NewProjectService(
		repository.NewProjectRepo(db),
		repository.NewProjectInstRepo(db),
		repository.NewProjectNodeRepo(db),
		repository.NewDatatableRepo(db),
		db,
		nil,
	)

	// Create project with associations
	vo, err := svc.CreateProject(context.Background(), &CreateProjectRequest{
		Name:    "cascade-test",
		NodeIDs: []string{"alice", "bob"},
		InstID:  "inst-1",
	}, "admin")
	if err != nil {
		t.Fatalf("CreateProject failed: %v", err)
	}

	// Add a datatable association
	_ = svc.AddDatatable(context.Background(), &AddDatatableRequest{
		ProjectID:   vo.ProjectID,
		NodeID:      "alice",
		DatatableID: "dt-1",
	})

	// Verify associations exist before deletion
	var nodeCount int64
	db.Model(&model.ProjectNodeDO{}).Where("project_id = ?", vo.ProjectID).Count(&nodeCount)
	if nodeCount != 2 {
		t.Fatalf("expected 2 project nodes before delete, got %d", nodeCount)
	}

	var instCount int64
	db.Model(&model.ProjectInstDO{}).Where("project_id = ?", vo.ProjectID).Count(&instCount)
	if instCount != 1 {
		t.Fatalf("expected 1 project inst before delete, got %d", instCount)
	}

	var dtCount int64
	db.Model(&model.ProjectDatatableDO{}).Where("project_id = ?", vo.ProjectID).Count(&dtCount)
	if dtCount != 1 {
		t.Fatalf("expected 1 datatable before delete, got %d", dtCount)
	}

	// Delete project
	err = svc.DeleteProject(context.Background(), vo.ProjectID)
	if err != nil {
		t.Fatalf("DeleteProject failed: %v", err)
	}

	// Verify all associations are cleaned up
	db.Model(&model.ProjectNodeDO{}).Where("project_id = ?", vo.ProjectID).Count(&nodeCount)
	if nodeCount != 0 {
		t.Errorf("expected 0 project nodes after delete, got %d", nodeCount)
	}

	db.Model(&model.ProjectInstDO{}).Where("project_id = ?", vo.ProjectID).Count(&instCount)
	if instCount != 0 {
		t.Errorf("expected 0 project insts after delete, got %d", instCount)
	}

	db.Model(&model.ProjectDatatableDO{}).Where("project_id = ?", vo.ProjectID).Count(&dtCount)
	if dtCount != 0 {
		t.Errorf("expected 0 datatables after delete, got %d", dtCount)
	}

	// Verify project itself is deleted
	_, err = svc.GetProject(context.Background(), vo.ProjectID)
	if err != ErrProjectNotFound {
		t.Errorf("expected ErrProjectNotFound, got %v", err)
	}
}
