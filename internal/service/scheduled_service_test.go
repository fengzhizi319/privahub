package service

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/pkg/auth"
	"github.com/fengzhizi319/privahub/pkg/errcode"
	"github.com/fengzhizi319/privahub/pkg/kuscia"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// ---------------------------------------------------------------------------
// fakes
// ---------------------------------------------------------------------------

type fakeScheduleRenderer struct {
	mu    sync.Mutex
	n     int
	dates []string
	err   error
	nilRq bool
}

func (f *fakeScheduleRenderer) RenderScheduledJob(_ context.Context, req *ScheduledRenderRequest) (*ScheduledRenderResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return nil, f.err
	}
	f.n++
	f.dates = append(f.dates, req.ScheduleExpectStartDate)
	jobID := fmt.Sprintf("sjob%d", f.n)
	res := &ScheduledRenderResult{
		Job: &model.ProjectJobDO{ProjectID: req.ProjectID, JobID: jobID, Name: "g1_run", Status: "RUNNING", GraphID: req.GraphID, Edges: `[{"source":"n1","target":"n2"}]`},
		Tasks: []model.ProjectJobTaskDO{
			{ProjectID: req.ProjectID, JobID: jobID, TaskID: jobID + "-n1", GraphNodeID: "n1", Status: "INITIALIZED", Parties: `["alice","bob"]`},
			{ProjectID: req.ProjectID, JobID: jobID, TaskID: jobID + "-n2", GraphNodeID: "n2", Status: "INITIALIZED", Parties: `["alice","bob"]`},
		},
		Logs: []model.ProjectJobTaskLogDO{{ProjectID: req.ProjectID, JobID: jobID, TaskID: jobID + "-n1", Log: "succeed"}},
	}
	if !f.nilRq {
		res.Request = &kuscia.CreateJobRequest{
			JobID:     jobID,
			Initiator: "alice",
			Tasks: []kuscia.TaskConfig{{
				TaskID:      jobID + "-n2",
				Alias:       "n2",
				InputConfig: `{"output_uris":["` + jobID + `_n2_output_0_` + req.ScheduleExpectStartDate + `"]}`,
			}},
		}
	}
	return res, nil
}

type fakeScheduleKuscia struct {
	mu        sync.Mutex
	created   []string
	stopped   []string
	deleted   []string
	restarted []string
	createErr error
	states    map[string]*kuscia.JobExecutionStatus
}

func (f *fakeScheduleKuscia) CreateJob(_ context.Context, req *kuscia.CreateJobRequest) (*kuscia.CreateJobResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.createErr != nil {
		return nil, f.createErr
	}
	f.created = append(f.created, req.JobID)
	return &kuscia.CreateJobResponse{}, nil
}

func (f *fakeScheduleKuscia) QueryJob(_ context.Context, jobID string) (*kuscia.QueryJobResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	st, ok := f.states[jobID]
	if !ok {
		return nil, errors.New("not found")
	}
	resp := &kuscia.QueryJobResponse{}
	resp.Data.JobID = jobID
	resp.Data.Status = st
	return resp, nil
}

func (f *fakeScheduleKuscia) StopJob(_ context.Context, jobID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stopped = append(f.stopped, jobID)
	return nil
}

func (f *fakeScheduleKuscia) DeleteJob(_ context.Context, jobID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deleted = append(f.deleted, jobID)
	return nil
}

func (f *fakeScheduleKuscia) RestartJob(_ context.Context, jobID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.restarted = append(f.restarted, jobID)
	return nil
}

type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fakeClock) Now() time.Time  { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *fakeClock) Set(t time.Time) { c.mu.Lock(); c.t = t; c.mu.Unlock() }

// ---------------------------------------------------------------------------
// fixture
// ---------------------------------------------------------------------------

var schedLoc = time.FixedZone("CST", 8*3600)

type schedFixture struct {
	db       *gorm.DB
	svc      *ScheduledService
	renderer *fakeScheduleRenderer
	kuscia   *fakeScheduleKuscia
	clock    *fakeClock
	ctx      context.Context
}

func schedTime(s string) time.Time {
	t, err := time.ParseInLocation("2006-01-02 15:04:05", s, schedLoc)
	if err != nil {
		panic(err)
	}
	return t
}

func openScheduledTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(
		&model.ProjectDO{}, &model.ProjectGraphDO{}, &model.ProjectGraphNodeDO{},
		&model.ProjectJobDO{}, &model.ProjectJobTaskDO{}, &model.ProjectResultDO{}, &model.ProjectJobTaskLogDO{},
		&model.NodeDO{}, &model.InstDO{},
		&model.ProjectScheduleDO{}, &model.ProjectScheduleJobDO{}, &model.ProjectScheduleTaskDO{},
	); err != nil {
		t.Fatal(err)
	}
	return db
}

func newSchedFixture(t *testing.T) *schedFixture {
	t.Helper()
	db := openScheduledTestDB(t)
	must := func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	}
	must(db.Create(&model.ProjectDO{ProjectID: "p1", Name: "p1"}).Error)
	must(db.Create(&model.NodeDO{NodeID: "alice", Name: "Alice Node", ControlNodeID: "alice"}).Error)
	must(db.Create(&model.ProjectGraphDO{ProjectID: "p1", GraphID: "g1", Name: "graph one", Edges: `[{"source":"n1","target":"n2"}]`}).Error)
	must(db.Create(&model.ProjectGraphNodeDO{ProjectID: "p1", GraphID: "g1", GraphNodeID: "n1", CodeName: "read_data/datatable", Outputs: `["n1-output-0"]`, NodeDef: `{"a":1}`}).Error)
	must(db.Create(&model.ProjectGraphNodeDO{ProjectID: "p1", GraphID: "g1", GraphNodeID: "n2", CodeName: "data_prep/psi", Inputs: `["n1-output-0"]`, Outputs: `["n2-output-0"]`}).Error)
	must(db.Create(&model.ProjectJobDO{ProjectID: "p1", JobID: "job0", Name: "g1_run", Status: "SUCCEED", GraphID: "g1"}).Error)
	must(db.Create(&model.ProjectJobTaskDO{ProjectID: "p1", JobID: "job0", TaskID: "job0-n2", GraphNodeID: "n2", Status: "SUCCEED", Parties: `["alice"]`}).Error)

	clock := &fakeClock{t: schedTime("2024-01-01 00:00:00")}
	svc := NewScheduledService(db, nil, nil, nil, ScheduledOptions{Location: schedLoc, Now: clock.Now, MisfireGrace: 10 * time.Minute})
	r := &fakeScheduleRenderer{}
	k := &fakeScheduleKuscia{states: map[string]*kuscia.JobExecutionStatus{}}
	svc.SetGraphRenderer(r)
	svc.SetKusciaClient(k)
	ctx := auth.ContextWithClaims(context.Background(), &auth.Claims{OwnerID: "alice", Username: "admin"})
	return &schedFixture{db: db, svc: svc, renderer: r, kuscia: k, clock: clock, ctx: ctx}
}

