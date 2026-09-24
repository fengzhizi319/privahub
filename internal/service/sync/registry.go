// Package datasync implements SecretPad's cross-platform data synchronization:
//
//   - CENTER: GORM change hooks -> change buffer -> SSE push to connected
//     edges (Java EntityChangeListener + DbChangeEventListener + SseServer),
//     plus the incremental catch-up on connect (JpaSyncDataService.syncByLastUpdateTime).
//   - EDGE: SSE client with reconnect that upserts/deletes into the local DB
//     (Java EdgeSyncTask + EdgeDataSyncServiceImpl + JpaSyncDataService.syncData).
//   - AUTONOMY: GORM change hooks -> per-peer persistent queue -> POST
//     /api/v1alpha1/data/sync on the peer (Java P2pDataSyncProducerTemplate +
//     DataSyncJob + P2pDataSyncRestTemplate) and the consumer side
//     (DataSyncConsumerTemplate).
//   - vote_sync/create consumer (Java VoteSyncServiceImpl) and the
//     service.DbSyncSender implementation (Java DbSyncUtil).
package datasync

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync"

	"gorm.io/gorm"
	"gorm.io/gorm/schema"

	"github.com/fengzhizi319/privahub/internal/dao/model"
)

// syncModels are the entities that may take part in data synchronization
// (Java JpaSyncDataService.doAndRepository minus accounts/tokens, which are
// never accepted from a remote node).
func syncModels() []interface{} {
	return []interface{}{
		&model.InstDO{},
		&model.NodeDO{},
		&model.NodeRouteDO{},
		&model.ProjectDatatableDO{},
		&model.ProjectFedTableDO{},
		&model.ProjectGraphNodeDO{},
		&model.ProjectGraphDO{},
		&model.ProjectInstDO{},
		&model.ProjectJobDO{},
		&model.ProjectJobTaskLogDO{},
		&model.ProjectJobTaskDO{},
		&model.ProjectModelDO{},
		&model.ProjectNodeDO{},
		&model.ProjectReportDO{},
		&model.ProjectDO{},
		&model.ProjectResultDO{},
		&model.ProjectRuleDO{},
		&model.TeeDownloadApprovalConfigDO{},
		&model.TeeNodeDatatableManagementDO{},
		&model.VoteInviteDO{},
		&model.VoteRequestDO{},
		&model.NodeRouteApprovalConfigDO{},
		&model.ProjectApprovalConfigDO{},
		&model.ProjectGraphNodeKusciaParamsDO{},
		&model.ProjectModelServingDO{},
		&model.ProjectModelPackDO{},
		&model.ProjectFeatureTableDO{},
		&model.FeatureTableDO{},
		&model.ProjectGraphDomainDatasourceDO{},
		&model.ProjectScheduleDO{},
		&model.ProjectScheduleJobDO{},
		&model.ProjectScheduleTaskDO{},
		// Go-only table replacing the Java node-local MODEL_EXPORT_CACHE: Java
		// never syncs it (not in any data.sync list), so it is registered for
		// opt-in via sync.tables only and is absent from the defaults.
		&model.ProjectModelExportDO{},
	}
}

// javaAliases maps Java entity simple class names whose Go table name is not
// derivable from the class name.
var javaAliases = map[string]string{
	"ProjectTaskDO":            "project_job_task",
	"TeeDownLoadAuditConfigDO": "tee_download_approval_config",
	"AccountsDO":               "user_accounts",
	"TokensDO":                 "user_tokens",
}

// Tables names (Go).
const (
	TableNode                       = "node"
	TableNodeRoute                  = "node_route"
	TableProject                    = "project"
	TableProjectNode                = "project_node"
	TableProjectInst                = "project_inst"
	TableProjectGraph               = "project_graph"
	TableProjectGraphNode           = "project_graph_node"
	TableProjectGraphNodeKuscia     = "project_graph_node_kuscia_params"
	TableProjectJob                 = "project_job"
	TableProjectJobTask             = "project_job_task"
	TableProjectDatatable           = "project_datatable"
	TableVoteRequest                = "vote_request"
	TableVoteInvite                 = "vote_invite"
	TableTeeDownloadApprovalConfig  = "tee_download_approval_config"
	TableNodeRouteApprovalConfig    = "node_route_approval_config"
	TableTeeNodeDatatableManagement = "tee_node_datatable_management"
	TableProjectModelServing        = "project_model_serving"
	TableProjectModelPack           = "project_model_pack"
	TableFeatureTable               = "feature_table"
	TableProjectFeatureTable        = "project_feature_table"
	TableProjectGraphDomainDs       = "project_graph_domain_datasource"
	TableProjectSchedule            = "project_schedule"
	TableProjectScheduleJob         = "project_schedule_job"
	TableProjectScheduleTask        = "project_schedule_task"
	TableProjectApprovalConfig      = "project_approval_config"
	TableProjectModelExport         = "project_model_export"
)

