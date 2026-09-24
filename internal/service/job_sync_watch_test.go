package service

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/internal/graphjob"
	"github.com/fengzhizi319/privahub/pkg/kuscia"
)

// notFoundKuscia answers QueryJobDetail with the KusciaAPI "job not found"
// error for the configured job ids.
type notFoundKuscia struct {
	*graphFakeKuscia
	gone map[string]bool
}

func (k *notFoundKuscia) QueryJobDetail(ctx context.Context, jobID string) (*kuscia.QueryJobDetailResponse, error) {
	if k.gone[jobID] {
		return nil, &kuscia.APIError{Path: "/api/v1/job/query", HTTPStatus: 200, Code: kuscia.ErrCodeQueryJob,
			Message: `kusciajobs.kuscia.secretflow "` + jobID + `" not found`}
	}
	return k.graphFakeKuscia.QueryJobDetail(ctx, jobID)
}

func jobStatus(f *graphFixture, jobID string) string {
	var j model.ProjectJobDO
	f.db.Where("job_id = ?", jobID).First(&j)
	return j.Status
}

func taskStatuses(f *graphFixture, jobID string) map[string]string {
	var ts []model.ProjectJobTaskDO
	f.db.Where("job_id = ?", jobID).Find(&ts)
	out := map[string]string{}
	for _, t := range ts {
		out[t.TaskID] = t.Status
	}
	return out
}

func TestJobSyncWatch_ModifiedThenDeletedStopsJob(t *testing.T) {
	f := newGraphFixture(t)
	f.seedPSIProject()
	jobID := startPSI(t, f)
	taskID := jobID + "-g1-node-3"
	ctx := context.Background()

	f.sync.HandleWatchEvent(ctx, &kuscia.WatchJobEvent{Type: kuscia.WatchEventModified, Object: &kuscia.WatchJobObject{
		JobID: jobID, Status: &kuscia.JobStatusDetailInfo{State: "Running", Tasks: []kuscia.JobTaskStatusDetail{{TaskID: taskID, State: "Running"}}},
	}})
	if st := taskStatuses(f, jobID)[taskID]; st != graphjob.TaskRunning {
		t.Fatalf("task after MODIFIED = %s", st)
	}
	before := len(taskLogs(f, jobID, taskID))

	f.sync.HandleWatchEvent(ctx, &kuscia.WatchJobEvent{Type: kuscia.WatchEventDeleted, Object: &kuscia.WatchJobObject{JobID: jobID}})
	if st := jobStatus(f, jobID); st != graphjob.JobStopped {
		t.Fatalf("job after DELETED = %s", st)
	}
	for id, st := range taskStatuses(f, jobID) {
		if st == graphjob.TaskRunning || st == graphjob.TaskInitialized {
			t.Fatalf("task %s still %s", id, st)
		}
	}
	logs := taskLogs(f, jobID, taskID)
	if len(logs) <= before || !strings.Contains(logs[len(logs)-1], "stop") {
		t.Fatalf("stop logs %v", logs)
	}

	// finished jobs ignore further DELETED / ERROR events
	f.db.Model(&model.ProjectJobDO{}).Where("job_id = ?", jobID).Update("status", graphjob.JobSucceed)
	f.sync.HandleWatchEvent(ctx, &kuscia.WatchJobEvent{Type: kuscia.WatchEventDeleted, Object: &kuscia.WatchJobObject{JobID: jobID}})
	f.sync.HandleWatchEvent(ctx, &kuscia.WatchJobEvent{Type: kuscia.WatchEventError, Object: &kuscia.WatchJobObject{JobID: jobID}})
	if st := jobStatus(f, jobID); st != graphjob.JobSucceed {
		t.Fatalf("finished job changed to %s", st)
	}
}

