package service

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/internal/dao/sqlutil"
	"github.com/fengzhizi319/privahub/internal/graphjob"
	"github.com/fengzhizi319/privahub/pkg/auth"
	"github.com/fengzhizi319/privahub/pkg/cronutil"
	"github.com/fengzhizi319/privahub/pkg/errcode"
	"github.com/fengzhizi319/privahub/pkg/kuscia"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// This file ports Java ScheduledServiceImpl + secretpad-scheduled (Quartz)
// to Go. Quartz is replaced by a DB-backed polling engine:
//
//   - every pre-computed trigger is a project_schedule_task row
//     (status TO_BE_RUN, schedule_task_expect_start = trigger time) holding the
//     pre-rendered Kuscia CreateJob request;
//   - the engine periodically selects due TO_BE_RUN tasks of UP schedules,
//     claims each with a status CAS (TO_BE_RUN -> RUNNING, safe across
//     several privahub instances sharing one DB) and submits the stored
//     request to Kuscia;
//   - restarts need no reload step: pending tasks are simply still in the DB;
//   - misfires (engine down at trigger time) run if still within the grace
//     window, otherwise the task is marked FAILED;
//   - a second loop polls Kuscia for RUNNING/STOPPING schedule tasks and
//     writes the status back to project_schedule_job / project_job_task /
//     project_schedule_task (ApplyKusciaJobStatus, also exposed as a hook for
//     the job status sync service).

// ---------------------------------------------------------------------------
// Collaborator contracts
// ---------------------------------------------------------------------------

// ScheduleKusciaClient is the subset of the Kuscia client used by schedules.
// *kuscia.Client satisfies it.
type ScheduleKusciaClient interface {
	CreateJob(ctx context.Context, req *kuscia.CreateJobRequest) (*kuscia.CreateJobResponse, error)
	QueryJob(ctx context.Context, jobID string) (*kuscia.QueryJobResponse, error)
	StopJob(ctx context.Context, jobID string) error
	DeleteJob(ctx context.Context, jobID string) error
	RestartJob(ctx context.Context, jobID string) error
}

// ScheduledRenderRequest asks the graph service to render a graph in
// "scheduled mode" (Java GraphContext.isScheduled): build the ProjectJob and
// the Kuscia CreateJob request exactly like graph/start, but
//   - do NOT submit anything to Kuscia,
//   - do NOT persist project_job / project_job_task rows,
//   - suffix every output URI with "_" + ScheduleExpectStartDate
//     (Java KusciaJobConverter DATA_TABLE_DELIMITER + yyyyMMdd) and use the
//     date for partition rules.
type ScheduledRenderRequest struct {
	ProjectID string
	GraphID   string
	Nodes     []string
	// ScheduleExpectStartDate is the trigger date formatted as yyyyMMdd.
	ScheduleExpectStartDate string
}

// ScheduledRenderResult is the output of a scheduled-mode render.
type ScheduledRenderResult struct {
	// Job is the rendered (unsaved) job; JobID must be unique.
	Job *model.ProjectJobDO
	// Tasks are the rendered (unsaved) tasks of Job; the scheduled service
	// persists them into project_job_task.
	Tasks []model.ProjectJobTaskDO
	// Logs are task logs produced while rendering (platform components that
	// succeed immediately); persisted into project_job_task_log.
	Logs []model.ProjectJobTaskLogDO
	// Request is the Kuscia request to submit at trigger time. Nil means the
	// job has nothing to submit (Java REQUEST_IS_NULL).
	Request *kuscia.CreateJobRequest
}

// ScheduledGraphRenderer renders a graph in scheduled mode. The default
// implementation adapts GraphService.RunGraphJob (StartGraphOptions.Scheduled);
// SetGraphRenderer overrides it (tests).
type ScheduledGraphRenderer interface {
	RenderScheduledJob(ctx context.Context, req *ScheduledRenderRequest) (*ScheduledRenderResult, error)
}

// graphServiceRenderer adapts GraphService.RunGraphJob in scheduled mode
// (Java GraphContext.setIsScheduled(true) + graphService.startGraph).
type graphServiceRenderer struct{ g *GraphService }

func (r graphServiceRenderer) RenderScheduledJob(ctx context.Context, req *ScheduledRenderRequest) (*ScheduledRenderResult, error) {
	res, err := r.g.RunGraphJob(ctx, &StartGraphRequest{
		ProjectID: req.ProjectID,
		GraphID:   req.GraphID,
		Nodes:     req.Nodes,
	}, StartGraphOptions{Scheduled: true, ScheduleExpectStartDate: req.ScheduleExpectStartDate})
	if err != nil {
		return nil, err
	}
	if res == nil {
		return nil, nil
	}
	return &ScheduledRenderResult{Job: res.Job, Tasks: res.Tasks, Logs: res.Logs, Request: res.Request}, nil
}

// ScheduleJobStatusHook lets the job status sync service hand Kuscia status of
// jobs that are not in project_job to the scheduled service. handled=false
// means jobID is not a schedule job.
type ScheduleJobStatusHook interface {
	ApplyKusciaJobStatus(ctx context.Context, jobID string, status *kuscia.JobExecutionStatus) (handled bool, err error)
}

var (
	scheduleHookMu sync.RWMutex
	scheduleHook   ScheduleJobStatusHook
)

// RegisterScheduleJobStatusHook registers the schedule job status hook (done
// in wire.go).
func RegisterScheduleJobStatusHook(h ScheduleJobStatusHook) {
	scheduleHookMu.Lock()
	scheduleHook = h
	scheduleHookMu.Unlock()
}

// CurrentScheduleJobStatusHook returns the registered hook or nil. The job
// status sync service may call it for Kuscia jobs unknown to project_job.
func CurrentScheduleJobStatusHook() ScheduleJobStatusHook {
	scheduleHookMu.RLock()
	defer scheduleHookMu.RUnlock()
	return scheduleHook
}

// ---------------------------------------------------------------------------
// Errors
// ---------------------------------------------------------------------------

// ScheduledError carries a Java-compatible error code and detail message.
type ScheduledError struct {
	Code *errcode.ErrorCode
	Msg  string
}

func (e *ScheduledError) Error() string {
	if e.Msg == "" {
		return e.Code.Message
	}
	return e.Code.Message + ": " + e.Msg
}

// Is lets errors.Is(err, errcode.X) match on the numeric code.
func (e *ScheduledError) Is(target error) bool {
	var ec *errcode.ErrorCode
	if errors.As(target, &ec) && e.Code != nil {
		return ec.Code == e.Code.Code
	}
	if t, ok := target.(*ScheduledError); ok && t.Code != nil && e.Code != nil {
		return t.Code.Code == e.Code.Code
	}
	return false
}

func schedErr(code *errcode.ErrorCode, format string, args ...any) error {
	return &ScheduledError{Code: code, Msg: fmt.Sprintf(format, args...)}
}

// Common scheduled errors (kept for callers comparing with errors.Is).
var (
	ErrScheduledNotFound = &ScheduledError{Code: errcode.ScheduledScheduleNotExist}
	ErrInvalidCron       = errors.New("invalid cron expression")
)

// ---------------------------------------------------------------------------
// Service
// ---------------------------------------------------------------------------

// Scheduled constants (Java ScheduledConstants).
const (
	ScheduledRestartTypeAll    = "0"
	ScheduledRestartTypeFailed = "1"
	scheduleIDDelimiter        = "-"
	scheduleRestartMaxAge      = 30 * 24 * time.Hour
	scheduleTimeLayout         = "2006-01-02 15:04:05"
)

// ScheduledOptions configures the scheduled engine.
type ScheduledOptions struct {
	// Location used for cron config and trigger times (default time.Local).
	Location *time.Location
	// PollInterval is how often due tasks are checked (default 5s).
	PollInterval time.Duration
	// SyncInterval is how often running schedule jobs are synced (default 10s).
	SyncInterval time.Duration
	// MisfireGrace: a task whose trigger time passed by more than this is
	// marked FAILED instead of being run late (default 10m).
	MisfireGrace time.Duration
	// Now overrides the clock (tests).
	Now func() time.Time
}

// ScheduledOptionsFromEnv reads optional engine tuning from the environment:
// PRIVAHUB_SCHEDULE_MISFIRE_GRACE, PRIVAHUB_SCHEDULE_POLL_INTERVAL and
// PRIVAHUB_SCHEDULE_SYNC_INTERVAL (Go durations, e.g. "15m", "5s").
func ScheduledOptionsFromEnv() ScheduledOptions {
	var o ScheduledOptions
	parse := func(key string) time.Duration {
		if v := strings.TrimSpace(os.Getenv(key)); v != "" {
			if d, err := time.ParseDuration(v); err == nil && d > 0 {
				return d
			}
		}
		return 0
	}
	o.MisfireGrace = parse("PRIVAHUB_SCHEDULE_MISFIRE_GRACE")
	o.PollInterval = parse("PRIVAHUB_SCHEDULE_POLL_INTERVAL")
	o.SyncInterval = parse("PRIVAHUB_SCHEDULE_SYNC_INTERVAL")
	return o
}

