package service

import (
	"strings"
	"sync"

	"go.uber.org/zap"
)

// PlatformType represents the deployment platform type.
type PlatformType string

const (
	PlatformCenter   PlatformType = "center"
	PlatformAutonomy PlatformType = "autonomy"
	PlatformLite     PlatformType = "lite"
	PlatformP2PEdge  PlatformType = "p2p_edge"
)

// EnvService provides platform topology detection and environment awareness.
// It is used by other services to make decisions based on the deployment mode
// (center, autonomy, lite, p2p_edge).
type EnvService struct {
	mu           sync.RWMutex
	platformType PlatformType
	nodeID       string
	instID       string
	embeddedNodes map[string]bool // nodes embedded in this instance
	log          *zap.Logger
}

// NewEnvService creates a new EnvService with the given platform configuration.
func NewEnvService(platformType, nodeID, instID string, log *zap.Logger) *EnvService {
	if log == nil {
		log = zap.NewNop()
	}
	return &EnvService{
		platformType:  PlatformType(platformType),
		nodeID:        nodeID,
		instID:        instID,
		embeddedNodes: make(map[string]bool),
		log:           log,
	}
}

// GetPlatformType returns the current platform type.
func (s *EnvService) GetPlatformType() PlatformType {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.platformType
}

// GetPlatformNodeId returns the current node ID.
func (s *EnvService) GetPlatformNodeId() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.nodeID
}

// GetInstID returns the current institution ID.
func (s *EnvService) GetInstID() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.instID
}

// IsCenter returns true if this instance is the center platform.
func (s *EnvService) IsCenter() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.platformType == PlatformCenter
}

// IsAutonomy returns true if this instance is an autonomy node.
func (s *EnvService) IsAutonomy() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.platformType == PlatformAutonomy
}

// IsLite returns true if this instance is a lite node.
func (s *EnvService) IsLite() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.platformType == PlatformLite
}

// IsP2pEdge returns true if this instance is a P2P edge node.
func (s *EnvService) IsP2pEdge() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.platformType == PlatformP2PEdge
}

// IsEmbeddedNode returns true if the given node is embedded in this instance.
func (s *EnvService) IsEmbeddedNode(nodeID string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.embeddedNodes[nodeID]
}

// IsCurrentNodeEnvironment returns true if the given nodeID matches this instance's node.
func (s *EnvService) IsCurrentNodeEnvironment(nodeID string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.nodeID == nodeID
}

// IsCurrentInstEnvironment returns true if the given instID matches this instance.
func (s *EnvService) IsCurrentInstEnvironment(instID string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.instID == instID
}

// IsNodeInCurrentInst returns true if the given node belongs to the current institution.
// In center mode, all nodes are considered in the current inst.
func (s *EnvService) IsNodeInCurrentInst(nodeID string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.platformType == PlatformCenter {
		return true
	}
	return s.nodeID == nodeID || s.embeddedNodes[nodeID]
}

// SetEmbeddedNodes updates the set of embedded nodes.
func (s *EnvService) SetEmbeddedNodes(nodeIDs []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.embeddedNodes = make(map[string]bool, len(nodeIDs))
	for _, id := range nodeIDs {
		s.embeddedNodes[id] = true
	}
}

// FindLocalNodeId determines which node in a task is local to this instance.
// Given a list of node IDs participating in a task, returns the one that
// belongs to this instance (either the platform node or an embedded node).
func (s *EnvService) FindLocalNodeId(nodeIDs []string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, id := range nodeIDs {
		if id == s.nodeID || s.embeddedNodes[id] {
			return id
		}
	}
	// Fallback: if center mode, return first node
	if s.platformType == PlatformCenter && len(nodeIDs) > 0 {
		return nodeIDs[0]
	}
	return ""
}

// EnvDTO represents the environment information returned by the /env API.
type EnvDTO struct {
	PlatformType string `json:"platform_type"`
	NodeID       string `json:"node_id"`
	InstID       string `json:"inst_id"`
	DeployMode   string `json:"deploy_mode"`
	IsCenter     bool   `json:"is_center"`
	IsAutonomy   bool   `json:"is_autonomy"`
	IsP2pEdge    bool   `json:"is_p2p_edge"`
}

// GetEnv returns the full environment DTO.
func (s *EnvService) GetEnv() *EnvDTO {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return &EnvDTO{
		PlatformType: string(s.platformType),
		NodeID:       s.nodeID,
		InstID:       s.instID,
		DeployMode:   string(s.platformType),
		IsCenter:     s.platformType == PlatformCenter,
		IsAutonomy:   s.platformType == PlatformAutonomy,
		IsP2pEdge:    s.platformType == PlatformP2PEdge,
	}
}

// NormalizePlatformType normalizes various mode strings to a PlatformType.
func NormalizePlatformType(mode string) PlatformType {
	switch strings.ToLower(mode) {
	case "center", "master":
		return PlatformCenter
	case "autonomy":
		return PlatformAutonomy
	case "lite":
		return PlatformLite
	case "p2p_edge", "p2pedge", "edge":
		return PlatformP2PEdge
	default:
		return PlatformCenter
	}
}
