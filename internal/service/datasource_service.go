package service

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/pkg/errcode"
	"github.com/fengzhizi319/privahub/pkg/kuscia"
)

// Datasource constants (Java DomainDatasourceConstants / DataSourceTypeEnum).
const (
	DatasourceTypeOSS   = "OSS"
	DatasourceTypeODPS  = "ODPS"
	DatasourceTypeMySQL = "MYSQL"
	DatasourceTypeHTTP  = "HTTP"
	DatasourceTypeLocal = "LOCAL"

	DefaultHTTPDatasourceID   = "http-data-source"
	DefaultHTTPDatasourceName = "http-data-source"
	DefaultLocalDatasourceID  = "default-data-source"
	DefaultLocalDatasourceNm  = "default-data-source"
	DefaultLocalKusciaType    = "localfs"

	datasourceOSSIDPrefix   = "oss-"
	datasourceODPSIDPrefix  = "odps-"
	datasourceMySQLIDPrefix = "mysql-"

	StatusAvailable   = "Available"
	StatusUnavailable = "UnAvailable"

	// kusciaPerNodeTimeout bounds each per-node Kuscia call in fan-out operations.
	kusciaPerNodeTimeout = 10 * time.Second
)

// Datasource service errors.
var (
	ErrDatasourceNotFound = errors.New("datasource not found")
)

// DatatableFinder lists the datatables of a node (Java DatatableService.findDatatableByNodeId).
type DatatableFinder interface {
	FindDatatablesByNodeID(ctx context.Context, nodeID string) ([]DatatableVO, error)
}

// DatasourceService implements Java DatasourceServiceImpl + the per-type
// datasource handlers (OSS/ODPS/MYSQL are Kuscia DomainDataSources, HTTP is a
// fixed virtual datasource). Kuscia is the source of truth; the local
// datasource/datasource_node tables are only a best-effort metadata cache
// (no credentials) for legacy readers.
type DatasourceService struct {
	db           *gorm.DB
	kusciaClient *kuscia.Client
	env          *EnvService
	oss          *OssService
	mysql        *MysqlChecker
	odps         *OdpsChecker
	ipFilter     *IPFilter
	datatables   DatatableFinder
	pool         *kuscia.ClientPool
	log          *zap.Logger
}

// NewDatasourceService creates a new DatasourceService with default checkers
// (Java default IP block list). Use the With* setters to customize.
func NewDatasourceService(db *gorm.DB, kusciaClient *kuscia.Client) *DatasourceService {
	f := NewIPFilter(nil)
	return &DatasourceService{
		db:           db,
		kusciaClient: kusciaClient,
		oss:          NewOssServiceWithFilter(f),
		mysql:        NewMysqlCheckerWithFilter(f),
		odps:         NewOdpsCheckerWithFilter(f),
		ipFilter:     f,
		log:          zap.NewNop(),
	}
}

// WithEnv sets the platform environment.
func (s *DatasourceService) WithEnv(env *EnvService) *DatasourceService { s.env = env; return s }

// WithIPFilter sets the SSRF filter (also used by the OSS checker).
func (s *DatasourceService) WithIPFilter(f *IPFilter) *DatasourceService {
	if f == nil {
		f = NewDisabledIPFilter()
	}
	s.ipFilter = f
	s.oss = NewOssServiceWithFilter(f)
	s.mysql = NewMysqlCheckerWithFilter(f)
	s.odps = NewOdpsCheckerWithFilter(f)
	return s
}

// WithMysqlChecker overrides the MySQL checker.
func (s *DatasourceService) WithMysqlChecker(m *MysqlChecker) *DatasourceService {
	s.mysql = m
	return s
}

// WithDatatableFinder sets the datatable lookup used for relatedDatas / delete checks.
func (s *DatasourceService) WithDatatableFinder(f DatatableFinder) *DatasourceService {
	s.datatables = f
	return s
}

// WithLogger sets the logger.
func (s *DatasourceService) WithLogger(l *zap.Logger) *DatasourceService {
	if l != nil {
		s.log = l
	}
	return s
}

// --- Request / Response DTOs (Java field names; JSON camelCase) ---

// OssDatasourceInfo is the OSS dataSourceInfo (Java OssDatasourceInfo).
type OssDatasourceInfo struct {
	Endpoint    string `json:"endpoint"`
	Bucket      string `json:"bucket"`
	Prefix      string `json:"prefix,omitempty"`
	AK          string `json:"ak"`
	SK          string `json:"sk"`
	StorageType string `json:"storageType,omitempty"`
	Virtualhost *bool  `json:"virtualhost,omitempty"`
}

// OdpsDatasourceInfo is the ODPS dataSourceInfo (Java OdpsDatasourceInfo).
type OdpsDatasourceInfo struct {
	Endpoint  string `json:"endpoint"`
	AccessID  string `json:"accessId"`
	AccessKey string `json:"accessKey"`
	Project   string `json:"project"`
}

