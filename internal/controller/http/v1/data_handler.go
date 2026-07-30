package v1

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/pkg/errcode"
	"github.com/fengzhizi319/privahub/pkg/kuscia"
	"github.com/fengzhizi319/privahub/pkg/response"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// DataHandler handles data upload/download/sync and feature datasource endpoints.
type DataHandler struct {
	db           *gorm.DB
	kusciaClient *kuscia.Client
	dataDir      string
}

// NewDataHandler creates a new DataHandler.
func NewDataHandler(db *gorm.DB, kusciaClient *kuscia.Client) *DataHandler {
	dataDir := os.Getenv("PRIVAHUB_DATA_DIR")
	if dataDir == "" {
		dataDir = "./data"
	}
	_ = os.MkdirAll(dataDir, 0o755)
	return &DataHandler{db: db, kusciaClient: kusciaClient, dataDir: dataDir}
}

// --- Data Endpoints ---

// Upload handles data file upload and saves to data directory.
// Filenames are sanitized to prevent path traversal attacks.
func (h *DataHandler) Upload(c *gin.Context) {
	// Limit upload size to 512 MB to prevent resource exhaustion.
	const maxUploadSize = 512 << 20 // 512 MB
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxUploadSize)

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}
	defer file.Close()

	nodeID := c.Query("Node-Id")
	if nodeID == "" {
		nodeID = c.GetHeader("Node-Id")
	}
	if nodeID == "" {
		nodeID = c.PostForm("node_id")
	}
	datasourceID := c.PostForm("datasource_id")

	// Sanitize nodeID and filename to prevent path traversal attacks (e.g. ../../etc/passwd)
	safeNodeID := sanitizePathSegment(nodeID)
	safeFilename := sanitizePathSegment(header.Filename)
	if safeNodeID == "" || safeFilename == "" {
		response.Fail(c, errcode.ParamError)
		return
	}

	// Save file to data directory
	nodeDir := filepath.Join(h.dataDir, safeNodeID)
	_ = os.MkdirAll(nodeDir, 0o755)
	dstPath := filepath.Join(nodeDir, safeFilename)

	out, err := os.Create(dstPath)
	if err != nil {
		response.Fail(c, errcode.SystemError)
		return
	}
	defer out.Close()

	written, err := io.Copy(out, file)
	if err != nil {
		response.Fail(c, errcode.SystemError)
		return
	}

	response.OK(c, gin.H{
		"name":            header.Filename,
		"real_name":       header.Filename,
		"datasource":      datasourceID,
		"datasource_type": "local",
		"filename":        header.Filename,
		"size":            written,
		"node_id":         nodeID,
		"datasource_id":   datasourceID,
		"path":            dstPath,
		"status":          "uploaded",
	})
}

// Create handles data table creation from uploaded file.
func (h *DataHandler) Create(c *gin.Context) {
	var req struct {
		NodeID       string `json:"node_id" binding:"required"`
		DatasourceID string `json:"datasource_id" binding:"required"`
		TableName    string `json:"table_name" binding:"required"`
		Description  string `json:"description"`
		Columns      []struct {
			Name string `json:"name"`
			Type string `json:"type"`
		} `json:"columns"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}

	// Register as a datatable
	dt := &model.ProjectDatatableDO{
		DatatableID: req.NodeID + "-" + req.TableName,
		NodeID:      req.NodeID,
		Source:      "CREATED",
	}
	if err := h.db.WithContext(c.Request.Context()).Create(dt).Error; err != nil {
		response.Fail(c, errcode.AlreadyExists)
		return
	}

	response.OK(c, gin.H{
		"datatable_id":   dt.DatatableID,
		"datatable_name": req.TableName,
	})
}

// Download handles data file download from data directory.
func (h *DataHandler) Download(c *gin.Context) {
	// Frontend sends {nodeId, domainDataId} via raw fetch; accept snake/camel variants.
	var req struct {
		DatatableID    string `json:"datatable_id"`
		DatatableIDAlt string `json:"domainDataId"`
		NodeID         string `json:"node_id"`
		NodeIDAlt      string `json:"nodeId"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}
	if req.DatatableID == "" {
		req.DatatableID = req.DatatableIDAlt
	}
	if req.NodeID == "" {
		req.NodeID = req.NodeIDAlt
	}
	if req.DatatableID == "" {
		response.Fail(c, errcode.ParamError)
		return
	}

	// Sanitize inputs to prevent path traversal attacks
	safeNodeID := sanitizePathSegment(req.NodeID)
	safeDatatableID := sanitizePathSegment(req.DatatableID)
	if safeDatatableID == "" {
		response.Fail(c, errcode.ParamError)
		return
	}

	// Look up file in data directory
	nodeDir := filepath.Join(h.dataDir, safeNodeID)
	filePath := filepath.Join(nodeDir, safeDatatableID+".csv")

	if _, err := os.Stat(filePath); err == nil {
		c.File(filePath)
		return
	}

	// Job outputs are stored by their relative URI (no extension, ORC payload).
	if _, err := os.Stat(filepath.Join(nodeDir, safeDatatableID)); err == nil {
		c.File(filepath.Join(nodeDir, safeDatatableID))
		return
	}

	// File not found on disk — return metadata reference
	response.OK(c, gin.H{
		"datatable_id": req.DatatableID,
		"download_url": "/api/v1alpha1/data/file/" + req.DatatableID,
		"note":         "file not found on local disk, may reside on remote node",
	})
}

