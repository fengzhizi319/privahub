package service

import (
	"context"
	"encoding/json"
	"errors"
	"sort"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/internal/dao/repository"
	"github.com/fengzhizi319/privahub/internal/graphjob"
	"github.com/fengzhizi319/privahub/pkg/errcode"
	"github.com/fengzhizi319/privahub/pkg/kuscia"
	"gorm.io/gorm"
)

// Job service errors.
var (
	ErrJobNotFound = errors.New("job not found")
)

// JobService handles project job queries and control (Java ProjectServiceImpl
// job part: listProjectJob / getProjectJob / stopProjectJob / task logs / output).
type JobService struct {
	jobRepo       repository.JobRepository
	taskRepo      repository.TaskRepository
	taskLogRepo   repository.TaskLogRepository
	graphRepo     repository.GraphRepository
	graphNodeRepo repository.GraphNodeRepository
	kuscia        GraphKusciaAPI
	db            *gorm.DB
}

// NewJobService creates a new JobService.
func NewJobService(
	jobRepo repository.JobRepository,
	taskRepo repository.TaskRepository,
	taskLogRepo repository.TaskLogRepository,
	graphRepo repository.GraphRepository,
	graphNodeRepo repository.GraphNodeRepository,
	kusciaClient *kuscia.Client,
	db *gorm.DB,
) *JobService {
	return &JobService{
		jobRepo:       jobRepo,
		taskRepo:      taskRepo,
		taskLogRepo:   taskLogRepo,
		graphRepo:     graphRepo,
		graphNodeRepo: graphNodeRepo,
		kuscia:        toGraphKuscia(kusciaClient),
		db:            db,
	}
}

// SetKuscia overrides the Kuscia API (tests).
func (s *JobService) SetKuscia(k GraphKusciaAPI) { s.kuscia = k }

// --- DTOs ---

// JobListRequest mirrors Java ListProjectJobRequest (page / size are legacy aliases).
type JobListRequest struct {
	// Optional filters are applied before pagination, never just to the current page.
	SearchKey string `json:"searchKey,omitempty"`
	Status    string `json:"status,omitempty"`
	StartTime string `json:"startTime,omitempty"`
	EndTime   string `json:"endTime,omitempty"`

	ProjectID string `json:"projectId"`
	GraphID   string `json:"graphId"`
	PageNum   int    `json:"pageNum"`
	PageSize  int    `json:"pageSize"`
	Page      int    `json:"page,omitempty"`
	Size      int    `json:"size,omitempty"`
}

// JobGraphRef is a Go extension on job summaries (graph name for the job list).
type JobGraphRef struct {
	GraphID string `json:"graphId"`
	Name    string `json:"name"`
}

// ProjectJobSummaryVO mirrors Java ProjectJobSummaryVO.
type ProjectJobSummaryVO struct {
	JobID             string       `json:"jobId"`
	Status            string       `json:"status"`
	ErrMsg            string       `json:"errMsg,omitempty"`
	GmtCreate         string       `json:"gmtCreate"`
	GmtModified       string       `json:"gmtModified,omitempty"`
	GmtFinished       string       `json:"gmtFinished,omitempty"`
	TableCount        int64        `json:"tableCount"`
	ModelCount        int64        `json:"modelCount"`
	RuleCount         int64        `json:"ruleCount"`
	ReportCount       int64        `json:"reportCount"`
	FinishedTaskCount int64        `json:"finishedTaskCount"`
	TaskCount         int64        `json:"taskCount"`
	Finished          bool         `json:"finished"`
	Graph             *JobGraphRef `json:"graph,omitempty"`
}

// JobListResponse mirrors Java PageResponse (total is a Go extension).
type JobListResponse struct {
	PageTotal int                   `json:"pageTotal"`
	PageSize  int                   `json:"pageSize"`
	Data      []ProjectJobSummaryVO `json:"data"`
	Total     int64                 `json:"total"`
}

// GetJobRequest mirrors GetProjectJobRequest.
type GetJobRequest struct {
	ProjectID string `json:"projectId"`
	JobID     string `json:"jobId"`
}

// ProjectJobVO mirrors Java ProjectJobVO.
type ProjectJobVO struct {
	JobID       string         `json:"jobId"`
	Status      string         `json:"status"`
	ErrMsg      string         `json:"errMsg,omitempty"`
	GmtCreate   string         `json:"gmtCreate"`
	GmtModified string         `json:"gmtModified,omitempty"`
	GmtFinished string         `json:"gmtFinished,omitempty"`
	Finished    bool           `json:"finished"`
	Graph       *GraphDetailVO `json:"graph"`
}

