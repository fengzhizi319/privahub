package kuscia

import (
	"context"
	"fmt"
)

// --- DomainDataSource Types ---
// Corresponds to Java DataProxyService which manages Kuscia DomainDatasource
// access_directly flag for data proxy routing.

// DomainDataSource represents a Kuscia DomainDataSource entry.
type DomainDataSource struct {
	DomainID       string `json:"domain_id"`
	DatasourceID   string `json:"datasource_id"`
	Name           string `json:"name,omitempty"`
	Type           string `json:"type,omitempty"`
	AccessDirectly bool   `json:"access_directly"`
}

// ListDomainDataSourceRequest represents a Kuscia ListDomainDataSource API request.
type ListDomainDataSourceRequest struct {
	DomainID string `json:"domain_id"`
}

// ListDomainDataSourceResponse represents a Kuscia ListDomainDataSource API response.
type ListDomainDataSourceResponse struct {
	Status Status `json:"status"`
	Data   struct {
		DatasourceList []DomainDataSource `json:"datasource_list"`
	} `json:"data"`
}

// UpdateDomainDataSourceRequest represents a Kuscia UpdateDomainDataSource API request.
type UpdateDomainDataSourceRequest struct {
	DomainID       string `json:"domain_id"`
	DatasourceID   string `json:"datasource_id"`
	AccessDirectly *bool  `json:"access_directly,omitempty"`
}

// UpdateDomainDataSourceResponse represents a Kuscia UpdateDomainDataSource API response.
type UpdateDomainDataSourceResponse struct {
	Status Status `json:"status"`
}

// --- DomainDataSource Service Methods ---

// ListDomainDataSource lists all data sources for a domain.
func (c *Client) ListDomainDataSource(ctx context.Context, domainID string) ([]DomainDataSource, error) {
	var resp ListDomainDataSourceResponse
	req := &ListDomainDataSourceRequest{DomainID: domainID}
	if err := c.doRequest(ctx, "/api/v1/domaindatasource/list", req, &resp); err != nil {
		return nil, err
	}
	if !resp.Status.IsSuccess() {
		return nil, fmt.Errorf("kuscia: list domain datasource for %s failed: [%d] %s", domainID, resp.Status.Code, resp.Status.Message)
	}
	return resp.Data.DatasourceList, nil
}

// UpdateDomainDataSource updates a domain data source (e.g., toggle access_directly).
func (c *Client) UpdateDomainDataSource(ctx context.Context, req *UpdateDomainDataSourceRequest) error {
	var resp UpdateDomainDataSourceResponse
	if err := c.doRequest(ctx, "/api/v1/domaindatasource/update", req, &resp); err != nil {
		return err
	}
	if !resp.Status.IsSuccess() {
		return fmt.Errorf("kuscia: update domain datasource %s failed: [%d] %s", req.DatasourceID, resp.Status.Code, resp.Status.Message)
	}
	return nil
}
