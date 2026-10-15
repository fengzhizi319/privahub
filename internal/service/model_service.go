package service

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/internal/dao/sqlutil"
	"github.com/fengzhizi319/privahub/internal/graphjob"
	"github.com/fengzhizi319/privahub/pkg/errcode"
	"github.com/fengzhizi319/privahub/pkg/kuscia"
)

// Model service errors (kept for errors.Is compatibility; returned wrapped in PadError).
var (
	ErrModelNotFound   = errors.New("model not found")
	ErrServingNotFound = errors.New("serving not found")
)

// Serving constants (Java ServingConstants / DomainDatasourceConstants).
const (
	servingDefaultCPU        = 0.1
	servingDefaultMemory     = 0.1
	servingDefaultMemoryUnit = "Gi"
	defaultHTTPDatasourceID  = "http-data-source"
	servingStatusAvailable   = "Available"
	servingStatusFailed      = "Failed"
)

// ModelService implements Java ModelExportServiceImpl + ModelManagementServiceImpl.
type ModelService struct {
	db           *gorm.DB
	kusciaClient *kuscia.Client
	pool         *kuscia.ClientPool
	env          *EnvService
	graph        *GraphService
	log          *zap.Logger
}

// NewModelService creates a new ModelService.
func NewModelService(db *gorm.DB, kusciaClient *kuscia.Client) *ModelService {
	return &ModelService{db: db, kusciaClient: kusciaClient, log: zap.NewNop()}
}

// WithKusciaPool routes per-node Kuscia calls through the multi-node pool.
func (s *ModelService) WithKusciaPool(p *kuscia.ClientPool) *ModelService { s.pool = p; return s }

// WithEnv sets the platform environment (center / edge / autonomy).
func (s *ModelService) WithEnv(e *EnvService) *ModelService { s.env = e; return s }

// WithGraphService sets the graph service used to snapshot the graph detail.
func (s *ModelService) WithGraphService(g *GraphService) *ModelService { s.graph = g; return s }

// WithLogger sets the logger.
func (s *ModelService) WithLogger(l *zap.Logger) *ModelService {
	if l != nil {
		s.log = l
	}
	return s
}

// kc returns the Kuscia client serving nodeID (pool entry or the default client).
func (s *ModelService) kc(nodeID string) *kuscia.Client {
	if s.pool != nil {
		if c := s.pool.Get(nodeID); c != nil {
			return c
		}
	}
	return s.kusciaClient
}

func (s *ModelService) isAutonomy() bool { return s.env != nil && s.env.IsAutonomy() }

func (s *ModelService) isCenter() bool { return s.env == nil || s.env.IsCenter() }

func (s *ModelService) platformNodeID() string {
	if s.env == nil {
		return ""
	}
	return s.env.GetPlatformNodeId()
}

// localParty returns the first party belonging to the current institution
// (autonomy mode), "" when none.
func (s *ModelService) localParty(parties []string) string {
	if s.env == nil {
		return ""
	}
	for _, p := range parties {
		if s.env.IsNodeInCurrentInst(p) {
			return p
		}
	}
	return ""
}

// targetNode mirrors ModelManagementServiceImpl.getTargetNode.
func (s *ModelService) targetNode(parties []string) string {
	if s.isAutonomy() {
		return s.localParty(parties)
	}
	return ""
}

// --- DTOs (Java VOs) ---

// PartyDataSource is Java PartyDataSource.
type PartyDataSource struct {
	PartyID    string `json:"partyId"`
	Datasource string `json:"datasource"`
}

// QueryModelPageRequest is Java QueryModelPageRequest.
type QueryModelPageRequest struct {
	Page       int               `json:"page"`
	Size       int               `json:"size"`
	Sort       map[string]string `json:"sort"`
	ProjectID  string            `json:"projectId"`
	SearchKey  string            `json:"searchKey"`
	ModelStats string            `json:"modelStats"`
}

// ModelPackVO is Java ModelPackVO.
type ModelPackVO struct {
	ModelID    string `json:"modelId"`
	ServingID  string `json:"servingId"`
	ModelName  string `json:"modelName"`
	ModelDesc  string `json:"modelDesc"`
	ModelStats string `json:"modelStats"`
	GmtCreate  string `json:"gmtCreate"`
	OwnerID    string `json:"ownerId"`
}

// ModelPackListVO is Java ModelPackListVO (AbstractPageResponse).
type ModelPackListVO struct {
	ModelPacks []ModelPackVO `json:"modelPacks"`
	PageSize   int           `json:"pageSize"`
	PageNum    int           `json:"pageNum"`
	Total      int64         `json:"total"`
	TotalPage  int64         `json:"totalPage"`
}

