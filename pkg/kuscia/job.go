package kuscia

import (
	"context"
	"fmt"
)

// --- Job Types ---

// Party represents a participating party in a task.
type Party struct {
	DomainID  string            `json:"domain_id"`
	Role      string            `json:"role"`
	Resources map[string]string `json:"resources,omitempty"`
}

// TaskConfig represents a task within a job.
type TaskConfig struct {
	AppImage     string   `json:"app_image"`
	Parties      []Party  `json:"parties"`
	Alias        string   `json:"alias"`
	TaskID       string   `json:"task_id,omitempty"`
	Dependencies []string `json:"dependencies,omitempty"`
	InputConfig  string   `json:"task_input_config,omitempty"`
	Priority     int32    `json:"priority,omitempty"`
}

// CreateJobRequest represents a Kuscia CreateJob API request.
type CreateJobRequest struct {
	JobID          string            `json:"job_id"`
	Initiator      string            `json:"initiator"`
	MaxParallelism int32             `json:"max_parallelism,omitempty"`
	Tasks          []TaskConfig      `json:"tasks"`
	CustomFields   map[string]string `json:"custom_fields,omitempty"`
}

// CreateJobResponse represents a Kuscia CreateJob API response.
type CreateJobResponse struct {
	Status Status `json:"status"`
	Data   struct {
		JobID string `json:"job_id"`
	} `json:"data"`
}

// QueryJobRequest represents a Kuscia QueryJob API request.
type QueryJobRequest struct {
	JobID string `json:"job_id"`
}

// TaskStatus represents the status of a task within a job.
type TaskStatus struct {
	TaskID string `json:"task_id"`
	Alias  string `json:"alias"`
	State  string `json:"state"`
	ErrMsg string `json:"err_msg,omitempty"`
}

// JobExecutionStatus represents the execution status block in a job query response.
type JobExecutionStatus struct {
	State  string       `json:"state"`
	ErrMsg string       `json:"err_msg,omitempty"`
	Tasks  []TaskStatus `json:"tasks,omitempty"`
}

// QueryJobResponse represents a Kuscia QueryJob API response.
type QueryJobResponse struct {
	Status Status `json:"status"`
	Data   struct {
		JobID     string              `json:"job_id"`
		Initiator string              `json:"initiator"`
		Status    *JobExecutionStatus `json:"status,omitempty"`
	} `json:"data"`
}

// StopJobRequest represents a Kuscia StopJob API request.
type StopJobRequest struct {
	JobID string `json:"job_id"`
}

// StopJobResponse represents a Kuscia StopJob API response.
type StopJobResponse struct {
	Status Status `json:"status"`
}

// DeleteJobRequest represents a Kuscia DeleteJob API request.
type DeleteJobRequest struct {
	JobID string `json:"job_id"`
}

// DeleteJobResponse represents a Kuscia DeleteJob API response.
type DeleteJobResponse struct {
	Status Status `json:"status"`
}

// BatchQueryJobStatusRequest represents a batch job status query.
type BatchQueryJobStatusRequest struct {
	JobIDs []string `json:"job_ids"`
}

// JobStatusEntry represents a single job status in batch response.
type JobStatusEntry struct {
	JobID  string `json:"job_id"`
	Status struct {
		State  string `json:"state"`
		ErrMsg string `json:"err_msg,omitempty"`
	} `json:"status"`
}

// BatchQueryJobStatusResponse represents a batch job status response.
type BatchQueryJobStatusResponse struct {
	Status Status `json:"status"`
	Data   struct {
		Jobs []JobStatusEntry `json:"jobs"`
	} `json:"data"`
}

// --- Job Service Methods ---

// CreateJob creates a new job in Kuscia.
func (c *Client) CreateJob(ctx context.Context, req *CreateJobRequest) (*CreateJobResponse, error) {
	var resp CreateJobResponse
	if err := c.doRequest(ctx, "/api/v1/job/create", req, &resp); err != nil {
		return nil, err
	}
	if !resp.Status.IsSuccess() {
		return nil, fmt.Errorf("kuscia: create job %s failed: [%d] %s", req.JobID, resp.Status.Code, resp.Status.Message)
	}
	return &resp, nil
}

// QueryJob queries a job's status and details.
func (c *Client) QueryJob(ctx context.Context, jobID string) (*QueryJobResponse, error) {
	var resp QueryJobResponse
	if err := c.doRequest(ctx, "/api/v1/job/query", &QueryJobRequest{JobID: jobID}, &resp); err != nil {
		return nil, err
	}
	if !resp.Status.IsSuccess() {
		return nil, fmt.Errorf("kuscia: query job %s failed: [%d] %s", jobID, resp.Status.Code, resp.Status.Message)
	}
	return &resp, nil
}

// StopJob stops a running job.
func (c *Client) StopJob(ctx context.Context, jobID string) error {
	var resp StopJobResponse
	if err := c.doRequest(ctx, "/api/v1/job/stop", &StopJobRequest{JobID: jobID}, &resp); err != nil {
		return err
	}
	if !resp.Status.IsSuccess() {
		return fmt.Errorf("kuscia: stop job %s failed: [%d] %s", jobID, resp.Status.Code, resp.Status.Message)
	}
	return nil
}

// DeleteJob deletes a job.
func (c *Client) DeleteJob(ctx context.Context, jobID string) error {
	var resp DeleteJobResponse
	if err := c.doRequest(ctx, "/api/v1/job/delete", &DeleteJobRequest{JobID: jobID}, &resp); err != nil {
		return err
	}
	if !resp.Status.IsSuccess() {
		return fmt.Errorf("kuscia: delete job %s failed: [%d] %s", jobID, resp.Status.Code, resp.Status.Message)
	}
	return nil
}

// BatchQueryJobStatus queries status for multiple jobs.
func (c *Client) BatchQueryJobStatus(ctx context.Context, jobIDs []string) ([]JobStatusEntry, error) {
	var resp BatchQueryJobStatusResponse
	if err := c.doRequest(ctx, "/api/v1/job/status/batchQuery", &BatchQueryJobStatusRequest{JobIDs: jobIDs}, &resp); err != nil {
		return nil, err
	}
	if !resp.Status.IsSuccess() {
		return nil, fmt.Errorf("kuscia: batch query job status failed: [%d] %s", resp.Status.Code, resp.Status.Message)
	}
	return resp.Data.Jobs, nil
}