func dailyCreate(id string) *ScheduledGraphCreateRequest {
	req := &ScheduledGraphCreateRequest{
		ScheduleID:   id,
		ScheduleDesc: "daily psi",
		ProjectID:    "p1",
		GraphID:      "g1",
		Nodes:        []string{"n1", "n2"},
	}
	req.Cron.StartTime = "2024-01-01 00:00:00"
	req.Cron.EndTime = "2024-01-03 23:59:59"
	req.Cron.ScheduleCycle = "D"
	req.Cron.ScheduleTime = "10:00"
	return req
}

func expectCode(t *testing.T, err error, code *errcode.ErrorCode) {
	t.Helper()
	var se *ScheduledError
	if !errors.As(err, &se) || se.Code.Code != code.Code {
		t.Fatalf("expected error code %d, got %v", code.Code, err)
	}
}

func (f *schedFixture) tasks(t *testing.T) []model.ProjectScheduleTaskDO {
	t.Helper()
	var ts []model.ProjectScheduleTaskDO
	if err := f.db.Order("schedule_task_expect_start ASC").Find(&ts).Error; err != nil {
		t.Fatal(err)
	}
	return ts
}

func (f *schedFixture) task(t *testing.T, id string) model.ProjectScheduleTaskDO {
	t.Helper()
	var tk model.ProjectScheduleTaskDO
	if err := f.db.Where("schedule_task_id = ?", id).First(&tk).Error; err != nil {
		t.Fatal(err)
	}
	return tk
}

func (f *schedFixture) job(t *testing.T, id string) model.ProjectScheduleJobDO {
	t.Helper()
	var j model.ProjectScheduleJobDO
	if err := f.db.Where("job_id = ?", id).First(&j).Error; err != nil {
		t.Fatal(err)
	}
	return j
}

// ---------------------------------------------------------------------------
// id / once success / create
// ---------------------------------------------------------------------------

func TestScheduled_BuildSchedulerID(t *testing.T) {
	f := newSchedFixture(t)
	id, err := f.svc.BuildSchedulerID(f.ctx, &ScheduledIdRequest{ProjectID: "p1", GraphID: "g1"})
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`^[a-z]{4}-20240101000000$`).MatchString(id) {
		t.Errorf("unexpected id %q", id)
	}
	id2, _ := f.svc.BuildSchedulerID(f.ctx, &ScheduledIdRequest{ProjectID: "p1", GraphID: "g1"})
	if len(id2) > 20 {
		t.Errorf("id too long: %q", id2)
	}
	_, err = f.svc.BuildSchedulerID(f.ctx, &ScheduledIdRequest{ProjectID: "nope", GraphID: "g1"})
	expectCode(t, err, errcode.ProjectNotFound)
	_, err = f.svc.BuildSchedulerID(f.ctx, &ScheduledIdRequest{ProjectID: "p1", GraphID: "nope"})
	expectCode(t, err, errcode.GraphNotFound)
}

func TestScheduled_OnceSuccess(t *testing.T) {
	f := newSchedFixture(t)
	ok, err := f.svc.OnceSuccess(f.ctx, &ScheduledGraphOnceSuccessRequest{ProjectID: "p1", GraphID: "g1"})
	if err != nil || !ok {
		t.Fatalf("expected true, got %v %v", ok, err)
	}
	// Graph with jobs but none succeeded.
	f.db.Create(&model.ProjectGraphDO{ProjectID: "p1", GraphID: "g2", Name: "g2"})
	f.db.Create(&model.ProjectJobDO{ProjectID: "p1", JobID: "job-f", Name: "x", Status: "FAILED", GraphID: "g2"})
	ok, err = f.svc.OnceSuccess(f.ctx, &ScheduledGraphOnceSuccessRequest{ProjectID: "p1", GraphID: "g2"})
	if err != nil || ok {
		t.Fatalf("expected false, got %v %v", ok, err)
	}
	// Graph without any job.
	f.db.Create(&model.ProjectGraphDO{ProjectID: "p1", GraphID: "g3", Name: "g3"})
	ok, err = f.svc.OnceSuccess(f.ctx, &ScheduledGraphOnceSuccessRequest{ProjectID: "p1", GraphID: "g3"})
	if err != nil || ok {
		t.Fatalf("expected false, got %v %v", ok, err)
	}
	// Legacy "SUCCEEDED" status also counts.
	f.db.Create(&model.ProjectJobDO{ProjectID: "p1", JobID: "job-s", Name: "x", Status: "SUCCEEDED", GraphID: "g3"})
	ok, _ = f.svc.OnceSuccess(f.ctx, &ScheduledGraphOnceSuccessRequest{ProjectID: "p1", GraphID: "g3"})
	if !ok {
		t.Fatal("expected SUCCEEDED to count as success")
	}
	_, err = f.svc.OnceSuccess(f.ctx, &ScheduledGraphOnceSuccessRequest{ProjectID: "p1", GraphID: "missing"})
	expectCode(t, err, errcode.GraphNotFound)
}