// Sync handles data sync between nodes via Kuscia DomainData.
func (h *DataHandler) Sync(c *gin.Context) {
	var req struct {
		NodeID      string `json:"node_id" binding:"required"`
		DatatableID string `json:"datatable_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}

	// Trigger Kuscia DomainData query to verify data availability
	syncStatus := "synced"
	if h.kusciaClient != nil && req.DatatableID != "" {
		_, err := h.kusciaClient.QueryDomainData(c.Request.Context(), req.NodeID, req.DatatableID)
		if err != nil {
			syncStatus = fmt.Sprintf("sync_warning: %v", err)
		}
	}

	response.OK(c, gin.H{
		"node_id":      req.NodeID,
		"datatable_id": req.DatatableID,
		"status":       syncStatus,
	})
}

// --- Feature Datasource Endpoints ---

// FeatureDatasourceCreate handles feature datasource creation.
func (h *DataHandler) FeatureDatasourceCreate(c *gin.Context) {
	var req struct {
		Name   string `json:"name" binding:"required"`
		Type   string `json:"type"`
		URI    string `json:"uri"`
		NodeID string `json:"node_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}

	dsType := req.Type
	if dsType == "" {
		dsType = "OSS"
	}
	ds := &model.DatasourceDO{
		DatasourceID: "fds-" + req.Name,
		Name:         req.Name,
		Type:         dsType,
		Status:       "Available",
		OwnerID:      req.NodeID,
	}
	if err := h.db.WithContext(c.Request.Context()).Create(ds).Error; err != nil {
		response.Fail(c, errcode.AlreadyExists)
		return
	}

	response.OK(c, gin.H{
		"datasource_id":   ds.DatasourceID,
		"datasource_name": ds.Name,
	})
}

// FeatureDatasourceAuthList handles feature datasource auth list via Kuscia DomainData.
func (h *DataHandler) FeatureDatasourceAuthList(c *gin.Context) {
	var req struct {
		NodeID string `json:"node_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		// optional param
	}

	// Query Kuscia DomainData for the node's authorized data list
	if h.kusciaClient != nil && req.NodeID != "" {
		items, err := h.kusciaClient.ListDomainData(c.Request.Context(), req.NodeID)
		if err == nil {
			result := make([]gin.H, 0, len(items))
			for _, item := range items {
				result = append(result, gin.H{
					"domaindata_id": item.DomainDataID,
					"name":          item.Name,
					"type":          item.Type,
					"datasource_id": item.DatasourceID,
					"author":        item.Author,
				})
			}
			response.OK(c, result)
			return
		}
	}

	// Fallback: return empty auth list
	response.OK(c, []gin.H{})
}

// --- Cloud Log Endpoint ---

// CloudLogSls handles cloud log SLS query.
func (h *DataHandler) CloudLogSls(c *gin.Context) {
	var req struct {
		ProjectID string `json:"project_id"`
		JobID     string `json:"job_id"`
		TaskID    string `json:"task_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}

	// Cloud log integration (SLS) - return empty logs
	response.OK(c, gin.H{
		"logs":  []string{},
		"total": 0,
	})
}

// --- Vote Sync Endpoint ---

// sanitizePathSegment strips directory components and rejects path traversal
// sequences from user-supplied path segments. Returns only the base filename
// with any leading dots/dashes removed to prevent hidden files or option injection.
func sanitizePathSegment(s string) string {
	if s == "" {
		return ""
	}
	// Take only the base name to strip any directory components
	base := filepath.Base(s)
	// Reject traversal patterns that survive Base (e.g. ".." on some platforms)
	if base == "." || base == ".." || base == string(filepath.Separator) {
		return ""
	}
	// Remove leading dots and dashes to prevent hidden files / CLI option injection
	base = strings.TrimLeft(base, ".-")
	if base == "" {
		return ""
	}
	return base
}

// VoteSyncCreate handles vote sync creation (P2P vote synchronization via Kuscia).
func (h *DataHandler) VoteSyncCreate(c *gin.Context) {
	var req struct {
		VoteID string `json:"vote_id" binding:"required"`
		NodeID string `json:"node_id" binding:"required"`
		Action string `json:"action"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}

	// Sync vote state to remote node via Kuscia Domain query (verifies node reachability)
	syncStatus := "synced"
	if h.kusciaClient != nil {
		_, err := h.kusciaClient.QueryDomain(c.Request.Context(), req.NodeID)
		if err != nil {
			syncStatus = "sync_failed: node unreachable"
		}
	}

	response.OK(c, gin.H{
		"vote_id": req.VoteID,
		"node_id": req.NodeID,
		"action":  req.Action,
		"status":  syncStatus,
	})
}
