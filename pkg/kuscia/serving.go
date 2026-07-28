package kuscia

import "context"

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

// CreateServing creates a serving in Kuscia.
func (c *Client) CreateServing(ctx context.Context, req *CreateServingRequest) error {
	return c.doRequest(ctx, "/api/v1/serving/create", req, nil)
}

// QueryServing queries a serving from Kuscia.
func (c *Client) QueryServing(ctx context.Context, servingID string) (*QueryServingResponse, error) {
	req := &QueryServingRequest{ServingID: servingID}
	var resp QueryServingResponse
	if err := c.doRequest(ctx, "/api/v1/serving/query", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// UpdateServing updates a serving in Kuscia.
func (c *Client) UpdateServing(ctx context.Context, req *UpdateServingRequest) error {
	return c.doRequest(ctx, "/api/v1/serving/update", req, nil)
}

// DeleteServing deletes a serving from Kuscia.
func (c *Client) DeleteServing(ctx context.Context, servingID string) error {
	req := &DeleteServingRequest{ServingID: servingID}
	return c.doRequest(ctx, "/api/v1/serving/delete", req, nil)
}

// BatchQueryServingStatus queries multiple serving statuses from Kuscia.
func (c *Client) BatchQueryServingStatus(ctx context.Context, servingIDs []string) ([]ServingStatusEntry, error) {
	req := &BatchQueryServingStatusRequest{ServingIDs: servingIDs}
	var resp BatchQueryServingStatusResponse
	if err := c.doRequest(ctx, "/api/v1/serving/batchQueryStatus", req, &resp); err != nil {
		return nil, err
	}
	if resp.Data == nil {
		return nil, nil
	}
	return resp.Data.Servings, nil
}
