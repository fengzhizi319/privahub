package service

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/internal/graphjob"
	"github.com/fengzhizi319/privahub/pkg/errcode"
	"github.com/fengzhizi319/privahub/pkg/kuscia"
)

func intPtr(i int) *int { return &i }

func TestGraphCRUD(t *testing.T) {
	f := newGraphFixture(t)
	f.seedPSIProject()
	ctx := adminCtx()

	created, err := f.graph.CreateGraph(ctx, &CreateGraphRequest{ProjectID: "p1", Name: "new",
		Nodes: []graphjob.GraphNodeInfo{{CodeName: "data_prep/psi", GraphNodeID: "x-node-1", NodeDef: json.RawMessage(`{"domain":"data_prep","name":"psi","version":"1.0.0"}`)}},
		Edges: []graphjob.GraphEdge{{EdgeID: "e", Source: "a", Target: "b"}}})
	if err != nil {
		t.Fatal(err)
	}
	var g model.ProjectGraphDO
	f.db.Where("graph_id = ?", created.GraphID).First(&g)
	if g.NodeMaxIndex != 32 || g.MaxParallelism != 1 || g.OwnerID != "admin" || !strings.Contains(g.Edges, `"edgeId":"e"`) {
		t.Fatalf("graph %+v", g)
	}
	var bindings []model.ProjectGraphDomainDatasourceDO
	f.db.Where("graph_id = ?", created.GraphID).Find(&bindings)
	if len(bindings) != 2 || bindings[0].DatasourceID != graphjob.DefaultDataSourceID || bindings[0].EditEnable == nil || !*bindings[0].EditEnable {
		t.Fatalf("default datasource bindings %+v", bindings)
	}

	// update without nodes must not clear the canvas
	if err := f.graph.FullUpdateGraph(ctx, &FullUpdateGraphRequest{ProjectID: "p1", GraphID: created.GraphID, MaxParallelism: intPtr(3)}); err != nil {
		t.Fatal(err)
	}
	detail, err := f.graph.GetGraphDetail(ctx, &GetGraphRequest{ProjectID: "p1", GraphID: created.GraphID})
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Nodes) != 1 || detail.MaxParallelism != 3 || len(detail.Edges) != 1 || detail.Nodes[0].Status != graphjob.TaskStaging {
		t.Fatalf("detail %+v", detail)
	}
	if len(detail.DataSourceConfig) != 2 || !detail.DataSourceConfig[0].EditEnable || detail.DataSourceConfig[0].NodeName != "Alice" {
		t.Fatalf("ds config %+v", detail.DataSourceConfig)
	}

	// datasource config is validated and locked
	f.k.datasources["alice"] = []kuscia.DomainDataSource{{DatasourceID: "oss-1", Name: "oss one", Type: "oss"}}
	err = f.graph.FullUpdateGraph(ctx, &FullUpdateGraphRequest{ProjectID: "p1", GraphID: created.GraphID,
		DataSourceConfig: []GraphDataSourceConfig{{NodeID: "alice", DataSourceID: "missing"}}})
	if pe, ok := AsPadError(err); !ok || pe.Code != errcode.SystemValidationError {
		t.Fatalf("invalid datasource: %v", err)
	}
	if err := f.graph.FullUpdateGraph(ctx, &FullUpdateGraphRequest{ProjectID: "p1", GraphID: created.GraphID, Nodes: []graphjob.GraphNodeInfo{},
		DataSourceConfig: []GraphDataSourceConfig{{NodeID: "alice", DataSourceID: "oss-1"}}}); err != nil {
		t.Fatal(err)
	}
	detail, _ = f.graph.GetGraphDetail(ctx, &GetGraphRequest{ProjectID: "p1", GraphID: created.GraphID})
	if len(detail.Nodes) != 0 {
		t.Fatal("explicit empty nodes must clear the canvas")
	}
	var b model.ProjectGraphDomainDatasourceDO
	f.db.Where("graph_id = ? AND domain_id = ?", created.GraphID, "alice").First(&b)
	if b.DatasourceID != "oss-1" || b.DatasourceName != "oss one" || *b.EditEnable {
		t.Fatalf("binding %+v", b)
	}
	err = f.graph.FullUpdateGraph(ctx, &FullUpdateGraphRequest{ProjectID: "p1", GraphID: created.GraphID,
		DataSourceConfig: []GraphDataSourceConfig{{NodeID: "alice", DataSourceID: "default-data-source"}}})
	if err == nil {
		f.k.datasources["alice"] = append(f.k.datasources["alice"], kuscia.DomainDataSource{DatasourceID: "default-data-source"})
		t.Fatal("locked binding must not be edited")
	}

	// graph/node/update: whole node replacement + owner check
	if err := f.graph.UpdateGraphNode(ctx, &UpdateGraphNodeRequest{ProjectID: "p1", GraphID: "g1", Node: &graphjob.GraphNodeInfo{GraphNodeID: "g1-node-4", CodeName: "stats/table_statistics", Label: "renamed"}}); err != nil {
		t.Fatal(err)
	}
	var n model.ProjectGraphNodeDO
	f.db.Where("graph_node_id = ?", "g1-node-4").First(&n)
	if n.Label != "renamed" || n.Inputs != "[]" {
		t.Fatalf("node replaced %+v", n)
	}
	other := WithOperator(context.Background(), Operator{OwnerID: "x"})
	if err := f.graph.UpdateGraphNode(other, &UpdateGraphNodeRequest{ProjectID: "p1", GraphID: "g1", Node: &graphjob.GraphNodeInfo{GraphNodeID: "g1-node-4"}}); err == nil {
		t.Fatal("owner check")
	}
	if err := f.graph.UpdateGraphNode(ctx, &UpdateGraphNodeRequest{ProjectID: "p1", GraphID: "g1", Node: &graphjob.GraphNodeInfo{GraphNodeID: "nope"}}); err == nil {
		t.Fatal("missing node")
	}

	metas, _ := f.graph.ListGraph(ctx, &ListGraphRequest{ProjectID: "p1"})
	if len(metas) != 2 {
		t.Fatalf("list %v", metas)
	}
	if err := f.graph.DeleteGraph(other, &DeleteGraphRequest{ProjectID: "p1", GraphID: created.GraphID}); err == nil {
		t.Fatal("delete owner check")
	}
	if err := f.graph.DeleteGraph(ctx, &DeleteGraphRequest{ProjectID: "p1", GraphID: created.GraphID}); err != nil {
		t.Fatal(err)
	}
	var cnt int64
	f.db.Model(&model.ProjectGraphDomainDatasourceDO{}).Where("graph_id = ?", created.GraphID).Count(&cnt)
	if cnt != 0 {
		t.Fatal("graph datasource bindings not cleaned")
	}
	if _, err := f.graph.GetGraphDetail(ctx, &GetGraphRequest{ProjectID: "p1", GraphID: created.GraphID}); !errors.Is(err, ErrGraphNotFound) {
		t.Fatalf("deleted graph %v", err)
	}
}

