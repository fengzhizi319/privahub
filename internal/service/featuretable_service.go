package service

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"

	"gorm.io/gorm"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/pkg/errcode"
)

// FeatureTableService implements Java FeatureTableServiceImpl (HTTP feature
// datasources stored in feature_table).
type FeatureTableService struct {
	db       *gorm.DB
	ipFilter *IPFilter
}

// NewFeatureTableService creates a new FeatureTableService (Java default IP block list).
func NewFeatureTableService(db *gorm.DB) *FeatureTableService {
	return &FeatureTableService{db: db, ipFilter: NewIPFilter(nil)}
}

// WithIPFilter sets the SSRF filter applied to feature service URLs.
func (s *FeatureTableService) WithIPFilter(f *IPFilter) *FeatureTableService {
	if f == nil {
		f = NewDisabledIPFilter()
	}
	s.ipFilter = f
	return s
}

// --- DTOs (Java field names) ---

// CreateFeatureDatasourceRequest is Java CreateFeatureDatasourceRequest.
type CreateFeatureDatasourceRequest struct {
	OwnerID          string              `json:"ownerId"`
	NodeIDs          []string            `json:"nodeIds"`
	FeatureTableName string              `json:"featureTableName"`
	Type             string              `json:"type"`
	Desc             string              `json:"desc"`
	URL              string              `json:"url"`
	Columns          []TableColumnCompat `json:"columns"`
	DatasourceID     string              `json:"datasourceId"`
}

// ListProjectFeatureDatasourceRequest is Java ListProjectFeatureDatasourceRequest.
type ListProjectFeatureDatasourceRequest struct {
	ProjectID string `json:"projectId"`
	NodeID    string `json:"nodeId"`
}

// FeatureDataSourceVO is Java FeatureDataSourceVO.
type FeatureDataSourceVO struct {
	NodeID           string              `json:"nodeId"`
	FeatureTableID   string              `json:"featureTableId"`
	FeatureTableName string              `json:"featureTableName"`
	Columns          []TableColumnCompat `json:"columns"`
}

func validateFeatureRequest(req *CreateFeatureDatasourceRequest) error {
	switch {
	case req.OwnerID == "":
		return bizErr(errcode.ParamError, "ownerId cannot be blank")
	case len(req.NodeIDs) == 0:
		return bizErr(errcode.ParamError, "nodeIds cannot be empty")
	case strings.TrimSpace(req.FeatureTableName) == "":
		return bizErr(errcode.ParamError, "featureTableName cannot be blank")
	case len([]rune(req.FeatureTableName)) > 32:
		return bizErr(errcode.ParamError, "feature table must less then 32 characters")
	case !strings.EqualFold(req.Type, DatasourceTypeHTTP):
		return bizErr(errcode.ParamError, "type must be HTTP")
	case len([]rune(req.Desc)) > 100:
		return bizErr(errcode.ParamError, "feature table desc must less then 100 characters")
	case strings.TrimSpace(req.URL) == "":
		return bizErr(errcode.ParamError, "URL cannot be blank")
	case len(req.Columns) == 0:
		return bizErr(errcode.ParamError, "columns cannot be empty")
	}
	u, err := url.ParseRequestURI(req.URL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return bizErr(errcode.ParamError, "Invalid URL format")
	}
	for _, n := range req.NodeIDs {
		if strings.TrimSpace(n) == "" {
			return bizErr(errcode.ParamError, "nodeIds cannot contain blank node id")
		}
	}
	return nil
}

// CreateFeatureTable mirrors Java createFeatureTable: IP-filters the URL and
// writes one UnAvailable HTTP feature_table row per node (random 8-letter id).
func (s *FeatureTableService) CreateFeatureTable(ctx context.Context, req *CreateFeatureDatasourceRequest) error {
	if err := validateFeatureRequest(req); err != nil {
		return err
	}
	if err := s.ipFilter.CheckURL(req.URL); err != nil {
		return err
	}
	dsID := req.DatasourceID
	if strings.TrimSpace(dsID) == "" {
		dsID = DefaultHTTPDatasourceID
	}
	cols, _ := json.Marshal(req.Columns)
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, nodeID := range req.NodeIDs {
			if err := tx.Create(&model.FeatureTableDO{
				FeatureTableID:   randomLetters(8),
				FeatureTableName: req.FeatureTableName,
				NodeID:           nodeID,
				DatasourceID:     dsID,
				Type:             strings.ToUpper(req.Type),
				Description:      req.Desc,
				URL:              req.URL,
				Columns:          string(cols),
				Status:           StatusUnavailable,
			}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// FeatureDatasourceList lists feature datasources of a node.
func (s *FeatureTableService) FeatureDatasourceList(ctx context.Context, nodeID string) ([]FeatureDataSourceVO, error) {
	var tables []model.FeatureTableDO
	if err := s.db.WithContext(ctx).Where("node_id = ?", nodeID).Order("gmt_create DESC").Find(&tables).Error; err != nil {
		return nil, err
	}
	out := make([]FeatureDataSourceVO, 0, len(tables))
	for _, ft := range tables {
		out = append(out, FeatureDataSourceVO{
			NodeID: ft.NodeID, FeatureTableID: ft.FeatureTableID, FeatureTableName: ft.FeatureTableName,
			Columns: parseColumns(ft.Columns),
		})
	}
	return out, nil
}

// ProjectFeatureTableList mirrors Java projectFeatureTableList: feature tables
// authorized to a project on a node (project_feature_table join feature_table).
func (s *FeatureTableService) ProjectFeatureTableList(ctx context.Context, nodeID, projectID string) ([]FeatureDataSourceVO, error) {
	var pfts []model.ProjectFeatureTableDO
	if err := s.db.WithContext(ctx).Where("node_id = ? AND project_id = ?", nodeID, projectID).Find(&pfts).Error; err != nil {
		return nil, err
	}
	out := make([]FeatureDataSourceVO, 0, len(pfts))
	if len(pfts) == 0 {
		return out, nil
	}
	ids := make([]string, 0, len(pfts))
	for _, p := range pfts {
		ids = append(ids, p.FeatureTableID)
	}
	var tables []model.FeatureTableDO
	if err := s.db.WithContext(ctx).Where("feature_table_id IN ?", ids).Find(&tables).Error; err != nil {
		return nil, err
	}
	byKey := map[string]*model.FeatureTableDO{}
	for i := range tables {
		byKey[tables[i].FeatureTableID+"|"+tables[i].NodeID] = &tables[i]
	}
	for _, p := range pfts {
		vo := FeatureDataSourceVO{NodeID: p.NodeID, FeatureTableID: p.FeatureTableID, Columns: []TableColumnCompat{}}
		if ft, ok := byKey[p.FeatureTableID+"|"+p.NodeID]; ok {
			vo.FeatureTableName = ft.FeatureTableName
			vo.Columns = parseColumns(ft.Columns)
		}
		out = append(out, vo)
	}
	return out, nil
}