// ModelPackDetailParty is Java ModelPackDetailVO.Parties.
type ModelPackDetailParty struct {
	NodeID   string   `json:"nodeId"`
	NodeName string   `json:"nodeName"`
	Columns  []string `json:"columns"`
}

// ModelPackDetailVO is Java ModelPackDetailVO.
type ModelPackDetailVO struct {
	Parties []ModelPackDetailParty `json:"parties"`
}

// ServingResourceVO is Java ResourceVO.
type ServingResourceVO struct {
	MinCPU    string `json:"minCPU"`
	MaxCPU    string `json:"maxCPU"`
	MinMemory string `json:"minMemory"`
	MaxMemory string `json:"maxMemory"`
}

// ServingDetailItemVO is Java ServingDetailVO.ServingDetail.
type ServingDetailItemVO struct {
	NodeID          string              `json:"nodeId"`
	NodeName        string              `json:"nodeName"`
	Endpoints       string              `json:"endpoints,omitempty"`
	FeatureHTTP     string              `json:"featureHttp,omitempty"`
	IsMock          *bool               `json:"isMock,omitempty"`
	FeatureMappings map[string]string   `json:"featureMappings,omitempty"`
	SourcePath      string              `json:"sourcePath,omitempty"`
	Resources       []ServingResourceVO `json:"resources,omitempty"`
}

// ServingDetailVO is Java ServingDetailVO.
type ServingDetailVO struct {
	ModelID        string                `json:"modelId"`
	ServingID      string                `json:"servingId"`
	ServingDetails []ServingDetailItemVO `json:"servingDetails"`
}

// ModelPackInfoVO is Java ModelPackInfoVO.
type ModelPackInfoVO struct {
	GraphDetailVO    json.RawMessage       `json:"graphDetailVO"`
	ModelGraphDetail []string              `json:"modelGraphDetail"`
	ModelStats       string                `json:"modelStats"`
	ServingDetails   []ServingDetailItemVO `json:"servingDetails"`
}

// ServingFeature is CreateModelServingRequest.PartyConfig.Feature.
type ServingFeature struct {
	OfflineName string `json:"offlineName"`
	OnlineName  string `json:"onlineName"`
}

// ServingPartyConfig is CreateModelServingRequest.PartyConfig.
type ServingPartyConfig struct {
	NodeID         string              `json:"nodeId"`
	FeatureTableID string              `json:"featureTableId"`
	IsMock         *bool               `json:"isMock"`
	Features       []ServingFeature    `json:"features"`
	Resources      []ServingResourceVO `json:"resources"`
}

// CreateModelServingRequest is Java CreateModelServingRequest.
type CreateModelServingRequest struct {
	ModelID      string               `json:"modelId"`
	ProjectID    string               `json:"projectId"`
	PartyConfigs []ServingPartyConfig `json:"partyConfigs"`
}

// --- persistence helpers ---

func (s *ModelService) findPack(ctx context.Context, modelID string) (*model.ProjectModelPackDO, error) {
	var m model.ProjectModelPackDO
	if modelID == "" || s.db.WithContext(ctx).Where("model_id = ? AND is_deleted = 0", modelID).First(&m).Error != nil {
		return nil, padErrBase(ErrModelNotFound, errcode.ProjectModelNotFound)
	}
	return &m, nil
}

// findProjectPack binds the model to the project checked by the HTTP resource
// guard. Checking projectId alone is insufficient when modelId is independently
// supplied: otherwise a caller could use its own project to access another's model.
// Return the normal not-found error so the foreign model's existence is not leaked.
func (s *ModelService) findProjectPack(ctx context.Context, modelID, projectID string) (*model.ProjectModelPackDO, error) {
	pack, err := s.findPack(ctx, modelID)
	if err != nil {
		return nil, err
	}
	if projectID == "" || pack.ProjectID != projectID {
		return nil, padErrBase(ErrModelNotFound, errcode.ProjectModelNotFound)
	}
	return pack, nil
}

func (s *ModelService) findServing(ctx context.Context, servingID string) (*model.ProjectModelServingDO, error) {
	var sv model.ProjectModelServingDO
	if servingID == "" || s.db.WithContext(ctx).Where("serving_id = ?", servingID).First(&sv).Error != nil {
		return nil, padErrBase(ErrServingNotFound, errcode.ProjectServingNotFound)
	}
	return &sv, nil
}

// ModelProjectID returns the project owning a model ("" when unknown).
func (s *ModelService) ModelProjectID(ctx context.Context, modelID string) string {
	if m, err := s.findPack(ctx, modelID); err == nil {
		return m.ProjectID
	}
	return ""
}

// ServingProjectID returns the project owning a serving ("" when unknown).
func (s *ModelService) ServingProjectID(ctx context.Context, servingID string) string {
	if sv, err := s.findServing(ctx, servingID); err == nil {
		return sv.ProjectID
	}
	return ""
}

