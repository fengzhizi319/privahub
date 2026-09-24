package service

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/internal/graphjob"
	"github.com/fengzhizi319/privahub/pkg/errcode"
)

func TestStartGraph_PSIChain(t *testing.T) {
	f := newGraphFixture(t)
	f.seedPSIProject()

	vo, err := f.graph.StartGraph(adminCtx(), &StartGraphRequest{ProjectID: "p1", GraphID: "g1", Nodes: []string{"g1-node-1", "g1-node-2", "g1-node-3"}})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if len(vo.JobID) != 4 {
		t.Fatalf("job id %q", vo.JobID)
	}
	var job model.ProjectJobDO
	if err := f.db.Where("job_id = ?", vo.JobID).First(&job).Error; err != nil {
		t.Fatal(err)
	}
	if job.Status != graphjob.JobRunning || job.GraphID != "g1" {
		t.Fatalf("job %+v", job)
	}
	var tasks []model.ProjectJobTaskDO
	f.db.Where("job_id = ?", vo.JobID).Order("id").Find(&tasks)
	if len(tasks) != 3 {
		t.Fatalf("tasks %d", len(tasks))
	}
	status := map[string]string{}
	for _, tk := range tasks {
		status[tk.GraphNodeID] = tk.Status
		var snap graphjob.GraphNodeInfo
		if err := json.Unmarshal([]byte(tk.GraphNode), &snap); err != nil || snap.GraphNodeID != tk.GraphNodeID {
			t.Fatalf("graph_node snapshot missing for %s: %s", tk.TaskID, tk.GraphNode)
		}
	}
	if status["g1-node-1"] != graphjob.TaskSucceed || status["g1-node-3"] != graphjob.TaskInitialized {
		t.Fatalf("statuses %v", status)
	}
	var logs int64
	f.db.Model(&model.ProjectJobTaskLogDO{}).Where("job_id = ?", vo.JobID).Count(&logs)
	if logs != 4 { // start + succeed for the two read_data tasks
		t.Fatalf("pad logs %d", logs)
	}

	if len(f.k.created) != 1 {
		t.Fatalf("kuscia create calls %d", len(f.k.created))
	}
	req := f.k.created[0]
	if req.JobID != vo.JobID || req.Initiator != "alice" || req.MaxParallelism != 2 || len(req.Tasks) != 1 {
		t.Fatalf("create job req %+v", req)
	}
	task := req.Tasks[0]
	if task.TaskID != vo.JobID+"-g1-node-3" || task.AppImage != "secretflow-image" || len(task.Parties) != 2 || len(task.Dependencies) != 0 {
		t.Fatalf("kuscia task %+v", task)
	}
	cfg := decodeTaskInput(t, task.InputConfig)
	ins := cfg["sf_input_ids"].([]any)
	if len(ins) != 2 || ins[0] != "alice-table" || ins[1] != "bob-table" {
		t.Fatalf("inputs %v", ins)
	}
	outs := cfg["sf_output_ids"].([]any)
	if outs[0] != vo.JobID+"-g1-node-3-output-0" {
		t.Fatalf("outputs %v", outs)
	}
	if cfg["sf_output_uris"].([]any)[0] != strings.ReplaceAll(vo.JobID+"-g1-node-3-output-0", "-", "_") {
		t.Fatalf("output uris %v", cfg["sf_output_uris"])
	}
	ds := cfg["sf_datasource_config"].(map[string]any)
	if ds["alice"].(map[string]any)["id"] != "default-data-source" || ds["bob"] == nil {
		t.Fatalf("datasource cfg %v", ds)
	}
	eval := cfg["sf_node_eval_param"].(map[string]any)
	if eval["comp_id"] != "data_prep/psi:1.0.0" || eval["checkpoint_uri"] != "ck"+vo.JobID+"-g1-node-3-output-0" {
		t.Fatalf("eval %v", eval)
	}
	if attrs := cfg["table_attrs"].([]any); len(attrs) != 2 {
		t.Fatalf("table attrs %v", attrs)
	}
	var params model.ProjectGraphNodeKusciaParamsDO
	if err := f.db.Where("graph_node_id = ?", "g1-node-3").First(&params).Error; err != nil || params.JobID != vo.JobID {
		t.Fatalf("kuscia params %+v %v", params, err)
	}
}

