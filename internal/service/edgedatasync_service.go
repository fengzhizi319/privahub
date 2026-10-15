package service

import (
	"context"
	"time"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// EdgeDataSyncService handles edge data synchronization state tracking.
type EdgeDataSyncService struct {
	db *gorm.DB
}

// NewEdgeDataSyncService creates a new EdgeDataSyncService.
func NewEdgeDataSyncService(db *gorm.DB) *EdgeDataSyncService {
	return &EdgeDataSyncService{db: db}
}

// --- DTOs ---

// SyncLogVO represents a sync log entry.
type SyncLogVO struct {
	TableName      string `json:"table_name"`
	LastUpdateTime string `json:"last_update_time"`
}

// --- Service Methods ---

// GetSyncLogs returns all edge data sync log entries.
func (s *EdgeDataSyncService) GetSyncLogs(ctx context.Context) ([]SyncLogVO, error) {
	var logs []model.EdgeDataSyncLogDO
	if err := s.db.WithContext(ctx).Find(&logs).Error; err != nil {
		return nil, err
	}

	result := make([]SyncLogVO, 0, len(logs))
	for _, l := range logs {
		result = append(result, SyncLogVO{
			TableName:      l.SyncTableName,
			LastUpdateTime: l.LastUpdateTime,
		})
	}
	return result, nil
}

// UpsertSyncLog creates or updates a sync log entry for a table.
// A single atomic upsert is safe when several sync streams checkpoint the
// same table concurrently; a read-then-create transaction can still race.
func (s *EdgeDataSyncService) UpsertSyncLog(ctx context.Context, tableName string) error {
	now := time.Now().Format("2006-01-02 15:04:05")
	return s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "table_name"}},
		DoUpdates: clause.AssignmentColumns([]string{"last_update_time"}),
	}).Create(&model.EdgeDataSyncLogDO{SyncTableName: tableName, LastUpdateTime: now}).Error
}

// GetLastSyncTime returns the last sync time for a specific table.
func (s *EdgeDataSyncService) GetLastSyncTime(ctx context.Context, tableName string) (string, error) {
	var log model.EdgeDataSyncLogDO
	if err := s.db.WithContext(ctx).Where("table_name = ?", tableName).First(&log).Error; err != nil {
		return "", err
	}
	return log.LastUpdateTime, nil
}