// DefaultCenterTables is Java application.yaml / application-edge.yaml data.sync.
var DefaultCenterTables = []string{
	TableProject, TableProjectNode, TableNode, TableNodeRoute, TableProjectJob, TableProjectJobTask,
	TableProjectDatatable, TableVoteRequest, TableVoteInvite, TableTeeDownloadApprovalConfig,
	TableNodeRouteApprovalConfig, TableTeeNodeDatatableManagement, TableProjectModelServing,
	TableProjectGraphNodeKuscia, TableProjectModelPack, TableFeatureTable, TableProjectFeatureTable,
	TableProjectGraphDomainDs, TableProjectSchedule, TableProjectScheduleJob, TableProjectScheduleTask,
}

// DefaultP2PTables is Java application-p2p.yaml data.sync.
var DefaultP2PTables = []string{
	TableProject, TableProjectGraph, TableProjectGraphNode, TableProjectJob, TableProjectDatatable,
	TableVoteRequest, TableVoteInvite, TableProjectApprovalConfig, TableProjectNode,
	TableProjectGraphNodeKuscia, TableProjectModelServing, TableProjectModelPack,
	TableProjectFeatureTable, TableProjectGraphDomainDs, TableProjectInst, TableProjectSchedule,
	TableProjectScheduleJob, TableProjectScheduleTask,
}

// entityInfo describes one synchronizable table.
type entityInfo struct {
	table   string
	typ     reflect.Type // struct type
	keyCols []string     // business-key columns used for upsert/delete
	columns []string     // all DB columns
	hasPK   bool         // has an "id" primary key
	sch     *schema.Schema
}

// timeColumn returns the modification time column (gmt_modified, else gmt_create).
func (i *entityInfo) timeColumn() string {
	col := ""
	for _, c := range i.columns {
		if c == "gmt_modified" {
			return c
		}
		if c == "gmt_create" {
			col = c
		}
	}
	return col
}

// Registry maps table names to GORM models.
type Registry struct {
	byTable map[string]*entityInfo
	byJava  map[string]string // Java simple class name -> table
}

var (
	defaultRegistryOnce sync.Once
	defaultRegistry     *Registry
)

// DefaultRegistry returns the registry of all synchronizable models.
func DefaultRegistry() *Registry {
	defaultRegistryOnce.Do(func() {
		r, err := NewRegistry(syncModels()...)
		if err != nil {
			panic(err)
		}
		defaultRegistry = r
	})
	return defaultRegistry
}

// NewRegistry parses the GORM schemas of the given models.
func NewRegistry(models ...interface{}) (*Registry, error) {
	r := &Registry{byTable: map[string]*entityInfo{}, byJava: map[string]string{}}
	cache := &sync.Map{}
	for _, m := range models {
		sch, err := schema.Parse(m, cache, schema.NamingStrategy{})
		if err != nil {
			return nil, fmt.Errorf("parse schema %T: %w", m, err)
		}
		info := &entityInfo{table: sch.Table, typ: sch.ModelType, sch: sch}
		for _, f := range sch.Fields {
			if f.DBName != "" {
				info.columns = append(info.columns, f.DBName)
			}
		}
		if f := sch.LookUpField("id"); f != nil && f.PrimaryKey {
			info.hasPK = true
		}
		info.keyCols = businessKey(sch)
		r.byTable[sch.Table] = info
		r.byJava[sch.ModelType.Name()] = sch.Table
	}
	for java, table := range javaAliases {
		r.byJava[java] = table
	}
	return r, nil
}

// businessKey returns the columns of the first (by name) unique index that
// does not contain the primary key, else the primary key columns.
func businessKey(sch *schema.Schema) []string {
	idx := sch.ParseIndexes()
	sort.Slice(idx, func(i, j int) bool { return idx[i].Name < idx[j].Name })
	for _, ix := range idx {
		if !strings.EqualFold(ix.Class, "UNIQUE") && ix.Option != "UNIQUE" {
			continue
		}
		cols := make([]string, 0, len(ix.Fields))
		for _, f := range ix.Fields {
			if f.Field.PrimaryKey {
				cols = nil
				break
			}
			cols = append(cols, f.DBName)
		}
		if len(cols) > 0 {
			return cols
		}
	}
	for _, f := range sch.Fields {
		if f.Unique && !f.PrimaryKey && f.DBName != "" {
			return []string{f.DBName}
		}
	}
	cols := make([]string, 0, len(sch.PrimaryFields))
	for _, f := range sch.PrimaryFields {
		cols = append(cols, f.DBName)
	}
	return cols
}

// Canonical maps a table name (Go table, Java simple class name or Java
// fully-qualified class name) to the Go table name.
func (r *Registry) Canonical(name string) (string, bool) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", false
	}
	if _, ok := r.byTable[name]; ok {
		return name, true
	}
	simple := name
	if i := strings.LastIndex(name, "."); i >= 0 {
		simple = name[i+1:]
	}
	if t, ok := r.byJava[simple]; ok {
		if _, known := r.byTable[t]; known {
			return t, true
		}
	}
	return "", false
}

