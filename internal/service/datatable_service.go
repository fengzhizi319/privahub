package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/internal/dao/repository"
	"github.com/fengzhizi319/privahub/pkg/errcode"
	"github.com/fengzhizi319/privahub/pkg/kuscia"
	"github.com/google/uuid"
)

// Datatable service errors.
var (
	ErrDatatableNotFound = errors.New("datatable not found")
)

// Datatable constants (Java DomainDataConstants).
const (
	attrDatasourceType = "DatasourceType"
	attrDatasourceName = "DatasourceName"
	attrNullStrs       = "NullStrs"
	attrDescription    = "description"

	datatableTypeCSV  = "CSV"
	datatableTypeHTTP = "HTTP"
)

var odpsPartitionNameRe = regexp.MustCompile(`^[a-zA-Z0-9_]+$`)

// DatatableService implements Java DatatableServiceImpl and the per-type
// datatable handlers (LOCAL/OSS/ODPS/MYSQL -> Kuscia DomainData, HTTP ->
// feature_table), plus the TEE push/delete data-management jobs.
type DatatableService struct {
	datatableRepo repository.DatatableRepository
	fedTableRepo  repository.FedTableRepository
	db            *gorm.DB
	kusciaClient  *kuscia.Client
	env           *EnvService
	nodes         *NodeService
	oss           *OssService
	mysql         *MysqlChecker
	odps          *OdpsChecker
	ipFilter      *IPFilter
	teeConverter  *TeeJobConverter
	teeNodeID     string
	log           *zap.Logger
	stopOnce      chan struct{}
	pool          *kuscia.ClientPool
}

// NewDatatableService creates a new DatatableService.
func NewDatatableService(
	datatableRepo repository.DatatableRepository,
	fedTableRepo repository.FedTableRepository,
	db *gorm.DB,
	kusciaClient *kuscia.Client,
) *DatatableService {
	f := NewIPFilter(nil)
	return &DatatableService{
		datatableRepo: datatableRepo,
		fedTableRepo:  fedTableRepo,
		db:            db,
		kusciaClient:  kusciaClient,
		oss:           NewOssServiceWithFilter(f),
		mysql:         NewMysqlChecker(),
		odps:          NewOdpsChecker(),
		ipFilter:      f,
		teeConverter:  NewTeeJobConverter("", 1),
		teeNodeID:     "tee",
		log:           zap.NewNop(),
	}
}

// WithEnv sets the platform environment.
func (s *DatatableService) WithEnv(env *EnvService) *DatatableService { s.env = env; return s }

// WithIPFilter sets the SSRF filter (also used by the OSS checker).
func (s *DatatableService) WithIPFilter(f *IPFilter) *DatatableService {
	if f == nil {
		f = NewDisabledIPFilter()
	}
	s.ipFilter = f
	s.oss = NewOssServiceWithFilter(f)
	return s
}

// WithCheckers overrides the OSS/MySQL/ODPS existence checkers (nil keeps current).
func (s *DatatableService) WithCheckers(o *OssService, m *MysqlChecker, d *OdpsChecker) *DatatableService {
	if o != nil {
		s.oss = o
	}
	if m != nil {
		s.mysql = m
	}
	if d != nil {
		s.odps = d
	}
	return s
}

// WithTee configures the TEE domain id and job converter.
func (s *DatatableService) WithTee(teeNodeID, capsuleManager string, maxParallelism int32) *DatatableService {
	if teeNodeID != "" {
		s.teeNodeID = teeNodeID
	}
	s.teeConverter = NewTeeJobConverter(capsuleManager, maxParallelism)
	return s
}

// WithLogger sets the logger.
func (s *DatatableService) WithLogger(l *zap.Logger) *DatatableService {
	if l != nil {
		s.log = l
	}
	return s
}

// TeeConverter exposes the TEE job converter (for TEE pull/auth flows).
func (s *DatatableService) TeeConverter() *TeeJobConverter { return s.teeConverter }

// --- Legacy Go-only DTOs (/datatable/register, /detail, /grant, /fed/create) ---

// RegisterDatatableRequest represents a datatable registration request.
type RegisterDatatableRequest struct {
	ProjectID    string `json:"project_id" binding:"required"`
	NodeID       string `json:"node_id" binding:"required"`
	DatatableID  string `json:"datatable_id"`
	TableConfigs string `json:"table_configs"`
	Source       string `json:"source"`
}

// LegacyProjectDatatableVO is the Go-only project_datatable view object.
type LegacyProjectDatatableVO struct {
	ProjectID    string `json:"project_id"`
	NodeID       string `json:"node_id"`
	DatatableID  string `json:"datatable_id"`
	TableConfigs string `json:"table_configs"`
	Source       string `json:"source"`
	GmtCreate    string `json:"gmt_create"`
}

// ProjectDatatableDetailRequest is the Go-only /datatable/detail request.
type ProjectDatatableDetailRequest struct {
	ProjectID   string `json:"project_id" binding:"required"`
	NodeID      string `json:"node_id" binding:"required"`
	DatatableID string `json:"datatable_id" binding:"required"`
}

// GrantDatatableRequest represents a datatable grant request (Go-only).
type GrantDatatableRequest struct {
	ProjectID   string `json:"project_id"`
	NodeID      string `json:"node_id" binding:"required"`
	DatatableID string `json:"datatable_id" binding:"required"`
	TargetNode  string `json:"target_node" binding:"required"`
}

// CreateFedTableRequest represents a federated table creation request.
type CreateFedTableRequest struct {
	ProjectID string    `json:"project_id" binding:"required"`
	Joins     []FedJoin `json:"joins" binding:"required"`
}

// FedJoin represents a single join entry in a federated table.
type FedJoin struct {
	NodeID      string `json:"node_id"`
	DatatableID string `json:"datatable_id"`
}

// FedTableVO represents a federated table view object.
type FedTableVO struct {
	FedTableID string `json:"fed_table_id"`
	ProjectID  string `json:"project_id"`
	Joins      string `json:"joins"`
	GmtCreate  string `json:"gmt_create"`
}

// --- Java DTOs (frontend contract, camelCase) ---

// TableColumnCompat is Java TableColumnVO.
type TableColumnCompat struct {
	ColName        string `json:"colName"`
	ColType        string `json:"colType"`
	ColComment     string `json:"colComment"`
	Classification string `json:"classification,omitempty"`
}

// TableColumnVO is the Java name of TableColumnCompat.
type TableColumnVO = TableColumnCompat

// OdpsPartitionField is Java OdpsPartitionRequest.Field.
type OdpsPartitionField struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	Comment string `json:"comment"`
}

// OdpsPartitionRequest is Java OdpsPartitionRequest (type: odps / path).
type OdpsPartitionRequest struct {
	Type   string               `json:"type"`
	Fields []OdpsPartitionField `json:"fields,omitempty"`
}

