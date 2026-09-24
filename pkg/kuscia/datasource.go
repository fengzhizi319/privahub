package kuscia

import (
	"context"
	"fmt"
)

// --- DomainDataSource Types ---
// Mirrors kuscia/proto/api/v1alpha1/kusciaapi/domaindatasource.proto.
// HTTP routes: /api/v1/domaindatasource/{create,update,delete,query,batchQuery,list}.

// Kuscia datasource types (DomainDataSource.type).
const (
	DataSourceTypeLocalFS    = "localfs"
	DataSourceTypeOSS        = "oss"
	DataSourceTypeMySQL      = "mysql"
	DataSourceTypeODPS       = "odps"
	DataSourceTypePostgreSQL = "postgresql"
)

// LocalDataSourceInfo is the localfs datasource info.
type LocalDataSourceInfo struct {
	Path string `json:"path,omitempty"`
}

// OssDataSourceInfo is the S3/OSS/MinIO datasource info.
type OssDataSourceInfo struct {
	Endpoint        string `json:"endpoint,omitempty"`
	Bucket          string `json:"bucket,omitempty"`
	Prefix          string `json:"prefix,omitempty"`
	AccessKeyID     string `json:"access_key_id,omitempty"`
	AccessKeySecret string `json:"access_key_secret,omitempty"`
	Virtualhost     bool   `json:"virtualhost,omitempty"`
	Version         string `json:"version,omitempty"`      // s3v2 / s3v4
	StorageType     string `json:"storage_type,omitempty"` // deprecated: oss / minio
}

// DatabaseDataSourceInfo is the relational database (mysql/postgresql) datasource info.
type DatabaseDataSourceInfo struct {
	Endpoint string `json:"endpoint,omitempty"`
	User     string `json:"user,omitempty"`
	Password string `json:"password,omitempty"`
	Database string `json:"database,omitempty"`
}

// OdpsDataSourceInfo is the aliyun ODPS (MaxCompute) datasource info.
type OdpsDataSourceInfo struct {
	Endpoint        string `json:"endpoint,omitempty"`
	Project         string `json:"project,omitempty"`
	AccessKeyID     string `json:"access_key_id,omitempty"`
	AccessKeySecret string `json:"access_key_secret,omitempty"`
}

// DataSourceInfo holds exactly one of the typed infos.
type DataSourceInfo struct {
	Localfs  *LocalDataSourceInfo    `json:"localfs,omitempty"`
	Oss      *OssDataSourceInfo      `json:"oss,omitempty"`
	Database *DatabaseDataSourceInfo `json:"database,omitempty"`
	Odps     *OdpsDataSourceInfo     `json:"odps,omitempty"`
}

// DomainDataSource represents a Kuscia DomainDataSource entry.
type DomainDataSource struct {
	DomainID       string          `json:"domain_id"`
	DatasourceID   string          `json:"datasource_id"`
	Name           string          `json:"name,omitempty"`
	Type           string          `json:"type,omitempty"`
	Status         string          `json:"status,omitempty"` // Available / Unavailable
	Info           *DataSourceInfo `json:"info,omitempty"`
	InfoKey        string          `json:"info_key,omitempty"`
	AccessDirectly bool            `json:"access_directly"`
}

// DomainDataSourceList wraps a list of datasources.
type DomainDataSourceList struct {
	DatasourceList []DomainDataSource `json:"datasource_list"`
}

// CreateDomainDataSourceRequest represents a Kuscia CreateDomainDataSource request.
type CreateDomainDataSourceRequest struct {
	DomainID       string          `json:"domain_id"`
	DatasourceID   string          `json:"datasource_id,omitempty"`
	Type           string          `json:"type"`
	Name           string          `json:"name,omitempty"`
	Info           *DataSourceInfo `json:"info,omitempty"`
	InfoKey        string          `json:"info_key,omitempty"`
	AccessDirectly *bool           `json:"access_directly,omitempty"`
}

// CreateDomainDataSourceResponse represents a Kuscia CreateDomainDataSource response.
type CreateDomainDataSourceResponse struct {
	Status Status `json:"status"`
	Data   struct {
		DatasourceID string `json:"datasource_id"`
	} `json:"data"`
}

// QueryDomainDataSourceRequest represents a Kuscia QueryDomainDataSource request (flat, not wrapped).
type QueryDomainDataSourceRequest struct {
	DomainID     string `json:"domain_id"`
	DatasourceID string `json:"datasource_id"`
}

// QueryDomainDataSourceRequestData is one item of a batch query.
type QueryDomainDataSourceRequestData = QueryDomainDataSourceRequest

// QueryDomainDataSourceResponse represents a Kuscia QueryDomainDataSource response.
type QueryDomainDataSourceResponse struct {
	Status Status           `json:"status"`
	Data   DomainDataSource `json:"data"`
}

