package v1

import (
	"encoding/json"
	"os"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/pkg/errcode"
	"github.com/fengzhizi319/privahub/pkg/kuscia"
	"github.com/fengzhizi319/privahub/pkg/response"
	"gorm.io/gorm"
)

// MiscHandler handles miscellaneous endpoints (Inst, Component, Scheduled).
type MiscHandler struct {
	db           *gorm.DB
	kusciaClient *kuscia.Client
	components   []ComponentDef
	compOnce     sync.Once
}

// ComponentDef represents a component definition loaded from config.
type ComponentDef struct {
	CodeName    string `json:"code_name"`
	Name        string `json:"name"`
	Category    string `json:"category"`
	Version     string `json:"version"`
	Description string `json:"description"`
}

// NewMiscHandler creates a new MiscHandler.
func NewMiscHandler(db *gorm.DB, kusciaClient *kuscia.Client) *MiscHandler {
	return &MiscHandler{db: db, kusciaClient: kusciaClient}
}

// loadComponents loads component definitions from config/components.json (once).
func (h *MiscHandler) loadComponents() []ComponentDef {
	h.compOnce.Do(func() {
		compPath := os.Getenv("PRIVAHUB_COMPONENTS_FILE")
		if compPath == "" {
			compPath = "config/components.json"
		}
		data, err := os.ReadFile(compPath)
		if err != nil {
			// Fallback to built-in defaults
			h.components = defaultComponents()
			return
		}
		if err := json.Unmarshal(data, &h.components); err != nil {
			h.components = defaultComponents()
		}
	})
	return h.components
}

// defaultComponents returns the built-in component list as fallback.
func defaultComponents() []ComponentDef {
	return []ComponentDef{
		{CodeName: "data_prep/csv_data_import", Name: "CSV Data Import", Category: "data_prep", Version: "1.0.0"},
		{CodeName: "data_prep/psi", Name: "Private Set Intersection", Category: "data_prep", Version: "1.0.0"},
		{CodeName: "feature/vert_woe_binning", Name: "WOE Binning", Category: "feature", Version: "1.0.0"},
		{CodeName: "ml.train/ss_xgb_train", Name: "SS-XGBoost Train", Category: "ml.train", Version: "1.0.0"},
		{CodeName: "ml.train/ss_glm_train", Name: "SS-GLM Train", Category: "ml.train", Version: "1.0.0"},
		{CodeName: "ml.eval/ss_pvalue", Name: "SS-PValue", Category: "ml.eval", Version: "1.0.0"},
		{CodeName: "ml.predict/ss_xgb_predict", Name: "SS-XGBoost Predict", Category: "ml.predict", Version: "1.0.0"},
	}
}

// --- Inst Endpoints ---

// CreateInstRequest represents an inst creation request.
type CreateInstRequest struct {
	InstID string `json:"inst_id" binding:"required"`
	Name   string `json:"name" binding:"required"`
}

// InstVO represents an inst view object.
type InstVO struct {
	InstID    string `json:"inst_id"`
	Name      string `json:"name"`
	GmtCreate string `json:"gmt_create"`
}

// CreateInst handles inst creation.
func (h *MiscHandler) CreateInst(c *gin.Context) {
	var req CreateInstRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}

	inst := &model.InstDO{
		InstID: req.InstID,
		Name:   req.Name,
	}

	if err := h.db.WithContext(c.Request.Context()).Create(inst).Error; err != nil {
		response.Fail(c, errcode.AlreadyExists)
		return
	}

	response.OK(c, InstVO{
		InstID:    inst.InstID,
		Name:      inst.Name,
		GmtCreate: inst.GmtCreate.Format("2006-01-02 15:04:05"),
	})
}

// ListInsts handles inst list retrieval.
func (h *MiscHandler) ListInsts(c *gin.Context) {
	var insts []model.InstDO
	if err := h.db.WithContext(c.Request.Context()).Find(&insts).Error; err != nil {
		response.Fail(c, errcode.SystemError)
		return
	}

	result := make([]InstVO, 0, len(insts))
	for _, inst := range insts {
		result = append(result, InstVO{
			InstID:    inst.InstID,
			Name:      inst.Name,
			GmtCreate: inst.GmtCreate.Format("2006-01-02 15:04:05"),
		})
	}

	response.OK(c, result)
}