func TestScheduled_CreateScheduler(t *testing.T) {
	f := newSchedFixture(t)
	if err := f.svc.CreateScheduler(f.ctx, dailyCreate("abcd-1")); err != nil {
		t.Fatal(err)
	}
	var sc model.ProjectScheduleDO
	if err := f.db.Where("schedule_id = ?", "abcd-1").First(&sc).Error; err != nil {
		t.Fatal(err)
	}
	if sc.Status != model.ScheduledStatusUp || sc.Owner != "alice" || sc.Creator != "admin" || sc.ScheduleDesc != "daily psi" || sc.GraphJobID != "job0" {
		t.Errorf("unexpected schedule: %+v", sc)
	}
	if sc.Cron != `["0 0 10 * * ?"]` || !strings.Contains(sc.GraphInfo, `"n2"`) || !strings.Contains(sc.JobInfo, "job0") || !strings.Contains(sc.Request, `"scheduleCycle":"D"`) {
		t.Errorf("unexpected snapshots: cron=%s graph=%s job=%s req=%s", sc.Cron, sc.GraphInfo, sc.JobInfo, sc.Request)
	}

	ts := f.tasks(t)
	if len(ts) != 3 {
		t.Fatalf("expected 3 tasks, got %d", len(ts))
	}
	wantDates := []string{"20240101", "20240102", "20240103"}
	for i, tk := range ts {
		if tk.Status != model.ScheduledStatusToBeRun || tk.Owner != "alice" || tk.Creator != "admin" || tk.Cron != "0 0 10 * * ?" || tk.AllReRun {
			t.Errorf("task %d unexpected: %+v", i, tk)
		}
		if got := tk.ScheduleTaskExpectStart.In(schedLoc).Format("20060102 15:04"); got != wantDates[i]+" 10:00" {
			t.Errorf("task %d expect start %s", i, got)
		}
		req, err := decodeJobRequest(tk.JobRequest)
		if err != nil || req.JobID != tk.ScheduleJobID || !strings.Contains(req.Tasks[0].InputConfig, "_"+wantDates[i]) {
			t.Errorf("task %d stored request %+v err %v", i, req, err)
		}
		sj := f.job(t, tk.ScheduleJobID)
		if sj.ScheduleTaskID != tk.ScheduleTaskID || sj.Status != model.ScheduleJobStatusRunning || sj.Owner != "alice" {
			t.Errorf("schedule job %+v", sj)
		}
		var cnt int64
		f.db.Model(&model.ProjectJobTaskDO{}).Where("job_id = ?", tk.ScheduleJobID).Count(&cnt)
		if cnt != 2 {
			t.Errorf("expected 2 job tasks for %s, got %d", tk.ScheduleJobID, cnt)
		}
	}
	var logCnt int64
	f.db.Model(&model.ProjectJobTaskLogDO{}).Count(&logCnt)
	if logCnt != 3 {
		t.Errorf("expected 3 render logs persisted, got %d", logCnt)
	}
	if strings.Join(f.renderer.dates, ",") != strings.Join(wantDates, ",") {
		t.Errorf("renderer dates %v", f.renderer.dates)
	}
	// Nothing submitted at creation time.
	if len(f.kuscia.created) != 0 {
		t.Errorf("unexpected kuscia submissions %v", f.kuscia.created)
	}
	// Duplicate schedule id.
	expectCode(t, f.svc.CreateScheduler(f.ctx, dailyCreate("abcd-1")), errcode.ScheduledScheduleCreateError)
}

func TestScheduled_CreateScheduler_WeeklyMonthly(t *testing.T) {
	f := newSchedFixture(t)
	w := dailyCreate("week-1")
	w.Cron.ScheduleCycle, w.Cron.ScheduleDate = "W", "1,7"
	w.Cron.EndTime = "2024-01-14 23:00:00"
	if err := f.svc.CreateScheduler(f.ctx, w); err != nil {
		t.Fatal(err)
	}
	m := dailyCreate("month-1")
	m.Cron.ScheduleCycle, m.Cron.ScheduleDate = "M", "15,end"
	m.Cron.EndTime = "2024-02-29 23:00:00"
	if err := f.svc.CreateScheduler(f.ctx, m); err != nil {
		t.Fatal(err)
	}
	var got []string
	var ts []model.ProjectScheduleTaskDO
	f.db.Order("schedule_id, schedule_task_expect_start").Find(&ts)
	for _, tk := range ts {
		got = append(got, tk.ScheduleID+"@"+tk.ScheduleTaskExpectStart.In(schedLoc).Format("01-02"))
	}
	want := "month-1@01-15,month-1@01-31,month-1@02-15,month-1@02-29,week-1@01-01,week-1@01-07,week-1@01-08,week-1@01-14"
	if strings.Join(got, ",") != want {
		t.Errorf("got %v", got)
	}
}

func TestScheduled_CreateScheduler_Validation(t *testing.T) {
	f := newSchedFixture(t)
	cases := []struct {
		name string
		mut  func(r *ScheduledGraphCreateRequest)
		code *errcode.ErrorCode
	}{
		{"no nodes", func(r *ScheduledGraphCreateRequest) { r.Nodes = nil }, errcode.ParamError},
		{"missing cron fields", func(r *ScheduledGraphCreateRequest) { r.Cron.ScheduleCycle = "" }, errcode.ParamError},
		{"desc too long", func(r *ScheduledGraphCreateRequest) { r.ScheduleDesc = strings.Repeat("x", 201) }, errcode.ParamError},
		{"id too long", func(r *ScheduledGraphCreateRequest) { r.ScheduleID = strings.Repeat("x", 21) }, errcode.ParamError},
		{"bad cycle", func(r *ScheduledGraphCreateRequest) { r.Cron.ScheduleCycle = "HOUR" }, errcode.ScheduledScheduleCreateError},
		{"raw cron in time", func(r *ScheduledGraphCreateRequest) { r.Cron.ScheduleTime = "0 0 2 * * ?" }, errcode.ScheduledScheduleCreateError},
		{"end before start", func(r *ScheduledGraphCreateRequest) { r.Cron.EndTime = "2023-01-01 00:00:00" }, errcode.ScheduledScheduleCreateError},
		{"no dates in range", func(r *ScheduledGraphCreateRequest) {
			r.Cron.EndTime = "2024-01-01 09:00:00"
		}, errcode.ScheduledScheduleCreateError},
		{"missing graph", func(r *ScheduledGraphCreateRequest) { r.GraphID = "nope" }, errcode.GraphNotFound},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := dailyCreate("v-" + fmt.Sprint(len(c.name)))
			c.mut(r)
			expectCode(t, f.svc.CreateScheduler(f.ctx, r), c.code)
		})
	}

	// Graph that never succeeded / never ran.
	f.db.Create(&model.ProjectGraphDO{ProjectID: "p1", GraphID: "g2", Name: "g2"})
	r := dailyCreate("never-1")
	r.GraphID = "g2"
	expectCode(t, f.svc.CreateScheduler(f.ctx, r), errcode.ScheduledProjectJobNotExist)
	f.db.Create(&model.ProjectJobDO{ProjectID: "p1", JobID: "jf", Name: "x", Status: "FAILED", GraphID: "g2"})
	expectCode(t, f.svc.CreateScheduler(f.ctx, r), errcode.ScheduledProjectJobNeedSuccessOnce)

	// Renderer returning no request / failing / missing.
	f.renderer.nilRq = true
	expectCode(t, f.svc.CreateScheduler(f.ctx, dailyCreate("nil-1")), errcode.ScheduledRequestIsNull)
	f.renderer.nilRq = false
	f.renderer.err = errors.New("boom")
	expectCode(t, f.svc.CreateScheduler(f.ctx, dailyCreate("err-1")), errcode.ScheduledScheduleCreateError)
	f.svc.SetGraphRenderer(nil)
	expectCode(t, f.svc.CreateScheduler(f.ctx, dailyCreate("nor-1")), errcode.ScheduledScheduleCreateError)

	var cnt int64
	f.db.Model(&model.ProjectScheduleDO{}).Count(&cnt)
	if cnt != 0 {
		t.Errorf("failed creations must not persist schedules, got %d", cnt)
	}
}

