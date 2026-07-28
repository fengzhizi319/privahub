package repository

import (
	"context"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"gorm.io/gorm"
)

// JobRepo is the GORM implementation of JobRepository.
type JobRepo struct {
	*BaseRepo[model.ProjectJobDO]
}

// NewJobRepo creates a new JobRepo.
func NewJobRepo(db *gorm.DB) *JobRepo {
	return &JobRepo{BaseRepo: NewBaseRepo[model.ProjectJobDO](db)}
}

// FindByJobID retrieves a job by job_id.
func (r *JobRepo) FindByJobID(ctx context.Context, jobID string) (*model.ProjectJobDO, error) {
	var job model.ProjectJobDO
	err := r.DB().WithContext(ctx).Where("job_id = ?", jobID).First(&job).Error
	if err != nil {
		return nil, err
	}
	return &job, nil
}

// FindByProjectID retrieves all jobs for a project.
func (r *JobRepo) FindByProjectID(ctx context.Context, projectID string) ([]model.ProjectJobDO, error) {
	var jobs []model.ProjectJobDO
	err := r.DB().WithContext(ctx).Where("project_id = ?", projectID).Order("gmt_create DESC").Find(&jobs).Error
	return jobs, err
}

// FindByProjectAndJobID retrieves a job by project and job ID.
func (r *JobRepo) FindByProjectAndJobID(ctx context.Context, projectID, jobID string) (*model.ProjectJobDO, error) {
	var job model.ProjectJobDO
	err := r.DB().WithContext(ctx).Where("project_id = ? AND job_id = ?", projectID, jobID).First(&job).Error
	if err != nil {
		return nil, err
	}
	return &job, nil
}

// UpdateStatus updates a job's status and error message.
func (r *JobRepo) UpdateStatus(ctx context.Context, jobID, status, errMsg string) error {
	updates := map[string]interface{}{"status": status}
	if errMsg != "" {
		updates["err_msg"] = errMsg
	}
	return r.DB().WithContext(ctx).Model(&model.ProjectJobDO{}).Where("job_id = ?", jobID).Updates(updates).Error
}

// TaskRepo is the GORM implementation of TaskRepository.
type TaskRepo struct {
	*BaseRepo[model.ProjectJobTaskDO]
}

// NewTaskRepo creates a new TaskRepo.
func NewTaskRepo(db *gorm.DB) *TaskRepo {
	return &TaskRepo{BaseRepo: NewBaseRepo[model.ProjectJobTaskDO](db)}
}

// FindByJobID retrieves all tasks for a job.
func (r *TaskRepo) FindByJobID(ctx context.Context, jobID string) ([]model.ProjectJobTaskDO, error) {
	var tasks []model.ProjectJobTaskDO
	err := r.DB().WithContext(ctx).Where("job_id = ?", jobID).Find(&tasks).Error
	return tasks, err
}

// FindByProjectAndJobID retrieves all tasks for a project's job.
func (r *TaskRepo) FindByProjectAndJobID(ctx context.Context, projectID, jobID string) ([]model.ProjectJobTaskDO, error) {
	var tasks []model.ProjectJobTaskDO
	err := r.DB().WithContext(ctx).Where("project_id = ? AND job_id = ?", projectID, jobID).Find(&tasks).Error
	return tasks, err
}

// FindByTaskID retrieves a task by task_id.
func (r *TaskRepo) FindByTaskID(ctx context.Context, taskID string) (*model.ProjectJobTaskDO, error) {
	var task model.ProjectJobTaskDO
	err := r.DB().WithContext(ctx).Where("task_id = ?", taskID).First(&task).Error
	if err != nil {
		return nil, err
	}
	return &task, nil
}

// UpdateStatus updates a task's status and error message.
func (r *TaskRepo) UpdateStatus(ctx context.Context, taskID, status, errMsg string) error {
	updates := map[string]interface{}{"status": status}
	if errMsg != "" {
		updates["err_msg"] = errMsg
	}
	return r.DB().WithContext(ctx).Model(&model.ProjectJobTaskDO{}).Where("task_id = ?", taskID).Updates(updates).Error
}

