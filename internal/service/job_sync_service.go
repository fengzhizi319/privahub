package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/internal/graphjob"
	"github.com/fengzhizi319/privahub/pkg/kuscia"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// JobSyncHandler lets other modules (model export / pack jobs, TEE jobs, ...)
// take over the status sync of their own Kuscia jobs, like Java
// JobManager.syncJob's syncTeeJob / syncModelExportJob branches.
type JobSyncHandler interface {
	// PendingJobIDs returns the Kuscia job ids the handler wants polled.
	PendingJobIDs(ctx context.Context) []string
	// HandleJob processes a job status; return true when the job belongs to
	// the handler (the project job sync is then skipped).
	HandleJob(ctx context.Context, jobID string, detail *kuscia.JobStatusDetailInfo) bool
}

// JobStatusSyncService syncs Kuscia job status into project jobs (Java
// JobManager.syncJob / updateJob / syncResult).
//
// Two sources feed it (see job_sync_watch.go):
//   - the KusciaAPI HTTP job watch stream (POST /api/v1/job/watch, the HTTP
//     twin of Java's gRPC WatchJob) applies ADDED / MODIFIED / DELETED
//     events as they happen;
//   - the poller (QueryJob for every unfinished job) stays on as a safety
//     net: it covers watch outages / reconnect gaps and Kuscia versions
//     without the HTTP watch, and detects jobs that vanished from Kuscia
//     while no watch was connected (treated like a DELETED event).
type JobStatusSyncService struct {
	db       *gorm.DB
	kuscia   GraphKusciaAPI
	log      *zap.Logger
	interval time.Duration
	stopCh   chan struct{}
	stopOnce sync.Once

	mu       sync.RWMutex
	handlers []JobSyncHandler

	// watch-based sync (job_sync_watch.go)
	watcher      JobWatcher
	pool         *kuscia.ClientPool
	nodeReady    WatchNodeReadyFunc // readiness gate of pool-node watches
	readyEvery   time.Duration      // readiness recheck period (default watchBackoffMax)
	backoffMin   time.Duration      // watch reconnect back-off bounds (tests; default watchBackoffMin/Max)
	backoffMax   time.Duration
	watchState   watchState
	applyMu      sync.Mutex // serializes watch / poll status application
	missMu       sync.Mutex
	notFound     map[string]int // job id -> consecutive QueryJob not-found polls
	pollTick     int
	notFoundWait time.Duration
}

// NewJobStatusSyncService creates a new JobStatusSyncService.
func NewJobStatusSyncService(db *gorm.DB, kusciaClient *kuscia.Client, log *zap.Logger) *JobStatusSyncService {
	if log == nil {
		log = zap.NewNop()
	}
	s := &JobStatusSyncService{
		db:           db,
		kuscia:       toGraphKuscia(kusciaClient),
		log:          log,
		interval:     5 * time.Second,
		stopCh:       make(chan struct{}),
		notFound:     map[string]int{},
		notFoundWait: defaultNotFoundGrace,
	}
	if kusciaClient != nil {
		s.watcher = kusciaClient
	}
	return s
}

// SetKuscia overrides the Kuscia API (tests).
func (s *JobStatusSyncService) SetKuscia(k GraphKusciaAPI) { s.kuscia = k }

// RegisterHandler registers a JobSyncHandler (e.g. model pack jobs).
func (s *JobStatusSyncService) RegisterHandler(h JobSyncHandler) {
	s.mu.Lock()
	s.handlers = append(s.handlers, h)
	s.mu.Unlock()
}

// Start begins the background sync loop.
func (s *JobStatusSyncService) Start() {
	if s.kuscia == nil {
		return
	}
	go s.loop()
	s.startWatches()
}

// Stop gracefully stops the sync loop. It is safe to call multiple times.
func (s *JobStatusSyncService) Stop() {
	s.stopOnce.Do(func() { close(s.stopCh) })
}

func (s *JobStatusSyncService) loop() {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			if s.skipPollTick() {
				continue
			}
			s.syncOnce()
		}
	}
}

