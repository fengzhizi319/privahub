package service

import (
	"context"
	"strings"
	"testing"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/internal/graphjob"
	"github.com/fengzhizi319/privahub/pkg/kuscia"
)

func startPSI(t *testing.T, f *graphFixture) string {
	t.Helper()
	vo, err := f.graph.StartGraph(adminCtx(), &StartGraphRequest{ProjectID: "p1", GraphID: "g1", Nodes: []string{"g1-node-1", "g1-node-2", "g1-node-3"}})
	if err != nil {
		t.Fatal(err)
	}
	return vo.JobID
}

func taskLogs(f *graphFixture, jobID, taskID string) []string {
	lines, _ := taskLogLines(context.Background(), f.db, jobID, taskID)
	return lines
}

func TestSyncJob_RunningThenSucceededWritesLogsAndResults(t *testing.T) {
	f := newGraphFixture(t)
	f.seedPSIProject()
	jobID := startPSI(t, f)
	taskID := jobID + "-g1-node-3"
	ctx := context.Background()

	err := f.sync.SyncJob(ctx, jobID, &kuscia.JobStatusDetailInfo{State: "Running", Tasks: []kuscia.JobTaskStatusDetail{
		{TaskID: taskID, Alias: taskID, State: "Running", Progress: 0.5},
	}})
	if err != nil {
		t.Fatal(err)
	}
	var task model.ProjectJobTaskDO
	f.db.Where("task_id = ?", taskID).First(&task)
	if task.Status != graphjob.TaskRunning || taskProgress(&task) == nil || *taskProgress(&task) != 0.5 {
		t.Fatalf("running task %+v", task)
	}
	if l := taskLogs(f, jobID, taskID); len(l) != 1 || !strings.Contains(l[0], "start ...") {
		t.Fatalf("start log %v", l)
	}

	out := jobID + "-g1-node-3-output-0"
	cols := []kuscia.DataColumn{{Name: "id", Type: "str"}, {Name: "x", Type: "float"}}
	f.k.addDomainData("alice", out, "table", cols, nil)
	f.k.addDomainData("bob", out, "table", cols[:1], nil)
	err = f.sync.SyncJob(ctx, jobID, &kuscia.JobStatusDetailInfo{State: "Succeeded", EndTime: "2026-09-24T10:00:00Z", Tasks: []kuscia.JobTaskStatusDetail{
		{TaskID: taskID, Alias: taskID, State: "Succeeded", Progress: 1},
	}})
	if err != nil {
		t.Fatal(err)
	}
	var job model.ProjectJobDO
	f.db.Where("job_id = ?", jobID).First(&job)
	if job.Status != graphjob.JobSucceed || job.FinishedTime == nil {
		t.Fatalf("job %+v", job)
	}
	if l := taskLogs(f, jobID, taskID); len(l) != 2 || !strings.Contains(l[1], "succeed") {
		t.Fatalf("succeed log %v", l)
	}
	var results []model.ProjectResultDO
	f.db.Where("job_id = ?", jobID).Order("node_id").Find(&results)
	if len(results) != 2 || results[0].Kind != graphjob.ResultFedTable || results[0].RefID != out || results[0].TaskID != taskID {
		t.Fatalf("results %+v", results)
	}
	var fed model.ProjectFedTableDO
	if err := f.db.Where("fed_table_id = ?", out).First(&fed).Error; err != nil || !strings.Contains(fed.Joins, "bob") {
		t.Fatalf("fed table %+v %v", fed, err)
	}
	var dt model.ProjectDatatableDO
	if err := f.db.Where("node_id = ? AND datatable_id = ?", "alice", out).First(&dt).Error; err != nil || dt.Source != "CREATED" ||
		len(graphjob.ParseColumnConfigs(dt.TableConfigs)) != 2 {
		t.Fatalf("created datatable %+v %v", dt, err)
	}

	// job list counts come from the synced results
	list, err := f.job.ListJobs(ctx, &JobListRequest{ProjectID: "p1", GraphID: "g1"})
	if err != nil || len(list.Data) != 1 {
		t.Fatalf("list %v %+v", err, list)
	}
	if s := list.Data[0]; s.TableCount != 1 || s.TaskCount != 3 || s.FinishedTaskCount != 3 || !s.Finished || s.Graph == nil || s.Graph.Name != "psi-graph" {
		t.Fatalf("summary %+v", s)
	}
	// finished jobs are skipped by later events
	if err := f.sync.SyncJob(ctx, jobID, &kuscia.JobStatusDetailInfo{State: "Failed"}); err != nil {
		t.Fatal(err)
	}
	f.db.Where("job_id = ?", jobID).First(&job)
	if job.Status != graphjob.JobSucceed {
		t.Fatal("finished job must not change")
	}
}

func TestSyncJob_FailedWithPartyReasons(t *testing.T) {
	f := newGraphFixture(t)
	f.seedPSIProject()
	jobID := startPSI(t, f)
	taskID := jobID + "-g1-node-3"
	err := f.sync.SyncJob(context.Background(), jobID, &kuscia.JobStatusDetailInfo{State: "Failed", ErrMsg: "job failed", EndTime: "2026-09-24T10:00:00Z",
		Tasks: []kuscia.JobTaskStatusDetail{{TaskID: taskID, State: "Failed", ErrMsg: "task boom",
			Parties: []kuscia.JobPartyStatusDetail{{DomainID: "bob", State: "Failed", ErrMsg: "oom"}, {DomainID: "alice", State: "Succeeded"}}}}})
	if err != nil {
		t.Fatal(err)
	}
	var job model.ProjectJobDO
	f.db.Where("job_id = ?", jobID).First(&job)
	if job.Status != graphjob.JobFailed || job.ErrMsg != "job failed" {
		t.Fatalf("job %+v", job)
	}
	var task model.ProjectJobTaskDO
	f.db.Where("task_id = ?", taskID).First(&task)
	if task.Status != graphjob.TaskFailed || task.ErrMsg != "task boom" {
		t.Fatalf("task %+v", task)
	}
	logs := strings.Join(taskLogs(f, jobID, taskID), "\n")
	if !strings.Contains(logs, "party bob failed msg: oom") || !strings.Contains(logs, "failed: task boom") {
		t.Fatalf("failed logs %s", logs)
	}
}

