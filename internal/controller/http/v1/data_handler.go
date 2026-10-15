package v1

import (
	"net/url"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/fengzhizi319/privahub/internal/controller/http/middleware"
	"github.com/fengzhizi319/privahub/internal/service"
	"github.com/fengzhizi319/privahub/pkg/errcode"
	"github.com/fengzhizi319/privahub/pkg/kuscia"
	"github.com/fengzhizi319/privahub/pkg/response"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// DataHandler handles /data/*, /feature_datasource/* and /cloud_log/sls
// (vote_sync/create and data/sync live in sync_handler.go).
type DataHandler struct {
	db           *gorm.DB
	kusciaClient *kuscia.Client
	dataService  *service.DataService
	featureSvc   *service.FeatureTableService
	cloudLog     *service.CloudLogService
}

// NewDataHandler creates a DataHandler with default services (data dir from
// $PRIVAHUB_DATA_DIR or ./data, Java default IP block list, SLS unconfigured).
// Use WithServices to inject configured ones.
func NewDataHandler(db *gorm.DB, kusciaClient *kuscia.Client) *DataHandler {
	return &DataHandler{
		db:           db,
		kusciaClient: kusciaClient,
		dataService:  service.NewDataService(db, kusciaClient, nil, ""),
		featureSvc:   service.NewFeatureTableService(db),
	}
}

// WithServices injects the configured services (nil keeps the current one).
func (h *DataHandler) WithServices(data *service.DataService, feature *service.FeatureTableService, cloudLog *service.CloudLogService) *DataHandler {
	if data != nil {
		h.dataService = data
	}
	if feature != nil {
		h.featureSvc = feature
	}
	if cloudLog != nil {
		h.cloudLog = cloudLog
	}
	return h
}

// --- Data Endpoints (Java DataController) ---

// Upload handles POST /data/upload?Node-Id=<nodeId> (multipart "file").
// Response: {name, realName, datasource: "default-data-source", datasourceType: "localfs"}
func (h *DataHandler) Upload(c *gin.Context) {
	nodeID := c.Query("Node-Id")
	if nodeID == "" {
		nodeID = c.GetHeader("Node-Id")
	}
	if nodeID == "" {
		nodeID = c.PostForm("Node-Id")
	}
	// Java checkDataPermissions, applied to every non-CENTER-owner account
	// (EDGE and P2P users alike): the target node must be the caller's own
	// node or a node of its institution. A missing Node-Id is denied.
	if !middleware.RequireNodeAccess(c, nodeID) {
		return
	}
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		response.FailWithMsg(c, errcode.ParamError, err.Error())
		return
	}
	defer file.Close()
	vo, err := h.dataService.Upload(c.Request.Context(), nodeID, header.Filename, file)
	if err != nil {
		failDataErr(c, err, errcode.SystemError)
		return
	}
	response.OK(c, vo)
}

// Create handles POST /data/create (Java DataController.createData).
// Request: {nodeId, name, realName?, tableName, description?, datasourceType, datasourceName,
// datatableSchema[{featureName,featureType,featureDescription}], nullStrs?[]}
// Response: data = domainDataId (string)
func (h *DataHandler) Create(c *gin.Context) {
	var req service.CreateDataRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMsg(c, errcode.ParamError, err.Error())
		return
	}
	id, err := h.dataService.CreateData(c.Request.Context(), &req)
	if err != nil {
		failDataErr(c, err, errcode.SystemError)
		return
	}
	response.OK(c, id)
}

// Download handles POST /data/download {nodeId, domainDataId}: streams the
// result file (CSV, or tar.gz for directories / Model / Rule results) as an
// attachment; an empty/missing file yields the body "No data".
func (h *DataHandler) Download(c *gin.Context) {
	var req service.DownloadDataRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMsg(c, errcode.ParamError, err.Error())
		return
	}
	info, err := h.dataService.Download(c.Request.Context(), &req)
	if err != nil {
		failDataErr(c, err, errcode.SystemError)
		return
	}
	c.Header("Content-Disposition", "attachment;filename*=UTF-8''"+url.PathEscape(info.FileName))
	if info.Empty() {
		c.Data(200, "application/octet-stream", []byte("No data"))
		return
	}
	// Stream the payload (io.Copy / on-the-fly tar.gz) instead of reading it
	// into memory; archives have no known length and are sent chunked.
	c.Header("Content-Type", "application/octet-stream")
	if info.Size >= 0 {
		c.Header("Content-Length", strconv.FormatInt(info.Size, 10))
	}
	c.Status(200)
	if _, err := info.WriteTo(c.Writer); err != nil {
		// Headers are already sent: the client sees a short (Content-Length)
		// or corrupt (gzip) transfer.
		zap.L().Warn("data download stream failed", zap.String("file", info.FileName), zap.Error(err))
	}
}

// --- Feature Datasource Endpoints (Java FeatureDatasourceController) ---

// FeatureDatasourceCreate handles POST /feature_datasource/create.
// Request: {ownerId, nodeIds[], featureTableName, type: "HTTP", desc?, url, columns[{colName,colType,colComment}], datasourceId?}
func (h *DataHandler) FeatureDatasourceCreate(c *gin.Context) {
	var req service.CreateFeatureDatasourceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMsg(c, errcode.ParamError, err.Error())
		return
	}
	if err := h.featureSvc.CreateFeatureTable(c.Request.Context(), &req); err != nil {
		failDataErr(c, err, errcode.SystemError)
		return
	}
	response.OKEmpty(c)
}

// FeatureDatasourceAuthList handles POST /feature_datasource/auth/list.
// Request: {projectId, nodeId}
// Response: [{nodeId, featureTableId, featureTableName, columns[]}]
func (h *DataHandler) FeatureDatasourceAuthList(c *gin.Context) {
	var req service.ListProjectFeatureDatasourceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMsg(c, errcode.ParamError, err.Error())
		return
	}
	if req.ProjectID == "" || req.NodeID == "" {
		response.FailWithMsg(c, errcode.ParamError, "projectId/nodeId cannot be blank")
		return
	}
	list, err := h.featureSvc.ProjectFeatureTableList(c.Request.Context(), req.NodeID, req.ProjectID)
	if err != nil {
		failDataErr(c, err, errcode.SystemError)
		return
	}
	response.OK(c, list)
}

// --- Cloud Log Endpoint (Java CloudLogController) ---

// CloudLogSls handles POST /cloud_log/sls.
// Request: {projectId, graphNodeId?, jobId?, taskId?, queryParties?, nodeId?}
// Response: CloudGraphNodeTaskLogsVO {config, status?, logs?, nodeParties?};
// {config:false} when SLS is not configured.
func (h *DataHandler) CloudLogSls(c *gin.Context) {
	var req service.GraphNodeCloudLogsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMsg(c, errcode.ParamError, err.Error())
		return
	}
	if !h.cloudLog.Configured() {
		response.OK(c, &service.CloudGraphNodeTaskLogsVO{Config: false})
		return
	}
	vo, err := h.cloudLog.FetchLog(c.Request.Context(), &req)
	if err != nil {
		failDataErr(c, err, errcode.ProjectJobCloudLogError)
		return
	}
	response.OK(c, vo)
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