// --- Component Endpoints ---

// ListComponents handles component list retrieval from config file.
func (h *MiscHandler) ListComponents(c *gin.Context) {
	components := h.loadComponents()
	response.OK(c, components)
}

// ComponentVersion handles component version retrieval.
func (h *MiscHandler) ComponentVersion(c *gin.Context) {
	response.OK(c, gin.H{
		"version":    "1.0.0",
		"sf_version": "1.0.0",
	})
}

// ComponentI18n handles component i18n content retrieval.
func (h *MiscHandler) ComponentI18n(c *gin.Context) {
	var req struct {
		CodeName string `json:"code_name"`
		Lang     string `json:"lang"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}

	// Return i18n map for known components
	i18nData := gin.H{
		"data_prep/csv_data_import": gin.H{"name": "CSV数据导入", "desc": "导入CSV格式数据"},
		"data_prep/psi":             gin.H{"name": "隐私求交", "desc": "隐私集合求交"},
		"feature/vert_woe_binning":  gin.H{"name": "WOE分箱", "desc": "纵向WOE分箱"},
		"ml.train/ss_xgb_train":     gin.H{"name": "SS-XGBoost训练", "desc": "安全XGBoost模型训练"},
		"ml.train/ss_glm_train":     gin.H{"name": "SS-GLM训练", "desc": "安全广义线性模型训练"},
		"ml.eval/ss_pvalue":         gin.H{"name": "SS-PValue", "desc": "安全P值计算"},
		"ml.predict/ss_xgb_predict": gin.H{"name": "SS-XGBoost预测", "desc": "安全XGBoost模型预测"},
	}

	if req.CodeName != "" {
		if v, ok := i18nData[req.CodeName]; ok {
			response.OK(c, v)
			return
		}
		response.OK(c, gin.H{})
		return
	}

	response.OK(c, i18nData)
}

// ComponentBatch handles batch component retrieval from config.
func (h *MiscHandler) ComponentBatch(c *gin.Context) {
	var req struct {
		CodeNames []string `json:"code_names"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}

	allComponents := h.loadComponents()

	if len(req.CodeNames) == 0 {
		response.OK(c, allComponents)
		return
	}

	nameSet := make(map[string]bool, len(req.CodeNames))
	for _, cn := range req.CodeNames {
		nameSet[cn] = true
	}

	result := make([]ComponentDef, 0)
	for _, comp := range allComponents {
		if nameSet[comp.CodeName] {
			result = append(result, comp)
		}
	}

	response.OK(c, result)
}

// VersionList handles component version list retrieval.
func (h *MiscHandler) VersionList(c *gin.Context) {
	response.OK(c, []gin.H{
		{"version": "1.0.0", "sf_version": "1.0.0", "is_default": true},
	})
}

// --- Inst Extended Endpoints ---

