package v1

import (
	"encoding/json"
	"io"
	"os"
	"strings"
	"sync"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/pkg/errcode"
	"github.com/fengzhizi319/privahub/pkg/kuscia"
	"github.com/fengzhizi319/privahub/pkg/response"
	"github.com/gin-gonic/gin"
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
// Enhanced to include full attrs/inputs/outputs schema (P1: corresponding to Java ComponentServiceImpl).
type ComponentDef struct {
	CodeName    string            `json:"code_name"`
	Domain      string            `json:"domain,omitempty"`
	Name        string            `json:"name"`
	Category    string            `json:"category"`
	Version     string            `json:"version"`
	Description string            `json:"description"`
	Desc        string            `json:"desc,omitempty"`
	Attrs       []json.RawMessage `json:"attrs,omitempty"`
	Inputs      []json.RawMessage `json:"inputs,omitempty"`
	Outputs     []json.RawMessage `json:"outputs,omitempty"`
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
	// Frontend sends a bare array of {domain, name, version?, app?}; codeName is domain/name.
	var requests []struct {
		Domain  string `json:"domain"`
		Name    string `json:"name"`
		Version string `json:"version"`
		App     string `json:"app"`
	}
	codeNames := make([]string, 0)
	if err := c.ShouldBindJSON(&requests); err == nil {
		for _, r := range requests {
			switch {
			case r.Domain != "" && r.Name != "":
				codeNames = append(codeNames, r.Domain+"/"+r.Name)
			case r.Name != "":
				codeNames = append(codeNames, r.Name)
			}
		}
	}

	allComponents := h.loadComponents()

	// Frontend expects a map keyed by codeName (Record<string, ComponentDef>).
	result := make(map[string]ComponentDef)
	if len(codeNames) == 0 {
		for _, comp := range allComponents {
			result[comp.CodeName] = comp
		}
		response.OK(c, result)
		return
	}

	nameSet := make(map[string]bool, len(codeNames))
	for _, cn := range codeNames {
		nameSet[cn] = true
	}
	for _, comp := range allComponents {
		if nameSet[comp.CodeName] {
			result[comp.CodeName] = comp
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
	// Bug47 fix: check the DB error instead of silently ignoring it.
	if err := h.db.WithContext(c.Request.Context()).Find(&nodes).Error; err != nil {
		response.Fail(c, errcode.SystemError)
		return
	}

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

// RegisterInstNode handles institution node registration via multipart form (frontend contract).
func (h *MiscHandler) RegisterInstNode(c *gin.Context) {
	jsonData := c.Query("json_data")

	var payload struct {
		NodeID     string `json:"nodeId"`
		NodeName   string `json:"nodeName"`
		InstID     string `json:"instId"`
		InstName   string `json:"instName"`
		NetAddress string `json:"netAddress"`
		CertText   string `json:"certText"`
		Token      string `json:"token"`
		Mode       int    `json:"mode"`
	}
	if jsonData != "" {
		if err := json.Unmarshal([]byte(jsonData), &payload); err != nil {
			response.Fail(c, errcode.ParamError)
			return
		}
	}

	// Optional multipart file uploads (cert / key / token) — best-effort.
	// Limit reads to 1 MB to prevent resource exhaustion.
	const maxFileSize = 1 << 20 // 1 MB
	certText := payload.CertText
	if f, err := c.FormFile("certFile"); err == nil {
		if fh, err := f.Open(); err == nil {
			if b, err := io.ReadAll(io.LimitReader(fh, maxFileSize)); err == nil {
				certText = string(b)
			}
			_ = fh.Close()
		}
	}
	token := payload.Token
	if f, err := c.FormFile("token"); err == nil {
		if fh, err := f.Open(); err == nil {
			if b, err := io.ReadAll(io.LimitReader(fh, maxFileSize)); err == nil {
				token = strings.TrimSpace(string(b))
			}
			_ = fh.Close()
		}
	}

	nodeID := payload.NodeID
	if nodeID == "" {
		response.Fail(c, errcode.ParamError)
		return
	}

	ctx := c.Request.Context()

	// Upsert the node record.
	var node model.NodeDO
	if err := h.db.WithContext(ctx).Where("node_id = ?", nodeID).First(&node).Error; err != nil {
		name := payload.NodeName
		if name == "" {
			name = nodeID
		}
		newNode := &model.NodeDO{
			NodeID:     nodeID,
			Name:       name,
			Auth:       certText,
			Token:      token,
			NetAddress: payload.NetAddress,
			Type:       "lite",
			Mode:       payload.Mode,
		}
		if err := h.db.WithContext(ctx).Create(newNode).Error; err != nil {
			response.Fail(c, errcode.SystemError)
			return
		}
	} else {
		updates := map[string]interface{}{}
		if certText != "" {
			updates["auth"] = certText
		}
		if token != "" {
			updates["token"] = token
		}
		if payload.NetAddress != "" {
			updates["net_address"] = payload.NetAddress
		}
		if len(updates) > 0 {
			// Bug49 fix: propagate the update error instead of silently ignoring it.
			if err := h.db.WithContext(ctx).Model(&node).Updates(updates).Error; err != nil {
				response.Fail(c, errcode.SystemError)
				return
			}
		}
	}

	// Register domain in Kuscia (best-effort).
	if h.kusciaClient != nil {
		_ = h.kusciaClient.CreateDomain(ctx, &kuscia.CreateDomainRequest{
			DomainID: nodeID,
			Cert:     certText,
		})
	}

	response.OKEmpty(c)
}

// InstNodeToken handles inst node token retrieval.
func (h *MiscHandler) InstNodeToken(c *gin.Context) {
	var req struct {
		NodeID    string `json:"node_id"`
		NodeIDAlt string `json:"nodeId"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}
	if req.NodeID == "" {
		req.NodeID = req.NodeIDAlt
	}
	if req.NodeID == "" {
		response.Fail(c, errcode.ParamError)
		return
	}

	var node model.NodeDO
	if err := h.db.WithContext(c.Request.Context()).Where("node_id = ?", req.NodeID).First(&node).Error; err != nil {
		response.Fail(c, errcode.NodeNotFound)
		return
	}

	response.OK(c, gin.H{
		"node_id":          node.NodeID,
		"node_name":        node.Name,
		"inst_token":       node.Token,
		"inst_token_state": "Available",
	})
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

	// Remove node and associated routes from DB atomically
	if err := h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("node_id = ?", req.NodeID).Delete(&model.NodeDO{}).Error; err != nil {
			return err
		}
		return tx.Where("src_node_id = ? OR dst_node_id = ?", req.NodeID, req.NodeID).Delete(&model.NodeRouteDO{}).Error
	}); err != nil {
		response.Fail(c, errcode.SystemError)
		return
	}

	response.OKEmpty(c)
}

// --- Node Extended Endpoints ---

// RefreshNode handles node status refresh.
func (h *MiscHandler) RefreshNode(c *gin.Context) {
	var req struct {
		NodeID    string `json:"node_id"`
		NodeIDAlt string `json:"nodeId"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}
	if req.NodeID == "" {
		req.NodeID = req.NodeIDAlt
	}
	if req.NodeID == "" {
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