func (o *ScheduledOptions) withDefaults() {
	if o.Location == nil {
		o.Location = time.Local
	}
	if o.PollInterval <= 0 {
		o.PollInterval = 5 * time.Second
	}
	if o.SyncInterval <= 0 {
		o.SyncInterval = 10 * time.Second
	}
	if o.MisfireGrace <= 0 {
		o.MisfireGrace = 10 * time.Minute
	}
	if o.Now == nil {
		o.Now = time.Now
	}
}

// ScheduledService implements /api/v1alpha1/scheduled/* and the engine.
type ScheduledService struct {
	db       *gorm.DB
	log      *zap.Logger
	renderer ScheduledGraphRenderer
	kuscia   ScheduleKusciaClient
	opts     ScheduledOptions

	startOnce sync.Once
	stopOnce  sync.Once
	stopCh    chan struct{}
	wg        sync.WaitGroup
}

// NewScheduledService creates the service. The engine is started by Start.
// graphService is used as renderer when it implements ScheduledGraphRenderer.
func NewScheduledService(db *gorm.DB, log *zap.Logger, graphService *GraphService, kusciaClient *kuscia.Client, opts ScheduledOptions) *ScheduledService {
	opts.withDefaults()
	if log == nil {
		log = zap.NewNop()
	}
	s := &ScheduledService{db: db, log: log, opts: opts, stopCh: make(chan struct{})}
	if graphService != nil {
		s.renderer = graphServiceRenderer{g: graphService}
	}
	if kusciaClient != nil {
		s.kuscia = kusciaClient
	}
	return s
}

// SetGraphRenderer overrides the scheduled-mode graph renderer.
func (s *ScheduledService) SetGraphRenderer(r ScheduledGraphRenderer) { s.renderer = r }

// SetKusciaClient overrides the Kuscia client (tests).
func (s *ScheduledService) SetKusciaClient(k ScheduleKusciaClient) { s.kuscia = k }

// Start launches the fire and sync loops.
func (s *ScheduledService) Start() {
	s.startOnce.Do(func() {
		s.wg.Add(2)
		go s.loop(s.opts.PollInterval, func(ctx context.Context) {
			if _, err := s.RunDue(ctx, s.opts.Now()); err != nil {
				s.log.Warn("scheduled engine: run due tasks failed", zap.Error(err))
			}
		})
		go s.loop(s.opts.SyncInterval, func(ctx context.Context) {
			if err := s.SyncRunning(ctx); err != nil {
				s.log.Warn("scheduled engine: sync running tasks failed", zap.Error(err))
			}
		})
	})
}

// Stop stops the engine loops and waits for them to exit. Safe to call twice.
func (s *ScheduledService) Stop() {
	s.stopOnce.Do(func() { close(s.stopCh) })
	s.wg.Wait()
}

func (s *ScheduledService) loop(interval time.Duration, fn func(ctx context.Context)) {
	defer s.wg.Done()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), interval*6)
			func() {
				defer func() {
					if r := recover(); r != nil {
						s.log.Error("scheduled engine: panic", zap.Any("panic", r))
					}
				}()
				fn(ctx)
			}()
			cancel()
		}
	}
}

// ---------------------------------------------------------------------------
// DTOs (frontend contract, camelCase; the case middleware adds snake twins)
// ---------------------------------------------------------------------------

// ScheduledIdRequest is the /scheduled/id request.
type ScheduledIdRequest struct {
	ProjectID string `json:"projectId"`
	GraphID   string `json:"graphId"`
}

// ScheduledGraphOnceSuccessRequest is the /scheduled/graph/once/success request.
type ScheduledGraphOnceSuccessRequest struct {
	ProjectID string `json:"projectId"`
	GraphID   string `json:"graphId"`
}

// ScheduledGraphCreateRequest is the /scheduled/graph/create request (Java
// ScheduledGraphCreateRequest).
type ScheduledGraphCreateRequest struct {
	ScheduleID   string          `json:"scheduleId"`
	ScheduleDesc string          `json:"scheduleDesc"`
	Cron         cronutil.Config `json:"cron"`
	ProjectID    string          `json:"projectId"`
	GraphID      string          `json:"graphId"`
	Nodes        []string        `json:"nodes"`
}

// PageScheduledRequest is the /scheduled/page request.
type PageScheduledRequest struct {
	ProjectID string            `json:"projectId"`
	Search    string            `json:"search"`
	Status    string            `json:"status"`
	Page      int               `json:"page"`
	Size      int               `json:"size"`
	Sort      map[string]string `json:"sort"`
}

// PageScheduledVO is one /scheduled/page row.
type PageScheduledVO struct {
	ScheduleID    string `json:"scheduleId"`
	ScheduleDesc  string `json:"scheduleDesc"`
	ScheduleStats string `json:"scheduleStats"`
	Creator       string `json:"creator"`
	CreateTime    string `json:"createTime"`
	TaskRunning   bool   `json:"taskRunning"`
	Owner         string `json:"owner"`
	OwnerName     string `json:"ownerName"`
}

// ScheduledPage is Java SecretPadPageResponse.
type ScheduledPage[T any] struct {
	List  []T   `json:"list"`
	Total int64 `json:"total"`
}

// ScheduledOfflineRequest / ScheduledDelRequest / ScheduledInfoRequest.
type ScheduledOfflineRequest struct {
	ScheduleID string `json:"scheduleId"`
}

// ScheduledDelRequest is the /scheduled/del request.
type ScheduledDelRequest struct {
	ScheduleID string `json:"scheduleId"`
}

// ScheduledInfoRequest is the /scheduled/info request.
type ScheduledInfoRequest struct {
	ScheduleID string `json:"scheduleId"`
}

// TaskPageScheduledRequest is the /scheduled/task/page request.
type TaskPageScheduledRequest struct {
	ScheduleID string            `json:"scheduleId"`
	Search     string            `json:"search"`
	Page       int               `json:"page"`
	Size       int               `json:"size"`
	Sort       map[string]string `json:"sort"`
}

// TaskPageScheduledVO is one /scheduled/task/page row.
type TaskPageScheduledVO struct {
	ScheduleTaskID              string `json:"scheduleTaskId"`
	ScheduleTaskExpectStartTime string `json:"scheduleTaskExpectStartTime"`
	ScheduleTaskStartTime       string `json:"scheduleTaskStartTime,omitempty"`
	ScheduleTaskEndTime         string `json:"scheduleTaskEndTime,omitempty"`
	ScheduleTaskStatus          string `json:"scheduleTaskStatus"`
	AllReRun                    bool   `json:"allReRun"`
}

// TaskStopScheduledRequest is the /scheduled/task/stop request.
type TaskStopScheduledRequest struct {
	ScheduleID     string `json:"scheduleId"`
	ScheduleTaskID string `json:"scheduleTaskId"`
}

// TaskReRunScheduledRequest is the /scheduled/task/rerun request; Type "0"
// (default) reruns everything, "1" restarts failed tasks.
type TaskReRunScheduledRequest struct {
	ScheduleID     string `json:"scheduleId"`
	ScheduleTaskID string `json:"scheduleTaskId"`
	Type           string `json:"type"`
}

// TaskInfoScheduledRequest is the /scheduled/task/info request.
type TaskInfoScheduledRequest struct {
	ScheduleID     string `json:"scheduleId"`
	ScheduleTaskID string `json:"scheduleTaskId"`
}

// ScheduleListProjectJobRequest is the /scheduled/job/list request.
type ScheduleListProjectJobRequest struct {
	ProjectID      string `json:"projectId"`
	GraphID        string `json:"graphId"`
	ScheduleTaskID string `json:"scheduleTaskId"`
	PageNum        int    `json:"pageNum"`
	PageSize       int    `json:"pageSize"`
}

// ScheduledJobSummaryVO is Java ProjectJobSummaryVO.
type ScheduledJobSummaryVO struct {
	JobID             string `json:"jobId"`
	Status            string `json:"status"`
	ErrMsg            string `json:"errMsg,omitempty"`
	GmtCreate         string `json:"gmtCreate"`
	GmtModified       string `json:"gmtModified"`
	GmtFinished       string `json:"gmtFinished,omitempty"`
	TableCount        int64  `json:"tableCount"`
	ModelCount        int64  `json:"modelCount"`
	RuleCount         int64  `json:"ruleCount"`
	ReportCount       int64  `json:"reportCount"`
	FinishedTaskCount int64  `json:"finishedTaskCount"`
	TaskCount         int64  `json:"taskCount"`
}