// StopJobRequest mirrors StopProjectJobTaskRequest.
type StopJobRequest struct {
	ProjectID string `json:"projectId"`
	JobID     string `json:"jobId"`
	TaskID    string `json:"taskId,omitempty"`
}

// GetTaskLogRequest mirrors GetProjectJobTaskLogRequest.
type GetTaskLogRequest struct {
	ProjectID string `json:"projectId"`
	JobID     string `json:"jobId"`
	TaskID    string `json:"taskId"`
}

// GetTaskOutputRequest mirrors GetProjectJobTaskOutputRequest.
type GetTaskOutputRequest struct {
	ProjectID string `json:"projectId"`
	JobID     string `json:"jobId"`
	TaskID    string `json:"taskId"`
	OutputID  string `json:"outputId"`
}

// --- job lookup (project_job, falling back to project_schedule_job) ---

type jobRecord struct {
	job   model.ProjectJobDO
	tasks []model.ProjectJobTaskDO
}

func openProjectJob(ctx context.Context, db *gorm.DB, projectID, jobID string) (*jobRecord, error) {
	rec := &jobRecord{}
	q := db.WithContext(ctx).Where("job_id = ?", jobID)
	if projectID != "" {
		q = q.Where("project_id = ?", projectID)
	}
	if err := q.First(&rec.job).Error; err != nil {
		var sj model.ProjectScheduleJobDO
		sq := db.WithContext(ctx).Where("job_id = ?", jobID)
		if projectID != "" {
			sq = sq.Where("project_id = ?", projectID)
		}
		if sq.First(&sj).Error != nil {
			return nil, padErrBase(ErrJobNotFound, errcode.ProjectJobNotExists)
		}
		rec.job = model.ProjectJobDO{
			BaseDO: sj.BaseDO, ProjectID: sj.ProjectID, JobID: sj.JobID, Name: sj.Name, Status: sj.Status,
			ErrMsg: sj.ErrMsg, GraphID: sj.GraphID, Edges: sj.Edges, FinishedTime: sj.FinishedTime,
		}
	}
	if err := db.WithContext(ctx).Where("project_id = ? AND job_id = ?", rec.job.ProjectID, jobID).Order("id ASC").Find(&rec.tasks).Error; err != nil {
		return nil, err
	}
	return rec, nil
}

// --- Service Methods ---

// ListJobs mirrors ProjectServiceImpl.listProjectJob (DB paging, graph filter, counts).
func (s *JobService) ListJobs(ctx context.Context, req *JobListRequest) (*JobListResponse, error) {
	page, size := req.PageNum, req.PageSize
	if page < 1 {
		page = req.Page
	}
	if size < 1 {
		size = req.Size
	}
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 10
	}
	if size > 100 {
		size = 100
	}
	q := s.db.WithContext(ctx).Model(&model.ProjectJobDO{}).Where("project_id = ?", req.ProjectID)
	if req.GraphID != "" {
		q = q.Where("graph_id = ?", req.GraphID)
	}
	var filterErr error
	q, filterErr = applyJobListFilters(q, req.ProjectID, req.SearchKey, req.Status, req.StartTime, req.EndTime)
	if filterErr != nil {
		return nil, filterErr
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, err
	}
	var jobs []model.ProjectJobDO
	if err := q.Order("gmt_create DESC").Order("id DESC").Offset((page - 1) * size).Limit(size).Find(&jobs).Error; err != nil {
		return nil, err
	}
	resp := &JobListResponse{PageSize: size, Data: []ProjectJobSummaryVO{}, Total: total}
	resp.PageTotal = int((total + int64(size) - 1) / int64(size))
	if len(jobs) == 0 {
		if resp.PageTotal == 0 {
			resp.PageTotal = 1
		}
		return resp, nil
	}
	ids := make([]string, 0, len(jobs))
	for _, j := range jobs {
		ids = append(ids, j.JobID)
	}
	counts := func(kind string) map[string]int64 {
		type row struct {
			JobID string
			Cnt   int64
		}
		var rows []row
		s.db.WithContext(ctx).Model(&model.ProjectResultDO{}).
			Select("job_id, COUNT(DISTINCT ref_id) AS cnt").
			Where("project_id = ? AND job_id IN ? AND kind = ?", req.ProjectID, ids, kind).
			Group("job_id").Scan(&rows)
		m := map[string]int64{}
		for _, r := range rows {
			m[r.JobID] = r.Cnt
		}
		return m
	}
	taskCounts := func(status string) map[string]int64 {
		type row struct {
			JobID string
			Cnt   int64
		}
		var rows []row
		tq := s.db.WithContext(ctx).Model(&model.ProjectJobTaskDO{}).Select("job_id, COUNT(*) AS cnt").
			Where("project_id = ? AND job_id IN ?", req.ProjectID, ids)
		if status != "" {
			tq = tq.Where("status = ?", status)
		}
		tq.Group("job_id").Scan(&rows)
		m := map[string]int64{}
		for _, r := range rows {
			m[r.JobID] = r.Cnt
		}
		return m
	}
	tables, models, rules, reports := counts(graphjob.ResultFedTable), counts(graphjob.ResultModel), counts(graphjob.ResultRule), counts(graphjob.ResultReport)
	finished, all := taskCounts(graphjob.TaskSucceed), taskCounts("")
	graphNames := map[string]string{}
	for _, j := range jobs {
		vo := ProjectJobSummaryVO{
			JobID:             j.JobID,
			Status:            graphjob.NormalizeJobStatus(j.Status),
			ErrMsg:            j.ErrMsg,
			GmtCreate:         rfc3339(j.GmtCreate),
			GmtModified:       rfc3339(j.GmtModified),
			TableCount:        tables[j.JobID],
			ModelCount:        models[j.JobID],
			RuleCount:         rules[j.JobID],
			ReportCount:       reports[j.JobID],
			FinishedTaskCount: finished[j.JobID],
			TaskCount:         all[j.JobID],
			Finished:          graphjob.IsJobFinished(j.Status),
		}
		if j.FinishedTime != nil {
			vo.GmtFinished = rfc3339(*j.FinishedTime)
		}
		if j.GraphID != "" {
			name, ok := graphNames[j.GraphID]
			if !ok {
				var g model.ProjectGraphDO
				if s.db.WithContext(ctx).Where("project_id = ? AND graph_id = ?", j.ProjectID, j.GraphID).First(&g).Error == nil {
					name = g.Name
				} else {
					name = j.Name
				}
				graphNames[j.GraphID] = name
			}
			vo.Graph = &JobGraphRef{GraphID: j.GraphID, Name: name}
		}
		resp.Data = append(resp.Data, vo)
	}
	return resp, nil
}