func TestScheduled_CreateScheduler_GeneratesIDWhenMissing(t *testing.T) {
	f := newSchedFixture(t)
	if err := f.svc.CreateScheduler(f.ctx, dailyCreate("")); err != nil {
		t.Fatal(err)
	}
	var sc model.ProjectScheduleDO
	if err := f.db.First(&sc).Error; err != nil || sc.ScheduleID == "" {
		t.Fatalf("schedule %+v err %v", sc, err)
	}
}

// ---------------------------------------------------------------------------
// engine
// ---------------------------------------------------------------------------

func TestScheduledEngine_FiresAtExpectedTime(t *testing.T) {
	f := newSchedFixture(t)
	if err := f.svc.CreateScheduler(f.ctx, dailyCreate("eng-1")); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	n, err := f.svc.RunDue(ctx, schedTime("2024-01-01 09:59:59"))
	if err != nil || n != 0 {
		t.Fatalf("early run fired %d err %v", n, err)
	}
	now := schedTime("2024-01-01 10:00:00")
	n, err = f.svc.RunDue(ctx, now)
	if err != nil || n != 1 {
		t.Fatalf("expected 1 fired, got %d err %v", n, err)
	}
	ts := f.tasks(t)
	if ts[0].Status != model.ScheduledStatusRunning || ts[0].ScheduleTaskStartTime == nil || !ts[0].ScheduleTaskStartTime.Equal(now) {
		t.Errorf("task after fire: %+v", ts[0])
	}
	if ts[1].Status != model.ScheduledStatusToBeRun {
		t.Errorf("second task must still wait: %s", ts[1].Status)
	}
	if len(f.kuscia.created) != 1 || f.kuscia.created[0] != ts[0].ScheduleJobID {
		t.Errorf("kuscia created %v", f.kuscia.created)
	}
	// Re-running does not re-fire.
	n, _ = f.svc.RunDue(ctx, now.Add(time.Second))
	if n != 0 || len(f.kuscia.created) != 1 {
		t.Errorf("task fired twice: %d %v", n, f.kuscia.created)
	}
}

func TestScheduledEngine_Misfire(t *testing.T) {
	f := newSchedFixture(t)
	if err := f.svc.CreateScheduler(f.ctx, dailyCreate("mis-1")); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	// Engine was down: day 1 is 1 day late (> grace), day 2 is 5 minutes late (< grace).
	n, err := f.svc.RunDue(ctx, schedTime("2024-01-02 10:05:00"))
	if err != nil || n != 1 {
		t.Fatalf("expected 1 fired, got %d err %v", n, err)
	}
	ts := f.tasks(t)
	if ts[0].Status != model.ScheduledStatusFailed || ts[0].ScheduleTaskEndTime == nil {
		t.Errorf("day1 should be misfired FAILED: %+v", ts[0])
	}
	if j := f.job(t, ts[0].ScheduleJobID); j.Status != model.ScheduleJobStatusFailed || !strings.Contains(j.ErrMsg, "misfired") {
		t.Errorf("misfired job: %+v", j)
	}
	if ts[1].Status != model.ScheduledStatusRunning {
		t.Errorf("day2 within grace should run: %s", ts[1].Status)
	}
	if len(f.kuscia.created) != 1 || f.kuscia.created[0] != ts[1].ScheduleJobID {
		t.Errorf("kuscia created %v", f.kuscia.created)
	}
}

func TestScheduledEngine_ReloadAfterRestart(t *testing.T) {
	f := newSchedFixture(t)
	if err := f.svc.CreateScheduler(f.ctx, dailyCreate("rs-1")); err != nil {
		t.Fatal(err)
	}
	// A brand new service instance on the same DB (process restart).
	svc2 := NewScheduledService(f.db, nil, nil, nil, ScheduledOptions{Location: schedLoc, MisfireGrace: time.Hour})
	k2 := &fakeScheduleKuscia{}
	svc2.SetKusciaClient(k2)
	n, err := svc2.RunDue(context.Background(), schedTime("2024-01-01 10:30:00"))
	if err != nil || n != 1 || len(k2.created) != 1 {
		t.Fatalf("restart reload fired %d err %v created %v", n, err, k2.created)
	}
}

func TestScheduledEngine_ConcurrentInstancesFireOnce(t *testing.T) {
	f := newSchedFixture(t)
	if err := f.svc.CreateScheduler(f.ctx, dailyCreate("cc-1")); err != nil {
		t.Fatal(err)
	}
	other := NewScheduledService(f.db, nil, nil, nil, ScheduledOptions{Location: schedLoc})
	other.SetKusciaClient(f.kuscia)
	now := schedTime("2024-01-01 10:00:01")
	var wg sync.WaitGroup
	for _, s := range []*ScheduledService{f.svc, other, f.svc, other} {
		wg.Add(1)
		go func(s *ScheduledService) {
			defer wg.Done()
			_, _ = s.RunDue(context.Background(), now)
		}(s)
	}
	wg.Wait()
	if len(f.kuscia.created) != 1 {
		t.Errorf("expected exactly one submission, got %v", f.kuscia.created)
	}
}

func TestScheduledEngine_SkipsDownAndNonPending(t *testing.T) {
	f := newSchedFixture(t)
	if err := f.svc.CreateScheduler(f.ctx, dailyCreate("dn-1")); err != nil {
		t.Fatal(err)
	}
	ts := f.tasks(t)
	// Task stopped before firing is skipped.
	f.db.Model(&model.ProjectScheduleTaskDO{}).Where("id = ?", ts[0].ID).Update("status", model.ScheduledStatusStopped)
	n, _ := f.svc.RunDue(context.Background(), schedTime("2024-01-01 10:00:00"))
	if n != 0 {
		t.Errorf("stopped task fired")
	}
	// Offline schedule: nothing fires.
	if err := f.svc.Offline(f.ctx, &ScheduledOfflineRequest{ScheduleID: "dn-1"}); err != nil {
		t.Fatal(err)
	}
	n, _ = f.svc.RunDue(context.Background(), schedTime("2024-01-02 10:00:00"))
	if n != 0 || len(f.kuscia.created) != 0 {
		t.Errorf("down schedule fired: %d %v", n, f.kuscia.created)
	}
}