// ScheduledJobPage is Java PageResponse; List duplicates Data for the
// current frontend client which reads `list`.
type ScheduledJobPage struct {
	PageTotal int                     `json:"pageTotal"`
	PageSize  int                     `json:"pageSize"`
	Total     int64                   `json:"total"`
	Data      []ScheduledJobSummaryVO `json:"data"`
	List      []ScheduledJobSummaryVO `json:"list"`
}

// ScheduledJobVO is Java ProjectJobVO (job + graph detail with task status).
type ScheduledJobVO struct {
	JobID       string                  `json:"jobId"`
	Status      string                  `json:"status"`
	ErrMsg      string                  `json:"errMsg,omitempty"`
	GmtCreate   string                  `json:"gmtCreate,omitempty"`
	GmtModified string                  `json:"gmtModified,omitempty"`
	GmtFinished string                  `json:"gmtFinished,omitempty"`
	Finished    bool                    `json:"finished"`
	Graph       *ScheduledGraphDetailVO `json:"graph,omitempty"`
}

// ScheduledGraphDetailVO is Java GraphDetailVO.
type ScheduledGraphDetailVO struct {
	ProjectID string                 `json:"projectId"`
	GraphID   string                 `json:"graphId"`
	Name      string                 `json:"name"`
	Edges     json.RawMessage        `json:"edges"`
	Nodes     []ScheduledGraphNodeVO `json:"nodes"`
}

// ScheduledGraphNodeVO is Java GraphNodeDetail.
type ScheduledGraphNodeVO struct {
	GraphNodeID string           `json:"graphNodeId"`
	CodeName    string           `json:"codeName"`
	Label       string           `json:"label"`
	X           int              `json:"x"`
	Y           int              `json:"y"`
	Inputs      []string         `json:"inputs"`
	Outputs     []string         `json:"outputs"`
	NodeDef     json.RawMessage  `json:"nodeDef"`
	Status      string           `json:"status,omitempty"`
	JobID       string           `json:"jobId,omitempty"`
	TaskID      string           `json:"taskId,omitempty"`
	Parties     []map[string]any `json:"parties,omitempty"`
	Results     []map[string]any `json:"results,omitempty"`
	Progress    *float64         `json:"progress,omitempty"`
}

// snapshots persisted in project_schedule.graph_info / job_info.
type scheduleGraphSnapshot struct {
	Graph model.ProjectGraphDO       `json:"graph"`
	Nodes []model.ProjectGraphNodeDO `json:"nodes"`
}

type scheduleJobSnapshot struct {
	Job   model.ProjectJobDO       `json:"job"`
	Tasks []model.ProjectJobTaskDO `json:"tasks"`
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

type scheduleUser struct {
	OwnerID string
	Name    string
}

func scheduleUserFrom(ctx context.Context) scheduleUser {
	if c, ok := auth.ClaimsFromContext(ctx); ok && c != nil {
		return scheduleUser{OwnerID: c.OwnerID, Name: c.Username}
	}
	u := scheduleUser{}
	if name, ok := auth.UsernameFromContext(ctx); ok {
		u.Name = name
	}
	return u
}

func checkScheduleOwner(ctx context.Context, owner string) error {
	if scheduleUserFrom(ctx).OwnerID != owner {
		return &ScheduledError{Code: errcode.ScheduledUserNotOwner}
	}
	return nil
}

const scheduleIDLetters = "abcdefghijklmnopqrstuvwxyz"

// buildScheduleID mirrors Java: 4 random lowercase letters + "-" + yyyyMMddHHmmss.
func buildScheduleID(now time.Time) string {
	b := make([]byte, 4)
	for i := range b {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(scheduleIDLetters))))
		if err != nil {
			b[i] = 'x'
			continue
		}
		b[i] = scheduleIDLetters[n.Int64()]
	}
	return string(b) + scheduleIDDelimiter + now.Format("20060102150405")
}

// fmtLocal formats t as Java DateTimes.toLocalDateTimeString in the engine location.
func (s *ScheduledService) fmtLocal(t *time.Time) string {
	if t == nil || t.IsZero() {
		return ""
	}
	return t.In(s.opts.Location).Format(scheduleTimeLayout)
}

// fmtRFC3339 formats t as Java DateTimes.toRfc3339 in the engine location.
func (s *ScheduledService) fmtRFC3339(t *time.Time) string {
	if t == nil || t.IsZero() {
		return ""
	}
	return t.In(s.opts.Location).Format(time.RFC3339)
}

func jsonArrayOfStrings(raw string) []string {
	if raw == "" {
		return []string{}
	}
	var out []string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return []string{}
	}
	return out
}

func jsonRawOr(raw, fallback string) json.RawMessage {
	if raw == "" || !json.Valid([]byte(raw)) {
		return json.RawMessage(fallback)
	}
	return json.RawMessage(raw)
}

func isScheduleJobFinished(status string) bool {
	switch status {
	case model.ScheduleJobStatusSucceed, model.ScheduleJobStatusFailed, model.ScheduleJobStatusStopped:
		return true
	}
	return false
}

func isScheduleTaskTerminal(status string) bool {
	switch status {
	case model.ScheduledStatusSucceed, model.ScheduledStatusFailed, model.ScheduledStatusStopped:
		return true
	}
	return false
}

// mapKusciaJobStateToScheduleJob maps a Kuscia job state to Java GraphJobStatus.
func mapKusciaJobStateToScheduleJob(state string) string {
	switch strings.ToLower(state) {
	case "succeeded":
		return model.ScheduleJobStatusSucceed
	case "failed", "approvalreject", "cancelled", "canceled":
		return model.ScheduleJobStatusFailed
	case "stopped":
		return model.ScheduleJobStatusStopped
	default:
		return model.ScheduleJobStatusRunning
	}
}

// mapKusciaTaskStateToNodeStatus maps a Kuscia task state to Java GraphNodeTaskStatus.
func mapKusciaTaskStateToNodeStatus(state string) string {
	switch strings.ToLower(state) {
	case "succeeded":
		return "SUCCEED"
	case "failed":
		return "FAILED"
	case "running":
		return "RUNNING"
	case "stopped":
		return "STOPPED"
	default:
		return "INITIALIZED"
	}
}

func normPage(page, size, defSize int) (int, int) {
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = defSize
	}
	if size > 1000 {
		size = 1000
	}
	return page, size
}

func (s *ScheduledService) now() time.Time { return s.opts.Now().In(s.opts.Location) }

func (s *ScheduledService) checkProjectAndGraph(ctx context.Context, projectID, graphID string) (*model.ProjectGraphDO, error) {
	if projectID == "" || graphID == "" {
		return nil, schedErr(errcode.ParamError, "projectId and graphId are required")
	}
	var cnt int64
	if err := s.db.WithContext(ctx).Model(&model.ProjectDO{}).Where("project_id = ?", projectID).Count(&cnt).Error; err != nil {
		return nil, err
	}
	if cnt == 0 {
		return nil, &ScheduledError{Code: errcode.ProjectNotFound, Msg: projectID}
	}
	var g model.ProjectGraphDO
	if err := s.db.WithContext(ctx).Where("project_id = ? AND graph_id = ?", projectID, graphID).First(&g).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, &ScheduledError{Code: errcode.GraphNotFound, Msg: graphID}
		}
		return nil, err
	}
	return &g, nil
}

// checkProjectJobExistsAndOnceSuccess returns the graph's latest job, requiring
// at least one successful run (Java checkProjectJobExitsAndOnceSuccess).
func (s *ScheduledService) checkProjectJobExistsAndOnceSuccess(ctx context.Context, projectID, graphID string) (*model.ProjectJobDO, error) {
	var job model.ProjectJobDO
	err := s.db.WithContext(ctx).Where("project_id = ? AND graph_id = ?", projectID, graphID).Order("id DESC").First(&job).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, &ScheduledError{Code: errcode.ScheduledProjectJobNotExist}
		}
		return nil, err
	}
	var cnt int64
	if err := s.db.WithContext(ctx).Model(&model.ProjectJobDO{}).
		Where("project_id = ? AND graph_id = ? AND status IN ?", projectID, graphID, []string{"SUCCEED", "SUCCEEDED", "SUCCESS"}).
		Count(&cnt).Error; err != nil {
		return nil, err
	}
	if cnt == 0 {
		return nil, &ScheduledError{Code: errcode.ScheduledProjectJobNeedSuccessOnce}
	}
	return &job, nil
}

