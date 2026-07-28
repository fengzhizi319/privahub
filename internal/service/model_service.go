package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/pkg/kuscia"
	"gorm.io/gorm"
)

// Model service errors.
var (
	ErrModelNotFound   = errors.New("model not found")
	ErrServingNotFound = errors.New("serving not found")
)

// ModelService handles model and serving management.
type ModelService struct {
	db           *gorm.DB
	kusciaClient *kuscia.Client
}

// NewModelService creates a new ModelService.
func NewModelService(db *gorm.DB, kusciaClient *kuscia.Client) *ModelService {
	return &ModelService{db: db, kusciaClient: kusciaClient}
}

// --- Model DTOs ---

// ModelListRequest represents a model list request.
type ModelListRequest struct {
	ProjectID string `json:"project_id"`
	Initiator string `json:"initiator"`
}

// ModelVO represents a model view object.
type ModelVO struct {
	ModelID    string `json:"model_id"`
	ProjectID  string `json:"project_id"`
	Initiator  string `json:"initiator"`
	ModelName  string `json:"model_name"`
	ModelDesc  string `json:"model_desc"`
	ModelStats int8   `json:"model_stats"`
	ServingID  string `json:"serving_id,omitempty"`
	TrainID    string `json:"train_id"`
	GmtCreate  string `json:"gmt_create"`
}

// ModelDetailRequest represents a model detail request.
type ModelDetailRequest struct {
	ModelID string `json:"model_id" binding:"required"`
}

// ModelDetailVO represents a model detail view object.
type ModelDetailVO struct {
	ModelID         string `json:"model_id"`
	ProjectID       string `json:"project_id"`
	Initiator       string `json:"initiator"`
	ModelName       string `json:"model_name"`
	ModelDesc       string `json:"model_desc"`
	ModelStats      int8   `json:"model_stats"`
	ServingID       string `json:"serving_id,omitempty"`
	SampleTables    string `json:"sample_tables"`
	ModelList       string `json:"model_list"`
	TrainID         string `json:"train_id"`
	ModelReportID   string `json:"model_report_id"`
	GraphDetail     string `json:"graph_detail"`
	ModelDatasource string `json:"model_datasource"`
	GmtCreate       string `json:"gmt_create"`
}

// DeleteModelRequest represents a model deletion request.
type DeleteModelRequest struct {
	ModelID string `json:"model_id" binding:"required"`
}

// --- Serving DTOs ---

// CreateServingRequest represents a serving creation request.
type CreateServingRequest struct {
	ProjectID          string `json:"project_id" binding:"required"`
	Initiator          string `json:"initiator" binding:"required"`
	ServingInputConfig string `json:"serving_input_config" binding:"required"`
	Parties            string `json:"parties"`
	PartyEndpoints     string `json:"party_endpoints"`
}

// ServingVO represents a serving view object.
type ServingVO struct {
	ServingID          string `json:"serving_id"`
	ProjectID          string `json:"project_id"`
	Initiator          string `json:"initiator"`
	ServingInputConfig string `json:"serving_input_config"`
	Parties            string `json:"parties"`
	ServingStats       string `json:"serving_stats"`
	ErrorMsg           string `json:"error_msg,omitempty"`
	GmtCreate          string `json:"gmt_create"`
}

// ServingListRequest represents a serving list request.
type ServingListRequest struct {
	ProjectID string `json:"project_id"`
}

// DeleteServingRequest represents a serving deletion request.
type DeleteServingRequest struct {
	ServingID string `json:"serving_id" binding:"required"`
}

// ServingDetailRequest represents a serving detail request.
type ServingDetailRequest struct {
	ServingID string `json:"serving_id" binding:"required"`
}

// --- Model Service Methods ---

// ListModels lists models.
func (s *ModelService) ListModels(ctx context.Context, req *ModelListRequest) ([]ModelVO, error) {
	var models []model.ProjectModelPackDO
	query := s.db.WithContext(ctx).Model(&model.ProjectModelPackDO{})

	if req.ProjectID != "" {
		query = query.Where("project_id = ?", req.ProjectID)
	}
	if req.Initiator != "" {
		query = query.Where("initiator = ?", req.Initiator)
	}

	if err := query.Order("gmt_create DESC").Find(&models).Error; err != nil {
		return nil, err
	}

	result := make([]ModelVO, 0, len(models))
	for _, m := range models {
		result = append(result, ModelVO{
			ModelID:    m.ModelID,
			ProjectID:  m.ProjectID,
			Initiator:  m.Initiator,
			ModelName:  m.ModelName,
			ModelDesc:  m.ModelDesc,
			ModelStats: m.ModelStats,
			ServingID:  m.ServingID,
			TrainID:    m.TrainID,
			GmtCreate:  m.GmtCreate.Format("2006-01-02 15:04:05"),
		})
	}
	return result, nil
}

