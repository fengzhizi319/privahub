package votehandler

import (
	"context"
	"strings"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/internal/service"
	"github.com/fengzhizi319/privahub/pkg/auth"
	"github.com/fengzhizi319/privahub/pkg/errcode"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// NodeRouteHandler is Java NodeRouteMessageHandler: a cooperative node route
// request (CENTER / EDGE). The destination node votes; once approved each
// executor living on this platform creates its half-duplex route.
type NodeRouteHandler struct{ base }

// NewNodeRouteHandler creates the NODE_ROUTE handler.
func NewNodeRouteHandler(d *Deps) *NodeRouteHandler { return &NodeRouteHandler{base{d}} }

func (h *NodeRouteHandler) Type() string       { return VoteTypeNodeRoute }
func (h *NodeRouteHandler) signAtCreate() bool { return false }

func (h *NodeRouteHandler) CreateApproval(ctx context.Context, initiator string, cfg *VoteConfig) (string, error) {
	return h.createApproval(ctx, h, initiator, cfg)
}

func (h *NodeRouteHandler) preCheck(ctx context.Context, tx *gorm.DB, initiator string, cfg *VoteConfig) error {
	// Java checkDataPermissions(nodeId): an EDGE user may only act for itself.
	if u := currentUser(ctx); u != nil && strings.EqualFold(u.PlatformType, auth.PlatformTypeEdge) && u.OwnerID != initiator {
		return bizErr(errcode.AuthFailed, "no Permissions")
	}
	if initiator != cfg.SrcNodeID {
		return bizErr(errcode.AuthFailed, "no Permissions")
	}
	var n int64
	tx.Model(&model.NodeRouteDO{}).Where("src_node_id = ? AND dst_node_id = ?", cfg.DesNodeID, cfg.SrcNodeID).Count(&n)
	if n > 0 {
		return bizErr(errcode.NodeRouteAlreadyExists, cfg.DesNodeID+" -> "+cfg.SrcNodeID)
	}
	return nil
}

func (h *NodeRouteHandler) createVoteConfig(ctx context.Context, tx *gorm.DB, voteID, initiator string, cfg *VoteConfig) error {
	if cfg.SrcNodeID == cfg.DesNodeID {
		return bizErr(errcode.NodeRouteSrcNodeAndDestNodeSame, cfg.SrcNodeID)
	}
	single := int8(0)
	if cfg.IsSingle != nil && *cfg.IsSingle {
		single = 1
	}
	return tx.Create(&model.NodeRouteApprovalConfigDO{
		VoteID: voteID, IsSingle: single,
		SrcNodeID: cfg.SrcNodeID, SrcNodeAddr: cfg.SrcNodeAddr,
		DesNodeID: cfg.DesNodeID, DesNodeAddr: cfg.DesNodeAddr,
		AllParticipants: toJSON([]string{initiator, cfg.DesNodeID}),
	}).Error
}

func (h *NodeRouteHandler) voters(_ context.Context, _ *gorm.DB, _ string, cfg *VoteConfig) ([]string, error) {
	return []string{cfg.DesNodeID}, nil
}

func (h *NodeRouteHandler) executors(_ string, cfg *VoteConfig) []string {
	return []string{cfg.SrcNodeID, cfg.DesNodeID}
}

func (h *NodeRouteHandler) approvedAction(_ context.Context, _ *gorm.DB, _ string, cfg *VoteConfig) (string, error) {
	return "NODE_ROUTE," + cfg.DesNodeID, nil
}

func (h *NodeRouteHandler) rejectAction(context.Context, *gorm.DB, string, *VoteConfig) (string, error) {
	return "", nil
}

func (h *NodeRouteHandler) requestDesc(ctx context.Context, tx *gorm.DB, _ string, cfg *VoteConfig) (string, error) {
	return h.nodeName(ctx, tx, cfg.DesNodeID), nil
}

func (h *NodeRouteHandler) inviteDesc(ctx context.Context, tx *gorm.DB, _ string, cfg *VoteConfig) (string, error) {
	return h.nodeName(ctx, tx, cfg.SrcNodeID), nil
}

func (h *NodeRouteHandler) config(ctx context.Context, voteID string) (*model.NodeRouteApprovalConfigDO, error) {
	var c model.NodeRouteApprovalConfigDO
	if err := h.d.DB.WithContext(ctx).Where("vote_id = ?", voteID).First(&c).Error; err != nil {
		return nil, bizErr(errcode.VoteNotExists, "")
	}
	return &c, nil
}

// GetVoteMessageDetail mirrors NodeRouteMessageHandler.getVoteMessageDetail.
func (h *NodeRouteHandler) GetVoteMessageDetail(ctx context.Context, isInitiator bool, nodeID, voteID string) (interface{}, error) {
	cfg, err := h.config(ctx, voteID)
	if err != nil {
		return nil, err
	}
	vr, err := h.findRequest(ctx, nil, voteID)
	if err != nil {
		return nil, err
	}
	out := &NodeRouteMessageDetail{IsSingle: cfg.IsSingle == 1}
	out.Type = VoteTypeNodeRoute
	out.Status = VoteStatusName(vr.Status)
	out.InitiatorNodeID = vr.Initiator
	out.InitiatorNodeName = h.nodeName(ctx, nil, vr.Initiator)
	var inv *model.VoteInviteDO
	if isInitiator {
		if inv, err = h.findInvite(ctx, voteID, cfg.DesNodeID); err != nil {
			return nil, err
		}
		out.MessageName = vr.Description
		out.URL = cfg.DesNodeAddr
	} else {
		if inv, err = h.findInvite(ctx, voteID, nodeID); err != nil {
			return nil, err
		}
		out.MessageName = inv.Description
		out.URL = cfg.SrcNodeAddr
	}
	out.NodeID = inv.VoteParticipantID
	out.NodeName = h.nodeName(ctx, nil, inv.VoteParticipantID)
	out.Reason = inv.Reason
	return out, nil
}

func (h *NodeRouteHandler) GetMessageListNecessaryInfo(context.Context, string) (interface{}, error) {
	return nil, nil
}

func (h *NodeRouteHandler) GetPartyStatusByVoteID(ctx context.Context, voteID string) ([]PartyVoteStatus, error) {
	return h.partyStatusByInvites(ctx, voteID)
}

func (h *NodeRouteHandler) DoCallBackRejected(ctx context.Context, vr *model.VoteRequestDO) {
	h.doCallBackRejectedDefault(ctx, vr)
}

// DoCallBackApproved creates the half-duplex route(s) owned by this platform
// (Java NodeRouteMessageHandler.doCallBackApproved).
func (h *NodeRouteHandler) DoCallBackApproved(ctx context.Context, vr *model.VoteRequestDO) {
	cfg, err := h.config(ctx, vr.VoteID)
	if err != nil {
		h.d.log().Error("node route callback: approval config not found", zap.String("vote", vr.VoteID))
		return
	}
	observer := true
	env := h.d.env()
	for _, executor := range parseStringList(vr.Executors) {
		if !env.IsCurrentNodeEnvironment(executor) {
			continue
		}
		observer = false
		req := &service.CreateRouteRequest{RouteType: service.RouteTypeHalfDuplex}
		if executor == vr.Initiator {
			req.SrcNodeID, req.DstNodeID = cfg.DesNodeID, cfg.SrcNodeID
			req.SrcNetAddress, req.DstNetAddress = cfg.DesNodeAddr, cfg.SrcNodeAddr
		} else {
			req.SrcNodeID, req.DstNodeID = cfg.SrcNodeID, cfg.DesNodeID
			req.SrcNetAddress, req.DstNetAddress = cfg.SrcNodeAddr, cfg.DesNodeAddr
		}
		h.createNodeRoute(ctx, vr, req)
	}
	if observer {
		h.observer(ctx, vr)
	}
}

func (h *NodeRouteHandler) createNodeRoute(ctx context.Context, vr *model.VoteRequestDO, req *service.CreateRouteRequest) {
	var n int64
	h.d.DB.WithContext(ctx).Model(&model.NodeRouteDO{}).
		Where("src_node_id = ? AND dst_node_id = ?", req.SrcNodeID, req.DstNodeID).Count(&n)
	if n > 0 {
		h.success(ctx, vr)
		return
	}
	if h.d.Routes == nil {
		h.failed(ctx, vr, "node route service not configured")
		return
	}
	// Java sets a UserContext named after the executor for the route creation.
	rctx := auth.ContextWithUser(ctx, &auth.UserContext{Name: req.DstNodeID, OwnerID: req.DstNodeID,
		OwnerType: auth.OwnerTypeCenter, PlatformType: h.d.env().PlatformTypeString()})
	if err := h.d.Routes.CreateRoute(rctx, req); err != nil {
		h.failed(ctx, vr, err.Error())
		return
	}
	h.success(ctx, vr)
}