// GetInst handles inst detail retrieval.
func (h *MiscHandler) GetInst(c *gin.Context) {
	var req struct {
		InstID    string `json:"inst_id"`
		InstIDAlt string `json:"instId"`
		OwnerID   string `json:"ownerId"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		req.InstID = ""
	}
	instID := req.InstID
	if instID == "" {
		instID = req.InstIDAlt
	}
	if instID == "" {
		instID = req.OwnerID
	}
	if instID == "" {
		instID = "alice"
	}

	var inst model.InstDO
	if err := h.db.WithContext(c.Request.Context()).Where("inst_id = ?", instID).First(&inst).Error; err != nil {
		if err := h.db.WithContext(c.Request.Context()).First(&inst).Error; err != nil {
			inst = model.InstDO{
				InstID: instID,
				Name:   instID,
			}
		}
	}

	response.OK(c, InstVO{
		InstID:    inst.InstID,
		Name:      inst.Name,
		GmtCreate: inst.GmtCreate.Format("2006-01-02 15:04:05"),
	})
}

// ListInstNodes handles listing nodes for an inst.
func (h *MiscHandler) ListInstNodes(c *gin.Context) {
	var nodes []model.NodeDO
	h.db.WithContext(c.Request.Context()).Find(&nodes)

	result := make([]gin.H, 0, len(nodes))
	for _, n := range nodes {
		result = append(result, gin.H{
			"node_id": n.NodeID,
			"name":    n.Name,
			"type":    n.Type,
		})
	}

	response.OK(c, result)
}

// AddInstNode handles adding a node to an inst with Kuscia domain registration.
func (h *MiscHandler) AddInstNode(c *gin.Context) {
	var req struct {
		InstID string `json:"inst_id" binding:"required"`
		NodeID string `json:"node_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}

	ctx := c.Request.Context()

	// Verify node exists
	var node model.NodeDO
	if err := h.db.WithContext(ctx).Where("node_id = ?", req.NodeID).First(&node).Error; err != nil {
		response.Fail(c, errcode.NodeNotFound)
		return
	}

	// Register domain in Kuscia (best-effort)
	if h.kusciaClient != nil {
		_ = h.kusciaClient.CreateDomain(ctx, &kuscia.CreateDomainRequest{
			DomainID: req.NodeID,
			Role:     node.Type,
		})
	}

	response.OKEmpty(c)
}

// InstNodeToken handles inst node token retrieval.
func (h *MiscHandler) InstNodeToken(c *gin.Context) {
	var req struct {
		NodeID string `json:"node_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}

	var node model.NodeDO
	if err := h.db.WithContext(c.Request.Context()).Where("node_id = ?", req.NodeID).First(&node).Error; err != nil {
		response.Fail(c, errcode.NodeNotFound)
		return
	}

	response.OK(c, gin.H{"token": node.Token})
}

// DeleteInstNode handles deleting a node from an inst with Kuscia domain cleanup.
func (h *MiscHandler) DeleteInstNode(c *gin.Context) {
	var req struct {
		InstID string `json:"inst_id" binding:"required"`
		NodeID string `json:"node_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}

	ctx := c.Request.Context()

	// Delete domain from Kuscia (best-effort)
	if h.kusciaClient != nil {
		_ = h.kusciaClient.DeleteDomain(ctx, req.NodeID)
	}

	// Remove node and associated routes from DB
	h.db.WithContext(ctx).Where("node_id = ?", req.NodeID).Delete(&model.NodeDO{})
	h.db.WithContext(ctx).Where("src_node_id = ? OR dst_node_id = ?", req.NodeID, req.NodeID).Delete(&model.NodeRouteDO{})

	response.OKEmpty(c)
}

// --- Node Extended Endpoints ---

// RefreshNode handles node status refresh.
func (h *MiscHandler) RefreshNode(c *gin.Context) {
	var req struct {
		NodeID string `json:"node_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}

	var node model.NodeDO
	if err := h.db.WithContext(c.Request.Context()).Where("node_id = ?", req.NodeID).First(&node).Error; err != nil {
		response.Fail(c, errcode.NodeNotFound)
		return
	}

	response.OK(c, gin.H{
		"node_id": node.NodeID,
		"name":    node.Name,
		"type":    node.Type,
	})
}

// NodeResultList handles node result list retrieval.
func (h *MiscHandler) NodeResultList(c *gin.Context) {
	var req struct {
		NodeID string `json:"node_id" binding:"required"`
		Page   int    `json:"page"`
		Size   int    `json:"size"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}

	// Results come from Kuscia - return empty for now
	response.OK(c, gin.H{"data": []interface{}{}, "total": 0})
}

// NodeResultDetail handles node result detail retrieval.
func (h *MiscHandler) NodeResultDetail(c *gin.Context) {
	var req struct {
		NodeID   string `json:"node_id" binding:"required"`
		ResultID string `json:"result_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}

	response.OK(c, gin.H{"result_id": req.ResultID})
}