// MysqlDatasourceInfo is the MYSQL dataSourceInfo (Java MysqlDatasourceInfo).
type MysqlDatasourceInfo struct {
	Endpoint string `json:"endpoint"`
	User     string `json:"user"`
	Password string `json:"password"`
	Database string `json:"database"`
}

// CreateDatasourceRequest is Java CreateDatasourceRequest. dataSourceInfo is
// polymorphic on `type` (OSS / ODPS / MYSQL).
type CreateDatasourceRequest struct {
	OwnerID        string          `json:"ownerId"`
	NodeIDs        []string        `json:"nodeIds"`
	Type           string          `json:"type"`
	Name           string          `json:"name"`
	DataSourceInfo json.RawMessage `json:"dataSourceInfo"`
}

// CreateDatasourceVO is Java CreateDatasourceVO.
type CreateDatasourceVO struct {
	DatasourceID       string            `json:"datasourceId"`
	FailedCreatedNodes map[string]string `json:"failedCreatedNodes"`
}

// DatasourceListRequest is Java DatasourceListRequest (SecretPadPageRequest: page/size).
type DatasourceListRequest struct {
	OwnerID string   `json:"ownerId"`
	Name    string   `json:"name"`
	Status  string   `json:"status"`
	Types   []string `json:"types"`
	Page    int      `json:"page"`
	Size    int      `json:"size"`
}

// DataSourceRelatedNode is Java DataSourceRelatedNode.
type DataSourceRelatedNode struct {
	NodeID   string `json:"nodeId"`
	NodeName string `json:"nodeName"`
	Status   string `json:"status"`
}

// DatasourceListInfoAggregate is Java DatasourceListInfoAggregate.
type DatasourceListInfoAggregate struct {
	Nodes        []DataSourceRelatedNode `json:"nodes"`
	DatasourceID string                  `json:"datasourceId"`
	Name         string                  `json:"name"`
	Type         string                  `json:"type"`
	RelatedDatas []string                `json:"relatedDatas,omitempty"`
}

// DatasourceListInfoUnAggregate is one datasource on one node.
type DatasourceListInfoUnAggregate struct {
	NodeID       string `json:"nodeId"`
	DatasourceID string `json:"datasourceId"`
	Name         string `json:"name"`
	Type         string `json:"type"`
	Status       string `json:"status"`
}

// DatasourceListVO is Java DatasourceListVO (AbstractPageResponse + infos).
type DatasourceListVO struct {
	Infos     []DatasourceListInfoAggregate `json:"infos"`
	PageSize  int                           `json:"pageSize"`
	PageNum   int                           `json:"pageNum"`
	Total     int64                         `json:"total"`
	TotalPage int64                         `json:"totalPage"`
}

// DatasourceDetailRequest is Java DatasourceDetailRequest.
type DatasourceDetailRequest struct {
	OwnerID      string `json:"ownerId"`
	DatasourceID string `json:"datasourceId"`
	Type         string `json:"type"`
}

// DatasourceDetailVO is Java DatasourceDetailAggregateVO. Info is one of the
// *DatasourceInfo structs with credentials desensitized ("abc****").
type DatasourceDetailVO struct {
	Nodes        []DataSourceRelatedNode `json:"nodes"`
	DatasourceID string                  `json:"datasourceId"`
	Name         string                  `json:"name"`
	Type         string                  `json:"type"`
	Status       string                  `json:"status"`
	Info         interface{}             `json:"info"`
}

// DeleteDatasourceRequest is Java DeleteDatasourceRequest.
type DeleteDatasourceRequest struct {
	OwnerID      string `json:"ownerId"`
	DatasourceID string `json:"datasourceId"`
	Type         string `json:"type"`
}

// DatasourceNodesRequest is Java DatasourceNodesRequest.
type DatasourceNodesRequest struct {
	OwnerID      string `json:"ownerId"`
	DatasourceID string `json:"datasourceId"`
}

// DatasourceNodesVO is Java DatasourceNodesVO.
type DatasourceNodesVO struct {
	Nodes []DataSourceRelatedNode `json:"nodes"`
}

// TestDatasourceRequest is the Go-only /datasource/test request: same shape as
// create (type + dataSourceInfo); the connectivity checks run without creating
// anything. `connectionInfo` (legacy JSON string) is accepted as a fallback.
type TestDatasourceRequest struct {
	Type           string          `json:"type"`
	DataSourceInfo json.RawMessage `json:"dataSourceInfo"`
	ConnectionInfo string          `json:"connectionInfo"`
}

// TestDatasourceVO is the connectivity test result.
type TestDatasourceVO struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// --- helpers ---

const lowerLetters = "abcdefghijklmnopqrstuvwxyz"