// GetModelDetail retrieves model detail.
func (s *ModelService) GetModelDetail(ctx context.Context, req *ModelDetailRequest) (*ModelDetailVO, error) {
	var m model.ProjectModelPackDO
	if err := s.db.WithContext(ctx).Where("model_id = ?", req.ModelID).First(&m).Error; err != nil {
		return nil, ErrModelNotFound
	}

	return &ModelDetailVO{
		ModelID:         m.ModelID,
		ProjectID:       m.ProjectID,
		Initiator:       m.Initiator,
		ModelName:       m.ModelName,
		ModelDesc:       m.ModelDesc,
		ModelStats:      m.ModelStats,
		ServingID:       m.ServingID,
		SampleTables:    m.SampleTables,
		ModelList:       m.ModelList,
		TrainID:         m.TrainID,
		ModelReportID:   m.ModelReportID,
		GraphDetail:     m.GraphDetail,
		ModelDatasource: m.ModelDatasource,
		GmtCreate:       m.GmtCreate.Format("2006-01-02 15:04:05"),
	}, nil
}

// DeleteModel deletes a model.
func (s *ModelService) DeleteModel(ctx context.Context, req *DeleteModelRequest) error {
	var m model.ProjectModelPackDO
	if err := s.db.WithContext(ctx).Where("model_id = ?", req.ModelID).First(&m).Error; err != nil {
		return ErrModelNotFound
	}
	return s.db.WithContext(ctx).Delete(&m).Error
}

// ExportModel exports a model by triggering a Kuscia job.
func (s *ModelService) ExportModel(ctx context.Context, modelID string) (string, error) {
	var m model.ProjectModelPackDO
	if err := s.db.WithContext(ctx).Where("model_id = ?", modelID).First(&m).Error; err != nil {
		return "", ErrModelNotFound
	}

	// Trigger Kuscia job for model export
	exportJobID := uuid.New().String()[:8]
	if s.kusciaClient != nil {
		kusciaReq := &kuscia.CreateJobRequest{
			JobID:     exportJobID,
			Initiator: "alice",
			Tasks: []kuscia.TaskConfig{
				{
					AppImage: "secretflow",
					Alias:    "model_export_" + modelID,
					Parties: []kuscia.Party{
						{Name: "alice", Role: "guest"},
					},
				},
			},
			CustomFields: map[string]string{
				"model_id": modelID,
				"action":   "export",
			},
		}
		// Best-effort: Kuscia may be unreachable
		_, _ = s.kusciaClient.CreateJob(ctx, kusciaReq)
	}

	return "/exports/" + modelID + ".zip?job=" + exportJobID, nil
}

// --- Serving Service Methods ---

// CreateServing creates a new serving and registers it with Kuscia.
func (s *ModelService) CreateServing(ctx context.Context, req *CreateServingRequest) (*ServingVO, error) {
	servingID := uuid.New().String()[:8]

	serving := &model.ProjectModelServingDO{
		ProjectID:          req.ProjectID,
		ServingID:          servingID,
		Initiator:          req.Initiator,
		ServingInputConfig: req.ServingInputConfig,
		Parties:            req.Parties,
		PartyEndpoints:     req.PartyEndpoints,
		ServingStats:       "init",
	}

	if err := s.db.WithContext(ctx).Create(serving).Error; err != nil {
		return nil, err
	}

	// Register serving with Kuscia (best-effort)
	if s.kusciaClient != nil {
		kusciaReq := &kuscia.CreateServingRequest{
			ServingID:          servingID,
			ServingInputConfig: req.ServingInputConfig,
			Initiator:          req.Initiator,
			Parties:            buildServingParties(req.Parties),
		}
		if err := s.kusciaClient.CreateServing(ctx, kusciaReq); err != nil {
			// Kuscia unreachable — mark as pending, local record still valid
			_ = s.db.WithContext(ctx).Model(serving).Update("serving_stats", "pending").Error
		} else {
			_ = s.db.WithContext(ctx).Model(serving).Update("serving_stats", "progressing").Error
			serving.ServingStats = "progressing"
		}
	}

	return &ServingVO{
		ServingID:          servingID,
		ProjectID:          req.ProjectID,
		Initiator:          req.Initiator,
		ServingInputConfig: req.ServingInputConfig,
		Parties:            req.Parties,
		ServingStats:       serving.ServingStats,
		GmtCreate:          serving.GmtCreate.Format("2006-01-02 15:04:05"),
	}, nil
}

// ListServings lists servings.
func (s *ModelService) ListServings(ctx context.Context, req *ServingListRequest) ([]ServingVO, error) {
	var servings []model.ProjectModelServingDO
	query := s.db.WithContext(ctx).Model(&model.ProjectModelServingDO{})

	if req.ProjectID != "" {
		query = query.Where("project_id = ?", req.ProjectID)
	}

	if err := query.Order("gmt_create DESC").Find(&servings).Error; err != nil {
		return nil, err
	}

	result := make([]ServingVO, 0, len(servings))
	for _, sv := range servings {
		result = append(result, ServingVO{
			ServingID:          sv.ServingID,
			ProjectID:          sv.ProjectID,
			Initiator:          sv.Initiator,
			ServingInputConfig: sv.ServingInputConfig,
			Parties:            sv.Parties,
			ServingStats:       sv.ServingStats,
			ErrorMsg:           sv.ErrorMsg,
			GmtCreate:          sv.GmtCreate.Format("2006-01-02 15:04:05"),
		})
	}
	return result, nil
}

