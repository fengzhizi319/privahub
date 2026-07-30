package service

import (
	"context"
	"sync"
	"time"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/pkg/kuscia"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// JobStatusSyncService periodically polls Kuscia for job status updates
// and synchronizes them to the local database. It runs as a background
// goroutine started via Start() and gracefully stopped via Stop().
type JobStatusSyncService struct {
	db           *gorm.DB
	kusciaClient *kuscia.Client
	log          *zap.Logger
	interval     time.Duration
	stopCh       chan struct{}
	stopOnce     sync.Once // guards against double-close of stopCh
}

// NewJobStatusSyncService creates a new JobStatusSyncService.
func NewJobStatusSyncService(db *gorm.DB, kusciaClient *kuscia.Client, log *zap.Logger) *JobStatusSyncService {
	return &JobStatusSyncService{
		db:           db,
		kusciaClient: kusciaClient,
		log:          log,
		interval:     10 * time.Second,
		stopCh:       make(chan struct{}),
	}
}

// Start begins the background sync loop.
func (s *JobStatusSyncService) Start() {
	if s.kusciaClient == nil {
		return
	}
	go s.loop()
}

// Stop gracefully stops the sync loop. It is safe to call multiple times.
func (s *JobStatusSyncService) Stop() {
	s.stopOnce.Do(func() {
		close(s.stopCh)
	})
}

func (s *JobStatusSyncService) loop() {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.syncOnce()
		}
	}
}

// syncOnce performs a single sync cycle: find active jobs, query Kuscia, update DB.
func (s *JobStatusSyncService) syncOnce() {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	// Find all active (non-terminal) jobs
	var activeJobs []model.ProjectJobDO
	if err := s.db.WithContext(ctx).
		Where("status IN ?", []string{"PENDING", "RUNNING"}).
		Find(&activeJobs).Error; err != nil {
		return
	}

	if len(activeJobs) == 0 {
		return
	}

	// Batch query Kuscia for job statuses
	jobIDs := make([]string, 0, len(activeJobs))
	for _, j := range activeJobs {
		jobIDs = append(jobIDs, j.JobID)
	}

	entries, err := s.kusciaClient.BatchQueryJobStatus(ctx, jobIDs)
	if err != nil {
		// Batch query failed (e.g. some jobs no longer exist in Kuscia).
		// Fall back to querying each job individually.
		for _, jid := range jobIDs {
			resp, qErr := s.kusciaClient.QueryJob(ctx, jid)
			if qErr != nil || resp.Data.Status == nil {
				continue
			}
			mappedStatus := mapKusciaState(resp.Data.Status.State)
			s.db.WithContext(ctx).Model(&model.ProjectJobDO{}).
				Where("job_id = ?", jid).
				Update("status", mappedStatus)
			s.syncTaskStatuses(ctx, jid)
		}
		return
	}

	// Build lookup map
	statusMap := make(map[string]string, len(entries))
	for _, e := range entries {
		statusMap[e.JobID] = e.Status.State
	}

	// Update local DB for jobs whose status changed
	for _, job := range activeJobs {
		newState, ok := statusMap[job.JobID]
		if !ok {
			continue
		}

		mappedStatus := mapKusciaState(newState)
		if mappedStatus != job.Status {
			updates := map[string]interface{}{
				"status": mappedStatus,
			}
			if isTerminalStatus(mappedStatus) {
				now := time.Now()
				updates["finished_time"] = &now
			}

			s.db.WithContext(ctx).Model(&model.ProjectJobDO{}).
				Where("job_id = ?", job.JobID).
				Updates(updates)

			if s.log != nil {
				s.log.Info("Job status synced from Kuscia",
					zap.String("job_id", job.JobID),
					zap.String("old_status", job.Status),
					zap.String("new_status", mappedStatus),
				)
			}
		}

		// Always sync task statuses (tasks may change even if job state is unchanged)
		s.syncTaskStatuses(ctx, job.JobID)
	}
}

// syncTaskStatuses queries individual job details and updates task statuses.
func (s *JobStatusSyncService) syncTaskStatuses(ctx context.Context, jobID string) {
	resp, err := s.kusciaClient.QueryJob(ctx, jobID)
	if err != nil {
		return
	}
	if resp.Data.Status == nil {
		return
	}

	for _, task := range resp.Data.Status.Tasks {
		mappedStatus := mapKusciaState(task.State)
		s.db.WithContext(ctx).Model(&model.ProjectJobTaskDO{}).
			Where("task_id = ?", task.TaskID).
			Update("status", mappedStatus)
	}
}

// mapKusciaState maps Kuscia job/task states to SecretPad status strings.
func mapKusciaState(state string) string {
	switch state {
	case "Pending", "pending":
		return "PENDING"
	case "Running", "running":
		return "RUNNING"
	case "Succeeded", "succeeded":
		return "SUCCEEDED"
	case "Failed", "failed":
		return "FAILED"
	case "Stopped", "stopped":
		return "STOPPED"
	default:
		return state
	}
}

// isTerminalStatus returns true if the status is a terminal state.
func isTerminalStatus(status string) bool {
	switch status {
	case "SUCCEEDED", "FAILED", "STOPPED":
		return true
	default:
		return false
	}
}
