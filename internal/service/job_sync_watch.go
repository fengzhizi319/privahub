package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/internal/graphjob"
	"github.com/fengzhizi319/privahub/pkg/kuscia"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// Watch-based job status sync (Java JobManager.startSync: gRPC
// jobServiceAsyncStub.watchJob + onError/onCompleted -> restart).
//
// KusciaAPI exposes the same stream over HTTP (kuscia
// pkg/kusciaapi/handler/httphandler/job/watch.go, POST /api/v1/job/watch),
// which is what the Go client speaks, so no gRPC stub is needed. The watch
// reconnects with back-off when the stream errors or ends; when the endpoint
// is missing (404/405/501: older Kuscia) the watch is disabled and the
// poller alone keeps syncing. While a watch stream is connected the poller
// runs every watchPollEvery ticks instead of every tick.

const (
	// watchTimeoutSeconds is WatchJobRequest.timeout_seconds: the stream is
	// renewed periodically so a silently dead connection cannot linger.
	watchTimeoutSeconds = 600
	watchBackoffMin     = time.Second
	watchBackoffMax     = time.Minute
	// watchPollEvery: poll cadence (in ticks) while the watch is healthy.
	watchPollEvery = 6
	// notFoundThreshold: consecutive QueryJob "not found" polls before a
	// vanished job is treated as DELETED (guards transient API glitches).
	notFoundThreshold = 3
	// defaultNotFoundGrace: a job younger than this is never treated as
	// deleted by the poller (it may not have been submitted to Kuscia yet).
	defaultNotFoundGrace = time.Minute
)

// JobWatcher streams Kuscia job events (implemented by *kuscia.Client).
type JobWatcher interface {
	WatchJobs(ctx context.Context, timeoutSeconds int64, fn func(*kuscia.WatchJobEvent) error) error
}

var _ JobWatcher = (*kuscia.Client)(nil)

// JobDeletedHandler is optionally implemented by a JobSyncHandler that wants
// DELETED events of its jobs; returning true marks the job as handled (Java
// syncTeeJob / syncModelExportJob return early on DELETED as well).
type JobDeletedHandler interface {
	HandleJobDeleted(ctx context.Context, jobID string) bool
}

// nodeWatch is the state of one watch stream ("" = default KusciaAPI).
type nodeWatch struct {
	connected   bool
	unsupported bool
	cancel      context.CancelFunc
}

// watchState tracks one watch stream per KusciaAPI channel (Java
// JobManager.startSync(nodeId) per registered node).
type watchState struct {
	mu    sync.Mutex
	nodes map[string]*nodeWatch
	// unsupported mirrors the default stream's flag (kept for callers/tests).
	unsupported bool
}

// SetWatcher overrides the default job watcher (tests); nil disables it.
func (s *JobStatusSyncService) SetWatcher(w JobWatcher) { s.watcher = w }

// SetPool enables one job watch per node registered in the pool (started
// and stopped on pool changes) in addition to the default watcher.
func (s *JobStatusSyncService) SetPool(pool *kuscia.ClientPool) { s.pool = pool }

// WatchConnected reports whether the job watch is healthy: every watch
// stream that is not unsupported is connected (and at least one is).
func (s *JobStatusSyncService) WatchConnected() bool {
	s.watchState.mu.Lock()
	defer s.watchState.mu.Unlock()
	active := 0
	for _, w := range s.watchState.nodes {
		if w.unsupported {
			continue
		}
		if !w.connected {
			return false
		}
		active++
	}
	return active > 0
}