// BatchQueryDomainDataSourceRequest represents a batch query request.
type BatchQueryDomainDataSourceRequest struct {
	Data []QueryDomainDataSourceRequestData `json:"data"`
}

// BatchQueryDomainDataSourceResponse represents a batch query response.
type BatchQueryDomainDataSourceResponse struct {
	Status Status               `json:"status"`
	Data   DomainDataSourceList `json:"data"`
}

// DeleteDomainDataSourceRequest represents a Kuscia DeleteDomainDataSource request.
type DeleteDomainDataSourceRequest struct {
	DomainID     string `json:"domain_id"`
	DatasourceID string `json:"datasource_id"`
}

// ListDomainDataSourceRequest represents a Kuscia ListDomainDataSource API request.
type ListDomainDataSourceRequest struct {
	DomainID string `json:"domain_id"`
}

// ListDomainDataSourceResponse represents a Kuscia ListDomainDataSource API response.
type ListDomainDataSourceResponse struct {
	Status Status               `json:"status"`
	Data   DomainDataSourceList `json:"data"`
}

// UpdateDomainDataSourceRequest represents a Kuscia UpdateDomainDataSource API request.
type UpdateDomainDataSourceRequest struct {
	DomainID       string          `json:"domain_id"`
	DatasourceID   string          `json:"datasource_id"`
	Type           string          `json:"type,omitempty"`
	Name           string          `json:"name,omitempty"`
	Info           *DataSourceInfo `json:"info,omitempty"`
	InfoKey        string          `json:"info_key,omitempty"`
	AccessDirectly *bool           `json:"access_directly,omitempty"`
}

// UpdateDomainDataSourceResponse represents a Kuscia UpdateDomainDataSource API response.
type UpdateDomainDataSourceResponse struct {
	Status Status `json:"status"`
}

// statusOnly is used for responses that only carry a status.
type statusOnly struct {
	Status Status `json:"status"`
}

// --- DomainDataSource Service Methods ---

// CreateDomainDataSource creates a datasource in the given domain and returns its id.
func (c *Client) CreateDomainDataSource(ctx context.Context, req *CreateDomainDataSourceRequest) (string, error) {
	var resp CreateDomainDataSourceResponse
	if err := c.doRequest(ctx, "/api/v1/domaindatasource/create", req, &resp); err != nil {
		return "", err
	}
	if !resp.Status.IsSuccess() {
		return "", fmt.Errorf("kuscia: create domain datasource %s/%s failed: [%d] %s", req.DomainID, req.DatasourceID, resp.Status.Code, resp.Status.Message)
	}
	id := resp.Data.DatasourceID
	if id == "" {
		id = req.DatasourceID
	}
	return id, nil
}

// QueryDomainDataSource queries one datasource of a domain.
func (c *Client) QueryDomainDataSource(ctx context.Context, domainID, datasourceID string) (*DomainDataSource, error) {
	var resp QueryDomainDataSourceResponse
	req := &QueryDomainDataSourceRequest{DomainID: domainID, DatasourceID: datasourceID}
	if err := c.doRequest(ctx, "/api/v1/domaindatasource/query", req, &resp); err != nil {
		return nil, err
	}
	if !resp.Status.IsSuccess() {
		return nil, fmt.Errorf("kuscia: query domain datasource %s/%s failed: [%d] %s", domainID, datasourceID, resp.Status.Code, resp.Status.Message)
	}
	return &resp.Data, nil
}

// BatchQueryDomainDataSource queries several datasources at once.
func (c *Client) BatchQueryDomainDataSource(ctx context.Context, items []QueryDomainDataSourceRequestData) ([]DomainDataSource, error) {
	var resp BatchQueryDomainDataSourceResponse
	if err := c.doRequest(ctx, "/api/v1/domaindatasource/batchQuery", &BatchQueryDomainDataSourceRequest{Data: items}, &resp); err != nil {
		return nil, err
	}
	if !resp.Status.IsSuccess() {
		return nil, fmt.Errorf("kuscia: batch query domain datasource failed: [%d] %s", resp.Status.Code, resp.Status.Message)
	}
	return resp.Data.DatasourceList, nil
}

// DeleteDomainDataSource deletes a datasource from a domain.
func (c *Client) DeleteDomainDataSource(ctx context.Context, domainID, datasourceID string) error {
	var resp statusOnly
	req := &DeleteDomainDataSourceRequest{DomainID: domainID, DatasourceID: datasourceID}
	if err := c.doRequest(ctx, "/api/v1/domaindatasource/delete", req, &resp); err != nil {
		return err
	}
	if !resp.Status.IsSuccess() {
		return fmt.Errorf("kuscia: delete domain datasource %s/%s failed: [%d] %s", domainID, datasourceID, resp.Status.Code, resp.Status.Message)
	}
	return nil
}

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