// randomLetters mirrors Java UUIDUtils.random(n): n lowercase letters.
func randomLetters(n int) string {
	b := make([]byte, n)
	max := big.NewInt(int64(len(lowerLetters)))
	for i := range b {
		v, err := rand.Int(rand.Reader, max)
		if err != nil {
			b[i] = 'a'
			continue
		}
		b[i] = lowerLetters[v.Int64()]
	}
	return string(b)
}

// akSkDesensitize mirrors Java DesensitizationUtils.akSkDesensitize for
// access key IDs only: the first 3 characters + "****" (and only when the ID
// is long enough that 3 characters are a small fraction of it).
func akSkDesensitize(s string) string {
	if len(s) < 6 {
		return secretMask(s)
	}
	return s[:3] + "****"
}

// secretMask fully masks a secret (password / secret key): no character of
// the secret, nor its length, is revealed.
func secretMask(s string) string {
	if s == "" {
		return ""
	}
	return "******"
}

func distinctStrings(in []string) []string {
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, v := range in {
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}

// fanOut runs fn for every node concurrently and returns node -> error message
// for the failed ones.
func fanOut(ctx context.Context, nodes []string, fn func(ctx context.Context, nodeID string) error) map[string]string {
	failed := make(map[string]string)
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, n := range nodes {
		wg.Add(1)
		go func(nodeID string) {
			defer wg.Done()
			cctx, cancel := context.WithTimeout(ctx, kusciaPerNodeTimeout)
			defer cancel()
			if err := fn(cctx, nodeID); err != nil {
				mu.Lock()
				failed[nodeID] = err.Error()
				mu.Unlock()
			}
		}(n)
	}
	wg.Wait()
	return failed
}

func (s *DatasourceService) isAutonomy() bool { return s.env != nil && s.env.IsAutonomy() }

func (s *DatasourceService) requireKuscia() error {
	if s.kusciaClient == nil {
		return bizErr(errcode.KusciaRpcError, "kuscia client not configured")
	}
	return nil
}

// endpointURL returns endpoint with a scheme so it can be probed.
func endpointURL(ep string) string {
	ep = strings.TrimSpace(ep)
	if ep != "" && !strings.Contains(ep, "://") {
		return "http://" + ep
	}
	return ep
}

// serviceCheck mirrors Java AbstractDatasourceHandler.serviceCheck.
func (s *DatasourceService) serviceCheck(ctx context.Context, endpoint string) error {
	ok, err := s.ipFilter.Detection(ctx, endpointURL(endpoint))
	if err != nil {
		return err
	}
	if !ok {
		return bizErr(errcode.DatasourceDataSourceEndpointConnectFail, "")
	}
	return nil
}

// parsedInfo is the decoded, validated dataSourceInfo.
type parsedInfo struct {
	oss   *OssDatasourceInfo
	odps  *OdpsDatasourceInfo
	mysql *MysqlDatasourceInfo
}

func parseDatasourceInfo(dsType string, raw json.RawMessage) (*parsedInfo, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, bizErr(errcode.ParamError, "dataSourceInfo is required")
	}
	p := &parsedInfo{}
	switch dsType {
	case DatasourceTypeOSS:
		p.oss = &OssDatasourceInfo{}
		if err := json.Unmarshal(raw, p.oss); err != nil {
			return nil, bizWrap(errcode.ParamError, err, "invalid oss dataSourceInfo")
		}
		if p.oss.Endpoint == "" || p.oss.Bucket == "" || p.oss.AK == "" || p.oss.SK == "" {
			return nil, bizErr(errcode.ParamError, "oss endpoint/bucket/ak/sk cannot be empty")
		}
	case DatasourceTypeODPS:
		p.odps = &OdpsDatasourceInfo{}
		if err := json.Unmarshal(raw, p.odps); err != nil {
			return nil, bizWrap(errcode.ParamError, err, "invalid odps dataSourceInfo")
		}
		if p.odps.Endpoint == "" || p.odps.AccessID == "" || p.odps.AccessKey == "" || p.odps.Project == "" {
			return nil, bizErr(errcode.ParamError, "odps endpoint/accessId/accessKey/project cannot be empty")
		}
	case DatasourceTypeMySQL:
		p.mysql = &MysqlDatasourceInfo{}
		if err := json.Unmarshal(raw, p.mysql); err != nil {
			return nil, bizWrap(errcode.ParamError, err, "invalid mysql dataSourceInfo")
		}
		if p.mysql.Endpoint == "" || p.mysql.User == "" || p.mysql.Password == "" || p.mysql.Database == "" {
			return nil, bizErr(errcode.ParamError, "mysql endpoint/user/password/database cannot be empty")
		}
		if !strings.Contains(p.mysql.Endpoint, ":") {
			return nil, bizErr(errcode.ParamError, "The endpoint is invalid, it must be a standard top-level domain or IP address + port, such as '127.0.0.1:8888")
		}
	default:
		return nil, bizErr(errcode.ParamError, "unsupported datasource type: "+dsType)
	}
	return p, nil
}