func TestScheduledEngine_KusciaFailureMarksFailed(t *testing.T) {
	f := newSchedFixture(t)
	if err := f.svc.CreateScheduler(f.ctx, dailyCreate("kf-1")); err != nil {
		t.Fatal(err)
	}
	f.kuscia.createErr = errors.New("kuscia down")
	n, _ := f.svc.RunDue(context.Background(), schedTime("2024-01-01 10:00:00"))
	if n != 0 {
		t.Errorf("expected no successful fire")
	}
	ts := f.tasks(t)
	if ts[0].Status != model.ScheduledStatusFailed || ts[0].ScheduleTaskEndTime == nil {
		t.Errorf("task: %+v", ts[0])
	}
	if j := f.job(t, ts[0].ScheduleJobID); j.Status != model.ScheduleJobStatusFailed || !strings.Contains(j.ErrMsg, "kuscia down") {
		t.Errorf("job: %+v", j)
	}
}

func TestScheduledEngine_StartStopLoop(t *testing.T) {
	f := newSchedFixture(t)
	if err := f.svc.CreateScheduler(f.ctx, dailyCreate("lp-1")); err != nil {
		t.Fatal(err)
	}
	svc := NewScheduledService(f.db, nil, nil, nil, ScheduledOptions{
		Location: schedLoc, PollInterval: 10 * time.Millisecond, SyncInterval: 10 * time.Millisecond,
		Now: func() time.Time { return schedTime("2024-01-01 10:00:30") },
	})
	svc.SetKusciaClient(f.kuscia)
	svc.Start()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		f.kuscia.mu.Lock()
		n := len(f.kuscia.created)
		f.kuscia.mu.Unlock()
		if n == 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	svc.Stop()
	svc.Stop() // idempotent
	if len(f.kuscia.created) != 1 {
		t.Fatalf("loop did not fire: %v", f.kuscia.created)
	}
}

// ---------------------------------------------------------------------------
// status write-back
// ---------------------------------------------------------------------------

func TestScheduled_ApplyKusciaJobStatus(t *testing.T) {
	f := newSchedFixture(t)
	if err := f.svc.CreateScheduler(f.ctx, dailyCreate("st-1")); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	f.svc.RunDue(ctx, schedTime("2024-01-01 10:00:00"))
	tk := f.tasks(t)[0]
	jobID := tk.ScheduleJobID

	f.kuscia.states[jobID] = &kuscia.JobExecutionStatus{State: "Running", Tasks: []kuscia.TaskStatus{{TaskID: jobID + "-n2", State: "Running"}}}
	if err := f.svc.SyncRunning(ctx); err != nil {
		t.Fatal(err)
	}
	if got := f.task(t, tk.ScheduleTaskID).Status; got != model.ScheduledStatusRunning {
		t.Errorf("status %s", got)
	}

	f.clock.Set(schedTime("2024-01-01 10:30:00"))
	f.kuscia.states[jobID] = &kuscia.JobExecutionStatus{State: "Succeeded", Tasks: []kuscia.TaskStatus{{TaskID: jobID + "-n2", State: "Succeeded"}}}
	if err := f.svc.SyncRunning(ctx); err != nil {
		t.Fatal(err)
	}
	got := f.task(t, tk.ScheduleTaskID)
	if got.Status != model.ScheduledStatusSucceed || got.ScheduleTaskEndTime == nil || !got.ScheduleTaskEndTime.Equal(schedTime("2024-01-01 10:30:00")) {
		t.Errorf("task %+v", got)
	}
	j := f.job(t, jobID)
	if j.Status != model.ScheduleJobStatusSucceed || j.FinishedTime == nil {
		t.Errorf("job %+v", j)
	}
	var jt model.ProjectJobTaskDO
	f.db.Where("job_id = ? AND task_id = ?", jobID, jobID+"-n2").First(&jt)
	if jt.Status != "SUCCEED" {
		t.Errorf("job task status %s", jt.Status)
	}
	// SUCCEED jobs are final (Java skips them).
	f.kuscia.states[jobID] = &kuscia.JobExecutionStatus{State: "Failed"}
	handled, err := f.svc.ApplyKusciaJobStatus(ctx, jobID, f.kuscia.states[jobID])
	if !handled || err != nil || f.job(t, jobID).Status != model.ScheduleJobStatusSucceed {
		t.Errorf("succeed job regressed: %v %v", handled, err)
	}
	// Unknown job is not handled.
	handled, err = f.svc.ApplyKusciaJobStatus(ctx, "not-a-schedule-job", &kuscia.JobExecutionStatus{State: "Running"})
	if handled || err != nil {
		t.Errorf("unknown job handled=%v err=%v", handled, err)
	}
	// Hook registry.
	RegisterScheduleJobStatusHook(f.svc)
	if CurrentScheduleJobStatusHook() == nil {
		t.Error("hook not registered")
	}
	RegisterScheduleJobStatusHook(nil)
}

func TestScheduled_ApplyKusciaJobStatus_FailedAndToBeRunUntouched(t *testing.T) {
	f := newSchedFixture(t)
	if err := f.svc.CreateScheduler(f.ctx, dailyCreate("st-2")); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	ts := f.tasks(t)
	// Not fired yet: a stray status must not resurrect the TO_BE_RUN task.
	f.svc.ApplyKusciaJobStatus(ctx, ts[1].ScheduleJobID, &kuscia.JobExecutionStatus{State: "Running"})
	if got := f.task(t, ts[1].ScheduleTaskID).Status; got != model.ScheduledStatusToBeRun {
		t.Errorf("TO_BE_RUN task changed to %s", got)
	}
	f.svc.RunDue(ctx, schedTime("2024-01-01 10:00:00"))
	f.svc.ApplyKusciaJobStatus(ctx, ts[0].ScheduleJobID, &kuscia.JobExecutionStatus{State: "Failed", ErrMsg: "oops"})
	if got := f.task(t, ts[0].ScheduleTaskID); got.Status != model.ScheduledStatusFailed || got.ScheduleTaskEndTime == nil {
		t.Errorf("task %+v", got)
	}
	if j := f.job(t, ts[0].ScheduleJobID); j.ErrMsg != "oops" || j.Status != model.ScheduleJobStatusFailed {
		t.Errorf("job %+v", j)
	}
}

