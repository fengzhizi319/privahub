package votehandler

import (
	"context"
	"encoding/json"
	"regexp"
	"sort"
	"strings"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/internal/graphjob"
	"github.com/fengzhizi319/privahub/internal/service"
	"github.com/fengzhizi319/privahub/pkg/auth"
	"github.com/fengzhizi319/privahub/pkg/errcode"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// P2PService implements the Java P2P project (ProjectServiceImpl.*P2P*) and
// P2P node (NodeServiceImpl.createP2pNode / deleteP2pNodeRoute) logic.
type P2PService struct {
	d *Deps
	b *autonomyBase
}

// NewP2PService creates the P2P service sharing the vote dependencies.
func NewP2PService(d *Deps) *P2PService {
	return &P2PService{d: d, b: &autonomyBase{base{d}}}
}

func (s *P2PService) ownerID(ctx context.Context) (string, error) {
	u := currentUser(ctx)
	if u == nil || u.OwnerID == "" {
		return "", bizErr(errcode.AuthFailed, "no login user")
	}
	return u.OwnerID, nil
}

// ============================== projects ==============================

// P2PCreateProjectRequest is Java CreateProjectRequest (P2P fields).
type P2PCreateProjectRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	ComputeMode string `json:"computeMode"`
	ComputeFunc string `json:"computeFunc"`
	TeeNodeID   string `json:"teeNodeId"`
}

// P2PProjectNodeVO is Java ProjectNodeVO (P2P list: no datatables).
type P2PProjectNodeVO struct {
	NodeID   string `json:"nodeId"`
	NodeName string `json:"nodeName"`
}

// P2PProjectInstVO is Java ProjectInstVO.
type P2PProjectInstVO struct {
	InstID   string `json:"instId"`
	InstName string `json:"instName"`
}

// PartyVoteInfoVO is Java PartyVoteInfoVO.
type PartyVoteInfoVO struct {
	PartyID   string `json:"partyId"`
	PartyName string `json:"partyName"`
	Action    string `json:"action"`
	Reason    string `json:"reason"`
}

// P2PProjectVO is Java ProjectVO as returned by p2p/project/list.
type P2PProjectVO struct {
	PartyVoteInfos []PartyVoteInfoVO  `json:"partyVoteInfos"`
	ProjectID      string             `json:"projectId"`
	ProjectName    string             `json:"projectName"`
	Description    string             `json:"description"`
	Nodes          []P2PProjectNodeVO `json:"nodes"`
	Insts          []P2PProjectInstVO `json:"insts"`
	GraphCount     int64              `json:"graphCount"`
	JobCount       int64              `json:"jobCount"`
	GmtCreate      string             `json:"gmtCreate"`
	ComputeMode    string             `json:"computeMode"`
	TeeNodeID      string             `json:"teeNodeId"`
	Status         string             `json:"status"`
	Initiator      string             `json:"initiator"`
	InitiatorName  string             `json:"initiatorName"`
	ComputeFunc    string             `json:"computeFunc"`
	VoteID         string             `json:"voteId"`
	gmt            int64
}

// ProjectParticipantsDetailVO is Java ProjectParticipantsDetailVO.
type ProjectParticipantsDetailVO struct {
	InitiatorID            string                  `json:"initiatorId"`
	InitiatorName          string                  `json:"initiatorName"`
	ProjectName            string                  `json:"projectName"`
	PartyVoteStatuses      []PartyVoteStatus       `json:"partyVoteStatuses"`
	ComputeMode            string                  `json:"computeMode"`
	ComputeFunc            string                  `json:"computeFunc"`
	ProjectDesc            string                  `json:"projectDesc"`
	GmtCreated             string                  `json:"gmtCreated"`
	ParticipantNodeInstVOS []ParticipantNodeInstVO `json:"participantNodeInstVOS"`
}

