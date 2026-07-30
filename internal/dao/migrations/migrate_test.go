package migration

import (
	"context"
	"testing"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupMigrateTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	if err := db.AutoMigrate(AllModels()...); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}
	return db
}

func TestAutoMigrate(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	log := zap.NewNop()
	if err := AutoMigrate(db, log); err != nil {
		t.Fatalf("AutoMigrate failed: %v", err)
	}
}

func TestSeedData(t *testing.T) {
	db := setupMigrateTestDB(t)
	log := zap.NewNop()
	ctx := context.Background()

	if err := SeedData(ctx, db, log); err != nil {
		t.Fatalf("SeedData failed: %v", err)
	}

	// Verify institutions were seeded
	var instCount int64
	db.Model(&model.InstDO{}).Count(&instCount)
	if instCount != 3 {
		t.Errorf("expected 3 institutions, got %d", instCount)
	}

	// Verify nodes were seeded
	var nodeCount int64
	db.Model(&model.NodeDO{}).Count(&nodeCount)
	if nodeCount != 2 {
		t.Errorf("expected 2 nodes, got %d", nodeCount)
	}

	// Verify routes were seeded
	var routeCount int64
	db.Model(&model.NodeRouteDO{}).Count(&routeCount)
	if routeCount != 2 {
		t.Errorf("expected 2 routes, got %d", routeCount)
	}

	// Verify admin user was seeded
	var userCount int64
	db.Model(&model.UserAccountsDO{}).Where("name = ?", "admin").Count(&userCount)
	if userCount != 1 {
		t.Errorf("expected 1 admin user, got %d", userCount)
	}

	// Verify RBAC was seeded
	var roleCount int64
	db.Model(&model.SysRoleDO{}).Where("role_code = ?", "ADMIN").Count(&roleCount)
	if roleCount != 1 {
		t.Errorf("expected 1 ADMIN role, got %d", roleCount)
	}

	// Verify datasources were seeded
	var dsCount int64
	db.Model(&model.DatasourceDO{}).Count(&dsCount)
	if dsCount != 2 {
		t.Errorf("expected 2 datasources, got %d", dsCount)
	}

	// Verify datasource-node associations
	var dsnCount int64
	db.Model(&model.DatasourceNodeDO{}).Count(&dsnCount)
	if dsnCount != 2 {
		t.Errorf("expected 2 datasource-node associations, got %d", dsnCount)
	}

	// Verify project was seeded
	var projCount int64
	db.Model(&model.ProjectDO{}).Where("project_id = ?", "p-default").Count(&projCount)
	if projCount != 1 {
		t.Errorf("expected 1 default project, got %d", projCount)
	}

	// Verify project-node associations
	var pnCount int64
	db.Model(&model.ProjectNodeDO{}).Where("project_id = ?", "p-default").Count(&pnCount)
	if pnCount != 2 {
		t.Errorf("expected 2 project-node associations, got %d", pnCount)
	}

	// Verify datatables were seeded
	var dtCount int64
	db.Model(&model.ProjectDatatableDO{}).Count(&dtCount)
	if dtCount != 2 {
		t.Errorf("expected 2 datatables, got %d", dtCount)
	}

	// Verify graph was seeded
	var graphCount int64
	db.Model(&model.ProjectGraphDO{}).Where("graph_id = ?", "g-demo").Count(&graphCount)
	if graphCount != 1 {
		t.Errorf("expected 1 demo graph, got %d", graphCount)
	}

	// Verify graph nodes were seeded
	var gnCount int64
	db.Model(&model.ProjectGraphNodeDO{}).Where("graph_id = ?", "g-demo").Count(&gnCount)
	if gnCount != 2 {
		t.Errorf("expected 2 graph nodes, got %d", gnCount)
	}
}

func TestSeedData_Idempotent(t *testing.T) {
	db := setupMigrateTestDB(t)
	log := zap.NewNop()
	ctx := context.Background()

	// Run seed twice - should not fail or duplicate
	if err := SeedData(ctx, db, log); err != nil {
		t.Fatalf("first SeedData failed: %v", err)
	}
	if err := SeedData(ctx, db, log); err != nil {
		t.Fatalf("second SeedData failed: %v", err)
	}

	// Verify no duplicates
	var instCount int64
	db.Model(&model.InstDO{}).Count(&instCount)
	if instCount != 3 {
		t.Errorf("expected 3 institutions after double seed, got %d", instCount)
	}

	var nodeCount int64
	db.Model(&model.NodeDO{}).Count(&nodeCount)
	if nodeCount != 2 {
		t.Errorf("expected 2 nodes after double seed, got %d", nodeCount)
	}

	var userCount int64
	db.Model(&model.UserAccountsDO{}).Count(&userCount)
	if userCount != 1 {
		t.Errorf("expected 1 user after double seed, got %d", userCount)
	}
}

func TestHashPassword(t *testing.T) {
	hash := hashPassword("12345678")
	// SHA-256 of "12345678" is a known value
	expected := "ef797c8118f02dfb649607dd5d3f8c7623048c9c063d532cc95c5ed7a898a64f"
	if hash != expected {
		t.Errorf("expected hash %q, got %q", expected, hash)
	}
}

func TestAllModels_NotEmpty(t *testing.T) {
	models := AllModels()
	if len(models) == 0 {
		t.Error("AllModels returned empty slice")
	}
	// We expect at least 30 models based on the current schema
	if len(models) < 30 {
		t.Errorf("expected at least 30 models, got %d", len(models))
	}
}