func TestStartGraph_BoundDatasourceAndSettings(t *testing.T) {
	f := newGraphFixture(t)
	f.seedPSIProject()
	f.mustCreate(&model.ProjectGraphDomainDatasourceDO{ProjectID: "p1", GraphID: "g1", DomainID: "bob", DatasourceID: "oss-ds"})
	s := graphjob.DefaultSettings()
	s.Devices = []graphjob.DeviceConfig{{Name: "spu", Config: `{"runtime_config":{"protocol":"ABY3"}}`}}
	graphjob.SetSettings(s)

	if _, err := f.graph.StartGraph(adminCtx(), &StartGraphRequest{ProjectID: "p1", GraphID: "g1", Nodes: []string{"g1-node-1", "g1-node-2", "g1-node-3"}}); err != nil {
		t.Fatal(err)
	}
	cfg := decodeTaskInput(t, f.k.created[0].Tasks[0].InputConfig)
	if cfg["sf_datasource_config"].(map[string]any)["bob"].(map[string]any)["id"] != "oss-ds" {
		t.Fatalf("bound datasource not used: %v", cfg["sf_datasource_config"])
	}
	devs := cfg["sf_cluster_desc"].(map[string]any)["devices"].([]any)
	if len(devs) != 1 || !strings.Contains(devs[0].(map[string]any)["config"].(string), "ABY3") {
		t.Fatalf("devices %v", devs)
	}
}

func TestStartGraph_UnselectedPadNeverRun(t *testing.T) {
	f := newGraphFixture(t)
	f.seedPSIProject()
	// Only psi selected and no previous run of the read_data nodes.
	_, err := f.graph.StartGraph(adminCtx(), &StartGraphRequest{ProjectID: "p1", GraphID: "g1", Nodes: []string{"g1-node-3"}})
	if err == nil {
		// seeded fixture: read_data tasks never ran -> Java GRAPH_DEPENDENT_NODE_NOT_RUN
		t.Fatal("expected dependent node not run")
	}
	pe, ok := AsPadError(err)
	if !ok || pe.Code != errcode.GraphDependentNodeNotRun {
		t.Fatalf("err %v", err)
	}
	var cnt int64
	f.db.Model(&model.ProjectJobDO{}).Count(&cnt)
	if cnt != 0 {
		t.Fatalf("failed start must roll back, jobs=%d", cnt)
	}
}

func TestStartGraph_PartialRunReusesUpstreamOutputs(t *testing.T) {
	f := newGraphFixture(t)
	f.seedPSIProject()
	first, err := f.graph.StartGraph(adminCtx(), &StartGraphRequest{ProjectID: "p1", GraphID: "g1", Nodes: []string{"g1-node-1", "g1-node-2", "g1-node-3"}})
	if err != nil {
		t.Fatal(err)
	}
	second, err := f.graph.StartGraph(adminCtx(), &StartGraphRequest{ProjectID: "p1", GraphID: "g1", Nodes: []string{"g1-node-4"}})
	if err != nil {
		t.Fatalf("partial run: %v", err)
	}
	req := f.k.created[1]
	if req.JobID != second.JobID || len(req.Tasks) != 1 || len(req.Tasks[0].Dependencies) != 0 {
		t.Fatalf("partial req %+v", req)
	}
	cfg := decodeTaskInput(t, req.Tasks[0].InputConfig)
	if in := cfg["sf_input_ids"].([]any)[0]; in != first.JobID+"-g1-node-3-output-0" {
		t.Fatalf("upstream output not reused: %v", in)
	}
	// parties of the stats node come from its root datatables (alice + bob)
	if len(req.Tasks[0].Parties) != 2 {
		t.Fatalf("parties %+v", req.Tasks[0].Parties)
	}
}

func TestStartGraph_DependencyInsideJob(t *testing.T) {
	f := newGraphFixture(t)
	f.seedPSIProject()
	vo, err := f.graph.StartGraph(adminCtx(), &StartGraphRequest{ProjectID: "p1", GraphID: "g1"}) // empty = all nodes
	if err != nil {
		t.Fatal(err)
	}
	req := f.k.created[0]
	if len(req.Tasks) != 2 {
		t.Fatalf("tasks %d", len(req.Tasks))
	}
	stats := req.Tasks[1]
	if len(stats.Dependencies) != 1 || stats.Dependencies[0] != vo.JobID+"-g1-node-3" {
		t.Fatalf("deps %v", stats.Dependencies)
	}
	if in := decodeTaskInput(t, stats.InputConfig)["sf_input_ids"].([]any)[0]; in != vo.JobID+"-g1-node-3-output-0" {
		t.Fatalf("in-job input %v", in)
	}
}

