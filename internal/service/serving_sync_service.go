package service

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/pkg/kuscia"
)

// ServingStatusSyncService periodically polls Kuscia for the servings of
// PUBLISHING models and moves them to PUBLISHED / PUBLISH_FAIL, updating both
// project_model_serving.serving_stats and project_model_pack.model_stats
// (Java refreshes on model/page only; the loop keeps the state fresh).
type ServingStatusSyncService struct {
	db           *gorm.DB
	kusciaClient *kuscia.Client
	models       *ModelService
	log          *zap.Logger
	interval     time.Duration
	stopCh       chan struct{}
	stopOnce     sync.Once // guards against double-close of stopCh
}

// NewServingStatusSyncService creates a new ServingStatusSyncService.
func NewServingStatusSyncService(db *gorm.DB, kusciaClient *kuscia.Client, log *zap.Logger) *ServingStatusSyncService {
	if log == nil {
		log = zap.NewNop()
	}
	return &ServingStatusSyncService{
		db:           db,
		kusciaClient: kusciaClient,
		log:          log,
		interval:     15 * time.Second,
		stopCh:       make(chan struct{}),
	}
}

// WithModelService shares the model service (pool / env aware Kuscia routing).
func (s *ServingStatusSyncService) WithModelService(m *ModelService) *ServingStatusSyncService {
	s.models = m
	return s
}

func (s *ServingStatusSyncService) modelService() *ModelService {
	if s.models != nil {
		return s.models
	}
	return NewModelService(s.db, s.kusciaClient).WithLogger(s.log)
}

// Start begins the background sync loop.
func (s *ServingStatusSyncService) Start() {
	if s.kusciaClient == nil && s.models == nil {
		return
	}
	go s.loop()
}

// Stop gracefully stops the sync loop. It is safe to call multiple times.
func (s *ServingStatusSyncService) Stop() {
	s.stopOnce.Do(func() {
		close(s.stopCh)
	})
}

func (s *ServingStatusSyncService) loop() {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.syncOnce()
		}
	}
}

// syncOnce refreshes every PUBLISHING model once.
func (s *ServingStatusSyncService) syncOnce() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	s.SyncOnce(ctx)
}

// SyncOnce refreshes every PUBLISHING model once and returns how many changed.
func (s *ServingStatusSyncService) SyncOnce(ctx context.Context) int {
	var packs []model.ProjectModelPackDO
	if err := s.db.WithContext(ctx).
		Where("model_stats = ? AND serving_id <> '' AND is_deleted = 0", model.ModelStatsPublishing).
		Find(&packs).Error; err != nil {
		return 0
	}
	ms := s.modelService()
	changed := 0
	for i := range packs {
		if ms.RefreshPublishing(ctx, &packs[i]) {
			changed++
		}
	}
	return changed
}

// mapServingState maps Kuscia serving states to lower-case status strings.
func mapServingState(state string) string {
	switch state {
	case "Pending", "pending":
		return "pending"
	case "Progressing", "progressing":
		return "progressing"
	case "PartialAvailable", "partial_available":
		return "partial_available"
	case "Available", "available":
		return "available"
	case "Failed", "failed":
		return "failed"
	default:
		return state
	}
}