// syncOnce polls every unfinished job once.
func (s *JobStatusSyncService) syncOnce() {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	ids := map[string]bool{}
	var jobs []model.ProjectJobDO
	s.db.WithContext(ctx).Where("status IN ?", []string{graphjob.JobRunning, "PENDING"}).Find(&jobs)
	for _, j := range jobs {
		ids[j.JobID] = true
	}
	var sjobs []model.ProjectScheduleJobDO
	s.db.WithContext(ctx).Where("status IN ?", []string{graphjob.JobRunning, "PENDING"}).Find(&sjobs)
	for _, j := range sjobs {
		ids[j.JobID] = true
	}
	s.mu.RLock()
	handlers := append([]JobSyncHandler(nil), s.handlers...)
	s.mu.RUnlock()
	for _, h := range handlers {
		for _, id := range h.PendingJobIDs(ctx) {
			ids[id] = true
		}
	}
	for _, id := range sortedKeys(ids) {
		resp, err := s.kuscia.QueryJobDetail(ctx, id)
		if kuscia.IsJobNotFound(err) {
			s.onJobNotFound(ctx, id)
			continue
		}
		if err != nil || resp == nil || resp.Data.Status == nil {
			continue
		}
		s.clearNotFound(id)
		if err := s.SyncJob(ctx, id, resp.Data.Status); err != nil {
			s.log.Warn("job status sync failed", zap.String("job_id", id), zap.Error(err))
		}
	}
	s.pruneNotFound(ids)
}

// SyncJob applies one Kuscia job status (Java JobManager.syncJob).
func (s *JobStatusSyncService) SyncJob(ctx context.Context, jobID string, detail *kuscia.JobStatusDetailInfo) error {
	if detail == nil {
		return nil
	}
	s.applyMu.Lock()
	defer s.applyMu.Unlock()
	s.mu.RLock()
	handlers := append([]JobSyncHandler(nil), s.handlers...)
	s.mu.RUnlock()
	for _, h := range handlers {
		if h.HandleJob(ctx, jobID, detail) {
			return nil
		}
	}
	var job model.ProjectJobDO
	if err := s.db.WithContext(ctx).Where("job_id = ?", jobID).First(&job).Error; err == nil {
		if graphjob.IsJobFinished(job.Status) {
			return nil
		}
		return s.updateJob(ctx, &jobHolder{projectID: job.ProjectID, jobID: jobID, status: job.Status, pk: job.ID}, detail)
	}
	var sj model.ProjectScheduleJobDO
	if err := s.db.WithContext(ctx).Where("job_id = ?", jobID).First(&sj).Error; err == nil {
		if graphjob.NormalizeJobStatus(sj.Status) == graphjob.JobSucceed {
			return nil
		}
		return s.updateJob(ctx, &jobHolder{projectID: sj.ProjectID, jobID: jobID, status: sj.Status, pk: sj.ID, schedule: true}, detail)
	}
	return nil
}

type jobHolder struct {
	projectID string
	jobID     string
	status    string
	pk        int64
	schedule  bool
}

// partyFailedReasons mirrors taskFailedReason.
func partyFailedReasons(t *kuscia.JobTaskStatusDetail) []string {
	var reasons []string
	for _, p := range t.Parties {
		if p.State == "Failed" {
			reasons = append(reasons, fmt.Sprintf("party %s failed msg: %s", p.DomainID, p.ErrMsg))
		}
	}
	return append(reasons, t.ErrMsg)
}

func parseKusciaTime(v string) *time.Time {
	if v == "" {
		return nil
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05"} {
		if t, err := time.Parse(layout, v); err == nil {
			return &t
		}
	}
	return nil
}

// mergeExtendedTasks mirrors removeContentAfterUnderscore + mergeKusciaTaskStatus:
// adapter tasks "{taskId}--1" are folded into their base task.
func mergeExtendedTasks(tasks []kuscia.JobTaskStatusDetail) map[string]kuscia.JobTaskStatusDetail {
	ext := map[string]kuscia.JobTaskStatusDetail{}
	base := map[string]kuscia.JobTaskStatusDetail{}
	for _, t := range tasks {
		raw := t.Alias
		if raw == "" {
			raw = t.TaskID
		}
		if i := strings.Index(raw, "--"); i >= 0 {
			ext[raw[:i]] = t
			continue
		}
		base[raw] = t
	}
	out := map[string]kuscia.JobTaskStatusDetail{}
	for id, e := range ext {
		st := graphjob.TaskStatusFromKuscia(e.State)
		if st == graphjob.TaskSucceed || st == graphjob.TaskRunning {
			e.State = "Running"
		}
		out[id] = e
	}
	for id, b := range base {
		if e, ok := ext[id]; ok {
			est := graphjob.TaskStatusFromKuscia(e.State)
			if graphjob.TaskStatusFromKuscia(b.State) == graphjob.TaskInitialized && est != graphjob.TaskInitialized {
				b.State = "Running"
			}
			if est == graphjob.TaskFailed {
				b.State, b.ErrMsg = "Failed", e.ErrMsg
				b.Parties = append(b.Parties, e.Parties...)
			}
		}
		out[id] = b
	}
	return out
}

