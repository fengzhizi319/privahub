package kuscia

import (
	"context"
	"fmt"
)

// --- Serving Types ---

// ServingParty represents a party in a serving deployment.
type ServingParty struct {
	DomainID          string `json:"domain_id"`
	Role              string `json:"role"`
	AppImage          string `json:"app_image"`
	Replicas          int32  `json:"replicas,omitempty"`
	ServiceNamePrefix string `json:"service_name_prefix,omitempty"`
}

// CreateServingRequest represents a Kuscia serving creation request.
type CreateServingRequest struct {
	ServingID          string         `json:"serving_id"`
	ServingInputConfig string         `json:"serving_input_config"`
	Initiator          string         `json:"initiator"`
	Parties            []ServingParty `json:"parties"`
	AffinityMode       string         `json:"affinity_mode,omitempty"`
}

// QueryServingRequest represents a Kuscia serving query request.
type QueryServingRequest struct {
	ServingID string `json:"serving_id"`
}

// PartyServingStatus represents a single party's serving status.
type PartyServingStatus struct {
	DomainID            string `json:"domain_id"`
	Role                string `json:"role"`
	State               string `json:"state"`
	Replicas            int32  `json:"replicas"`
	AvailableReplicas   int32  `json:"available_replicas"`
	UnavailableReplicas int32  `json:"unavailable_replicas"`
	CreateTime          string `json:"create_time"`
}

// ServingStatusDetail represents detailed serving status.
type ServingStatusDetail struct {
	State            string               `json:"state"`
	Reason           string               `json:"reason"`
	Message          string               `json:"message"`
	TotalParties     int32                `json:"total_parties"`
	AvailableParties int32                `json:"available_parties"`
	CreateTime       string               `json:"create_time"`
	PartyStatuses    []PartyServingStatus `json:"party_statuses"`
}

// QueryServingResponseData represents the data in a serving query response.
type QueryServingResponseData struct {
	ServingInputConfig string              `json:"serving_input_config"`
	Initiator          string              `json:"initiator"`
	Parties            []ServingParty      `json:"parties"`
	Status             ServingStatusDetail `json:"status"`
}

// QueryServingResponse represents a Kuscia serving query response.
type QueryServingResponse struct {
	Status Status                    `json:"status"`
	Data   *QueryServingResponseData `json:"data"`
}

// UpdateServingRequest represents a Kuscia serving update request.
type UpdateServingRequest struct {
	ServingID          string         `json:"serving_id"`
	ServingInputConfig string         `json:"serving_input_config"`
	Parties            []ServingParty `json:"parties"`
}

// DeleteServingRequest represents a Kuscia serving deletion request.
type DeleteServingRequest struct {
	ServingID string `json:"serving_id"`
}

// BatchQueryServingStatusRequest represents a batch serving status query.
type BatchQueryServingStatusRequest struct {
	ServingIDs []string `json:"serving_ids"`
}

// ServingStatusEntry represents a single serving status in batch query.
type ServingStatusEntry struct {
	ServingID string              `json:"serving_id"`
	Status    ServingStatusDetail `json:"status"`
}

// BatchQueryServingStatusResponseData represents batch query response data.
type BatchQueryServingStatusResponseData struct {
	Servings []ServingStatusEntry `json:"servings"`
}

// BatchQueryServingStatusResponse represents a batch serving status response.
type BatchQueryServingStatusResponse struct {
	Status Status                               `json:"status"`
	Data   *BatchQueryServingStatusResponseData `json:"data"`
}

// --- Serving Client Methods ---

// CreateServingResponse represents a Kuscia serving creation response.
type CreateServingResponse struct {
	Status Status `json:"status"`
}

// UpdateServingResponse represents a Kuscia serving update response.
type UpdateServingResponse struct {
	Status Status `json:"status"`
}

// DeleteServingResponse represents a Kuscia serving deletion response.
type DeleteServingResponse struct {
	Status Status `json:"status"`
}

// CreateServing creates a serving in Kuscia.
func (c *Client) CreateServing(ctx context.Context, req *CreateServingRequest) error {
	// Bug70 fix: parse response and check status code instead of passing nil
	// (which silently ignored API-level errors).
	var resp CreateServingResponse
	if err := c.doRequest(ctx, "/api/v1/serving/create", req, &resp); err != nil {
		return err
	}
	if !resp.Status.IsSuccess() {
		return fmt.Errorf("kuscia: create serving %s failed: [%d] %s", req.ServingID, resp.Status.Code, resp.Status.Message)
	}
	return nil
}

// QueryServing queries a serving from Kuscia.
func (c *Client) QueryServing(ctx context.Context, servingID string) (*QueryServingResponse, error) {
	req := &QueryServingRequest{ServingID: servingID}
	var resp QueryServingResponse
	if err := c.doRequest(ctx, "/api/v1/serving/query", req, &resp); err != nil {
		return nil, err
	}
	// Bug71 fix: check response status for consistency with other query methods.
	if !resp.Status.IsSuccess() {
		return nil, fmt.Errorf("kuscia: query serving %s failed: [%d] %s", servingID, resp.Status.Code, resp.Status.Message)
	}
	return &resp, nil
}

// UpdateServing updates a serving in Kuscia.
func (c *Client) UpdateServing(ctx context.Context, req *UpdateServingRequest) error {
	// Bug70 fix: parse response and check status code.
	var resp UpdateServingResponse
	if err := c.doRequest(ctx, "/api/v1/serving/update", req, &resp); err != nil {
		return err
	}
	if !resp.Status.IsSuccess() {
		return fmt.Errorf("kuscia: update serving %s failed: [%d] %s", req.ServingID, resp.Status.Code, resp.Status.Message)
	}
	return nil
}

// DeleteServing deletes a serving from Kuscia.
func (c *Client) DeleteServing(ctx context.Context, servingID string) error {
	req := &DeleteServingRequest{ServingID: servingID}
	// Bug70 fix: parse response and check status code.
	var resp DeleteServingResponse
	if err := c.doRequest(ctx, "/api/v1/serving/delete", req, &resp); err != nil {
		return err
	}
	if !resp.Status.IsSuccess() {
		return fmt.Errorf("kuscia: delete serving %s failed: [%d] %s", servingID, resp.Status.Code, resp.Status.Message)
	}
	return nil
}

// BatchQueryServingStatus queries multiple serving statuses from Kuscia.
func (c *Client) BatchQueryServingStatus(ctx context.Context, servingIDs []string) ([]ServingStatusEntry, error) {
	req := &BatchQueryServingStatusRequest{ServingIDs: servingIDs}
	var resp BatchQueryServingStatusResponse
	if err := c.doRequest(ctx, "/api/v1/serving/status/batchQuery", req, &resp); err != nil {
		return nil, err
	}
	// Bug72 fix: check response status for consistency with all other batch query methods.
	if !resp.Status.IsSuccess() {
		return nil, fmt.Errorf("kuscia: batch query serving status failed: [%d] %s", resp.Status.Code, resp.Status.Message)
	}
	if resp.Data == nil {
		return nil, nil
	}
	return resp.Data.Servings, nil
}