// CreateProject is Java createP2PProject: owner = login owner, status REVIEWING.
func (s *P2PService) CreateProject(ctx context.Context, req *P2PCreateProjectRequest) (string, error) {
	owner, err := s.ownerID(ctx)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(req.Name) == "" {
		return "", bizErr(errcode.ParamError, "name is required")
	}
	mode := strings.ToUpper(strings.TrimSpace(req.ComputeMode))
	if mode == "" {
		mode = "MPC"
	}
	if mode == "TEE" && req.TeeNodeID == "" {
		return "", bizErr(errcode.SystemValidationError, "teeNodeId can not be empty in TEE mode")
	}
	info, _ := json.Marshal(map[string]string{"teeDomainId": req.TeeNodeID})
	p := &model.ProjectDO{
		ProjectID:   graphjob.RandomLetters(8),
		Name:        req.Name,
		Description: req.Description,
		ComputeMode: mode,
		ComputeFunc: firstNonEmpty(req.ComputeFunc, "ALL"),
		ProjectInfo: string(info),
		OwnerID:     owner,
		Status:      ProjectReviewing,
	}
	if err := s.d.DB.WithContext(ctx).Create(p).Error; err != nil {
		return "", err
	}
	return p.ProjectID, nil
}

func firstNonEmpty(vs ...string) string {
	for _, v := range vs {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// ListProjects is Java listP2PProject: projects of my institution plus every
// project with a PROJECT_CREATE vote, enriched with vote information.
func (s *P2PService) ListProjects(ctx context.Context) ([]P2PProjectVO, error) {
	owner, err := s.ownerID(ctx)
	if err != nil {
		return nil, err
	}
	db := s.d.DB.WithContext(ctx)
	idSet := map[string]bool{}
	var mine []string
	db.Model(&model.ProjectInstDO{}).Where("inst_id = ?", owner).Pluck("project_id", &mine)
	for _, id := range mine {
		idSet[id] = true
	}
	var cfgs []model.ProjectApprovalConfigDO
	if err := db.Where("type = ?", VoteTypeProjectCreate).Order("id").Find(&cfgs).Error; err != nil {
		return nil, err
	}
	voteOf := map[string]string{}
	for _, c := range cfgs {
		idSet[c.ProjectID] = true
		voteOf[c.ProjectID] = c.VoteID
	}
	out := []P2PProjectVO{}
	if len(idSet) == 0 {
		return out, nil
	}
	ids := make([]string, 0, len(idSet))
	for id := range idSet {
		ids = append(ids, id)
	}
	var projects []model.ProjectDO
	if err := db.Where("project_id IN ?", ids).Find(&projects).Error; err != nil {
		return nil, err
	}
	for _, p := range projects {
		voteID, ok := voteOf[p.ProjectID]
		if !ok {
			// Java throws PROJECT_VOTE_NOT_EXISTS here; skipping keeps the list usable.
			s.d.log().Warn("p2p project without PROJECT_CREATE vote", zap.String("project", p.ProjectID))
			continue
		}
		out = append(out, s.projectVO(ctx, &p, voteID))
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].gmt > out[j].gmt })
	return out, nil
}

func (s *P2PService) projectVO(ctx context.Context, p *model.ProjectDO, voteID string) P2PProjectVO {
	db := s.d.DB.WithContext(ctx)
	vo := P2PProjectVO{
		ProjectID: p.ProjectID, ProjectName: p.Name, Description: p.Description, ComputeMode: p.ComputeMode,
		GmtCreate: rfc3339(p.GmtCreate), Status: ProjectStatusName(p.Status), Initiator: p.OwnerID,
		ComputeFunc: p.ComputeFunc, VoteID: voteID, gmt: p.GmtCreate.UnixNano(),
		Nodes: []P2PProjectNodeVO{}, Insts: []P2PProjectInstVO{}, PartyVoteInfos: []PartyVoteInfoVO{},
	}
	var info struct {
		TeeDomainID string `json:"teeDomainId"`
	}
	if json.Unmarshal([]byte(p.ProjectInfo), &info) == nil {
		vo.TeeNodeID = info.TeeDomainID
	}
	var nodeIDs []string
	db.Model(&model.ProjectNodeDO{}).Where("project_id = ?", p.ProjectID).Order("id").Pluck("node_id", &nodeIDs)
	names := s.b.nodeNames(ctx, nodeIDs)
	for _, n := range nodeIDs {
		vo.Nodes = append(vo.Nodes, P2PProjectNodeVO{NodeID: n, NodeName: names[n]})
	}
	var instIDs []string
	db.Model(&model.ProjectInstDO{}).Where("project_id = ?", p.ProjectID).Order("id").Pluck("inst_id", &instIDs)
	db.Model(&model.ProjectGraphDO{}).Where("project_id = ?", p.ProjectID).Count(&vo.GraphCount)
	db.Model(&model.ProjectJobDO{}).Where("project_id = ?", p.ProjectID).Count(&vo.JobCount)

	var infos []PartyVoteInfo
	var vr model.VoteRequestDO
	if db.Where("vote_id = ?", voteID).First(&vr).Error == nil {
		infos = parsePartyVoteInfos(vr.PartyVoteInfo)
	}
	lookup := append([]string{p.OwnerID}, instIDs...)
	for _, i := range infos {
		lookup = append(lookup, i.PartyID)
	}
	instNames := s.b.instNames(ctx, lookup)
	for _, i := range instIDs {
		vo.Insts = append(vo.Insts, P2PProjectInstVO{InstID: i, InstName: instNames[i]})
	}
	for _, i := range infos {
		if i.PartyID == p.OwnerID {
			continue
		}
		vo.PartyVoteInfos = append(vo.PartyVoteInfos, PartyVoteInfoVO{PartyID: i.PartyID, PartyName: instNames[i.PartyID],
			Action: i.Action, Reason: i.Reason})
	}
	vo.InitiatorName = instNames[p.OwnerID]
	return vo
}