// updateJob mirrors JobManager.updateJob: job status / errMsg / finished
// time, task status / errMsg / progress, task logs and result sync.
func (s *JobStatusSyncService) updateJob(ctx context.Context, h *jobHolder, detail *kuscia.JobStatusDetailInfo) error {
	now := timeNow()
	jobUpdates := map[string]any{}
	newJobStatus := graphjob.NormalizeJobStatus(h.status)
	if !(graphjob.IsKusciaFinishedState(detail.State) && detail.EndTime == "") {
		newJobStatus = graphjob.JobStatusFromKuscia(detail.State)
		jobUpdates["status"] = newJobStatus
		jobUpdates["err_msg"] = detail.ErrMsg
	}
	if ft := parseKusciaTime(detail.EndTime); ft != nil {
		jobUpdates["finished_time"] = ft
	}
	var succeeded []model.ProjectJobTaskDO
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for taskID, kt := range mergeExtendedTasks(detail.Tasks) {
			var task model.ProjectJobTaskDO
			if err := tx.Where("project_id = ? AND job_id = ? AND task_id = ?", h.projectID, h.jobID, taskID).First(&task).Error; err != nil {
				continue // secretpad does not know the task: skip like Java
			}
			from := graphjob.NormalizeTaskStatus(task.Status)
			to := graphjob.TaskStatusFromKuscia(kt.State)
			var reasons []string
			if to == graphjob.TaskFailed {
				reasons = partyFailedReasons(&kt)
			}
			for _, l := range graphjob.TransitionLogs(now, h.jobID, taskID, from, to, reasons) {
				if err := tx.Create(&model.ProjectJobTaskLogDO{ProjectID: h.projectID, JobID: h.jobID, TaskID: taskID, Log: l}).Error; err != nil {
					return err
				}
			}
			updates := map[string]any{"status": to, "err_msg": kt.ErrMsg}
			if kt.Progress > 0 {
				ei := map[string]any{}
				_ = json.Unmarshal([]byte(task.ExtraInfo), &ei)
				ei["progress"] = kt.Progress
				b, _ := json.Marshal(ei)
				updates["extra_info"] = string(b)
			}
			if err := tx.Model(&model.ProjectJobTaskDO{}).Where("id = ?", task.ID).Updates(updates).Error; err != nil {
				return err
			}
			if to == graphjob.TaskSucceed && from != graphjob.TaskSucceed {
				task.Status = to
				succeeded = append(succeeded, task)
			}
		}
		// A FAILED / STOPPED Kuscia job never starts its remaining tasks
		// (Kuscia keeps reporting them as "Pending"): stop them like
		// ProjectJobDO.stop() so no task of a finished job stays INITIALIZED /
		// RUNNING — the DAG would otherwise show them as queued and keep
		// polling graph/node/status forever.
		if st, ok := jobUpdates["status"].(string); ok && (st == graphjob.JobFailed || st == graphjob.JobStopped) {
			if err := stopJobTasksTx(tx, h.projectID, h.jobID, now); err != nil {
				return err
			}
		}
		if len(jobUpdates) == 0 {
			return nil
		}
		if h.schedule && CurrentScheduleJobStatusHook() != nil {
			return nil // job-level schedule state is owned by the scheduled service hook
		}
		if h.schedule {
			if err := tx.Model(&model.ProjectScheduleJobDO{}).Where("id = ?", h.pk).Updates(jobUpdates).Error; err != nil {
				return err
			}
			taskUpdates := map[string]any{"status": model.ScheduledStatusFrom(newJobStatus)}
			if newJobStatus == graphjob.JobSucceed || newJobStatus == graphjob.JobFailed {
				taskUpdates["schedule_task_end_time"] = now
			}
			return tx.Model(&model.ProjectScheduleTaskDO{}).Where("schedule_job_id = ?", h.jobID).Updates(taskUpdates).Error
		}
		return tx.Model(&model.ProjectJobDO{}).Where("id = ?", h.pk).Updates(jobUpdates).Error
	})
	if err != nil {
		return err
	}
	if h.schedule {
		if hook := CurrentScheduleJobStatusHook(); hook != nil {
			if _, err := hook.ApplyKusciaJobStatus(ctx, h.jobID, toExecutionStatus(detail)); err != nil {
				return err
			}
		}
	}
	for i := range succeeded {
		if err := s.syncResult(ctx, &succeeded[i]); err != nil {
			s.log.Warn("sync task result failed", zap.String("task_id", succeeded[i].TaskID), zap.Error(err))
		}
	}
	return nil
}

// toExecutionStatus converts the detailed status for the schedule hook.
func toExecutionStatus(d *kuscia.JobStatusDetailInfo) *kuscia.JobExecutionStatus {
	out := &kuscia.JobExecutionStatus{State: d.State, ErrMsg: d.ErrMsg}
	for _, t := range d.Tasks {
		out.Tasks = append(out.Tasks, kuscia.TaskStatus{TaskID: t.TaskID, Alias: t.Alias, State: t.State, ErrMsg: t.ErrMsg})
	}
	return out
}