// CreateDatatableCompatRequest is Java CreateDatatableRequest.
type CreateDatatableCompatRequest struct {
	OwnerID        string                `json:"ownerId"`
	NodeIDs        []string              `json:"nodeIds"`
	DatatableName  string                `json:"datatableName"`
	DatasourceID   string                `json:"datasourceId"`
	DatasourceName string                `json:"datasourceName"`
	DatasourceType string                `json:"datasourceType"`
	Desc           string                `json:"desc"`
	RelativeURI    string                `json:"relativeUri"`
	Columns        []TableColumnCompat   `json:"columns"`
	Partition      *OdpsPartitionRequest `json:"partition,omitempty"`
	NullStrs       []string              `json:"nullStrs"`
}

// CreateDatatableRequest is the Java name.
type CreateDatatableRequest = CreateDatatableCompatRequest

// DataTableNodeInfo is Java CreateDatatableVO.DataTableNodeInfo.
type DataTableNodeInfo struct {
	NodeID       string `json:"nodeId"`
	DomainDataID string `json:"domainDataId"`
}

// CreateDatatableCompatVO is Java CreateDatatableVO.
type CreateDatatableCompatVO struct {
	DataTableNodeInfos []DataTableNodeInfo `json:"dataTableNodeInfos"`
	FailedCreatedNodes map[string]string   `json:"failedCreatedNodes"`
}

// ListDatatableRequest is Java ListDatatableRequest.
type ListDatatableRequest struct {
	PageSize            int      `json:"pageSize"`
	PageNumber          int      `json:"pageNumber"`
	NodeNamesFilter     []string `json:"nodeNamesFilter"`
	StatusFilter        string   `json:"statusFilter"`
	DatatableNameFilter *string  `json:"datatableNameFilter"`
	Types               []string `json:"types"`
	OwnerID             string   `json:"ownerId"`
	TeeNodeID           string   `json:"teeNodeId"`
}

// AuthProjectVO is Java AuthProjectVO.
type AuthProjectVO struct {
	ProjectID     string   `json:"projectId"`
	Name          string   `json:"name"`
	ComputeMode   string   `json:"computeMode"`
	AssociateKeys []string `json:"associateKeys"`
	GroupKeys     []string `json:"groupKeys"`
	LabelKeys     []string `json:"labelKeys"`
	GmtCreate     string   `json:"gmtCreate"`
}

// DatatableVO is Java DatatableVO.
type DatatableVO struct {
	DatatableID     string                `json:"datatableId"`
	DatatableName   string                `json:"datatableName"`
	Status          string                `json:"status"`
	PushToTeeStatus string                `json:"pushToTeeStatus"`
	PushToTeeErrMsg string                `json:"pushToTeeErrMsg"`
	DatasourceID    string                `json:"datasourceId"`
	DatasourceType  string                `json:"datasourceType"`
	DatasourceName  string                `json:"datasourceName"`
	NodeID          string                `json:"nodeId"`
	RelativeURI     string                `json:"relativeUri"`
	Type            string                `json:"type"`
	Description     string                `json:"description"`
	Schema          []TableColumnCompat   `json:"schema"`
	AuthProjects    []AuthProjectVO       `json:"authProjects,omitempty"`
	Partition       *OdpsPartitionRequest `json:"partition,omitempty"`
	NullStrs        []string              `json:"nullStrs,omitempty"`
}

// DatatableCompatVO is kept as an alias for existing callers.
type DatatableCompatVO = DatatableVO

// DatatableNodeVO is Java DatatableNodeVO.
type DatatableNodeVO struct {
	DatatableVO *DatatableVO `json:"datatableVO"`
	NodeName    string       `json:"nodeName"`
	NodeID      string       `json:"nodeId"`
}

// DatatableNodeCompatVO is kept as an alias for existing callers.
type DatatableNodeCompatVO = DatatableNodeVO

// AllDatatableListVO is Java AllDatatableListVO.
type AllDatatableListVO struct {
	DatatableNodeVOList []DatatableNodeVO `json:"datatableNodeVOList"`
	TotalDatatableNums  int               `json:"totalDatatableNums"`
}

// GetDatatableCompatRequest is Java GetDatatableRequest.
type GetDatatableCompatRequest struct {
	NodeID         string `json:"nodeId"`
	DatatableID    string `json:"datatableId"`
	DatasourceType string `json:"datasourceType"`
	Type           string `json:"type"`
	TeeNodeID      string `json:"teeNodeId"`
}

// GetDatatableRequest is the Java name.
type GetDatatableRequest = GetDatatableCompatRequest

// DeleteDatatableRequest is Java DeleteDatatableRequest (no project id).
type DeleteDatatableRequest struct {
	NodeID         string `json:"nodeId"`
	DatatableID    string `json:"datatableId"`
	TeeNodeID      string `json:"teeNodeId"`
	DatasourceID   string `json:"datasourceId"`
	DatasourceType string `json:"datasourceType"`
	RelativeURI    string `json:"relativeUri"`
	Type           string `json:"type"`
}

// PushDatatableToTeeRequest is Java PushDatatableToTeeRequest.
type PushDatatableToTeeRequest struct {
	NodeID       string `json:"nodeId" binding:"required"`
	DatatableID  string `json:"datatableId" binding:"required"`
	TeeNodeID    string `json:"teeNodeId"`
	DatasourceID string `json:"datasourceId"`
	RelativeURI  string `json:"relativeUri"`
}

// datatableDTO is Java DatatableDTO.
type datatableDTO struct {
	NodeID         string
	DatatableID    string
	DatatableName  string
	RelativeURI    string
	DatasourceID   string
	DatasourceType string
	DatasourceName string
	Attributes     map[string]string
	Status         string
	Type           string
	Schema         []TableColumnCompat
	Partition      *kuscia.Partition
}

func fromDomainData(d *kuscia.DomainData) *datatableDTO {
	attrs := d.Attributes
	if attrs == nil {
		attrs = map[string]string{}
	}
	dsType := attrs[attrDatasourceType]
	// data/create writes the upload's Kuscia type ("localfs", Java
	// DEFAULT_DATASOURCE_TYPE); the platform type is LOCAL.
	if dsType == "" || strings.EqualFold(dsType, DefaultLocalKusciaType) {
		dsType = DatasourceTypeLocal
	}
	dsName := attrs[attrDatasourceName]
	if dsName == "" {
		dsName = DefaultLocalDatasourceNm
	}
	schema := make([]TableColumnCompat, 0, len(d.Columns))
	for _, c := range d.Columns {
		schema = append(schema, TableColumnCompat{ColName: c.Name, ColType: c.Type, ColComment: c.Comment})
	}
	nodeID := d.Author
	if nodeID == "" {
		nodeID = d.DomainID
	}
	return &datatableDTO{
		NodeID: nodeID, DatatableID: d.DomainDataID, DatatableName: d.Name, RelativeURI: d.RelativeURI,
		DatasourceID: d.DatasourceID, DatasourceType: dsType, DatasourceName: dsName, Attributes: attrs,
		Status: d.Status, Type: d.Type, Schema: schema, Partition: d.Partition,
	}
}

