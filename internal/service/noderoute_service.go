package service

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/internal/graphjob"
	"github.com/fengzhizi319/privahub/pkg/auth"
	"github.com/fengzhizi319/privahub/pkg/errcode"
	"github.com/fengzhizi319/privahub/pkg/kuscia"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// Route types (Java DomainRouterConstants.DomainRouterTypeEnum).
const (
	RouteTypeFullDuplex = "FullDuplex"
	RouteTypeHalfDuplex = "HalfDuplex"
)

// defaultRouteIDs are protected from deletion (Java NodeRouteManager.deleteNodeRoute).
var defaultRouteIDs = map[string]bool{"1": true, "2": true, "3": true, "4": true, "5": true, "6": true}

// RouteIDOf returns the Java route_id convention "src__dst".
func RouteIDOf(src, dst string) string { return src + "__" + dst }

// NodeRouteService implements Java NodeRouterServiceImpl + NodeRouteManager.
// It shares dependencies with NodeService (see NodeService.Routes()).
type NodeRouteService struct {
	ns *NodeService
}

// --- DTOs ---

// PageNodeRouteRequest is the nodeRoute/page request.
type PageNodeRouteRequest struct {
	Search     string `json:"search"`
	OwnerID    string `json:"ownerId"`
	Page       int    `json:"page"`
	Size       int    `json:"size"`
	PageNumber int    `json:"pageNumber"`
	PageSize   int    `json:"pageSize"`
	SrcNodeID  string `json:"srcNodeId"`
	DstNodeID  string `json:"dstNodeId"`
	// Go legacy filter: routes where node_id is src or dst.
	NodeID string `json:"node_id"`
}

// UpdateNodeRouterRequest is the nodeRoute/update request.
type UpdateNodeRouterRequest struct {
	RouterID         string `json:"routerId"`
	RouterIDAlt      string `json:"router_id"`
	SrcNetAddress    string `json:"srcNetAddress"`
	DstNetAddress    string `json:"dstNetAddress"`
	DstNetAddressAlt string `json:"dst_net_address"`
}

// NodeRouterVO is the Java NodeRouterVO.
type NodeRouterVO struct {
	RouteID             string  `json:"routeId"`
	SrcNodeID           string  `json:"srcNodeId"`
	DstNodeID           string  `json:"dstNodeId"`
	SrcNode             *NodeVO `json:"srcNode,omitempty"`
	DstNode             *NodeVO `json:"dstNode,omitempty"`
	SrcNetAddress       string  `json:"srcNetAddress"`
	DstNetAddress       string  `json:"dstNetAddress"`
	Status              string  `json:"status"`
	StatusReason        string  `json:"statusReason,omitempty"`
	GmtCreate           string  `json:"gmtCreate"`
	GmtModified         string  `json:"gmtModified"`
	IsProjectJobRunning bool    `json:"isProjectJobRunning"`
	RouteType           string  `json:"routeType,omitempty"`
}

// NodeRouterPageVO is the paged route list. `list` is the Java field, `data`
// is kept for the legacy Go contract.
type NodeRouterPageVO struct {
	List  []NodeRouterVO `json:"list"`
	Data  []NodeRouterVO `json:"data"`
	Total int64          `json:"total"`
	Page  int            `json:"page"`
	Size  int            `json:"size"`
}

// --- helpers ---

func (r *NodeRouteService) db() *gorm.DB { return r.ns.db }

func (r *NodeRouteService) findRoute(ctx context.Context, routeID string) (*model.NodeRouteDO, error) {
	var route model.NodeRouteDO
	if err := r.db().WithContext(ctx).Where("route_id = ?", routeID).First(&route).Error; err != nil {
		return nil, &BizError{Code: errcode.NodeRouteNotExistError, Msg: "route not exist " + routeID}
	}
	return &route, nil
}

func (r *NodeRouteService) findNode(ctx context.Context, nodeID string) *model.NodeDO {
	n, err := r.ns.nodeRepo.FindByNodeID(ctx, nodeID)
	if err != nil {
		return nil
	}
	return n
}

// kusciaSrcDst returns the Kuscia (src,dst) and channel for a DB route: in
// AUTONOMY Java stores the route reversed and queries through the src channel.
func (r *NodeRouteService) kusciaSrcDst(route *model.NodeRouteDO) (src, dst, channel string) {
	if r.ns.isAutonomy() {
		return route.DstNodeID, route.SrcNodeID, route.DstNodeID
	}
	return route.SrcNodeID, route.DstNodeID, ""
}

