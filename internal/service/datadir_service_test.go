package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func setupDataDirTest(t *testing.T) (string, *DataDirectoryService) {
	t.Helper()
	tmpDir := t.TempDir()
	return tmpDir, NewDataDirectoryService(tmpDir)
}

func TestDataDirectoryService_ListFiles(t *testing.T) {
	tmpDir, svc := setupDataDirTest(t)

	// Create node directory with files
	nodeDir := filepath.Join(tmpDir, "alice")
	os.MkdirAll(nodeDir, 0755)
	os.WriteFile(filepath.Join(nodeDir, "data.csv"), []byte("a,b,c"), 0644)
	os.WriteFile(filepath.Join(nodeDir, "data.json"), []byte(`{"x":1}`), 0644)
	os.WriteFile(filepath.Join(nodeDir, "readme.md"), []byte("# readme"), 0644) // not a data file
	os.MkdirAll(filepath.Join(nodeDir, "subdir"), 0755)                         // directory, should be skipped

	files, err := svc.ListFiles(context.Background(), "alice")
	if err != nil {
		t.Fatalf("ListFiles failed: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("expected 2 data files, got %d", len(files))
	}

	// Verify file info
	found := map[string]bool{}
	for _, f := range files {
		found[f.FileName] = true
		if f.NodeID != "alice" {
			t.Errorf("expected node_id 'alice', got %q", f.NodeID)
		}
	}
	if !found["data.csv"] || !found["data.json"] {
		t.Error("expected data.csv and data.json in results")
	}
}

func TestDataDirectoryService_ListFiles_NonExistentNode(t *testing.T) {
	_, svc := setupDataDirTest(t)

	files, err := svc.ListFiles(context.Background(), "nonexistent")
	if err != nil {
		t.Fatalf("ListFiles should not fail for non-existent dir: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("expected 0 files, got %d", len(files))
	}
}

func TestDataDirectoryService_ListNodes(t *testing.T) {
	tmpDir, svc := setupDataDirTest(t)

	os.MkdirAll(filepath.Join(tmpDir, "bob"), 0755)
	os.MkdirAll(filepath.Join(tmpDir, "alice"), 0755)
	os.WriteFile(filepath.Join(tmpDir, "not-a-dir.txt"), []byte("x"), 0644)

	nodes, err := svc.ListNodes(context.Background())
	if err != nil {
		t.Fatalf("ListNodes failed: %v", err)
	}
	if len(nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(nodes))
	}
	// Should be sorted
	if nodes[0] != "alice" || nodes[1] != "bob" {
		t.Errorf("expected [alice, bob], got %v", nodes)
	}
}

func TestDataDirectoryService_ListNodes_EmptyBase(t *testing.T) {
	_, svc := setupDataDirTest(t)

	nodes, err := svc.ListNodes(context.Background())
	if err != nil {
		t.Fatalf("ListNodes failed: %v", err)
	}
	if len(nodes) != 0 {
		t.Errorf("expected 0 nodes, got %d", len(nodes))
	}
}

func TestDataDirectoryService_ListNodes_NonExistentBase(t *testing.T) {
	svc := NewDataDirectoryService("/nonexistent/path/that/does/not/exist")

	nodes, err := svc.ListNodes(context.Background())
	if err != nil {
		t.Fatalf("ListNodes should not fail for non-existent base: %v", err)
	}
	if len(nodes) != 0 {
		t.Errorf("expected 0 nodes, got %d", len(nodes))
	}
}

func TestDataDirectoryService_FileExists(t *testing.T) {
	tmpDir, svc := setupDataDirTest(t)

	nodeDir := filepath.Join(tmpDir, "alice")
	os.MkdirAll(nodeDir, 0755)
	os.WriteFile(filepath.Join(nodeDir, "data.csv"), []byte("a,b"), 0644)

	if !svc.FileExists("alice", "data.csv") {
		t.Error("expected FileExists to return true")
	}
	if svc.FileExists("alice", "nonexistent.csv") {
		t.Error("expected FileExists to return false for missing file")
	}
	if svc.FileExists("bob", "data.csv") {
		t.Error("expected FileExists to return false for missing node")
	}
}

func TestDataDirectoryService_GetFilePath(t *testing.T) {
	tmpDir, svc := setupDataDirTest(t)

	nodeDir := filepath.Join(tmpDir, "alice")
	os.MkdirAll(nodeDir, 0755)
	os.WriteFile(filepath.Join(nodeDir, "data.csv"), []byte("a,b"), 0644)

	path := svc.GetFilePath("alice", "data.csv")
	if path == "" {
		t.Fatal("expected non-empty path")
	}
	expected := filepath.Join(tmpDir, "alice", "data.csv")
	if path != expected {
		t.Errorf("expected %q, got %q", expected, path)
	}

	// Non-existent file
	path = svc.GetFilePath("alice", "missing.csv")
	if path != "" {
		t.Errorf("expected empty path for missing file, got %q", path)
	}
}

func TestDataDirectoryService_SafePath_TraversalBlocked(t *testing.T) {
	_, svc := setupDataDirTest(t)

	// Path traversal in fileName
	path := svc.safePath("alice", "../../../etc/passwd")
	if path != "" {
		t.Error("expected empty path for traversal in fileName")
	}

	// Path traversal in nodeID
	path = svc.safePath("../etc", "passwd")
	if path != "" {
		t.Error("expected empty path for traversal in nodeID")
	}

	// Slash in fileName
	path = svc.safePath("alice", "sub/file.csv")
	if path != "" {
		t.Error("expected empty path for slash in fileName")
	}

	// Backslash in nodeID
	path = svc.safePath("al\\ice", "file.csv")
	if path != "" {
		t.Error("expected empty path for backslash in nodeID")
	}
}

func TestDataDirectoryService_SafePath_ValidPath(t *testing.T) {
	tmpDir, svc := setupDataDirTest(t)

	path := svc.safePath("alice", "data.csv")
	if path == "" {
		t.Fatal("expected non-empty path for valid input")
	}
	expected := filepath.Join(tmpDir, "alice", "data.csv")
	if path != expected {
		t.Errorf("expected %q, got %q", expected, path)
	}
}

func TestDataDirectoryService_EnsureNodeDir_Idempotent(t *testing.T) {
	tmpDir, svc := setupDataDirTest(t)

	dir, err := svc.EnsureNodeDir("new-node")
	if err != nil {
		t.Fatalf("EnsureNodeDir failed: %v", err)
	}
	expected := filepath.Join(tmpDir, "new-node")
	if dir != expected {
		t.Errorf("expected %q, got %q", expected, dir)
	}

	// Verify directory exists
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("directory not created: %v", err)
	}
	if !info.IsDir() {
		t.Error("expected a directory")
	}

	// Calling again should not fail
	_, err = svc.EnsureNodeDir("new-node")
	if err != nil {
		t.Fatalf("EnsureNodeDir idempotent call failed: %v", err)
	}
}

func TestIsDataFile(t *testing.T) {
	tests := []struct {
		ext      string
		expected bool
	}{
		{".csv", true},
		{".tsv", true},
		{".json", true},
		{".parquet", true},
		{".txt", true},
		{".md", false},
		{".exe", false},
		{".png", false},
		{"", false},
		{".CSV", false}, // case-sensitive; caller lowercases
	}

	for _, tt := range tests {
		got := isDataFile(tt.ext)
		if got != tt.expected {
			t.Errorf("isDataFile(%q) = %v, want %v", tt.ext, got, tt.expected)
		}
	}
}

func TestNewDataDirectoryService_DefaultBaseDir(t *testing.T) {
	svc := NewDataDirectoryService("")
	if svc.baseDir != "./data" {
		t.Errorf("expected default baseDir './data', got %q", svc.baseDir)
	}
}
