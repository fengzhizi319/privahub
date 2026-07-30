package service

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/internal/dao/repository"
	"github.com/fengzhizi319/privahub/pkg/kuscia"
	"github.com/google/uuid"
)

// Job service errors.
var (
	ErrJobNotFound = errors.New("job not found")
)

// JobService handles Kuscia job management.
type JobService struct {
	jobRepo       repository.JobRepository
	taskRepo      repository.TaskRepository
	taskLogRepo   repository.TaskLogRepository
	graphRepo     repository.GraphRepository
	graphNodeRepo repository.GraphNodeRepository
	kusciaClient  *kuscia.Client
}

// NewJobService creates a new JobService.
func NewJobService(
	jobRepo repository.JobRepository,
	taskRepo repository.TaskRepository,
	taskLogRepo repository.TaskLogRepository,
	graphRepo repository.GraphRepository,
	graphNodeRepo repository.GraphNodeRepository,
	kusciaClient *kuscia.Client,
) *JobService {
	return &JobService{
		jobRepo:       jobRepo,
		taskRepo:      taskRepo,
		taskLogRepo:   taskLogRepo,
		graphRepo:     graphRepo,
		graphNodeRepo: graphNodeRepo,
		kusciaClient:  kusciaClient,
	}
}

// --- Request / Response DTOs ---

// CreateJobRequest represents a job creation request.
type CreateJobRequest struct {
	ProjectID    string `json:"project_id"`
	ProjectIDAlt string `json:"projectId"`
	Name         string `json:"name"`
	GraphID      string `json:"graph_id"`
	GraphIDAlt   string `json:"graphId"`
	Edges        string `json:"edges"`
}

// JobVO represents a job view object. Field names follow the Java SecretPad
// camelCase contract consumed by the frontend (ProjectJobVOSchema).
type JobVO struct {
	JobID       string      `json:"jobId"`
	ProjectID   string      `json:"projectId"`
	Name        string      `json:"name"`
	Status      string      `json:"status"`
	ErrMsg      string      `json:"errMsg,omitempty"`
	GraphID     string      `json:"graphId,omitempty"`
	Tasks       []TaskVO    `json:"tasks,omitempty"`
	GmtCreate   string      `json:"gmtCreate"`
	GmtModified string      `json:"gmtModified,omitempty"`
	GmtFinished string      `json:"gmtFinished,omitempty"`
	Finished    bool        `json:"finished"`
	Graph       *JobGraphVO `json:"graph,omitempty"`
}

// TaskVO represents a task view object.
type TaskVO struct {
	TaskID      string `json:"taskId"`
	JobID       string `json:"jobId"`
	Parties     string `json:"parties,omitempty"`
	Status      string `json:"status"`
	ErrMsg      string `json:"errMsg,omitempty"`
	GraphNodeID string `json:"graphNodeId,omitempty"`
}

// JobGraphVO is a task-oriented view of the DAG graph attached to a job detail.
type JobGraphVO struct {
	GraphID string           `json:"graphId"`
	Name    string           `json:"name"`
	Nodes   []JobGraphNodeVO `json:"nodes"`
}

// JobGraphNodeVO is a graph node enriched with its runtime task status.
type JobGraphNodeVO struct {
	GraphNodeID string   `json:"graphNodeId"`
	CodeName    string   `json:"codeName,omitempty"`
	Label       string   `json:"label,omitempty"`
	Status      string   `json:"status,omitempty"`
	Outputs     []string `json:"outputs,omitempty"`
}

// JobListRequest represents a job list request. The frontend sends pageNum and
// pageSize (see getProjectJobs in client.ts); page/size are kept for compatibility.
type JobListRequest struct {
	ProjectID    string `json:"project_id"`
	ProjectIDAlt string `json:"projectId"`
	Page         int    `json:"page"`
	PageAlt      int    `json:"pageNum"`
	Size         int    `json:"size"`
	SizeAlt      int    `json:"pageSize"`
}

// JobListResponse represents a paginated job list. The Data field matches the
// frontend's expected `data` key (see getProjectJobs in client.ts).
type JobListResponse struct {
	Data      []JobVO `json:"data"`
	Total     int64   `json:"total"`
	PageSize  int     `json:"pageSize"`
	PageTotal int     `json:"pageTotal"`
}

// GetJobRequest represents a job detail request.
type GetJobRequest struct {
	ProjectID    string `json:"project_id"`
	ProjectIDAlt string `json:"projectId"`
	JobID        string `json:"job_id"`
	JobIDAlt     string `json:"jobId"`
}

// StopJobRequest represents a job stop request.
type StopJobRequest struct {
	ProjectID    string `json:"project_id"`
	ProjectIDAlt string `json:"projectId"`
	JobID        string `json:"job_id"`
	JobIDAlt     string `json:"jobId"`
	TaskID       string `json:"task_id"`
	TaskIDAlt    string `json:"taskId"`
}

