package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/internal/dao/repository"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

func TestRawJSONString(t *testing.T) {
	tests := []struct {
		name     string
		input    json.RawMessage
		expected string
	}{
		{"empty", nil, ""},
		{"empty bytes", json.RawMessage{}, ""},
		{"json string", json.RawMessage(`"hello"`), "hello"},
		{"json object", json.RawMessage(`{"key":"value"}`), `{"key":"value"}`},
		{"json array", json.RawMessage(`["a","b"]`), `["a","b"]`},
		{"json with spaces", json.RawMessage(`{ "key" : "value" }`), `{"key":"value"}`},
		{"number", json.RawMessage(`123`), "123"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := rawJSONString(tt.input)
			if result != tt.expected {
				t.Errorf("rawJSONString(%s) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestRawJSONMessage(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"empty", "", "null"},
		{"invalid json", "not json", "null"},
		{"valid object", `{"key":"value"}`, `{"key":"value"}`},
		{"valid array", `["a","b"]`, `["a","b"]`},
		{"valid string", `"hello"`, `"hello"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := rawJSONMessage(tt.input)
			if string(result) != tt.expected {
				t.Errorf("rawJSONMessage(%q) = %s, want %s", tt.input, result, tt.expected)
			}
		})
	}
}

func TestUpstreamNodeID(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"with output suffix", "node1-output-0", "node1"},
		{"with multiple suffixes", "node1-output-0-output-1", "node1-output-0"},
		{"no suffix", "node1", "node1"},
		{"empty", "", ""},
		{"only suffix", "-output-0", "-output-0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := upstreamNodeID(tt.input)
			if result != tt.expected {
				t.Errorf("upstreamNodeID(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestNodeStatusString(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"succeeded", "SUCCEEDED", "SUCCEED"},
		{"running", "RUNNING", "RUNNING"},
		{"pending", "PENDING", "PENDING"},
		{"failed", "FAILED", "FAILED"},
		{"empty", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := nodeStatusString(tt.input)
			if result != tt.expected {
				t.Errorf("nodeStatusString(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestSplitOutputs(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{"empty", "", nil},
		{"invalid json", "not json", nil},
		{"valid array", `["a","b","c"]`, []string{"a", "b", "c"}},
		{"single element", `["single"]`, []string{"single"}},
		{"empty array", `[]`, []string{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := splitOutputs(tt.input)
			if tt.expected == nil {
				if result != nil {
					t.Errorf("splitOutputs(%q) = %v, want nil", tt.input, result)
				}
				return
			}
			if len(result) != len(tt.expected) {
				t.Errorf("splitOutputs(%q) len = %d, want %d", tt.input, len(result), len(tt.expected))
				return
			}
			for i := range result {
				if result[i] != tt.expected[i] {
					t.Errorf("splitOutputs(%q)[%d] = %q, want %q", tt.input, i, result[i], tt.expected[i])
				}
			}
		})
	}
}

func TestContainsStr(t *testing.T) {
	tests := []struct {
		name     string
		slice    []string
		val      string
		expected bool
	}{
		{"found", []string{"a", "b", "c"}, "b", true},
		{"not found", []string{"a", "b", "c"}, "d", false},
		{"empty slice", []string{}, "a", false},
		{"nil slice", nil, "a", false},
		{"empty value", []string{"a", "", "c"}, "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := containsStr(tt.slice, tt.val)
			if result != tt.expected {
				t.Errorf("containsStr(%v, %q) = %v, want %v", tt.slice, tt.val, result, tt.expected)
			}
		})
	}
}

func TestDatatableIDFromNodeDef(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"empty", "", ""},
		{"invalid json", "not json", ""},
		{"no datatable_selected", `{"domain":"read_data","name":"datatable","attrPaths":["other"],"attrs":[{"s":"val"}]}`, ""},
		{"with datatable_selected", `{"domain":"read_data","name":"datatable","attrPaths":["datatable_selected"],"attrs":[{"s":"table-123"}]}`, "table-123"},
		{"datatable_selected at index 1", `{"domain":"read_data","name":"datatable","attrPaths":["other","datatable_selected"],"attrs":[{"s":"x"},{"s":"table-456"}]}`, "table-456"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := datatableIDFromNodeDef(tt.input)
			if result != tt.expected {
				t.Errorf("datatableIDFromNodeDef(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

// --- Bug 50: GraphService.DeleteGraph atomicity ---

func setupGraphTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	if err := db.AutoMigrate(
		&model.ProjectGraphDO{},
		&model.ProjectGraphNodeDO{},
		&model.ProjectJobDO{},
		&model.ProjectJobTaskDO{},
		&model.ProjectJobTaskLogDO{},
	); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}
	return db
}

func newTestGraphService(db *gorm.DB) *GraphService {
	return NewGraphService(
		repository.NewGraphRepo(db),
		repository.NewGraphNodeRepo(db),
		repository.NewJobRepo(db),
		repository.NewTaskRepo(db),
		repository.NewTaskLogRepo(db),
		nil, // no kuscia client in tests
		db,
	)
}

func TestGraphService_DeleteGraph_Atomic(t *testing.T) {
	db := setupGraphTestDB(t)
	svc := newTestGraphService(db)
	ctx := context.Background()

	// Seed a graph with nodes
	db.Create(&model.ProjectGraphDO{
		ProjectID: "p1",
		GraphID:   "g1",
		Name:      "Test Graph",
		Edges:     "[]",
	})
	db.Create(&model.ProjectGraphNodeDO{ProjectID: "p1", GraphID: "g1", GraphNodeID: "n1", CodeName: "test/node1"})
	db.Create(&model.ProjectGraphNodeDO{ProjectID: "p1", GraphID: "g1", GraphNodeID: "n2", CodeName: "test/node2"})

	// Delete the graph
	err := svc.DeleteGraph(ctx, &DeleteGraphRequest{ProjectID: "p1", GraphID: "g1"})
	if err != nil {
		t.Fatalf("DeleteGraph failed: %v", err)
	}

	// Verify graph is deleted
	var graphCount int64
	db.Model(&model.ProjectGraphDO{}).Where("project_id = ? AND graph_id = ?", "p1", "g1").Count(&graphCount)
	if graphCount != 0 {
		t.Errorf("expected 0 graphs, got %d", graphCount)
	}

	// Verify nodes are deleted atomically
	var nodeCount int64
	db.Model(&model.ProjectGraphNodeDO{}).Where("project_id = ? AND graph_id = ?", "p1", "g1").Count(&nodeCount)
	if nodeCount != 0 {
		t.Errorf("expected 0 graph nodes, got %d", nodeCount)
	}
}

func TestGraphService_DeleteGraph_NotFound(t *testing.T) {
	db := setupGraphTestDB(t)
	svc := newTestGraphService(db)
	ctx := context.Background()

	err := svc.DeleteGraph(ctx, &DeleteGraphRequest{ProjectID: "p1", GraphID: "nonexistent"})
	if err != ErrGraphNotFound {
		t.Errorf("expected ErrGraphNotFound, got %v", err)
	}
}