func parseColumns(s string) []TableColumnCompat {
	out := make([]TableColumnCompat, 0)
	if strings.TrimSpace(s) == "" {
		return out
	}
	_ = json.Unmarshal([]byte(s), &out)
	return out
}

func fromFeatureTable(ft *model.FeatureTableDO) *datatableDTO {
	return &datatableDTO{
		NodeID: ft.NodeID, DatatableID: ft.FeatureTableID, DatatableName: ft.FeatureTableName,
		RelativeURI: ft.URL, DatasourceID: DefaultHTTPDatasourceID, DatasourceType: DatasourceTypeHTTP,
		DatasourceName: DefaultHTTPDatasourceName, Status: ft.Status, Type: datatableTypeHTTP,
		Schema: parseColumns(ft.Columns),
	}
}

// toVO mirrors Java DatatableVO.from.
func (d *datatableDTO) toVO(auth []AuthProjectVO, tee *model.TeeNodeDatatableManagementDO) *DatatableVO {
	t := d.Type
	if strings.EqualFold(t, kuscia.DomainDataTypeTable) {
		t = datatableTypeCSV
	}
	vo := &DatatableVO{
		DatatableID: d.DatatableID, DatatableName: d.DatatableName, Status: d.Status,
		DatasourceID: d.DatasourceID, RelativeURI: d.RelativeURI, Type: t,
		Description: d.Attributes[attrDescription], DatasourceType: d.DatasourceType,
		DatasourceName: d.DatasourceName, Schema: d.Schema, AuthProjects: auth, NodeID: d.NodeID,
	}
	if vo.Schema == nil {
		vo.Schema = []TableColumnCompat{}
	}
	if d.Partition != nil {
		p := &OdpsPartitionRequest{Type: d.Partition.Type}
		for _, f := range d.Partition.Fields {
			p.Fields = append(p.Fields, OdpsPartitionField{Name: f.Name, Type: f.Type, Comment: f.Comment})
		}
		vo.Partition = p
	}
	if tee != nil {
		vo.PushToTeeStatus = tee.Status
		vo.PushToTeeErrMsg = tee.ErrMsg
	}
	if raw, ok := d.Attributes[attrNullStrs]; ok && raw != "" {
		var ns []string
		if json.Unmarshal([]byte(raw), &ns) == nil {
			vo.NullStrs = ns
		}
	}
	return vo
}

// --- helpers ---

func (s *DatatableService) requireKuscia() error {
	if s.kusciaClient == nil {
		return bizErr(errcode.KusciaRpcError, "kuscia client not configured")
	}
	return nil
}

func (s *DatatableService) teeDomain(req string) string {
	if strings.TrimSpace(req) != "" {
		return req
	}
	return s.teeNodeID
}

func (s *DatatableService) nodeName(ctx context.Context, nodeID string) string {
	var n model.NodeDO
	if s.db != nil && s.db.WithContext(ctx).Where("node_id = ?", nodeID).First(&n).Error == nil {
		return n.Name
	}
	return ""
}

// tableColumnConfig is one entry of project_datatable.table_configs.
type tableColumnConfig struct {
	ColName        string `json:"colName"`
	IsAssociateKey bool   `json:"isAssociateKey"`
	IsGroupKey     bool   `json:"isGroupKey"`
	IsLabelKey     bool   `json:"isLabelKey"`
}

func authProjectFrom(p *model.ProjectDO, tableConfigs string, gmt time.Time) AuthProjectVO {
	vo := AuthProjectVO{
		ProjectID: p.ProjectID, Name: p.Name, ComputeMode: p.ComputeMode,
		AssociateKeys: []string{}, GroupKeys: []string{}, LabelKeys: []string{},
		GmtCreate: gmt.Format(time.RFC3339),
	}
	var cols []tableColumnConfig
	if json.Unmarshal([]byte(tableConfigs), &cols) == nil {
		for _, c := range cols {
			if c.IsAssociateKey {
				vo.AssociateKeys = append(vo.AssociateKeys, c.ColName)
			}
			if c.IsGroupKey {
				vo.GroupKeys = append(vo.GroupKeys, c.ColName)
			}
			if c.IsLabelKey {
				vo.LabelKeys = append(vo.LabelKeys, c.ColName)
			}
		}
	}
	return vo
}

type authRow struct {
	ProjectID    string
	DatatableID  string
	TableConfigs string
	GmtCreate    time.Time
}

func (s *DatatableService) authPairs(ctx context.Context, rows []authRow) map[string][]AuthProjectVO {
	out := map[string][]AuthProjectVO{}
	if len(rows) == 0 || s.db == nil {
		return out
	}
	ids := make([]string, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.ProjectID)
	}
	var projects []model.ProjectDO
	s.db.WithContext(ctx).Where("project_id IN ?", distinctStrings(ids)).Find(&projects)
	pm := map[string]*model.ProjectDO{}
	for i := range projects {
		pm[projects[i].ProjectID] = &projects[i]
	}
	for _, r := range rows {
		p, ok := pm[r.ProjectID]
		if !ok {
			continue
		}
		out[r.DatatableID] = append(out[r.DatatableID], authProjectFrom(p, r.TableConfigs, r.GmtCreate))
	}
	return out
}

// datatableAuthProjects mirrors Java getAuthProjectPairs (project_datatable).
func (s *DatatableService) datatableAuthProjects(ctx context.Context, nodeID string, ids []string) map[string][]AuthProjectVO {
	if len(ids) == 0 || s.db == nil {
		return map[string][]AuthProjectVO{}
	}
	var pds []model.ProjectDatatableDO
	s.db.WithContext(ctx).Where("node_id = ? AND datatable_id IN ?", nodeID, ids).Find(&pds)
	rows := make([]authRow, 0, len(pds))
	for _, p := range pds {
		rows = append(rows, authRow{p.ProjectID, p.DatatableID, p.TableConfigs, p.GmtCreate})
	}
	return s.authPairs(ctx, rows)
}

// featureAuthProjects mirrors Java getHttpFeatureAuthProjectPairs (project_feature_table).
func (s *DatatableService) featureAuthProjects(ctx context.Context, nodeID string, ids []string) map[string][]AuthProjectVO {
	if len(ids) == 0 || s.db == nil {
		return map[string][]AuthProjectVO{}
	}
	var pfs []model.ProjectFeatureTableDO
	s.db.WithContext(ctx).Where("node_id = ? AND feature_table_id IN ?", nodeID, ids).Find(&pfs)
	rows := make([]authRow, 0, len(pfs))
	for _, p := range pfs {
		rows = append(rows, authRow{p.ProjectID, p.FeatureTableID, p.TableConfigs, p.GmtCreate})
	}
	return s.authPairs(ctx, rows)
}

