package service

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/internal/dao/repository"
	"github.com/fengzhizi319/privahub/pkg/kuscia"
)

// Job service errors.
var (
	ErrJobNotFound = errors.New("job not found")
)

// JobService handles Kuscia job management.
type JobService struct {
	jobRepo      repository.JobRepository
	taskRepo     repository.TaskRepository
	taskLogRepo  repository.TaskLogRepository
	kusciaClient *kuscia.Client
}

// NewJobService creates a new JobService.
func NewJobService(
	jobRepo repository.JobRepository,
	taskRepo repository.TaskRepository,
	taskLogRepo repository.TaskLogRepository,
	kusciaClient *kuscia.Client,
) *JobService {
	return &JobService{
		jobRepo:      jobRepo,
		taskRepo:     taskRepo,
		taskLogRepo:  taskLogRepo,
		kusciaClient: kusciaClient,
	}
}

// --- Request / Response DTOs ---

// CreateJobRequest represents a job creation request.
type CreateJobRequest struct {
	ProjectID string `json:"project_id" binding:"required"`
	Name      string `json:"name" binding:"required"`
	GraphID   string `json:"graph_id"`
	Edges     string `json:"edges"`
}

// JobVO represents a job view object.
type JobVO struct {
	JobID        string   `json:"job_id"`
	ProjectID    string   `json:"project_id"`
	Name         string   `json:"name"`
	Status       string   `json:"status"`
	ErrMsg       string   `json:"err_msg,omitempty"`
	GraphID      string   `json:"graph_id,omitempty"`
	Tasks        []TaskVO `json:"tasks,omitempty"`
	GmtCreate    string   `json:"gmt_create"`
	FinishedTime string   `json:"finished_time,omitempty"`
}

// TaskVO represents a task view object.
type TaskVO struct {
	TaskID      string `json:"task_id"`
	JobID       string `json:"job_id"`
	Parties     string `json:"parties"`
	Status      string `json:"status"`
	ErrMsg      string `json:"err_msg,omitempty"`
	GraphNodeID string `json:"graph_node_id,omitempty"`
}

// JobListRequest represents a job list request.
type JobListRequest struct {
	ProjectID string `json:"project_id" binding:"required"`
	Page      int    `json:"page"`
	Size      int    `json:"size"`
}

// JobListResponse represents a paginated job list.
type JobListResponse struct {
	Jobs  []JobVO `json:"jobs"`
	Total int64   `json:"total"`
	Page  int     `json:"page"`
	Size  int     `json:"size"`
}

// GetJobRequest represents a job detail request.
type GetJobRequest struct {
	ProjectID string `json:"project_id" binding:"required"`
	JobID     string `json:"job_id" binding:"required"`
}

// StopJobRequest represents a job stop request.
type StopJobRequest struct {
	ProjectID string `json:"project_id" binding:"required"`
	JobID     string `json:"job_id" binding:"required"`
	TaskID    string `json:"task_id"`
}

// GetTaskLogRequest represents a task log request.
type GetTaskLogRequest struct {
	ProjectID string `json:"project_id" binding:"required"`
	JobID     string `json:"job_id" binding:"required"`
	TaskID    string `json:"task_id" binding:"required"`
}

// TaskLogVO represents task log entries.
type TaskLogVO struct {
	TaskID string   `json:"task_id"`
	Logs   []string `json:"logs"`
}

// --- Service Methods ---

// CreateJob creates a new job and submits it to Kuscia.
func (s *JobService) CreateJob(ctx context.Context, req *CreateJobRequest) (*JobVO, error) {
	jobID := uuid.New().String()[:8]

	job := &model.ProjectJobDO{
		ProjectID: req.ProjectID,
		JobID:     jobID,
		Name:      req.Name,
		Status:    "PENDING",
		GraphID:   req.GraphID,
		Edges:     req.Edges,
	}

	if err := s.jobRepo.Create(ctx, job); err != nil {
		return nil, err
	}

	// Submit to Kuscia if client is available
	if s.kusciaClient != nil {
		kusciaReq := &kuscia.CreateJobRequest{
			JobID:     jobID,
			Initiator: "alice",
			Tasks: []kuscia.TaskConfig{
				{
					AppImage: "secretflow",
					Alias:    req.Name,
					Parties: []kuscia.Party{
						{Name: "alice", Role: "guest"},
						{Name: "bob", Role: "host"},
					},
				},
			},
			CustomFields: map[string]string{
				"project_id": req.ProjectID,
				"graph_id":   req.GraphID,
			},
		}
		if _, err := s.kusciaClient.CreateJob(ctx, kusciaReq); err != nil {
			// Kuscia unreachable — keep job in PENDING, log warning
			_ = s.jobRepo.UpdateStatus(ctx, jobID, "PENDING", "kuscia unreachable: "+err.Error())
		}
	}

	return &JobVO{
		JobID:     jobID,
		ProjectID: req.ProjectID,
		Name:      req.Name,
		Status:    "PENDING",
		GmtCreate: job.GmtCreate.Format("2006-01-02 15:04:05"),
	}, nil
}