// ---------------------------------------------------------------------------
// offline / del / page / info
// ---------------------------------------------------------------------------

func TestScheduled_OfflineAndDel(t *testing.T) {
	f := newSchedFixture(t)
	if err := f.svc.CreateScheduler(f.ctx, dailyCreate("od-1")); err != nil {
		t.Fatal(err)
	}
	other := auth.ContextWithClaims(context.Background(), &auth.Claims{OwnerID: "bob", Username: "bob"})

	expectCode(t, f.svc.Del(f.ctx, &ScheduledDelRequest{ScheduleID: "od-1"}), errcode.ScheduledScheduleUpNotDel)
	expectCode(t, f.svc.Offline(f.ctx, &ScheduledOfflineRequest{ScheduleID: "missing"}), errcode.ScheduledScheduleNotExist)

	// Running task blocks offline.
	f.svc.RunDue(context.Background(), schedTime("2024-01-01 10:00:00"))
	expectCode(t, f.svc.Offline(f.ctx, &ScheduledOfflineRequest{ScheduleID: "od-1"}), errcode.ScheduledScheduleRunningNotOffline)
	f.db.Model(&model.ProjectScheduleTaskDO{}).Where("status = ?", model.ScheduledStatusRunning).Update("status", model.ScheduledStatusSucceed)

	expectCode(t, f.svc.Offline(other, &ScheduledOfflineRequest{ScheduleID: "od-1"}), errcode.ScheduledUserNotOwner)
	if err := f.svc.Offline(f.ctx, &ScheduledOfflineRequest{ScheduleID: "od-1"}); err != nil {
		t.Fatal(err)
	}
	var sc model.ProjectScheduleDO
	f.db.Where("schedule_id = ?", "od-1").First(&sc)
	if sc.Status != model.ScheduledStatusDown {
		t.Errorf("status %s", sc.Status)
	}
	expectCode(t, f.svc.Del(other, &ScheduledDelRequest{ScheduleID: "od-1"}), errcode.ScheduledUserNotOwner)
	if err := f.svc.Del(f.ctx, &ScheduledDelRequest{ScheduleID: "od-1"}); err != nil {
		t.Fatal(err)
	}
	var cnt int64
	f.db.Model(&model.ProjectScheduleDO{}).Where("schedule_id = ?", "od-1").Count(&cnt)
	if cnt != 0 {
		t.Error("schedule not deleted")
	}
	f.db.Unscoped().Model(&model.ProjectScheduleDO{}).Where("schedule_id = ? AND is_deleted = 1", "od-1").Count(&cnt)
	if cnt != 1 {
		t.Error("expected soft delete")
	}
}

func TestScheduled_QueryPageAndTaskPage(t *testing.T) {
	f := newSchedFixture(t)
	for _, id := range []string{"pg-1", "pg-2"} {
		if err := f.svc.CreateScheduler(f.ctx, dailyCreate(id)); err != nil {
			t.Fatal(err)
		}
	}
	f.svc.RunDue(context.Background(), schedTime("2024-01-01 10:00:00"))

	page, err := f.svc.QueryPage(f.ctx, &PageScheduledRequest{ProjectID: "p1", Page: 1, Size: 10})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 2 || len(page.List) != 2 {
		t.Fatalf("page %+v", page)
	}
	for _, r := range page.List {
		if !r.TaskRunning || r.OwnerName != "Alice Node" || r.ScheduleStats != "UP" || r.Creator != "admin" || r.CreateTime != "2024-01-01 00:00:00" {
			t.Errorf("row %+v", r)
		}
	}
	page, _ = f.svc.QueryPage(f.ctx, &PageScheduledRequest{ProjectID: "p1", Search: "pg-2"})
	if page.Total != 1 || page.List[0].ScheduleID != "pg-2" {
		t.Errorf("search %+v", page)
	}
	page, _ = f.svc.QueryPage(f.ctx, &PageScheduledRequest{ProjectID: "p1", Status: "DOWN"})
	if page.Total != 0 || page.List == nil {
		t.Errorf("status filter %+v", page)
	}
	page, _ = f.svc.QueryPage(f.ctx, &PageScheduledRequest{ProjectID: "p1", Page: 2, Size: 1})
	if page.Total != 2 || len(page.List) != 1 {
		t.Errorf("paging %+v", page)
	}

	tp, err := f.svc.TaskPage(f.ctx, &TaskPageScheduledRequest{ScheduleID: "pg-1", Page: 1, Size: 2})
	if err != nil {
		t.Fatal(err)
	}
	if tp.Total != 3 || len(tp.List) != 2 {
		t.Fatalf("task page %+v", tp)
	}
	statuses := map[string]bool{}
	all, _ := f.svc.TaskPage(f.ctx, &TaskPageScheduledRequest{ScheduleID: "pg-1", Size: 10})
	for _, r := range all.List {
		statuses[r.ScheduleTaskStatus] = true
		if r.ScheduleTaskExpectStartTime == "" {
			t.Errorf("missing expect time %+v", r)
		}
	}
	if !statuses["RUNNING"] || !statuses["TO_BE_RUN"] {
		t.Errorf("statuses %v", statuses)
	}
}

