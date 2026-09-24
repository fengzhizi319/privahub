package service

import (
	"context"
	"encoding/json"
	"sort"
	"strings"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/pkg/errcode"
	"github.com/fengzhizi319/privahub/pkg/kuscia"
	"gorm.io/gorm"
)

// AUTONOMY (P2P) semantics of the graph chain (Java GraphServiceImpl
// verifyNodeAndRouteHealthy AUTONOMY branch, AutonomyNodeRouteUtil,
// EnvServiceImpl.findLocalNodeId, output target node conversion).

// localInstNodeIDs is Java P2pDataSyncProducerTemplate.nodeIds: the nodes of
// the local institution (the platform node plus the embedded / inst nodes).
func localInstNodeIDs() map[string]bool {
	env := GlobalEnv()
	out := map[string]bool{}
	if id := env.GetPlatformNodeId(); id != "" {
		out[id] = true
	}
	for _, id := range env.EmbeddedNodes() {
		out[id] = true
	}
	return out
}

// findLocalNodeID mirrors EnvServiceImpl.findLocalNodeId(task): the (last)
// party belonging to the local institution, else the (last) project node of
// the local institution; "" when none.
func (s *GraphService) findLocalNodeID(ctx context.Context, projectID string, parties []string) string {
	local := localInstNodeIDs()
	if len(local) == 0 {
		return ""
	}
	resp := ""
	for _, p := range parties {
		if local[p] {
			resp = p
		}
	}
	if resp != "" || projectID == "" {
		return resp
	}
	var pns []model.ProjectNodeDO
	s.db.WithContext(ctx).Where("project_id = ?", projectID).Order("id ASC").Find(&pns)
	for _, pn := range pns {
		if local[pn.NodeID] {
			resp = pn.NodeID
		}
	}
	return resp
}

// autonomySourceRoute is AutonomyNodeRouteUtil.AutonomySourceNodeRouteInfo.
type autonomySourceRoute struct {
	src       string
	available bool
}

type autonomyParticipant struct {
	InitiatorNodeID string `json:"initiatorNodeId"`
	Invitees        []struct {
		InviteeID string `json:"inviteeId"`
	} `json:"invitees"`
}

func (p *autonomyParticipant) hasInvitee(id string) bool {
	for _, i := range p.Invitees {
		if i.InviteeID == id {
			return true
		}
	}
	return false
}

// routeReady mirrors NodeRouteManager.checkNodeRouteReady(src, dst, channel).
func (s *GraphService) routeReady(ctx context.Context, src, dst, channel string) bool {
	k := s.kusciaFor(channel)
	if k == nil {
		return false
	}
	resp, err := k.QueryDomainRoute(ctx, src, dst)
	return err == nil && resp != nil && resp.Data.Status.Status == kuscia.RouteStatusSucceeded
}

// autonomySelfDstRoutes mirrors AutonomyNodeRouteUtil.getAutonomySelfDstNodeRouteInfoMap:
// for every route whose destination is a node of the operator's institution,
// dst -> [(src, src->dst ready)].
func (s *GraphService) autonomySelfDstRoutes(ctx context.Context, instID string) map[string][]autonomySourceRoute {
	out := map[string][]autonomySourceRoute{}
	var nodeIDs []string
	s.db.WithContext(ctx).Model(&model.NodeDO{}).Where("inst_id = ?", instID).Pluck("node_id", &nodeIDs)
	if len(nodeIDs) == 0 {
		return out
	}
	var routes []model.NodeRouteDO
	s.db.WithContext(ctx).Where("dst_node_id IN ?", nodeIDs).Order("id ASC").Find(&routes)
	for _, r := range routes {
		out[r.DstNodeID] = append(out[r.DstNodeID], autonomySourceRoute{
			src:       r.SrcNodeID,
			available: s.routeReady(ctx, r.SrcNodeID, r.DstNodeID, r.DstNodeID),
		})
	}
	return out
}

// verifyAutonomyRoutes is the AUTONOMY branch of verifyNodeAndRouteHealthy.
func (s *GraphService) verifyAutonomyRoutes(ctx context.Context, projectID string, parties []string) error {
	if len(parties) <= 1 {
		return nil // unilateral mission
	}
	instID := OperatorFrom(ctx).OwnerID
	var instNodes []string
	s.db.WithContext(ctx).Model(&model.NodeDO{}).Where("inst_id = ?", instID).Pluck("node_id", &instNodes)
	partySet := map[string]bool{}
	for _, p := range parties {
		partySet[p] = true
	}
	own := map[string]bool{}
	for _, n := range instNodes {
		if partySet[n] {
			own[n] = true
		}
	}
	all := s.autonomySelfDstRoutes(ctx, instID)
	filtered := map[string][]autonomySourceRoute{}
	dsts := make([]string, 0)
	for dst, v := range all {
		if own[dst] {
			filtered[dst] = v
			dsts = append(dsts, dst)
		}
	}
	sort.Strings(dsts)

	var cfg model.ProjectApprovalConfigDO
	if err := s.db.WithContext(ctx).Where("project_id = ? AND type = ?", projectID, "PROJECT_CREATE").First(&cfg).Error; err != nil {
		return padErr(errcode.ProjectNotExists, "project approval config not exists")
	}
	var participants []autonomyParticipant
	if strings.TrimSpace(cfg.ParticipantNodeInfo) != "" {
		_ = json.Unmarshal([]byte(cfg.ParticipantNodeInfo), &participants)
	}

	for _, party := range parties {
		if _, ok := filtered[party]; ok {
			continue
		}
		found := false
		for _, dst := range dsts {
			var info *autonomySourceRoute
			for i := range filtered[dst] {
				if filtered[dst][i].src == party {
					info = &filtered[dst][i]
					break
				}
			}
			if info == nil {
				continue
			}
			found = true
			if !info.available {
				for i := range participants {
					vo := &participants[i]
					if vo.InitiatorNodeID == info.src {
						if vo.hasInvitee(dst) {
							return padErr(errcode.GraphNodeRouteNotExists, party+"->"+dst)
						}
					} else if vo.hasInvitee(info.src) && vo.InitiatorNodeID == dst {
						return padErr(errcode.GraphNodeRouteNotExists, party+"-> "+dst)
					}
				}
			}
			break
		}
		if !found {
			return padErr(errcode.GraphNodeRouteNotExists, party+"-> ["+strings.Join(parties, ", ")+"]")
		}
	}
	return nil
}

// autonomyTargetNode mirrors the output target node conversion of
// getGraphNodeTaskOutputVO: in AUTONOMY mode a result owned by a node of
// another institution is read through the first project node that is not a
// party of the task.
func autonomyTargetNode(ctx context.Context, db *gorm.DB, projectID, nodeID string, taskParties []string) string {
	if !GlobalEnv().IsAutonomy() || localInstNodeIDs()[nodeID] {
		return nodeID
	}
	var pns []model.ProjectNodeDO
	db.WithContext(ctx).Where("project_id = ?", projectID).Order("id ASC").Find(&pns)
	for _, pn := range pns {
		if !containsString(taskParties, pn.NodeID) {
			return pn.NodeID
		}
	}
	return nodeID
}