// connectivityError logs the detailed connectivity failure server-side and
// returns a generic client error: echoing dial/driver errors ("connection
// refused", "i/o timeout", server banners) would turn the datasource test
// into an internal port scanner.
func (s *DatasourceService) connectivityError(dsType, endpoint string, err error) error {
	if errors.Is(err, ErrIPBlocked) {
		return bizErr(errcode.FeatureTableIpFilter, endpoint)
	}
	if s.log != nil {
		s.log.Warn("datasource connectivity check failed", zap.String("type", dsType),
			zap.String("endpoint", endpoint), zap.Error(err))
	}
	return &BizError{Code: errcode.DatasourceDataSourceCreateFail, Err: err}
}

// clientErrorMessage is the client-safe message of a service error (never
// the wrapped cause).
func clientErrorMessage(err error) string {
	if be, ok := AsBizError(err); ok {
		if be.Msg != "" {
			return be.Msg
		}
		if be.Code != nil {
			return be.Code.Message
		}
	}
	return "datasource connectivity check failed"
}

// checkConnectivity runs the per-type connectivity checks of Java
// {Oss,Odps,Mysql}KusciaControlDatasourceHandler.createDatasource.
func (s *DatasourceService) checkConnectivity(ctx context.Context, dsType string, p *parsedInfo) error {
	switch dsType {
	case DatasourceTypeOSS:
		if err := s.serviceCheck(ctx, p.oss.Endpoint); err != nil {
			return err
		}
		vh := p.oss.Virtualhost != nil && *p.oss.Virtualhost
		return s.oss.CheckBucketExists(ctx, &OssConfig{
			Endpoint: p.oss.Endpoint, AccessKeyID: p.oss.AK, AccessKeySecret: p.oss.SK, VirtualHost: vh,
		}, p.oss.Bucket)
	case DatasourceTypeODPS:
		if err := s.serviceCheck(ctx, p.odps.Endpoint); err != nil {
			return err
		}
		if err := s.odps.TestConnection(ctx, &OdpsConfig{
			Endpoint: p.odps.Endpoint, AccessID: p.odps.AccessID, AccessKey: p.odps.AccessKey, Project: p.odps.Project,
		}); err != nil {
			return s.connectivityError("ODPS", p.odps.Endpoint, err)
		}
		return nil
	case DatasourceTypeMySQL:
		if err := s.ipFilter.CheckURL(p.mysql.Endpoint); err != nil {
			return err
		}
		if err := s.mysql.TestConnection(ctx, &MysqlConfig{
			Endpoint: p.mysql.Endpoint, User: p.mysql.User, Password: p.mysql.Password, Database: p.mysql.Database,
		}); err != nil {
			return s.connectivityError("MYSQL", p.mysql.Endpoint, err)
		}
		return nil
	}
	return bizErr(errcode.ParamError, "unsupported datasource type: "+dsType)
}

func toKusciaInfo(dsType string, p *parsedInfo) (string, *kuscia.DataSourceInfo, string) {
	switch dsType {
	case DatasourceTypeOSS:
		info := &kuscia.OssDataSourceInfo{
			Endpoint: p.oss.Endpoint, Bucket: p.oss.Bucket, Prefix: p.oss.Prefix,
			AccessKeyID: p.oss.AK, AccessKeySecret: p.oss.SK,
		}
		if p.oss.Virtualhost != nil {
			info.Virtualhost = *p.oss.Virtualhost
		}
		if p.oss.StorageType != "" {
			info.StorageType = p.oss.StorageType
		}
		return kuscia.DataSourceTypeOSS, &kuscia.DataSourceInfo{Oss: info}, datasourceOSSIDPrefix
	case DatasourceTypeODPS:
		return kuscia.DataSourceTypeODPS, &kuscia.DataSourceInfo{Odps: &kuscia.OdpsDataSourceInfo{
			Endpoint: p.odps.Endpoint, Project: p.odps.Project,
			AccessKeyID: p.odps.AccessID, AccessKeySecret: p.odps.AccessKey,
		}}, datasourceODPSIDPrefix
	default:
		return kuscia.DataSourceTypeMySQL, &kuscia.DataSourceInfo{Database: &kuscia.DatabaseDataSourceInfo{
			Endpoint: p.mysql.Endpoint, User: p.mysql.User, Password: p.mysql.Password, Database: p.mysql.Database,
		}}, datasourceMySQLIDPrefix
	}
}

// --- Service Methods ---

