package v1

import (
	"github.com/fengzhizi319/privahub/internal/dao/repository"
	"github.com/fengzhizi319/privahub/internal/service"
	"github.com/fengzhizi319/privahub/pkg/errcode"
	"github.com/fengzhizi319/privahub/pkg/kuscia"
	"github.com/fengzhizi319/privahub/pkg/response"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// NodeRouteHandler handles nodeRoute/* (Java NodeRouteController).
type NodeRouteHandler struct {
	svc *service.NodeRouteService
}

// NewNodeRouteHandler creates a handler with a standalone NodeService (tests / legacy wiring).
func NewNodeRouteHandler(db *gorm.DB, kusciaClient *kuscia.Client) *NodeRouteHandler {
	ns := service.NewNodeService(repository.NewNodeRepo(db), repository.NewNodeRouteRepo(db), kusciaClient, db)
	return &NodeRouteHandler{svc: ns.Routes()}
}

// NewNodeRouteHandlerWithService creates a handler bound to the shared NodeService.
func NewNodeRouteHandlerWithService(ns *service.NodeService) *NodeRouteHandler {
	return &NodeRouteHandler{svc: ns.Routes()}
}

// Page handles nodeRoute/page: {list, data, total, page, size}.
func (h *NodeRouteHandler) Page(c *gin.Context) {
	var req service.PageNodeRouteRequest
	_ = c.ShouldBindJSON(&req)
	page, err := h.svc.Page(nodeReqCtx(c), &req)
	if err != nil {
		failNodeErr(c, err)
		return
	}
	response.OK(c, page)
}

// Get handles nodeRoute/get.
func (h *NodeRouteHandler) Get(c *gin.Context) {
	var req routerIDBody
	if err := c.ShouldBindJSON(&req); err != nil || req.id() == "" {
		response.Fail(c, errcode.ParamError)
		return
	}
	vo, err := h.svc.Get(nodeReqCtx(c), req.id())
	if err != nil {
		failNodeErr(c, err)
		return
	}
	response.OK(c, vo)
}

// Update handles nodeRoute/update (rebuilds the Kuscia DomainRoute).
func (h *NodeRouteHandler) Update(c *gin.Context) {
	var req service.UpdateNodeRouterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}
	if err := h.svc.Update(nodeReqCtx(c), &req); err != nil {
		failNodeErr(c, err)
		return
	}
	response.OK(c, firstNonEmptyStr(req.RouterID, req.RouterIDAlt))
}

// ListNode handles nodeRoute/listNode: full NodeVO list.
func (h *NodeRouteHandler) ListNode(c *gin.Context) {
	nodes, err := h.svc.ListNode(nodeReqCtx(c))
	if err != nil {
		failNodeErr(c, err)
		return
	}
	response.OK(c, nodes)
}

// Refresh handles nodeRoute/refresh (Kuscia route status {status, reason}).
func (h *NodeRouteHandler) Refresh(c *gin.Context) {
	var req routerIDBody
	if err := c.ShouldBindJSON(&req); err != nil || req.id() == "" {
		response.Fail(c, errcode.ParamError)
		return
	}
	vo, err := h.svc.Refresh(nodeReqCtx(c), req.id())
	if err != nil {
		failNodeErr(c, err)
		return
	}
	response.OK(c, vo)
}

// Delete handles nodeRoute/delete.
func (h *NodeRouteHandler) Delete(c *gin.Context) {
	var req routerIDBody
	if err := c.ShouldBindJSON(&req); err != nil || req.id() == "" {
		response.Fail(c, errcode.ParamError)
		return
	}
	if err := h.svc.Delete(nodeReqCtx(c), req.id()); err != nil {
		failNodeErr(c, err)
		return
	}
	response.OKEmpty(c)
}
