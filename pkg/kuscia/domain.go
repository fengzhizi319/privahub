package kuscia

import (
	"context"
	"fmt"
)

// --- Domain Types ---

// CreateDomainRequest represents a Kuscia CreateDomain API request.
type CreateDomainRequest struct {
	DomainID string `json:"domain_id"`
	Role     string `json:"role,omitempty"` // center / lite / autonomy
	Cert     string `json:"cert,omitempty"`
}

// CreateDomainResponse represents a Kuscia CreateDomain API response.
type CreateDomainResponse struct {
	Status Status `json:"status"`
}

// QueryDomainRequest represents a Kuscia QueryDomain API request.
type QueryDomainRequest struct {
	DomainID string `json:"domain_id"`
}

// QueryDomainResponse represents a Kuscia QueryDomain API response.
type QueryDomainResponse struct {
	Status Status `json:"status"`
	Data   struct {
		DomainID string `json:"domain_id"`
		Role     string `json:"role"`
		Cert     string `json:"cert"`
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

// --- DomainData Types ---

// CreateDomainDataRequest represents a Kuscia CreateDomainData API request.
type CreateDomainDataRequest struct {
	DomainID     string            `json:"domain_id"`
	DomainDataID string            `json:"domaindata_id"`
	Name         string            `json:"name"`
	Type         string            `json:"type"` // table / model / rule
	RelativeURI  string            `json:"relative_uri"`
	DatasourceID string            `json:"datasource_id"`
	Attributes   map[string]string `json:"attributes,omitempty"`
	Columns      []DataColumn      `json:"columns,omitempty"`
	Author       string            `json:"author,omitempty"`
}

// DataColumn represents a column in domain data.
type DataColumn struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
}

// CreateDomainDataResponse represents a Kuscia CreateDomainData API response.
type CreateDomainDataResponse struct {
	Status Status `json:"status"`
}

// QueryDomainDataRequest represents a Kuscia QueryDomainData API request.
type QueryDomainDataRequest struct {
	DomainID     string `json:"domain_id"`
	DomainDataID string `json:"domaindata_id"`
}

// QueryDomainDataResponse represents a Kuscia QueryDomainData API response.
type QueryDomainDataResponse struct {
	Status Status `json:"status"`
	Data   struct {
		DomainDataID string            `json:"domaindata_id"`
		Name         string            `json:"name"`
		Type         string            `json:"type"`
		RelativeURI  string            `json:"relative_uri"`
		DatasourceID string            `json:"datasource_id"`
		Attributes   map[string]string `json:"attributes"`
		Columns      []DataColumn      `json:"columns"`
		Author       string            `json:"author"`
	} `json:"data"`
}

// ListDomainDataRequest represents a Kuscia ListDomainData API request.
type ListDomainDataRequest struct {
	DomainID string `json:"domain_id"`
}

// DomainDataItem represents a domain data entry in list response.
type DomainDataItem struct {
	DomainDataID string `json:"domaindata_id"`
	Name         string `json:"name"`
	Type         string `json:"type"`
	RelativeURI  string `json:"relative_uri"`
	DatasourceID string `json:"datasource_id"`
	Author       string `json:"author"`
}

// ListDomainDataResponse represents a Kuscia ListDomainData API response.
type ListDomainDataResponse struct {
	Status Status `json:"status"`
	Data   struct {
		DomainDataList []DomainDataItem `json:"domaindata_list"`
	} `json:"data"`
}

// DeleteDomainDataRequest represents a Kuscia DeleteDomainData API request.
type DeleteDomainDataRequest struct {
	DomainID     string `json:"domain_id"`
	DomainDataID string `json:"domaindata_id"`
}

// DeleteDomainDataResponse represents a Kuscia DeleteDomainData API response.
type DeleteDomainDataResponse struct {
	Status Status `json:"status"`
}

// --- Domain Service Methods ---

// CreateDomain registers a new domain in Kuscia.
func (c *Client) CreateDomain(ctx context.Context, req *CreateDomainRequest) error {
	var resp CreateDomainResponse
	if err := c.doRequest(ctx, "/api/v1alpha1/domain/create", req, &resp); err != nil {
		return err
	}
	if !resp.Status.IsSuccess() {
		return fmt.Errorf("kuscia: create domain %s failed: [%d] %s", req.DomainID, resp.Status.Code, resp.Status.Message)
	}
	return nil
}

// QueryDomain queries a domain's details.
func (c *Client) QueryDomain(ctx context.Context, domainID string) (*QueryDomainResponse, error) {
	var resp QueryDomainResponse
	if err := c.doRequest(ctx, "/api/v1alpha1/domain/query", &QueryDomainRequest{DomainID: domainID}, &resp); err != nil {
		return nil, err
	}
	if !resp.Status.IsSuccess() {
		return nil, fmt.Errorf("kuscia: query domain %s failed: [%d] %s", domainID, resp.Status.Code, resp.Status.Message)
	}
	return &resp, nil
}

// DeleteDomain removes a domain from Kuscia.
func (c *Client) DeleteDomain(ctx context.Context, domainID string) error {
	var resp DeleteDomainResponse
	if err := c.doRequest(ctx, "/api/v1alpha1/domain/delete", &DeleteDomainRequest{DomainID: domainID}, &resp); err != nil {
		return err
	}
	if !resp.Status.IsSuccess() {
		return fmt.Errorf("kuscia: delete domain %s failed: [%d] %s", domainID, resp.Status.Code, resp.Status.Message)
	}
	return nil
}

// --- DomainData Service Methods ---

// CreateDomainData registers domain data (datatable) in Kuscia.
func (c *Client) CreateDomainData(ctx context.Context, req *CreateDomainDataRequest) error {
	var resp CreateDomainDataResponse
	if err := c.doRequest(ctx, "/api/v1alpha1/domaindata/create", req, &resp); err != nil {
		return err
	}
	if !resp.Status.IsSuccess() {
		return fmt.Errorf("kuscia: create domain data %s failed: [%d] %s", req.DomainDataID, resp.Status.Code, resp.Status.Message)
	}
	return nil
}

// QueryDomainData queries domain data details.
func (c *Client) QueryDomainData(ctx context.Context, domainID, domainDataID string) (*QueryDomainDataResponse, error) {
	var resp QueryDomainDataResponse
	req := &QueryDomainDataRequest{DomainID: domainID, DomainDataID: domainDataID}
	if err := c.doRequest(ctx, "/api/v1alpha1/domaindata/query", req, &resp); err != nil {
		return nil, err
	}
	if !resp.Status.IsSuccess() {
		return nil, fmt.Errorf("kuscia: query domain data %s failed: [%d] %s", domainDataID, resp.Status.Code, resp.Status.Message)
	}
	return &resp, nil
}

// ListDomainData lists all domain data for a domain.
func (c *Client) ListDomainData(ctx context.Context, domainID string) ([]DomainDataItem, error) {
	var resp ListDomainDataResponse
	if err := c.doRequest(ctx, "/api/v1alpha1/domaindata/list", &ListDomainDataRequest{DomainID: domainID}, &resp); err != nil {
		return nil, err
	}
	if !resp.Status.IsSuccess() {
		return nil, fmt.Errorf("kuscia: list domain data for %s failed: [%d] %s", domainID, resp.Status.Code, resp.Status.Message)
	}
	return resp.Data.DomainDataList, nil
}

// DeleteDomainData removes domain data from Kuscia.
func (c *Client) DeleteDomainData(ctx context.Context, domainID, domainDataID string) error {
	var resp DeleteDomainDataResponse
	req := &DeleteDomainDataRequest{DomainID: domainID, DomainDataID: domainDataID}
	if err := c.doRequest(ctx, "/api/v1alpha1/domaindata/delete", req, &resp); err != nil {
		return err
	}
	if !resp.Status.IsSuccess() {
		return fmt.Errorf("kuscia: delete domain data %s failed: [%d] %s", domainDataID, resp.Status.Code, resp.Status.Message)
	}
	return nil
}

// --- DomainData Grant Types ---

// GrantDomainDataRequest represents a Kuscia GrantDomainData API request.
type GrantDomainDataRequest struct {
	DomainID     string `json:"domain_id"`
	DomainDataID string `json:"domaindata_id"`
	GrantDomain  string `json:"grant_domain"`
}

// GrantDomainDataResponse represents a Kuscia GrantDomainData API response.
type GrantDomainDataResponse struct {
	Status Status `json:"status"`
}

// GrantDomainData grants access to domain data for another domain.
func (c *Client) GrantDomainData(ctx context.Context, req *GrantDomainDataRequest) error {
	var resp GrantDomainDataResponse
	if err := c.doRequest(ctx, "/api/v1alpha1/domaindata/grant", req, &resp); err != nil {
		return err
	}
	if !resp.Status.IsSuccess() {
		return fmt.Errorf("kuscia: grant domain data %s to %s failed: [%d] %s", req.DomainDataID, req.GrantDomain, resp.Status.Code, resp.Status.Message)
	}
	return nil
}