func TestRefreshNodeMaxIndex(t *testing.T) {
	f := newGraphFixture(t)
	f.seedPSIProject()
	ctx := adminCtx()
	vo, _ := f.graph.RefreshNodeMaxIndex(ctx, &RefreshNodeMaxIndexRequest{ProjectID: "p1", GraphID: "g1"})
	if vo.MaxIndex != 32 {
		t.Fatalf("first %d", vo.MaxIndex)
	}
	vo, _ = f.graph.RefreshNodeMaxIndex(ctx, &RefreshNodeMaxIndexRequest{ProjectID: "p1", GraphID: "g1"})
	if vo.MaxIndex != 33 {
		t.Fatalf("second %d", vo.MaxIndex)
	}
	vo, _ = f.graph.RefreshNodeMaxIndex(ctx, &RefreshNodeMaxIndexRequest{ProjectID: "p1", GraphID: "g1", CurrentIndex: intPtr(50)})
	if vo.MaxIndex != 50 {
		t.Fatalf("current index %d", vo.MaxIndex)
	}
	vo, _ = f.graph.RefreshNodeMaxIndex(ctx, &RefreshNodeMaxIndexRequest{ProjectID: "p1", GraphID: "g1", CurrentIndex: intPtr(3)})
	if vo.MaxIndex != 51 {
		t.Fatalf("after current index %d", vo.MaxIndex)
	}
}