// GetJob mirrors ProjectServiceImpl.getProjectJob: the graph is rebuilt from
// the job's edge snapshot and each task's graph_node snapshot.
func (s *JobService) GetJob(ctx context.Context, req *GetJobRequest) (*ProjectJobVO, error) {
	var cnt int64
	s.db.WithContext(ctx).Model(&model.ProjectDO{}).Where("project_id = ?", req.ProjectID).Count(&cnt)
	if cnt == 0 {
		return nil, padErrBase(ErrProjectNotFound, errcode.ProjectNotExists)
	}
	rec, err := openProjectJob(ctx, s.db, req.ProjectID, req.JobID)
	if err != nil {
		return nil, err
	}
	var results []model.ProjectResultDO
	s.db.WithContext(ctx).Where("project_id = ? AND job_id = ?", req.ProjectID, req.JobID).Order("id ASC").Find(&results)
	byTask := map[string][]ProjectResultBaseVO{}
	seen := map[string]bool{}
	for _, r := range results {
		key := r.TaskID + "|" + r.Kind + "|" + r.RefID // MergedProjectResult merges parties
		if seen[key] {
			continue
		}
		seen[key] = true
		byTask[r.TaskID] = append(byTask[r.TaskID], ProjectResultBaseVO{Kind: r.Kind, RefID: r.RefID})
	}
	nodes := make([]GraphNodeDetail, 0, len(rec.tasks))
	for i := range rec.tasks {
		t := &rec.tasks[i]
		info := loadTaskNodeInfo(ctx, s.db, t, rec.job.GraphID)
		if string(info.NodeDef) == "" || string(info.NodeDef) == "null" {
			info.NodeDef = json.RawMessage("{}")
		}
		nodes = append(nodes, GraphNodeDetail{
			GraphNodeInfo: info,
			Status:        graphjob.NormalizeTaskStatus(t.Status),
			JobID:         t.JobID,
			TaskID:        t.TaskID,
			Results:       byTask[t.TaskID],
			Parties:       nodeSimpleInfos(ctx, s.db, taskParties(t)),
			Progress:      taskProgress(t),
		})
	}
	edges := graphjob.ParseEdges(rec.job.Edges)
	if edges == nil {
		edges = []graphjob.GraphEdge{}
	}
	vo := &ProjectJobVO{
		JobID:       rec.job.JobID,
		Status:      graphjob.NormalizeJobStatus(rec.job.Status),
		ErrMsg:      rec.job.ErrMsg,
		GmtCreate:   rfc3339(rec.job.GmtCreate),
		GmtModified: rfc3339(rec.job.GmtModified),
		Finished:    graphjob.IsJobFinished(rec.job.Status),
		Graph: &GraphDetailVO{
			ProjectID: rec.job.ProjectID,
			GraphID:   rec.job.GraphID,
			Name:      rec.job.Name,
			Edges:     edges,
			Nodes:     nodes,
		},
	}
	if rec.job.FinishedTime != nil {
		vo.GmtFinished = rfc3339(*rec.job.FinishedTime)
	}
	return vo, nil
}