// UpdateProject is Java updateP2PProject: only the owner may update.
func (s *P2PService) UpdateProject(ctx context.Context, projectID, name, description string) error {
	owner, err := s.ownerID(ctx)
	if err != nil {
		return err
	}
	p, err := s.b.openProject(ctx, nil, projectID)
	if err != nil {
		return err
	}
	if p.OwnerID != owner {
		return bizErr(errcode.ProjectUpdateFail, "")
	}
	updates := map[string]interface{}{}
	if name != "" {
		updates["name"] = name
	}
	if description != "" {
		updates["description"] = description
	}
	if len(updates) == 0 {
		return nil
	}
	return s.d.DB.WithContext(ctx).Model(&model.ProjectDO{}).Where("project_id = ?", projectID).Updates(updates).Error
}

// ArchiveProject is Java archiveProject: owner only, project must still be
// REVIEWING; members are removed and the status becomes ARCHIVED.
func (s *P2PService) ArchiveProject(ctx context.Context, projectID string) error {
	p, err := s.b.openProject(ctx, nil, projectID)
	if err != nil {
		return err
	}
	owner, err := s.ownerID(ctx)
	if err != nil {
		return err
	}
	if p.OwnerID != owner {
		return bizErr(errcode.ProjectArchiveFail, "")
	}
	if p.Status != ProjectReviewing {
		return bizErr(errcode.ProjectCanNotArchive, "")
	}
	return s.d.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Unscoped().Where("project_id = ?", projectID).Delete(&model.ProjectInstDO{}).Error; err != nil {
			return err
		}
		if err := tx.Unscoped().Where("project_id = ?", projectID).Delete(&model.ProjectNodeDO{}).Error; err != nil {
			return err
		}
		return setProjectStatus(tx, projectID, ProjectArchived)
	})
}

// Participants is Java getProjectParticipants(voteId).
func (s *P2PService) Participants(ctx context.Context, voteID string) (*ProjectParticipantsDetailVO, error) {
	vr, err := s.b.findRequest(ctx, nil, voteID)
	if err != nil {
		return nil, err
	}
	_, body, err := parseRequest(vr.RequestMsg)
	if err != nil {
		return nil, err
	}
	statuses, err := s.b.partyStatusByPartyInfos(ctx, voteID)
	if err != nil {
		return nil, err
	}
	cfg, err := s.b.approvalConfig(ctx, nil, voteID)
	if err != nil {
		return nil, err
	}
	participants, err := s.b.fillParticipantNames(ctx, parseParticipantNodeInfo(cfg.ParticipantNodeInfo))
	if err != nil {
		return nil, err
	}
	p, err := s.b.openProject(ctx, nil, cfg.ProjectID)
	if err != nil {
		return nil, err
	}
	return &ProjectParticipantsDetailVO{
		InitiatorID: body.Initiator, InitiatorName: s.b.instName(ctx, body.Initiator), ProjectName: p.Name,
		PartyVoteStatuses: statuses, ParticipantNodeInstVOS: participants, ComputeFunc: p.ComputeFunc,
		ComputeMode: p.ComputeMode, ProjectDesc: p.Description, GmtCreated: rfc3339(p.GmtCreate),
	}, nil
}

