// Copyright 2024 Ant Group Co., Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package service

import (
	"context"
	"testing"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// setupDatasourceTestDB creates an in-memory SQLite DB with datasource tables.
func setupDatasourceTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)

	err = db.AutoMigrate(
		&model.DatasourceDO{},
		&model.DatasourceNodeDO{},
		&model.NodeDO{},
	)
	require.NoError(t, err)
	return db
}

// TestDatasourceService_CreateAtomic verifies that datasource creation with
// node associations is atomic: all-or-nothing (Bug 28).
func TestDatasourceService_CreateAtomic(t *testing.T) {
	db := setupDatasourceTestDB(t)
	svc := NewDatasourceService(db, nil)
	ctx := context.Background()

	// Create datasource with node associations
	vo, err := svc.CreateDatasource(ctx, &CreateDatasourceRequest{
		Name:    "test-ds",
		Type:    "OSS",
		OwnerID: "alice",
		NodeIDs: []string{"node1", "node2"},
	})
	require.NoError(t, err)
	assert.NotEmpty(t, vo.DatasourceID)

	// Verify datasource exists
	var ds model.DatasourceDO
	err = db.Where("datasource_id = ?", vo.DatasourceID).First(&ds).Error
	assert.NoError(t, err)
	assert.Equal(t, "test-ds", ds.Name)
	assert.Equal(t, "OSS", ds.Type)

	// Verify node associations exist
	var nodes []model.DatasourceNodeDO
	db.Where("datasource_id = ?", vo.DatasourceID).Find(&nodes)
	assert.Len(t, nodes, 2)
}

// TestDatasourceService_DeleteAtomic verifies that datasource deletion removes
// both the datasource and its node associations atomically (Bug 28).
func TestDatasourceService_DeleteAtomic(t *testing.T) {
	db := setupDatasourceTestDB(t)
	svc := NewDatasourceService(db, nil)
	ctx := context.Background()

	// Create datasource with nodes
	vo, err := svc.CreateDatasource(ctx, &CreateDatasourceRequest{
		Name:    "delete-me",
		OwnerID: "bob",
		NodeIDs: []string{"n1", "n2", "n3"},
	})
	require.NoError(t, err)

	// Delete
	err = svc.DeleteDatasource(ctx, &DeleteDatasourceRequest{DatasourceID: vo.DatasourceID})
	require.NoError(t, err)

	// Verify datasource is gone
	var count int64
	db.Model(&model.DatasourceDO{}).Where("datasource_id = ?", vo.DatasourceID).Count(&count)
	assert.Equal(t, int64(0), count)

	// Verify node associations are gone
	db.Model(&model.DatasourceNodeDO{}).Where("datasource_id = ?", vo.DatasourceID).Count(&count)
	assert.Equal(t, int64(0), count)
}

// TestDatasourceService_DeleteNotFound verifies that deleting a non-existent
// datasource returns ErrDatasourceNotFound.
func TestDatasourceService_DeleteNotFound(t *testing.T) {
	db := setupDatasourceTestDB(t)
	svc := NewDatasourceService(db, nil)

	err := svc.DeleteDatasource(context.Background(), &DeleteDatasourceRequest{
		DatasourceID: "nonexistent",
	})
	assert.ErrorIs(t, err, ErrDatasourceNotFound)
}

// TestDatasourceService_ListWithFilter verifies datasource listing with
// owner and name filters.
func TestDatasourceService_ListWithFilter(t *testing.T) {
	db := setupDatasourceTestDB(t)
	svc := NewDatasourceService(db, nil)
	ctx := context.Background()

	// Create multiple datasources
	_, _ = svc.CreateDatasource(ctx, &CreateDatasourceRequest{Name: "alpha-ds", OwnerID: "alice"})
	_, _ = svc.CreateDatasource(ctx, &CreateDatasourceRequest{Name: "beta-ds", OwnerID: "bob"})
	_, _ = svc.CreateDatasource(ctx, &CreateDatasourceRequest{Name: "alpha-2", OwnerID: "alice"})

	// Filter by owner
	result, err := svc.ListDatasources(ctx, &DatasourceListRequest{OwnerID: "alice"})
	require.NoError(t, err)
	assert.Equal(t, int64(2), result.Total)

	// Filter by name (LIKE)
	result, err = svc.ListDatasources(ctx, &DatasourceListRequest{Name: "alpha"})
	require.NoError(t, err)
	assert.Equal(t, int64(2), result.Total)

	// No filter
	result, err = svc.ListDatasources(ctx, &DatasourceListRequest{})
	require.NoError(t, err)
	assert.Equal(t, int64(3), result.Total)
}

// TestDatasourceService_Detail verifies datasource detail retrieval with
// associated node IDs.
func TestDatasourceService_Detail(t *testing.T) {
	db := setupDatasourceTestDB(t)
	svc := NewDatasourceService(db, nil)
	ctx := context.Background()

	vo, err := svc.CreateDatasource(ctx, &CreateDatasourceRequest{
		Name:    "detail-ds",
		OwnerID: "alice",
		NodeIDs: []string{"nodeA", "nodeB"},
	})
	require.NoError(t, err)

	detail, err := svc.GetDatasourceDetail(ctx, &DatasourceDetailRequest{DatasourceID: vo.DatasourceID})
	require.NoError(t, err)
	assert.Equal(t, "detail-ds", detail.Name)
	assert.ElementsMatch(t, []string{"nodeA", "nodeB"}, detail.NodeIDs)
}

// TestDatasourceService_GetNodes verifies the datasource-nodes query.
func TestDatasourceService_GetNodes(t *testing.T) {
	db := setupDatasourceTestDB(t)
	svc := NewDatasourceService(db, nil)
	ctx := context.Background()

	// Seed a node record for name resolution
	db.Create(&model.NodeDO{NodeID: "n1", Name: "Node One"})

	vo, err := svc.CreateDatasource(ctx, &CreateDatasourceRequest{
		Name:    "nodes-ds",
		OwnerID: "alice",
		NodeIDs: []string{"n1", "n2"},
	})
	require.NoError(t, err)

	nodesVO, err := svc.GetDatasourceNodes(ctx, &DatasourceNodesRequest{DatasourceID: vo.DatasourceID})
	require.NoError(t, err)
	assert.Len(t, nodesVO.Nodes, 2)

	// n1 should have its name resolved
	for _, n := range nodesVO.Nodes {
		if n.NodeID == "n1" {
			assert.Equal(t, "Node One", n.NodeName)
		}
	}
}