// GetTaskLogRequest represents a task log request.
type GetTaskLogRequest struct {
	ProjectID    string `json:"project_id"`
	ProjectIDAlt string `json:"projectId"`
	JobID        string `json:"job_id"`
	JobIDAlt     string `json:"jobId"`
	TaskID       string `json:"task_id"`
	TaskIDAlt    string `json:"taskId"`
}

// GetTaskOutputRequest represents a task output request. Field names follow the
// Java SecretPad GetProjectJobTaskOutputRequest contract.
type GetTaskOutputRequest struct {
	ProjectID    string `json:"project_id"`
	ProjectIDAlt string `json:"projectId"`
	JobID        string `json:"job_id"`
	JobIDAlt     string `json:"jobId"`
	TaskID       string `json:"task_id"`
	TaskIDAlt    string `json:"taskId"`
	OutputID     string `json:"output_id"`
	OutputIDAlt  string `json:"outputId"`
}

// TaskLogVO represents task log entries.
type TaskLogVO struct {
	TaskID string   `json:"taskId"`
	Logs   []string `json:"logs"`
}

// TaskOutputVO represents a task's output. Field names follow the frontend
// GraphNodeOutputVOSchema (type/codeName/meta/...).
type TaskOutputVO struct {
	Type      string                 `json:"type,omitempty"`
	CodeName  string                 `json:"codeName,omitempty"`
	Meta      map[string]interface{} `json:"meta,omitempty"`
	JobID     string                 `json:"jobId,omitempty"`
	TaskID    string                 `json:"taskId,omitempty"`
	GraphID   string                 `json:"graphID,omitempty"`
	GmtCreate string                 `json:"gmtCreate,omitempty"`
}

// --- Service Methods ---

// CreateJob creates a new job and submits it to Kuscia.
func (s *JobService) CreateJob(ctx context.Context, req *CreateJobRequest) (*JobVO, error) {
	if req.ProjectID == "" {
		req.ProjectID = req.ProjectIDAlt
	}
	if req.GraphID == "" {
		req.GraphID = req.GraphIDAlt
	}
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
					AppImage: "secretflow-image",
					Alias:    req.Name,
					Parties: []kuscia.Party{
						{DomainID: "alice", Role: "guest"},
						{DomainID: "bob", Role: "host"},
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
	if req.ProjectID == "" {
		req.ProjectID = req.ProjectIDAlt
	}
	if req.Page < 1 {
		req.Page = req.PageAlt
	}
	if req.Size < 1 {
		req.Size = req.SizeAlt
	}
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
		vo := s.toJobVO(&j, nil)
		// Populate the graph name so the frontend job list can display it
		// (getProjectJobs reads j.graph?.name).
		if s.graphRepo != nil && j.GraphID != "" {
			if g, gerr := s.graphRepo.FindByProjectAndGraphID(ctx, j.ProjectID, j.GraphID); gerr == nil && g != nil {
				vo.Graph = &JobGraphVO{GraphID: g.GraphID, Name: g.Name}
			}
		}
		result = append(result, vo)
	}

	pageTotal := int((total + int64(req.Size) - 1) / int64(req.Size))

	return &JobListResponse{
		Data:      result,
		Total:     total,
		PageSize:  req.Size,
		PageTotal: pageTotal,
	}, nil
}

// GetJob retrieves job detail with tasks.
func (s *JobService) GetJob(ctx context.Context, req *GetJobRequest) (*JobVO, error) {
	if req.ProjectID == "" {
		req.ProjectID = req.ProjectIDAlt
	}
	if req.JobID == "" {
		req.JobID = req.JobIDAlt
	}
	job, err := s.jobRepo.FindByProjectAndJobID(ctx, req.ProjectID, req.JobID)
	if err != nil {
		return nil, ErrJobNotFound
	}

	tasks, err := s.taskRepo.FindByProjectAndJobID(ctx, req.ProjectID, req.JobID)
	if err != nil {
		tasks = nil // non-fatal
	}

	vo := s.toJobVO(job, tasks)
	vo.Graph = s.buildJobGraph(ctx, job, tasks)
	return &vo, nil
}

