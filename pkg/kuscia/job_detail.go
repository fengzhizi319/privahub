package kuscia

import (
	"context"
	"fmt"
)

// This file holds the Kuscia calls needed by the graph execution chain / job
// status sync (graph migration work): full job status detail (task progress,
// party states) and DomainDataGrant with an explicit grant id.

// JobPartyStatusDetail is kusciaapi PartyStatus.
type JobPartyStatusDetail struct {
	DomainID string `json:"domain_id"`
	State    string `json:"state"`
	ErrMsg   string `json:"err_msg,omitempty"`
}

// JobTaskStatusDetail is kusciaapi TaskStatus.
type JobTaskStatusDetail struct {
	TaskID     string                 `json:"task_id"`
	Alias      string                 `json:"alias"`
	State      string                 `json:"state"`
	ErrMsg     string                 `json:"err_msg,omitempty"`
	CreateTime string                 `json:"create_time,omitempty"`
	StartTime  string                 `json:"start_time,omitempty"`
	EndTime    string                 `json:"end_time,omitempty"`
	Parties    []JobPartyStatusDetail `json:"parties,omitempty"`
	Progress   float32                `json:"progress,omitempty"`
}

// JobStatusDetailInfo is kusciaapi JobStatusDetail.
type JobStatusDetailInfo struct {
	State      string                `json:"state"`
	ErrMsg     string                `json:"err_msg,omitempty"`
	CreateTime string                `json:"create_time,omitempty"`
	StartTime  string                `json:"start_time,omitempty"`
	EndTime    string                `json:"end_time,omitempty"`
	Tasks      []JobTaskStatusDetail `json:"tasks,omitempty"`
}

// QueryJobDetailResponse is kusciaapi QueryJobResponse with the full status.
type QueryJobDetailResponse struct {
	Status Status `json:"status"`
	Data   struct {
		JobID     string               `json:"job_id"`
		Initiator string               `json:"initiator"`
		Status    *JobStatusDetailInfo `json:"status,omitempty"`
	} `json:"data"`
}

// QueryJobDetail queries a job with its full status detail.
func (c *Client) QueryJobDetail(ctx context.Context, jobID string) (*QueryJobDetailResponse, error) {
	var resp QueryJobDetailResponse
	if err := c.doRequest(ctx, "/api/v1/job/query", map[string]string{"job_id": jobID}, &resp); err != nil {
		return nil, err
	}
	if !resp.Status.IsSuccess() {
		return nil, fmt.Errorf("kuscia: query job %s failed: [%d] %s", jobID, resp.Status.Code, resp.Status.Message)
	}
	return &resp, nil
}

// DomainDataGrantWithIDRequest is kusciaapi CreateDomainDataGrantRequest.
type DomainDataGrantWithIDRequest struct {
	DomainID          string            `json:"domain_id"`
	DomainDataGrantID string            `json:"domaindatagrant_id,omitempty"`
	DomainDataID      string            `json:"domaindata_id"`
	GrantDomain       string            `json:"grant_domain"`
	Description       map[string]string `json:"description,omitempty"`
}

// CreateDomainDataGrantWithID creates a DomainDataGrant with an explicit id
// (Java DatatableGrantManager.createDomainGrant, grantId "{domainDataId}-{grantNode}").
func (c *Client) CreateDomainDataGrantWithID(ctx context.Context, req *DomainDataGrantWithIDRequest) error {
	var resp struct {
		Status Status `json:"status"`
	}
	if err := c.doRequest(ctx, "/api/v1/domaindatagrant/create", req, &resp); err != nil {
		return err
	}
	if !resp.Status.IsSuccess() {
		return fmt.Errorf("kuscia: create domain data grant %s failed: [%d] %s", req.DomainDataGrantID, resp.Status.Code, resp.Status.Message)
	}
	return nil
}

// QueryDomainDataGrantByID queries a DomainDataGrant; it returns an error when
// the grant does not exist (Java DatatableGrantManager.queryDomainGrant).
func (c *Client) QueryDomainDataGrantByID(ctx context.Context, domainID, grantID string) error {
	var resp struct {
		Status Status `json:"status"`
	}
	req := map[string]string{"domain_id": domainID, "domaindatagrant_id": grantID}
	if err := c.doRequest(ctx, "/api/v1/domaindatagrant/query", req, &resp); err != nil {
		return err
	}
	if !resp.Status.IsSuccess() {
		return fmt.Errorf("kuscia: query domain data grant %s failed: [%d] %s", grantID, resp.Status.Code, resp.Status.Message)
	}
	return nil
}