func TestNodeStatusLogsAndStop(t *testing.T) {
	f := newGraphFixture(t)
	f.seedPSIProject()
	ctx := adminCtx()
	st, err := f.graph.ListNodeStatus(ctx, &ListGraphNodeStatusRequest{ProjectID: "p1", GraphID: "g1"})
	if err != nil || !st.Finished || len(st.Nodes) != 4 || st.Nodes[0].Status != graphjob.TaskStaging {
		t.Fatalf("idle status %+v %v", st, err)
	}
	if _, err := f.graph.GetNodeLogs(ctx, &GraphNodeLogsRequest{ProjectID: "p1", GraphID: "g1", GraphNodeID: "g1-node-3"}); err == nil {
		t.Fatal("logs without task must fail")
	}
	jobID := startPSI(t, f)
	st, _ = f.graph.ListNodeStatus(ctx, &ListGraphNodeStatusRequest{ProjectID: "p1", GraphID: "g1"})
	byNode := map[string]GraphNodeStatusVO{}
	for _, n := range st.Nodes {
		byNode[n.GraphNodeID] = n
	}
	if st.Finished || byNode["g1-node-3"].Status != graphjob.TaskInitialized || byNode["g1-node-3"].JobID != jobID ||
		len(byNode["g1-node-3"].Parties) != 2 || byNode["g1-node-4"].Status != graphjob.TaskStaging || byNode["g1-node-1"].Status != graphjob.TaskSucceed {
		t.Fatalf("status %+v", st)
	}
	logs, err := f.graph.GetNodeLogs(ctx, &GraphNodeLogsRequest{ProjectID: "p1", GraphID: "g1", GraphNodeID: "g1-node-1"})
	if err != nil || len(logs.Logs) != 2 || logs.Status != graphjob.TaskSucceed {
		t.Fatalf("read_data logs %+v %v", logs, err)
	}

	// graph/stop without graphNodeId stops every RUNNING job of the graph
	if err := f.graph.StopGraph(ctx, &StopGraphRequest{ProjectID: "p1", GraphID: "g1"}); err != nil {
		t.Fatal(err)
	}
	if len(f.k.stopped) != 1 || f.k.stopped[0] != jobID {
		t.Fatalf("stopped %v", f.k.stopped)
	}
	var task model.ProjectJobTaskDO
	f.db.Where("task_id = ?", jobID+"-g1-node-3").First(&task)
	if task.Status != graphjob.TaskStopped {
		t.Fatalf("task %+v", task)
	}
	if l := taskLogs(f, jobID, task.TaskID); len(l) != 1 || !strings.Contains(l[0], "stopped") {
		t.Fatalf("stop log %v", l)
	}
	var job model.ProjectJobDO
	f.db.Where("job_id = ?", jobID).First(&job)
	if job.Status != graphjob.JobStopped {
		t.Fatalf("job %+v", job)
	}

	// graph/stop with graphNodeId stops the job running that node
	job2 := startPSI(t, f)
	f.db.Model(&model.ProjectJobTaskDO{}).Where("task_id = ?", job2+"-g1-node-3").Update("status", graphjob.TaskRunning)
	if err := f.graph.StopGraph(ctx, &StopGraphRequest{ProjectID: "p1", GraphID: "g1", GraphNodeID: "g1-node-3"}); err != nil {
		t.Fatal(err)
	}
	if len(f.k.stopped) != 2 || f.k.stopped[1] != job2 {
		t.Fatalf("stopped %v", f.k.stopped)
	}
}