// TaskLogRepo is the GORM implementation of TaskLogRepository.
type TaskLogRepo struct {
	db *gorm.DB
}

// NewTaskLogRepo creates a new TaskLogRepo.
func NewTaskLogRepo(db *gorm.DB) *TaskLogRepo {
	return &TaskLogRepo{db: db}
}

// Create inserts a new task log entry.
func (r *TaskLogRepo) Create(ctx context.Context, log *model.ProjectJobTaskLogDO) error {
	return r.db.WithContext(ctx).Create(log).Error
}

// FindByTaskID retrieves all logs for a task.
func (r *TaskLogRepo) FindByTaskID(ctx context.Context, projectID, jobID, taskID string) ([]model.ProjectJobTaskLogDO, error) {
	var logs []model.ProjectJobTaskLogDO
	err := r.db.WithContext(ctx).
		Where("project_id = ? AND job_id = ? AND task_id = ?", projectID, jobID, taskID).
		Order("gmt_create ASC").
		Find(&logs).Error
	return logs, err
}

// GraphRepo is the GORM implementation of GraphRepository.
type GraphRepo struct {
	*BaseRepo[model.ProjectGraphDO]
}

// NewGraphRepo creates a new GraphRepo.
func NewGraphRepo(db *gorm.DB) *GraphRepo {
	return &GraphRepo{BaseRepo: NewBaseRepo[model.ProjectGraphDO](db)}
}

// FindByProjectAndGraphID retrieves a graph by project and graph ID.
func (r *GraphRepo) FindByProjectAndGraphID(ctx context.Context, projectID, graphID string) (*model.ProjectGraphDO, error) {
	var graph model.ProjectGraphDO
	err := r.DB().WithContext(ctx).Where("project_id = ? AND graph_id = ?", projectID, graphID).First(&graph).Error
	if err != nil {
		return nil, err
	}
	return &graph, nil
}

// FindByProjectID retrieves all graphs for a project.
func (r *GraphRepo) FindByProjectID(ctx context.Context, projectID string) ([]model.ProjectGraphDO, error) {
	var graphs []model.ProjectGraphDO
	err := r.DB().WithContext(ctx).Where("project_id = ?", projectID).Find(&graphs).Error
	return graphs, err
}

// GraphNodeRepo is the GORM implementation of GraphNodeRepository.
type GraphNodeRepo struct {
	*BaseRepo[model.ProjectGraphNodeDO]
}

// NewGraphNodeRepo creates a new GraphNodeRepo.
func NewGraphNodeRepo(db *gorm.DB) *GraphNodeRepo {
	return &GraphNodeRepo{BaseRepo: NewBaseRepo[model.ProjectGraphNodeDO](db)}
}

// FindByGraphID retrieves all nodes for a graph.
func (r *GraphNodeRepo) FindByGraphID(ctx context.Context, projectID, graphID string) ([]model.ProjectGraphNodeDO, error) {
	var nodes []model.ProjectGraphNodeDO
	err := r.DB().WithContext(ctx).Where("project_id = ? AND graph_id = ?", projectID, graphID).Find(&nodes).Error
	return nodes, err
}

// FindByGraphNodeID retrieves a specific graph node.
func (r *GraphNodeRepo) FindByGraphNodeID(ctx context.Context, projectID, graphID, graphNodeID string) (*model.ProjectGraphNodeDO, error) {
	var node model.ProjectGraphNodeDO
	err := r.DB().WithContext(ctx).
		Where("project_id = ? AND graph_id = ? AND graph_node_id = ?", projectID, graphID, graphNodeID).
		First(&node).Error
	if err != nil {
		return nil, err
	}
	return &node, nil
}

// DeleteByGraphID deletes all nodes for a graph.
func (r *GraphNodeRepo) DeleteByGraphID(ctx context.Context, projectID, graphID string) error {
	return r.DB().WithContext(ctx).
		Where("project_id = ? AND graph_id = ?", projectID, graphID).
		Delete(&model.ProjectGraphNodeDO{}).Error
}