// CreateDatasource validates connectivity per type, then concurrently creates
// the DomainDataSource on every node (id = <type prefix> + 16 random letters).
// Fails only when every node fails; partial failures are reported in
// failedCreatedNodes.
func (s *DatasourceService) CreateDatasource(ctx context.Context, req *CreateDatasourceRequest) (*CreateDatasourceVO, error) {
	req.Type = strings.ToUpper(strings.TrimSpace(req.Type))
	if req.OwnerID == "" {
		return nil, bizErr(errcode.ParamError, "ownerId id cannot be blank")
	}
	if strings.TrimSpace(req.Name) == "" {
		return nil, bizErr(errcode.ParamError, "name can not be empty")
	}
	if len([]rune(req.Name)) > 32 {
		return nil, bizErr(errcode.ParamError, "name length cannot exceed 32 characters")
	}
	req.NodeIDs = distinctStrings(req.NodeIDs)
	if len(req.NodeIDs) == 0 {
		return nil, bizErr(errcode.ParamError, "nodes can not be empty, you should select at least one node")
	}
	// Java createDatasource: AUTONOMY nodes must belong to the owner institution.
	if s.isAutonomy() && s.db != nil && !checkNodesInInst(ctx, s.db, req.OwnerID, req.NodeIDs) {
		return nil, bizErr(errcode.InstNotMatchNode, "")
	}
	p, err := parseDatasourceInfo(req.Type, req.DataSourceInfo)
	if err != nil {
		return nil, err
	}
	if err := s.requireKuscia(); err != nil {
		return nil, err
	}
	if err := s.checkConnectivity(ctx, req.Type, p); err != nil {
		return nil, err
	}

	kType, info, prefix := toKusciaInfo(req.Type, p)
	datasourceID := prefix + randomLetters(16)
	accessDirectly := false
	failed := fanOut(ctx, req.NodeIDs, func(cctx context.Context, nodeID string) error {
		_, err := s.kc(nodeID).CreateDomainDataSource(cctx, &kuscia.CreateDomainDataSourceRequest{
			DomainID:       nodeID,
			DatasourceID:   datasourceID,
			Type:           kType,
			Name:           req.Name,
			Info:           info,
			AccessDirectly: &accessDirectly,
		})
		return err
	})
	if len(failed) == len(req.NodeIDs) {
		s.log.Error("all node create datasource failed", zap.Any("failed", failed))
		return nil, bizErr(errcode.DatasourceDataSourceCreateFail, "all node create datasource failed")
	}
	if len(failed) > 0 {
		s.log.Warn("some node create datasource failed", zap.Any("failed", failed))
	}
	s.cacheDatasource(ctx, req, datasourceID, failed)
	return &CreateDatasourceVO{DatasourceID: datasourceID, FailedCreatedNodes: failed}, nil
}

// cacheDatasource writes the credential-free metadata cache (best effort).
func (s *DatasourceService) cacheDatasource(ctx context.Context, req *CreateDatasourceRequest, id string, failed map[string]string) {
	if s.db == nil {
		return
	}
	_ = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&model.DatasourceDO{
			DatasourceID: id, Name: req.Name, Type: req.Type, Status: StatusAvailable, OwnerID: req.OwnerID,
		}).Error; err != nil {
			return err
		}
		for _, n := range req.NodeIDs {
			if _, bad := failed[n]; bad {
				continue
			}
			if err := tx.Create(&model.DatasourceNodeDO{DatasourceID: id, NodeID: n}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *DatasourceService) uncacheDatasource(ctx context.Context, id string, nodeIDs []string) {
	if s.db == nil {
		return
	}
	_ = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		q := tx.Where("datasource_id = ?", id)
		if len(nodeIDs) > 0 {
			q = q.Where("node_id IN ?", nodeIDs)
		}
		if err := q.Delete(&model.DatasourceNodeDO{}).Error; err != nil {
			return err
		}
		var left int64
		tx.Model(&model.DatasourceNodeDO{}).Where("datasource_id = ?", id).Count(&left)
		if left == 0 {
			return tx.Where("datasource_id = ?", id).Delete(&model.DatasourceDO{}).Error
		}
		return nil
	})
}

// filterKusciaType mirrors Java filterKusciaType: kuscia controlled types are
// listed with a single Kuscia list call; HTTP is virtual.
func filterKusciaType(types []string) (listKuscia, listHTTP bool) {
	if len(types) == 0 {
		return true, true
	}
	for _, t := range types {
		switch strings.ToUpper(t) {
		case DatasourceTypeHTTP:
			listHTTP = true
		default:
			listKuscia = true
		}
	}
	return
}

// ListNodeDatasources lists the datasources of a single node (Kuscia list +
// the virtual HTTP datasource). Exposed for other services (graph/job).
func (s *DatasourceService) ListNodeDatasources(ctx context.Context, nodeID string, types []string) ([]DatasourceListInfoUnAggregate, error) {
	listKuscia, listHTTP := filterKusciaType(types)
	out := make([]DatasourceListInfoUnAggregate, 0)
	if listKuscia {
		if err := s.requireKuscia(); err != nil {
			return nil, err
		}
		list, err := s.kc(nodeID).ListDomainDataSource(ctx, nodeID)
		if err != nil {
			return nil, err
		}
		for _, d := range list {
			st := d.Status
			if st == "" {
				st = StatusAvailable
			}
			dom := d.DomainID
			if dom == "" {
				dom = nodeID
			}
			out = append(out, DatasourceListInfoUnAggregate{
				NodeID: dom, DatasourceID: d.DatasourceID, Name: d.Name,
				Type: strings.ToUpper(d.Type), Status: st,
			})
		}
	}
	if listHTTP {
		out = append(out, DatasourceListInfoUnAggregate{
			NodeID: nodeID, DatasourceID: DefaultHTTPDatasourceID, Name: DefaultHTTPDatasourceName,
			Type: DatasourceTypeHTTP, Status: StatusAvailable,
		})
	}
	return out, nil
}