// StopJob stops a running job both locally and in Kuscia.
func (s *JobService) StopJob(ctx context.Context, req *StopJobRequest) error {
	if req.ProjectID == "" {
		req.ProjectID = req.ProjectIDAlt
	}
	if req.JobID == "" {
		req.JobID = req.JobIDAlt
	}
	if req.TaskID == "" {
		req.TaskID = req.TaskIDAlt
	}
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
	if req.ProjectID == "" {
		req.ProjectID = req.ProjectIDAlt
	}
	if req.JobID == "" {
		req.JobID = req.JobIDAlt
	}
	if req.TaskID == "" {
		req.TaskID = req.TaskIDAlt
	}
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

// GetTaskOutput retrieves the output of a job task. It resolves the task's
// graph node for its codeName and, when Kuscia is reachable, enriches the
// result with the output's DomainData metadata (type/name/columns). All
// enrichment steps are best-effort so the endpoint degrades gracefully.
func (s *JobService) GetTaskOutput(ctx context.Context, req *GetTaskOutputRequest) (*TaskOutputVO, error) {
	if req.ProjectID == "" {
		req.ProjectID = req.ProjectIDAlt
	}
	if req.JobID == "" {
		req.JobID = req.JobIDAlt
	}
	if req.TaskID == "" {
		req.TaskID = req.TaskIDAlt
	}
	if req.OutputID == "" {
		req.OutputID = req.OutputIDAlt
	}

	vo := &TaskOutputVO{
		JobID:  req.JobID,
		TaskID: req.TaskID,
		Type:   "table",
	}

	// Resolve the task's graph node to obtain the codeName and graph id.
	if task, err := s.taskRepo.FindByTaskID(ctx, req.TaskID); err == nil && task != nil {
		if job, jerr := s.jobRepo.FindByProjectAndJobID(ctx, req.ProjectID, req.JobID); jerr == nil && job != nil {
			vo.GraphID = job.GraphID
			if s.graphNodeRepo != nil {
				if node, nerr := s.graphNodeRepo.FindByGraphNodeID(ctx, req.ProjectID, job.GraphID, task.GraphNodeID); nerr == nil && node != nil {
					vo.CodeName = node.CodeName
				}
			}
		}
	}

	// Best-effort: query Kuscia DomainData for the output's metadata.
	if s.kusciaClient != nil && req.OutputID != "" {
		if resp, err := s.kusciaClient.QueryDomainData(ctx, "alice", req.OutputID); err == nil {
			if resp.Data.Type != "" {
				vo.Type = resp.Data.Type
			}
			meta := map[string]interface{}{
				"name":         resp.Data.Name,
				"datasourceId": resp.Data.DatasourceID,
			}
			if len(resp.Data.Columns) > 0 {
				cols := make([]map[string]string, 0, len(resp.Data.Columns))
				for _, col := range resp.Data.Columns {
					cols = append(cols, map[string]string{"name": col.Name, "type": col.Type})
				}
				meta["columns"] = cols
			}
			vo.Meta = meta
		}
	}

	return vo, nil
}

func (s *JobService) toJobVO(job *model.ProjectJobDO, tasks []model.ProjectJobTaskDO) JobVO {
	vo := JobVO{
		JobID:       job.JobID,
		ProjectID:   job.ProjectID,
		Name:        job.Name,
		Status:      job.Status,
		ErrMsg:      job.ErrMsg,
		GraphID:     job.GraphID,
		GmtCreate:   job.GmtCreate.Format("2006-01-02 15:04:05"),
		GmtModified: job.GmtModified.Format("2006-01-02 15:04:05"),
		Finished:    job.FinishedTime != nil,
	}

	if job.FinishedTime != nil {
		vo.GmtFinished = job.FinishedTime.Format("2006-01-02 15:04:05")
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

// buildJobGraph loads the DAG graph bound to the job and enriches each node
// with its runtime task status. It returns nil (non-fatal) when the graph
// repositories are unavailable, the job has no graph, or loading fails.
func (s *JobService) buildJobGraph(ctx context.Context, job *model.ProjectJobDO, tasks []model.ProjectJobTaskDO) *JobGraphVO {
	if s.graphRepo == nil || s.graphNodeRepo == nil || job.GraphID == "" {
		return nil
	}

	graph, err := s.graphRepo.FindByProjectAndGraphID(ctx, job.ProjectID, job.GraphID)
	if err != nil {
		return nil
	}

	nodes, err := s.graphNodeRepo.FindByGraphID(ctx, job.ProjectID, job.GraphID)
	if err != nil {
		return nil
	}

	// Map graph node id -> task status for runtime enrichment.
	statusByNode := make(map[string]string, len(tasks))
	for _, t := range tasks {
		if t.GraphNodeID != "" {
			statusByNode[t.GraphNodeID] = t.Status
		}
	}

	nodeVOs := make([]JobGraphNodeVO, 0, len(nodes))
	for _, n := range nodes {
		nodeVOs = append(nodeVOs, JobGraphNodeVO{
			GraphNodeID: n.GraphNodeID,
			CodeName:    n.CodeName,
			Label:       n.Label,
			Status:      statusByNode[n.GraphNodeID],
			Outputs:     splitOutputs(n.Outputs),
		})
	}

	return &JobGraphVO{
		GraphID: graph.GraphID,
		Name:    graph.Name,
		Nodes:   nodeVOs,
	}
}

// splitOutputs parses a node's Outputs field, which is stored as a JSON array
// string (e.g. `["alice/output-0"]`), into a []string.
func splitOutputs(raw string) []string {
	if raw == "" {
		return nil
	}
	var out []string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil
	}
	return out
}
