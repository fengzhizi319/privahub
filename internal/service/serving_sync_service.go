package service

import (
	"context"
	"time"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/pkg/kuscia"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// ServingStatusSyncService periodically polls Kuscia for serving status updates
// and synchronizes them to the local database.
type ServingStatusSyncService struct {
	db           *gorm.DB
	kusciaClient *kuscia.Client
	log          *zap.Logger
	interval     time.Duration
	stopCh       chan struct{}
}

// NewServingStatusSyncService creates a new ServingStatusSyncService.
func NewServingStatusSyncService(db *gorm.DB, kusciaClient *kuscia.Client, log *zap.Logger) *ServingStatusSyncService {
	return &ServingStatusSyncService{
		db:           db,
		kusciaClient: kusciaClient,
		log:          log,
		interval:     15 * time.Second,
		stopCh:       make(chan struct{}),
	}
}

// Start begins the background sync loop.
func (s *ServingStatusSyncService) Start() {
	if s.kusciaClient == nil {
		return
	}
	go s.loop()
}

// Stop gracefully stops the sync loop.
func (s *ServingStatusSyncService) Stop() {
	close(s.stopCh)
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

// syncOnce performs a single sync cycle: find active servings, query Kuscia, update DB.
func (s *ServingStatusSyncService) syncOnce() {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	// Find all non-terminal servings
	var activeServings []model.ProjectModelServingDO
	if err := s.db.WithContext(ctx).
		Where("serving_stats IN ?", []string{"init", "pending", "progressing", "partial_available"}).
		Find(&activeServings).Error; err != nil {
		return
	}

	if len(activeServings) == 0 {
		return
	}

	// Batch query Kuscia for serving statuses
	servingIDs := make([]string, 0, len(activeServings))
	for _, sv := range activeServings {
		servingIDs = append(servingIDs, sv.ServingID)
	}

	entries, err := s.kusciaClient.BatchQueryServingStatus(ctx, servingIDs)
	if err != nil {
		// Kuscia unreachable — skip this cycle
		return
	}

	// Build lookup map
	statusMap := make(map[string]string, len(entries))
	for _, e := range entries {
		statusMap[e.ServingID] = mapServingState(e.Status.State)
	}

	// Update local DB for servings whose status changed
	for _, sv := range activeServings {
		newState, ok := statusMap[sv.ServingID]
		if !ok || newState == sv.ServingStats {
			continue
		}

		s.db.WithContext(ctx).Model(&model.ProjectModelServingDO{}).
			Where("serving_id = ?", sv.ServingID).
			Update("serving_stats", newState)

		if s.log != nil {
			s.log.Info("Serving status synced from Kuscia",
				zap.String("serving_id", sv.ServingID),
				zap.String("old_status", sv.ServingStats),
				zap.String("new_status", newState),
			)
		}
	}
}

// mapServingState maps Kuscia serving states to SecretPad status strings.
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
