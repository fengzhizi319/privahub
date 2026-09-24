package v1

import (
	"github.com/fengzhizi319/privahub/internal/service"
	"github.com/fengzhizi319/privahub/pkg/errcode"
	"github.com/fengzhizi319/privahub/pkg/kuscia"
	"github.com/fengzhizi319/privahub/pkg/response"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// MiscHandler serves the component endpoints (Java GraphController component
// part + ComponentVersionController). Inst endpoints moved to InstHandler.
type MiscHandler struct {
	db           *gorm.DB
	kusciaClient *kuscia.Client
	components   *service.ComponentService
}

// ComponentHandler is the descriptive name of MiscHandler.
type ComponentHandler = MiscHandler

// NewMiscHandler creates a new MiscHandler backed by config/components + config/i18n.
func NewMiscHandler(db *gorm.DB, kusciaClient *kuscia.Client) *MiscHandler {
	return &MiscHandler{db: db, kusciaClient: kusciaClient, components: service.NewComponentService()}
}

// WithComponentService overrides the component service (tests).
func (h *MiscHandler) WithComponentService(s *service.ComponentService) *MiscHandler {
	h.components = s
	return h
}

// ListComponents returns Map<app, CompListVO> (Java listComponents).
func (h *MiscHandler) ListComponents(c *gin.Context) {
	resp, err := h.components.ListComponents()
	if err != nil {
		failService(c, err, errcode.GraphComponentNotExists)
		return
	}
	response.OK(c, resp)
}

// ComponentVersion is a Go-only helper returning the configured image versions.
func (h *MiscHandler) ComponentVersion(c *gin.Context) {
	response.OK(c, h.components.VersionList())
}

// ComponentI18n returns Map<app, Map<text, translation>> (Java listComponentI18n).
func (h *MiscHandler) ComponentI18n(c *gin.Context) {
	resp, err := h.components.ListComponentI18n()
	if err != nil {
		failService(c, err, errcode.GraphComponent18nError)
		return
	}
	response.OK(c, resp)
}

// ComponentBatch returns the ComponentDef list in request order (Java
// batchGetComponent); the body is [{app, domain, name, version}].
func (h *MiscHandler) ComponentBatch(c *gin.Context) {
	var keys []service.ComponentKey
	if c.Request.ContentLength != 0 {
		if err := c.ShouldBindJSON(&keys); err != nil {
			response.FailWithMsg(c, errcode.ParamError, err.Error())
			return
		}
	}
	defs, err := h.components.BatchGetComponent(keys)
	if err != nil {
		failService(c, err, errcode.GraphComponentNotExists)
		return
	}
	response.OK(c, defs)
}

// VersionList returns the image versions of the deploy mode (Java listVersion).
func (h *MiscHandler) VersionList(c *gin.Context) {
	response.OK(c, h.components.VersionList())
}
