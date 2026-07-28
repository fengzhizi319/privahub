package service

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// DataFileInfo represents metadata about a data file in the local data directory.
type DataFileInfo struct {
	FileName  string    `json:"file_name"`
	FilePath  string    `json:"file_path"`
	Size      int64     `json:"size"`
	ModTime   time.Time `json:"mod_time"`
	Extension string    `json:"extension"`
	NodeID    string    `json:"node_id"`
}

// DataDirectoryService scans and manages local data files (CSV, etc.) per node.
// It mirrors the Java DataService's file scanning functionality.
type DataDirectoryService struct {
	baseDir string
}

// NewDataDirectoryService creates a new DataDirectoryService.
// baseDir is the root data directory (e.g., /app/data/ or ./data/).
func NewDataDirectoryService(baseDir string) *DataDirectoryService {
	if baseDir == "" {
		baseDir = "./data"
	}
	return &DataDirectoryService{baseDir: baseDir}
}

// ListFiles lists all data files for a given node.
// Files are stored under baseDir/{nodeID}/ directory.
func (s *DataDirectoryService) ListFiles(ctx context.Context, nodeID string) ([]DataFileInfo, error) {
	nodeDir := filepath.Join(s.baseDir, nodeID)

	entries, err := os.ReadDir(nodeDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []DataFileInfo{}, nil
		}
		return nil, err
	}

	var files []DataFileInfo
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if !isDataFile(ext) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		files = append(files, DataFileInfo{
			FileName:  entry.Name(),
			FilePath:  filepath.Join(nodeDir, entry.Name()),
			Size:      info.Size(),
			ModTime:   info.ModTime(),
			Extension: ext,
			NodeID:    nodeID,
		})
	}

	// Sort by modification time descending (newest first)
	sort.Slice(files, func(i, j int) bool {
		return files[i].ModTime.After(files[j].ModTime)
	})

	return files, nil
}

// ListNodes lists all node directories that have data files.
func (s *DataDirectoryService) ListNodes(ctx context.Context) ([]string, error) {
	entries, err := os.ReadDir(s.baseDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}

	var nodes []string
	for _, entry := range entries {
		if entry.IsDir() {
			nodes = append(nodes, entry.Name())
		}
	}
	sort.Strings(nodes)
	return nodes, nil
}

// FileExists checks if a specific file exists in a node's data directory.
func (s *DataDirectoryService) FileExists(nodeID, fileName string) bool {
	filePath := s.safePath(nodeID, fileName)
	if filePath == "" {
		return false
	}
	info, err := os.Stat(filePath)
	return err == nil && !info.IsDir()
}

// GetFilePath returns the full path for a node's data file.
// Returns empty string if the file doesn't exist or path is invalid.
func (s *DataDirectoryService) GetFilePath(nodeID, fileName string) string {
	filePath := s.safePath(nodeID, fileName)
	if filePath == "" {
		return ""
	}
	if info, err := os.Stat(filePath); err == nil && !info.IsDir() {
		return filePath
	}
	return ""
}

// safePath validates and resolves a path within the base directory.
// Returns empty string if the path is invalid or escapes the base directory.
func (s *DataDirectoryService) safePath(nodeID, fileName string) string {
	// Reject path traversal in both nodeID and fileName
	if strings.Contains(nodeID, "..") || strings.Contains(nodeID, "/") || strings.Contains(nodeID, "\\") {
		return ""
	}
	if strings.Contains(fileName, "..") || strings.Contains(fileName, "/") || strings.Contains(fileName, "\\") {
		return ""
	}
	// Resolve and verify the final path stays within baseDir
	resolved := filepath.Clean(filepath.Join(s.baseDir, nodeID, fileName))
	absBase, err := filepath.Abs(s.baseDir)
	if err != nil {
		return ""
	}
	absResolved, err := filepath.Abs(resolved)
	if err != nil {
		return ""
	}
	if !strings.HasPrefix(absResolved, absBase+string(os.PathSeparator)) {
		return ""
	}
	return resolved
}

// EnsureNodeDir creates the node data directory if it doesn't exist.
func (s *DataDirectoryService) EnsureNodeDir(nodeID string) (string, error) {
	nodeDir := filepath.Join(s.baseDir, nodeID)
	if err := os.MkdirAll(nodeDir, 0750); err != nil {
		return "", err
	}
	return nodeDir, nil
}

// isDataFile returns true if the extension is a supported data file type.
func isDataFile(ext string) bool {
	switch ext {
	case ".csv", ".tsv", ".json", ".parquet", ".txt":
		return true
	}
	return false
}
