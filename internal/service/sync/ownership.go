package datasync

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"go.uber.org/zap"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/internal/service"
)

// ErrNotOwner rejects peer sync data the sending node does not own.
var ErrNotOwner = errors.New("sync data is not owned by the sending node")

// peer identifies the node posting P2P sync data (kuscia-origin-source) and
// its institution (node.inst_id, when known locally).
type peer struct {
	node string
	inst string
}

// isPeer reports whether id is the sending node or its institution.
func (p peer) isPeer(id string) bool {
	return id != "" && (strings.EqualFold(id, p.node) || (p.inst != "" && strings.EqualFold(id, p.inst)))
}

// ApplyFromPeer applies one SyncData received from a P2P peer (inner-port
// /api/v1alpha1/data/sync). It is Apply(ModeP2P) plus a per-table ownership
// check that mirrors, in reverse, the producer filter
// (P2PSync.Filter / Java P2pDataSyncProducerTemplate.filter): a peer may only
// write rows it would legitimately have produced — rows owned by its
// institution, bound to its own nodes, or votes it initiated / answers.
// Both the incoming row and the local row with the same business key (the
// row that would be overwritten or deleted) must pass. Tables without a rule
// are rejected (default deny).
func (c *Consumer) ApplyFromPeer(ctx context.Context, origin string, d *service.SyncData) (interface{}, error) {
	origin = strings.TrimSpace(origin)
	if origin == "" {
		return nil, fmt.Errorf("%w: missing sending node", ErrNotOwner)
	}
	if d == nil {
		return nil, fmt.Errorf("nil sync data")
	}
	table, ok := c.reg.Canonical(d.TableName)
	if !ok || (c.allowed != nil && !c.allowed.Has(table)) {
		return nil, fmt.Errorf("%w: %s", ErrTableNotAllowed, d.TableName)
	}
	action := service.NormalizeSyncAction(d.Action)
	if action == "" {
		return nil, fmt.Errorf("unknown sync action %q", d.Action)
	}
	raw, err := d.DataJSON()
	if err != nil {
		return nil, err
	}
	entity, err := c.reg.Decode(table, raw)
	if err != nil {
		return nil, err
	}
	p := c.peerOf(ctx, origin)
	if err := c.checkPeerOwnership(ctx, table, action, p, entity); err != nil {
		c.log.Warn("P2P data sync rejected: not owned by the sending node",
			zap.String("origin", origin), zap.String("table", table), zap.String("action", action), zap.Error(err))
		return nil, err
	}
	return c.Apply(ctx, ModeP2P, d)
}

func (c *Consumer) peerOf(ctx context.Context, origin string) peer {
	p := peer{node: origin}
	if c.db == nil || !c.db.Migrator().HasColumn(&model.NodeDO{}, "inst_id") {
		return p
	}
	var n model.NodeDO
	if c.db.WithContext(ctx).Where("node_id = ?", origin).Limit(1).Find(&n).RowsAffected > 0 {
		p.inst = n.InstID
	}
	return p
}

// peerNode reports whether nodeID is the sending node or a node of its
// institution.
func (c *Consumer) peerNode(ctx context.Context, p peer, nodeID string) bool {
	if p.isPeer(nodeID) {
		return true
	}
	if nodeID == "" || p.inst == "" || c.db == nil {
		return false
	}
	var n int64
	c.db.WithContext(ctx).Model(&model.NodeDO{}).Where("node_id = ? AND inst_id = ?", nodeID, p.inst).Count(&n)
	return n > 0
}

// localParty reports whether id is this platform's institution or one of its nodes.
func (c *Consumer) localParty(id string) bool {
	if id == "" {
		return false
	}
	if c.identity != nil && strings.EqualFold(id, c.identity.InstID()) {
		return true
	}
	return c.isLocalNode(id)
}