// ================================ nodes ================================

// P2PCreateNodeRequest is Java P2pCreateNodeRequest.
type P2PCreateNodeRequest struct {
	Name          string `json:"name"`
	Mode          *int   `json:"mode"`
	MasterNodeID  string `json:"masterNodeId"`
	CertText      string `json:"certText"`
	DstNodeID     string `json:"dstNodeId"`
	SrcNetAddress string `json:"srcNetAddress"`
	SrcNodeID     string `json:"srcNodeId"`
	DstNetAddress string `json:"dstNetAddress"`
	DstInstID     string `json:"dstInstId"`
	DstInstName   string `json:"dstInstName"`
}

var (
	nodeNamePattern = regexp.MustCompile(`^[\p{Han}a-zA-Z0-9_-]{1,32}$`)
	ipPrefixPattern = regexp.MustCompile(`^(http|https)://.*`)
	ipPortPattern   = regexp.MustCompile(`^.{1,50}:([0-9]|[1-9]\d|[1-9]\d{2}|[1-9]\d{3}|[1-5]\d{4}|6[0-4]\d{3}|65[0-4]\d{2}|655[0-2]\d|6553[0-5])$`)
)

// Validate applies the Java bean-validation annotations.
func (r *P2PCreateNodeRequest) Validate() error {
	if r.Name != "" && !nodeNamePattern.MatchString(r.Name) {
		return bizErr(errcode.SystemValidationError, "node name illegal")
	}
	if r.Mode != nil && (*r.Mode < 0 || *r.Mode > 2) {
		return bizErr(errcode.SystemValidationError, "mode must be between 0 and 2")
	}
	if strings.TrimSpace(r.CertText) == "" {
		return bizErr(errcode.SystemValidationError, "certText must not be blank")
	}
	if strings.TrimSpace(r.DstNodeID) == "" {
		return bizErr(errcode.SystemValidationError, "dstNodeId must not be blank")
	}
	if strings.TrimSpace(r.SrcNodeID) == "" {
		return bizErr(errcode.SystemValidationError, "srcNodeId cannot be empty")
	}
	if strings.TrimSpace(r.DstNetAddress) == "" {
		return bizErr(errcode.SystemValidationError, "dstNetAddress must not be blank")
	}
	if !ipPrefixPattern.MatchString(r.DstNetAddress) || (r.SrcNetAddress != "" && !ipPrefixPattern.MatchString(r.SrcNetAddress)) {
		return bizErr(errcode.SystemValidationError, "address needs to start with http or https")
	}
	if !ipPortPattern.MatchString(r.DstNetAddress) {
		return bizErr(errcode.SystemValidationError, "address not support rule")
	}
	return nil
}