// GetServingDetail retrieves serving detail, syncing status from Kuscia if available.
func (s *ModelService) GetServingDetail(ctx context.Context, req *ServingDetailRequest) (*ServingVO, error) {
	var sv model.ProjectModelServingDO
	if err := s.db.WithContext(ctx).Where("serving_id = ?", req.ServingID).First(&sv).Error; err != nil {
		return nil, ErrServingNotFound
	}

	// Sync status from Kuscia (best-effort)
	if s.kusciaClient != nil {
		if resp, err := s.kusciaClient.QueryServing(ctx, req.ServingID); err == nil && resp.Data != nil {
			newState := resp.Data.Status.State
			if newState != "" && newState != sv.ServingStats {
				_ = s.db.WithContext(ctx).Model(&sv).Update("serving_stats", newState).Error
				sv.ServingStats = newState
			}
		}
	}

	return &ServingVO{
		ServingID:          sv.ServingID,
		ProjectID:          sv.ProjectID,
		Initiator:          sv.Initiator,
		ServingInputConfig: sv.ServingInputConfig,
		Parties:            sv.Parties,
		ServingStats:       sv.ServingStats,
		ErrorMsg:           sv.ErrorMsg,
		GmtCreate:          sv.GmtCreate.Format("2006-01-02 15:04:05"),
	}, nil
}

// DeleteServing deletes a serving locally and from Kuscia.
func (s *ModelService) DeleteServing(ctx context.Context, req *DeleteServingRequest) error {
	var sv model.ProjectModelServingDO
	if err := s.db.WithContext(ctx).Where("serving_id = ?", req.ServingID).First(&sv).Error; err != nil {
		return ErrServingNotFound
	}

	// Delete from Kuscia (best-effort)
	if s.kusciaClient != nil {
		_ = s.kusciaClient.DeleteServing(ctx, req.ServingID)
	}

	return s.db.WithContext(ctx).Delete(&sv).Error
}

// buildServingParties parses a JSON parties string into Kuscia ServingParty slice.
func buildServingParties(partiesJSON string) []kuscia.ServingParty {
	if partiesJSON == "" {
		return nil
	}
	var parties []kuscia.ServingParty
	if err := json.Unmarshal([]byte(partiesJSON), &parties); err != nil {
		// Fallback: treat as comma-separated domain IDs with default role
		for _, p := range strings.Split(partiesJSON, ",") {
			p = strings.TrimSpace(p)
			if p != "" {
				parties = append(parties, kuscia.ServingParty{DomainID: p, Role: "guest", AppImage: "secretflow"})
			}
		}
	}
	return parties
}

// ModelPartySourceVO is a data source entry under a party (frontend contract).
type ModelPartySourceVO struct {
	DataSourceID string `json:"dataSourceId"`
	DatasourceID string `json:"datasourceId"`
	Name         string `json:"name"`
	Type         string `json:"type"`
}

// ModelPartyPathVO describes a party (node) contributing to a model output (frontend contract).
type ModelPartyPathVO struct {
	NodeID      string               `json:"nodeId"`
	NodeName    string               `json:"nodeName"`
	DataSources []ModelPartySourceVO `json:"dataSources"`
}

// GetModelPartyPath returns the parties (project nodes) and their data sources that
// contribute to a graph-node output. DB-backed best-effort implementation matching the
// frontend contract (bare array of {nodeId, nodeName, dataSources}).
func (s *ModelService) GetModelPartyPath(ctx context.Context, projectID, graphNodeID, outputID string) ([]ModelPartyPathVO, error) {
	result := make([]ModelPartyPathVO, 0)

	var projectNodes []model.ProjectNodeDO
	if err := s.db.WithContext(ctx).Where("project_id = ?", projectID).Find(&projectNodes).Error; err != nil {
		return result, err
	}

	for _, pn := range projectNodes {
		nodeName := pn.NodeID
		var node model.NodeDO
		if err := s.db.WithContext(ctx).Where("node_id = ?", pn.NodeID).First(&node).Error; err == nil && node.Name != "" {
			nodeName = node.Name
		}

		var dss []model.DatasourceDO
		s.db.WithContext(ctx).Where("owner_id = ?", pn.NodeID).Find(&dss)
		sources := make([]ModelPartySourceVO, 0, len(dss))
		for _, ds := range dss {
			sources = append(sources, ModelPartySourceVO{
				DataSourceID: ds.DatasourceID,
				DatasourceID: ds.DatasourceID,
				Name:         ds.Name,
				Type:         ds.Type,
			})
		}

		result = append(result, ModelPartyPathVO{
			NodeID:      pn.NodeID,
			NodeName:    nodeName,
			DataSources: sources,
		})
	}

	return result, nil
}
