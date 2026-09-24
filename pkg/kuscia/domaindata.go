package kuscia

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
)

// --- DomainData Types ---
// Mirrors kuscia/proto/api/v1alpha1/kusciaapi/domaindata.proto and common.proto.
// HTTP routes: /api/v1/domaindata/{create,update,delete,deleteDataAndRaw,query,batchQuery,list}.

// DomainData type / vendor constants.
const (
	DomainDataTypeTable  = "table"
	DomainDataTypeModel  = "model"
	DomainDataTypeRule   = "rule"
	DomainDataTypeReport = "report"

	DomainDataVendorManual     = "manual"
	DomainDataVendorSecretFlow = "secretflow"

	PartitionTypeOdps = "odps"
	PartitionTypePath = "path"
)

// FileFormat is the common.proto FileFormat enum. Kuscia's HTTP gateway
// (protojson) accepts the enum name ("CSV") or its number and renders names;
// FileFormat accepts both on decode.
type FileFormat string

// FileFormat values.
const (
	FileFormatUnknown FileFormat = "UNKNOWN"
	FileFormatCSV     FileFormat = "CSV"
	FileFormatBinary  FileFormat = "BINARY"
)

// UnmarshalJSON accepts "CSV" / 1 style encodings.
func (f *FileFormat) UnmarshalJSON(b []byte) error {
	if len(b) > 0 && b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		*f = FileFormat(s)
		return nil
	}
	n, err := strconv.Atoi(string(b))
	if err != nil {
		if string(b) == "null" {
			*f = ""
			return nil
		}
		return err
	}
	switch n {
	case 1:
		*f = FileFormatCSV
	case 2:
		*f = FileFormatBinary
	default:
		*f = FileFormatUnknown
	}
	return nil
}

// DataColumn represents a column in domain data.
// Per kuscia proto (common.proto DataColumn), the free-text field is `comment`.
type DataColumn struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Comment     string `json:"comment,omitempty"`
	NotNullable bool   `json:"not_nullable,omitempty"`
}

// Partition is the common.proto Partition message (type: odps / path).
type Partition struct {
	Type   string       `json:"type,omitempty"`
	Fields []DataColumn `json:"fields,omitempty"`
}

// CreateDomainDataRequest represents a Kuscia CreateDomainData API request.
type CreateDomainDataRequest struct {
	DomainID     string            `json:"domain_id"`
	DomainDataID string            `json:"domaindata_id,omitempty"`
	Name         string            `json:"name"`
	Type         string            `json:"type"` // table / model / rule / report
	RelativeURI  string            `json:"relative_uri"`
	DatasourceID string            `json:"datasource_id,omitempty"`
	Attributes   map[string]string `json:"attributes,omitempty"`
	Partition    *Partition        `json:"partition,omitempty"`
	Columns      []DataColumn      `json:"columns,omitempty"`
	Vendor       string            `json:"vendor,omitempty"`
	FileFormat   FileFormat        `json:"file_format,omitempty"`
	// Author is not part of the Kuscia create request (Kuscia derives it);
	// kept for backward compatibility and never serialized.
	Author string `json:"-"`
}

// UpdateDomainDataRequest represents a Kuscia UpdateDomainData API request.
type UpdateDomainDataRequest = CreateDomainDataRequest

// CreateDomainDataResponse represents a Kuscia CreateDomainData API response.
type CreateDomainDataResponse struct {
	Status Status `json:"status"`
	Data   struct {
		DomainDataID string `json:"domaindata_id"`
	} `json:"data"`
}

// DomainData is the Kuscia DomainData message.
type DomainData struct {
	DomainDataID string            `json:"domaindata_id"`
	Name         string            `json:"name"`
	Type         string            `json:"type"`
	RelativeURI  string            `json:"relative_uri"`
	DomainID     string            `json:"domain_id,omitempty"`
	DatasourceID string            `json:"datasource_id"`
	Attributes   map[string]string `json:"attributes,omitempty"`
	Partition    *Partition        `json:"partition,omitempty"`
	Columns      []DataColumn      `json:"columns"`
	Vendor       string            `json:"vendor,omitempty"`
	Status       string            `json:"status,omitempty"`
	Author       string            `json:"author"`
	FileFormat   FileFormat        `json:"file_format,omitempty"`
}

// DomainDataItem is kept as an alias of DomainData for existing callers.
type DomainDataItem = DomainData

// DomainDataList wraps a list of DomainData.
type DomainDataList struct {
	DomainDataList []DomainData `json:"domaindata_list"`
}

// QueryDomainDataRequest represents a Kuscia QueryDomainData API request.
// The Kuscia HTTP API expects the fields wrapped in a `data` sub-message
// (see QueryDomainDataRequest in domaindata.proto).
type QueryDomainDataRequest struct {
	Data QueryDomainDataRequestData `json:"data"`
}

// QueryDomainDataRequestData holds the query keys.
type QueryDomainDataRequestData struct {
	DomainID     string `json:"domain_id"`
	DomainDataID string `json:"domaindata_id"`
}

// QueryDomainDataResponse represents a Kuscia QueryDomainData API response.
type QueryDomainDataResponse struct {
	Status Status     `json:"status"`
	Data   DomainData `json:"data"`
}

// BatchQueryDomainDataRequest represents a Kuscia BatchQueryDomainData request.
type BatchQueryDomainDataRequest struct {
	Data []QueryDomainDataRequestData `json:"data"`
}

