package v1

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/internal/service"
	"github.com/fengzhizi319/privahub/pkg/errcode"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

type schedResp struct {
	Status struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	} `json:"status"`
	Data json.RawMessage `json:"data"`
}

func setupScheduledHandler(t *testing.T) (*gin.Engine, *gorm.DB) {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: gormlogger.Default.LogMode(gormlogger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(
		&model.ProjectDO{}, &model.ProjectGraphDO{}, &model.ProjectGraphNodeDO{},
		&model.ProjectJobDO{}, &model.ProjectJobTaskDO{}, &model.ProjectResultDO{},
		&model.NodeDO{}, &model.InstDO{},
		&model.ProjectScheduleDO{}, &model.ProjectScheduleJobDO{}, &model.ProjectScheduleTaskDO{},
	); err != nil {
		t.Fatal(err)
	}
	db.Create(&model.ProjectDO{ProjectID: "p1", Name: "p1"})
	db.Create(&model.ProjectGraphDO{ProjectID: "p1", GraphID: "g1", Name: "g1"})
	db.Create(&model.ProjectJobDO{ProjectID: "p1", JobID: "j1", Name: "j1", Status: "SUCCEED", GraphID: "g1"})

	svc := service.NewScheduledService(db, nil, nil, nil, service.ScheduledOptions{Location: time.UTC})
	h := NewScheduledHandler(svc)
	r := setupTestRouter()
	g := r.Group("/api/v1alpha1")
	g.POST("/scheduled/id", h.GetID)
	g.POST("/scheduled/graph/once/success", h.OnceSuccess)
	g.POST("/scheduled/graph/create", h.GraphCreate)
	g.POST("/scheduled/page", h.Page)
	g.POST("/scheduled/offline", h.Offline)
	g.POST("/scheduled/del", h.Del)
	g.POST("/scheduled/info", h.Info)
	g.POST("/scheduled/task/page", h.TaskPage)
	g.POST("/scheduled/task/stop", h.TaskStop)
	g.POST("/scheduled/task/rerun", h.TaskReRun)
	g.POST("/scheduled/task/info", h.TaskInfo)
	g.POST("/scheduled/job/list", h.JobList)
	return r, db
}

func doSched(t *testing.T, r *gin.Engine, path string, body any) schedResp {
	t.Helper()
	w := postJSON(r, "/api/v1alpha1/scheduled"+path, body)
	var resp schedResp
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("%s: bad json %s", path, w.Body.String())
	}
	return resp
}

func TestScheduledHandler_ID(t *testing.T) {
	r, _ := setupScheduledHandler(t)
	resp := doSched(t, r, "/id", map[string]any{"projectId": "p1", "graphId": "g1"})
	var id string
	_ = json.Unmarshal(resp.Data, &id)
	if resp.Status.Code != 0 || !regexp.MustCompile(`^[a-z]{4}-\d{14}$`).MatchString(id) {
		t.Fatalf("resp %+v id %q", resp.Status, id)
	}
	resp = doSched(t, r, "/id", map[string]any{"projectId": "p1"})
	if resp.Status.Code != errcode.ParamError.Code {
		t.Errorf("missing graphId: %+v", resp.Status)
	}
	resp = doSched(t, r, "/id", map[string]any{"projectId": "p1", "graphId": "nope"})
	if resp.Status.Code != errcode.GraphNotFound.Code {
		t.Errorf("missing graph: %+v", resp.Status)
	}
}

func TestScheduledHandler_OnceSuccess(t *testing.T) {
	r, db := setupScheduledHandler(t)
	resp := doSched(t, r, "/graph/once/success", map[string]any{"projectId": "p1", "graphId": "g1"})
	if resp.Status.Code != 0 || string(resp.Data) != "true" {
		t.Fatalf("resp %+v %s", resp.Status, resp.Data)
	}
	db.Create(&model.ProjectGraphDO{ProjectID: "p1", GraphID: "g2", Name: "g2"})
	resp = doSched(t, r, "/graph/once/success", map[string]any{"projectId": "p1", "graphId": "g2"})
	if resp.Status.Code != 0 || string(resp.Data) != "false" {
		t.Fatalf("resp %+v %s", resp.Status, resp.Data)
	}
}

func TestScheduledHandler_GraphCreateErrors(t *testing.T) {
	r, _ := setupScheduledHandler(t)
	body := map[string]any{
		"scheduleId": "abcd-1", "projectId": "p1", "graphId": "g1", "nodes": []string{"n1"},
		"cron": map[string]any{"startTime": "2099-01-01 00:00:00", "endTime": "2099-01-03 00:00:00", "scheduleCycle": "D", "scheduleTime": "10:00"},
	}
	// No scheduled-mode renderer wired in this test -> SCHEDULE_CREATE_ERROR.
	resp := doSched(t, r, "/graph/create", body)
	if resp.Status.Code != errcode.ScheduledScheduleCreateError.Code || !strings.Contains(resp.Status.Msg, "render") {
		t.Errorf("resp %+v", resp.Status)
	}
	body["cron"] = map[string]any{"startTime": "2099-01-01 00:00:00", "endTime": "2099-01-03 00:00:00", "scheduleCycle": "X", "scheduleTime": "10:00"}
	resp = doSched(t, r, "/graph/create", body)
	if resp.Status.Code != errcode.ScheduledScheduleCreateError.Code || !strings.Contains(resp.Status.Msg, "cycle") {
		t.Errorf("bad cycle resp %+v", resp.Status)
	}
	delete(body, "nodes")
	resp = doSched(t, r, "/graph/create", body)
	if resp.Status.Code != errcode.ParamError.Code {
		t.Errorf("no nodes resp %+v", resp.Status)
	}
}