// pushToTeeInfos returns the latest Push record per tee datatable id.
func (s *DatatableService) pushToTeeInfos(ctx context.Context, nodeID, teeNodeID string, ids []string) map[string]*model.TeeNodeDatatableManagementDO {
	out := map[string]*model.TeeNodeDatatableManagementDO{}
	if len(ids) == 0 || s.db == nil {
		return out
	}
	teeIDs := make([]string, 0, len(ids))
	for _, id := range ids {
		teeIDs = append(teeIDs, BuildTeeDatatableID(teeNodeID, id))
	}
	var list []model.TeeNodeDatatableManagementDO
	s.db.WithContext(ctx).Where("node_id = ? AND tee_node_id = ? AND datatable_id IN ? AND kind = ?",
		nodeID, teeNodeID, teeIDs, TeeJobKindPush).Order("gmt_create DESC").Find(&list)
	for i := range list {
		if _, ok := out[list[i].DatatableID]; !ok {
			out[list[i].DatatableID] = &list[i]
		}
	}
	return out
}

// kusciaDatatables lists the manually registered tables of a node in Kuscia
// (Java DatatableManager.findByNodeId(nodeId, "manual")). An unregistered
// domain yields an empty list.
func (s *DatatableService) kusciaDatatables(ctx context.Context, nodeID string) ([]*datatableDTO, error) {
	if err := s.requireKuscia(); err != nil {
		return nil, err
	}
	list, err := s.kc(nodeID).ListDomainDataWithFilter(ctx, &kuscia.ListDomainDataRequestData{
		DomainID: nodeID, DomainDataType: kuscia.DomainDataTypeTable, DomainDataVendor: kuscia.DomainDataVendorManual,
	})
	if err != nil {
		if kuscia.IsNotFound(err) {
			return []*datatableDTO{}, nil
		}
		return nil, bizWrap(errcode.DatatableQueryDatatableFailed, err, "")
	}
	out := make([]*datatableDTO, 0, len(list))
	for i := range list {
		out = append(out, fromDomainData(&list[i]))
	}
	return out, nil
}

func (s *DatatableService) featureDatatables(ctx context.Context, nodeID string) []*datatableDTO {
	out := make([]*datatableDTO, 0)
	if s.db == nil {
		return out
	}
	var fts []model.FeatureTableDO
	s.db.WithContext(ctx).Where("node_id = ?", nodeID).Order("gmt_create DESC").Find(&fts)
	for i := range fts {
		out = append(out, fromFeatureTable(&fts[i]))
	}
	return out
}

// FindDatatablesByNodeID mirrors Java findDatatableByNodeId: HTTP feature
// tables + Kuscia manual tables of a node (used by datasource relatedDatas).
func (s *DatatableService) FindDatatablesByNodeID(ctx context.Context, nodeID string) ([]DatatableVO, error) {
	dtos := s.featureDatatables(ctx, nodeID)
	kd, err := s.kusciaDatatables(ctx, nodeID)
	if err != nil {
		return nil, err
	}
	dtos = append(dtos, kd...)
	out := make([]DatatableVO, 0, len(dtos))
	for _, d := range dtos {
		out = append(out, DatatableVO{
			DatatableID: d.DatatableID, DatatableName: d.DatatableName, Type: d.Type,
			DatasourceID: d.DatasourceID, NodeID: d.NodeID,
		})
	}
	return out, nil
}

// --- Create ---

// CreateDatatableCompat mirrors Java createDataTable + the per-type handlers.
func (s *DatatableService) CreateDatatableCompat(ctx context.Context, req *CreateDatatableCompatRequest) (*CreateDatatableCompatVO, error) {
	req.DatasourceType = strings.ToUpper(strings.TrimSpace(req.DatasourceType))
	if err := validateCreateDatatable(req); err != nil {
		return nil, err
	}
	if s.env != nil && s.env.IsAutonomy() {
		// Java createDataTable: AUTONOMY nodes must belong to the owner institution.
		req.NodeIDs = distinctStrings(req.NodeIDs)
		if s.db != nil && !checkNodesInInst(ctx, s.db, req.OwnerID, req.NodeIDs) {
			return nil, bizErr(errcode.InstNotMatchNode, "")
		}
	}
	if req.DatasourceType == DatasourceTypeHTTP {
		return s.createHTTPDatatable(ctx, req)
	}
	if req.DatasourceType == DatasourceTypeLocal && s.env != nil && s.env.IsAutonomy() &&
		!s.env.IsCurrentNodeEnvironment(req.NodeIDs[0]) {
		return nil, bizErr(errcode.NodeNotExistError, "The nodeId is not the platform node id.")
	}
	var partition *kuscia.Partition
	if req.DatasourceType == DatasourceTypeODPS && req.Partition != nil {
		partition = &kuscia.Partition{Type: req.Partition.Type}
		for _, f := range req.Partition.Fields {
			if !odpsPartitionNameRe.MatchString(f.Name) {
				return nil, bizErr(errcode.ParamError, "Partition name must match [a-zA-Z0-9_]+")
			}
			partition.Fields = append(partition.Fields, kuscia.DataColumn{Name: f.Name, Type: f.Type, Comment: f.Comment})
		}
	}
	if err := s.requireKuscia(); err != nil {
		return nil, err
	}
	nullStrs := req.NullStrs
	if nullStrs == nil {
		nullStrs = []string{}
	}
	nullJSON, _ := json.Marshal(nullStrs)
	columns := make([]kuscia.DataColumn, 0, len(req.Columns))
	for _, c := range req.Columns {
		columns = append(columns, kuscia.DataColumn{Name: c.ColName, Type: c.ColType, Comment: c.ColComment})
	}

	vo := &CreateDatatableCompatVO{DataTableNodeInfos: []DataTableNodeInfo{}, FailedCreatedNodes: map[string]string{}}
	var mu sync.Mutex
	failed := fanOut(ctx, req.NodeIDs, func(cctx context.Context, nodeID string) error {
		id, err := s.kc(nodeID).CreateDomainDataWithID(cctx, &kuscia.CreateDomainDataRequest{
			DomainDataID: randomLetters(8),
			DomainID:     nodeID,
			Name:         req.DatatableName,
			Type:         kuscia.DomainDataTypeTable,
			FileFormat:   kuscia.FileFormatCSV,
			Attributes: map[string]string{
				attrDatasourceType: req.DatasourceType,
				attrDatasourceName: req.DatasourceName,
				attrNullStrs:       string(nullJSON),
				attrDescription:    req.Desc,
			},
			DatasourceID: req.DatasourceID,
			RelativeURI:  req.RelativeURI,
			Columns:      columns,
			Partition:    partition,
		})
		if err != nil {
			return err
		}
		mu.Lock()
		vo.DataTableNodeInfos = append(vo.DataTableNodeInfos, DataTableNodeInfo{NodeID: nodeID, DomainDataID: id})
		mu.Unlock()
		return nil
	})
	vo.FailedCreatedNodes = failed
	sort.Slice(vo.DataTableNodeInfos, func(i, j int) bool { return vo.DataTableNodeInfos[i].NodeID < vo.DataTableNodeInfos[j].NodeID })
	if len(failed) > 0 {
		s.log.Info("some nodes failed to create datatable", zap.Any("failed", failed))
		// Java logFailedDatatable intends "all nodes failed" -> error.
		if len(failed) == len(req.NodeIDs) {
			return nil, bizErr(errcode.DatatableCreateFailed, "All nodes create datatable failed")
		}
	}
	return vo, nil
}