// ListJobs lists jobs for a project with pagination.
func (s *JobService) ListJobs(ctx context.Context, req *JobListRequest) (*JobListResponse, error) {
	if req.Page < 1 {
		req.Page = 1
	}
	if req.Size < 1 || req.Size > 100 {
		req.Size = 10
	}

	jobs, err := s.jobRepo.FindByProjectID(ctx, req.ProjectID)
	if err != nil {
		return nil, err
	}

	// Simple in-memory pagination
	total := int64(len(jobs))
	start := (req.Page - 1) * req.Size
	if start >= len(jobs) {
		start = len(jobs)
	}
	end := start + req.Size
	if end > len(jobs) {
		end = len(jobs)
	}

	result := make([]JobVO, 0, end-start)
	for _, j := range jobs[start:end] {
		result = append(result, s.toJobVO(&j, nil))
	}

	return &JobListResponse{
		Jobs:  result,
		Total: total,
		Page:  req.Page,
		Size:  req.Size,
	}, nil
}

// GetJob retrieves job detail with tasks.
func (s *JobService) GetJob(ctx context.Context, req *GetJobRequest) (*JobVO, error) {
	job, err := s.jobRepo.FindByProjectAndJobID(ctx, req.ProjectID, req.JobID)
	if err != nil {
		return nil, ErrJobNotFound
	}

	tasks, err := s.taskRepo.FindByProjectAndJobID(ctx, req.ProjectID, req.JobID)
	if err != nil {
		tasks = nil // non-fatal
	}

	vo := s.toJobVO(job, tasks)
	return &vo, nil
}

// StopJob stops a running job both locally and in Kuscia.
func (s *JobService) StopJob(ctx context.Context, req *StopJobRequest) error {
	job, err := s.jobRepo.FindByProjectAndJobID(ctx, req.ProjectID, req.JobID)
	if err != nil {
		return ErrJobNotFound
	}

	// Stop in Kuscia first (best-effort)
	if s.kusciaClient != nil {
		_ = s.kusciaClient.StopJob(ctx, req.JobID)
	}

	// Update job status to STOPPED
	if err := s.jobRepo.UpdateStatus(ctx, job.JobID, "STOPPED", "stopped by user"); err != nil {
		return err
	}

	// Stop specific task or all tasks
	if req.TaskID != "" {
		return s.taskRepo.UpdateStatus(ctx, req.TaskID, "STOPPED", "stopped by user")
	}

	// Stop all running tasks
	tasks, _ := s.taskRepo.FindByJobID(ctx, req.JobID)
	for _, t := range tasks {
		if t.Status == "RUNNING" || t.Status == "PENDING" {
			_ = s.taskRepo.UpdateStatus(ctx, t.TaskID, "STOPPED", "stopped by user")
		}
	}

	return nil
}

// GetTaskLogs retrieves logs for a task.
func (s *JobService) GetTaskLogs(ctx context.Context, req *GetTaskLogRequest) (*TaskLogVO, error) {
	logs, err := s.taskLogRepo.FindByTaskID(ctx, req.ProjectID, req.JobID, req.TaskID)
	if err != nil {
		return nil, err
	}

	logStrs := make([]string, 0, len(logs))
	for _, l := range logs {
		logStrs = append(logStrs, l.Log)
	}

	return &TaskLogVO{
		TaskID: req.TaskID,
		Logs:   logStrs,
	}, nil
}

func (s *JobService) toJobVO(job *model.ProjectJobDO, tasks []model.ProjectJobTaskDO) JobVO {
	vo := JobVO{
		JobID:     job.JobID,
		ProjectID: job.ProjectID,
		Name:      job.Name,
		Status:    job.Status,
		ErrMsg:    job.ErrMsg,
		GraphID:   job.GraphID,
		GmtCreate: job.GmtCreate.Format("2006-01-02 15:04:05"),
	}

	if job.FinishedTime != nil {
		vo.FinishedTime = job.FinishedTime.Format("2006-01-02 15:04:05")
	}

	if tasks != nil {
		vo.Tasks = make([]TaskVO, 0, len(tasks))
		for _, t := range tasks {
			vo.Tasks = append(vo.Tasks, TaskVO{
				TaskID:      t.TaskID,
				JobID:       t.JobID,
				Parties:     t.Parties,
				Status:      t.Status,
				ErrMsg:      t.ErrMsg,
				GraphNodeID: t.GraphNodeID,
			})
		}
	}

	return vo
}