func TestStartGraph_PadOnlySucceedsImmediately(t *testing.T) {
	f := newGraphFixture(t)
	f.seedPSIProject()
	vo, err := f.graph.StartGraph(adminCtx(), &StartGraphRequest{ProjectID: "p1", GraphID: "g1", Nodes: []string{"g1-node-1"}})
	if err != nil {
		t.Fatal(err)
	}
	var job model.ProjectJobDO
	f.db.Where("job_id = ?", vo.JobID).First(&job)
	if job.Status != graphjob.JobSucceed || job.FinishedTime == nil {
		t.Fatalf("pad-only job %+v", job)
	}
	if len(f.k.created) != 0 {
		t.Fatal("pad-only job must not be submitted")
	}
}

func TestStartGraph_KusciaFailureRollsBack(t *testing.T) {
	f := newGraphFixture(t)
	f.seedPSIProject()
	f.k.createErr = errors.New("kuscia down")
	_, err := f.graph.StartGraph(adminCtx(), &StartGraphRequest{ProjectID: "p1", GraphID: "g1", Nodes: []string{"g1-node-1", "g1-node-2", "g1-node-3"}})
	pe, ok := AsPadError(err)
	if !ok || pe.Code != errcode.ProjectJobCreateError || !strings.Contains(pe.Error(), "kuscia down") {
		t.Fatalf("err %v", err)
	}
	for _, m := range []any{&model.ProjectJobDO{}, &model.ProjectJobTaskDO{}, &model.ProjectJobTaskLogDO{}, &model.ProjectGraphNodeKusciaParamsDO{}} {
		var cnt int64
		f.db.Model(m).Count(&cnt)
		if cnt != 0 {
			t.Fatalf("%T not rolled back: %d", m, cnt)
		}
	}
}

func TestStartGraph_HealthCheck(t *testing.T) {
	f := newGraphFixture(t)
	f.seedPSIProject()
	f.k.routeNotReady["alice->bob"] = true
	_, err := f.graph.StartGraph(adminCtx(), &StartGraphRequest{ProjectID: "p1", GraphID: "g1", Nodes: []string{"g1-node-1", "g1-node-2", "g1-node-3"}})
	if pe, ok := AsPadError(err); !ok || pe.Code != errcode.GraphNodeRouteNotExists || pe.Args[0] != "Alice->Bob" {
		t.Fatalf("route check: %v", err)
	}
	f.k.routeNotReady = map[string]bool{}
	f.k.notReady["bob"] = true
	_, err = f.graph.StartGraph(adminCtx(), &StartGraphRequest{ProjectID: "p1", GraphID: "g1", Nodes: []string{"g1-node-1", "g1-node-2", "g1-node-3"}})
	if pe, ok := AsPadError(err); !ok || pe.Code != errcode.GraphNodeNotExists {
		t.Fatalf("node check: %v", err)
	}
}

func TestStartGraph_OwnerAndNodeChecks(t *testing.T) {
	f := newGraphFixture(t)
	f.seedPSIProject()
	other := WithOperator(adminCtx(), Operator{OwnerID: "someone-else", OwnerType: "CENTER"})
	if _, err := f.graph.StartGraph(other, &StartGraphRequest{ProjectID: "p1", GraphID: "g1"}); err == nil {
		t.Fatal("non owner must be rejected")
	}
	if _, err := f.graph.StartGraph(adminCtx(), &StartGraphRequest{ProjectID: "p1", GraphID: "g1", Nodes: []string{"nope"}}); err == nil {
		t.Fatal("unknown node must be rejected")
	}
	_, err := f.graph.StartGraph(adminCtx(), &StartGraphRequest{ProjectID: "p1", GraphID: "missing"})
	if !errors.Is(err, ErrGraphNotFound) {
		t.Fatalf("missing graph: %v", err)
	}
}