// listNodes returns the nodes to query for ownerId and a nodeId->name map.
func (s *DatasourceService) listNodes(ctx context.Context, ownerID string) ([]string, map[string]string) {
	names := map[string]string{}
	var nodeIDs []string
	if s.isAutonomy() && s.db != nil {
		// Java listDatasource: AUTONOMY lists the nodes of the owner
		// institution (ownerId is the institution id).
		var nodes []model.NodeDO
		s.db.WithContext(ctx).Where("inst_id = ?", ownerID).Order("id").Find(&nodes)
		for _, n := range nodes {
			nodeIDs = append(nodeIDs, n.NodeID)
			names[n.NodeID] = n.Name
		}
		return nodeIDs, names
	}
	if len(nodeIDs) == 0 {
		nodeIDs = []string{ownerID}
		if s.db != nil {
			var n model.NodeDO
			if err := s.db.WithContext(ctx).Where("node_id = ?", ownerID).First(&n).Error; err == nil {
				names[n.NodeID] = n.Name
			}
		}
	}
	return nodeIDs, names
}

// ListDatasources aggregates Kuscia datasources across nodes by datasourceId
// (Java listDatasource), then filters by name/types/status, attaches the
// related datatable names and paginates.
func (s *DatasourceService) ListDatasources(ctx context.Context, req *DatasourceListRequest) (*DatasourceListVO, error) {
	if req.OwnerID == "" {
		return nil, bizErr(errcode.ParamError, "ownerId id cannot be blank")
	}
	if req.Page <= 0 {
		req.Page = 1
	}
	if req.Size <= 0 {
		req.Size = 10
	}
	nodeIDs, names := s.listNodes(ctx, req.OwnerID)

	var mu sync.Mutex
	perNode := map[string][]DatasourceListInfoUnAggregate{}
	failed := fanOut(ctx, nodeIDs, func(cctx context.Context, nodeID string) error {
		list, err := s.ListNodeDatasources(cctx, nodeID, req.Types)
		if err != nil {
			return err
		}
		mu.Lock()
		perNode[nodeID] = list
		mu.Unlock()
		return nil
	})
	if len(failed) > 0 {
		s.log.Info("list datasource failed on some nodes", zap.Any("failed", failed))
	}

	// Aggregate by datasourceId in a stable order (node order, then Kuscia order).
	index := map[string]int{}
	aggs := make([]DatasourceListInfoAggregate, 0)
	for _, nodeID := range nodeIDs {
		for _, it := range perNode[nodeID] {
			i, ok := index[it.DatasourceID]
			if !ok {
				i = len(aggs)
				index[it.DatasourceID] = i
				aggs = append(aggs, DatasourceListInfoAggregate{
					DatasourceID: it.DatasourceID, Name: it.Name, Type: it.Type,
					Nodes: []DataSourceRelatedNode{},
				})
			}
			aggs[i].Nodes = append(aggs[i].Nodes, DataSourceRelatedNode{
				NodeID: it.NodeID, NodeName: names[it.NodeID], Status: it.Status,
			})
		}
	}

	// Filters (Java filterByName / filterByType / filterByStatus).
	filtered := aggs[:0:0]
	for _, a := range aggs {
		if req.Name != "" && !strings.Contains(a.Name, req.Name) {
			continue
		}
		if len(req.Types) > 0 && !containsString(req.Types, strings.ToUpper(a.Type)) {
			continue
		}
		if req.Status != "" {
			match := false
			for _, n := range a.Nodes {
				if strings.EqualFold(n.Status, req.Status) {
					match = true
					break
				}
			}
			if !match {
				continue
			}
		}
		filtered = append(filtered, a)
	}
	s.attachRelatedDatas(ctx, filtered)

	start := req.Size * (req.Page - 1)
	if start > len(filtered) {
		return nil, bizErr(errcode.SystemOutOfRangeError, "page start index > datasource  list length.")
	}
	end := start + req.Size
	if end > len(filtered) {
		end = len(filtered)
	}
	total := int64(len(filtered))
	totalPage := total / int64(req.Size)
	if total%int64(req.Size) != 0 {
		totalPage++
	}
	return &DatasourceListVO{
		Infos: filtered[start:end], PageNum: req.Page, PageSize: req.Size, Total: total, TotalPage: totalPage,
	}, nil
}