// checkDataPermissions: an EDGE platform user can only touch routes whose dst is itself.
func (r *NodeRouteService) checkDataPermissions(ctx context.Context, nodeID string) error {
	u, ok := auth.UserFromContext(ctx)
	if !ok || u == nil {
		return nil
	}
	if strings.EqualFold(u.PlatformType, auth.PlatformTypeEdge) && u.OwnerID != nodeID {
		return bizErr(errcode.AuthFailed, "no Permissions")
	}
	return nil
}

// validateNoRunningJobs rejects the operation when a project shared by src and
// dst has a RUNNING job (Java NodeRouterServiceImpl.validateNoRunningJobs).
func (r *NodeRouteService) validateNoRunningJobs(ctx context.Context, src, dst string) error {
	running, err := r.projectJobRunning(ctx, src, dst)
	if err != nil {
		return err
	}
	if running {
		return bizErr(errcode.NodeRouteUpdateError, "cannot update node route while a job is running.")
	}
	return nil
}

// sharedProjectIDs returns the projects src and dst both take part in:
// project_node co-membership plus approval configs listing both parties
// (Java validateNoRunningJobs: projectApprovalConfigRepository.findByInitiator).
func (r *NodeRouteService) sharedProjectIDs(ctx context.Context, src, dst string) ([]string, error) {
	db := r.db().WithContext(ctx)
	projectIDs := map[string]bool{}
	var shared []string
	if err := db.Model(&model.ProjectNodeDO{}).Select("project_id").
		Where("node_id = ? AND project_id IN (?)", src,
			r.db().Model(&model.ProjectNodeDO{}).Select("project_id").Where("node_id = ?", dst)).
		Find(&shared).Error; err != nil {
		return nil, err
	}
	for _, p := range shared {
		projectIDs[p] = true
	}
	var configs []model.ProjectApprovalConfigDO
	if err := db.Where("parties LIKE ? AND parties LIKE ?", "%"+src+"%", "%"+dst+"%").Find(&configs).Error; err == nil {
		for _, c := range configs {
			if c.ProjectID != "" {
				projectIDs[c.ProjectID] = true
			}
		}
	}
	ids := make([]string, 0, len(projectIDs))
	for k := range projectIDs {
		ids = append(ids, k)
	}
	sort.Strings(ids)
	return ids, nil
}

// projectJobRunning reports whether a project shared by src and dst has a
// RUNNING project_job. It backs both
// NodeRouterVO.isProjectJobRunning (page/get) and validateNoRunningJobs
// (update/delete), so the UI flag matches what update actually enforces.
func (r *NodeRouteService) projectJobRunning(ctx context.Context, src, dst string) (bool, error) {
	ids, err := r.sharedProjectIDs(ctx, src, dst)
	if err != nil || len(ids) == 0 {
		return false, err
	}
	db := r.db().WithContext(ctx)
	var running int64
	if err := db.Model(&model.ProjectJobDO{}).Where("project_id IN ? AND status = ?", ids, graphjob.JobRunning).
		Count(&running).Error; err != nil {
		return false, err
	}
	return running > 0, nil
}

// createKusciaRoute creates src->dst in Kuscia (deleting an existing one
// first), endpoint from dstNetAddress, Token auth + RSA-GEN (Java createNodeRouteNotInDb).
func (r *NodeRouteService) createKusciaRoute(ctx context.Context, src, dst, dstNetAddress, channel string) error {
	c := r.ns.kc(channel)
	if c == nil {
		return nil
	}
	req, err := kuscia.NewTokenRouteRequest(src, dst, dstNetAddress)
	if err != nil {
		return bizWrap(errcode.NodeRouteCreateError, err, "address format error")
	}
	exists, err := c.DomainRouteExists(ctx, src, dst)
	if err != nil {
		return bizWrap(errcode.NodeRouteCreateError, err, "")
	}
	if exists {
		if err := c.DeleteDomainRoute(ctx, src, dst); err != nil {
			return bizWrap(errcode.NodeRouteCreateError, err, "")
		}
	}
	if err := c.CreateDomainRoute(ctx, req); err != nil {
		return bizWrap(errcode.NodeRouteCreateError, err, "create node router failed.")
	}
	return nil
}