func (s *ScheduledService) findSchedule(ctx context.Context, scheduleID string) (*model.ProjectScheduleDO, error) {
	var sc model.ProjectScheduleDO
	if err := s.db.WithContext(ctx).Where("schedule_id = ?", scheduleID).First(&sc).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, &ScheduledError{Code: errcode.ScheduledScheduleNotExist, Msg: scheduleID}
		}
		return nil, err
	}
	return &sc, nil
}

func (s *ScheduledService) findTask(ctx context.Context, scheduleID, scheduleTaskID string) (*model.ProjectScheduleTaskDO, error) {
	var t model.ProjectScheduleTaskDO
	if err := s.db.WithContext(ctx).Where("schedule_id = ? AND schedule_task_id = ?", scheduleID, scheduleTaskID).First(&t).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, &ScheduledError{Code: errcode.ScheduledScheduleTaskNotExist, Msg: scheduleTaskID}
		}
		return nil, err
	}
	return &t, nil
}

func (s *ScheduledService) haveTaskRunning(ctx context.Context, scheduleID string) (bool, error) {
	var cnt int64
	err := s.db.WithContext(ctx).Model(&model.ProjectScheduleTaskDO{}).
		Where("schedule_id = ? AND status IN ?", scheduleID, []string{model.ScheduledStatusRunning, model.ScheduledStatusStopping}).Count(&cnt).Error
	return cnt > 0, err
}

// schedulePageOrder accepts only mapped columns and literal SQL directions.
// Never pass the client's sort keys/directions directly into GORM Order.
// Twin keys from the compatibility middleware are intentionally ignored; the
// canonical camelCase keys are retained, and id provides stable page boundaries.
func schedulePageOrder(q *gorm.DB, requested, columns map[string]string, fallback string) *gorm.DB {
	ordered := false
	for _, key := range sortedMapKeys(requested) {
		column, ok := columns[key]
		direction := strings.ToUpper(requested[key])
		if !ok || (direction != "ASC" && direction != "DESC") {
			continue
		}
		q = q.Order(column + " " + direction)
		ordered = true
	}
	if !ordered {
		q = q.Order(fallback)
	}
	return q.Order("id DESC")
}

func encodeJobRequest(req *kuscia.CreateJobRequest) (string, error) {
	data, err := json.Marshal(req)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(data), nil
}

func decodeJobRequest(s string) (*kuscia.CreateJobRequest, error) {
	if s == "" {
		return nil, errors.New("empty job request")
	}
	data, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return nil, err
	}
	var req kuscia.CreateJobRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, err
	}
	return &req, nil
}

// ---------------------------------------------------------------------------
// API: id / once success / create
// ---------------------------------------------------------------------------

// BuildSchedulerID implements /scheduled/id: a brand new schedule id.
func (s *ScheduledService) BuildSchedulerID(ctx context.Context, req *ScheduledIdRequest) (string, error) {
	if _, err := s.checkProjectAndGraph(ctx, req.ProjectID, req.GraphID); err != nil {
		return "", err
	}
	return buildScheduleID(s.now()), nil
}

