package datasync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/internal/graphjob"
	"github.com/fengzhizi319/privahub/internal/service"
)

// ErrTableNotAllowed is returned for tables outside the accepted set.
var ErrTableNotAllowed = errors.New("table is not synchronized")

// Consumer applies SyncData to the local database (Java
// JpaSyncDataService.syncData / syncDataP2p + DataSyncConsumerTemplate).
type Consumer struct {
	db      *gorm.DB
	reg     *Registry
	allowed TableSet
	log     *zap.Logger
	// localNodeID is used by the graph-domain-datasource expansion (P2P).
	localNodeID string
	// identity / autonomy drive NodeManager.getTargetNodeIds in the
	// graph-domain-datasource expansion (AUTONOMY only).
	identity LocalIdentity
	autonomy bool
}

// SetLocalIdentity installs the local inst node set used by the AUTONOMY
// graph-domain-datasource expansion (Java NodeManager.getTargetNodeIds).
func (c *Consumer) SetLocalIdentity(id LocalIdentity, autonomy bool) *Consumer {
	c.identity = id
	c.autonomy = autonomy
	return c
}

// NewConsumer creates a consumer accepting only the allowed tables.
func NewConsumer(db *gorm.DB, reg *Registry, allowed TableSet, localNodeID string, log *zap.Logger) *Consumer {
	if reg == nil {
		reg = DefaultRegistry()
	}
	if log == nil {
		log = zap.NewNop()
	}
	return &Consumer{db: db, reg: reg, allowed: allowed, localNodeID: localNodeID, log: log}
}

// ConsumeMode selects the Java consumer variant.
type ConsumeMode int

const (
	// ModeEdge is JpaSyncDataService.syncData (EDGE receiving from CENTER).
	ModeEdge ConsumeMode = iota
	// ModeP2P is JpaSyncDataService.syncDataP2p (AUTONOMY peer data/sync).
	ModeP2P
)

// p2pRepublished are the tables whose P2P consumption is NOT marked as a
// consumer write, so the local producer may forward them (Java: VoteRequestDO,
// VoteInviteDO, ProjectApprovalConfigDO skip DataSyncConsumerContext).
var p2pRepublished = map[string]bool{
	TableVoteRequest:           true,
	TableVoteInvite:            true,
	TableProjectApprovalConfig: true,
}

// Apply decodes and applies one SyncData. It returns the decoded entity.
func (c *Consumer) Apply(ctx context.Context, mode ConsumeMode, d *service.SyncData) (interface{}, error) {
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

	switch mode {
	case ModeEdge:
		if c.ignoreEdge(table, entity) {
			c.log.Info("sync ignore", zap.String("table", table))
			return entity, nil
		}
		ctx = service.WithSyncConsumer(ctx)
	case ModeP2P:
		if !p2pRepublished[table] {
			ctx = service.WithSyncConsumer(ctx)
		}
		if table == TableProjectJob && c.localJobFinished(ctx, entity) {
			c.log.Info("ignore sync by local job is finished", zap.String("job_id", fieldString(reflect.ValueOf(entity), "JobID")))
			return entity, nil
		}
	}

	if action == service.SyncActionDelete {
		err = c.Delete(ctx, table, entity)
	} else {
		err = c.Upsert(ctx, table, entity)
	}
	if err != nil {
		return nil, err
	}
	if mode == ModeP2P && table == TableProjectGraphDomainDs && action == service.SyncActionCreate {
		c.expandGraphDomainDatasource(ctx, entity)
	}
	return entity, nil
}

// ignoreEdge mirrors JpaSyncDataService.ignore: executed/observer vote
// requests are not applied on the edge.
func (c *Consumer) ignoreEdge(table string, entity interface{}) bool {
	if table != TableVoteRequest {
		return false
	}
	st := strings.ToUpper(fieldString(reflect.ValueOf(entity), "ExecuteStatus"))
	return st == "SUCCESS" || st == "OBSERVER"
}

func (c *Consumer) localJobFinished(ctx context.Context, entity interface{}) bool {
	jobID := fieldString(reflect.ValueOf(entity), "JobID")
	if jobID == "" {
		return false
	}
	var job model.ProjectJobDO
	if err := c.db.WithContext(ctx).Where("job_id = ?", jobID).Limit(1).Find(&job).Error; err != nil || job.ID == 0 {
		return false
	}
	return graphjob.IsJobFinished(job.Status)
}

// expandGraphDomainDatasource mirrors DataSyncConsumerTemplate: a created
// graph domain datasource of a peer gets a default binding for the local node
// and, in AUTONOMY, for every target node of the source domain in the project
// (NodeManager.getTargetNodeIds). Existing bindings are kept.
func (c *Consumer) expandGraphDomainDatasource(ctx context.Context, entity interface{}) {
	src, ok := entity.(*model.ProjectGraphDomainDatasourceDO)
	if !ok {
		return
	}
	var targets []string
	if c.localNodeID != "" {
		targets = append(targets, c.localNodeID)
	}
	if c.autonomy {
		targets = append(targets, c.targetNodeIDs(ctx, src.DomainID, src.ProjectID)...)
	}
	for _, nodeID := range compact(targets) {
		edit := true
		local := &model.ProjectGraphDomainDatasourceDO{
			ProjectID: src.ProjectID, GraphID: src.GraphID, DomainID: nodeID,
			DatasourceID: defaultDataSource, DatasourceName: defaultDataSource, EditEnable: &edit,
		}
		var n int64
		c.db.WithContext(ctx).Model(&model.ProjectGraphDomainDatasourceDO{}).
			Where("project_id = ? AND graph_id = ? AND domain_id = ?", local.ProjectID, local.GraphID, local.DomainID).Count(&n)
		if n > 0 {
			continue
		}
		if err := c.db.WithContext(ctx).Create(local).Error; err != nil {
			c.log.Warn("create graph domain datasource failed", zap.String("domain", nodeID), zap.Error(err))
		}
	}
}

