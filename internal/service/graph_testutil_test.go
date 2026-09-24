package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"

	migration "github.com/fengzhizi319/privahub/internal/dao/migrations"
	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/internal/dao/repository"
	"github.com/fengzhizi319/privahub/internal/graphjob"
	"github.com/fengzhizi319/privahub/pkg/kuscia"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// graphFakeKuscia is an in-memory GraphKusciaAPI used by the graph / job tests.
type graphFakeKuscia struct {
	mu            sync.Mutex
	created       []*kuscia.CreateJobRequest
	createErr     error
	stopped       []string
	jobs          map[string]*kuscia.JobStatusDetailInfo
	domainData    map[string]*kuscia.QueryDomainDataResponse // key node/id
	grants        map[string]bool                            // key node/grantId
	grantCreates  []kuscia.DomainDataGrantWithIDRequest
	datasources   map[string][]kuscia.DomainDataSource
	notReady      map[string]bool
	routeNotReady map[string]bool // key src->dst
}

func newGraphFakeKuscia() *graphFakeKuscia {
	return &graphFakeKuscia{
		jobs:          map[string]*kuscia.JobStatusDetailInfo{},
		domainData:    map[string]*kuscia.QueryDomainDataResponse{},
		grants:        map[string]bool{},
		datasources:   map[string][]kuscia.DomainDataSource{},
		notReady:      map[string]bool{},
		routeNotReady: map[string]bool{},
	}
}

func (f *graphFakeKuscia) addDomainData(node, id, typ string, cols []kuscia.DataColumn, attrs map[string]string) {
	resp := &kuscia.QueryDomainDataResponse{}
	resp.Data.DomainDataID = id
	resp.Data.Name = id + "-name"
	resp.Data.Type = typ
	resp.Data.RelativeURI = id + ".csv"
	resp.Data.DatasourceID = "default-data-source"
	resp.Data.Columns = cols
	resp.Data.Attributes = attrs
	f.mu.Lock()
	f.domainData[node+"/"+id] = resp
	f.mu.Unlock()
}

func (f *graphFakeKuscia) CreateJob(_ context.Context, req *kuscia.CreateJobRequest) (*kuscia.CreateJobResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.createErr != nil {
		return nil, f.createErr
	}
	f.created = append(f.created, req)
	return &kuscia.CreateJobResponse{}, nil
}

func (f *graphFakeKuscia) StopJob(_ context.Context, jobID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stopped = append(f.stopped, jobID)
	return nil
}

func (f *graphFakeKuscia) QueryJobDetail(_ context.Context, jobID string) (*kuscia.QueryJobDetailResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	st, ok := f.jobs[jobID]
	if !ok {
		return nil, errors.New("not found")
	}
	resp := &kuscia.QueryJobDetailResponse{}
	resp.Data.JobID = jobID
	resp.Data.Status = st
	return resp, nil
}

func (f *graphFakeKuscia) QueryDomainData(_ context.Context, domainID, id string) (*kuscia.QueryDomainDataResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if d, ok := f.domainData[domainID+"/"+id]; ok {
		return d, nil
	}
	return nil, fmt.Errorf("domaindata %s/%s not found", domainID, id)
}

func (f *graphFakeKuscia) CreateDomainDataGrantWithID(_ context.Context, req *kuscia.DomainDataGrantWithIDRequest) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.grantCreates = append(f.grantCreates, *req)
	f.grants[req.DomainID+"/"+req.DomainDataGrantID] = true
	return nil
}

func (f *graphFakeKuscia) QueryDomainDataGrantByID(_ context.Context, domainID, grantID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.grants[domainID+"/"+grantID] {
		return nil
	}
	return errors.New("grant not found")
}

func (f *graphFakeKuscia) ListDomainDataSource(_ context.Context, domainID string) ([]kuscia.DomainDataSource, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if ds, ok := f.datasources[domainID]; ok {
		return ds, nil
	}
	return []kuscia.DomainDataSource{{DomainID: domainID, DatasourceID: "default-data-source", Name: "default", Type: "localfs"}}, nil
}

func (f *graphFakeKuscia) QueryDomain(_ context.Context, domainID string) (*kuscia.QueryDomainResponse, error) {
	resp := &kuscia.QueryDomainResponse{}
	resp.Data.DomainID = domainID
	status := kuscia.DomainStatusReady
	if f.notReady[domainID] {
		status = kuscia.DomainStatusNotReady
	}
	resp.Data.NodeStatuses = []kuscia.NodeStatus{{Name: domainID, Status: status}}
	return resp, nil
}

func (f *graphFakeKuscia) QueryDomainRoute(_ context.Context, src, dst string) (*kuscia.QueryDomainRouteResponse, error) {
	resp := &kuscia.QueryDomainRouteResponse{}
	resp.Data.Source, resp.Data.Destination = src, dst
	resp.Data.Status.Status = kuscia.RouteStatusSucceeded
	if f.routeNotReady[src+"->"+dst] {
		resp.Data.Status.Status = kuscia.RouteStatusFailed
	}
	return resp, nil
}

// graphFixture bundles an in-memory DB, the fake Kuscia and the services.
type graphFixture struct {
	t       *testing.T
	db      *gorm.DB
	k       *graphFakeKuscia
	graph   *GraphService
	job     *JobService
	project *ProjectService
	sync    *JobStatusSyncService
}

