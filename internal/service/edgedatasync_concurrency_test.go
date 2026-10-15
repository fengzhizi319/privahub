// Copyright 2026 Privahub Authors. Licensed under the Apache License, Version 2.0.
package service

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// Exercise multiple real connections: a single-connection in-memory DB would
// serialize read/create transactions and hide their duplicate-key race.
func TestEdgeDataSync_ConcurrentCheckpoints(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "sync.db")+"?_busy_timeout=10000&_journal_mode=WAL"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	defer sqlDB.Close()
	sqlDB.SetMaxOpenConns(8)
	require.NoError(t, db.AutoMigrate(&model.EdgeDataSyncLogDO{}))
	svc := NewEdgeDataSyncService(db)
	start := make(chan struct{})
	results := make(chan error, 24)
	for i := 0; i < cap(results); i++ {
		go func() { <-start; results <- svc.UpsertSyncLog(context.Background(), "project_graph") }()
	}
	close(start)
	for i := 0; i < cap(results); i++ {
		require.NoError(t, <-results)
	}
	var entries []model.EdgeDataSyncLogDO
	require.NoError(t, db.Find(&entries).Error)
	require.Len(t, entries, 1)
	require.Equal(t, "project_graph", entries[0].SyncTableName)
	require.NotEmpty(t, entries[0].LastUpdateTime)
}