func (s *ModelService) nodeNames(ctx context.Context, ids []string) map[string]string {
	out := map[string]string{}
	if len(ids) == 0 {
		return out
	}
	var nodes []model.NodeDO
	s.db.WithContext(ctx).Where("node_id IN ?", ids).Find(&nodes)
	for _, n := range nodes {
		out[n.NodeID] = n.Name
	}
	return out
}

func parseSampleTables(raw string) map[string]string {
	m := map[string]string{}
	if raw != "" {
		_ = json.Unmarshal([]byte(raw), &m)
	}
	return m
}

func sortedMapKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func packPartyDataSources(pack *model.ProjectModelPackDO) []PartyDataSource {
	out := []PartyDataSource{}
	for _, pd := range parsePartyDataSources(pack.ModelDatasource, pack.Initiator) {
		out = append(out, PartyDataSource{PartyID: pd[0], Datasource: pd[1]})
	}
	return out
}

// --- model/page ---

var modelSortColumns = map[string]string{
	"modelid": "model_id", "model_id": "model_id",
	"modelname": "model_name", "model_name": "model_name",
	"modelstats": "model_stats", "model_stats": "model_stats",
	"gmtcreate": "gmt_create", "gmt_create": "gmt_create",
	"gmtmodified": "gmt_modified", "gmt_modified": "gmt_modified",
	"initiator": "initiator", "servingid": "serving_id", "serving_id": "serving_id",
}

// transferKeyword mirrors ModelManagementServiceImpl.transferKeyword.
func transferKeyword(keyword string) string {
	if strings.TrimSpace(keyword) == "" {
		return ""
	}
	return sqlutil.EscapeLike(keyword)
}

// ModelPackPage mirrors ModelManagementServiceImpl.modelPackPage.
func (s *ModelService) ModelPackPage(ctx context.Context, req *QueryModelPageRequest) (*ModelPackListVO, error) {
	page, size := req.Page, req.Size
	if page <= 0 {
		page = 1
	}
	if size <= 0 {
		size = 10
	}
	q := s.db.WithContext(ctx).Model(&model.ProjectModelPackDO{}).Where("project_id = ? AND is_deleted = 0", req.ProjectID)
	if strings.TrimSpace(req.ModelStats) != "" {
		code, ok := model.ModelStatsCode(strings.TrimSpace(req.ModelStats))
		if !ok {
			return nil, padErr(errcode.SystemValidationError, "invalid modelStats: "+req.ModelStats)
		}
		q = q.Where("model_stats = ?", code)
	}
	if strings.TrimSpace(req.SearchKey) != "" {
		pattern := "%" + transferKeyword(req.SearchKey) + "%"
		q = q.Where("(model_name LIKE ? ESCAPE '!' OR model_id LIKE ? ESCAPE '!')", pattern, pattern)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, err
	}
	ordered := false
	keys := sortedMapKeys(req.Sort)
	for _, k := range keys {
		col, ok := modelSortColumns[strings.ToLower(k)]
		if !ok {
			continue
		}
		dir := "ASC"
		if strings.EqualFold(req.Sort[k], "DESC") {
			dir = "DESC"
		}
		q = q.Order(col + " " + dir)
		ordered = true
	}
	if !ordered {
		q = q.Order("gmt_create DESC").Order("id DESC")
	}
	var packs []model.ProjectModelPackDO
	if err := q.Offset((page - 1) * size).Limit(size).Find(&packs).Error; err != nil {
		return nil, err
	}
	vos := make([]ModelPackVO, 0, len(packs))
	for i := range packs {
		vos = append(vos, s.convertPack(ctx, &packs[i]))
	}
	return &ModelPackListVO{
		ModelPacks: vos,
		PageSize:   size,
		PageNum:    page,
		Total:      total,
		TotalPage:  (total + int64(size) - 1) / int64(size),
	}, nil
}

// convertPack mirrors ModelManagementServiceImpl.convert (refreshes PUBLISHING models).
func (s *ModelService) convertPack(ctx context.Context, pack *model.ProjectModelPackDO) ModelPackVO {
	if pack.ServingID != "" && pack.ModelStats == model.ModelStatsPublishing {
		s.RefreshPublishing(ctx, pack)
	}
	owner := pack.Initiator
	if s.isAutonomy() && s.env.IsNodeInCurrentInst(pack.Initiator) {
		owner = s.env.GetInstID()
	}
	return ModelPackVO{
		ModelID:    pack.ModelID,
		ServingID:  pack.ServingID,
		ModelName:  pack.ModelName,
		ModelDesc:  pack.ModelDesc,
		ModelStats: model.ModelStatsName(pack.ModelStats),
		GmtCreate:  rfc3339(pack.GmtCreate),
		OwnerID:    owner,
	}
}