func TestScheduledHandler_PageAndLifecycle(t *testing.T) {
	r, db := setupScheduledHandler(t)
	now := time.Now()
	db.Create(&model.ProjectScheduleDO{ScheduleID: "s1", ProjectID: "p1", GraphID: "g1", Status: "UP", Owner: "", Creator: "admin", CreateTime: now, JobInfo: `{"job":{"project_id":"p1","job_id":"j1","status":"SUCCEED","graph_id":"g1"},"tasks":[]}`})
	db.Create(&model.ProjectScheduleTaskDO{ProjectID: "p1", GraphID: "g1", ScheduleID: "s1", ScheduleTaskID: "t1", Cron: "0 0 10 * * ?", ScheduleTaskExpectStart: now.Add(time.Hour), Status: "TO_BE_RUN"})

	resp := doSched(t, r, "/page", map[string]any{"projectId": "p1", "page": 1, "size": 10})
	var page struct {
		List  []map[string]any `json:"list"`
		Total int              `json:"total"`
	}
	_ = json.Unmarshal(resp.Data, &page)
	if resp.Status.Code != 0 || page.Total != 1 || page.List[0]["scheduleId"] != "s1" || page.List[0]["scheduleStats"] != "UP" {
		t.Fatalf("page %+v %s", resp.Status, resp.Data)
	}

	resp = doSched(t, r, "/task/page", map[string]any{"scheduleId": "s1"})
	_ = json.Unmarshal(resp.Data, &page)
	if resp.Status.Code != 0 || page.Total != 1 || page.List[0]["scheduleTaskStatus"] != "TO_BE_RUN" || page.List[0]["allReRun"] != false {
		t.Fatalf("task page %+v %s", resp.Status, resp.Data)
	}

	resp = doSched(t, r, "/info", map[string]any{"scheduleId": "s1"})
	if resp.Status.Code != 0 || !strings.Contains(string(resp.Data), `"jobId":"j1"`) || !strings.Contains(string(resp.Data), `"finished":true`) {
		t.Fatalf("info %+v %s", resp.Status, resp.Data)
	}

	// UP schedule cannot be deleted; offline then delete.
	resp = doSched(t, r, "/del", map[string]any{"scheduleId": "s1"})
	if resp.Status.Code != errcode.ScheduledScheduleUpNotDel.Code {
		t.Errorf("del UP %+v", resp.Status)
	}
	resp = doSched(t, r, "/offline", map[string]any{"scheduleId": "s1"})
	if resp.Status.Code != 0 {
		t.Errorf("offline %+v", resp.Status)
	}
	resp = doSched(t, r, "/del", map[string]any{"scheduleId": "s1"})
	if resp.Status.Code != 0 {
		t.Errorf("del %+v", resp.Status)
	}
	resp = doSched(t, r, "/offline", map[string]any{"scheduleId": "s1"})
	if resp.Status.Code != errcode.ScheduledScheduleNotExist.Code {
		t.Errorf("offline deleted %+v", resp.Status)
	}
	resp = doSched(t, r, "/offline", map[string]any{})
	if resp.Status.Code != errcode.ParamError.Code {
		t.Errorf("offline without id %+v", resp.Status)
	}
}

func TestScheduledHandler_TaskEndpointsValidation(t *testing.T) {
	r, _ := setupScheduledHandler(t)
	resp := doSched(t, r, "/task/rerun", map[string]any{"scheduleId": "s1", "scheduleTaskId": "t1", "type": "5"})
	if resp.Status.Code != errcode.ParamError.Code {
		t.Errorf("rerun bad type %+v", resp.Status)
	}
	resp = doSched(t, r, "/task/rerun", map[string]any{"scheduleId": "s1", "scheduleTaskId": "t1", "type": "1"})
	if resp.Status.Code != errcode.ScheduledScheduleNotExist.Code {
		t.Errorf("rerun missing schedule %+v", resp.Status)
	}
	resp = doSched(t, r, "/task/stop", map[string]any{"scheduleId": "s1", "scheduleTaskId": "t1"})
	if resp.Status.Code != errcode.ScheduledScheduleTaskNotExist.Code {
		t.Errorf("stop missing task %+v", resp.Status)
	}
	resp = doSched(t, r, "/task/stop", map[string]any{"scheduleId": "s1"})
	if resp.Status.Code != errcode.ParamError.Code {
		t.Errorf("stop without task id %+v", resp.Status)
	}
	resp = doSched(t, r, "/task/info", map[string]any{"scheduleId": "s1", "scheduleTaskId": "t1"})
	if resp.Status.Code != errcode.ScheduledScheduleNotExist.Code {
		t.Errorf("task info %+v", resp.Status)
	}
	resp = doSched(t, r, "/job/list", map[string]any{"projectId": "p1", "graphId": "g1", "scheduleTaskId": "t1", "pageNum": 1, "pageSize": 5})
	if resp.Status.Code != 0 || !strings.Contains(string(resp.Data), `"pageSize":5`) || !strings.Contains(string(resp.Data), `"list":[]`) {
		t.Errorf("job list %+v %s", resp.Status, resp.Data)
	}
	// Schedule-level listing (no scheduleTaskId) is a Go extension.
	resp = doSched(t, r, "/job/list", map[string]any{"projectId": "p1", "graphId": "g1"})
	if resp.Status.Code != 0 || !strings.Contains(string(resp.Data), `"total":0`) {
		t.Errorf("job list without task id %+v %s", resp.Status, resp.Data)
	}
	resp = doSched(t, r, "/job/list", map[string]any{"projectId": "p1"})
	if resp.Status.Code != errcode.ParamError.Code {
		t.Errorf("job list without graph id %+v", resp.Status)
	}
}