// routeStatus queries Kuscia route status; ok=false when Kuscia returned an error.
func (r *NodeRouteService) routeStatus(ctx context.Context, route *model.NodeRouteDO) (kuscia.RouteStatus, bool) {
	src, dst, channel := r.kusciaSrcDst(route)
	c := r.ns.kc(channel)
	if c == nil {
		return kuscia.RouteStatus{}, false
	}
	resp, err := c.QueryDomainRoute(ctx, src, dst)
	if err != nil {
		return kuscia.RouteStatus{}, false
	}
	return resp.Data.Status, true
}

func (r *NodeRouteService) nodeVO(ctx context.Context, nodeID string) *NodeVO {
	vo, err := r.ns.GetNode(ctx, nodeID)
	if err != nil {
		return nil
	}
	return vo
}

func (r *NodeRouteService) toVO(route *model.NodeRouteDO) NodeRouterVO {
	return NodeRouterVO{
		RouteID:       route.RouteID,
		SrcNodeID:     route.SrcNodeID,
		DstNodeID:     route.DstNodeID,
		SrcNetAddress: route.SrcNetAddress,
		DstNetAddress: route.DstNetAddress,
		GmtCreate:     fmtTime(route.GmtCreate),
		GmtModified:   fmtTime(route.GmtModified),
		RouteType:     "DomainRoute",
	}
}

func (r *NodeRouteService) fillStatus(ctx context.Context, route *model.NodeRouteDO, vo *NodeRouterVO) {
	if st, ok := r.routeStatus(ctx, route); ok {
		vo.Status = st.Status
		vo.StatusReason = st.Reason
	} else if r.ns.hasKuscia() {
		vo.Status = kuscia.RouteStatusFailed
	}
}

// --- API ---

// Page lists routes (Java queryPage) with src/dst NodeVO, Kuscia status and
// isProjectJobRunning.
func (r *NodeRouteService) Page(ctx context.Context, req *PageNodeRouteRequest) (*NodeRouterPageVO, error) {
	page, size := normalizePage(firstPositive(req.Page, req.PageNumber), firstPositive(req.Size, req.PageSize))
	q := r.db().WithContext(ctx).Model(&model.NodeRouteDO{})
	if r.ns.isAutonomy() {
		u, _ := auth.UserFromContext(ctx)
		instID := ""
		if u != nil {
			instID = u.OwnerID
		}
		q = q.Where("dst_node_id IN (?)", r.db().Model(&model.NodeDO{}).Select("node_id").Where("inst_id = ?", instID))
	} else if req.OwnerID != "" {
		q = q.Where("dst_node_id = ?", req.OwnerID)
	}
	if req.NodeID != "" {
		q = q.Where("src_node_id = ? OR dst_node_id = ?", req.NodeID, req.NodeID)
	}
	if req.SrcNodeID != "" {
		q = q.Where("src_node_id = ?", req.SrcNodeID)
	}
	if req.DstNodeID != "" {
		q = q.Where("dst_node_id = ?", req.DstNodeID)
	}
	if kw := strings.TrimSpace(req.Search); kw != "" {
		like := "%" + kw + "%"
		q = q.Where("src_node_id LIKE ? OR dst_net_address LIKE ? OR src_node_id IN (?)", like, like,
			r.db().Model(&model.NodeDO{}).Select("node_id").Where("name LIKE ?", like))
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, err
	}
	var routes []model.NodeRouteDO
	if err := q.Order("gmt_create DESC").Offset((page - 1) * size).Limit(size).Find(&routes).Error; err != nil {
		return nil, err
	}

	list := make([]NodeRouterVO, 0, len(routes))
	for i := range routes {
		vo := r.toVO(&routes[i])
		vo.SrcNode = r.nodeVO(ctx, routes[i].SrcNodeID)
		vo.DstNode = r.nodeVO(ctx, routes[i].DstNodeID)
		r.fillStatus(ctx, &routes[i], &vo)
		vo.IsProjectJobRunning, _ = r.projectJobRunning(ctx, routes[i].SrcNodeID, routes[i].DstNodeID)
		list = append(list, vo)
	}
	return &NodeRouterPageVO{List: list, Data: list, Total: total, Page: page, Size: size}, nil
}

