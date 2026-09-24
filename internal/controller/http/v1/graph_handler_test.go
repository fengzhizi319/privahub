package v1

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fengzhizi319/privahub/internal/controller/http/middleware"
	migration "github.com/fengzhizi319/privahub/internal/dao/migrations"
	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/internal/dao/repository"
	"github.com/fengzhizi319/privahub/internal/graphjob"
	"github.com/fengzhizi319/privahub/internal/service"
	"github.com/fengzhizi319/privahub/pkg/kuscia"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// graphKusciaServer fakes the KusciaAPI HTTP endpoints used by the graph flow.
type graphKusciaServer struct {
	mu      sync.Mutex
	created []map[string]any
	stopped []string
}

func (k *graphKusciaServer) handler(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	var req map[string]any
	_ = json.Unmarshal(body, &req)
	ok := map[string]any{"code": 0, "message": "success"}
	write := func(v any) { _ = json.NewEncoder(w).Encode(v) }
	k.mu.Lock()
	defer k.mu.Unlock()
	switch r.URL.Path {
	case "/api/v1/job/create":
		k.created = append(k.created, req)
		write(map[string]any{"status": ok, "data": map[string]any{"job_id": req["job_id"]}})
	case "/api/v1/job/stop":
		k.stopped = append(k.stopped, req["job_id"].(string))
		write(map[string]any{"status": ok})
	case "/api/v1/domain/query":
		write(map[string]any{"status": ok, "data": map[string]any{"domain_id": req["domain_id"], "node_statuses": []any{map[string]any{"name": "n", "status": "Ready"}}}})
	case "/api/v1/route/query":
		write(map[string]any{"status": ok, "data": map[string]any{"status": map[string]any{"status": "Succeeded"}}})
	case "/api/v1/domaindata/query":
		data, _ := req["data"].(map[string]any)
		id, _ := data["domaindata_id"].(string)
		write(map[string]any{"status": ok, "data": map[string]any{
			"domaindata_id": id, "name": id, "type": "table", "relative_uri": id + ".csv", "datasource_id": "default-data-source",
			"columns": []any{map[string]any{"name": "id", "type": "str"}},
		}})
	case "/api/v1/domaindatagrant/query":
		write(map[string]any{"status": map[string]any{"code": 404, "message": "not found"}})
	case "/api/v1/domaindatagrant/create":
		write(map[string]any{"status": ok})
	case "/api/v1/domaindatasource/list":
		write(map[string]any{"status": ok, "data": map[string]any{"datasource_list": []any{
			map[string]any{"domain_id": req["domain_id"], "datasource_id": "default-data-source", "name": "default", "type": "localfs"},
		}}})
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func setupGraphEndpoints(t *testing.T) (*gin.Engine, *gorm.DB, *graphKusciaServer) {
	t.Helper()
	graphjob.SetSettings(graphjob.DefaultSettings())
	db, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{Logger: gormlogger.Discard})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(migration.AllModels()...); err != nil {
		t.Fatal(err)
	}
	ks := &graphKusciaServer{}
	srv := httptest.NewServer(http.HandlerFunc(ks.handler))
	t.Cleanup(srv.Close)
	client := kuscia.NewClient(&kuscia.ClientConfig{Host: strings.TrimPrefix(srv.URL, "http://"), Protocol: "notls", Timeout: 5 * time.Second})

	graphSvc := service.NewGraphService(repository.NewGraphRepo(db), repository.NewGraphNodeRepo(db), repository.NewJobRepo(db),
		repository.NewTaskRepo(db), repository.NewTaskLogRepo(db), client, db)
	jobSvc := service.NewJobService(repository.NewJobRepo(db), repository.NewTaskRepo(db), repository.NewTaskLogRepo(db),
		repository.NewGraphRepo(db), repository.NewGraphNodeRepo(db), client, db)
	projectSvc := service.NewProjectService(repository.NewProjectRepo(db), repository.NewProjectInstRepo(db),
		repository.NewProjectNodeRepo(db), repository.NewDatatableRepo(db), db, client)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.CaseKeysResponse(), middleware.CaseKeysRequest())
	r.Use(func(c *gin.Context) { c.Set("owner_id", "admin"); c.Set("owner_type", "CENTER"); c.Next() })
	gh := NewGraphHandler(graphSvc)
	jh := NewJobHandler(jobSvc).WithGraphService(graphSvc)
	ph := NewProjectHandler(projectSvc, nil, nil)
	api := r.Group("/api/v1alpha1")
	api.POST("/graph/create", gh.Create)
	api.POST("/graph/update", gh.FullUpdate)
	api.POST("/graph/detail", gh.Detail)
	api.POST("/graph/start", gh.Start)
	api.POST("/graph/stop", gh.Stop)
	api.POST("/graph/node/status", gh.NodeStatus)
	api.POST("/graph/node/max_index", gh.NodeMaxIndex)
	api.POST("/graph/node/logs", gh.NodeLogs)
	api.POST("/project/job/list", jh.List)
	api.POST("/project/job/get", jh.Detail)
	api.POST("/project/job/create", jh.Create)
	api.POST("/project/list", ph.List)
	api.POST("/project/get", ph.Get)
	api.POST("/project/delete", ph.Delete)
	api.POST("/project/datatable/add", ph.AddDatatable)
	api.POST("/project/datatable/get", ph.GetDatatable)

	mustCreate := func(v any) {
		if err := db.Create(v).Error; err != nil {
			t.Fatal(err)
		}
	}
	mustCreate(&model.NodeDO{NodeID: "alice", Name: "alice", ControlNodeID: "alice"})
	mustCreate(&model.NodeDO{NodeID: "bob", Name: "bob", ControlNodeID: "bob"})
	mustCreate(&model.ProjectDO{ProjectID: "p1", Name: "proj", ComputeMode: "MPC"})
	mustCreate(&model.ProjectNodeDO{ProjectID: "p1", NodeID: "alice"})
	mustCreate(&model.ProjectNodeDO{ProjectID: "p1", NodeID: "bob"})
	mustCreate(&model.ProjectDatatableDO{ProjectID: "p1", NodeID: "alice", DatatableID: "alice-table", TableConfigs: `[{"colName":"id","isAssociateKey":true}]`, Source: "IMPORTED"})
	mustCreate(&model.ProjectDatatableDO{ProjectID: "p1", NodeID: "bob", DatatableID: "bob-table", TableConfigs: `[{"colName":"id","isAssociateKey":true}]`, Source: "IMPORTED"})
	return r, db, ks
}