// packExecNode mirrors ModelManagementServiceImpl.getInitiator (old data compensation).
func (s *ModelService) packExecNode(pack *model.ProjectModelPackDO) string {
	if pack.Initiator == s.platformNodeID() {
		if s.isAutonomy() {
			parties := []string{}
			for _, p := range packPartyDataSources(pack) {
				parties = append(parties, p.PartyID)
			}
			return s.localParty(parties)
		}
		return s.platformNodeID()
	}
	return pack.Initiator
}

// partyEndpoint is Java ProjectModelServingDO.PartyEndpoints.
type partyEndpoint struct {
	NodeID    string `json:"nodeId"`
	Endpoints string `json:"endpoints"`
}

// RefreshPublishing queries Kuscia for a PUBLISHING model's serving and moves
// the model to PUBLISHED / PUBLISH_FAIL (updating serving_stats and endpoints).
// It returns true when the model status changed.
func (s *ModelService) RefreshPublishing(ctx context.Context, pack *model.ProjectModelPackDO) bool {
	if pack.ServingID == "" || pack.ModelStats != model.ModelStatsPublishing {
		return false
	}
	c := s.kc(s.packExecNode(pack))
	if c == nil {
		return false
	}
	resp, err := c.QueryServing(ctx, pack.ServingID)
	if err != nil || resp == nil || resp.Data == nil {
		if err != nil {
			s.log.Warn("query serving failed", zap.String("serving_id", pack.ServingID), zap.Error(err))
		}
		return false
	}
	st := resp.Data.Status
	var sv model.ProjectModelServingDO
	hasServing := s.db.WithContext(ctx).Where("serving_id = ?", pack.ServingID).First(&sv).Error == nil
	var newStats int8
	switch st.State {
	case servingStatusAvailable:
		newStats = model.ModelStatsPublished
		if hasServing {
			eps := make([]partyEndpoint, 0, len(st.PartyStatuses))
			for _, ps := range st.PartyStatuses {
				if s.isAutonomy() && ps.DomainID != sv.Initiator {
					continue
				}
				ep := ""
				for _, e := range ps.Endpoints {
					if e.PortName == "service" {
						ep = e.Endpoint
						break
					}
				}
				eps = append(eps, partyEndpoint{NodeID: ps.DomainID, Endpoints: ep})
			}
			b, _ := json.Marshal(eps)
			s.db.WithContext(ctx).Model(&model.ProjectModelServingDO{}).Where("serving_id = ?", sv.ServingID).
				Updates(map[string]any{"party_endpoints": string(b), "serving_stats": "success", "error_msg": ""})
		}
	case servingStatusFailed:
		newStats = model.ModelStatsPublishFail
		if hasServing {
			msg := st.Message
			if msg == "" {
				msg = st.Reason
			}
			s.db.WithContext(ctx).Model(&model.ProjectModelServingDO{}).Where("serving_id = ?", sv.ServingID).
				Updates(map[string]any{"serving_stats": "failed", "error_msg": msg})
		}
	default:
		return false
	}
	if err := s.db.WithContext(ctx).Model(&model.ProjectModelPackDO{}).Where("model_id = ?", pack.ModelID).
		Update("model_stats", newStats).Error; err != nil {
		return false
	}
	s.log.Info("project model serving stats changed", zap.String("model_id", pack.ModelID), zap.String("stats", model.ModelStatsName(newStats)))
	pack.ModelStats = newStats
	return true
}

// --- model/detail ---

// ModelPackDetail mirrors ModelManagementServiceImpl.modelPackDetail: the
// per-party feature columns = model report schema ∩ own sample table schema.
func (s *ModelService) ModelPackDetail(ctx context.Context, modelID, projectID string) (*ModelPackDetailVO, error) {
	pack, err := s.findProjectPack(ctx, modelID, projectID)
	if err != nil {
		return nil, err
	}
	tables := parseSampleTables(pack.SampleTables)
	keys := sortedMapKeys(tables)
	names := s.nodeNames(ctx, keys)
	target := s.targetNode(keys)
	out := &ModelPackDetailVO{Parties: []ModelPackDetailParty{}}
	for _, nodeID := range keys {
		c := s.kc(firstNonEmpty(target, nodeID))
		if c == nil {
			return nil, padErr(errcode.SystemError, "kuscia client not configured")
		}
		report, err := c.QueryDomainData(ctx, nodeID, pack.ModelReportID)
		if err != nil {
			return nil, padErr(errcode.ModelExportFailed, "query model report failed: "+err.Error())
		}
		columns := explainColumnsFromDistData(report.Data.Attributes)
		raw, err := c.QueryDomainData(ctx, nodeID, tables[nodeID])
		if err != nil {
			return nil, padErr(errcode.ModelExportFailed, "query sample table failed: "+err.Error())
		}
		out.Parties = append(out.Parties, ModelPackDetailParty{
			NodeID:   nodeID,
			NodeName: names[nodeID],
			Columns:  filterOwnSchema(columns, raw.Data.Columns),
		})
	}
	return out, nil
}