func TestSyncJob_ReportResultAndOutput(t *testing.T) {
	f := newGraphFixture(t)
	f.seedPSIProject()
	vo, err := f.graph.StartGraph(adminCtx(), &StartGraphRequest{ProjectID: "p1", GraphID: "g1"})
	if err != nil {
		t.Fatal(err)
	}
	jobID := vo.JobID
	report := jobID + "-g1-node-4-output-0"
	f.k.addDomainData("alice", report, "report", nil, map[string]string{"dist_data": `{"type":"sf.report","meta":{"tabs":[{"name":"t1"}]}}`})
	if err := f.sync.SyncJob(context.Background(), jobID, &kuscia.JobStatusDetailInfo{State: "Running", Tasks: []kuscia.JobTaskStatusDetail{
		{TaskID: jobID + "-g1-node-4", State: "Succeeded"},
	}}); err != nil {
		t.Fatal(err)
	}
	var rep model.ProjectReportDO
	if err := f.db.Where("report_id = ?", report).First(&rep).Error; err != nil || !strings.Contains(rep.Content, "tabs") {
		t.Fatalf("report %+v %v", rep, err)
	}
	out, err := f.graph.GetNodeOutput(adminCtx(), &GraphNodeOutputRequest{ProjectID: "p1", GraphID: "g1", GraphNodeID: "g1-node-4", OutputID: "g1-node-4-output-0"})
	if err != nil {
		t.Fatal(err)
	}
	if out.Type != "report" || out.JobID != jobID || out.GraphID != "g1" || out.CodeName != "stats/table_statistics" || out.Tabs == nil {
		t.Fatalf("report output %+v", out)
	}
	// task output endpoint reuses the same renderer
	tout, err := f.job.GetTaskOutput(adminCtx(), &GetTaskOutputRequest{ProjectID: "p1", JobID: jobID, TaskID: jobID + "-g1-node-4", OutputID: "g1-node-4-output-0"})
	if err != nil || tout.Type != "report" {
		t.Fatalf("task output %+v %v", tout, err)
	}
}

func TestMergeExtendedTasks(t *testing.T) {
	m := mergeExtendedTasks([]kuscia.JobTaskStatusDetail{
		{Alias: "t1", State: "Pending"},
		{Alias: "t1--1", State: "Succeeded"},
		{Alias: "t2", State: "Pending"},
		{Alias: "t2--1", State: "Failed", ErrMsg: "write failed"},
	})
	if m["t1"].State != "Running" {
		t.Fatalf("t1 %+v", m["t1"])
	}
	if m["t2"].State != "Failed" || m["t2"].ErrMsg != "write failed" {
		t.Fatalf("t2 %+v", m["t2"])
	}
	if _, ok := m["t1--1"]; ok {
		t.Fatal("extended task must be folded")
	}
}

func TestParseIndividualSchema(t *testing.T) {
	cols := parseIndividualSchema(`{"type":"sf.table.individual","meta":{"@type":"x","schema":{"ids":["id"],"id_types":["str"],"features":["f1"],"featureTypes":["float"],"labels":["y"],"label_types":["int"]}},"data_refs":[{"party":"alice"}]}`)
	if len(cols) != 3 || cols[0].ColName != "id" || cols[1].ColType != "float" || cols[2].ColComment != "individual:alice" {
		t.Fatalf("cols %+v", cols)
	}
	if len(parseIndividualSchema(`{"type":"sf.table.vertical"}`)) != 0 {
		t.Fatal("non individual")
	}
}

type testSyncHandler struct{ handled []string }

func (h *testSyncHandler) PendingJobIDs(context.Context) []string { return []string{"pack1"} }
func (h *testSyncHandler) HandleJob(_ context.Context, id string, _ *kuscia.JobStatusDetailInfo) bool {
	if id == "pack1" {
		h.handled = append(h.handled, id)
		return true
	}
	return false
}

func TestJobSyncHandlerAndPoll(t *testing.T) {
	f := newGraphFixture(t)
	f.seedPSIProject()
	jobID := startPSI(t, f)
	h := &testSyncHandler{}
	f.sync.RegisterHandler(h)
	f.k.jobs["pack1"] = &kuscia.JobStatusDetailInfo{State: "Succeeded"}
	f.k.jobs[jobID] = &kuscia.JobStatusDetailInfo{State: "Running", Tasks: []kuscia.JobTaskStatusDetail{{TaskID: jobID + "-g1-node-3", State: "Running"}}}
	f.sync.syncOnce()
	if len(h.handled) != 1 {
		t.Fatalf("handler not called: %v", h.handled)
	}
	var task model.ProjectJobTaskDO
	f.db.Where("task_id = ?", jobID+"-g1-node-3").First(&task)
	if task.Status != graphjob.TaskRunning {
		t.Fatalf("poll did not sync task: %+v", task)
	}
}
