package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	migrations "github.com/fengzhizi319/privahub/internal/dao/migrations"
	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/internal/graphjob"
	"github.com/fengzhizi319/privahub/pkg/errcode"
	"github.com/fengzhizi319/privahub/pkg/kuscia"
)

// modelFakeKuscia is an in-memory KusciaAPI for model / serving tests.
type modelFakeKuscia struct {
	mu           sync.Mutex
	srv          *httptest.Server
	jobs         map[string]kuscia.CreateJobRequest
	jobState     map[string]string // job id -> task state
	domaindata   map[string]kuscia.DomainData
	servings     map[string]kuscia.CreateServingRequest
	servingState map[string]string
	deleted      []string
}

func newModelFakeKuscia(t *testing.T) *modelFakeKuscia {
	f := &modelFakeKuscia{
		jobs: map[string]kuscia.CreateJobRequest{}, jobState: map[string]string{},
		domaindata: map[string]kuscia.DomainData{}, servings: map[string]kuscia.CreateServingRequest{},
		servingState: map[string]string{},
	}
	f.srv = httptest.NewServer(http.HandlerFunc(f.handle))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *modelFakeKuscia) client() *kuscia.Client {
	u, _ := url.Parse(f.srv.URL)
	port, _ := strconv.Atoi(u.Port())
	return kuscia.NewClient(&kuscia.ClientConfig{Host: u.Hostname(), Port: port, Protocol: "notls"})
}