// explainColumnsFromDistData parses attributes["dist_data"].meta.desc ("a,b,c").
func explainColumnsFromDistData(attrs map[string]string) []string {
	raw := attrs["dist_data"]
	if raw == "" {
		return []string{}
	}
	var dd struct {
		Meta struct {
			Desc string `json:"desc"`
		} `json:"meta"`
	}
	if json.Unmarshal([]byte(raw), &dd) != nil || dd.Meta.Desc == "" {
		return []string{}
	}
	return strings.Split(dd.Meta.Desc, ",")
}

func filterOwnSchema(columns []string, own []kuscia.DataColumn) []string {
	if len(columns) == 0 {
		return columns
	}
	set := map[string]bool{}
	for _, c := range own {
		set[c.Name] = true
	}
	out := []string{}
	for _, c := range columns {
		if set[c] {
			out = append(out, c)
		}
	}
	return out
}

// --- model/info ---

// ModelPackInfo mirrors ModelManagementServiceImpl.modelPackInfo.
func (s *ModelService) ModelPackInfo(ctx context.Context, modelID, projectID string) (*ModelPackInfoVO, error) {
	pack, err := s.findProjectPack(ctx, modelID, projectID)
	if err != nil {
		return nil, err
	}
	var modelList []string
	_ = json.Unmarshal([]byte(pack.ModelList), &modelList)
	if modelList == nil {
		modelList = []string{}
	}
	graphDetail := json.RawMessage("null")
	if pack.GraphDetail != "" && json.Valid([]byte(pack.GraphDetail)) {
		graphDetail = json.RawMessage(pack.GraphDetail)
	}
	tables := parseSampleTables(pack.SampleTables)
	keys := sortedMapKeys(tables)
	names := s.nodeNames(ctx, keys)
	dsMap := map[string]string{}
	for _, p := range packPartyDataSources(pack) {
		dsMap[p.PartyID] = p.Datasource
	}
	details := make([]ServingDetailItemVO, 0, len(keys))
	for _, k := range keys {
		details = append(details, ServingDetailItemVO{NodeID: k, NodeName: names[k], SourcePath: dsMap[k]})
	}
	return &ModelPackInfoVO{
		GraphDetailVO:    graphDetail,
		ModelGraphDetail: modelList,
		ModelStats:       model.ModelStatsName(pack.ModelStats),
		ServingDetails:   details,
	}, nil
}

// --- model/serving/create ---

