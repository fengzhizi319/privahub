package service

import (
	"context"
	"time"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"gorm.io/gorm"
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
// Uses a transaction to prevent race conditions on concurrent upserts.
func (s *EdgeDataSyncService) UpsertSyncLog(ctx context.Context, tableName string) error {
	now := time.Now().Format("2006-01-02 15:04:05")
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing model.EdgeDataSyncLogDO
		err := tx.Where("table_name = ?", tableName).First(&existing).Error
		if err == nil {
			return tx.Model(&existing).
				Update("last_update_time", now).Error
		}

		log := &model.EdgeDataSyncLogDO{
			SyncTableName:  tableName,
			LastUpdateTime: now,
		}
		return tx.Create(log).Error
	})
}

// GetLastSyncTime returns the last sync time for a specific table.
func (s *EdgeDataSyncService) GetLastSyncTime(ctx context.Context, tableName string) (string, error) {
	var log model.EdgeDataSyncLogDO
	if err := s.db.WithContext(ctx).Where("table_name = ?", tableName).First(&log).Error; err != nil {
		return "", err
	}
	return log.LastUpdateTime, nil
}