// CreateNode is Java createP2pNode: validate address and cert, create the
// partner node (DB + P2P_NODE permission + inst + Kuscia partner domain via
// the src node channel) and the full-duplex route.
func (s *P2PService) CreateNode(ctx context.Context, req *P2PCreateNodeRequest) (string, error) {
	if err := req.Validate(); err != nil {
		return "", err
	}
	if req.DstNetAddress != "" && req.SrcNetAddress == req.DstNetAddress {
		return "", bizErr(errcode.NodeRouteConfigError, "")
	}
	self := s.d.platformNodeID()
	if u := currentUser(ctx); u != nil && u.PlatformNodeID != "" {
		self = u.PlatformNodeID
	}
	if self == req.DstNodeID {
		return "", bizErr(errcode.NodeCertConfigError, "")
	}
	if s.d.Kuscia != nil {
		if own := s.d.Kuscia.DomainCert(ctx, self, self); own != "" && own == req.CertText {
			return "", bizErr(errcode.NodeCertConfigError, "")
		}
	}
	mode := 0
	if req.Mode != nil {
		mode = *req.Mode
	}
	master := firstNonEmpty(req.MasterNodeID, self)
	nodeID := req.DstNodeID
	err := s.d.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var node model.NodeDO
		if tx.Unscoped().Where("node_id = ?", nodeID).First(&node).Error == nil {
			if node.DeletedAt.Valid || node.IsDeleted != 0 {
				if err := tx.Unscoped().Model(&node).Updates(map[string]interface{}{
					"name": req.Name, "mode": mode, "master_node_id": master, "net_address": req.DstNetAddress,
					"inst_id": req.DstInstID, "is_deleted": 0, "deleted_at": nil,
				}).Error; err != nil {
					return bizWrap(errcode.NodeCreateError, err, nodeID+" node create fail in db :"+err.Error())
				}
			} else {
				return bizErr(errcode.NodeCreateError, nodeID+" node create fail in db : node already exists")
			}
		} else if err := tx.Create(&model.NodeDO{
			NodeID: nodeID, ControlNodeID: nodeID, NetAddress: req.DstNetAddress, MasterNodeID: master,
			Type: "normal", Mode: mode, Name: req.Name, InstID: req.DstInstID,
		}).Error; err != nil {
			return bizWrap(errcode.NodeCreateError, err, nodeID+" node create fail in db :"+err.Error())
		}
		if err := tx.Where(model.SysUserPermissionRelDO{UserKey: nodeID, TargetCode: auth.RoleP2PNode}).
			Attrs(model.SysUserPermissionRelDO{UserType: auth.PermissionUserTypeNode, TargetType: "ROLE"}).
			FirstOrCreate(&model.SysUserPermissionRelDO{}).Error; err != nil {
			return err
		}
		if req.DstInstID != "" {
			var inst model.InstDO
			if tx.Where("inst_id = ?", req.DstInstID).First(&inst).Error != nil {
				if err := tx.Create(&model.InstDO{InstID: req.DstInstID, Name: req.DstInstName}).Error; err != nil {
					return err
				}
			} else if req.DstInstName != "" && inst.Name != req.DstInstName {
				tx.Model(&inst).Update("name", req.DstInstName)
			}
		}
		if s.d.Kuscia != nil {
			if err := s.d.Kuscia.CreatePartnerDomain(ctx, req.SrcNodeID, nodeID, req.CertText); err != nil {
				return bizWrap(errcode.NodeCreateError, err, nodeID+" node create fail in kuscia :"+err.Error())
			}
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if s.d.Routes != nil {
		if err := s.d.Routes.CreateRoute(ctx, &service.CreateRouteRequest{
			SrcNodeID: req.SrcNodeID, DstNodeID: req.DstNodeID,
			SrcNetAddress: req.SrcNetAddress, DstNetAddress: req.DstNetAddress,
		}); err != nil {
			s.rollbackNode(ctx, req.SrcNodeID, nodeID)
			return "", err
		}
	}
	return nodeID, nil
}

// rollbackNode compensates a failed route creation (Java relies on @Transactional).
func (s *P2PService) rollbackNode(ctx context.Context, channel, nodeID string) {
	db := s.d.DB.WithContext(ctx)
	db.Unscoped().Where("node_id = ?", nodeID).Delete(&model.NodeDO{})
	db.Where("user_key = ? AND target_code = ?", nodeID, auth.RoleP2PNode).Delete(&model.SysUserPermissionRelDO{})
	if s.d.Kuscia != nil {
		if err := s.d.Kuscia.DeleteDomain(ctx, channel, nodeID); err != nil {
			s.d.log().Warn("rollback p2p node domain failed", zap.String("node", nodeID), zap.Error(err))
		}
	}
}

// DeleteNodeRoute is Java deleteP2pNodeRoute(routerId).
func (s *P2PService) DeleteNodeRoute(ctx context.Context, routerID string) error {
	db := s.d.DB.WithContext(ctx)
	var route model.NodeRouteDO
	if routerID == "" || db.Where("route_id = ?", routerID).First(&route).Error != nil {
		return bizErr(errcode.NodeRouteNotExistError, "route not exist "+routerID)
	}
	// verifyAndFind: the src node must not be a party of a reviewing project.
	var reviewing []string
	db.Model(&model.ProjectDO{}).Where("status = ?", ProjectReviewing).Pluck("project_id", &reviewing)
	if len(reviewing) > 0 {
		var cfgs []model.ProjectApprovalConfigDO
		db.Where("project_id IN ? AND type = ?", reviewing, VoteTypeProjectCreate).Find(&cfgs)
		for _, c := range cfgs {
			if contains(parseStringList(c.Parties), route.SrcNodeID) {
				return bizErr(errcode.NodeDeleteError, "node have project reviewing")
			}
		}
	}
	if err := s.validateNoRunningJobs(ctx, route.SrcNodeID, route.DstNodeID); err != nil {
		return err
	}
	channel, err := s.channelNodeID(ctx, &route)
	if err != nil {
		return err
	}
	if s.d.Kuscia != nil {
		for _, pair := range [][2]string{{route.SrcNodeID, route.DstNodeID}, {route.DstNodeID, route.SrcNodeID}} {
			if ok, _ := s.d.Kuscia.DomainRouteExists(ctx, channel, pair[0], pair[1]); ok {
				if err := s.d.Kuscia.DeleteDomainRoute(ctx, channel, pair[0], pair[1]); err != nil {
					return bizWrap(errcode.NodeRouteDeleteError, err, "")
				}
			}
		}
		if err := s.d.Kuscia.DeleteDomain(ctx, channel, route.SrcNodeID); err != nil {
			return bizWrap(errcode.NodeDeleteError, err, "")
		}
	}
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Unscoped().Where("route_id = ?", routerID).Delete(&model.NodeRouteDO{}).Error; err != nil {
			return err
		}
		// Java also removes the paired route in the DB when the node is gone.
		var others int64
		tx.Model(&model.NodeRouteDO{}).Where("src_node_id = ?", route.SrcNodeID).Count(&others)
		if others > 0 {
			return nil
		}
		if err := tx.Unscoped().Where("node_id = ?", route.SrcNodeID).Delete(&model.NodeDO{}).Error; err != nil {
			return err
		}
		return tx.Where("user_key = ? AND target_code = ?", route.SrcNodeID, auth.RoleP2PNode).
			Delete(&model.SysUserPermissionRelDO{}).Error
	})
}