func validateCreateDatatable(req *CreateDatatableCompatRequest) error {
	switch {
	case req.OwnerID == "":
		return bizErr(errcode.ParamError, "ownerId cannot be blank")
	case len(req.NodeIDs) == 0:
		return bizErr(errcode.ParamError, "nodeIds cannot be empty")
	case strings.TrimSpace(req.DatatableName) == "":
		return bizErr(errcode.ParamError, "datatableName cannot be blank")
	case len([]rune(req.DatatableName)) > 32:
		return bizErr(errcode.ParamError, "feature table must less then 32 characters")
	case len([]rune(req.Desc)) > 100:
		return bizErr(errcode.ParamError, "feature table desc must less then 100 characters")
	case strings.TrimSpace(req.RelativeURI) == "":
		return bizErr(errcode.ParamError, "relativeUri cannot be blank")
	case len(req.Columns) == 0:
		return bizErr(errcode.ParamError, "columns cannot be empty")
	}
	for _, n := range req.NodeIDs {
		if strings.TrimSpace(n) == "" {
			return bizErr(errcode.ParamError, "nodeIds cannot contain blank node id")
		}
	}
	switch req.DatasourceType {
	case DatasourceTypeOSS, DatasourceTypeODPS, DatasourceTypeMySQL, DatasourceTypeLocal:
		if req.DatasourceID == "" || req.DatasourceName == "" {
			return bizErr(errcode.ParamError, "datasourceId/datasourceName cannot be blank")
		}
	case DatasourceTypeHTTP:
	default:
		return bizErr(errcode.ParamError, "datasourceType must be one of OSS/ODPS/MYSQL/HTTP/LOCAL")
	}
	return nil
}