// BatchQueryDomainDataResponse represents a Kuscia BatchQueryDomainData response.
type BatchQueryDomainDataResponse struct {
	Status Status         `json:"status"`
	Data   DomainDataList `json:"data"`
}

// ListDomainDataRequest represents a Kuscia ListDomainData API request.
// Like QueryDomainDataRequest, the fields are wrapped in a `data` sub-message.
type ListDomainDataRequest struct {
	Data ListDomainDataRequestData `json:"data"`
}

// ListDomainDataRequestData holds the list filter.
type ListDomainDataRequestData struct {
	DomainID         string `json:"domain_id"`
	DomainDataType   string `json:"domaindata_type,omitempty"`
	DomainDataVendor string `json:"domaindata_vendor,omitempty"`
}

// ListDomainDataResponse represents a Kuscia ListDomainData API response.
type ListDomainDataResponse struct {
	Status Status         `json:"status"`
	Data   DomainDataList `json:"data"`
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

// --- DomainData Service Methods ---

// CreateDomainData registers domain data (datatable) in Kuscia.
func (c *Client) CreateDomainData(ctx context.Context, req *CreateDomainDataRequest) error {
	_, err := c.CreateDomainDataWithID(ctx, req)
	return err
}

// CreateDomainDataWithID registers domain data and returns the domaindata id
// assigned by Kuscia (equal to req.DomainDataID when provided).
func (c *Client) CreateDomainDataWithID(ctx context.Context, req *CreateDomainDataRequest) (string, error) {
	var resp CreateDomainDataResponse
	if err := c.doRequest(ctx, "/api/v1/domaindata/create", req, &resp); err != nil {
		return "", err
	}
	if !resp.Status.IsSuccess() {
		return "", fmt.Errorf("kuscia: create domain data %s failed: [%d] %s", req.DomainDataID, resp.Status.Code, resp.Status.Message)
	}
	id := resp.Data.DomainDataID
	if id == "" {
		id = req.DomainDataID
	}
	return id, nil
}

// UpdateDomainData updates domain data metadata.
func (c *Client) UpdateDomainData(ctx context.Context, req *UpdateDomainDataRequest) error {
	var resp statusOnly
	if err := c.doRequest(ctx, "/api/v1/domaindata/update", req, &resp); err != nil {
		return err
	}
	if !resp.Status.IsSuccess() {
		return fmt.Errorf("kuscia: update domain data %s failed: [%d] %s", req.DomainDataID, resp.Status.Code, resp.Status.Message)
	}
	return nil
}

// QueryDomainData queries domain data details.
func (c *Client) QueryDomainData(ctx context.Context, domainID, domainDataID string) (*QueryDomainDataResponse, error) {
	var resp QueryDomainDataResponse
	req := &QueryDomainDataRequest{Data: QueryDomainDataRequestData{DomainID: domainID, DomainDataID: domainDataID}}
	if err := c.doRequest(ctx, "/api/v1/domaindata/query", req, &resp); err != nil {
		return nil, err
	}
	if !resp.Status.IsSuccess() {
		return nil, fmt.Errorf("kuscia: query domain data %s failed: [%d] %s", domainDataID, resp.Status.Code, resp.Status.Message)
	}
	return &resp, nil
}

// BatchQueryDomainData queries several domain data at once.
func (c *Client) BatchQueryDomainData(ctx context.Context, items []QueryDomainDataRequestData) ([]DomainData, error) {
	var resp BatchQueryDomainDataResponse
	if err := c.doRequest(ctx, "/api/v1/domaindata/batchQuery", &BatchQueryDomainDataRequest{Data: items}, &resp); err != nil {
		return nil, err
	}
	if !resp.Status.IsSuccess() {
		return nil, fmt.Errorf("kuscia: batch query domain data failed: [%d] %s", resp.Status.Code, resp.Status.Message)
	}
	return resp.Data.DomainDataList, nil
}

// ListDomainData lists all domain data for a domain.
func (c *Client) ListDomainData(ctx context.Context, domainID string) ([]DomainDataItem, error) {
	return c.ListDomainDataWithFilter(ctx, &ListDomainDataRequestData{DomainID: domainID})
}

// ListDomainDataWithFilter lists domain data filtered by type and/or vendor.
func (c *Client) ListDomainDataWithFilter(ctx context.Context, filter *ListDomainDataRequestData) ([]DomainData, error) {
	var resp ListDomainDataResponse
	if err := c.doRequest(ctx, "/api/v1/domaindata/list", &ListDomainDataRequest{Data: *filter}, &resp); err != nil {
		return nil, err
	}
	if !resp.Status.IsSuccess() {
		return nil, fmt.Errorf("kuscia: list domain data for %s failed: [%d] %s", filter.DomainID, resp.Status.Code, resp.Status.Message)
	}
	return resp.Data.DomainDataList, nil
}

// DeleteDomainData removes domain data from Kuscia.
func (c *Client) DeleteDomainData(ctx context.Context, domainID, domainDataID string) error {
	var resp DeleteDomainDataResponse
	req := &DeleteDomainDataRequest{DomainID: domainID, DomainDataID: domainDataID}
	if err := c.doRequest(ctx, "/api/v1/domaindata/delete", req, &resp); err != nil {
		return err
	}
	if !resp.Status.IsSuccess() {
		return fmt.Errorf("kuscia: delete domain data %s failed: [%d] %s", domainDataID, resp.Status.Code, resp.Status.Message)
	}
	return nil
}