func TestRunGraphJob_ScheduledMode(t *testing.T) {
	f := newGraphFixture(t)
	f.seedPSIProject()
	res, err := f.graph.RunGraphJob(adminCtx(), &StartGraphRequest{ProjectID: "p1", GraphID: "g1", Nodes: []string{"g1-node-1", "g1-node-2", "g1-node-3"}},
		StartGraphOptions{Scheduled: true, ScheduleExpectStartDate: "20260924"})
	if err != nil {
		t.Fatal(err)
	}
	var cnt int64
	f.db.Model(&model.ProjectJobDO{}).Count(&cnt)
	if cnt != 0 || len(f.k.created) != 0 {
		t.Fatalf("scheduled mode persisted/submitted: jobs=%d creates=%d", cnt, len(f.k.created))
	}
	if res.Job == nil || len(res.Tasks) != 3 || len(res.Logs) != 4 || res.Request == nil {
		t.Fatalf("scheduled result %+v", res)
	}
	uri := decodeTaskInput(t, res.Request.Tasks[0].InputConfig)["sf_output_uris"].([]any)[0].(string)
	if !strings.HasSuffix(uri, "_20260924") {
		t.Fatalf("schedule date suffix missing: %s", uri)
	}
	if err := f.graph.SubmitRenderedJob(adminCtx(), res.Request); err != nil || len(f.k.created) != 1 {
		t.Fatalf("submit rendered: %v", err)
	}
}

func TestStartGraph_BinningModificationsAdapter(t *testing.T) {
	f := newGraphFixture(t)
	f.seedPSIProject()
	// Run the upstream psi once, then a binning_modifications node fed by it.
	if _, err := f.graph.StartGraph(adminCtx(), &StartGraphRequest{ProjectID: "p1", GraphID: "g1", Nodes: []string{"g1-node-1", "g1-node-2", "g1-node-3"}}); err != nil {
		t.Fatal(err)
	}
	f.mustCreate(nodeDOFromInfo("p1", "g1", &graphjob.GraphNodeInfo{
		CodeName: graphjob.BinningModificationsCodeName, GraphNodeID: "g1-node-5",
		Inputs: []string{"g1-node-3-output-0"}, Outputs: []string{"g1-node-5-output-0", "g1-node-5-output-1"},
		NodeDef: json.RawMessage(`{"domain":"feature","name":"binning_modifications","version":"0.0.1","attrPaths":[],"attrs":[{"s":"{\"modelHash\":\"h1\"}"}]}`),
	}))
	vo, err := f.graph.StartGraph(adminCtx(), &StartGraphRequest{ProjectID: "p1", GraphID: "g1", Nodes: []string{"g1-node-5"}})
	if err != nil {
		t.Fatal(err)
	}
	req := f.k.created[1]
	if len(req.Tasks) != 2 {
		t.Fatalf("adapter should split the task, got %d", len(req.Tasks))
	}
	write, read := req.Tasks[0], req.Tasks[1]
	base := vo.JobID + "-g1-node-5"
	if write.TaskID != base+"--1" || read.TaskID != base || len(read.Dependencies) != 1 || read.Dependencies[0] != base+"--1" {
		t.Fatalf("split tasks %s %s %v", write.TaskID, read.TaskID, read.Dependencies)
	}
	wcfg := decodeTaskInput(t, write.InputConfig)
	if wcfg["sf_node_eval_param"].(map[string]any)["comp_id"] != "io/write_data:1.0.0" {
		t.Fatalf("write comp %v", wcfg["sf_node_eval_param"])
	}
	rcfg := decodeTaskInput(t, read.InputConfig)
	if rcfg["sf_node_eval_param"].(map[string]any)["comp_id"] != "io/read_data:1.0.0" ||
		rcfg["sf_input_ids"].([]any)[0] != base+"-output-0" || rcfg["sf_output_ids"].([]any)[0] != base+"-output-1" {
		t.Fatalf("read task cfg %v", rcfg)
	}
}

func TestStartGraph_TEEProjectRejected(t *testing.T) {
	f := newGraphFixture(t)
	f.seedPSIProject()
	f.db.Model(&model.ProjectDO{}).Where("project_id = ?", "p1").Update("compute_mode", "TEE")
	if _, err := f.graph.StartGraph(adminCtx(), &StartGraphRequest{ProjectID: "p1", GraphID: "g1"}); err == nil {
		t.Fatal("TEE execution is not migrated and must fail explicitly")
	}
}