// CreateModelServing mirrors ModelManagementServiceImpl.createModelServing.
func (s *ModelService) CreateModelServing(ctx context.Context, req *CreateModelServingRequest) error {
	pack, err := s.findProjectPack(ctx, req.ModelID, req.ProjectID)
	if err != nil {
		return err
	}
	// Validate in the service too: background/internal callers do not pass
	// through bean-style HTTP validation. Empty deployments must never succeed.
	if msg := ValidateServingPartyConfigs(req.PartyConfigs); msg != "" {
		return padErr(errcode.SystemValidationError, msg)
	}
	nodeIDs := make([]string, 0, len(req.PartyConfigs))
	for _, p := range req.PartyConfigs {
		nodeIDs = append(nodeIDs, p.NodeID)
	}
	target := s.targetNode(nodeIDs)
	image := graphjob.GetSettings().ServingImage
	if image == "" {
		image = "sf-serving-image"
	}

	partyConfigs := map[string]any{}
	servingParties := make([]kuscia.ServingParty, 0, len(req.PartyConfigs))
	for _, party := range req.PartyConfigs {
		if party.NodeID == "" {
			return padErr(errcode.SystemValidationError, "nodeId is required")
		}
		featureMapping := map[string]string{}
		for _, f := range party.Features {
			if f.OnlineName == "" {
				continue
			}
			featureMapping[f.OnlineName] = f.OfflineName
		}
		c := s.kc(firstNonEmpty(target, party.NodeID))
		if c == nil {
			return padErr(errcode.SystemError, "kuscia client not configured")
		}
		dd, err := c.QueryDomainData(ctx, party.NodeID, pack.ModelID)
		if err != nil {
			return padErr(errcode.ProjectModelNotFound, err.Error())
		}
		sourcePath := dd.Data.DomainDataID
		if sourcePath == "" {
			sourcePath = pack.ModelID
		}
		isMock := strings.EqualFold(party.FeatureTableID, "mock")
		if party.IsMock != nil {
			isMock = *party.IsMock
		}
		endpoint := "mock"
		if !strings.EqualFold(party.FeatureTableID, "mock") && !(isMock && party.FeatureTableID == "") {
			var ft model.FeatureTableDO
			if s.db.WithContext(ctx).Where("feature_table_id = ? AND node_id = ? AND datasource_id = ?",
				party.FeatureTableID, party.NodeID, defaultHTTPDatasourceID).First(&ft).Error != nil {
				return padErr(errcode.SystemValidationError, party.FeatureTableID+" not exist")
			}
			endpoint = ft.URL
		}
		featureSource := map[string]any{}
		if isMock {
			featureSource["mock_opts"] = map[string]any{}
		} else {
			featureSource["http_opts"] = map[string]any{"endpoint": endpoint}
		}
		partyConfigs[party.NodeID] = map[string]any{
			"server_config": map[string]any{"feature_mapping": featureMapping},
			"model_config": map[string]any{
				"model_id":    pack.ModelID,
				"base_path":   "/",
				"source_path": sourcePath,
				"source_type": "ST_DP",
			},
			"feature_source_config": featureSource,
			"channel_desc":          map[string]any{"protocol": "http"},
		}
		sp := kuscia.ServingParty{DomainID: party.NodeID, AppImage: image}
		for _, r := range party.Resources {
			if err := validateServingResource(r, party.NodeID); err != nil {
				return err
			}
			sp.Resources = append(sp.Resources, kuscia.ServingResource{
				MinCPU: r.MinCPU, MaxCPU: r.MaxCPU, MinMemory: r.MinMemory, MaxMemory: r.MaxMemory,
			})
		}
		servingParties = append(servingParties, sp)
	}
	inputCfg, _ := json.Marshal(map[string]any{"party_configs": partyConfigs})
	servingInputConfig := string(inputCfg)

	servingID := graphjob.RandomLetters(8)
	reuse := pack.ModelStats == model.ModelStatsOffline && pack.ServingID != ""
	if reuse {
		servingID = pack.ServingID
	}

	filtered := make([]kuscia.ServingParty, 0, len(servingParties))
	partiesStr := make([]string, 0, len(servingParties))
	for _, sp := range servingParties {
		b, _ := json.Marshal(storedServingParty(sp))
		partiesStr = append(partiesStr, string(b))
		fp := sp
		fp.Resources = nil
		for _, r := range sp.Resources {
			minCPU, _ := strconv.ParseFloat(strings.TrimSpace(r.MinCPU), 64)
			maxCPU, _ := strconv.ParseFloat(strings.TrimSpace(r.MaxCPU), 64)
			minMem, _ := parseMemorySize(r.MinMemory)
			maxMem, _ := parseMemorySize(r.MaxMemory)
			if maxCPU == 0 && minCPU == maxCPU && maxMem == 0 && minMem == maxMem {
				s.log.Warn("invalid serving resource configuration, skipping it", zap.String("node", sp.DomainID))
				continue
			}
			minCPU = math.Max(minCPU, servingDefaultCPU)
			minMem = math.Max(minMem, servingDefaultMemory)
			maxCPU = math.Max(math.Max(maxCPU, servingDefaultCPU), minCPU)
			maxMem = math.Max(math.Max(maxMem, servingDefaultMemory), minMem)
			fp.Resources = append(fp.Resources, kuscia.ServingResource{
				MinCPU:    javaDouble(minCPU),
				MaxCPU:    javaDouble(maxCPU),
				MinMemory: javaDouble(minMem) + servingDefaultMemoryUnit,
				MaxMemory: javaDouble(maxMem) + servingDefaultMemoryUnit,
			})
		}
		filtered = append(filtered, fp)
	}

	initiator := servingParties[0].DomainID
	if s.isAutonomy() {
		if lp := s.localParty(nodeIDs); lp != "" {
			initiator = lp
		}
	}
	c := s.kc(initiator)
	if c == nil {
		return padErr(errcode.SystemError, "kuscia client not configured")
	}
	if err := c.CreateServing(ctx, &kuscia.CreateServingRequest{
		ServingID:          servingID,
		ServingInputConfig: servingInputConfig,
		Initiator:          initiator,
		Parties:            filtered,
	}); err != nil {
		return padErr(errcode.SystemError, "create serving failed: "+err.Error())
	}

	partiesJSON, _ := json.Marshal(partiesStr)
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		row := model.ProjectModelServingDO{
			ServingID:          servingID,
			ProjectID:          req.ProjectID,
			ServingStats:       "init",
			Parties:            string(partiesJSON),
			ServingInputConfig: servingInputConfig,
			Initiator:          initiator,
			PartyEndpoints:     "[]",
		}
		var existing model.ProjectModelServingDO
		if tx.Where("serving_id = ?", servingID).First(&existing).Error == nil {
			row.BaseDO = existing.BaseDO
			if err := tx.Save(&row).Error; err != nil {
				return err
			}
		} else if err := tx.Create(&row).Error; err != nil {
			return err
		}
		return tx.Model(&model.ProjectModelPackDO{}).Where("model_id = ?", pack.ModelID).
			Updates(map[string]any{"serving_id": servingID, "model_stats": model.ModelStatsPublishing}).Error
	})
}