func (f *modelFakeKuscia) handle(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	f.mu.Lock()
	defer f.mu.Unlock()
	ok := func(data any) {
		_ = json.NewEncoder(w).Encode(map[string]any{"status": map[string]any{"code": 0, "message": "success"}, "data": data})
	}
	notFound := func() {
		_ = json.NewEncoder(w).Encode(map[string]any{"status": map[string]any{"code": 11404, "message": "not found"}})
	}
	switch r.URL.Path {
	case "/api/v1/job/create":
		var req kuscia.CreateJobRequest
		_ = json.Unmarshal(body, &req)
		f.jobs[req.JobID] = req
		f.jobState[req.JobID] = "Pending"
		ok(map[string]string{"job_id": req.JobID})
	case "/api/v1/job/query":
		var req struct {
			JobID string `json:"job_id"`
		}
		_ = json.Unmarshal(body, &req)
		st, found := f.jobState[req.JobID]
		if !found {
			notFound()
			return
		}
		jobState := st
		end := ""
		if st == "Succeeded" || st == "Failed" {
			end = "2024-01-01T00:00:00Z"
		}
		ok(map[string]any{"job_id": req.JobID, "status": map[string]any{
			"state": jobState, "end_time": end,
			"tasks": []map[string]any{{"task_id": "t", "state": st, "err_msg": map[bool]string{true: "boom"}[st == "Failed"]}},
		}})
	case "/api/v1/domaindata/query":
		var req kuscia.QueryDomainDataRequest
		_ = json.Unmarshal(body, &req)
		dd, found := f.domaindata[req.Data.DomainID+"/"+req.Data.DomainDataID]
		if !found {
			notFound()
			return
		}
		ok(dd)
	case "/api/v1/serving/create":
		var req kuscia.CreateServingRequest
		_ = json.Unmarshal(body, &req)
		f.servings[req.ServingID] = req
		f.servingState[req.ServingID] = "Progressing"
		ok(nil)
	case "/api/v1/serving/query":
		var req kuscia.QueryServingRequest
		_ = json.Unmarshal(body, &req)
		st, found := f.servingState[req.ServingID]
		if !found {
			notFound()
			return
		}
		parties := []map[string]any{}
		for _, p := range f.servings[req.ServingID].Parties {
			parties = append(parties, map[string]any{"domain_id": p.DomainID, "state": st, "endpoints": []map[string]any{
				{"port_name": "service", "endpoint": req.ServingID + "-service." + p.DomainID + ".svc:53509"},
			}})
		}
		ok(map[string]any{"status": map[string]any{"state": st, "party_statuses": parties}})
	case "/api/v1/serving/delete":
		var req kuscia.DeleteServingRequest
		_ = json.Unmarshal(body, &req)
		delete(f.servingState, req.ServingID)
		f.deleted = append(f.deleted, req.ServingID)
		ok(nil)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func setupModelTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	if err := db.AutoMigrate(migrations.AllModels()...); err != nil {
		t.Fatal(err)
	}
	return db
}

func padCode(err error) int {
	if pe, ok := AsPadError(err); ok {
		return pe.Code.Code
	}
	return 0
}

// seedTrainedGraph seeds: read tables (alice/bob) -> psi -> train (lr).
func seedTrainedGraph(t *testing.T, db *gorm.DB) {
	t.Helper()
	db.Create(&model.NodeDO{NodeID: "alice", Name: "Alice", ControlNodeID: "alice"})
	db.Create(&model.NodeDO{NodeID: "bob", Name: "Bob", ControlNodeID: "bob"})
	db.Create(&model.ProjectGraphDO{ProjectID: "p1", GraphID: "g1", Name: "g",
		Edges: `[{"source":"g1-node-1","sourceAnchor":"g1-node-1-output-0","target":"g1-node-3","targetAnchor":"g1-node-3-input-0"},` +
			`{"source":"g1-node-2","sourceAnchor":"g1-node-2-output-0","target":"g1-node-3","targetAnchor":"g1-node-3-input-1"},` +
			`{"source":"g1-node-3","sourceAnchor":"g1-node-3-output-0","target":"g1-node-4","targetAnchor":"g1-node-4-input-0"}]`})
	rt := func(id, dt string) {
		db.Create(&model.ProjectGraphNodeDO{ProjectID: "p1", GraphID: "g1", GraphNodeID: id, CodeName: graphjob.CompReadDataDatatable,
			Outputs: `["` + id + `-output-0"]`,
			NodeDef: `{"domain":"read_data","name":"datatable","version":"1.0.0","attrPaths":["datatable_selected"],"attrs":[{"s":"` + dt + `"}]}`})
	}
	rt("g1-node-1", "dt-a")
	rt("g1-node-2", "dt-b")
	db.Create(&model.ProjectGraphNodeDO{ProjectID: "p1", GraphID: "g1", GraphNodeID: "g1-node-3", CodeName: "data_prep/psi", NodeDef: `{"domain":"data_prep","name":"psi"}`})
	db.Create(&model.ProjectGraphNodeDO{ProjectID: "p1", GraphID: "g1", GraphNodeID: "g1-node-4", CodeName: "ml.train/ss_sgd_train", NodeDef: `{"domain":"ml.train","name":"ss_sgd_train"}`})
	db.Create(&model.ProjectDatatableDO{ProjectID: "p1", NodeID: "alice", DatatableID: "dt-a", TableConfigs: "[]", Source: "IMPORTED"})
	db.Create(&model.ProjectDatatableDO{ProjectID: "p1", NodeID: "bob", DatatableID: "dt-b", TableConfigs: "[]", Source: "IMPORTED"})
	db.Create(&model.ProjectJobDO{ProjectID: "p1", JobID: "trainjob", Name: "j", Status: "SUCCEED", GraphID: "g1"})
	db.Create(&model.ProjectJobTaskDO{ProjectID: "p1", JobID: "trainjob", TaskID: "trainjob-g1-node-4", Parties: `["alice","bob"]`, Status: "SUCCEED", GraphNodeID: "g1-node-4"})
	db.Create(&model.ProjectGraphNodeKusciaParamsDO{ProjectID: "p1", GraphID: "g1", GraphNodeID: "g1-node-1", JobID: "trainjob", TaskID: "x",
		Inputs: `[]`, Outputs: `["trainjob-g1-node-1-output-0"]`, NodeEvalParam: `{"version":"1.0.0","comp_id":"io/read_data:1.0.0"}`})
	db.Create(&model.ProjectGraphNodeKusciaParamsDO{ProjectID: "p1", GraphID: "g1", GraphNodeID: "g1-node-4", JobID: "trainjob", TaskID: "trainjob-g1-node-4",
		Inputs: `["trainjob-g1-node-3-output-0"]`, Outputs: `["trainjob-g1-node-4-output-0"]`,
		NodeEvalParam: `{"version":"1.0.0","comp_id":"ml.train/ss_sgd_train:1.0.0","attr_paths":["epochs"],"attrs":[{"i64":10}],"checkpoint_uri":"ckpt://x"}`})
	db.Create(&model.ProjectGraphDomainDatasourceDO{ProjectID: "p1", GraphID: "g1", DomainID: "bob", DatasourceID: "bob-oss", DatasourceName: "Bob OSS"})
}

func TestModelExport_PackSyncWritesPack(t *testing.T) {
	db := setupModelTestDB(t)
	seedTrainedGraph(t, db)
	fk := newModelFakeKuscia(t)
	svc := NewModelService(db, fk.client())
	ctx := context.Background()

	paths, err := svc.ModelPartyPath(ctx, &ModelPartyPathRequest{ProjectID: "p1", GraphNodeID: "g1-node-4"})
	if err != nil {
		t.Fatalf("party path: %v", err)
	}
	if len(paths) != 2 || paths[0].NodeName != "Alice" || paths[0].DataSources[0].DataSourceID != graphjob.DefaultDataSourceID ||
		paths[1].DataSources[0].DataSourceID != "bob-oss" || paths[1].DataSources[0].Type != "LOCAL" {
		t.Fatalf("unexpected party paths: %+v", paths)
	}

	req := &ModelExportPackageRequest{
		ProjectID: "p1", GraphID: "g1", TrainID: "g1-node-4", ModelName: "m", GraphNodeOutPutID: "g1-node-4-output-0",
		ModelComponent: []ModelComponent{{GraphNodeID: "g1-node-1"}, {GraphNodeID: "g1-node-4"}},
		ModelPartyConfig: []ModelPartyConfig{
			{ModelParty: "alice", ModelDataSource: "default-data-source"},
			{ModelParty: "bob", ModelDataSource: "bob-oss"},
		},
	}
	resp, err := svc.ExportModel(ctx, req)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	job, ok := fk.jobs[resp.JobID]
	if !ok || len(job.Tasks) != 1 || job.Initiator != "alice" {
		t.Fatalf("kuscia job not created as expected: %+v", job)
	}
	var cfg graphjob.TaskInputConfig
	if err := json.Unmarshal([]byte(job.Tasks[0].InputConfig), &cfg); err != nil {
		t.Fatal(err)
	}
	id := resp.JobID + "-" + job.Tasks[0].TaskID
	if cfg.NodeEvalParam.CompID != "model/model_export:1.0.0" || len(cfg.OutputIDs) != 2 || cfg.OutputIDs[0] != id || cfg.OutputIDs[1] != id+"-report" {
		t.Fatalf("unexpected task config: %s", job.Tasks[0].InputConfig)
	}
	if cfg.DatasourceConfig["bob"].ID != "bob-oss" || cfg.DatasourceConfig["alice"].ID != "default-data-source" || len(cfg.ClusterDesc.Parties) != 2 {
		t.Fatalf("unexpected datasource / cluster config: %s", job.Tasks[0].InputConfig)
	}
	var ins, params struct {
		SS []string `json:"ss"`
	}
	_ = json.Unmarshal(cfg.NodeEvalParam.Attrs[2], &ins)
	_ = json.Unmarshal(cfg.NodeEvalParam.Attrs[4], &params)
	if len(ins.SS) != 1 || ins.SS[0] != "trainjob-g1-node-3-output-0" || len(params.SS) != 1 {
		t.Fatalf("read_data must be skipped; inputs=%v params=%v", ins.SS, params.SS)
	}
	raw, _ := base64.StdEncoding.DecodeString(params.SS[0])
	if strings.Contains(string(raw), "checkpoint_uri") || !strings.Contains(string(raw), "ss_sgd_train") {
		t.Fatalf("unexpected component eval param: %s", raw)
	}

	st, err := svc.QueryModelExport(ctx, &ModelExportStatusRequest{ProjectID: "p1", JobID: resp.JobID})
	if err != nil || st.Status != ModelExportInitialized {
		t.Fatalf("status = %+v, %v", st, err)
	}
	if ids := svc.PendingJobIDs(ctx); len(ids) != 1 || ids[0] != resp.JobID {
		t.Fatalf("pending ids = %v", ids)
	}

	// Kuscia reports success; the job sync handler writes the model pack.
	sync := NewJobStatusSyncService(db, nil, nil)
	sync.RegisterHandler(svc)
	if err := sync.SyncJob(ctx, resp.JobID, &kuscia.JobStatusDetailInfo{State: "Succeeded", EndTime: "x",
		Tasks: []kuscia.JobTaskStatusDetail{{State: "Succeeded"}}}); err != nil {
		t.Fatal(err)
	}
	var pack model.ProjectModelPackDO
	if err := db.Where("model_id = ?", id).First(&pack).Error; err != nil {
		t.Fatalf("model pack not written: %v", err)
	}
	if pack.ModelStats != model.ModelStatsInit || pack.ModelReportID != id+"-report" || pack.TrainID != "trainjob-g1-node-4-output-0" ||
		pack.SampleTables != `{"alice":"dt-a","bob":"dt-b"}` || pack.ModelList != `["g1-node-1","g1-node-4"]` {
		t.Fatalf("unexpected pack: %+v", pack)
	}
	if !strings.Contains(pack.ModelDatasource, `"partyId":"bob","datasource":"bob-oss/`) {
		t.Fatalf("unexpected party datasources: %s", pack.ModelDatasource)
	}
	if ids := svc.PendingJobIDs(ctx); len(ids) != 0 {
		t.Fatalf("finished job still pending: %v", ids)
	}
	st, err = svc.QueryModelExport(ctx, &ModelExportStatusRequest{ProjectID: "p1", JobID: resp.JobID})
	if err != nil || st.Status != ModelExportSucceed {
		t.Fatalf("status after sync = %+v, %v", st, err)
	}
	var cnt int64
	db.Model(&model.ProjectModelPackDO{}).Count(&cnt)
	if cnt != 1 {
		t.Fatalf("pack rows = %d", cnt)
	}
	if _, err := svc.QueryModelExport(ctx, &ModelExportStatusRequest{ProjectID: "other", JobID: resp.JobID}); padCode(err) != errcode.ModelExportFailed.Code {
		t.Fatalf("foreign project must not see the job: %v", err)
	}
}

func TestModelExport_StatusPollsKusciaAndFails(t *testing.T) {
	db := setupModelTestDB(t)
	seedTrainedGraph(t, db)
	fk := newModelFakeKuscia(t)
	svc := NewModelService(db, fk.client())
	ctx := context.Background()
	resp, err := svc.ExportModel(ctx, &ModelExportPackageRequest{
		ProjectID: "p1", GraphID: "g1", TrainID: "g1-node-4", ModelName: "m", GraphNodeOutPutID: "g1-node-4-output-0",
		ModelComponent:   []ModelComponent{{GraphNodeID: "g1-node-4"}},
		ModelPartyConfig: []ModelPartyConfig{{ModelParty: "alice", ModelDataSource: "default-data-source"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	fk.mu.Lock()
	fk.jobState[resp.JobID] = "Running"
	fk.mu.Unlock()
	st, _ := svc.QueryModelExport(ctx, &ModelExportStatusRequest{ProjectID: "p1", JobID: resp.JobID})
	if st.Status != ModelExportRunning {
		t.Fatalf("want RUNNING, got %s", st.Status)
	}
	fk.mu.Lock()
	fk.jobState[resp.JobID] = "Failed"
	fk.mu.Unlock()
	st, _ = svc.QueryModelExport(ctx, &ModelExportStatusRequest{ProjectID: "p1", JobID: resp.JobID})
	if st.Status != ModelExportFailed || st.ErrMsg != "boom" || st.Msg != "boom" {
		t.Fatalf("want FAILED/boom, got %+v", st)
	}
	var cnt int64
	db.Model(&model.ProjectModelPackDO{}).Count(&cnt)
	if cnt != 0 {
		t.Fatal("failed export must not write a pack")
	}
}

func TestModelExport_Validation(t *testing.T) {
	db := setupModelTestDB(t)
	seedTrainedGraph(t, db)
	svc := NewModelService(db, newModelFakeKuscia(t).client())
	ctx := context.Background()
	base := ModelExportPackageRequest{ProjectID: "p1", GraphID: "g1", TrainID: "g1-node-4", ModelName: "m",
		ModelPartyConfig: []ModelPartyConfig{{ModelParty: "alice"}}}
	r := base
	r.ModelComponent = []ModelComponent{{GraphNodeID: "g1-node-3"}}
	if _, err := svc.ExportModel(ctx, &r); padCode(err) != errcode.ModelExportFailed.Code {
		t.Fatalf("unrun component: %v", err)
	}
	r = base
	r.TrainID = "nope"
	r.ModelComponent = []ModelComponent{{GraphNodeID: "g1-node-4"}}
	if _, err := svc.ExportModel(ctx, &r); padCode(err) != errcode.GraphNodeOutputNotExists.Code {
		t.Fatalf("no task: %v", err)
	}
	if _, err := svc.ModelPartyPath(ctx, &ModelPartyPathRequest{ProjectID: "p1", GraphNodeID: "nope"}); padCode(err) != errcode.GraphNodeOutputNotExists.Code {
		t.Fatalf("party path without task: %v", err)
	}
}

func seedPack(db *gorm.DB, id string, stats int8) *model.ProjectModelPackDO {
	p := &model.ProjectModelPackDO{ProjectID: "p1", ModelID: id, Initiator: "alice", ModelName: "name-" + id, ModelStats: stats,
		SampleTables: `{"alice":"dt-a","bob":"dt-b"}`, ModelList: `["g1-node-4"]`, TrainID: "t", ModelReportID: id + "-report",
		GraphDetail:     `{"graphId":"g1"}`,
		ModelDatasource: `[{"partyId":"alice","datasource":"default-data-source/ab_1"},{"partyId":"bob","datasource":"bob-oss/ab_1"}]`}
	db.Create(p)
	return p
}

func TestModelServing_CreateDetailDeleteStateMachine(t *testing.T) {
	db := setupModelTestDB(t)
	seedTrainedGraph(t, db)
	fk := newModelFakeKuscia(t)
	svc := NewModelService(db, fk.client())
	ctx := context.Background()
	seedPack(db, "mdl", model.ModelStatsInit)
	fk.domaindata["alice/mdl"] = kuscia.DomainData{DomainDataID: "mdl"}
	fk.domaindata["bob/mdl"] = kuscia.DomainData{DomainDataID: "mdl"}
	db.Create(&model.FeatureTableDO{FeatureTableID: "ft1", FeatureTableName: "f", NodeID: "bob", DatasourceID: "http-data-source",
		Type: "HTTP", URL: "http://bob-feature:8080/feature", Columns: "[]", Status: "Available"})

	mock := true
	notMock := false
	req := &CreateModelServingRequest{ModelID: "mdl", ProjectID: "p1", PartyConfigs: []ServingPartyConfig{
		{NodeID: "alice", FeatureTableID: "mock", IsMock: &mock, Features: []ServingFeature{{OfflineName: "x1", OnlineName: "x1_on"}},
			Resources: []ServingResourceVO{{MinCPU: "0", MaxCPU: "2", MinMemory: "0Gi", MaxMemory: "4Gi"}}},
		{NodeID: "bob", FeatureTableID: "ft1", IsMock: &notMock, Features: []ServingFeature{{OfflineName: "y1", OnlineName: "y1"}},
			Resources: []ServingResourceVO{{MinCPU: "0", MaxCPU: "0", MinMemory: "0Gi", MaxMemory: "0Gi"}}},
	}}
	// invalid resources are rejected
	bad := *req
	bad.PartyConfigs = []ServingPartyConfig{{NodeID: "alice", IsMock: &mock, FeatureTableID: "mock",
		Resources: []ServingResourceVO{{MinCPU: "3", MaxCPU: "2", MinMemory: "1Gi", MaxMemory: "2Gi"}}}}
	if err := svc.CreateModelServing(ctx, &bad); padCode(err) != errcode.SystemValidationError.Code {
		t.Fatalf("want validation error, got %v", err)
	}

	if err := svc.CreateModelServing(ctx, req); err != nil {
		t.Fatalf("create serving: %v", err)
	}
	var pack model.ProjectModelPackDO
	db.Where("model_id = ?", "mdl").First(&pack)
	if pack.ModelStats != model.ModelStatsPublishing || pack.ServingID == "" {
		t.Fatalf("model not PUBLISHING: %+v", pack)
	}
	servingID := pack.ServingID
	ks := fk.servings[servingID]
	if ks.Initiator != "alice" || len(ks.Parties) != 2 || ks.Parties[0].AppImage != "sf-serving-image" {
		t.Fatalf("unexpected kuscia serving: %+v", ks)
	}
	if r := ks.Parties[0].Resources; len(r) != 1 || r[0].MinCPU != "0.1" || r[0].MaxCPU != "2.0" || r[0].MinMemory != "0.1Gi" || r[0].MaxMemory != "4.0Gi" {
		t.Fatalf("resources not normalised: %+v", r)
	}
	if len(ks.Parties[1].Resources) != 0 {
		t.Fatalf("all-zero resources must be skipped: %+v", ks.Parties[1].Resources)
	}
	if !strings.Contains(ks.ServingInputConfig, `"source_type":"ST_DP"`) || !strings.Contains(ks.ServingInputConfig, `"mock_opts":{}`) ||
		!strings.Contains(ks.ServingInputConfig, `"endpoint":"http://bob-feature:8080/feature"`) || !strings.Contains(ks.ServingInputConfig, `"x1_on":"x1"`) {
		t.Fatalf("unexpected serving input config: %s", ks.ServingInputConfig)
	}

	// discard is refused while publishing
	if err := svc.DiscardModelPack(ctx, "mdl"); padCode(err) != errcode.ProjectServingNotOffline.Code {
		t.Fatalf("discard while publishing: %v", err)
	}

	// page refreshes PUBLISHING -> PUBLISHED through Kuscia and stores endpoints
	fk.mu.Lock()
	fk.servingState[servingID] = "Available"
	fk.mu.Unlock()
	page, err := svc.ModelPackPage(ctx, &QueryModelPageRequest{ProjectID: "p1", Page: 1, Size: 10})
	if err != nil || page.Total != 1 || page.ModelPacks[0].ModelStats != "PUBLISHED" || page.ModelPacks[0].OwnerID != "alice" {
		t.Fatalf("page = %+v, %v", page, err)
	}

	detail, err := svc.QueryModelServingDetail(ctx, servingID)
	if err != nil {
		t.Fatal(err)
	}
	if detail.ModelID != "mdl" || len(detail.ServingDetails) != 2 {
		t.Fatalf("detail = %+v", detail)
	}
	a, b := detail.ServingDetails[0], detail.ServingDetails[1]
	if a.NodeName != "Alice" || !*a.IsMock || a.FeatureMappings["x1_on"] != "x1" || a.SourcePath != "mdl" ||
		len(a.Resources) != 1 || a.Resources[0].MaxMemory != "4Gi" || !strings.Contains(a.Endpoints, "-service.alice.svc") {
		t.Fatalf("alice detail = %+v", a)
	}
	if *b.IsMock || b.FeatureHTTP != "http://bob-feature:8080/feature" || !strings.Contains(b.Endpoints, "-service.bob.svc") {
		t.Fatalf("bob detail = %+v", b)
	}
	var sv model.ProjectModelServingDO
	db.Where("serving_id = ?", servingID).First(&sv)
	if sv.ServingStats != "success" {
		t.Fatalf("serving_stats = %s", sv.ServingStats)
	}

	// delete serving -> OFFLINE, row kept
	if err := svc.DeleteModelServing(ctx, servingID); err != nil {
		t.Fatal(err)
	}
	db.Where("model_id = ?", "mdl").First(&pack)
	if pack.ModelStats != model.ModelStatsOffline || len(fk.deleted) != 1 {
		t.Fatalf("after delete: %+v deleted=%v", pack, fk.deleted)
	}
	if _, err := svc.QueryModelServingDetail(ctx, servingID); err != nil {
		t.Fatalf("serving row must be kept: %v", err)
	}

	// republish from OFFLINE reuses the serving id
	if err := svc.CreateModelServing(ctx, req); err != nil {
		t.Fatal(err)
	}
	db.Where("model_id = ?", "mdl").First(&pack)
	if pack.ServingID != servingID || pack.ModelStats != model.ModelStatsPublishing {
		t.Fatalf("serving id not reused: %+v", pack)
	}
	var svCnt int64
	db.Model(&model.ProjectModelServingDO{}).Count(&svCnt)
	if svCnt != 1 {
		t.Fatalf("serving rows = %d", svCnt)
	}

	// serving sync: Failed -> PUBLISH_FAIL
	fk.mu.Lock()
	fk.servingState[servingID] = "Failed"
	fk.mu.Unlock()
	if n := NewServingStatusSyncService(db, nil, nil).WithModelService(svc).SyncOnce(ctx); n != 1 {
		t.Fatalf("sync changed %d", n)
	}
	db.Where("model_id = ?", "mdl").First(&pack)
	db.Where("serving_id = ?", servingID).First(&sv)
	if pack.ModelStats != model.ModelStatsPublishFail || sv.ServingStats != "failed" {
		t.Fatalf("after failed sync: model=%d serving=%s", pack.ModelStats, sv.ServingStats)
	}

	// delete requires DISCARDED; discard requires OFFLINE / INIT
	if err := svc.DeleteModelPack(ctx, "alice", "mdl"); padCode(err) != errcode.ProjectServingNotOffline.Code {
		t.Fatalf("delete non-discarded: %v", err)
	}
	if err := svc.DeleteModelServing(ctx, servingID); err != nil {
		t.Fatal(err)
	}
	if err := svc.DiscardModelPack(ctx, "mdl"); err != nil {
		t.Fatal(err)
	}
	if err := svc.DeleteModelPack(ctx, "alice", "mdl"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ModelPackInfo(ctx, "mdl", "p1"); padCode(err) != errcode.ProjectModelNotFound.Code {
		t.Fatalf("deleted model still visible: %v", err)
	}
}

func TestModelPack_PageFilterDetailInfo(t *testing.T) {
	db := setupModelTestDB(t)
	seedTrainedGraph(t, db)
	fk := newModelFakeKuscia(t)
	svc := NewModelService(db, fk.client())
	ctx := context.Background()
	seedPack(db, "m_one", model.ModelStatsInit)
	seedPack(db, "mtwo", model.ModelStatsOffline)

	page, err := svc.ModelPackPage(ctx, &QueryModelPageRequest{ProjectID: "p1", Page: 1, Size: 10, ModelStats: "OFFLINE"})
	if err != nil || page.Total != 1 || page.ModelPacks[0].ModelID != "mtwo" {
		t.Fatalf("stats filter: %+v %v", page, err)
	}
	// '_' is escaped (literal), so "m_" matches only m_one
	page, _ = svc.ModelPackPage(ctx, &QueryModelPageRequest{ProjectID: "p1", SearchKey: "m_"})
	if page.Total != 1 || page.ModelPacks[0].ModelID != "m_one" {
		t.Fatalf("search: %+v", page)
	}
	page, _ = svc.ModelPackPage(ctx, &QueryModelPageRequest{ProjectID: "p1", Page: 2, Size: 1, Sort: map[string]string{"modelId": "ASC"}})
	if page.Total != 2 || page.TotalPage != 2 || len(page.ModelPacks) != 1 || page.ModelPacks[0].ModelID != "mtwo" {
		t.Fatalf("paging: %+v", page)
	}
	if _, err := svc.ModelPackPage(ctx, &QueryModelPageRequest{ProjectID: "p1", ModelStats: "BAD"}); err == nil {
		t.Fatal("invalid modelStats must fail")
	}

	fk.domaindata["alice/m_one-report"] = kuscia.DomainData{DomainDataID: "m_one-report",
		Attributes: map[string]string{"dist_data": `{"name":"r","type":"sf.report","meta":{"desc":"a1,a2,b1"}}`}}
	fk.domaindata["bob/m_one-report"] = fk.domaindata["alice/m_one-report"]
	fk.domaindata["alice/dt-a"] = kuscia.DomainData{DomainDataID: "dt-a", Columns: []kuscia.DataColumn{{Name: "id"}, {Name: "a1"}, {Name: "a2"}}}
	fk.domaindata["bob/dt-b"] = kuscia.DomainData{DomainDataID: "dt-b", Columns: []kuscia.DataColumn{{Name: "id"}, {Name: "b1"}}}
	d, err := svc.ModelPackDetail(ctx, "m_one", "p1")
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Parties) != 2 || d.Parties[0].NodeName != "Alice" || strings.Join(d.Parties[0].Columns, ",") != "a1,a2" ||
		strings.Join(d.Parties[1].Columns, ",") != "b1" {
		t.Fatalf("detail = %+v", d)
	}

	info, err := svc.ModelPackInfo(ctx, "m_one", "p1")
	if err != nil {
		t.Fatal(err)
	}
	if info.ModelStats != "INIT" || len(info.ModelGraphDetail) != 1 || string(info.GraphDetailVO) != `{"graphId":"g1"}` ||
		len(info.ServingDetails) != 2 || info.ServingDetails[1].SourcePath != "bob-oss/ab_1" {
		t.Fatalf("info = %+v", info)
	}
	if err := svc.DiscardModelPack(ctx, "m_one"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ModelPackDetail(ctx, "missing", "p1"); padCode(err) != errcode.ProjectModelNotFound.Code {
		t.Fatalf("missing model: %v", err)
	}
}

// Even a valid caller project must not authorize a model owned by a different
// project. These calls must fail before any Kuscia lookup or deployment.
func TestModel_ProjectBinding(t *testing.T) {
	db := setupModelTestDB(t)
	pack := seedPack(db, "foreign-model", model.ModelStatsInit)
	svc := NewModelService(db, nil)
	ctx := context.Background()
	for _, projectID := range []string{"other-project", ""} {
		if _, err := svc.ModelPackInfo(ctx, pack.ModelID, projectID); padCode(err) != errcode.ProjectModelNotFound.Code {
			t.Fatalf("info project %q: %v", projectID, err)
		}
		if _, err := svc.ModelPackDetail(ctx, pack.ModelID, projectID); padCode(err) != errcode.ProjectModelNotFound.Code {
			t.Fatalf("detail project %q: %v", projectID, err)
		}
		if err := svc.CreateModelServing(ctx, &CreateModelServingRequest{ModelID: pack.ModelID, ProjectID: projectID}); padCode(err) != errcode.ProjectModelNotFound.Code {
			t.Fatalf("serving project %q: %v", projectID, err)
		}
	}
	if err := svc.CreateModelServing(ctx, &CreateModelServingRequest{ModelID: pack.ModelID, ProjectID: pack.ProjectID}); padCode(err) != errcode.SystemValidationError.Code {
		t.Fatalf("empty serving reported success: %v", err)
	}
	var got model.ProjectModelPackDO
	if err := db.Where("model_id = ?", pack.ModelID).First(&got).Error; err != nil {
		t.Fatal(err)
	}
	if got.ModelStats != pack.ModelStats || got.ServingID != "" {
		t.Fatalf("rejected deployment modified model: %+v", got)
	}
	mock := true
	if msg := ValidateServingPartyConfigs([]ServingPartyConfig{
		{NodeID: "alice", FeatureTableID: "mock", IsMock: &mock},
		{NodeID: "alice", FeatureTableID: "mock", IsMock: &mock},
	}); !strings.Contains(msg, "unique") {
		t.Fatalf("duplicate parties accepted: %s", msg)
	}
}
