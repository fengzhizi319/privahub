package kuscia

import (
	"context"
	"fmt"
)

// --- DomainDataGrant Types ---
// Mirrors kuscia/proto/api/v1alpha1/kusciaapi/domaindatagrant.proto.
// HTTP routes: /api/v1/domaindatagrant/{create,update,delete,query,batchQuery,list}.

// GrantLimit restricts a DomainDataGrant. int64 fields are encoded as JSON
// strings, matching protojson.
type GrantLimit struct {
	ExpirationTime int64    `json:"expiration_time,string,omitempty"`
	UseCount       int32    `json:"use_count,omitempty"`
	FlowID         string   `json:"flow_id,omitempty"`
	Components     []string `json:"components,omitempty"`
	Initiator      string   `json:"initiator,omitempty"`
	InputConfig    string   `json:"input_config,omitempty"`
}

// GrantDomainDataRequest represents a Kuscia CreateDomainDataGrant request.
// DomainDataGrantID is optional; Kuscia generates one when empty.
type GrantDomainDataRequest struct {
	DomainDataGrantID string            `json:"domaindatagrant_id,omitempty"`
	DomainID          string            `json:"domain_id"`
	DomainDataID      string            `json:"domaindata_id"`
	GrantDomain       string            `json:"grant_domain"`
	Limit             *GrantLimit       `json:"limit,omitempty"`
	Description       map[string]string `json:"description,omitempty"`
	Signature         string            `json:"signature,omitempty"`
}

// CreateDomainDataGrantRequest is the canonical name of GrantDomainDataRequest.
type CreateDomainDataGrantRequest = GrantDomainDataRequest

// GrantDomainDataResponse represents a Kuscia CreateDomainDataGrant response.
type GrantDomainDataResponse struct {
	Status Status `json:"status"`
	Data   struct {
		DomainDataGrantID string `json:"domaindatagrant_id"`
	} `json:"data"`
}

// UseRecord is one usage record of a grant.
type UseRecord struct {
	UseTime     int64  `json:"use_time,string,omitempty"`
	GrantDomain string `json:"grant_domain,omitempty"`
	Component   string `json:"component,omitempty"`
	Output      string `json:"output,omitempty"`
}

// DomainDataGrantData is the grant spec.
type DomainDataGrantData struct {
	DomainDataGrantID string            `json:"domaindatagrant_id"`
	Author            string            `json:"author,omitempty"`
	DomainDataID      string            `json:"domaindata_id"`
	GrantDomain       string            `json:"grant_domain"`
	Limit             *GrantLimit       `json:"limit,omitempty"`
	Description       map[string]string `json:"description,omitempty"`
	Signature         string            `json:"signature,omitempty"`
	DomainID          string            `json:"domain_id"`
}

// DomainDataGrantStatus is the grant status.
type DomainDataGrantStatus struct {
	Phase   string      `json:"phase,omitempty"`
	Message string      `json:"message,omitempty"`
	Records []UseRecord `json:"records,omitempty"`
}

// DomainDataGrant is the Kuscia DomainDataGrant message.
type DomainDataGrant struct {
	Data   DomainDataGrantData   `json:"data"`
	Status DomainDataGrantStatus `json:"status"`
}

// QueryDomainDataGrantRequest is a flat query request.
type QueryDomainDataGrantRequest struct {
	DomainID          string `json:"domain_id"`
	DomainDataGrantID string `json:"domaindatagrant_id"`
}

// QueryDomainDataGrantRequestData is one item of a batch query.
type QueryDomainDataGrantRequestData = QueryDomainDataGrantRequest

type queryDomainDataGrantResponse struct {
	Status Status          `json:"status"`
	Data   DomainDataGrant `json:"data"`
}

type batchQueryDomainDataGrantRequest struct {
	Data []QueryDomainDataGrantRequestData `json:"data"`
}

type batchQueryDomainDataGrantResponse struct {
	Status Status            `json:"status"`
	Data   []DomainDataGrant `json:"data"`
}

// --- DomainDataGrant Service Methods ---

// GrantDomainData grants access to domain data for another domain.
func (c *Client) GrantDomainData(ctx context.Context, req *GrantDomainDataRequest) error {
	_, err := c.CreateDomainDataGrant(ctx, req)
	return err
}

// CreateDomainDataGrant creates a grant and returns the grant id.
func (c *Client) CreateDomainDataGrant(ctx context.Context, req *CreateDomainDataGrantRequest) (string, error) {
	var resp GrantDomainDataResponse
	if err := c.doRequest(ctx, "/api/v1/domaindatagrant/create", req, &resp); err != nil {
		return "", err
	}
	if !resp.Status.IsSuccess() {
		return "", fmt.Errorf("kuscia: grant domain data %s to %s failed: [%d] %s", req.DomainDataID, req.GrantDomain, resp.Status.Code, resp.Status.Message)
	}
	id := resp.Data.DomainDataGrantID
	if id == "" {
		id = req.DomainDataGrantID
	}
	return id, nil
}

// QueryDomainDataGrant queries a grant.
func (c *Client) QueryDomainDataGrant(ctx context.Context, domainID, grantID string) (*DomainDataGrant, error) {
	var resp queryDomainDataGrantResponse
	req := &QueryDomainDataGrantRequest{DomainID: domainID, DomainDataGrantID: grantID}
	if err := c.doRequest(ctx, "/api/v1/domaindatagrant/query", req, &resp); err != nil {
		return nil, err
	}
	if !resp.Status.IsSuccess() {
		return nil, fmt.Errorf("kuscia: query domain data grant %s/%s failed: [%d] %s", domainID, grantID, resp.Status.Code, resp.Status.Message)
	}
	return &resp.Data, nil
}

// BatchQueryDomainDataGrant queries several grants.
func (c *Client) BatchQueryDomainDataGrant(ctx context.Context, items []QueryDomainDataGrantRequestData) ([]DomainDataGrant, error) {
	var resp batchQueryDomainDataGrantResponse
	if err := c.doRequest(ctx, "/api/v1/domaindatagrant/batchQuery", &batchQueryDomainDataGrantRequest{Data: items}, &resp); err != nil {
		return nil, err
	}
	if !resp.Status.IsSuccess() {
		return nil, fmt.Errorf("kuscia: batch query domain data grant failed: [%d] %s", resp.Status.Code, resp.Status.Message)
	}
	return resp.Data, nil
}

// DeleteDomainDataGrant deletes a grant.
func (c *Client) DeleteDomainDataGrant(ctx context.Context, domainID, grantID string) error {
	var resp statusOnly
	req := &QueryDomainDataGrantRequest{DomainID: domainID, DomainDataGrantID: grantID}
	if err := c.doRequest(ctx, "/api/v1/domaindatagrant/delete", req, &resp); err != nil {
		return err
	}
	if !resp.Status.IsSuccess() {
		return fmt.Errorf("kuscia: delete domain data grant %s/%s failed: [%d] %s", domainID, grantID, resp.Status.Code, resp.Status.Message)
	}
	return nil
}