func TestGraphNodeOutput_ReadDataAndFedTable(t *testing.T) {
	f := newGraphFixture(t)
	f.seedPSIProject()
	ctx := adminCtx()
	jobID := startPSI(t, f)
	out, err := f.graph.GetNodeOutput(ctx, &GraphNodeOutputRequest{ProjectID: "p1", GraphID: "g1", GraphNodeID: "g1-node-1", OutputID: "g1-node-1-output-0"})
	if err != nil {
		t.Fatal(err)
	}
	if out.Type != "table" || out.Meta == nil || len(out.Meta.Rows) != 1 || out.Meta.Rows[0].NodeName != "Alice" ||
		out.Meta.Rows[0].Path != "alice-table.csv" || out.Meta.Rows[0].Fields != "id,y,x" || out.Meta.Headers[0]["name"] != "metas" {
		t.Fatalf("read_data output %+v", out.Meta)
	}
	outID := jobID + "-g1-node-3-output-0"
	f.k.addDomainData("alice", outID, "table", []kuscia.DataColumn{{Name: "id", Type: "str"}}, nil)
	if err := f.sync.SyncJob(context.Background(), jobID, &kuscia.JobStatusDetailInfo{State: "Running", Tasks: []kuscia.JobTaskStatusDetail{{TaskID: jobID + "-g1-node-3", State: "Succeeded"}}}); err != nil {
		t.Fatal(err)
	}
	out, err = f.graph.GetNodeOutput(ctx, &GraphNodeOutputRequest{ProjectID: "p1", GraphID: "g1", GraphNodeID: "g1-node-3", OutputID: "g1-node-3-output-0"})
	if err != nil {
		t.Fatal(err)
	}
	if out.Type != "table" || len(out.Meta.Rows) != 1 || out.Meta.Rows[0].TableID != outID || out.Meta.Rows[0].DatasourceType != "LOCAL" || out.TaskID != jobID+"-g1-node-3" {
		t.Fatalf("fed table output %+v rows=%+v", out, out.Meta.Rows)
	}
	if _, err := f.graph.GetNodeOutput(ctx, &GraphNodeOutputRequest{ProjectID: "p1", GraphID: "g1", GraphNodeID: "g1-node-4"}); err == nil {
		t.Fatal("never-run node output must fail")
	}
}

func TestJobGetStopLogs(t *testing.T) {
	f := newGraphFixture(t)
	f.seedPSIProject()
	ctx := adminCtx()
	jobID := startPSI(t, f)
	// the graph changes after the run: job/get must use the task snapshot
	f.db.Model(&model.ProjectGraphNodeDO{}).Where("graph_node_id = ?", "g1-node-3").Update("label", "changed")
	vo, err := f.job.GetJob(ctx, &GetJobRequest{ProjectID: "p1", JobID: jobID})
	if err != nil {
		t.Fatal(err)
	}
	if vo.Status != graphjob.JobRunning || vo.Finished || vo.Graph == nil || len(vo.Graph.Nodes) != 3 || len(vo.Graph.Edges) != 3 {
		t.Fatalf("job vo %+v", vo)
	}
	for _, n := range vo.Graph.Nodes {
		if n.GraphNodeID == "g1-node-3" && (n.Label != "psi" || n.TaskID != jobID+"-g1-node-3" || len(n.Parties) != 2) {
			t.Fatalf("snapshot node %+v", n)
		}
	}
	if _, err := f.job.GetJob(ctx, &GetJobRequest{ProjectID: "p1", JobID: "nope"}); err == nil {
		t.Fatal("missing job")
	}
	logs, err := f.job.GetTaskLogs(ctx, &GetTaskLogRequest{ProjectID: "p1", JobID: jobID, TaskID: jobID + "-g1-node-1"})
	if err != nil || logs.Status != graphjob.TaskSucceed || len(logs.Logs) != 2 {
		t.Fatalf("task logs %+v %v", logs, err)
	}
	other := WithOperator(context.Background(), Operator{OwnerID: "x"})
	if err := f.job.StopJob(other, &StopJobRequest{ProjectID: "p1", JobID: jobID}); err == nil {
		t.Fatal("stop owner check")
	}
	if err := f.job.StopJob(ctx, &StopJobRequest{ProjectID: "p1", JobID: jobID}); err != nil {
		t.Fatal(err)
	}
	vo, _ = f.job.GetJob(ctx, &GetJobRequest{ProjectID: "p1", JobID: jobID})
	if vo.Status != graphjob.JobStopped || !vo.Finished {
		t.Fatalf("stopped job %+v", vo)
	}
	list, _ := f.job.ListJobs(ctx, &JobListRequest{ProjectID: "p1", GraphID: "other"})
	if len(list.Data) != 0 {
		t.Fatal("graph filter")
	}
	list, _ = f.job.ListJobs(ctx, &JobListRequest{ProjectID: "p1", PageNum: 1, PageSize: 1})
	if len(list.Data) != 1 || list.PageTotal != 1 || list.Total != 1 {
		t.Fatalf("paging %+v", list)
	}
}