// ownerOf mirrors P2PSync.owner: the graph owner when graphID is set, else
// the project owner (soft-deleted rows included).
func (c *Consumer) ownerOf(ctx context.Context, projectID, graphID string) string {
	if c.db == nil || projectID == "" {
		return ""
	}
	db := c.db.WithContext(ctx).Unscoped()
	if graphID != "" {
		var g model.ProjectGraphDO
		if db.Where("project_id = ? AND graph_id = ?", projectID, graphID).Limit(1).Find(&g).RowsAffected > 0 {
			return g.OwnerID
		}
		return ""
	}
	var pr model.ProjectDO
	if db.Where("project_id = ?", projectID).Limit(1).Find(&pr).RowsAffected > 0 {
		return pr.OwnerID
	}
	return ""
}

// peerOwns applies the reverse producer rule of table to one row.
func (c *Consumer) peerOwns(ctx context.Context, table, action string, p peer, entity interface{}) bool {
	v := reflect.ValueOf(entity)
	f := func(name string) string { return fieldString(v, name) }
	switch table {
	case TableProject, TableProjectGraph:
		return p.isPeer(f("OwnerID"))
	case TableProjectGraphNode, TableProjectGraphNodeKuscia, TableProjectJob:
		return p.isPeer(c.ownerOf(ctx, f("ProjectID"), f("GraphID")))
	case TableProjectNode, TableProjectInst:
		return p.isPeer(c.ownerOf(ctx, f("ProjectID"), ""))
	case TableProjectDatatable, TableProjectFeatureTable:
		return c.peerNode(ctx, p, f("NodeID"))
	case TableProjectGraphDomainDs:
		return c.peerNode(ctx, p, f("DomainID"))
	case TableProjectModelServing, TableProjectModelPack:
		return c.peerNode(ctx, p, f("Initiator"))
	case TableVoteRequest, TableProjectApprovalConfig:
		return p.isPeer(f("Initiator"))
	case TableVoteInvite:
		// Invites are created / withdrawn by the initiator; an invitee only
		// answers (update) its own invite of a vote this platform initiated.
		if p.isPeer(f("Initiator")) {
			return true
		}
		return action == service.SyncActionUpdate && c.localParty(f("Initiator")) &&
			c.peerNode(ctx, p, f("VoteParticipantID"))
	case TableProjectSchedule, TableProjectScheduleJob, TableProjectScheduleTask:
		return p.isPeer(f("Owner"))
	}
	return false // no rule: default deny
}

// checkPeerOwnership checks the incoming row (create / update) and the local
// row it would overwrite or delete.
func (c *Consumer) checkPeerOwnership(ctx context.Context, table, action string, p peer, entity interface{}) error {
	if action != service.SyncActionDelete && !c.peerOwns(ctx, table, action, p, entity) {
		return fmt.Errorf("%w: %s row from %s", ErrNotOwner, table, p.node)
	}
	existing, err := c.existingRow(ctx, table, entity)
	if err != nil {
		return fmt.Errorf("%w: %s: %v", ErrNotOwner, table, err)
	}
	if existing != nil && !c.peerOwns(ctx, table, action, p, existing) {
		return fmt.Errorf("%w: local %s row is not owned by %s", ErrNotOwner, table, p.node)
	}
	return nil
}

// existingRow loads the local row (soft-deleted included) with the entity's
// business key, or nil.
func (c *Consumer) existingRow(ctx context.Context, table string, entity interface{}) (interface{}, error) {
	info, ok := c.reg.info(table)
	if !ok || c.db == nil {
		return nil, nil
	}
	// Tables keyed by the auto-increment id are upserted with db.Save, i.e.
	// by id: the local row with that id is the one overwritten.
	if len(info.keyCols) == 0 {
		return nil, nil
	}
	q, err := keyWhere(c.db.WithContext(ctx).Unscoped(), info, entity)
	if err != nil {
		return nil, err
	}
	row, _ := c.reg.New(table)
	if row == nil {
		return nil, nil
	}
	res := q.Limit(1).Find(row)
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, nil
	}
	return row, nil
}