// attachRelatedDatas fills relatedDatas with the names of datatables that use
// each datasource on its nodes.
func (s *DatasourceService) attachRelatedDatas(ctx context.Context, aggs []DatasourceListInfoAggregate) {
	if s.datatables == nil || len(aggs) == 0 {
		return
	}
	nodeSet := map[string]bool{}
	for _, a := range aggs {
		for _, n := range a.Nodes {
			nodeSet[n.NodeID] = true
		}
	}
	nodes := make([]string, 0, len(nodeSet))
	for n := range nodeSet {
		nodes = append(nodes, n)
	}
	sort.Strings(nodes)
	var mu sync.Mutex
	byDs := map[string][]string{}
	fanOut(ctx, nodes, func(cctx context.Context, nodeID string) error {
		tables, err := s.datatables.FindDatatablesByNodeID(cctx, nodeID)
		if err != nil {
			return err
		}
		mu.Lock()
		defer mu.Unlock()
		for _, t := range tables {
			byDs[t.DatasourceID] = append(byDs[t.DatasourceID], t.DatatableName)
		}
		return nil
	})
	for i := range aggs {
		if names, ok := byDs[aggs[i].DatasourceID]; ok {
			aggs[i].RelatedDatas = distinctStrings(names)
		}
	}
}

func (s *DatasourceService) findAll(ctx context.Context, ownerID string, types []string) ([]DatasourceListInfoAggregate, error) {
	vo, err := s.ListDatasources(ctx, &DatasourceListRequest{OwnerID: ownerID, Types: types, Page: 1, Size: 1000})
	if err != nil {
		return nil, err
	}
	return vo.Infos, nil
}

func findAgg(aggs []DatasourceListInfoAggregate, id string) *DatasourceListInfoAggregate {
	for i := range aggs {
		if aggs[i].DatasourceID == id {
			return &aggs[i]
		}
	}
	return nil
}

// GetDatasourceDetail returns the aggregated nodes plus the Kuscia detail
// (credentials desensitized) of the first node holding the datasource.
func (s *DatasourceService) GetDatasourceDetail(ctx context.Context, req *DatasourceDetailRequest) (*DatasourceDetailVO, error) {
	req.Type = strings.ToUpper(req.Type)
	if req.OwnerID == "" || req.DatasourceID == "" || req.Type == "" {
		return nil, bizErr(errcode.ParamError, "ownerId/datasourceId/type can not be empty")
	}
	aggs, err := s.findAll(ctx, req.OwnerID, []string{req.Type})
	if err != nil {
		return nil, err
	}
	agg := findAgg(aggs, req.DatasourceID)
	if agg == nil || len(agg.Nodes) == 0 {
		return nil, bizErr(errcode.DatasourceDataSourceNotFound, "")
	}
	vo := &DatasourceDetailVO{Nodes: agg.Nodes, DatasourceID: req.DatasourceID, Status: StatusAvailable}
	if req.Type == DatasourceTypeHTTP {
		vo.Type = DatasourceTypeHTTP
		vo.Name = DefaultHTTPDatasourceName
		return vo, nil
	}
	nodeID := agg.Nodes[0].NodeID
	ds, err := s.kc(nodeID).QueryDomainDataSource(ctx, nodeID, req.DatasourceID)
	if err != nil {
		return nil, bizWrap(errcode.KusciaRpcError, err, "")
	}
	vo.Name = ds.Name
	vo.Type = strings.ToUpper(ds.Type)
	if ds.Status != "" {
		vo.Status = ds.Status
	}
	vo.Info = desensitizedInfo(ds)
	return vo, nil
}

func desensitizedInfo(ds *kuscia.DomainDataSource) interface{} {
	if ds.Info == nil {
		return nil
	}
	switch strings.ToUpper(ds.Type) {
	case DatasourceTypeOSS:
		if o := ds.Info.Oss; o != nil {
			vh := o.Virtualhost
			return &OssDatasourceInfo{
				Endpoint: o.Endpoint, Bucket: o.Bucket, Prefix: o.Prefix,
				AK: akSkDesensitize(o.AccessKeyID), SK: secretMask(o.AccessKeySecret),
				StorageType: o.StorageType, Virtualhost: &vh,
			}
		}
	case DatasourceTypeODPS:
		if o := ds.Info.Odps; o != nil {
			return &OdpsDatasourceInfo{
				Endpoint: o.Endpoint, Project: o.Project,
				AccessID: akSkDesensitize(o.AccessKeyID), AccessKey: secretMask(o.AccessKeySecret),
			}
		}
	case DatasourceTypeMySQL:
		if d := ds.Info.Database; d != nil {
			return &MysqlDatasourceInfo{
				Endpoint: d.Endpoint, User: d.User, Password: secretMask(d.Password), Database: d.Database,
			}
		}
	}
	return nil
}