func graphPost(t *testing.T, r *gin.Engine, path string, body any) map[string]any {
	t.Helper()
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("%s: http %d %s", path, w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	return resp
}

func statusCode(resp map[string]any) int {
	return int(resp["status"].(map[string]any)["code"].(float64))
}

func TestGraphEndpoints_EndToEnd(t *testing.T) {
	r, db, ks := setupGraphEndpoints(t)
	readNode := func(id, table string) map[string]any {
		return map[string]any{"codeName": "read_data/datatable", "graphNodeId": id, "label": table, "x": 1, "y": 2, "inputs": []string{},
			"outputs": []string{id + "-output-0"},
			"nodeDef": map[string]any{"domain": "read_data", "name": "datatable", "version": "0.0.1", "attrPaths": []string{"datatable_selected"}, "attrs": []any{map[string]any{"s": table}}}}
	}
	resp := graphPost(t, r, "/api/v1alpha1/graph/create", map[string]any{"projectId": "p1", "name": "g"})
	if statusCode(resp) != 0 {
		t.Fatalf("create %v", resp)
	}
	graphID := resp["data"].(map[string]any)["graphId"].(string)
	nodes := []any{readNode(graphID+"-node-1", "alice-table"), readNode(graphID+"-node-2", "bob-table"),
		map[string]any{"codeName": "data_prep/psi", "graphNodeId": graphID + "-node-3", "label": "psi",
			"inputs": []string{graphID + "-node-1-output-0", graphID + "-node-2-output-0"}, "outputs": []string{graphID + "-node-3-output-0"},
			"nodeDef": map[string]any{"domain": "data_prep", "name": "psi", "version": "1.0.0", "attrPaths": []string{"protocol"}, "attrs": []any{map[string]any{"s": "PROTOCOL_RR22"}}}},
	}
	edges := []any{
		map[string]any{"edgeId": "e1", "source": graphID + "-node-1", "sourceAnchor": graphID + "-node-1-output-0", "target": graphID + "-node-3", "targetAnchor": "a"},
		map[string]any{"edgeId": "e2", "source": graphID + "-node-2", "sourceAnchor": graphID + "-node-2-output-0", "target": graphID + "-node-3", "targetAnchor": "b"},
	}
	if resp := graphPost(t, r, "/api/v1alpha1/graph/update", map[string]any{"projectId": "p1", "graphId": graphID, "nodes": nodes, "edges": edges}); statusCode(resp) != 0 {
		t.Fatalf("update %v", resp)
	}
	// a meta-only update must not wipe nodes
	graphPost(t, r, "/api/v1alpha1/graph/update", map[string]any{"projectId": "p1", "graphId": graphID, "maxParallelism": 2})
	detail := graphPost(t, r, "/api/v1alpha1/graph/detail", map[string]any{"projectId": "p1", "graphId": graphID})["data"].(map[string]any)
	if len(detail["nodes"].([]any)) != 3 || detail["maxParallelism"].(float64) != 2 {
		t.Fatalf("detail %v", detail)
	}
	nd := detail["nodes"].([]any)[2].(map[string]any)["nodeDef"].(map[string]any)
	if _, polluted := nd["attr_paths"]; polluted {
		t.Fatalf("nodeDef keeps snake twins: %v", nd)
	}
	mi := graphPost(t, r, "/api/v1alpha1/graph/node/max_index", map[string]any{"projectId": "p1", "graphId": graphID, "currentIndex": 3})
	if mi["data"].(map[string]any)["maxIndex"].(float64) != 32 {
		t.Fatalf("max index %v", mi)
	}

	start := graphPost(t, r, "/api/v1alpha1/graph/start", map[string]any{"projectId": "p1", "graphId": graphID, "nodes": []string{graphID + "-node-1", graphID + "-node-2", graphID + "-node-3"}})
	if statusCode(start) != 0 {
		t.Fatalf("start %v", start)
	}
	jobID := start["data"].(map[string]any)["jobId"].(string)
	if len(ks.created) != 1 || ks.created[0]["job_id"] != jobID || ks.created[0]["initiator"] != "alice" {
		t.Fatalf("kuscia create %v", ks.created)
	}
	task := ks.created[0]["tasks"].([]any)[0].(map[string]any)
	var cfg map[string]any
	_ = json.Unmarshal([]byte(task["task_input_config"].(string)), &cfg)
	if ids := cfg["sf_input_ids"].([]any); len(ids) != 2 || ids[0] != "alice-table" {
		t.Fatalf("task input %v", cfg)
	}

	st := graphPost(t, r, "/api/v1alpha1/graph/node/status", map[string]any{"projectId": "p1", "graphId": graphID})["data"].(map[string]any)
	if st["finished"].(bool) {
		t.Fatalf("status %v", st)
	}
	logs := graphPost(t, r, "/api/v1alpha1/graph/node/logs", map[string]any{"projectId": "p1", "graphId": graphID, "graphNodeId": graphID + "-node-1"})
	if l := logs["data"].(map[string]any)["logs"].([]any); len(l) != 2 {
		t.Fatalf("logs %v", logs)
	}
	list := graphPost(t, r, "/api/v1alpha1/project/job/list", map[string]any{"projectId": "p1", "graphId": graphID, "pageNum": 1, "pageSize": 10})["data"].(map[string]any)
	if d := list["data"].([]any); len(d) != 1 || d[0].(map[string]any)["taskCount"].(float64) != 3 {
		t.Fatalf("job list %v", list)
	}
	job := graphPost(t, r, "/api/v1alpha1/project/job/get", map[string]any{"projectId": "p1", "jobId": jobID})["data"].(map[string]any)
	if job["status"] != "RUNNING" || len(job["graph"].(map[string]any)["nodes"].([]any)) != 3 {
		t.Fatalf("job get %v", job)
	}

	// frontend never sends jobId to graph/stop
	if resp := graphPost(t, r, "/api/v1alpha1/graph/stop", map[string]any{"projectId": "p1", "graphId": graphID}); statusCode(resp) != 0 {
		t.Fatalf("stop %v", resp)
	}
	if len(ks.stopped) != 1 || ks.stopped[0] != jobID {
		t.Fatalf("kuscia stop %v", ks.stopped)
	}
	var j model.ProjectJobDO
	db.Where("job_id = ?", jobID).First(&j)
	if j.Status != graphjob.JobStopped {
		t.Fatalf("job status %s", j.Status)
	}

	// legacy job/create is redirected to the graph chain
	created := graphPost(t, r, "/api/v1alpha1/project/job/create", map[string]any{"projectId": "p1", "graphId": graphID, "nodes": []string{graphID + "-node-1"}})
	if statusCode(created) != 0 || created["data"].(map[string]any)["jobId"] == "" {
		t.Fatalf("job/create %v", created)
	}

	// unknown graph -> Java GRAPH_NOT_EXISTS code
	if resp := graphPost(t, r, "/api/v1alpha1/graph/detail", map[string]any{"projectId": "p1", "graphId": "nope"}); statusCode(resp) != 202011702 {
		t.Fatalf("missing graph code %v", resp)
	}
}

func TestProjectEndpoints_ListAndDatatable(t *testing.T) {
	r, _, _ := setupGraphEndpoints(t)
	list := graphPost(t, r, "/api/v1alpha1/project/list", map[string]any{})
	arr, ok := list["data"].([]any)
	if !ok || len(arr) != 1 {
		t.Fatalf("project/list must return every project as an array: %v", list)
	}
	p := arr[0].(map[string]any)
	if p["projectName"] != "proj" || len(p["nodes"].([]any)) != 2 || p["nodes"].([]any)[0].(map[string]any)["nodeName"] != "alice" {
		t.Fatalf("project vo %v", p)
	}
	paged := graphPost(t, r, "/api/v1alpha1/project/list", map[string]any{"page": 1, "size": 5})["data"].(map[string]any)
	if paged["total"].(float64) != 1 {
		t.Fatalf("paged %v", paged)
	}
	add := graphPost(t, r, "/api/v1alpha1/project/datatable/add", map[string]any{"projectId": "p1", "nodeId": "alice", "datatableId": "t9",
		"configs": []any{map[string]any{"colName": "id", "isAssociateKey": true}}})
	if statusCode(add) != 0 {
		t.Fatalf("datatable add %v", add)
	}
	get := graphPost(t, r, "/api/v1alpha1/project/datatable/get", map[string]any{"projectId": "p1", "nodeId": "alice", "datatableId": "t9"})["data"].(map[string]any)
	cfgs := get["configs"].([]any)
	if len(cfgs) != 1 || cfgs[0].(map[string]any)["isAssociateKey"] != true || get["datatableVO"] == nil {
		t.Fatalf("datatable get %v", get)
	}
	graphPost(t, r, "/api/v1alpha1/graph/create", map[string]any{"projectId": "p1", "name": "g"})
	if resp := graphPost(t, r, "/api/v1alpha1/project/delete", map[string]any{"projectId": "p1"}); statusCode(resp) != 202011506 {
		t.Fatalf("delete with graphs must fail with PROJECT_GRAPH_NOT_EMPTY: %v", resp)
	}
}

func TestComponentEndpoints(t *testing.T) {
	s := graphjob.DefaultSettings()
	s.ComponentDir, s.I18nDir = "../../../../config/components", "../../../../config/i18n"
	h := NewMiscHandler(nil, nil).WithComponentService(service.NewComponentServiceWithSettings(s))
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.CaseKeysResponse())
	r.POST("/api/v1alpha1/component/list", h.ListComponents)
	r.POST("/api/v1alpha1/component/batch", h.ComponentBatch)
	r.POST("/api/v1alpha1/component/i18n", h.ComponentI18n)
	r.POST("/api/v1alpha1/version/list", h.VersionList)

	list := graphPost(t, r, "/api/v1alpha1/component/list", map[string]any{})
	apps, ok := list["data"].(map[string]any)
	if !ok || apps["secretflow"] == nil {
		t.Skipf("component config unavailable: %v", list)
	}
	if _, twin := apps["secretpadTee"]; twin {
		t.Fatal("component response must not get camelCase twin keys")
	}
	batch := graphPost(t, r, "/api/v1alpha1/component/batch", []any{map[string]any{"app": "secretflow", "domain": "data_prep", "name": "psi"}})
	if defs := batch["data"].([]any); len(defs) != 1 || defs[0].(map[string]any)["name"] != "psi" {
		t.Fatalf("batch %v", batch)
	}
	if resp := graphPost(t, r, "/api/v1alpha1/component/batch", []any{map[string]any{"app": "secretflow", "domain": "x", "name": "y"}}); statusCode(resp) != 202011701 {
		t.Fatalf("missing component %v", resp)
	}
	i18n := graphPost(t, r, "/api/v1alpha1/component/i18n", map[string]any{})["data"].(map[string]any)
	if i18n["secretflow"] == nil {
		t.Fatalf("i18n %v", i18n)
	}
	if v := graphPost(t, r, "/api/v1alpha1/version/list", map[string]any{})["data"].(map[string]any); v["secretflowImage"] == nil {
		t.Fatalf("versions %v", v)
	}
}