// channelNodeID is the node of my institution among src/dst (Java findChannelNodeId).
func (s *P2PService) channelNodeID(ctx context.Context, route *model.NodeRouteDO) (string, error) {
	inst := s.d.currentInstID(ctx)
	var mine []string
	if inst != "" {
		s.d.DB.WithContext(ctx).Model(&model.NodeDO{}).Where("inst_id = ?", inst).Pluck("node_id", &mine)
	}
	if contains(mine, route.SrcNodeID) {
		return route.SrcNodeID, nil
	}
	if contains(mine, route.DstNodeID) {
		return route.DstNodeID, nil
	}
	if self := s.d.platformNodeID(); self == route.SrcNodeID || self == route.DstNodeID {
		return self, nil
	}
	return "", bizErr(errcode.NodeNotExistError, "Both nodes--> sourceNode "+route.SrcNodeID+
		" and destNode "+route.DstNodeID+" are not within the inst")
}

// validateNoRunningJobs rejects the deletion while a project shared by both
// nodes has a RUNNING job (Java NodeRouterServiceImpl.validateNoRunningJobs).
func (s *P2PService) validateNoRunningJobs(ctx context.Context, src, dst string) error {
	db := s.d.DB.WithContext(ctx)
	projects := map[string]bool{}
	var shared []string
	db.Model(&model.ProjectNodeDO{}).Where("node_id = ? AND project_id IN (?)", src,
		s.d.DB.Model(&model.ProjectNodeDO{}).Select("project_id").Where("node_id = ?", dst)).Pluck("project_id", &shared)
	for _, p := range shared {
		projects[p] = true
	}
	var cfgs []model.ProjectApprovalConfigDO
	db.Where("type = ?", VoteTypeProjectCreate).Find(&cfgs)
	for _, c := range cfgs {
		nodes := map[string]bool{}
		for _, p := range parseParticipantNodeInfo(c.ParticipantNodeInfo) {
			nodes[p.InitiatorNodeID] = true
			for _, i := range p.Invitees {
				nodes[i.InviteeID] = true
			}
		}
		if nodes[src] && nodes[dst] {
			projects[c.ProjectID] = true
		}
	}
	if len(projects) == 0 {
		return nil
	}
	ids := make([]string, 0, len(projects))
	for p := range projects {
		ids = append(ids, p)
	}
	var running int64
	db.Model(&model.ProjectJobDO{}).Where("project_id IN ? AND status = ?", ids, "RUNNING").Count(&running)
	if running > 0 {
		return bizErr(errcode.NodeRouteUpdateError, "cannot update node route while a job is running.")
	}
	return nil
}