func TestProjectEndpoints(t *testing.T) {
	f := newGraphFixture(t)
	f.seedPSIProject()
	ctx := adminCtx()
	f.mustCreate(&model.InstDO{InstID: "inst1", Name: "i"})

	vo, err := f.project.CreateProject(ctx, &CreateProjectRequest{Name: "tee", ComputeMode: "tee"}, "admin")
	if err == nil {
		t.Fatalf("TEE without teeNodeId must fail: %+v", vo)
	}
	vo, err = f.project.CreateProject(ctx, &CreateProjectRequest{Name: "p2", ComputeMode: "mpc"}, "admin")
	if err != nil || vo.ComputeMode != "MPC" || vo.ProjectID == "" {
		t.Fatalf("create %+v %v", vo, err)
	}
	vo, _ = f.project.CreateProject(ctx, &CreateProjectRequest{Name: "p3", ComputeMode: "TEE", TeeNodeID: "tee"}, "admin")
	if vo.TeeNodeID != "tee" || vo.ComputeMode != "TEE" {
		t.Fatalf("tee project %+v", vo)
	}
	all, err := f.project.ListAllProjects(ctx, "")
	if err != nil || len(all) != 3 {
		t.Fatalf("list all %d %v", len(all), err)
	}
	var p1 ProjectVO
	for _, p := range all {
		if p.ProjectID == "p1" {
			p1 = p
		}
	}
	if p1.GraphCount != 1 || len(p1.Nodes) != 2 || p1.Nodes[0].NodeName != "Alice" {
		t.Fatalf("p1 vo %+v", p1)
	}
	edge := WithOperator(context.Background(), Operator{OwnerID: "bob", OwnerType: "EDGE"})
	if edgeList, _ := f.project.ListAllProjects(edge, ""); len(edgeList) != 1 || edgeList[0].ProjectID != "p1" {
		t.Fatalf("edge filter %+v", edgeList)
	}
	got, err := f.project.GetProject(ctx, "p1")
	if err != nil || len(got.Nodes[0].Datatables) != 1 || got.Nodes[0].Datatables[0].DatatableName != "alice-table-name" {
		t.Fatalf("get %+v %v", got, err)
	}
	if err := f.project.DeleteProject(ctx, "p1"); err == nil {
		t.Fatal("project with graphs must not be deleted")
	} else if pe, ok := AsPadError(err); !ok || pe.Code != errcode.ProjectGraphNotEmpty {
		t.Fatalf("delete err %v", err)
	}
	if err := f.project.DeleteProject(ctx, vo.ProjectID); err != nil {
		t.Fatal(err)
	}

	// node / inst add: validated and idempotent
	if err := f.project.AddNode(ctx, "p1", "ghost"); err == nil {
		t.Fatal("unknown node")
	}
	for i := 0; i < 2; i++ {
		if err := f.project.AddNode(ctx, "p1", "alice"); err != nil {
			t.Fatalf("idempotent add node: %v", err)
		}
		if err := f.project.AddInst(ctx, "p1", "inst1"); err != nil {
			t.Fatalf("idempotent add inst: %v", err)
		}
	}
	if err := f.project.AddInst(ctx, "p1", "ghost"); err == nil {
		t.Fatal("unknown inst")
	}

	// datatable/add merges the Kuscia schema and grants to the other nodes
	f.k.addDomainData("alice", "t2", "table", []kuscia.DataColumn{{Name: "id", Type: "str", Comment: "c"}, {Name: "age", Type: "int"}}, nil)
	err = f.project.AddDatatable(ctx, &AddDatatableRequest{ProjectID: "p1", NodeID: "alice", DatatableID: "t2",
		Configs: []TableColumnConfigParam{{ColName: "id", IsAssociateKey: true}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(f.k.grantCreates) != 1 || f.k.grantCreates[0].DomainDataGrantID != "t2-bob" || f.k.grantCreates[0].GrantDomain != "bob" {
		t.Fatalf("grants %+v", f.k.grantCreates)
	}
	_ = f.project.AddDatatable(ctx, &AddDatatableRequest{ProjectID: "p1", NodeID: "alice", DatatableID: "t2",
		Configs: []TableColumnConfigParam{{ColName: "id", IsAssociateKey: true}}})
	if len(f.k.grantCreates) != 1 {
		t.Fatal("existing grant must not be re-created")
	}
	if err := f.project.AddDatatable(ctx, &AddDatatableRequest{ProjectID: "p1", NodeID: "carol", DatatableID: "t2"}); err == nil {
		t.Fatal("node not in project")
	}
	if err := f.project.AddDatatable(ctx, &AddDatatableRequest{ProjectID: "p1", NodeID: "alice", DatatableID: "absent"}); err == nil {
		t.Fatal("datatable must exist in kuscia")
	}
	dt, err := f.project.GetDatatable(ctx, &ProjGetDatatableRequest{ProjectID: "p1", NodeID: "alice", DatatableID: "t2"})
	if err != nil || len(dt.Configs) != 2 || !dt.Configs[0].IsAssociateKey || dt.Configs[0].ColComment != "c" || dt.DatatableVO == nil || dt.NodeName != "Alice" {
		t.Fatalf("datatable get %+v %v", dt, err)
	}
	// update/tableConfig: column-wise merge, must exist
	if err := f.project.UpdateTableConfig(ctx, &AddDatatableRequest{ProjectID: "p1", NodeID: "alice", DatatableID: "t2",
		Configs: []TableColumnConfigParam{{ColName: "age", IsProtection: true}}}); err != nil {
		t.Fatal(err)
	}
	dt, _ = f.project.GetDatatable(ctx, &ProjGetDatatableRequest{ProjectID: "p1", NodeID: "alice", DatatableID: "t2"})
	if dt.Configs[0].IsAssociateKey || !dt.Configs[1].IsProtection || dt.Configs[1].ColType != "int" {
		t.Fatalf("merged configs %+v", dt.Configs)
	}
	if err := f.project.UpdateTableConfig(ctx, &AddDatatableRequest{ProjectID: "p1", NodeID: "alice", DatatableID: "zzz"}); err == nil {
		t.Fatal("update of missing datatable")
	}
	if err := f.project.DeleteDatatable(ctx, &ProjDeleteDatatableRequest{ProjectID: "p1", NodeID: "alice", DatatableID: "t2"}); err != nil {
		t.Fatal(err)
	}
	if err := f.project.DeleteDatatable(ctx, &ProjDeleteDatatableRequest{ProjectID: "p1", NodeID: "alice", DatatableID: "t2"}); err == nil {
		t.Fatal("delete of missing datatable")
	}
	// datasource/list from Kuscia; TEE projects only list LOCAL
	f.k.datasources["alice"] = []kuscia.DomainDataSource{{DatasourceID: "d1", Type: "localfs"}, {DatasourceID: "d2", Type: "oss"}}
	dss, err := f.project.ListProjectDatasources(ctx, "p1")
	if err != nil || len(dss) != 2 || len(dss[0].DataSources) != 2 || dss[0].NodeName != "Alice" {
		t.Fatalf("datasources %+v %v", dss, err)
	}
	f.db.Model(&model.ProjectDO{}).Where("project_id = ?", "p1").Update("compute_mode", "TEE")
	dss, _ = f.project.ListProjectDatasources(ctx, "p1")
	if len(dss[0].DataSources) != 1 || dss[0].DataSources[0].Type != "LOCAL" {
		t.Fatalf("tee datasources %+v", dss)
	}
}

func TestComponentService(t *testing.T) {
	dir := t.TempDir()
	comps := filepath.Join(dir, "components")
	i18n := filepath.Join(dir, "i18n")
	_ = os.MkdirAll(comps, 0o755)
	_ = os.MkdirAll(i18n, 0o755)
	write := func(p, s string) {
		if err := os.WriteFile(p, []byte(s), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(comps, "secretflow.json"), `{"name":"secretflow","desc":"sf","version":"1.0","comps":[
	  {"domain":"data_prep","name":"psi","version":"1.0.0","desc":"psi","attrs":[{"name":"p"}]},
	  {"domain":"io","name":"read_data","version":"1.0.0","desc":"hidden"}]}`)
	write(filepath.Join(comps, "secretpad.json"), `{"name":"secretpad","comps":[{"domain":"read_data","name":"datatable","version":"0.0.1"}]}`)
	write(filepath.Join(comps, "scql.json"), `{"name":"scql","comps":[{"domain":"stats","name":"scql_analysis","version":"0.0.1"}]}`)
	write(filepath.Join(i18n, "secretflow.json"), `{"data_prep/psi:1.0.0":{"psi":"隐私求交"},"io/read_data:1.0.0":{"x":"y"}}`)
	write(filepath.Join(i18n, "secretpad.json"), `{"read_data/datatable:0.0.1":{"read_data":"读样本表"}}`)
	write(filepath.Join(i18n, "scql.json"), `{"stats/scql_analysis:0.0.1":{"a":"b"}}`)
	s := graphjob.DefaultSettings()
	s.ComponentDir, s.I18nDir = comps, i18n
	s.Hide = append(s.Hide, "secretflow/io/read_data:1.0.0")
	cs := NewComponentServiceWithSettings(s)

	list, err := cs.ListComponents()
	if err != nil {
		t.Fatal(err)
	}
	sf, ok := list["secretflow"]
	if !ok || len(list) != 1 {
		t.Fatalf("apps %v", list)
	}
	names := []string{}
	for _, c := range sf.Comps {
		names = append(names, c.Domain+"/"+c.Name)
	}
	if strings.Join(names, ",") != "data_prep/psi,read_data/datatable,stats/scql_analysis" {
		t.Fatalf("comps %v", names)
	}
	defs, err := cs.BatchGetComponent([]ComponentKey{{App: "secretflow", Domain: "read_data", Name: "datatable"}, {Domain: "data_prep", Name: "psi"}})
	if err != nil || len(defs) != 2 || !strings.Contains(string(defs[1]), `"attrs"`) {
		t.Fatalf("batch %v %v", defs, err)
	}
	if _, err := cs.BatchGetComponent([]ComponentKey{{App: "secretflow", Domain: "x", Name: "y"}}); err == nil {
		t.Fatal("missing component must fail")
	}
	tr, err := cs.ListComponentI18n()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := tr["secretflow"]["read_data/datatable:0.0.1"]; !ok {
		t.Fatal("secretpad i18n not merged")
	}
	if _, ok := tr["secretflow"]["stats/scql_analysis:0.0.1"]; !ok {
		t.Fatal("scql i18n not merged")
	}
	if _, ok := tr["secretflow"]["io/read_data:1.0.0"]; ok {
		t.Fatal("hidden key not removed")
	}
	if _, ok := tr["secretpad"]; ok {
		t.Fatal("secretpad app must be merged, not listed")
	}
	if v := cs.VersionList(); v["secretflowImage"] == "" || v["teeAppImage"] == "" {
		t.Fatalf("versions %v", v)
	}
}

func TestComponentService_RepoConfig(t *testing.T) {
	s := graphjob.DefaultSettings()
	s.ComponentDir, s.I18nDir = "../../config/components", "../../config/i18n"
	cs := NewComponentServiceWithSettings(s)
	list, err := cs.ListComponents()
	if err != nil {
		t.Skipf("repo component config unavailable: %v", err)
	}
	if len(list["secretflow"].Comps) < 10 {
		t.Fatalf("expected generated SecretFlow components, got %d", len(list["secretflow"].Comps))
	}
	if _, err := cs.BatchGetComponent([]ComponentKey{{App: "secretflow", Domain: "data_prep", Name: "psi"}, {App: "secretflow", Domain: "read_data", Name: "datatable"}}); err != nil {
		t.Fatal(err)
	}
}