// createHTTPDatatable mirrors Java HttpDatatableHandler.createDatatable.
func (s *DatatableService) createHTTPDatatable(ctx context.Context, req *CreateDatatableCompatRequest) (*CreateDatatableCompatVO, error) {
	if err := s.ipFilter.CheckURL(req.RelativeURI); err != nil {
		return nil, err
	}
	cols, _ := json.Marshal(req.Columns)
	dsID := req.DatasourceID
	if dsID == "" {
		dsID = DefaultHTTPDatasourceID
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, n := range req.NodeIDs {
			if err := tx.Create(&model.FeatureTableDO{
				FeatureTableID: randomLetters(8), FeatureTableName: req.DatatableName, NodeID: n,
				DatasourceID: dsID, Type: req.DatasourceType, Description: req.Desc, URL: req.RelativeURI,
				Columns: string(cols), Status: StatusUnavailable,
			}).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, bizWrap(errcode.DatatableCreateFailed, err, "Failed to save feature tables")
	}
	return &CreateDatatableCompatVO{DataTableNodeInfos: []DataTableNodeInfo{}, FailedCreatedNodes: map[string]string{}}, nil
}

// --- List ---

// listNodeIDs returns the nodes to aggregate (Java getNodeIds): AUTONOMY ->
// the Ready nodes of the local institution (Java InstServiceImpl.INST_ID)
// filtered by node name; otherwise ownerId.
func (s *DatatableService) listNodeIDs(ctx context.Context, req *ListDatatableRequest) []string {
	if s.env != nil && s.env.IsAutonomy() && s.db != nil {
		instID := s.env.GetInstID()
		if instID == "" {
			instID = req.OwnerID
		}
		if s.nodes != nil {
			return s.nodes.ReadyInstNodeIDsByNames(ctx, instID, req.NodeNamesFilter)
		}
		q := s.db.WithContext(ctx).Model(&model.NodeDO{}).Where("inst_id = ?", instID)
		if len(req.NodeNamesFilter) > 0 {
			q = q.Where("name IN ?", req.NodeNamesFilter)
		}
		ids := []string{}
		q.Order("id").Pluck("node_id", &ids)
		return distinctStrings(ids)
	}
	return []string{req.OwnerID}
}

// WithNodeService lets AUTONOMY listing use the Kuscia Ready state of the
// institution's nodes (Java nodeManager.listReadyNodeByNames).
func (s *DatatableService) WithNodeService(ns *NodeService) *DatatableService { s.nodes = ns; return s }

// datatableNodeTimeout bounds each node's Kuscia query in list (Java waits
// 5s for the per-node futures).
const datatableNodeTimeout = 5 * time.Second

// ListDatatablesByOwnerID mirrors Java listDatatablesByOwnerId: per-node
// Kuscia domaindata (+HTTP feature tables), filtered, with authProjects and
// pushToTee status, paginated over the aggregate.
func (s *DatatableService) ListDatatablesByOwnerID(ctx context.Context, req *ListDatatableRequest) (*AllDatatableListVO, error) {
	if req.OwnerID == "" {
		return nil, bizErr(errcode.ParamError, "ownerId cannot be blank")
	}
	nodeIDs := s.listNodeIDs(ctx, req)
	var mu sync.Mutex
	perNode := map[string][]DatatableVO{}
	totals := map[string]int{}
	// A node whose KusciaAPI fails or times out contributes no Kuscia tables
	// (its HTTP feature tables are still listed) instead of failing the
	// whole call: Java's per-node futures intend the same ("单个节点异常不能
	// 影响整体结果"), although its null-result path ends in a
	// TASK_EXECUTION_ERROR.
	failed := fanOut(ctx, nodeIDs, func(cctx context.Context, nodeID string) error {
		nctx, cancel := context.WithTimeout(cctx, datatableNodeTimeout)
		defer cancel()
		list, kerr := s.listDatatablesByNodeID(nctx, nodeID, req)
		mu.Lock()
		perNode[nodeID] = list
		totals[nodeID] = len(list)
		mu.Unlock()
		return kerr
	})
	if len(failed) > 0 {
		s.log.Warn("list datatable: kuscia query failed on some nodes, returning partial data", zap.Any("failed", failed))
	}
	all := make([]DatatableNodeVO, 0)
	total := 0
	for _, nodeID := range nodeIDs {
		name := s.nodeName(ctx, nodeID)
		if name == "" {
			name = nodeID
		}
		total += totals[nodeID]
		for i := range perNode[nodeID] {
			all = append(all, DatatableNodeVO{DatatableVO: &perNode[nodeID][i], NodeID: nodeID, NodeName: name})
		}
	}
	size, page := req.PageSize, req.PageNumber
	if page <= 0 {
		page = 1
	}
	if size <= 0 {
		size = len(all)
	}
	start := size * (page - 1)
	if start > len(all) {
		return nil, bizErr(errcode.SystemOutOfRangeError, "page start index > dataList length.")
	}
	end := start + size
	if end > len(all) {
		end = len(all)
	}
	return &AllDatatableListVO{DatatableNodeVOList: all[start:end], TotalDatatableNums: total}, nil
}

// listDatatablesByNodeID lists one node's tables. A Kuscia failure is
// returned alongside the tables still known locally (HTTP feature tables).
func (s *DatatableService) listDatatablesByNodeID(ctx context.Context, nodeID string, req *ListDatatableRequest) ([]DatatableVO, error) {
	dtos, kerr := s.kusciaDatatables(ctx, nodeID)
	if kerr != nil {
		dtos = nil
	}
	dtos = append(dtos, s.featureDatatables(ctx, nodeID)...)
	// de-duplicate by id keeping the first
	seen := map[string]bool{}
	filtered := make([]*datatableDTO, 0, len(dtos))
	for _, d := range dtos {
		if seen[d.DatatableID] {
			continue
		}
		seen[d.DatatableID] = true
		if (strings.EqualFold(req.StatusFilter, StatusAvailable) || strings.EqualFold(req.StatusFilter, StatusUnavailable)) &&
			!strings.EqualFold(req.StatusFilter, d.Status) {
			continue
		}
		if req.DatatableNameFilter != nil && !strings.Contains(d.DatatableName, *req.DatatableNameFilter) {
			continue
		}
		if len(req.Types) > 0 && !containsString(req.Types, d.DatasourceType) {
			continue
		}
		filtered = append(filtered, d)
	}
	var tableIDs, httpIDs, allIDs []string
	for _, d := range filtered {
		allIDs = append(allIDs, d.DatatableID)
		if d.Type == datatableTypeHTTP {
			httpIDs = append(httpIDs, d.DatatableID)
		} else {
			tableIDs = append(tableIDs, d.DatatableID)
		}
	}
	auth := s.datatableAuthProjects(ctx, nodeID, tableIDs)
	for k, v := range s.featureAuthProjects(ctx, nodeID, httpIDs) {
		auth[k] = v
	}
	teeDomain := s.teeDomain(req.TeeNodeID)
	tee := s.pushToTeeInfos(ctx, nodeID, teeDomain, allIDs)
	out := make([]DatatableVO, 0, len(filtered))
	for _, d := range filtered {
		out = append(out, *d.toVO(auth[d.DatatableID], tee[BuildTeeDatatableID(teeDomain, d.DatatableID)]))
	}
	return out, kerr
}

// --- Get ---

// resolveDatasourceType fills an empty datasourceType from Kuscia / feature_table.
func (s *DatatableService) resolveDatasourceType(ctx context.Context, nodeID, datatableID, reqType, typ string) string {
	if reqType != "" {
		return strings.ToUpper(reqType)
	}
	if strings.EqualFold(typ, datatableTypeHTTP) {
		return DatasourceTypeHTTP
	}
	if s.db != nil {
		var cnt int64
		s.db.WithContext(ctx).Model(&model.FeatureTableDO{}).Where("feature_table_id = ? AND node_id = ?", datatableID, nodeID).Count(&cnt)
		if cnt > 0 {
			return DatasourceTypeHTTP
		}
	}
	if s.kusciaClient != nil {
		if resp, err := s.kc(nodeID).QueryDomainData(ctx, nodeID, datatableID); err == nil {
			return fromDomainData(&resp.Data).DatasourceType
		}
	}
	return DatasourceTypeLocal
}

// GetDatatableCompat mirrors Java getDatatable (per datasourceType handler).
func (s *DatatableService) GetDatatableCompat(ctx context.Context, req *GetDatatableCompatRequest) (*DatatableNodeVO, error) {
	if req.NodeID == "" || req.DatatableID == "" {
		return nil, bizErr(errcode.ParamError, "nodeId/datatableId cannot be blank")
	}
	dsType := s.resolveDatasourceType(ctx, req.NodeID, req.DatatableID, req.DatasourceType, req.Type)
	if dsType == DatasourceTypeHTTP {
		return s.getHTTPDatatable(ctx, req)
	}
	if err := s.requireKuscia(); err != nil {
		return nil, err
	}
	resp, err := s.kc(req.NodeID).QueryDomainData(ctx, req.NodeID, req.DatatableID)
	if err != nil {
		if kuscia.IsNotFound(err) {
			return nil, bizErr(errcode.DatatableNotExists, "")
		}
		return nil, bizWrap(errcode.DatatableQueryDatatableFailed, err, "")
	}
	dto := fromDomainData(&resp.Data)
	s.checkTableExistence(ctx, dsType, dto)
	auth := s.datatableAuthProjects(ctx, req.NodeID, []string{dto.DatatableID})
	teeDomain := s.teeDomain(req.TeeNodeID)
	tee := s.pushToTeeInfos(ctx, req.NodeID, teeDomain, []string{dto.DatatableID})
	vo := dto.toVO(auth[dto.DatatableID], tee[BuildTeeDatatableID(teeDomain, dto.DatatableID)])
	return &DatatableNodeVO{DatatableVO: vo, NodeID: req.NodeID, NodeName: s.nodeName(ctx, req.NodeID)}, nil
}

// checkTableExistence mirrors the queryDatatable hooks of the OSS / MYSQL /
// ODPS handlers: a missing underlying object/table marks the table UnAvailable.
func (s *DatatableService) checkTableExistence(ctx context.Context, dsType string, dto *datatableDTO) {
	if dsType != DatasourceTypeOSS && dsType != DatasourceTypeMySQL && dsType != DatasourceTypeODPS {
		return
	}
	ds, err := s.kc(dto.NodeID).QueryDomainDataSource(ctx, dto.NodeID, dto.DatasourceID)
	if err != nil || ds.Info == nil {
		s.log.Warn("query datasource for existence check failed", zap.String("datasource", dto.DatasourceID), zap.Error(err))
		return
	}
	exists := true
	switch dsType {
	case DatasourceTypeOSS:
		if o := ds.Info.Oss; o != nil {
			key := dto.RelativeURI
			if o.Prefix != "" {
				key = strings.TrimRight(o.Prefix, "/") + "/" + strings.TrimLeft(key, "/")
			}
			exists, _ = s.oss.CheckObjectExists(ctx, &OssConfig{
				Endpoint: o.Endpoint, AccessKeyID: o.AccessKeyID, AccessKeySecret: o.AccessKeySecret, VirtualHost: o.Virtualhost,
			}, o.Bucket, key)
		}
	case DatasourceTypeMySQL:
		if d := ds.Info.Database; d != nil {
			exists = s.mysql.TableExists(ctx, &MysqlConfig{Endpoint: d.Endpoint, User: d.User, Password: d.Password, Database: d.Database}, d.Database, dto.RelativeURI)
		}
	case DatasourceTypeODPS:
		if o := ds.Info.Odps; o != nil {
			exists = s.odps.TableExists(ctx, &OdpsConfig{Endpoint: o.Endpoint, AccessID: o.AccessKeyID, AccessKey: o.AccessKeySecret, Project: o.Project}, o.Project, dto.RelativeURI)
		}
	}
	if !exists {
		dto.Status = StatusUnavailable
	}
}

// getHTTPDatatable mirrors Java HttpDatatableHandler.queryDatatable.
func (s *DatatableService) getHTTPDatatable(ctx context.Context, req *GetDatatableCompatRequest) (*DatatableNodeVO, error) {
	var ft model.FeatureTableDO
	if err := s.db.WithContext(ctx).Where("feature_table_id = ? AND node_id = ?", req.DatatableID, req.NodeID).First(&ft).Error; err != nil {
		return nil, bizErr(errcode.FeatureTableNotExist, "")
	}
	auth := s.featureAuthProjects(ctx, req.NodeID, []string{ft.FeatureTableID})
	ok, _ := s.ipFilter.Detection(ctx, ft.URL)
	status := StatusUnavailable
	if ok {
		status = StatusAvailable
	}
	if ft.Status != status {
		s.db.WithContext(ctx).Model(&model.FeatureTableDO{}).Where("id = ?", ft.ID).Update("status", status)
		ft.Status = status
	}
	dto := fromFeatureTable(&ft)
	return &DatatableNodeVO{DatatableVO: dto.toVO(auth[ft.FeatureTableID], nil), NodeID: req.NodeID, NodeName: s.nodeName(ctx, req.NodeID)}, nil
}

// --- Delete ---

// DeleteDatatable mirrors Java deleteDatatable. It needs no project id:
// tables authorized to any project are refused; tables pushed to TEE get a
// TEE delete job first; then the Kuscia DomainData (or feature_table row for
// HTTP) is removed.
func (s *DatatableService) DeleteDatatable(ctx context.Context, req *DeleteDatatableRequest) error {
	if req.NodeID == "" || req.DatatableID == "" {
		return bizErr(errcode.ParamError, "nodeId/datatableId cannot be blank")
	}
	dsType := s.resolveDatasourceType(ctx, req.NodeID, req.DatatableID, req.DatasourceType, req.Type)
	if dsType == DatasourceTypeHTTP {
		if len(s.featureAuthProjects(ctx, req.NodeID, []string{req.DatatableID})) > 0 {
			return bizErr(errcode.DatatableDuplicatedAuthorized, "")
		}
		return s.db.WithContext(ctx).Where("feature_table_id = ? AND node_id = ?", req.DatatableID, req.NodeID).
			Delete(&model.FeatureTableDO{}).Error
	}
	if len(s.datatableAuthProjects(ctx, req.NodeID, []string{req.DatatableID})) > 0 {
		return bizErr(errcode.DatatableDuplicatedAuthorized, "")
	}
	if err := s.requireKuscia(); err != nil {
		return err
	}
	// The TEE node is the request teeNodeId, falling back to the configured
	// one; the TEE-side datatable id is built from that same node, exactly
	// like PushDatatableToTee (records written before the ids were unified
	// used the configured node: still matched).
	teeDomain := s.teeDomain(req.TeeNodeID)
	teeDatatableID := BuildTeeDatatableID(teeDomain, req.DatatableID)
	candidates := []string{teeDatatableID}
	if legacy := BuildTeeDatatableID(s.teeNodeID, req.DatatableID); legacy != teeDatatableID {
		candidates = append(candidates, legacy)
	}
	var push model.TeeNodeDatatableManagementDO
	if s.db != nil && s.db.WithContext(ctx).Where("node_id = ? AND tee_node_id = ? AND datatable_id IN ? AND kind = ?",
		req.NodeID, teeDomain, candidates, TeeJobKindPush).Order("gmt_create DESC").First(&push).Error == nil &&
		push.Status == TeeJobStatusSuccess {
		teeDatatableID = push.DatatableID
		dsID := req.DatasourceID
		if dsID == "" {
			dsID = DefaultLocalDatasourceID
		}
		info, _ := json.Marshal(map[string]string{teeOperatePushToTeeJobID: push.JobID, teeOperateRelativeURI: req.RelativeURI})
		del := &model.TeeNodeDatatableManagementDO{
			NodeID: req.NodeID, TeeNodeID: teeDomain, DatatableID: teeDatatableID, JobID: randomLetters(4),
			DatasourceID: dsID, Status: TeeJobStatusRunning, Kind: TeeJobKindDelete, OperateInfo: string(info),
		}
		if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := saveTeeManagement(tx, del); err != nil {
				return err
			}
			_, err := s.kc(req.NodeID).CreateJob(ctx, s.teeConverter.Convert(GenTeeJob(del, []string{req.NodeID}, "", nil, nil)))
			return err
		}); err != nil {
			return bizWrap(errcode.DatatableCreateTeeJobFailed, err, "")
		}
	}
	if err := s.kc(req.NodeID).DeleteDomainData(ctx, req.NodeID, req.DatatableID); err != nil {
		return bizWrap(errcode.DatatableDeleteDatatableFailed, err, "")
	}
	return nil
}

