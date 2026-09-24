package kuscia

import (
	"context"
	"fmt"
)

// RestartJobRequest represents a Kuscia RestartJob API request.
type RestartJobRequest struct {
	JobID string `json:"job_id"`
}

// RestartJobResponse represents a Kuscia RestartJob API response.
type RestartJobResponse struct {
	Status Status `json:"status"`
	Data   struct {
		JobID string `json:"job_id"`
	} `json:"data"`
}

// RestartJob restarts a failed job in Kuscia (only failed tasks are re-run).
func (c *Client) RestartJob(ctx context.Context, jobID string) error {
	var resp RestartJobResponse
	if err := c.doRequest(ctx, "/api/v1/job/restart", &RestartJobRequest{JobID: jobID}, &resp); err != nil {
		return err
	}
	if !resp.Status.IsSuccess() {
		return fmt.Errorf("kuscia: restart job %s failed: [%d] %s", jobID, resp.Status.Code, resp.Status.Message)
	}
	return nil
}