// defaultDataSource is Java KusciaDataSourceConstants.DEFAULT_DATA_SOURCE.
const defaultDataSource = "default-data-source"

// participantNodeInst is Java ParticipantNodeInstVO.
type participantNodeInst struct {
	InitiatorNodeID string `json:"initiatorNodeId"`
	Invitees        []struct {
		InviteeID string `json:"inviteeId"`
	} `json:"invitees"`
}

// targetNodeIDs mirrors NodeManager.getTargetNodeIds (AUTONOMY): for a node
// outside the local institution, the nodes it pairs with in the project's
// participant tree (initiator -> all invitees; invitee -> initiator).
func (c *Consumer) targetNodeIDs(ctx context.Context, nodeID, projectID string) []string {
	if nodeID == "" || projectID == "" || c.isLocalNode(nodeID) {
		return nil
	}
	var cfg model.ProjectApprovalConfigDO
	db := c.db.WithContext(ctx)
	if db.Where("project_id = ? AND type = ?", projectID, "PROJECT_CREATE").Limit(1).Find(&cfg).RowsAffected == 0 &&
		db.Where("project_id = ?", projectID).Order("id ASC").Limit(1).Find(&cfg).RowsAffected == 0 {
		return nil
	}
	var tree []participantNodeInst
	if strings.TrimSpace(cfg.ParticipantNodeInfo) == "" || json.Unmarshal([]byte(cfg.ParticipantNodeInfo), &tree) != nil {
		return nil
	}
	var out []string
	for _, vo := range tree {
		if vo.InitiatorNodeID == nodeID {
			for _, inv := range vo.Invitees {
				out = append(out, inv.InviteeID)
			}
			continue
		}
		for _, inv := range vo.Invitees {
			if inv.InviteeID == nodeID {
				out = append(out, vo.InitiatorNodeID)
				break
			}
		}
	}
	return compact(out)
}

// isLocalNode mirrors NodeManager.isLocalInstNode.
func (c *Consumer) isLocalNode(nodeID string) bool {
	if strings.EqualFold(nodeID, c.localNodeID) {
		return true
	}
	if c.identity == nil {
		return false
	}
	for _, id := range c.identity.NodeIDs() {
		if id == nodeID {
			return true
		}
	}
	return false
}

// setID zeroes (or sets) the auto-increment id of an entity.
func setID(entity interface{}, id int64) {
	v := reflect.ValueOf(entity)
	for v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	if f := v.FieldByName("ID"); f.IsValid() && f.CanSet() && f.Kind() == reflect.Int64 {
		f.SetInt(id)
	}
}

func keyWhere(db *gorm.DB, sch *entityInfo, entity interface{}) (*gorm.DB, error) {
	v := reflect.ValueOf(entity)
	for v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	stmt := &gorm.Statement{DB: db}
	if err := stmt.Parse(entity); err != nil {
		return nil, err
	}
	conds := map[string]interface{}{}
	for _, col := range sch.keyCols {
		f := stmt.Schema.LookUpField(col)
		if f == nil {
			return nil, fmt.Errorf("key column %s not found", col)
		}
		val, zero := f.ValueOf(context.Background(), v)
		if zero && f.DataType == "string" {
			return nil, fmt.Errorf("empty key column %s", col)
		}
		conds[col] = val
	}
	return db.Where(conds), nil
}

// Upsert inserts or updates entity by its business key, keeping the sender's
// timestamps (so edge checkpoints follow the center clock) and restoring a
// soft-deleted row. The remote auto-increment id is never used.
func (c *Consumer) Upsert(ctx context.Context, table string, entity interface{}) error {
	info, ok := c.reg.info(table)
	if !ok {
		return fmt.Errorf("%w: %s", ErrTableNotAllowed, table)
	}
	db := c.db.WithContext(ctx)
	if !info.hasPK || len(info.keyCols) == 0 || (len(info.keyCols) == 1 && info.keyCols[0] == "id") {
		return db.Save(entity).Error
	}
	if _, err := keyWhere(db, info, entity); err != nil {
		return err
	}
	setID(entity, 0)
	now := time.Now()
	v := reflect.ValueOf(entity).Elem()
	for _, name := range []string{"GmtCreate", "GmtModified"} {
		if f := v.FieldByName(name); f.IsValid() && f.CanSet() && f.Type() == timeType && f.Interface().(time.Time).IsZero() {
			f.Set(reflect.ValueOf(now))
		}
	}
	keys := make([]clause.Column, 0, len(info.keyCols))
	skip := map[string]bool{"id": true, "gmt_create": true}
	for _, k := range info.keyCols {
		keys = append(keys, clause.Column{Name: k})
		skip[k] = true
	}
	updates := make([]string, 0, len(info.columns))
	for _, col := range info.columns {
		if !skip[col] {
			updates = append(updates, col)
		}
	}
	return db.Clauses(clause.OnConflict{Columns: keys, DoUpdates: clause.AssignmentColumns(updates)}).Create(entity).Error
}

// Delete soft-deletes the rows matching the entity's business key.
func (c *Consumer) Delete(ctx context.Context, table string, entity interface{}) error {
	info, ok := c.reg.info(table)
	if !ok {
		return fmt.Errorf("%w: %s", ErrTableNotAllowed, table)
	}
	db := c.db.WithContext(ctx)
	q, err := keyWhere(db, info, entity)
	if err != nil {
		return err
	}
	zero, _ := c.reg.New(table)
	return q.Delete(zero).Error
}
