package v1

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/pkg/errcode"
	"github.com/fengzhizi319/privahub/pkg/kuscia"
	"github.com/fengzhizi319/privahub/pkg/response"
	"gorm.io/gorm"
)

// P2PHandler handles P2P mode HTTP requests with real DB + Kuscia integration.
type P2PHandler struct {
	db           *gorm.DB
	kusciaClient *kuscia.Client
}

// NewP2PHandler creates a new P2PHandler.
func NewP2PHandler(db *gorm.DB, kusciaClient *kuscia.Client) *P2PHandler {
	return &P2PHandler{db: db, kusciaClient: kusciaClient}
}

// ProjectCreate handles P2P project creation.
func (h *P2PHandler) ProjectCreate(c *gin.Context) {
	var req struct {
		Name        string   `json:"name" binding:"required"`
		Description string   `json:"description"`
		ComputeMode string   `json:"compute_mode"`
		NodeIDs     []string `json:"node_ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}

	projectID := uuid.New().String()[:8]
	computeMode := req.ComputeMode
	if computeMode == "" {
		computeMode = "p2p"
	}

	project := &model.ProjectDO{
		ProjectID:   projectID,
		Name:        req.Name,
		Description: req.Description,
		ComputeMode: computeMode,
		ComputeFunc: "ALL",
		OwnerID:     "admin",
	}

	ctx := c.Request.Context()
	if err := h.db.WithContext(ctx).Create(project).Error; err != nil {
		response.Fail(c, errcode.SystemError)
		return
	}

	// Associate nodes with the project
	for _, nodeID := range req.NodeIDs {
		pn := &model.ProjectNodeDO{ProjectID: projectID, NodeID: nodeID}
		_ = h.db.WithContext(ctx).Create(pn).Error
	}

	response.OK(c, gin.H{"project_id": projectID})
}

// ProjectList handles P2P project list.
func (h *P2PHandler) ProjectList(c *gin.Context) {
	result, err := h.buildP2pProjectList(c.Request.Context())
	if err != nil {
		response.Fail(c, errcode.SystemError)
		return
	}

	response.OK(c, result)
}

// buildP2pProjectList queries P2P projects and builds the frontend-facing bare array.
func (h *P2PHandler) buildP2pProjectList(ctx context.Context) ([]gin.H, error) {
	var projects []model.ProjectDO
	if err := h.db.WithContext(ctx).
		Where("compute_mode = ?", "p2p").
		Order("gmt_create DESC").Find(&projects).Error; err != nil {
		return nil, err
	}

	result := make([]gin.H, 0, len(projects))
	for _, p := range projects {
		result = append(result, gin.H{
			"project_id":   p.ProjectID,
			"name":         p.Name,
			"description":  p.Description,
			"compute_mode": p.ComputeMode,
			"status":       p.Status,
			"gmt_create":   p.GmtCreate.Format("2006-01-02 15:04:05"),
		})
	}
	return result, nil
}

// ProjectUpdate handles P2P project update.
func (h *P2PHandler) ProjectUpdate(c *gin.Context) {
	var req struct {
		ProjectID   string `json:"project_id" binding:"required"`
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}

	ctx := c.Request.Context()
	updates := map[string]interface{}{}
	if req.Name != "" {
		updates["name"] = req.Name
	}
	if req.Description != "" {
		updates["description"] = req.Description
	}

	if len(updates) > 0 {
		if err := h.db.WithContext(ctx).Model(&model.ProjectDO{}).
			Where("project_id = ?", req.ProjectID).Updates(updates).Error; err != nil {
			response.Fail(c, errcode.SystemError)
			return
		}
	}

	response.OKEmpty(c)
}

// ProjectArchive handles P2P project archive.
func (h *P2PHandler) ProjectArchive(c *gin.Context) {
	var req struct {
		ProjectID string `json:"project_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}

	if err := h.db.WithContext(c.Request.Context()).Model(&model.ProjectDO{}).
		Where("project_id = ?", req.ProjectID).Update("status", 1).Error; err != nil {
		response.Fail(c, errcode.SystemError)
		return
	}

	// Frontend validates the response as a bare ProjectVO array (z.array); return the refreshed list.
	result, err := h.buildP2pProjectList(c.Request.Context())
	if err != nil {
		response.Fail(c, errcode.SystemError)
		return
	}

	response.OK(c, result)
}

// ProjectParticipants handles P2P project participants retrieval.
func (h *P2PHandler) ProjectParticipants(c *gin.Context) {
	var req struct {
		ProjectID string `json:"project_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}

	ctx := c.Request.Context()
	var projectNodes []model.ProjectNodeDO
	h.db.WithContext(ctx).Where("project_id = ?", req.ProjectID).Find(&projectNodes)

	participants := make([]gin.H, 0, len(projectNodes))
	for _, pn := range projectNodes {
		var node model.NodeDO
		if err := h.db.WithContext(ctx).Where("node_id = ?", pn.NodeID).First(&node).Error; err == nil {
			participants = append(participants, gin.H{
				"node_id":     node.NodeID,
				"node_name":   node.Name,
				"net_address": node.NetAddress,
			})
		} else {
			participants = append(participants, gin.H{"node_id": pn.NodeID})
		}
	}

	response.OK(c, gin.H{"participants": participants})
}

// NodeCreate handles P2P node creation with Kuscia domain registration.
func (h *P2PHandler) NodeCreate(c *gin.Context) {
	var req struct {
		NodeID  string `json:"node_id" binding:"required"`
		Name    string `json:"name" binding:"required"`
		Address string `json:"address"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}

	ctx := c.Request.Context()
	node := &model.NodeDO{
		NodeID:        req.NodeID,
		Name:          req.Name,
		NetAddress:    req.Address,
		ControlNodeID: req.NodeID,
		Type:          "normal",
		MasterNodeID:  "master",
	}

	if err := h.db.WithContext(ctx).Create(node).Error; err != nil {
		response.Fail(c, errcode.AlreadyExists)
		return
	}

	// Register domain in Kuscia (best-effort)
	if h.kusciaClient != nil {
		_ = h.kusciaClient.CreateDomain(ctx, &kuscia.CreateDomainRequest{
			DomainID: req.NodeID,
			Role:     "normal",
		})
	}

	response.OK(c, gin.H{"node_id": req.NodeID})
}

// NodeDelete handles P2P node deletion with Kuscia domain cleanup.
func (h *P2PHandler) NodeDelete(c *gin.Context) {
	var req struct {
		NodeID string `json:"node_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}

	ctx := c.Request.Context()

	// Delete from Kuscia (best-effort)
	if h.kusciaClient != nil {
		_ = h.kusciaClient.DeleteDomain(ctx, req.NodeID)
	}

	// Delete node and associated routes from DB
	h.db.WithContext(ctx).Where("node_id = ?", req.NodeID).Delete(&model.NodeDO{})
	h.db.WithContext(ctx).Where("src_node_id = ? OR dst_node_id = ?", req.NodeID, req.NodeID).Delete(&model.NodeRouteDO{})

	response.OKEmpty(c)
}

// DataSync handles P2P data synchronization from remote nodes.
func (h *P2PHandler) DataSync(c *gin.Context) {
	var req struct {
		TableName string `json:"table_name"`
		Data      string `json:"data"`
		Action    string `json:"action"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}

	// Log the sync event
	syncLog := &model.EdgeDataSyncLogDO{
		SyncTableName:  req.TableName,
		LastUpdateTime: "now",
	}
	_ = h.db.WithContext(c.Request.Context()).Save(syncLog).Error

	response.OKEmpty(c)
}