// storedParty is the persisted ServingParty JSON (Java PartyDTO field names).
type storedParty struct {
	DomainID  string           `json:"domain_id"`
	AppImage  string           `json:"app_image"`
	Resources []storedResource `json:"resources"`
}

type storedResource struct {
	MinCPU    string `json:"min_cpu"`
	MaxCPU    string `json:"max_cpu"`
	MinMemory string `json:"min_memory"`
	MaxMemory string `json:"max_memory"`
}

func storedServingParty(sp kuscia.ServingParty) storedParty {
	out := storedParty{DomainID: sp.DomainID, AppImage: sp.AppImage, Resources: []storedResource{}}
	for _, r := range sp.Resources {
		out.Resources = append(out.Resources, storedResource{MinCPU: r.MinCPU, MaxCPU: r.MaxCPU, MinMemory: r.MinMemory, MaxMemory: r.MaxMemory})
	}
	return out
}

// javaDouble formats like Java String.valueOf(double) for plain values ("1.0", "0.5").
func javaDouble(v float64) string {
	if v == math.Trunc(v) && math.Abs(v) < 1e7 {
		return strconv.FormatFloat(v, 'f', 1, 64)
	}
	return strconv.FormatFloat(v, 'f', -1, 64)
}

// parseMemorySize mirrors JobUtils.parseMemorySize ("4Gi" -> 4).
func parseMemorySize(v string) (float64, error) {
	return strconv.ParseFloat(strings.TrimSpace(strings.ReplaceAll(v, "Gi", "")), 64)
}

// validateServingResource mirrors ModelManagementServiceImpl.validateResourceConfig.
func validateServingResource(r ServingResourceVO, nodeID string) error {
	minCPU, err1 := strconv.ParseFloat(strings.TrimSpace(r.MinCPU), 64)
	maxCPU, err2 := strconv.ParseFloat(strings.TrimSpace(r.MaxCPU), 64)
	if err1 != nil || err2 != nil {
		return padErr(errcode.SystemValidationError, "Invalid resource value for nodeId: "+nodeID)
	}
	if minCPU > maxCPU || maxCPU < 0 {
		return padErr(errcode.SystemValidationError, "CPU value setting error for nodeId: "+nodeID)
	}
	minMem, err1 := parseMemorySize(r.MinMemory)
	maxMem, err2 := parseMemorySize(r.MaxMemory)
	if err1 != nil || err2 != nil {
		return padErr(errcode.SystemValidationError, "Invalid memory value for nodeId: "+nodeID)
	}
	if minMem > maxMem || maxMem < 0 {
		return padErr(errcode.SystemValidationError, "Memory value setting error for nodeId: "+nodeID)
	}
	return nil
}

// --- model/serving/detail ---

var endpointServiceRe = regexp.MustCompile(`(-service\.).*(\.svc)`)

