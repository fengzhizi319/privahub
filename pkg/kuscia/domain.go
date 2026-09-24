package kuscia

import (
	"context"
	"fmt"
)

// --- Domain Types (kusciaapi/domain.proto) ---

// AuthCenter configures how a lite domain authenticates to master.
// Java NodeManager uses {authentication_type: "Token", token_gen_method: "UID-RSA-GEN"}
// for center-mode nodes and "RSA-GEN" for P2P partner domains.
type AuthCenter struct {
	AuthenticationType string `json:"authentication_type,omitempty"`
	TokenGenMethod     string `json:"token_gen_method,omitempty"`
}

// Domain token / auth constants used by Java NodeManager.
const (
	AuthTypeToken        = "Token"
	TokenGenUIDRSAGen    = "UID-RSA-GEN"
	TokenGenRSAGen       = "RSA-GEN"
	DomainRolePartner    = "partner"
	DeployTokenUnused    = "unused"
	DeployTokenUsed      = "used"
	DomainStatusReady    = "Ready"
	DomainStatusNotReady = "NotReady"
)

// CreateDomainRequest represents a Kuscia CreateDomain API request.
type CreateDomainRequest struct {
	DomainID       string      `json:"domain_id"`
	Role           string      `json:"role,omitempty"` // "" (normal) / partner
	Cert           string      `json:"cert,omitempty"`
	AuthCenter     *AuthCenter `json:"auth_center,omitempty"`
	MasterDomainID string      `json:"master_domain_id,omitempty"`
}

// CreateDomainResponse represents a Kuscia CreateDomain API response.
type CreateDomainResponse struct {
	Status Status `json:"status"`
}

// UpdateDomainRequest represents a Kuscia UpdateDomain API request.
type UpdateDomainRequest = CreateDomainRequest

// QueryDomainRequest represents a Kuscia QueryDomain API request.
type QueryDomainRequest struct {
	DomainID string `json:"domain_id"`
}

// NodeStatus is the status of one Kuscia node (instance) inside a domain.
type NodeStatus struct {
	Name               string `json:"name"`
	Status             string `json:"status"`
	Version            string `json:"version"`
	LastHeartbeatTime  string `json:"last_heartbeat_time"`
	LastTransitionTime string `json:"last_transition_time"`
}

// DeployTokenStatus is a lite-node deploy token (state used/unused).
type DeployTokenStatus struct {
	Token              string `json:"token"`
	State              string `json:"state"`
	LastTransitionTime string `json:"last_transition_time"`
}

// DomainInfo is QueryDomainResponseData.
type DomainInfo struct {
	DomainID            string              `json:"domain_id"`
	Role                string              `json:"role"`
	Cert                string              `json:"cert"`
	NodeStatuses        []NodeStatus        `json:"node_statuses"`
	DeployTokenStatuses []DeployTokenStatus `json:"deploy_token_statuses"`
	Annotations         map[string]string   `json:"annotations"`
	AuthCenter          *AuthCenter         `json:"auth_center,omitempty"`
	MasterDomainID      string              `json:"master_domain_id"`
}

// IsReady reports whether at least one node instance is Ready (Java fillNodeDTOByKResp).
func (d *DomainInfo) IsReady() bool {
	for _, n := range d.NodeStatuses {
		if n.Status == DomainStatusReady {
			return true
		}
	}
	return false
}

// QueryDomainResponse represents a Kuscia QueryDomain API response.
type QueryDomainResponse struct {
	Status Status     `json:"status"`
	Data   DomainInfo `json:"data"`
}

// BatchQueryDomainRequest is the Kuscia BatchQueryDomain request.
type BatchQueryDomainRequest struct {
	DomainIDs []string `json:"domain_ids"`
}

// BatchDomain is one entry of BatchQueryDomainResponseData.
type BatchDomain struct {
	DomainID     string       `json:"domain_id"`
	Role         string       `json:"role"`
	Cert         string       `json:"cert"`
	NodeStatuses []NodeStatus `json:"node_statuses"`
}

// IsReady reports whether at least one node instance is Ready.
func (d *BatchDomain) IsReady() bool {
	for _, n := range d.NodeStatuses {
		if n.Status == DomainStatusReady {
			return true
		}
	}
	return false
}

// BatchQueryDomainResponse is the Kuscia BatchQueryDomain response.
type BatchQueryDomainResponse struct {
	Status Status `json:"status"`
	Data   struct {
		Domains []BatchDomain `json:"domains"`
	} `json:"data"`
}

// DeleteDomainRequest represents a Kuscia DeleteDomain API request.
type DeleteDomainRequest struct {
	DomainID string `json:"domain_id"`
}

// DeleteDomainResponse represents a Kuscia DeleteDomain API response.
type DeleteDomainResponse struct {
	Status Status `json:"status"`
}

// --- Domain Service Methods ---

// CreateDomain registers a new domain in Kuscia.
func (c *Client) CreateDomain(ctx context.Context, req *CreateDomainRequest) error {
	var resp CreateDomainResponse
	if err := c.doRequest(ctx, "/api/v1/domain/create", req, &resp); err != nil {
		return err
	}
	if !resp.Status.IsSuccess() {
		return fmt.Errorf("kuscia: create domain %s failed: [%d] %s", req.DomainID, resp.Status.Code, resp.Status.Message)
	}
	return nil
}

// UpdateDomain updates a domain (role / cert / master_domain_id).
func (c *Client) UpdateDomain(ctx context.Context, req *UpdateDomainRequest) error {
	var resp CreateDomainResponse
	return c.doRequest(ctx, "/api/v1/domain/update", req, &resp)
}

// QueryDomain queries a domain's details (node statuses, deploy tokens, cert...).
func (c *Client) QueryDomain(ctx context.Context, domainID string) (*QueryDomainResponse, error) {
	var resp QueryDomainResponse
	if err := c.doRequest(ctx, "/api/v1/domain/query", &QueryDomainRequest{DomainID: domainID}, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// DomainExists reports whether the domain exists in Kuscia (Java checkNodeExists:
// QueryDomain status.code == 0). Transport errors are returned.
func (c *Client) DomainExists(ctx context.Context, domainID string) (bool, error) {
	_, err := c.QueryDomain(ctx, domainID)
	if err == nil {
		return true, nil
	}
	if ErrorCode(err) != 0 {
		return false, nil
	}
	return false, err
}

// BatchQueryDomain queries several domains at once.
func (c *Client) BatchQueryDomain(ctx context.Context, domainIDs []string) ([]BatchDomain, error) {
	var resp BatchQueryDomainResponse
	if err := c.doRequest(ctx, "/api/v1/domain/batchQuery", &BatchQueryDomainRequest{DomainIDs: domainIDs}, &resp); err != nil {
		return nil, err
	}
	return resp.Data.Domains, nil
}

// DeleteDomain removes a domain from Kuscia.
func (c *Client) DeleteDomain(ctx context.Context, domainID string) error {
	var resp DeleteDomainResponse
	if err := c.doRequest(ctx, "/api/v1/domain/delete", &DeleteDomainRequest{DomainID: domainID}, &resp); err != nil {
		return err
	}
	return nil
}