// DeleteDatasource refuses when datatables still reference the datasource and
// otherwise deletes it from every node (Java deleteDatasource). Per-node
// failures are logged; the call fails only if every node failed.
func (s *DatasourceService) DeleteDatasource(ctx context.Context, req *DeleteDatasourceRequest) error {
	req.Type = strings.ToUpper(req.Type)
	if req.OwnerID == "" || req.DatasourceID == "" || req.Type == "" {
		return bizErr(errcode.ParamError, "ownerId/datasourceId/type can not be empty")
	}
	if req.Type != DatasourceTypeOSS && req.Type != DatasourceTypeODPS && req.Type != DatasourceTypeMySQL {
		return bizErr(errcode.ParamError, "unsupported datasource type: "+req.Type)
	}
	if err := s.requireKuscia(); err != nil {
		return err
	}
	aggs, err := s.findAll(ctx, req.OwnerID, []string{req.Type})
	if err != nil {
		return err
	}
	agg := findAgg(aggs, req.DatasourceID)
	if agg == nil {
		return bizErr(errcode.DatasourceDataSourceDeleteFail, "not found the datasource")
	}
	if len(agg.RelatedDatas) > 0 {
		return bizErr(errcode.DatasourceDataSourceDeleteFail, "has related data table")
	}
	nodes := make([]string, 0, len(agg.Nodes))
	for _, n := range agg.Nodes {
		nodes = append(nodes, n.NodeID)
	}
	failed := fanOut(ctx, nodes, func(cctx context.Context, nodeID string) error {
		if s.datatables != nil {
			tables, err := s.datatables.FindDatatablesByNodeID(cctx, nodeID)
			if err == nil {
				for _, t := range tables {
					if t.DatasourceID == req.DatasourceID {
						return fmt.Errorf("has related data table")
					}
				}
			}
		}
		return s.kc(nodeID).DeleteDomainDataSource(cctx, nodeID, req.DatasourceID)
	})
	if len(failed) > 0 {
		s.log.Info("delete datasource failed on some nodes", zap.Any("failed", failed))
	}
	if len(nodes) > 0 && len(failed) == len(nodes) {
		msgs := make([]string, 0, len(failed))
		for n, m := range failed {
			msgs = append(msgs, n+": "+m)
		}
		sort.Strings(msgs)
		return bizErr(errcode.DatasourceDataSourceDeleteFail, strings.Join(msgs, "; "))
	}
	ok := make([]string, 0, len(nodes))
	for _, n := range nodes {
		if _, bad := failed[n]; !bad {
			ok = append(ok, n)
		}
	}
	s.uncacheDatasource(ctx, req.DatasourceID, ok)
	return nil
}

// GetDatasourceNodes returns the nodes holding a datasource (Java datasourceNodes).
func (s *DatasourceService) GetDatasourceNodes(ctx context.Context, req *DatasourceNodesRequest) (*DatasourceNodesVO, error) {
	if req.OwnerID == "" || req.DatasourceID == "" {
		return nil, bizErr(errcode.ParamError, "ownerId/datasourceId can not be empty")
	}
	aggs, err := s.findAll(ctx, req.OwnerID, []string{DatasourceTypeHTTP, DatasourceTypeOSS, DatasourceTypeODPS, DatasourceTypeMySQL})
	if err != nil {
		return nil, err
	}
	agg := findAgg(aggs, req.DatasourceID)
	if agg == nil {
		return nil, bizErr(errcode.DatasourceDataSourceNotFound, "")
	}
	return &DatasourceNodesVO{Nodes: agg.Nodes}, nil
}

// TestDatasource runs the real per-type connectivity check (OSS signed bucket
// check, MySQL SELECT 1, ODPS signed project read) without creating anything.
func (s *DatasourceService) TestDatasource(ctx context.Context, req *TestDatasourceRequest) (*TestDatasourceVO, error) {
	t := strings.ToUpper(strings.TrimSpace(req.Type))
	raw := req.DataSourceInfo
	if len(raw) == 0 && req.ConnectionInfo != "" {
		raw = json.RawMessage(req.ConnectionInfo)
	}
	p, err := parseDatasourceInfo(t, raw)
	if err != nil {
		return nil, err
	}
	if err := s.checkConnectivity(ctx, t, p); err != nil {
		return &TestDatasourceVO{Success: false, Message: clientErrorMessage(err)}, nil
	}
	return &TestDatasourceVO{Success: true, Message: "connection test passed"}, nil
}

// WithKusciaPool routes per-node Kuscia calls through the multi-node pool.
func (s *DatasourceService) WithKusciaPool(p *kuscia.ClientPool) *DatasourceService {
	s.pool = p
	return s
}

// kc returns the Kuscia client serving nodeID (pool entry or the default client).
func (s *DatasourceService) kc(nodeID string) *kuscia.Client {
	if s.pool != nil {
		if c := s.pool.Get(nodeID); c != nil {
			return c
		}
	}
	return s.kusciaClient
}