// QueryModelServingDetail mirrors ModelManagementServiceImpl.queryModelServingDetail.
func (s *ModelService) QueryModelServingDetail(ctx context.Context, servingID string) (*ServingDetailVO, error) {
	sv, err := s.findServing(ctx, servingID)
	if err != nil {
		return nil, err
	}
	var partyStrs []string
	_ = json.Unmarshal([]byte(sv.Parties), &partyStrs)
	parties := map[string]storedParty{}
	for _, ps := range partyStrs {
		var p storedParty
		if json.Unmarshal([]byte(ps), &p) == nil {
			parties[p.DomainID] = p
		}
	}
	var eps []partyEndpoint
	_ = json.Unmarshal([]byte(sv.PartyEndpoints), &eps)
	template := ""
	epMap := map[string]string{}
	for _, e := range eps {
		if e.Endpoints != "" {
			template = endpointServiceRe.ReplaceAllString(e.Endpoints, "${1}#${2}")
		}
		epMap[e.NodeID] = e.Endpoints
	}

	var cfg struct {
		PartyConfigs map[string]struct {
			ServerConfig struct {
				FeatureMapping map[string]string `json:"feature_mapping"`
			} `json:"server_config"`
			ModelConfig struct {
				ModelID    string `json:"model_id"`
				SourcePath string `json:"source_path"`
			} `json:"model_config"`
			FeatureSourceConfig struct {
				HTTPOpts *struct {
					Endpoint string `json:"endpoint"`
				} `json:"http_opts"`
			} `json:"feature_source_config"`
		} `json:"party_configs"`
	}
	if err := json.Unmarshal([]byte(sv.ServingInputConfig), &cfg); err != nil {
		return nil, padErr(errcode.SystemError, "invalid serving input config: "+err.Error())
	}
	keys := make([]string, 0, len(cfg.PartyConfigs))
	for k := range cfg.PartyConfigs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	names := s.nodeNames(ctx, keys)
	vo := &ServingDetailVO{ServingID: sv.ServingID, ServingDetails: []ServingDetailItemVO{}}
	for _, k := range keys {
		pc := cfg.PartyConfigs[k]
		ep := epMap[k]
		if ep == "" {
			ep = strings.ReplaceAll(template, "#", k)
		}
		resources := []ServingResourceVO{}
		for _, r := range parties[k].Resources {
			resources = append(resources, ServingResourceVO{MinCPU: r.MinCPU, MaxCPU: r.MaxCPU, MinMemory: r.MinMemory, MaxMemory: r.MaxMemory})
		}
		httpEndpoint := ""
		if pc.FeatureSourceConfig.HTTPOpts != nil {
			httpEndpoint = pc.FeatureSourceConfig.HTTPOpts.Endpoint
		}
		isMock := httpEndpoint == ""
		fm := pc.ServerConfig.FeatureMapping
		if fm == nil {
			fm = map[string]string{}
		}
		name := names[k]
		if name == "" {
			name = k
		}
		vo.ServingDetails = append(vo.ServingDetails, ServingDetailItemVO{
			NodeID:          k,
			NodeName:        name,
			Endpoints:       ep,
			FeatureHTTP:     httpEndpoint,
			IsMock:          &isMock,
			FeatureMappings: fm,
			SourcePath:      pc.ModelConfig.SourcePath,
			Resources:       resources,
		})
		vo.ModelID = pc.ModelConfig.ModelID
	}
	return vo, nil
}

// --- model/serving/delete ---

// DeleteModelServing mirrors ModelManagementServiceImpl.deleteModelServing:
// the Kuscia serving is deleted, the model goes OFFLINE and the serving row is kept.
func (s *ModelService) DeleteModelServing(ctx context.Context, servingID string) error {
	sv, err := s.findServing(ctx, servingID)
	if err != nil {
		return err
	}
	var pack model.ProjectModelPackDO
	if s.db.WithContext(ctx).Where("serving_id = ? AND is_deleted = 0", servingID).First(&pack).Error != nil {
		return padErrBase(ErrModelNotFound, errcode.ProjectModelNotFound)
	}
	c := s.kc(sv.Initiator)
	if c == nil {
		return padErr(errcode.SystemError, "kuscia client not configured")
	}
	if err := c.DeleteServing(ctx, servingID); err != nil && !kuscia.IsNotFound(err) {
		return padErr(errcode.SystemError, "delete serving failed: "+err.Error())
	}
	return s.db.WithContext(ctx).Model(&model.ProjectModelPackDO{}).Where("model_id = ?", pack.ModelID).
		Update("model_stats", model.ModelStatsOffline).Error
}

// --- model/discard, model/delete ---

// DiscardModelPack mirrors ModelManagementServiceImpl.discardModelPack.
func (s *ModelService) DiscardModelPack(ctx context.Context, modelID string) error {
	pack, err := s.findPack(ctx, modelID)
	if err != nil {
		return err
	}
	if pack.ModelStats != model.ModelStatsOffline && pack.ModelStats != model.ModelStatsInit {
		return padErr(errcode.ProjectServingNotOffline)
	}
	return s.db.WithContext(ctx).Model(&model.ProjectModelPackDO{}).Where("model_id = ?", modelID).
		Update("model_stats", model.ModelStatsDiscarded).Error
}

// DeleteModelPack mirrors ModelManagementServiceImpl.deleteModelPack (soft delete).
func (s *ModelService) DeleteModelPack(ctx context.Context, nodeID, modelID string) error {
	pack, err := s.findPack(ctx, modelID)
	if err != nil {
		return err
	}
	if pack.ModelStats != model.ModelStatsDiscarded {
		return padErr(errcode.ProjectServingNotOffline)
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.ProjectModelPackDO{}).Where("id = ?", pack.ID).Update("is_deleted", 1).Error; err != nil {
			return err
		}
		return tx.Delete(&model.ProjectModelPackDO{}, pack.ID).Error
	})
}