// StopJob mirrors ProjectServiceImpl.stopProjectJob (graph owner check).
func (s *JobService) StopJob(ctx context.Context, req *StopJobRequest) error {
	return stopProjectJob(ctx, s.db, s.kuscia, req.ProjectID, req.JobID, true)
}

// stopProjectJob stops a job: RUNNING / INITIALIZED tasks become STOPPED with
// stop logs, the job becomes STOPPED and Kuscia StopJob is called.
func stopProjectJob(ctx context.Context, db *gorm.DB, k GraphKusciaAPI, projectID, jobID string, checkOwner bool) error {
	var job model.ProjectJobDO
	q := db.WithContext(ctx).Where("job_id = ?", jobID)
	if projectID != "" {
		q = q.Where("project_id = ?", projectID)
	}
	if err := q.First(&job).Error; err != nil {
		return padErrBase(ErrJobNotFound, errcode.ProjectJobNotExists)
	}
	if checkOwner && job.GraphID != "" {
		var g model.ProjectGraphDO
		if err := db.WithContext(ctx).Where("project_id = ? AND graph_id = ?", job.ProjectID, job.GraphID).First(&g).Error; err != nil {
			return padErrBase(ErrGraphNotFound, errcode.GraphNotExists)
		}
		if err := ownerCheck(ctx, &g); err != nil {
			return err
		}
	}
	if k != nil {
		// Best effort: a job already gone from Kuscia is still stopped locally.
		_ = k.StopJob(ctx, jobID)
	}
	now := timeNow()
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var tasks []model.ProjectJobTaskDO
		if err := tx.Where("project_id = ? AND job_id = ?", job.ProjectID, jobID).Find(&tasks).Error; err != nil {
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
				if err := tx.Create(&model.ProjectJobTaskLogDO{ProjectID: job.ProjectID, JobID: jobID, TaskID: t.TaskID, Log: l}).Error; err != nil {
					return err
				}
			}
		}
		return tx.Model(&model.ProjectJobDO{}).Where("id = ?", job.ID).
			Updates(map[string]any{"status": graphjob.JobStopped}).Error
	})
}

// GetTaskLogs mirrors ProjectServiceImpl.getProjectJobTaskLogs.
func (s *JobService) GetTaskLogs(ctx context.Context, req *GetTaskLogRequest) (*GraphNodeTaskLogsVO, error) {
	rec, err := openProjectJob(ctx, s.db, req.ProjectID, req.JobID)
	if err != nil {
		return nil, err
	}
	var task *model.ProjectJobTaskDO
	for i := range rec.tasks {
		if rec.tasks[i].TaskID == req.TaskID {
			task = &rec.tasks[i]
		}
	}
	if task == nil {
		return nil, padErr(errcode.ProjectJobTaskNotExists)
	}
	logs, err := taskLogLines(ctx, s.db, req.JobID, req.TaskID)
	if err != nil {
		return nil, err
	}
	prefix := "INFO the jobId=" + req.JobID + ", taskId=" + req.TaskID
	logs = graphjob.DistinctSpecifyLogs(logs, prefix+" start")
	logs = graphjob.DistinctSpecifyLogs(logs, prefix+" succeed")
	return &GraphNodeTaskLogsVO{Status: graphjob.NormalizeTaskStatus(task.Status), Logs: logs}, nil
}

// GetTaskOutput mirrors GraphServiceImpl.getGraphNodeTaskOutputVO(request).
func (s *JobService) GetTaskOutput(ctx context.Context, req *GetTaskOutputRequest) (*GraphNodeOutputVO, error) {
	rec, err := openProjectJob(ctx, s.db, req.ProjectID, req.JobID)
	if err != nil {
		return nil, err
	}
	for i := range rec.tasks {
		if rec.tasks[i].TaskID == req.TaskID {
			return newOutputRenderer(s.db, s.kuscia).taskOutput(ctx, &rec.tasks[i], req.OutputID)
		}
	}
	return nil, padErr(errcode.ProjectJobTaskNotExists)
}

// sortedKeys returns the sorted keys of a set.
func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