func TestJobSyncPoll_VanishedJobTreatedAsDeleted(t *testing.T) {
	f := newGraphFixture(t)
	f.seedPSIProject()
	jobID := startPSI(t, f)
	k := &notFoundKuscia{graphFakeKuscia: f.k, gone: map[string]bool{jobID: true}}
	f.sync.SetKuscia(k)

	// young job: never marked (may not be submitted yet)
	f.sync.notFoundWait = time.Hour
	for i := 0; i < notFoundThreshold+1; i++ {
		f.sync.syncOnce()
	}
	if st := jobStatus(f, jobID); st != graphjob.JobRunning {
		t.Fatalf("young vanished job = %s", st)
	}

	f.sync.notFoundWait = 0
	f.sync.clearNotFound(jobID)
	for i := 0; i < notFoundThreshold-1; i++ {
		f.sync.syncOnce()
	}
	if st := jobStatus(f, jobID); st != graphjob.JobRunning {
		t.Fatalf("job marked before threshold: %s", st)
	}
	f.sync.syncOnce()
	if st := jobStatus(f, jobID); st != graphjob.JobStopped {
		t.Fatalf("vanished job = %s", st)
	}
}

func TestJobSyncPoll_TransientNotFoundResets(t *testing.T) {
	f := newGraphFixture(t)
	f.seedPSIProject()
	jobID := startPSI(t, f)
	k := &notFoundKuscia{graphFakeKuscia: f.k, gone: map[string]bool{jobID: true}}
	f.sync.SetKuscia(k)
	f.sync.notFoundWait = 0
	for i := 0; i < notFoundThreshold-1; i++ {
		f.sync.syncOnce()
	}
	// the job shows up again: counter resets
	k.gone[jobID] = false
	f.k.mu.Lock()
	f.k.jobs[jobID] = &kuscia.JobStatusDetailInfo{State: "Running"}
	f.k.mu.Unlock()
	f.sync.syncOnce()
	k.gone[jobID] = true
	for i := 0; i < notFoundThreshold-1; i++ {
		f.sync.syncOnce()
	}
	if st := jobStatus(f, jobID); st != graphjob.JobRunning {
		t.Fatalf("transient not-found stopped the job: %s", st)
	}
}

func TestJobSync_ScheduleJobDeleted(t *testing.T) {
	f := newSchedFixture(t)
	if err := f.svc.CreateScheduler(f.ctx, dailyCreate("del-1")); err != nil {
		t.Fatal(err)
	}
	sync := NewJobStatusSyncService(f.db, nil, nil)
	tk := f.tasks(t)[0]
	jobID := tk.ScheduleJobID

	// pre-rendered, not fired: untouched
	if err := sync.MarkJobDeleted(context.Background(), jobID); err != nil {
		t.Fatal(err)
	}
	var sj model.ProjectScheduleJobDO
	f.db.Where("job_id = ?", jobID).First(&sj)
	if sj.Status != model.ScheduleJobStatusRunning {
		t.Fatalf("unfired schedule job = %s", sj.Status)
	}

	f.db.Model(&model.ProjectScheduleTaskDO{}).Where("id = ?", tk.ID).Update("status", model.ScheduledStatusRunning)
	if err := sync.MarkJobDeleted(context.Background(), jobID); err != nil {
		t.Fatal(err)
	}
	f.db.Where("job_id = ?", jobID).First(&sj)
	if sj.Status != model.ScheduleJobStatusStopped {
		t.Fatalf("schedule job = %s", sj.Status)
	}
	var task model.ProjectScheduleTaskDO
	f.db.Where("id = ?", tk.ID).First(&task)
	if task.Status != model.ScheduledStatusStopped || task.ScheduleTaskEndTime == nil {
		t.Fatalf("schedule task %+v", task)
	}
	var jts []model.ProjectJobTaskDO
	f.db.Where("job_id = ?", jobID).Find(&jts)
	for _, jt := range jts {
		if jt.Status != graphjob.TaskStopped {
			t.Fatalf("job task %s = %s", jt.TaskID, jt.Status)
		}
	}
	// other (unfired) schedule tasks keep TO_BE_RUN
	for _, other := range f.tasks(t)[1:] {
		if other.Status != model.ScheduledStatusToBeRun {
			t.Fatalf("other task %s = %s", other.ScheduleTaskID, other.Status)
		}
	}
}

// fakeWatcher replays events then blocks until the context ends.
type fakeWatcher struct {
	mu     sync.Mutex
	calls  int
	err    error
	events []*kuscia.WatchJobEvent
}

func (w *fakeWatcher) WatchJobs(ctx context.Context, _ int64, fn func(*kuscia.WatchJobEvent) error) error {
	w.mu.Lock()
	w.calls++
	err, events := w.err, w.events
	w.mu.Unlock()
	if err != nil {
		return err
	}
	for _, ev := range events {
		if err := fn(ev); err != nil {
			return err
		}
	}
	<-ctx.Done()
	return nil
}