// Get returns one route (Java getNodeRouter).
func (r *NodeRouteService) Get(ctx context.Context, routeID string) (*NodeRouterVO, error) {
	route, err := r.findRoute(ctx, routeID)
	if err != nil {
		return nil, err
	}
	if err := r.checkDataPermissions(ctx, route.DstNodeID); err != nil {
		return nil, err
	}
	vo := r.toVO(route)
	vo.SrcNode = r.nodeVO(ctx, route.SrcNodeID)
	vo.DstNode = r.nodeVO(ctx, route.DstNodeID)
	r.fillStatus(ctx, route, &vo)
	vo.IsProjectJobRunning, _ = r.projectJobRunning(ctx, route.SrcNodeID, route.DstNodeID)
	return &vo, nil
}

// Refresh re-queries Kuscia route status (Java refreshRouter).
func (r *NodeRouteService) Refresh(ctx context.Context, routeID string) (*NodeRouterVO, error) {
	return r.Get(ctx, routeID)
}

// Update changes the route net address and rebuilds the Kuscia DomainRoute
// (Java updateNodeRouter: no running jobs; delete+create HalfDuplex; DB upsert).
func (r *NodeRouteService) Update(ctx context.Context, req *UpdateNodeRouterRequest) error {
	routeID := firstNonEmpty(req.RouterID, req.RouterIDAlt)
	if routeID == "" {
		return bizErr(errcode.ParamError, "routerId is required")
	}
	route, err := r.findRoute(ctx, routeID)
	if err != nil {
		return err
	}
	if err := r.validateNoRunningJobs(ctx, route.SrcNodeID, route.DstNodeID); err != nil {
		return err
	}
	if err := r.checkDataPermissions(ctx, route.DstNodeID); err != nil {
		return err
	}
	srcAddr, dstAddr := route.SrcNetAddress, route.DstNetAddress
	if n := r.findNode(ctx, route.SrcNodeID); n != nil && srcAddr == "" {
		srcAddr = n.NetAddress
	}
	if n := r.findNode(ctx, route.DstNodeID); n != nil && dstAddr == "" {
		dstAddr = n.NetAddress
	}
	if v := firstNonEmpty(req.DstNetAddress, req.DstNetAddressAlt); v != "" {
		dstAddr = v
	}
	if r.ns.isAutonomy() && strings.TrimSpace(req.SrcNetAddress) != "" {
		srcAddr = strings.TrimSpace(req.SrcNetAddress)
	}

	return r.db().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.NodeRouteDO{}).Where("id = ?", route.ID).Updates(map[string]interface{}{
			"src_net_address": srcAddr,
			"dst_net_address": dstAddr,
			"gmt_modified":    time.Now(),
		}).Error; err != nil {
			return err
		}
		if r.ns.isAutonomy() {
			// Java: updateNodeRoute(route, dstNode, srcNode) → Kuscia dst->src via the dst channel.
			return r.createKusciaRoute(ctx, route.DstNodeID, route.SrcNodeID, srcAddr, route.DstNodeID)
		}
		return r.createKusciaRoute(ctx, route.SrcNodeID, route.DstNodeID, dstAddr, "")
	})
}

// ListNode returns full NodeVOs (Java nodeRoute/listNode → nodeService.listNodes()).
func (r *NodeRouteService) ListNode(ctx context.Context) ([]NodeVO, error) {
	return r.ns.ListNodes(ctx)
}

// Delete removes one route (Java deleteNodeRouter): default ids are
// protected; no running jobs; Kuscia delete (11404 = already deleted); DB delete.
func (r *NodeRouteService) Delete(ctx context.Context, routeID string) error {
	if defaultRouteIDs[routeID] {
		return bizErr(errcode.SystemValidationError, "default route can not delete")
	}
	route, err := r.findRoute(ctx, routeID)
	if err != nil {
		return err
	}
	if err := r.checkDataPermissions(ctx, route.DstNodeID); err != nil {
		return err
	}
	if err := r.validateNoRunningJobs(ctx, route.SrcNodeID, route.DstNodeID); err != nil {
		return err
	}
	src, dst, channel := r.kusciaSrcDst(route)
	return r.db().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Unscoped().Delete(&model.NodeRouteDO{}, route.ID).Error; err != nil {
			return err
		}
		if c := r.ns.kc(channel); c != nil {
			if err := c.DeleteDomainRoute(ctx, src, dst); err != nil {
				return bizWrap(errcode.NodeRouteDeleteError, err, "")
			}
		}
		return nil
	})
}

