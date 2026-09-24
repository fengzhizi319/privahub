package service

import (
	"context"
	"testing"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/internal/dao/repository"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupExtendedTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	err = db.AutoMigrate(
		&model.UserAccountsDO{},
		&model.UserTokensDO{},
		&model.ProjectDO{},
		&model.ProjectInstDO{},
		&model.ProjectNodeDO{},
		&model.ProjectGraphDO{},
		&model.ProjectGraphNodeDO{},
		&model.ProjectJobDO{},
		&model.ProjectJobTaskDO{},
		&model.ProjectJobTaskLogDO{},
		&model.NodeDO{},
		&model.NodeRouteDO{},
		&model.InstDO{},
		&model.ProjectModelServingDO{},
		&model.ProjectModelPackDO{},
		&model.ProjectDatatableDO{},
		&model.DatasourceDO{},
		&model.DatasourceNodeDO{},
	)
	if err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}
	return db
}

// --- ProjectService Tests ---

func TestFirstNonEmpty(t *testing.T) {
	if got := firstNonEmpty("", "", "x"); got != "x" {
		t.Errorf("firstNonEmpty(\"\",\"\",\"x\") = %q, want \"x\"", got)
	}
	if got := firstNonEmpty("a", "b"); got != "a" {
		t.Errorf("firstNonEmpty(\"a\",\"b\") = %q, want \"a\"", got)
	}
	if got := firstNonEmpty(); got != "" {
		t.Errorf("firstNonEmpty() = %q, want \"\"", got)
	}
}

func TestNodeService_ListTeeNodes(t *testing.T) {
	db := setupExtendedTestDB(t)
	svc := NewNodeService(repository.NewNodeRepo(db), repository.NewNodeRouteRepo(db), nil, db)

	// mode 0 = mpc-only (excluded), mode 1 = tee-only, mode 2 = mpc&tee (both included)
	for _, n := range []model.NodeDO{
		{NodeID: "mpc-node", Name: "mpc", ControlNodeID: "mpc-node", Mode: 0},
		{NodeID: "tee-node", Name: "tee", ControlNodeID: "tee-node", Mode: 1},
		{NodeID: "hybrid-node", Name: "hybrid", ControlNodeID: "hybrid-node", Mode: 2},
	} {
		n := n
		if err := db.Create(&n).Error; err != nil {
			t.Fatalf("seed node %s failed: %v", n.NodeID, err)
		}
	}

	teeNodes, err := svc.ListTeeNodes(context.Background())
	if err != nil {
		t.Fatalf("ListTeeNodes failed: %v", err)
	}
	if len(teeNodes) != 2 {
		t.Fatalf("expected 2 tee-capable nodes, got %d", len(teeNodes))
	}
	ids := map[string]bool{}
	for _, n := range teeNodes {
		ids[n.NodeID] = true
	}
	if !ids["tee-node"] || !ids["hybrid-node"] {
		t.Errorf("expected tee-node and hybrid-node, got %v", ids)
	}
	if ids["mpc-node"] {
		t.Errorf("mpc-only node must not be tee-capable")
	}
}

// --- GraphService Tests ---

// --- ModelService Tests ---

// --- mapServingState Tests ---

func TestMapServingState(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Pending", "pending"},
		{"Progressing", "progressing"},
		{"PartialAvailable", "partial_available"},
		{"Available", "available"},
		{"Failed", "failed"},
		{"Unknown", "Unknown"},
	}
	for _, tt := range tests {
		got := mapServingState(tt.input)
		if got != tt.expected {
			t.Errorf("mapServingState(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}
