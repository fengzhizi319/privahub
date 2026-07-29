package kuscia

import (
	"context"
	"fmt"
)

// --- DomainRoute Types ---

// CreateDomainRouteRequest represents a Kuscia CreateDomainRoute API request.
type CreateDomainRouteRequest struct {
	Source                string `json:"source"`
	Destination           string `json:"destination"`
	SourceNetAddress      string `json:"source_net_address,omitempty"`
	DestinationNetAddress string `json:"destination_net_address,omitempty"`
	AuthenticationType    string `json:"authentication_type,omitempty"` // Token / RSA / MTLS
}

// CreateDomainRouteResponse represents a Kuscia CreateDomainRoute API response.
type CreateDomainRouteResponse struct {
	Status Status `json:"status"`
}

// QueryDomainRouteRequest represents a Kuscia QueryDomainRoute API request.
type QueryDomainRouteRequest struct {
	Source      string `json:"source"`
	Destination string `json:"destination"`
}

// QueryDomainRouteResponse represents a Kuscia QueryDomainRoute API response.
type QueryDomainRouteResponse struct {
	Status Status `json:"status"`
	Data   struct {
		Source      string `json:"source"`
		Destination string `json:"destination"`
		Status      string `json:"status"` // Ready / NotReady / Pending
	} `json:"data"`
}

// DeleteDomainRouteRequest represents a Kuscia DeleteDomainRoute API request.
type DeleteDomainRouteRequest struct {
	Source      string `json:"source"`
	Destination string `json:"destination"`
}

// DeleteDomainRouteResponse represents a Kuscia DeleteDomainRoute API response.
type DeleteDomainRouteResponse struct {
	Status Status `json:"status"`
}

// BatchQueryDomainRouteStatusRequest represents a batch route status query.
type BatchQueryDomainRouteStatusRequest struct {
	Routes []QueryDomainRouteRequest `json:"routes"`
}

// DomainRouteStatusEntry represents a single route status in batch response.
type DomainRouteStatusEntry struct {
	Source      string `json:"source"`
	Destination string `json:"destination"`
	Status      string `json:"status"`
}

// BatchQueryDomainRouteStatusResponse represents a batch route status response.
type BatchQueryDomainRouteStatusResponse struct {
	Status Status `json:"status"`
	Data   struct {
		Routes []DomainRouteStatusEntry `json:"routes"`
	} `json:"data"`
}

// --- DomainRoute Service Methods ---

// CreateDomainRoute creates a route between two domains.
func (c *Client) CreateDomainRoute(ctx context.Context, req *CreateDomainRouteRequest) error {
	var resp CreateDomainRouteResponse
	if err := c.doRequest(ctx, "/api/v1/route/create", req, &resp); err != nil {
		return err
	}
	if !resp.Status.IsSuccess() {
		return fmt.Errorf("kuscia: create domain route %s->%s failed: [%d] %s",
			req.Source, req.Destination, resp.Status.Code, resp.Status.Message)
	}
	return nil
}

// QueryDomainRoute queries a route's status.
func (c *Client) QueryDomainRoute(ctx context.Context, source, destination string) (*QueryDomainRouteResponse, error) {
	var resp QueryDomainRouteResponse
	req := &QueryDomainRouteRequest{Source: source, Destination: destination}
	if err := c.doRequest(ctx, "/api/v1/route/query", req, &resp); err != nil {
		return nil, err
	}
	if !resp.Status.IsSuccess() {
		return nil, fmt.Errorf("kuscia: query domain route %s->%s failed: [%d] %s",
			source, destination, resp.Status.Code, resp.Status.Message)
	}
	return &resp, nil
}

// DeleteDomainRoute removes a route between two domains.
func (c *Client) DeleteDomainRoute(ctx context.Context, source, destination string) error {
	var resp DeleteDomainRouteResponse
	req := &DeleteDomainRouteRequest{Source: source, Destination: destination}
	if err := c.doRequest(ctx, "/api/v1/route/delete", req, &resp); err != nil {
		return err
	}
	if !resp.Status.IsSuccess() {
		return fmt.Errorf("kuscia: delete domain route %s->%s failed: [%d] %s",
			source, destination, resp.Status.Code, resp.Status.Message)
	}
	return nil
}

// BatchQueryDomainRouteStatus queries status for multiple routes.
func (c *Client) BatchQueryDomainRouteStatus(ctx context.Context, routes []QueryDomainRouteRequest) ([]DomainRouteStatusEntry, error) {
	var resp BatchQueryDomainRouteStatusResponse
	req := &BatchQueryDomainRouteStatusRequest{Routes: routes}
	if err := c.doRequest(ctx, "/api/v1/route/status/batchQuery", req, &resp); err != nil {
		return nil, err
	}
	if !resp.Status.IsSuccess() {
		return nil, fmt.Errorf("kuscia: batch query domain route status failed: [%d] %s",
			resp.Status.Code, resp.Status.Message)
	}
	return resp.Data.Routes, nil
}