// OnceSuccess implements /scheduled/graph/once/success.
func (s *ScheduledService) OnceSuccess(ctx context.Context, req *ScheduledGraphOnceSuccessRequest) (bool, error) {
	if _, err := s.checkProjectAndGraph(ctx, req.ProjectID, req.GraphID); err != nil {
		return false, err
	}
	if _, err := s.checkProjectJobExistsAndOnceSuccess(ctx, req.ProjectID, req.GraphID); err != nil {
		var se *ScheduledError
		if errors.As(err, &se) && (se.Code == errcode.ScheduledProjectJobNeedSuccessOnce || se.Code == errcode.ScheduledProjectJobNotExist) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

type renderedTrigger struct {
	trigger cronutil.Trigger
	taskID  string
	result  *ScheduledRenderResult
	request string
}

// CreateScheduler implements /scheduled/graph/create.
func (s *ScheduledService) CreateScheduler(ctx context.Context, req *ScheduledGraphCreateRequest) error {
	if len(req.Nodes) == 0 {
		return schedErr(errcode.ParamError, "nodes must not be empty")
	}
	if len(req.ScheduleDesc) > 200 {
		return schedErr(errcode.ParamError, "scheduleDesc length must be less than or equal to 200")
	}
	c := req.Cron
	if c.StartTime == "" || c.EndTime == "" || c.ScheduleCycle == "" || c.ScheduleTime == "" {
		return schedErr(errcode.ParamError, "cron.startTime, cron.endTime, cron.scheduleCycle and cron.scheduleTime are required")
	}
	graph, err := s.checkProjectAndGraph(ctx, req.ProjectID, req.GraphID)
	if err != nil {
		return err
	}
	job, err := s.checkProjectJobExistsAndOnceSuccess(ctx, req.ProjectID, req.GraphID)
	if err != nil {
		return err
	}
	now := s.now()
	if req.ScheduleID == "" {
		req.ScheduleID = buildScheduleID(now)
	}
	if len(req.ScheduleID) > 20 {
		return schedErr(errcode.ParamError, "scheduleId length must be less than or equal to 20")
	}
	var exists int64
	if err := s.db.WithContext(ctx).Unscoped().Model(&model.ProjectScheduleDO{}).
		Where("schedule_id = ?", req.ScheduleID).Count(&exists).Error; err != nil {
		return err
	}
	if exists > 0 {
		return schedErr(errcode.ScheduledScheduleCreateError, "schedule already exists")
	}

	exprs, triggers, err := cronutil.Expand(c, now, s.opts.Location)
	if err != nil {
		return schedErr(errcode.ScheduledScheduleCreateError, "%v", err)
	}
	if len(triggers) == 0 {
		return schedErr(errcode.ScheduledScheduleCreateError, "no scheduling dates available")
	}
	if s.renderer == nil {
		return schedErr(errcode.ScheduledScheduleCreateError, "graph scheduled-mode render is not available")
	}

	user := scheduleUserFrom(ctx)

	// Snapshots of the graph and of its latest job.
	var nodes []model.ProjectGraphNodeDO
	if err := s.db.WithContext(ctx).Where("project_id = ? AND graph_id = ?", req.ProjectID, req.GraphID).Find(&nodes).Error; err != nil {
		return err
	}
	var jobTasks []model.ProjectJobTaskDO
	if err := s.db.WithContext(ctx).Where("project_id = ? AND job_id = ?", job.ProjectID, job.JobID).Find(&jobTasks).Error; err != nil {
		return err
	}
	graphInfo, _ := json.Marshal(scheduleGraphSnapshot{Graph: *graph, Nodes: nodes})
	jobInfo, _ := json.Marshal(scheduleJobSnapshot{Job: *job, Tasks: jobTasks})
	cronJSON, _ := json.Marshal(exprs)
	reqJSON, _ := json.Marshal(req)

	// Render every trigger first (outside the DB transaction: the renderer
	// reads the DB on its own connection).
	usedIDs := map[string]bool{}
	rendered := make([]renderedTrigger, 0, len(triggers))
	for _, tr := range triggers {
		res, rErr := s.renderer.RenderScheduledJob(ctx, &ScheduledRenderRequest{
			ProjectID:               req.ProjectID,
			GraphID:                 req.GraphID,
			Nodes:                   req.Nodes,
			ScheduleExpectStartDate: tr.Time.Format("20060102"),
		})
		if rErr != nil {
			return schedErr(errcode.ScheduledScheduleCreateError, "render graph for %s: %v", tr.Time.Format(scheduleTimeLayout), rErr)
		}
		if res == nil || res.Job == nil {
			return &ScheduledError{Code: errcode.ScheduledProjectJobNotExist}
		}
		if res.Request == nil {
			return &ScheduledError{Code: errcode.ScheduledRequestIsNull}
		}
		encoded, eErr := encodeJobRequest(res.Request)
		if eErr != nil {
			return schedErr(errcode.ScheduledScheduleCreateError, "encode job request: %v", eErr)
		}
		taskID := buildScheduleID(now)
		for usedIDs[taskID] {
			taskID = buildScheduleID(now)
		}
		usedIDs[taskID] = true
		rendered = append(rendered, renderedTrigger{trigger: tr, taskID: taskID, result: res, request: encoded})
	}

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		sc := &model.ProjectScheduleDO{
			ScheduleID:   req.ScheduleID,
			ScheduleDesc: req.ScheduleDesc,
			Cron:         string(cronJSON),
			Request:      string(reqJSON),
			GraphInfo:    string(graphInfo),
			GraphJobID:   job.JobID,
			JobInfo:      string(jobInfo),
			Owner:        user.OwnerID,
			Creator:      user.Name,
			Status:       model.ScheduledStatusUp,
			ProjectID:    req.ProjectID,
			GraphID:      req.GraphID,
			CreateTime:   now,
		}
		if err := tx.Create(sc).Error; err != nil {
			return err
		}
		for _, r := range rendered {
			j := r.result.Job
			status := j.Status
			if status == "" || !isScheduleJobFinished(status) {
				status = model.ScheduleJobStatusRunning
			}
			sj := &model.ProjectScheduleJobDO{
				ProjectID:      req.ProjectID,
				JobID:          j.JobID,
				Name:           j.Name,
				Status:         status,
				GraphID:        req.GraphID,
				Edges:          j.Edges,
				Owner:          user.OwnerID,
				ScheduleTaskID: r.taskID,
			}
			if sj.Name == "" {
				sj.Name = graph.Name
			}
			if err := tx.Create(sj).Error; err != nil {
				return err
			}
			for i := range r.result.Tasks {
				t := r.result.Tasks[i]
				t.ID = 0
				t.ProjectID = req.ProjectID
				t.JobID = j.JobID
				if err := tx.Create(&t).Error; err != nil {
					return err
				}
			}
			for i := range r.result.Logs {
				l := r.result.Logs[i]
				l.ID = 0
				if err := tx.Create(&l).Error; err != nil {
					return err
				}
			}
			task := &model.ProjectScheduleTaskDO{
				ProjectID:               req.ProjectID,
				GraphID:                 req.GraphID,
				ScheduleJobID:           j.JobID,
				ScheduleID:              req.ScheduleID,
				ScheduleTaskID:          r.taskID,
				Cron:                    r.trigger.Cron,
				ScheduleTaskExpectStart: r.trigger.Time.In(s.opts.Location),
				Status:                  model.ScheduledStatusToBeRun,
				Owner:                   user.OwnerID,
				Creator:                 user.Name,
				JobRequest:              r.request,
			}
			if err := tx.Create(task).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// ---------------------------------------------------------------------------
// API: page / offline / del / info
// ---------------------------------------------------------------------------

// QueryPage implements /scheduled/page.
func (s *ScheduledService) QueryPage(ctx context.Context, req *PageScheduledRequest) (*ScheduledPage[PageScheduledVO], error) {
	if req.ProjectID == "" {
		return nil, schedErr(errcode.ParamError, "projectId is required")
	}
	page, size := normPage(req.Page, req.Size, 10)
	q := s.db.WithContext(ctx).Model(&model.ProjectScheduleDO{}).
		Where("project_id = ? AND owner = ?", req.ProjectID, scheduleUserFrom(ctx).OwnerID)
	if req.Search != "" {
		q = q.Where("schedule_id LIKE ? ESCAPE '!'", sqlutil.Contains(req.Search))
	}
	if req.Status != "" {
		q = q.Where("status = ?", req.Status)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, err
	}
	var rows []model.ProjectScheduleDO
	if err := schedulePageOrder(q, req.Sort, map[string]string{"createTime": "create_time"}, "gmt_create DESC").
		Offset((page - 1) * size).Limit(size).Find(&rows).Error; err != nil {
		return nil, err
	}
	// Resolve running flags in one query rather than one COUNT per schedule.
	// Both RUNNING and STOPPING are active: a stop acknowledgement is not a
	// terminal Kuscia status, so the plan cannot be safely offlined yet.
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ScheduleID)
	}
	var runningIDs []string
	if len(ids) > 0 {
		if err := s.db.WithContext(ctx).Model(&model.ProjectScheduleTaskDO{}).
			Where("schedule_id IN ? AND status IN ?", ids, []string{model.ScheduledStatusRunning, model.ScheduledStatusStopping}).
			Distinct("schedule_id").Pluck("schedule_id", &runningIDs).Error; err != nil {
			return nil, err
		}
	}
	runningSet := make(map[string]bool, len(runningIDs))
	for _, id := range runningIDs {
		runningSet[id] = true
	}
	ownerName := s.ownerName(ctx, scheduleUserFrom(ctx).OwnerID)
	list := make([]PageScheduledVO, 0, len(rows))
	for i := range rows {
		r := &rows[i]
		ct := r.CreateTime
		list = append(list, PageScheduledVO{
			ScheduleID:    r.ScheduleID,
			ScheduleDesc:  r.ScheduleDesc,
			ScheduleStats: r.Status,
			Creator:       r.Creator,
			CreateTime:    s.fmtLocal(&ct),
			TaskRunning:   runningSet[r.ScheduleID],
			Owner:         r.Owner,
			OwnerName:     ownerName,
		})
	}
	return &ScheduledPage[PageScheduledVO]{List: list, Total: total}, nil
}

// ownerName resolves the current user's owner name (Java UserContext ownerName).
func (s *ScheduledService) ownerName(ctx context.Context, ownerID string) string {
	if ownerID == "" {
		return ""
	}
	var node model.NodeDO
	if err := s.db.WithContext(ctx).Where("node_id = ?", ownerID).First(&node).Error; err == nil && node.Name != "" {
		return node.Name
	}
	var inst model.InstDO
	if err := s.db.WithContext(ctx).Where("inst_id = ?", ownerID).First(&inst).Error; err == nil && inst.Name != "" {
		return inst.Name
	}
	return ownerID
}

// Offline implements /scheduled/offline.
func (s *ScheduledService) Offline(ctx context.Context, req *ScheduledOfflineRequest) error {
	sc, err := s.findSchedule(ctx, req.ScheduleID)
	if err != nil {
		return err
	}
	if err := checkScheduleOwner(ctx, sc.Owner); err != nil {
		return err
	}
	running, err := s.haveTaskRunning(ctx, req.ScheduleID)
	if err != nil {
		return err
	}
	if running {
		return &ScheduledError{Code: errcode.ScheduledScheduleRunningNotOffline}
	}
	// Setting DOWN pauses the schedule: the engine only fires tasks of UP schedules.
	return s.db.WithContext(ctx).Model(&model.ProjectScheduleDO{}).
		Where("schedule_id = ?", req.ScheduleID).Update("status", model.ScheduledStatusDown).Error
}

// Del implements /scheduled/del (soft delete).
func (s *ScheduledService) Del(ctx context.Context, req *ScheduledDelRequest) error {
	sc, err := s.findSchedule(ctx, req.ScheduleID)
	if err != nil {
		return err
	}
	if err := checkScheduleOwner(ctx, sc.Owner); err != nil {
		return err
	}
	if sc.Status == model.ScheduledStatusUp {
		return &ScheduledError{Code: errcode.ScheduledScheduleUpNotDel}
	}
	return s.db.WithContext(ctx).Model(sc).Updates(map[string]any{"is_deleted": 1, "deleted_at": s.now()}).Error
}

// Info implements /scheduled/info: the graph job snapshot taken at creation.
func (s *ScheduledService) Info(ctx context.Context, req *ScheduledInfoRequest) (*ScheduledJobVO, error) {
	sc, err := s.findSchedule(ctx, req.ScheduleID)
	if err != nil {
		return nil, err
	}
	if err := checkScheduleOwner(ctx, sc.Owner); err != nil {
		return nil, err
	}
	var snap scheduleJobSnapshot
	if sc.JobInfo == "" || json.Unmarshal([]byte(sc.JobInfo), &snap) != nil {
		return nil, &ScheduledError{Code: errcode.ScheduledProjectJobNotExist}
	}
	return s.buildJobVO(ctx, sc, &snap.Job, snap.Tasks)
}

// ---------------------------------------------------------------------------
// API: task page / stop / rerun / info, job list
// ---------------------------------------------------------------------------

// TaskPage implements /scheduled/task/page.
func (s *ScheduledService) TaskPage(ctx context.Context, req *TaskPageScheduledRequest) (*ScheduledPage[TaskPageScheduledVO], error) {
	page, size := normPage(req.Page, req.Size, 10)
	q := s.db.WithContext(ctx).Model(&model.ProjectScheduleTaskDO{}).
		Where("owner = ?", scheduleUserFrom(ctx).OwnerID)
	if req.ScheduleID != "" {
		q = q.Where("schedule_id = ?", req.ScheduleID)
	}
	if req.Search != "" {
		q = q.Where("schedule_task_id LIKE ? ESCAPE '!'", sqlutil.Contains(req.Search))
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, err
	}
	var rows []model.ProjectScheduleTaskDO
	if err := schedulePageOrder(q, req.Sort, map[string]string{
		"scheduleTaskExpectStartTime": "schedule_task_expect_start", "scheduleTaskStartTime": "schedule_task_start_time",
		"scheduleTaskEndTime": "schedule_task_end_time", "createTime": "gmt_create",
	}, "gmt_create DESC, schedule_task_expect_start DESC").Offset((page - 1) * size).Limit(size).Find(&rows).Error; err != nil {
		return nil, err
	}
	list := make([]TaskPageScheduledVO, 0, len(rows))
	for i := range rows {
		t := &rows[i]
		exp := t.ScheduleTaskExpectStart
		list = append(list, TaskPageScheduledVO{
			ScheduleTaskID:              t.ScheduleTaskID,
			ScheduleTaskExpectStartTime: s.fmtLocal(&exp),
			ScheduleTaskStartTime:       s.fmtLocal(t.ScheduleTaskStartTime),
			ScheduleTaskEndTime:         s.fmtLocal(t.ScheduleTaskEndTime),
			ScheduleTaskStatus:          t.Status,
			AllReRun:                    t.AllReRun,
		})
	}
	return &ScheduledPage[TaskPageScheduledVO]{List: list, Total: total}, nil
}

// TaskStop implements /scheduled/task/stop.
//
// Deviations from Java: a TO_BE_RUN task (no Kuscia job yet) goes directly to
// STOPPED, and the rest of the schedule is not paused (Java paused the whole
// Quartz group without marking the schedule DOWN, leaving it unrecoverable).
func (s *ScheduledService) TaskStop(ctx context.Context, req *TaskStopScheduledRequest) error {
	task, err := s.findTask(ctx, req.ScheduleID, req.ScheduleTaskID)
	if err != nil {
		return err
	}
	if err := checkScheduleOwner(ctx, task.Owner); err != nil {
		return err
	}
	now := s.now()
	switch task.Status {
	case model.ScheduledStatusToBeRun:
		res := s.db.WithContext(ctx).Model(&model.ProjectScheduleTaskDO{}).
			Where("schedule_task_id = ? AND status = ?", task.ScheduleTaskID, model.ScheduledStatusToBeRun).
			Updates(map[string]any{"status": model.ScheduledStatusStopped, "schedule_task_end_time": now})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return schedErr(errcode.ScheduledScheduleTaskStatusNotRunning, "task status changed concurrently")
		}
		return s.markScheduleJobStopped(ctx, task, now)
	case model.ScheduledStatusRunning:
		var sj model.ProjectScheduleJobDO
		if err := s.db.WithContext(ctx).Where("job_id = ?", task.ScheduleJobID).First(&sj).Error; err != nil {
			return &ScheduledError{Code: errcode.ScheduledScheduleTaskNotExist, Msg: "schedule job not found"}
		}
		if s.kuscia == nil {
			return schedErr(errcode.KusciaConnError, "kuscia client unavailable")
		}
		if err := s.kuscia.StopJob(ctx, sj.JobID); err != nil {
			return schedErr(errcode.KusciaConnError, "%v", err)
		}
		if err := s.markScheduleJobStopped(ctx, task, now); err != nil {
			return err
		}
		return s.db.WithContext(ctx).Model(&model.ProjectScheduleTaskDO{}).
			Where("schedule_task_id = ?", task.ScheduleTaskID).
			Update("status", model.ScheduledStatusStopping).Error
	default:
		return schedErr(errcode.ScheduledScheduleTaskStatusNotRunning, "%s", task.Status)
	}
}

// markScheduleJobStopped mirrors Java ProjectScheduleJobDO.stop().
func (s *ScheduledService) markScheduleJobStopped(ctx context.Context, task *model.ProjectScheduleTaskDO, now time.Time) error {
	if task.ScheduleJobID == "" {
		return nil
	}
	if err := s.db.WithContext(ctx).Model(&model.ProjectScheduleJobDO{}).
		Where("project_id = ? AND job_id = ?", task.ProjectID, task.ScheduleJobID).
		Updates(map[string]any{"status": model.ScheduleJobStatusStopped, "finished_time": now}).Error; err != nil {
		return err
	}
	return s.db.WithContext(ctx).Model(&model.ProjectJobTaskDO{}).
		Where("project_id = ? AND job_id = ? AND status IN ?", task.ProjectID, task.ScheduleJobID,
			[]string{"RUNNING", "INITIALIZED", "PENDING", "STAGING"}).
		Update("status", "STOPPED").Error
}

// TaskRerun implements /scheduled/task/rerun.
func (s *ScheduledService) TaskRerun(ctx context.Context, req *TaskReRunScheduledRequest) error {
	sc, err := s.findSchedule(ctx, req.ScheduleID)
	if err != nil {
		return err
	}
	if sc.Status == model.ScheduledStatusDown {
		return schedErr(errcode.ScheduledProjectJobRestartError, "schedule is down")
	}
	task, err := s.findTask(ctx, req.ScheduleID, req.ScheduleTaskID)
	if err != nil {
		return err
	}
	if err := checkScheduleOwner(ctx, task.Owner); err != nil {
		return err
	}
	switch task.Status {
	case model.ScheduledStatusToBeRun, model.ScheduledStatusRunning, model.ScheduledStatusStopping:
		return schedErr(errcode.ScheduledProjectJobRestartError, "schedule task status is not finish, now is %s", task.Status)
	}
	if s.kuscia == nil {
		return schedErr(errcode.ScheduledProjectJobRestartError, "kuscia client unavailable")
	}
	typ := req.Type
	if typ == "" {
		typ = ScheduledRestartTypeAll
	}
	now := s.now()
	switch typ {
	case ScheduledRestartTypeAll:
		if task.AllReRun {
			return schedErr(errcode.ScheduledProjectJobRestartError, "schedule task already performed a rerun of all")
		}
		createReq, dErr := decodeJobRequest(task.JobRequest)
		if dErr != nil {
			return schedErr(errcode.ScheduledProjectJobRestartError, "%v", dErr)
		}
		// A task that never reached Kuscia (misfired / stopped before firing)
		// has no job to delete.
		if task.ScheduleTaskStartTime != nil {
			if err := s.kuscia.DeleteJob(ctx, task.ScheduleJobID); err != nil {
				return schedErr(errcode.ScheduledProjectJobRestartError, "%v", err)
			}
		}
		if _, err := s.kuscia.CreateJob(ctx, createReq); err != nil {
			return schedErr(errcode.ScheduledProjectJobRestartError, "%v", err)
		}
		if err := s.db.WithContext(ctx).Where("project_id = ? AND job_id = ?", task.ProjectID, task.ScheduleJobID).
			Delete(&model.ProjectResultDO{}).Error; err != nil {
			return err
		}
		task.AllReRun = true
	case ScheduledRestartTypeFailed:
		if task.ScheduleTaskStartTime != nil && now.After(task.ScheduleTaskStartTime.Add(scheduleRestartMaxAge)) {
			return schedErr(errcode.ScheduledProjectJobRestartError, "scheduleTaskStartTime is more than 30 days")
		}
		if err := s.kuscia.RestartJob(ctx, task.ScheduleJobID); err != nil {
			return schedErr(errcode.ScheduledProjectJobRestartError, "%v", err)
		}
	default:
		return schedErr(errcode.ScheduledProjectJobRestartError, "type is not support")
	}
	if err := s.db.WithContext(ctx).Model(&model.ProjectScheduleTaskDO{}).
		Where("schedule_task_id = ?", task.ScheduleTaskID).
		Updates(map[string]any{
			"status":                   model.ScheduledStatusRunning,
			"schedule_task_start_time": now,
			"schedule_task_end_time":   nil,
			"all_re_run":               task.AllReRun,
		}).Error; err != nil {
		return err
	}
	return s.db.WithContext(ctx).Model(&model.ProjectScheduleJobDO{}).
		Where("project_id = ? AND job_id = ?", task.ProjectID, task.ScheduleJobID).
		Updates(map[string]any{"status": model.ScheduleJobStatusRunning, "finished_time": nil, "err_msg": ""}).Error
}

// TaskInfo implements /scheduled/task/info.
func (s *ScheduledService) TaskInfo(ctx context.Context, req *TaskInfoScheduledRequest) (*ScheduledJobVO, error) {
	sc, err := s.findSchedule(ctx, req.ScheduleID)
	if err != nil {
		return nil, err
	}
	task, err := s.findTask(ctx, req.ScheduleID, req.ScheduleTaskID)
	if err != nil {
		return nil, err
	}
	if err := checkScheduleOwner(ctx, task.Owner); err != nil {
		return nil, err
	}
	var sj model.ProjectScheduleJobDO
	if err := s.db.WithContext(ctx).Where("job_id = ?", task.ScheduleJobID).First(&sj).Error; err != nil {
		return nil, &ScheduledError{Code: errcode.ScheduledProjectJobNotExist}
	}
	var tasks []model.ProjectJobTaskDO
	if err := s.db.WithContext(ctx).Where("project_id = ? AND job_id = ?", sj.ProjectID, sj.JobID).Find(&tasks).Error; err != nil {
		return nil, err
	}
	job := model.ProjectJobDO{
		BaseDO:       sj.BaseDO,
		ProjectID:    sj.ProjectID,
		JobID:        sj.JobID,
		Name:         sj.Name,
		Status:       sj.Status,
		ErrMsg:       sj.ErrMsg,
		GraphID:      sj.GraphID,
		Edges:        sj.Edges,
		FinishedTime: sj.FinishedTime,
	}
	return s.buildJobVO(ctx, sc, &job, tasks)
}

// buildJobVO builds Java ProjectJobVO from a job, its tasks and the schedule's
// graph snapshot.
func (s *ScheduledService) buildJobVO(ctx context.Context, sc *model.ProjectScheduleDO, job *model.ProjectJobDO, tasks []model.ProjectJobTaskDO) (*ScheduledJobVO, error) {
	var results []model.ProjectResultDO
	if err := s.db.WithContext(ctx).Where("project_id = ? AND job_id = ?", sc.ProjectID, job.JobID).Find(&results).Error; err != nil {
		return nil, err
	}
	resultsByTask := map[string][]map[string]any{}
	for _, r := range results {
		resultsByTask[r.TaskID] = append(resultsByTask[r.TaskID], map[string]any{
			"kind": r.Kind, "refId": r.RefID, "nodeId": r.NodeID, "taskId": r.TaskID, "jobId": r.JobID,
		})
	}
	taskByNode := map[string]model.ProjectJobTaskDO{}
	for _, t := range tasks {
		if t.GraphNodeID != "" {
			taskByNode[t.GraphNodeID] = t
		}
	}

	var gs scheduleGraphSnapshot
	_ = json.Unmarshal([]byte(sc.GraphInfo), &gs)
	edges := job.Edges
	if edges == "" {
		edges = gs.Graph.Edges
	}
	detail := &ScheduledGraphDetailVO{
		ProjectID: job.ProjectID,
		GraphID:   job.GraphID,
		Name:      job.Name,
		Edges:     jsonRawOr(edges, "[]"),
		Nodes:     []ScheduledGraphNodeVO{},
	}
	if detail.GraphID == "" {
		detail.GraphID = sc.GraphID
	}
	nodeNames := map[string]string{}
	var nodesDO []model.NodeDO
	_ = s.db.WithContext(ctx).Find(&nodesDO).Error
	for _, n := range nodesDO {
		nodeNames[n.NodeID] = n.Name
	}
	for _, n := range gs.Nodes {
		vo := ScheduledGraphNodeVO{
			GraphNodeID: n.GraphNodeID,
			CodeName:    n.CodeName,
			Label:       n.Label,
			X:           n.X,
			Y:           n.Y,
			Inputs:      jsonArrayOfStrings(n.Inputs),
			Outputs:     jsonArrayOfStrings(n.Outputs),
			NodeDef:     jsonRawOr(n.NodeDef, "null"),
		}
		if t, ok := taskByNode[n.GraphNodeID]; ok {
			vo.Status = graphjob.NormalizeTaskStatus(t.Status)
			vo.JobID = t.JobID
			vo.TaskID = t.TaskID
			vo.Results = resultsByTask[t.TaskID]
			for _, p := range jsonArrayOfStrings(t.Parties) {
				name := nodeNames[p]
				if name == "" {
					name = p
				}
				vo.Parties = append(vo.Parties, map[string]any{"nodeId": p, "nodeName": name})
			}
		}
		detail.Nodes = append(detail.Nodes, vo)
	}
	return &ScheduledJobVO{
		JobID:       job.JobID,
		Status:      graphjob.NormalizeJobStatus(job.Status),
		ErrMsg:      job.ErrMsg,
		GmtCreate:   s.fmtRFC3339(&job.GmtCreate),
		GmtModified: s.fmtRFC3339(&job.GmtModified),
		GmtFinished: s.fmtRFC3339(job.FinishedTime),
		Finished:    graphjob.IsJobFinished(job.Status),
		Graph:       detail,
	}, nil
}

// ListProjectJob implements /scheduled/job/list.
func (s *ScheduledService) ListProjectJob(ctx context.Context, req *ScheduleListProjectJobRequest) (*ScheduledJobPage, error) {
	if _, err := s.checkProjectAndGraph(ctx, req.ProjectID, req.GraphID); err != nil {
		return nil, err
	}
	page, size := normPage(req.PageNum, req.PageSize, 10)
	q := s.db.WithContext(ctx).Model(&model.ProjectScheduleJobDO{}).Where("project_id = ?", req.ProjectID)
	if req.GraphID != "" {
		q = q.Where("graph_id = ?", req.GraphID)
	}
	if req.ScheduleTaskID != "" {
		q = q.Where("schedule_task_id = ?", req.ScheduleTaskID)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, err
	}
	var rows []model.ProjectScheduleJobDO
	if err := q.Order("gmt_create DESC, id DESC").Offset((page - 1) * size).Limit(size).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := &ScheduledJobPage{
		PageTotal: int((total + int64(size) - 1) / int64(size)),
		PageSize:  size,
		Total:     total,
		Data:      []ScheduledJobSummaryVO{},
	}
	if len(rows) == 0 {
		out.PageTotal = 1
		out.List = out.Data
		return out, nil
	}
	jobIDs := make([]string, 0, len(rows))
	for _, r := range rows {
		jobIDs = append(jobIDs, r.JobID)
	}
	type kindCount struct {
		JobID string
		Kind  string
		Cnt   int64
	}
	var kcs []kindCount
	if err := s.db.WithContext(ctx).Model(&model.ProjectResultDO{}).
		Select("job_id, kind, COUNT(*) AS cnt").
		Where("project_id = ? AND job_id IN ?", req.ProjectID, jobIDs).
		Group("job_id, kind").Scan(&kcs).Error; err != nil {
		return nil, err
	}
	counts := map[string]map[string]int64{}
	for _, kc := range kcs {
		if counts[kc.JobID] == nil {
			counts[kc.JobID] = map[string]int64{}
		}
		counts[kc.JobID][strings.ToLower(kc.Kind)] += kc.Cnt
	}
	type taskCount struct {
		JobID    string
		Total    int64
		Finished int64
	}
	var tcs []taskCount
	if err := s.db.WithContext(ctx).Model(&model.ProjectJobTaskDO{}).
		Select("job_id, COUNT(*) AS total, SUM(CASE WHEN status IN ('SUCCEED','SUCCEEDED','SUCCESS') THEN 1 ELSE 0 END) AS finished").
		Where("project_id = ? AND job_id IN ?", req.ProjectID, jobIDs).
		Group("job_id").Scan(&tcs).Error; err != nil {
		return nil, err
	}
	taskCounts := map[string]taskCount{}
	for _, tc := range tcs {
		taskCounts[tc.JobID] = tc
	}
	for i := range rows {
		r := &rows[i]
		c := counts[r.JobID]
		tc := taskCounts[r.JobID]
		out.Data = append(out.Data, ScheduledJobSummaryVO{
			JobID:             r.JobID,
			Status:            graphjob.NormalizeJobStatus(r.Status),
			ErrMsg:            r.ErrMsg,
			GmtCreate:         s.fmtRFC3339(&r.GmtCreate),
			GmtModified:       s.fmtRFC3339(&r.GmtModified),
			GmtFinished:       s.fmtRFC3339(r.FinishedTime),
			TableCount:        c["fedtable"],
			ModelCount:        c["model"],
			RuleCount:         c["rule"],
			ReportCount:       c["report"],
			FinishedTaskCount: tc.Finished,
			TaskCount:         tc.Total,
		})
	}
	out.List = out.Data
	return out, nil
}

// ---------------------------------------------------------------------------
// Engine: fire due tasks
// ---------------------------------------------------------------------------

// dueBatchSize bounds the number of tasks handled per engine tick.
const dueBatchSize = 100

// RunDue fires every due TO_BE_RUN task of an UP schedule at `now`. Tasks
// overdue by more than MisfireGrace are marked FAILED. Returns the number of
// tasks submitted to Kuscia by this instance.
func (s *ScheduledService) RunDue(ctx context.Context, now time.Time) (int, error) {
	now = now.In(s.opts.Location)
	var due []model.ProjectScheduleTaskDO
	err := s.db.WithContext(ctx).
		Table("project_schedule_task AS t").
		Select("t.*").
		Joins("JOIN project_schedule AS s ON s.schedule_id = t.schedule_id AND s.deleted_at IS NULL AND s.status = ?", model.ScheduledStatusUp).
		Where("t.deleted_at IS NULL AND t.status = ? AND t.schedule_task_expect_start <= ?", model.ScheduledStatusToBeRun, now).
		Order("t.schedule_task_expect_start ASC").
		Limit(dueBatchSize).
		Find(&due).Error
	if err != nil {
		return 0, err
	}
	fired := 0
	for i := range due {
		t := &due[i]
		if now.Sub(t.ScheduleTaskExpectStart) > s.opts.MisfireGrace {
			s.markMisfired(ctx, t, now)
			continue
		}
		ok, fErr := s.fireTask(ctx, t, now)
		if fErr != nil {
			s.log.Warn("scheduled engine: fire task failed", zap.String("schedule_task_id", t.ScheduleTaskID), zap.Error(fErr))
		}
		if ok {
			fired++
		}
	}
	return fired, nil
}

func (s *ScheduledService) markMisfired(ctx context.Context, t *model.ProjectScheduleTaskDO, now time.Time) {
	res := s.db.WithContext(ctx).Model(&model.ProjectScheduleTaskDO{}).
		Where("id = ? AND status = ?", t.ID, model.ScheduledStatusToBeRun).
		Updates(map[string]any{"status": model.ScheduledStatusFailed, "schedule_task_end_time": now})
	if res.Error != nil || res.RowsAffected == 0 {
		return
	}
	msg := fmt.Sprintf("misfired: expected at %s, engine resumed at %s (grace %s)",
		t.ScheduleTaskExpectStart.Format(scheduleTimeLayout), now.Format(scheduleTimeLayout), s.opts.MisfireGrace)
	s.db.WithContext(ctx).Model(&model.ProjectScheduleJobDO{}).
		Where("project_id = ? AND job_id = ?", t.ProjectID, t.ScheduleJobID).
		Updates(map[string]any{"status": model.ScheduleJobStatusFailed, "err_msg": msg, "finished_time": now})
	s.log.Warn("scheduled engine: task misfired", zap.String("schedule_task_id", t.ScheduleTaskID), zap.String("detail", msg))
}

// fireTask claims the task (status CAS TO_BE_RUN -> RUNNING, the multi-instance
// lock) and submits its stored request (Java SecretpadJob.executeInternal).
func (s *ScheduledService) fireTask(ctx context.Context, t *model.ProjectScheduleTaskDO, now time.Time) (bool, error) {
	res := s.db.WithContext(ctx).Model(&model.ProjectScheduleTaskDO{}).
		Where("id = ? AND status = ?", t.ID, model.ScheduledStatusToBeRun).
		Updates(map[string]any{"status": model.ScheduledStatusRunning, "schedule_task_start_time": now, "schedule_task_end_time": nil})
	if res.Error != nil {
		return false, res.Error
	}
	if res.RowsAffected == 0 {
		return false, nil // claimed by another instance or status changed
	}
	fail := func(cause error) (bool, error) {
		end := s.now()
		s.db.WithContext(ctx).Model(&model.ProjectScheduleTaskDO{}).Where("id = ?", t.ID).
			Updates(map[string]any{"status": model.ScheduledStatusFailed, "schedule_task_end_time": end})
		s.db.WithContext(ctx).Model(&model.ProjectScheduleJobDO{}).
			Where("project_id = ? AND job_id = ?", t.ProjectID, t.ScheduleJobID).
			Updates(map[string]any{"status": model.ScheduleJobStatusFailed, "err_msg": cause.Error(), "finished_time": end})
		return false, cause
	}
	req, err := decodeJobRequest(t.JobRequest)
	if err != nil {
		return fail(fmt.Errorf("decode job request: %w", err))
	}
	if s.kuscia == nil {
		return fail(errors.New("kuscia client unavailable"))
	}
	if _, err := s.kuscia.CreateJob(ctx, req); err != nil {
		return fail(fmt.Errorf("kuscia create job: %w", err))
	}
	s.db.WithContext(ctx).Model(&model.ProjectScheduleJobDO{}).
		Where("project_id = ? AND job_id = ?", t.ProjectID, t.ScheduleJobID).
		Updates(map[string]any{"status": model.ScheduleJobStatusRunning, "err_msg": ""})
	s.log.Info("scheduled engine: task fired", zap.String("schedule_task_id", t.ScheduleTaskID), zap.String("job_id", req.JobID))
	return true, nil
}

// ---------------------------------------------------------------------------
// Engine: status write-back
// ---------------------------------------------------------------------------

// SyncRunning polls Kuscia for every RUNNING / STOPPING schedule task's job.
func (s *ScheduledService) SyncRunning(ctx context.Context) error {
	if s.kuscia == nil {
		return nil
	}
	var jobIDs []string
	if err := s.db.WithContext(ctx).Model(&model.ProjectScheduleTaskDO{}).
		Where("status IN ? AND schedule_job_id <> ''", []string{model.ScheduledStatusRunning, model.ScheduledStatusStopping}).
		Distinct("schedule_job_id").Pluck("schedule_job_id", &jobIDs).Error; err != nil {
		return err
	}
	for _, id := range jobIDs {
		resp, err := s.kuscia.QueryJob(ctx, id)
		if err != nil || resp == nil || resp.Data.Status == nil {
			continue
		}
		if _, err := s.ApplyKusciaJobStatus(ctx, id, resp.Data.Status); err != nil {
			s.log.Warn("scheduled engine: apply job status failed", zap.String("job_id", id), zap.Error(err))
		}
	}
	return nil
}

// ApplyKusciaJobStatus writes a Kuscia job status back to the schedule job,
// its tasks and the owning schedule task (Java JobManager scheduleJob branch).
// It implements ScheduleJobStatusHook.
func (s *ScheduledService) ApplyKusciaJobStatus(ctx context.Context, jobID string, st *kuscia.JobExecutionStatus) (bool, error) {
	var sj model.ProjectScheduleJobDO
	if err := s.db.WithContext(ctx).Where("job_id = ?", jobID).First(&sj).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, err
	}
	if st == nil || sj.Status == model.ScheduleJobStatusSucceed {
		return true, nil
	}
	now := s.now()
	jobStatus := mapKusciaJobStateToScheduleJob(st.State)

	for _, kt := range st.Tasks {
		ids := []string{kt.TaskID}
		if kt.Alias != "" && kt.Alias != kt.TaskID {
			ids = append(ids, kt.Alias)
		}
		upd := map[string]any{"status": mapKusciaTaskStateToNodeStatus(kt.State)}
		if kt.ErrMsg != "" {
			upd["err_msg"] = kt.ErrMsg
		}
		if err := s.db.WithContext(ctx).Model(&model.ProjectJobTaskDO{}).
			Where("project_id = ? AND job_id = ? AND task_id IN ?", sj.ProjectID, sj.JobID, ids).
			Updates(upd).Error; err != nil {
			return true, err
		}
	}

	jobUpd := map[string]any{"status": jobStatus}
	if st.ErrMsg != "" {
		jobUpd["err_msg"] = st.ErrMsg
	}
	if isScheduleJobFinished(jobStatus) {
		jobUpd["finished_time"] = now
	}
	if err := s.db.WithContext(ctx).Model(&model.ProjectScheduleJobDO{}).Where("id = ?", sj.ID).Updates(jobUpd).Error; err != nil {
		return true, err
	}

	var tasks []model.ProjectScheduleTaskDO
	if err := s.db.WithContext(ctx).Where("schedule_job_id = ?", jobID).Find(&tasks).Error; err != nil {
		return true, err
	}
	taskStatus := model.ScheduledStatusFrom(jobStatus)
	for i := range tasks {
		t := &tasks[i]
		// Never resurrect a task that has not fired, and keep STOPPING until
		// Kuscia reports a terminal state.
		if t.Status == model.ScheduledStatusToBeRun {
			continue
		}
		if t.Status == model.ScheduledStatusStopping && !isScheduleTaskTerminal(taskStatus) {
			continue
		}
		if t.Status == taskStatus {
			continue
		}
		upd := map[string]any{"status": taskStatus}
		if isScheduleTaskTerminal(taskStatus) {
			upd["schedule_task_end_time"] = now
		}
		if err := s.db.WithContext(ctx).Model(&model.ProjectScheduleTaskDO{}).Where("id = ?", t.ID).Updates(upd).Error; err != nil {
			return true, err
		}
	}
	return true, nil
}