// --- Push to TEE ---

func (s *DatatableService) routeExists(ctx context.Context, src, dst string) bool {
	if s.db == nil {
		return false
	}
	var cnt int64
	s.db.WithContext(ctx).Model(&model.NodeRouteDO{}).Where("src_node_id = ? AND dst_node_id = ?", src, dst).Count(&cnt)
	return cnt > 0
}

// PushDatatableToTee mirrors Java pushDatatableToTeeNode: check the two-way
// route with the TEE node, reuse or create the DomainDataGrant (PushAuth
// record), save a RUNNING Push record and create the TEE upload job in Kuscia.
// The Push record status is updated by SyncTeeJobs.
func (s *DatatableService) PushDatatableToTee(ctx context.Context, req *PushDatatableToTeeRequest) error {
	if req.NodeID == "" || req.DatatableID == "" {
		return bizErr(errcode.ParamError, "nodeId/datatableId cannot be blank")
	}
	if err := s.requireKuscia(); err != nil {
		return err
	}
	teeDomain := s.teeDomain(req.TeeNodeID)
	dsID := req.DatasourceID
	if dsID == "" {
		dsID = DefaultLocalDatasourceID
	}
	// Same TEE node for the record and the TEE-side datatable id (see
	// DeleteDatatable / pushToTeeInfos).
	teeDatatableID := BuildTeeDatatableID(teeDomain, req.DatatableID)
	relativeURI := req.RelativeURI
	if relativeURI == "" {
		relativeURI = teeDatatableID
	}
	if !s.routeExists(ctx, req.NodeID, teeDomain) {
		return bizErr(errcode.NodeRouteNotExistError, fmt.Sprintf("route not exist %s->%s", req.NodeID, teeDomain))
	}
	if !s.routeExists(ctx, teeDomain, req.NodeID) {
		return bizErr(errcode.NodeRouteNotExistError, fmt.Sprintf("route not exist %s->%s", teeDomain, req.NodeID))
	}

	pushAuth := false
	var pa model.TeeNodeDatatableManagementDO
	if s.db.WithContext(ctx).Where("node_id = ? AND tee_node_id = ? AND datatable_id = ? AND kind = ?",
		req.NodeID, teeDomain, req.DatatableID, TeeJobKindPushAuth).First(&pa).Error == nil {
		if grantID := operateInfoString(pa.OperateInfo, teeOperateGrantID); grantID != "" {
			if _, err := s.kc(req.NodeID).QueryDomainDataGrant(ctx, req.NodeID, grantID); err == nil {
				pushAuth = true
			} else {
				s.log.Info("datatable grant is empty, recreate", zap.String("node", req.NodeID), zap.String("datatable", req.DatatableID))
			}
		}
	}

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if !pushAuth {
			grantID, err := s.kc(req.NodeID).CreateDomainDataGrant(ctx, &kuscia.CreateDomainDataGrantRequest{
				DomainID: req.NodeID, DomainDataID: req.DatatableID, GrantDomain: teeDomain,
			})
			if err != nil {
				return bizWrap(errcode.DatatableCreateDatatableGrantFailed, err, "")
			}
			info, _ := json.Marshal(map[string]string{teeOperateGrantID: grantID})
			if err := saveTeeManagement(tx, &model.TeeNodeDatatableManagementDO{
				NodeID: req.NodeID, DatatableID: req.DatatableID, TeeNodeID: teeDomain, JobID: randomLetters(4),
				DatasourceID: dsID, Status: TeeJobStatusSuccess, Kind: TeeJobKindPushAuth, OperateInfo: string(info),
			}); err != nil {
				return err
			}
		}
		info, _ := json.Marshal(map[string]string{teeOperateRelativeURI: relativeURI})
		push := &model.TeeNodeDatatableManagementDO{
			NodeID: req.NodeID, DatatableID: teeDatatableID, TeeNodeID: teeDomain, JobID: randomLetters(4),
			DatasourceID: dsID, Status: TeeJobStatusRunning, Kind: TeeJobKindPush, OperateInfo: string(info),
		}
		if err := saveTeeManagement(tx, push); err != nil {
			return err
		}
		job := GenTeeJob(push, []string{req.NodeID, teeDomain}, "", nil, nil)
		if _, err := s.kc(req.NodeID).CreateJob(ctx, s.teeConverter.Convert(job)); err != nil {
			return bizWrap(errcode.DatatableCreateTeeJobFailed, err, "")
		}
		return nil
	})
}