func newGraphFixture(t *testing.T) *graphFixture {
	t.Helper()
	graphjob.SetSettings(func() *graphjob.Settings {
		s := graphjob.DefaultSettings()
		return s
	}())
	db, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(migration.AllModels()...); err != nil {
		t.Fatal(err)
	}
	k := newGraphFakeKuscia()
	f := &graphFixture{t: t, db: db, k: k}
	f.graph = NewGraphService(repository.NewGraphRepo(db), repository.NewGraphNodeRepo(db), repository.NewJobRepo(db),
		repository.NewTaskRepo(db), repository.NewTaskLogRepo(db), nil, db)
	f.graph.SetKuscia(k)
	f.job = NewJobService(repository.NewJobRepo(db), repository.NewTaskRepo(db), repository.NewTaskLogRepo(db),
		repository.NewGraphRepo(db), repository.NewGraphNodeRepo(db), nil, db)
	f.job.SetKuscia(k)
	f.project = NewProjectService(repository.NewProjectRepo(db), repository.NewProjectInstRepo(db), repository.NewProjectNodeRepo(db),
		repository.NewDatatableRepo(db), db, nil)
	f.project.SetKuscia(k)
	f.sync = NewJobStatusSyncService(db, nil, nil)
	f.sync.SetKuscia(k)
	return f
}

func (f *graphFixture) mustCreate(v any) {
	f.t.Helper()
	if err := f.db.Create(v).Error; err != nil {
		f.t.Fatal(err)
	}
}

// seedPSIProject creates project p1 with nodes alice/bob, their datatables
// and graph g1: alice-table, bob-table -> psi -> train.
func (f *graphFixture) seedPSIProject() {
	f.t.Helper()
	f.mustCreate(&model.NodeDO{NodeID: "alice", Name: "Alice", ControlNodeID: "alice", Type: "normal"})
	f.mustCreate(&model.NodeDO{NodeID: "bob", Name: "Bob", ControlNodeID: "bob", Type: "normal"})
	f.mustCreate(&model.ProjectDO{ProjectID: "p1", Name: "proj", ComputeMode: "MPC", OwnerID: "admin"})
	f.mustCreate(&model.ProjectNodeDO{ProjectID: "p1", NodeID: "alice"})
	f.mustCreate(&model.ProjectNodeDO{ProjectID: "p1", NodeID: "bob"})
	cfg := `[{"colName":"id","colType":"str","isAssociateKey":true},{"colName":"y","colType":"int","isProtection":true},{"colName":"x","colType":"float"}]`
	f.mustCreate(&model.ProjectDatatableDO{ProjectID: "p1", NodeID: "alice", DatatableID: "alice-table", TableConfigs: cfg, Source: "IMPORTED"})
	f.mustCreate(&model.ProjectDatatableDO{ProjectID: "p1", NodeID: "bob", DatatableID: "bob-table", TableConfigs: `[{"colName":"id","colType":"str","isAssociateKey":true}]`, Source: "IMPORTED"})
	f.k.addDomainData("alice", "alice-table", "table", []kuscia.DataColumn{{Name: "id", Type: "str"}, {Name: "y", Type: "int"}, {Name: "x", Type: "float"}}, nil)
	f.k.addDomainData("bob", "bob-table", "table", []kuscia.DataColumn{{Name: "id", Type: "str"}}, nil)

	edges := []graphjob.GraphEdge{
		{EdgeID: "e1", Source: "g1-node-1", SourceAnchor: "g1-node-1-output-0", Target: "g1-node-3", TargetAnchor: "g1-node-3-input-0"},
		{EdgeID: "e2", Source: "g1-node-2", SourceAnchor: "g1-node-2-output-0", Target: "g1-node-3", TargetAnchor: "g1-node-3-input-1"},
		{EdgeID: "e3", Source: "g1-node-3", SourceAnchor: "g1-node-3-output-0", Target: "g1-node-4", TargetAnchor: "g1-node-4-input-0"},
	}
	f.mustCreate(&model.ProjectGraphDO{ProjectID: "p1", GraphID: "g1", Name: "psi-graph", Edges: graphjob.EdgesJSON(edges), OwnerID: "admin", NodeMaxIndex: 32, MaxParallelism: 2})
	nodes := []graphjob.GraphNodeInfo{
		{CodeName: "read_data/datatable", GraphNodeID: "g1-node-1", Label: "alice", Outputs: []string{"g1-node-1-output-0"},
			NodeDef: json.RawMessage(`{"domain":"read_data","name":"datatable","version":"0.0.1","attrPaths":["datatable_selected"],"attrs":[{"s":"alice-table"}]}`)},
		{CodeName: "read_data/datatable", GraphNodeID: "g1-node-2", Label: "bob", Outputs: []string{"g1-node-2-output-0"},
			NodeDef: json.RawMessage(`{"domain":"read_data","name":"datatable","version":"0.0.1","attrPaths":["datatable_selected"],"attrs":[{"s":"bob-table"}]}`)},
		{CodeName: "data_prep/psi", GraphNodeID: "g1-node-3", Label: "psi", Inputs: []string{"g1-node-1-output-0", "g1-node-2-output-0"}, Outputs: []string{"g1-node-3-output-0"},
			NodeDef: json.RawMessage(`{"domain":"data_prep","name":"psi","version":"1.0.0","attrPaths":["protocol"],"attrs":[{"s":"PROTOCOL_RR22"}]}`)},
		{CodeName: "stats/table_statistics", GraphNodeID: "g1-node-4", Label: "stats", Inputs: []string{"g1-node-3-output-0"}, Outputs: []string{"g1-node-4-output-0"},
			NodeDef: json.RawMessage(`{"domain":"stats","name":"table_statistics","version":"1.0.2","attrPaths":[],"attrs":[]}`)},
	}
	for i := range nodes {
		f.mustCreate(nodeDOFromInfo("p1", "g1", &nodes[i]))
	}
}

func adminCtx() context.Context {
	return WithOperator(context.Background(), Operator{OwnerID: "admin", OwnerType: "CENTER"})
}

func decodeTaskInput(t *testing.T, s string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		t.Fatalf("task_input_config: %v", err)
	}
	return m
}