func TestScheduled_InfoAndTaskInfo(t *testing.T) {
	f := newSchedFixture(t)
	if err := f.svc.CreateScheduler(f.ctx, dailyCreate("in-1")); err != nil {
		t.Fatal(err)
	}
	vo, err := f.svc.Info(f.ctx, &ScheduledInfoRequest{ScheduleID: "in-1"})
	if err != nil {
		t.Fatal(err)
	}
	if vo.JobID != "job0" || vo.Status != "SUCCEED" || !vo.Finished || vo.Graph == nil || len(vo.Graph.Nodes) != 2 {
		t.Fatalf("info %+v", vo)
	}
	var n2 ScheduledGraphNodeVO
	for _, n := range vo.Graph.Nodes {
		if n.GraphNodeID == "n2" {
			n2 = n
		}
	}
	if n2.Status != "SUCCEED" || n2.TaskID != "job0-n2" || len(n2.Parties) != 1 || n2.Parties[0]["nodeName"] != "Alice Node" {
		t.Errorf("node n2 %+v", n2)
	}
	_, err = f.svc.Info(f.ctx, &ScheduledInfoRequest{ScheduleID: "missing"})
	expectCode(t, err, errcode.ScheduledScheduleNotExist)

	tk := f.tasks(t)[0]
	f.db.Create(&model.ProjectResultDO{ProjectID: "p1", Kind: "FedTable", NodeID: "alice", RefID: "r1", JobID: tk.ScheduleJobID, TaskID: tk.ScheduleJobID + "-n2"})
	tvo, err := f.svc.TaskInfo(f.ctx, &TaskInfoScheduledRequest{ScheduleID: "in-1", ScheduleTaskID: tk.ScheduleTaskID})
	if err != nil {
		t.Fatal(err)
	}
	if tvo.JobID != tk.ScheduleJobID || tvo.Status != model.ScheduleJobStatusRunning || tvo.Finished || string(tvo.Graph.Edges) == "" {
		t.Fatalf("task info %+v", tvo)
	}
	found := false
	for _, n := range tvo.Graph.Nodes {
		if n.GraphNodeID == "n2" && n.Status == "INITIALIZED" && len(n.Results) == 1 {
			found = true
		}
	}
	if !found {
		t.Errorf("task info nodes %+v", tvo.Graph.Nodes)
	}
	_, err = f.svc.TaskInfo(f.ctx, &TaskInfoScheduledRequest{ScheduleID: "in-1", ScheduleTaskID: "missing"})
	expectCode(t, err, errcode.ScheduledScheduleTaskNotExist)
}