// --- Legacy Go-only methods ---

// RegisterDatatable registers a datatable to a project (Go-only /datatable/register).
func (s *DatatableService) RegisterDatatable(ctx context.Context, req *RegisterDatatableRequest) (*LegacyProjectDatatableVO, error) {
	datatableID := req.DatatableID
	if datatableID == "" {
		datatableID = uuid.New().String()[:8]
	}
	source := req.Source
	if source == "" {
		source = "IMPORTED"
	}
	dt := &model.ProjectDatatableDO{
		ProjectID: req.ProjectID, NodeID: req.NodeID, DatatableID: datatableID,
		TableConfigs: req.TableConfigs, Source: source,
	}
	if err := s.datatableRepo.Create(ctx, dt); err != nil {
		return nil, err
	}
	return &LegacyProjectDatatableVO{
		ProjectID: req.ProjectID, NodeID: req.NodeID, DatatableID: datatableID,
		TableConfigs: req.TableConfigs, Source: source, GmtCreate: dt.GmtCreate.Format("2006-01-02 15:04:05"),
	}, nil
}

// GetProjectDatatable retrieves a project datatable row (Go-only /datatable/detail).
func (s *DatatableService) GetProjectDatatable(ctx context.Context, req *ProjectDatatableDetailRequest) (*LegacyProjectDatatableVO, error) {
	dt, err := s.datatableRepo.FindByProjectNodeDatatable(ctx, req.ProjectID, req.NodeID, req.DatatableID)
	if err != nil {
		return nil, ErrDatatableNotFound
	}
	return &LegacyProjectDatatableVO{
		ProjectID: dt.ProjectID, NodeID: dt.NodeID, DatatableID: dt.DatatableID,
		TableConfigs: dt.TableConfigs, Source: dt.Source, GmtCreate: dt.GmtCreate.Format("2006-01-02 15:04:05"),
	}, nil
}

// GrantDatatable grants a node's DomainData to another domain (Go-only
// /datatable/grant). Errors are returned to the caller.
func (s *DatatableService) GrantDatatable(ctx context.Context, req *GrantDatatableRequest) (string, error) {
	if err := s.requireKuscia(); err != nil {
		return "", err
	}
	id, err := s.kc(req.NodeID).CreateDomainDataGrant(ctx, &kuscia.CreateDomainDataGrantRequest{
		DomainID: req.NodeID, DomainDataID: req.DatatableID, GrantDomain: req.TargetNode,
	})
	if err != nil {
		return "", bizWrap(errcode.DatatableCreateDatatableGrantFailed, err, "")
	}
	return id, nil
}

// CreateFedTable creates a federated table by persisting the join configuration
// as a JSON array. Uses json.Marshal to prevent JSON injection from user input.
func (s *DatatableService) CreateFedTable(ctx context.Context, req *CreateFedTableRequest) (*FedTableVO, error) {
	fedTableID := uuid.New().String()[:8]
	joinsBytes, err := json.Marshal(req.Joins)
	if err != nil {
		return nil, errors.New("failed to serialize join configuration")
	}
	joinsJSON := string(joinsBytes)
	fedTable := &model.ProjectFedTableDO{ProjectID: req.ProjectID, FedTableID: fedTableID, Joins: joinsJSON}
	if err := s.fedTableRepo.Create(ctx, fedTable); err != nil {
		return nil, err
	}
	return &FedTableVO{
		FedTableID: fedTableID, ProjectID: req.ProjectID, Joins: joinsJSON,
		GmtCreate: fedTable.GmtCreate.Format("2006-01-02 15:04:05"),
	}, nil
}

// WithKusciaPool routes per-node Kuscia calls through the multi-node pool.
func (s *DatatableService) WithKusciaPool(p *kuscia.ClientPool) *DatatableService {
	s.pool = p
	return s
}

// kc returns the Kuscia client serving nodeID (pool entry or the default client).
func (s *DatatableService) kc(nodeID string) *kuscia.Client {
	if s.pool != nil {
		if c := s.pool.Get(nodeID); c != nil {
			return c
		}
	}
	return s.kusciaClient
}