// WatchedNodes returns the channels with a running watch ("" = default).
func (s *JobStatusSyncService) WatchedNodes() []string {
	s.watchState.mu.Lock()
	defer s.watchState.mu.Unlock()
	out := make([]string, 0, len(s.watchState.nodes))
	for k, w := range s.watchState.nodes {
		if !w.unsupported {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

func (s *JobStatusSyncService) nodeWatchLocked(node string) *nodeWatch {
	if s.watchState.nodes == nil {
		s.watchState.nodes = map[string]*nodeWatch{}
	}
	w, ok := s.watchState.nodes[node]
	if !ok {
		w = &nodeWatch{}
		s.watchState.nodes[node] = w
	}
	return w
}

func (s *JobStatusSyncService) setWatchConnected(node string, v bool) {
	s.watchState.mu.Lock()
	if w, ok := s.watchState.nodes[node]; ok {
		w.connected = v
	}
	s.watchState.mu.Unlock()
}

// skipPollTick thins the poller out while the watch is healthy.
func (s *JobStatusSyncService) skipPollTick() bool {
	if !s.WatchConnected() {
		s.pollTick = 0
		return false
	}
	s.pollTick++
	if s.pollTick >= watchPollEvery {
		s.pollTick = 0
		return false
	}
	return true
}

// watchUnsupported reports an HTTP status meaning "no such endpoint".
func watchUnsupported(err error) bool {
	var ae *kuscia.APIError
	if !errors.As(err, &ae) {
		return false
	}
	switch ae.HTTPStatus {
	case http.StatusNotFound, http.StatusMethodNotAllowed, http.StatusNotImplemented:
		return true
	}
	return false
}

// startWatches starts the default watch and one watch per registered pool
// node, and follows pool registrations (Java RegisterKusciaEvent ->
// startSync(nodeId); UnRegisterKusciaEvent -> stream closed).
func (s *JobStatusSyncService) startWatches() {
	if s.pool != nil {
		s.pool.OnChange(func(ep kuscia.NodeEndpoint, registered bool) {
			// The endpoint of a re-registered node may have changed.
			s.stopNodeWatch(ep.DomainID)
			s.reconcileWatches()
		})
	}
	s.reconcileWatches()
}

// watcherKey identifies the KusciaAPI behind a watcher: nodes registered with
// the same endpoint (e.g. several domains served by one KusciaAPI) share one
// stream instead of receiving every event several times.
func watcherKey(w JobWatcher) string {
	if b, ok := w.(interface{ BaseURL() string }); ok {
		return b.BaseURL()
	}
	return fmt.Sprintf("%p", w)
}

// reconcileWatches runs exactly one watch per distinct KusciaAPI endpoint:
// the default client ("") plus the pool nodes, stopping watches of nodes no
// longer registered (or whose endpoint is already watched).
func (s *JobStatusSyncService) reconcileWatches() {
	desired := map[string]JobWatcher{}
	seen := map[string]bool{}
	if s.watcher != nil {
		desired[""] = s.watcher
		seen[watcherKey(s.watcher)] = true
	}
	if s.pool != nil {
		for _, ep := range s.pool.Endpoints() {
			c, ok := s.pool.Lookup(ep.DomainID)
			if !ok || c == nil {
				continue
			}
			if k := watcherKey(c); !seen[k] {
				seen[k] = true
				desired[ep.DomainID] = c
			}
		}
	}
	s.watchState.mu.Lock()
	var stale []string
	for node := range s.watchState.nodes {
		if _, ok := desired[node]; !ok {
			stale = append(stale, node)
		}
	}
	s.watchState.mu.Unlock()
	for _, node := range stale {
		s.stopNodeWatch(node)
	}
	for node, w := range desired {
		s.startNodeWatch(node, w)
	}
}

// startNodeWatch runs a watch loop for one channel unless one is running.
func (s *JobStatusSyncService) startNodeWatch(node string, w JobWatcher) {
	select {
	case <-s.stopCh:
		return
	default:
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.watchState.mu.Lock()
	nw := s.nodeWatchLocked(node)
	if nw.cancel != nil {
		s.watchState.mu.Unlock()
		cancel()
		return
	}
	nw.cancel = cancel
	nw.unsupported = false
	s.watchState.mu.Unlock()
	go func() {
		select {
		case <-s.stopCh:
			cancel()
		case <-ctx.Done():
		}
	}()
	go s.watchNode(ctx, node, w)
}

// stopNodeWatch cancels the watch of one channel.
func (s *JobStatusSyncService) stopNodeWatch(node string) {
	s.watchState.mu.Lock()
	nw, ok := s.watchState.nodes[node]
	if ok {
		delete(s.watchState.nodes, node)
	}
	s.watchState.mu.Unlock()
	if ok && nw.cancel != nil {
		nw.cancel()
	}
}

// watchLoop keeps the default watch stream open until Stop.
func (s *JobStatusSyncService) watchLoop() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.watchState.mu.Lock()
	s.nodeWatchLocked("").cancel = cancel
	s.watchState.mu.Unlock()
	go func() {
		<-s.stopCh
		cancel()
	}()
	s.watchNode(ctx, "", s.watcher)
}

// watchNode keeps one watch stream open until ctx ends; it reconnects with
// back-off and gives up (polling only) when the endpoint is unsupported.
func (s *JobStatusSyncService) watchNode(ctx context.Context, node string, w JobWatcher) {
	log := s.log.With(zap.String("node", node))
	backoff := watchBackoffMin
	for {
		started := time.Now()
		err := w.WatchJobs(ctx, watchTimeoutSeconds, func(ev *kuscia.WatchJobEvent) error {
			s.setWatchConnected(node, true)
			s.HandleWatchEvent(ctx, ev)
			return nil
		})
		s.setWatchConnected(node, false)
		if ctx.Err() != nil {
			return
		}
		if watchUnsupported(err) {
			s.watchState.mu.Lock()
			if nw, ok := s.watchState.nodes[node]; ok {
				nw.unsupported = true
			}
			if node == "" {
				s.watchState.unsupported = true
			}
			s.watchState.mu.Unlock()
			log.Info("kuscia job watch not supported, falling back to polling", zap.Error(err))
			return
		}
		if err != nil {
			log.Warn("kuscia job watch failed, retrying", zap.Error(err), zap.Duration("backoff", backoff))
		}
		if time.Since(started) > watchBackoffMax {
			backoff = watchBackoffMin // the stream was healthy for a while
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		if backoff *= 2; backoff > watchBackoffMax {
			backoff = watchBackoffMax
		}
	}
}

// HandleWatchEvent applies one watch event (Java JobManager.syncJob):
// ERROR is ignored, DELETED stops the job, ADDED / MODIFIED sync the status.
func (s *JobStatusSyncService) HandleWatchEvent(ctx context.Context, ev *kuscia.WatchJobEvent) {
	if ev == nil || ev.Object == nil || ev.Object.JobID == "" {
		return
	}
	jobID := ev.Object.JobID
	var err error
	switch ev.Type {
	case kuscia.WatchEventDeleted:
		err = s.MarkJobDeleted(ctx, jobID)
	case kuscia.WatchEventAdded, kuscia.WatchEventModified:
		s.clearNotFound(jobID)
		err = s.SyncJob(ctx, jobID, ev.Object.Status)
	default:
		return
	}
	if err != nil {
		s.log.Warn("apply kuscia job watch event failed", zap.String("job_id", jobID),
			zap.Stringer("type", ev.Type), zap.Error(err))
	}
}

// --- vanished jobs (poller) ---

func (s *JobStatusSyncService) clearNotFound(jobID string) {
	s.missMu.Lock()
	delete(s.notFound, jobID)
	s.missMu.Unlock()
}

// pruneNotFound forgets counters of jobs no longer pending.
func (s *JobStatusSyncService) pruneNotFound(pending map[string]bool) {
	s.missMu.Lock()
	for id := range s.notFound {
		if !pending[id] {
			delete(s.notFound, id)
		}
	}
	s.missMu.Unlock()
}

// onJobNotFound counts consecutive "job not found" polls and treats the job
// as DELETED once the threshold is reached.
func (s *JobStatusSyncService) onJobNotFound(ctx context.Context, jobID string) {
	s.missMu.Lock()
	if s.notFound == nil {
		s.notFound = map[string]int{}
	}
	s.notFound[jobID]++
	n := s.notFound[jobID]
	s.missMu.Unlock()
	if n < notFoundThreshold {
		return
	}
	if err := s.markJobDeleted(ctx, jobID, true); err != nil {
		s.log.Warn("mark vanished kuscia job failed", zap.String("job_id", jobID), zap.Error(err))
		return
	}
	s.clearNotFound(jobID)
}

// MarkJobDeleted applies a Kuscia DELETED job event (Java
// JobManager.updateJob case DELETED -> ProjectJobDO.stop()): the job becomes
// STOPPED and its RUNNING / INITIALIZED tasks become STOPPED with stop logs
// (errMsg / finished time are left untouched like Java). Finished project
// jobs and SUCCEED schedule jobs are skipped. For a schedule job the owning
// fired schedule tasks (RUNNING / STOPPING) become STOPPED as well (Java
// ScheduledStatus.from("STOPPED")).
func (s *JobStatusSyncService) MarkJobDeleted(ctx context.Context, jobID string) error {
	return s.markJobDeleted(ctx, jobID, false)
}

func (s *JobStatusSyncService) markJobDeleted(ctx context.Context, jobID string, fromPoll bool) error {
	s.applyMu.Lock()
	defer s.applyMu.Unlock()
	s.mu.RLock()
	handlers := append([]JobSyncHandler(nil), s.handlers...)
	s.mu.RUnlock()
	for _, h := range handlers {
		if dh, ok := h.(JobDeletedHandler); ok && dh.HandleJobDeleted(ctx, jobID) {
			return nil
		}
	}
	grace := s.notFoundWait
	now := timeNow()
	var job model.ProjectJobDO
	if err := s.db.WithContext(ctx).Where("job_id = ?", jobID).First(&job).Error; err == nil {
		if graphjob.IsJobFinished(job.Status) {
			return nil
		}
		if fromPoll && now.Sub(job.GmtCreate) < grace {
			return nil
		}
		return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := stopJobTasksTx(tx, job.ProjectID, jobID, now); err != nil {
				return err
			}
			return tx.Model(&model.ProjectJobDO{}).Where("id = ?", job.ID).Update("status", graphjob.JobStopped).Error
		})
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	var sj model.ProjectScheduleJobDO
	if err := s.db.WithContext(ctx).Where("job_id = ?", jobID).First(&sj).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	if graphjob.NormalizeJobStatus(sj.Status) == graphjob.JobSucceed {
		return nil
	}
	// Schedule jobs are pre-rendered (status RUNNING) long before their task
	// fires; only a job whose schedule task was submitted to Kuscia can have
	// been deleted there.
	var fired []model.ProjectScheduleTaskDO
	if err := s.db.WithContext(ctx).Where("schedule_job_id = ? AND status IN ?", jobID,
		[]string{model.ScheduledStatusRunning, model.ScheduledStatusStopping}).Find(&fired).Error; err != nil {
		return err
	}
	if len(fired) == 0 {
		return nil
	}
	if fromPoll {
		for _, t := range fired {
			if t.ScheduleTaskStartTime != nil && now.Sub(*t.ScheduleTaskStartTime) < grace {
				return nil
			}
		}
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := stopJobTasksTx(tx, sj.ProjectID, jobID, now); err != nil {
			return err
		}
		if err := tx.Model(&model.ProjectScheduleJobDO{}).Where("id = ?", sj.ID).
			Update("status", model.ScheduleJobStatusStopped).Error; err != nil {
			return err
		}
		ids := make([]int64, 0, len(fired))
		for _, t := range fired {
			ids = append(ids, t.ID)
		}
		return tx.Model(&model.ProjectScheduleTaskDO{}).Where("id IN ?", ids).
			Updates(map[string]any{"status": model.ScheduledStatusStopped, "schedule_task_end_time": now}).Error
	})
}

// stopJobTasksTx mirrors ProjectJobDO.stop() for tasks: RUNNING /
// INITIALIZED tasks become STOPPED and get the transition logs.
func stopJobTasksTx(tx *gorm.DB, projectID, jobID string, now time.Time) error {
	var tasks []model.ProjectJobTaskDO
	if err := tx.Where("project_id = ? AND job_id = ?", projectID, jobID).Find(&tasks).Error; err != nil {
		return err
	}
	for _, t := range tasks {
		st := graphjob.NormalizeTaskStatus(t.Status)
		if st != graphjob.TaskRunning && st != graphjob.TaskInitialized {
			continue
		}
		if err := tx.Model(&model.ProjectJobTaskDO{}).Where("id = ?", t.ID).Update("status", graphjob.TaskStopped).Error; err != nil {
			return err
		}
		for _, l := range graphjob.TransitionLogs(now, jobID, t.TaskID, st, graphjob.TaskStopped, nil) {
			if err := tx.Create(&model.ProjectJobTaskLogDO{ProjectID: projectID, JobID: jobID, TaskID: t.TaskID, Log: l}).Error; err != nil {
				return err
			}
		}
	}
	return nil
}