func (r *Registry) info(table string) (*entityInfo, bool) {
	i, ok := r.byTable[table]
	return i, ok
}

// KeyColumns returns the upsert/delete key columns of a table.
func (r *Registry) KeyColumns(table string) []string {
	if i, ok := r.byTable[table]; ok {
		return i.keyCols
	}
	return nil
}

// New returns a pointer to a new zero model of the table.
func (r *Registry) New(table string) (interface{}, bool) {
	i, ok := r.byTable[table]
	if !ok {
		return nil, false
	}
	return reflect.New(i.typ).Interface(), true
}

// TableSet is an immutable set of Go table names.
type TableSet map[string]bool

// ResolveTables canonicalizes names (unknown names are dropped and returned).
func (r *Registry) ResolveTables(names []string) (TableSet, []string) {
	set := TableSet{}
	var unknown []string
	for _, n := range names {
		if t, ok := r.Canonical(n); ok {
			set[t] = true
		} else if strings.TrimSpace(n) != "" {
			unknown = append(unknown, n)
		}
	}
	return set, unknown
}

// Has reports membership.
func (s TableSet) Has(t string) bool { return s[t] }

// List returns the sorted table names.
func (s TableSet) List() []string {
	out := make([]string, 0, len(s))
	for t := range s {
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}

// ---------------------------------------------------------------------------
// Routing (Java ProjectNodesInfo.getProjectId / getNodeIds)
// ---------------------------------------------------------------------------

// Route describes who an entity belongs to.
type Route struct {
	// Broadcast: every node receives it (Java NodeDO).
	Broadcast bool
	ProjectID string
	// NodeIDs are the explicit owners; when empty the project nodes are used.
	NodeIDs []string
}

func fieldString(v reflect.Value, name string) string {
	for v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return ""
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return ""
	}
	f := v.FieldByName(name)
	if !f.IsValid() {
		return ""
	}
	for f.Kind() == reflect.Ptr {
		if f.IsNil() {
			return ""
		}
		f = f.Elem()
	}
	if f.Kind() == reflect.String {
		return f.String()
	}
	return ""
}

// ParseIDList parses a JSON string array or a comma separated list.
func ParseIDList(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	var arr []string
	if strings.HasPrefix(s, "[") && json.Unmarshal([]byte(s), &arr) == nil {
		return compact(arr)
	}
	return compact(strings.Split(s, ","))
}

func compact(in []string) []string {
	out := make([]string, 0, len(in))
	seen := map[string]bool{}
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

// RouteOf returns the routing info of an entity of the given table.
func RouteOf(table string, entity interface{}) Route {
	v := reflect.ValueOf(entity)
	switch table {
	case TableNode:
		return Route{Broadcast: true}
	case TableNodeRoute:
		return Route{NodeIDs: compact([]string{fieldString(v, "SrcNodeID"), fieldString(v, "DstNodeID")})}
	case TableVoteRequest:
		ids := append([]string{fieldString(v, "Initiator")}, ParseIDList(fieldString(v, "Voters"))...)
		return Route{NodeIDs: compact(ids)}
	case TableVoteInvite:
		return Route{NodeIDs: compact([]string{fieldString(v, "Initiator"), fieldString(v, "VoteParticipantID")})}
	case TableTeeDownloadApprovalConfig, TableNodeRouteApprovalConfig:
		return Route{ProjectID: fieldString(v, "ProjectID"), NodeIDs: ParseIDList(fieldString(v, "AllParticipants"))}
	case TableProjectApprovalConfig:
		ids := append(ParseIDList(fieldString(v, "Parties")), fieldString(v, "InviteNodeID"))
		return Route{ProjectID: fieldString(v, "ProjectID"), NodeIDs: compact(ids)}
	case TableProjectJobTask:
		return Route{ProjectID: fieldString(v, "ProjectID")}
	case TableProjectGraphDomainDs:
		return Route{ProjectID: fieldString(v, "ProjectID"), NodeIDs: compact([]string{fieldString(v, "DomainID")})}
	}
	return Route{ProjectID: fieldString(v, "ProjectID"), NodeIDs: compact([]string{fieldString(v, "NodeID")})}
}

// projectNodeIDs returns the node ids of a project (Java
// projectNodeRepository.findProjectNodesByProjectId).
func projectNodeIDs(db *gorm.DB, projectID string) []string {
	if projectID == "" || db == nil {
		return nil
	}
	var ids []string
	_ = db.Model(&model.ProjectNodeDO{}).Where("project_id = ?", projectID).Pluck("node_id", &ids).Error
	return ids
}

// Visible reports whether nodeID may receive an entity with route r (Java
// JpaSyncDataService.SseSendFlag / DbChangeEventListener filter).
func Visible(db *gorm.DB, r Route, nodeID string) bool {
	if r.Broadcast {
		return true
	}
	ids := r.NodeIDs
	if len(ids) == 0 {
		ids = projectNodeIDs(db, r.ProjectID)
	}
	for _, id := range ids {
		if id == nodeID {
			return true
		}
	}
	return false
}