func TestJobSyncWatch_LoopAppliesEventsAndStops(t *testing.T) {
	f := newGraphFixture(t)
	f.seedPSIProject()
	jobID := startPSI(t, f)
	w := &fakeWatcher{events: []*kuscia.WatchJobEvent{
		{Type: kuscia.WatchEventModified, Object: &kuscia.WatchJobObject{JobID: jobID, Status: &kuscia.JobStatusDetailInfo{
			State: "Failed", ErrMsg: "boom", EndTime: "2026-09-24T10:00:00Z"}}},
	}}
	f.sync.SetWatcher(w)
	done := make(chan struct{})
	go func() { f.sync.watchLoop(); close(done) }()
	deadline := time.Now().Add(5 * time.Second)
	for jobStatus(f, jobID) != graphjob.JobFailed {
		if time.Now().After(deadline) {
			t.Fatalf("watch event not applied, job = %s", jobStatus(f, jobID))
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !f.sync.WatchConnected() {
		t.Fatal("watch should be connected")
	}
	if !f.sync.skipPollTick() {
		t.Fatal("poller should be thinned out while watch is connected")
	}
	f.sync.Stop()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("watch loop did not stop")
	}
	if f.sync.WatchConnected() {
		t.Fatal("watch should be disconnected after stop")
	}
}

func TestJobSyncWatch_UnsupportedFallsBackToPolling(t *testing.T) {
	f := newGraphFixture(t)
	w := &fakeWatcher{err: &kuscia.APIError{Path: kuscia.WatchJobPath, HTTPStatus: http.StatusNotFound}}
	f.sync.SetWatcher(w)
	done := make(chan struct{})
	go func() { f.sync.watchLoop(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("watch loop should give up on 404")
	}
	if w.calls != 1 || !f.sync.watchState.unsupported || f.sync.skipPollTick() {
		t.Fatalf("calls=%d unsupported=%v", w.calls, f.sync.watchState.unsupported)
	}
	f.sync.Stop()
}

// --- item 6a: project_schedule_job fallback ---

func TestJobService_ScheduleJobFallback(t *testing.T) {
	f := newGraphFixture(t)
	f.seedPSIProject()
	ctx := context.Background()
	f.mustCreate(&model.ProjectScheduleJobDO{ProjectID: "p1", JobID: "sj1", Name: "sched", Status: "Succeeded", GraphID: "g1", Edges: "[]"})
	f.mustCreate(&model.ProjectJobTaskDO{ProjectID: "p1", JobID: "sj1", TaskID: "sj1-g1-node-3", GraphNodeID: "g1-node-3", Status: "SUCCEEDED", Parties: `["alice"]`})
	f.mustCreate(&model.ProjectJobTaskLogDO{ProjectID: "p1", JobID: "sj1", TaskID: "sj1-g1-node-3", Log: "hello"})

	vo, err := f.job.GetJob(ctx, &GetJobRequest{ProjectID: "p1", JobID: "sj1"})
	if err != nil {
		t.Fatal(err)
	}
	if vo.Status != graphjob.JobSucceed || !vo.Finished || vo.Graph.GraphID != "g1" || len(vo.Graph.Nodes) != 1 || vo.Graph.Nodes[0].Status != graphjob.TaskSucceed {
		t.Fatalf("schedule job vo %+v", vo)
	}
	logs, err := f.job.GetTaskLogs(ctx, &GetTaskLogRequest{ProjectID: "p1", JobID: "sj1", TaskID: "sj1-g1-node-3"})
	if err != nil || len(logs.Logs) != 1 || logs.Status != graphjob.TaskSucceed {
		t.Fatalf("logs %+v %v", logs, err)
	}
	if gid, ok := jobGraphID(ctx, f.db, "p1", "sj1"); !ok || gid != "g1" {
		t.Fatalf("jobGraphID = %q %v", gid, ok)
	}
	if _, err := f.job.GetJob(ctx, &GetJobRequest{ProjectID: "p1", JobID: "nope"}); err == nil {
		t.Fatal("missing job must fail")
	}
}

// --- item 7a: GraphService extension fields ---

func TestGraphService_ExtensionFieldsPerInstance(t *testing.T) {
	a, b := &GraphService{}, &GraphService{}
	if got := a.teeConfig(); got.DomainID != "tee" || got.CertDir != "./config/certs" {
		t.Fatalf("zero-value defaults %+v", got)
	}
	a.SetTeeConfig(TeeGraphConfig{DomainID: "tee-a"})
	if a.teeConfig().DomainID != "tee-a" || b.teeConfig().DomainID != "tee" {
		t.Fatalf("configs leaked between instances: %+v %+v", a.teeConfig(), b.teeConfig())
	}
	if s, ok := b.teeSigner().(*FileTeeTaskSigner); !ok || s.CertDir != "./config/certs" {
		t.Fatalf("default signer %+v", b.teeSigner())
	}
	fk := newGraphFakeKuscia()
	b.SetKuscia(fk)
	if b.kusciaFor("alice") != fk {
		t.Fatal("no pool: default kuscia expected")
	}
	pool := kuscia.NewClientPool(nil, "")
	if b.WithKusciaPool(pool) != b || b.kusciaFor("alice") != fk {
		t.Fatal("unregistered channel must use default kuscia")
	}
}

// --- item 6b: schedule responses use Java enum names ---

func TestScheduled_StatusesNormalized(t *testing.T) {
	f := newSchedFixture(t)
	if err := f.svc.CreateScheduler(f.ctx, dailyCreate("norm-1")); err != nil {
		t.Fatal(err)
	}
	// legacy / Kuscia spellings in the snapshot and the rows
	var sc model.ProjectScheduleDO
	f.db.Where("schedule_id = ?", "norm-1").First(&sc)
	f.db.Model(&model.ProjectScheduleDO{}).Where("id = ?", sc.ID).
		Update("job_info", strings.ReplaceAll(sc.JobInfo, `"SUCCEED"`, `"Succeeded"`))
	tk := f.tasks(t)[0]
	f.db.Model(&model.ProjectScheduleJobDO{}).Where("job_id = ?", tk.ScheduleJobID).Update("status", "SUCCEEDED")
	f.db.Model(&model.ProjectJobTaskDO{}).Where("task_id = ?", tk.ScheduleJobID+"-n1").Update("status", "Succeeded")
	f.db.Model(&model.ProjectJobTaskDO{}).Where("task_id = ?", tk.ScheduleJobID+"-n2").Update("status", "PENDING")

	info, err := f.svc.Info(f.ctx, &ScheduledInfoRequest{ScheduleID: "norm-1"})
	if err != nil {
		t.Fatal(err)
	}
	if info.Status != graphjob.JobSucceed || !info.Finished {
		t.Fatalf("info status %q finished=%v", info.Status, info.Finished)
	}
	for _, n := range info.Graph.Nodes {
		if n.Status != "" && n.Status != graphjob.TaskSucceed && n.Status != graphjob.TaskInitialized {
			t.Fatalf("info node status %q", n.Status)
		}
	}

	tvo, err := f.svc.TaskInfo(f.ctx, &TaskInfoScheduledRequest{ScheduleID: "norm-1", ScheduleTaskID: tk.ScheduleTaskID})
	if err != nil {
		t.Fatal(err)
	}
	if tvo.Status != graphjob.JobSucceed || !tvo.Finished {
		t.Fatalf("task info status %q finished=%v", tvo.Status, tvo.Finished)
	}
	want := map[string]string{"n1": graphjob.TaskSucceed, "n2": graphjob.TaskInitialized}
	for _, n := range tvo.Graph.Nodes {
		if n.Status != want[n.GraphNodeID] {
			t.Fatalf("task info node %s status %q", n.GraphNodeID, n.Status)
		}
	}

	page, err := f.svc.ListProjectJob(f.ctx, &ScheduleListProjectJobRequest{ProjectID: "p1", GraphID: "g1", ScheduleTaskID: tk.ScheduleTaskID})
	if err != nil || len(page.Data) != 1 {
		t.Fatalf("list %v %+v", err, page)
	}
	if page.Data[0].Status != graphjob.JobSucceed {
		t.Fatalf("list status %q", page.Data[0].Status)
	}
}