// CreateRoute creates a route (Java createNodeRouter + createNodeRouteInKuscia).
// FullDuplex (default) creates both directions in Kuscia and in the DB; route
// ids follow "src__dst". Net addresses default to the nodes' netAddress.
func (r *NodeRouteService) CreateRoute(ctx context.Context, req *CreateRouteRequest) error {
	if req.SrcNodeID == "" || req.DstNodeID == "" {
		return bizErr(errcode.ParamError, "srcNodeId and dstNodeId are required")
	}
	if req.SrcNodeID == req.DstNodeID {
		return bizErr(errcode.NodeRouteSrcNodeAndDestNodeSame, "")
	}
	if _, err := r.ns.routeRepo.FindByPair(ctx, req.SrcNodeID, req.DstNodeID); err == nil {
		return ErrRouteAlreadyExists
	}
	srcAddr, dstAddr := req.SrcNetAddress, req.DstNetAddress
	if n := r.findNode(ctx, req.SrcNodeID); n != nil && srcAddr == "" {
		srcAddr = n.NetAddress
	}
	if n := r.findNode(ctx, req.DstNodeID); n != nil && dstAddr == "" {
		dstAddr = n.NetAddress
	}
	full := req.RouteType == "" || req.RouteType == RouteTypeFullDuplex
	channel := ""
	if r.ns.isAutonomy() {
		channel = req.SrcNodeID
	}

	_, reverseErr := r.ns.routeRepo.FindByPair(ctx, req.DstNodeID, req.SrcNodeID)
	createReverse := full && reverseErr != nil

	return r.db().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&model.NodeRouteDO{
			RouteID: RouteIDOf(req.SrcNodeID, req.DstNodeID), SrcNodeID: req.SrcNodeID, DstNodeID: req.DstNodeID,
			SrcNetAddress: srcAddr, DstNetAddress: dstAddr,
		}).Error; err != nil {
			return err
		}
		if createReverse {
			if err := tx.Create(&model.NodeRouteDO{
				RouteID: RouteIDOf(req.DstNodeID, req.SrcNodeID), SrcNodeID: req.DstNodeID, DstNodeID: req.SrcNodeID,
				SrcNetAddress: dstAddr, DstNetAddress: srcAddr,
			}).Error; err != nil {
				return err
			}
		}
		if !r.ns.hasKuscia() {
			return nil
		}
		if createReverse {
			if err := r.createKusciaRoute(ctx, req.DstNodeID, req.SrcNodeID, srcAddr, channel); err != nil {
				return err
			}
		}
		if err := r.createKusciaRoute(ctx, req.SrcNodeID, req.DstNodeID, dstAddr, channel); err != nil {
			if createReverse {
				if derr := r.ns.kc(channel).DeleteDomainRoute(ctx, req.DstNodeID, req.SrcNodeID); derr != nil {
					r.ns.log.Warn("rollback reverse route failed", zap.Error(derr))
				}
			}
			return err
		}
		return nil
	})
}

// DeletePair deletes src->dst and dst->src (Go-only node/route/delete).
func (r *NodeRouteService) DeletePair(ctx context.Context, src, dst string) error {
	route, err := r.ns.routeRepo.FindByPair(ctx, src, dst)
	if err != nil || route == nil {
		return ErrRouteNotFound
	}
	if err := r.validateNoRunningJobs(ctx, src, dst); err != nil {
		return err
	}
	channel := ""
	if r.ns.isAutonomy() {
		channel = src
	}
	return r.db().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Unscoped().Where("(src_node_id = ? AND dst_node_id = ?) OR (src_node_id = ? AND dst_node_id = ?)",
			src, dst, dst, src).Delete(&model.NodeRouteDO{}).Error; err != nil {
			return err
		}
		if c := r.ns.kc(channel); c != nil {
			if err := c.DeleteDomainRoute(ctx, src, dst); err != nil {
				return bizWrap(errcode.NodeRouteDeleteError, err, "")
			}
			if err := c.DeleteDomainRoute(ctx, dst, src); err != nil {
				return bizWrap(errcode.NodeRouteDeleteError, err, "")
			}
		}
		return nil
	})
}