func TestScheduled_ListProjectJob(t *testing.T) {
	f := newSchedFixture(t)
	if err := f.svc.CreateScheduler(f.ctx, dailyCreate("jl-1")); err != nil {
		t.Fatal(err)
	}
	tk := f.tasks(t)[0]
	f.db.Create(&model.ProjectResultDO{ProjectID: "p1", Kind: "FedTable", NodeID: "alice", RefID: "t1", JobID: tk.ScheduleJobID})
	f.db.Create(&model.ProjectResultDO{ProjectID: "p1", Kind: "Model", NodeID: "alice", RefID: "m1", JobID: tk.ScheduleJobID})
	f.db.Create(&model.ProjectResultDO{ProjectID: "p1", Kind: "Report", NodeID: "alice", RefID: "rp1", JobID: tk.ScheduleJobID})
	f.db.Model(&model.ProjectJobTaskDO{}).Where("task_id = ?", tk.ScheduleJobID+"-n1").Update("status", "SUCCEED")

	page, err := f.svc.ListProjectJob(f.ctx, &ScheduleListProjectJobRequest{ProjectID: "p1", GraphID: "g1", ScheduleTaskID: tk.ScheduleTaskID, PageNum: 1, PageSize: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Data) != 1 || len(page.List) != 1 || page.PageTotal != 1 || page.PageSize != 10 {
		t.Fatalf("page %+v", page)
	}
	d := page.Data[0]
	if d.JobID != tk.ScheduleJobID || d.TableCount != 1 || d.ModelCount != 1 || d.ReportCount != 1 || d.RuleCount != 0 || d.TaskCount != 2 || d.FinishedTaskCount != 1 {
		t.Errorf("summary %+v", d)
	}
	page, _ = f.svc.ListProjectJob(f.ctx, &ScheduleListProjectJobRequest{ProjectID: "p1", GraphID: "g1", ScheduleTaskID: "none"})
	if len(page.Data) != 0 || page.Data == nil {
		t.Errorf("empty page %+v", page)
	}
	_, err = f.svc.ListProjectJob(f.ctx, &ScheduleListProjectJobRequest{ProjectID: "p1", GraphID: "missing", ScheduleTaskID: "x"})
	expectCode(t, err, errcode.GraphNotFound)
}

// ---------------------------------------------------------------------------
// task stop / rerun
// ---------------------------------------------------------------------------

func TestScheduled_TaskStop(t *testing.T) {
	f := newSchedFixture(t)
	if err := f.svc.CreateScheduler(f.ctx, dailyCreate("sp-1")); err != nil {
		t.Fatal(err)
	}
	ts := f.tasks(t)
	other := auth.ContextWithClaims(context.Background(), &auth.Claims{OwnerID: "bob"})
	expectCode(t, f.svc.TaskStop(other, &TaskStopScheduledRequest{ScheduleID: "sp-1", ScheduleTaskID: ts[0].ScheduleTaskID}), errcode.ScheduledUserNotOwner)

	// Pending task: stopped without touching Kuscia, never fires.
	if err := f.svc.TaskStop(f.ctx, &TaskStopScheduledRequest{ScheduleID: "sp-1", ScheduleTaskID: ts[1].ScheduleTaskID}); err != nil {
		t.Fatal(err)
	}
	if got := f.task(t, ts[1].ScheduleTaskID).Status; got != model.ScheduledStatusStopped {
		t.Errorf("pending stop status %s", got)
	}

	// Running task: Kuscia StopJob + STOPPING, then STOPPED via sync.
	f.svc.RunDue(context.Background(), schedTime("2024-01-01 10:00:00"))
	if err := f.svc.TaskStop(f.ctx, &TaskStopScheduledRequest{ScheduleID: "sp-1", ScheduleTaskID: ts[0].ScheduleTaskID}); err != nil {
		t.Fatal(err)
	}
	if len(f.kuscia.stopped) != 1 || f.kuscia.stopped[0] != ts[0].ScheduleJobID {
		t.Errorf("stopped %v", f.kuscia.stopped)
	}
	if got := f.task(t, ts[0].ScheduleTaskID).Status; got != model.ScheduledStatusStopping {
		t.Errorf("status %s", got)
	}
	if j := f.job(t, ts[0].ScheduleJobID); j.Status != model.ScheduleJobStatusStopped {
		t.Errorf("job %s", j.Status)
	}
	f.kuscia.states[ts[0].ScheduleJobID] = &kuscia.JobExecutionStatus{State: "Running"}
	f.svc.SyncRunning(context.Background())
	if got := f.task(t, ts[0].ScheduleTaskID).Status; got != model.ScheduledStatusStopping {
		t.Errorf("STOPPING must hold while kuscia still running, got %s", got)
	}
	f.kuscia.states[ts[0].ScheduleJobID] = &kuscia.JobExecutionStatus{State: "Stopped"}
	f.svc.SyncRunning(context.Background())
	if got := f.task(t, ts[0].ScheduleTaskID).Status; got != model.ScheduledStatusStopped {
		t.Errorf("expected STOPPED, got %s", got)
	}
	// Finished task cannot be stopped.
	expectCode(t, f.svc.TaskStop(f.ctx, &TaskStopScheduledRequest{ScheduleID: "sp-1", ScheduleTaskID: ts[0].ScheduleTaskID}), errcode.ScheduledScheduleTaskStatusNotRunning)
	expectCode(t, f.svc.TaskStop(f.ctx, &TaskStopScheduledRequest{ScheduleID: "sp-1", ScheduleTaskID: "missing"}), errcode.ScheduledScheduleTaskNotExist)
}

func TestScheduled_TaskRerun(t *testing.T) {
	f := newSchedFixture(t)
	if err := f.svc.CreateScheduler(f.ctx, dailyCreate("rr-1")); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	ts := f.tasks(t)
	req := func(typ string) *TaskReRunScheduledRequest {
		return &TaskReRunScheduledRequest{ScheduleID: "rr-1", ScheduleTaskID: ts[0].ScheduleTaskID, Type: typ}
	}
	// Not finished yet.
	expectCode(t, f.svc.TaskRerun(f.ctx, req("0")), errcode.ScheduledProjectJobRestartError)

	f.svc.RunDue(ctx, schedTime("2024-01-01 10:00:00"))
	f.svc.ApplyKusciaJobStatus(ctx, ts[0].ScheduleJobID, &kuscia.JobExecutionStatus{State: "Failed"})
	f.db.Create(&model.ProjectResultDO{ProjectID: "p1", Kind: "FedTable", NodeID: "alice", RefID: "old", JobID: ts[0].ScheduleJobID})

	expectCode(t, f.svc.TaskRerun(f.ctx, req("9")), errcode.ScheduledProjectJobRestartError)

	// FAILED rerun -> Kuscia RestartJob.
	f.clock.Set(schedTime("2024-01-02 00:00:00"))
	if err := f.svc.TaskRerun(f.ctx, req("1")); err != nil {
		t.Fatal(err)
	}
	if len(f.kuscia.restarted) != 1 {
		t.Errorf("restarted %v", f.kuscia.restarted)
	}
	got := f.task(t, ts[0].ScheduleTaskID)
	if got.Status != model.ScheduledStatusRunning || got.ScheduleTaskEndTime != nil || !got.ScheduleTaskStartTime.Equal(schedTime("2024-01-02 00:00:00")) {
		t.Errorf("after restart %+v", got)
	}
	if j := f.job(t, ts[0].ScheduleJobID); j.Status != model.ScheduleJobStatusRunning || j.FinishedTime != nil {
		t.Errorf("job %+v", j)
	}

	// ALL rerun -> delete + recreate from stored request, clears results, once only.
	f.svc.ApplyKusciaJobStatus(ctx, ts[0].ScheduleJobID, &kuscia.JobExecutionStatus{State: "Failed"})
	if err := f.svc.TaskRerun(f.ctx, req("")); err != nil {
		t.Fatal(err)
	}
	if len(f.kuscia.deleted) != 1 || f.kuscia.deleted[0] != ts[0].ScheduleJobID || len(f.kuscia.created) != 2 {
		t.Errorf("deleted %v created %v", f.kuscia.deleted, f.kuscia.created)
	}
	var cnt int64
	f.db.Model(&model.ProjectResultDO{}).Where("job_id = ?", ts[0].ScheduleJobID).Count(&cnt)
	if cnt != 0 {
		t.Error("results not cleared")
	}
	if !f.task(t, ts[0].ScheduleTaskID).AllReRun {
		t.Error("allReRun not set")
	}
	f.svc.ApplyKusciaJobStatus(ctx, ts[0].ScheduleJobID, &kuscia.JobExecutionStatus{State: "Failed"})
	expectCode(t, f.svc.TaskRerun(f.ctx, req("0")), errcode.ScheduledProjectJobRestartError)

	// FAILED rerun older than 30 days is refused.
	f.clock.Set(schedTime("2024-03-01 00:00:00"))
	expectCode(t, f.svc.TaskRerun(f.ctx, req("1")), errcode.ScheduledProjectJobRestartError)

	// Owner check and DOWN schedule.
	other := auth.ContextWithClaims(context.Background(), &auth.Claims{OwnerID: "bob"})
	expectCode(t, f.svc.TaskRerun(other, req("0")), errcode.ScheduledUserNotOwner)
	f.db.Model(&model.ProjectScheduleDO{}).Where("schedule_id = ?", "rr-1").Update("status", model.ScheduledStatusDown)
	expectCode(t, f.svc.TaskRerun(f.ctx, req("1")), errcode.ScheduledProjectJobRestartError)
}

func TestScheduled_TaskRerunAll_NeverSubmitted(t *testing.T) {
	f := newSchedFixture(t)
	if err := f.svc.CreateScheduler(f.ctx, dailyCreate("ns-1")); err != nil {
		t.Fatal(err)
	}
	// Misfire day 1 (never reached Kuscia), then rerun all: no DeleteJob.
	f.svc.RunDue(context.Background(), schedTime("2024-01-01 12:00:00"))
	tk := f.tasks(t)[0]
	if tk.Status != model.ScheduledStatusFailed {
		t.Fatalf("status %s", tk.Status)
	}
	if err := f.svc.TaskRerun(f.ctx, &TaskReRunScheduledRequest{ScheduleID: "ns-1", ScheduleTaskID: tk.ScheduleTaskID}); err != nil {
		t.Fatal(err)
	}
	if len(f.kuscia.deleted) != 0 || len(f.kuscia.created) != 1 {
		t.Errorf("deleted %v created %v", f.kuscia.deleted, f.kuscia.created)
	}
}

func TestScheduledStatusFrom(t *testing.T) {
	cases := map[string]string{
		"SUCCESS": "SUCCEED", "Succeeded": "SUCCEED", "succeed": "SUCCEED", "FAILED": "FAILED",
		"Stopped": "STOPPED", "STOPPING": "STOPPING", "UP": "UP", "DOWN": "DOWN", "TO_BE_RUN": "TO_BE_RUN",
		"Running": "RUNNING", "whatever": "RUNNING",
	}
	for in, want := range cases {
		if got := model.ScheduledStatusFrom(in); got != want {
			t.Errorf("ScheduledStatusFrom(%q)=%q want %q", in, got, want)
		}
	}
}
