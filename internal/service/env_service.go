package service

import (
	"strings"
	"sync"

	"go.uber.org/zap"
)

// PlatformType represents the deployment platform type (Java PlatformTypeEnum).
type PlatformType string

const (
	// PlatformCenter is the centralized platform (Java CENTER, config "master"/"center").
	PlatformCenter PlatformType = "CENTER"
	// PlatformEdge is an edge platform attached to a center (Java EDGE, config "lite"/"edge").
	PlatformEdge PlatformType = "EDGE"
	// PlatformAutonomy is the P2P/autonomy platform (Java AUTONOMY, config "autonomy"/"p2p").
	PlatformAutonomy PlatformType = "AUTONOMY"
	// PlatformTest is the unit-test platform (Java TEST).
	PlatformTest PlatformType = "TEST"

	// PlatformLite is kept for backward compatibility; lite == EDGE.
	PlatformLite = PlatformEdge
	// PlatformP2PEdge is kept for backward compatibility; a P2P edge is an AUTONOMY platform.
	PlatformP2PEdge = PlatformAutonomy
)

// DefaultEmbeddedNodes are the nodes embedded in a center deployment (Java
// EnvServiceImpl.EMBEDDED_NODE).
var DefaultEmbeddedNodes = []string{"alice", "bob", "tee"}

// EnvService provides platform topology detection and environment awareness
// (Java EnvService). Services/handlers use it to branch on CENTER / EDGE /
// AUTONOMY behaviour.
type EnvService struct {
	mu            sync.RWMutex
	platformType  PlatformType
	nodeID        string
	instID        string
	deployMode    string
	embeddedNodes map[string]bool // nodes embedded in this instance
	log           *zap.Logger
}

// NewEnvService creates a new EnvService. platformType accepts any config
// spelling (master/center/lite/edge/autonomy/p2p/test, case-insensitive) and
// is normalized with NormalizePlatformType.
func NewEnvService(platformType, nodeID, instID string, log *zap.Logger) *EnvService {
	if log == nil {
		log = zap.NewNop()
	}
	return &EnvService{
		platformType:  NormalizePlatformType(platformType),
		nodeID:        nodeID,
		instID:        instID,
		deployMode:    "ALL-IN-ONE",
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

// PlatformTypeString returns the platform type as the Java enum name.
func (s *EnvService) PlatformTypeString() string { return string(s.GetPlatformType()) }

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

// SetInstID updates the current institution ID (e.g. after P2P inst init).
func (s *EnvService) SetInstID(instID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.instID = instID
}

// GetDeployMode returns the deploy mode (MPC | TEE | ALL-IN-ONE).
func (s *EnvService) GetDeployMode() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.deployMode
}

// SetDeployMode sets the deploy mode.
func (s *EnvService) SetDeployMode(mode string) {
	if mode == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deployMode = strings.ToUpper(mode)
}

// IsCenter returns true if this instance is the center platform.
func (s *EnvService) IsCenter() bool { return s.GetPlatformType() == PlatformCenter }

// IsEdge returns true if this instance is an edge platform.
func (s *EnvService) IsEdge() bool { return s.GetPlatformType() == PlatformEdge }

// IsAutonomy returns true if this instance is an autonomy (P2P) node.
func (s *EnvService) IsAutonomy() bool { return s.GetPlatformType() == PlatformAutonomy }

// IsTest returns true for the TEST platform.
func (s *EnvService) IsTest() bool { return s.GetPlatformType() == PlatformTest }

// IsLite is an alias of IsEdge (kept for backward compatibility).
func (s *EnvService) IsLite() bool { return s.IsEdge() }

// IsP2pEdge is an alias of IsAutonomy (kept for backward compatibility).
func (s *EnvService) IsP2pEdge() bool { return s.IsAutonomy() }

// IsEmbeddedNode returns true if the given node is embedded in this instance.
// As in Java, embedded nodes only exist on a CENTER platform.
func (s *EnvService) IsEmbeddedNode(nodeID string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.embeddedNodes[nodeID] && s.platformType == PlatformCenter
}

// IsCurrentNodeEnvironment returns true if nodeID is this platform's node or
// an embedded node (Java isCurrentNodeEnvironment).
func (s *EnvService) IsCurrentNodeEnvironment(nodeID string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.nodeID == nodeID || (s.embeddedNodes[nodeID] && s.platformType == PlatformCenter)
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

// SetEmbeddedNodes updates the set of embedded nodes (for AUTONOMY this is
// the set of nodes of the local institution).
func (s *EnvService) SetEmbeddedNodes(nodeIDs []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.embeddedNodes = make(map[string]bool, len(nodeIDs))
	for _, id := range nodeIDs {
		s.embeddedNodes[id] = true
	}
}

// EmbeddedNodes returns the configured embedded node IDs.
func (s *EnvService) EmbeddedNodes() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]string, 0, len(s.embeddedNodes))
	for id := range s.embeddedNodes {
		out = append(out, id)
	}
	return out
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
// platformType/platformNodeId follow the Java EnvDTO; the snake_case fields
// are kept for existing Go consumers.
type EnvDTO struct {
	PlatformType   string `json:"platformType"`
	PlatformNodeID string `json:"platformNodeId"`
	NodeID         string `json:"node_id"`
	InstID         string `json:"inst_id"`
	DeployMode     string `json:"deploy_mode"`
	IsCenter       bool   `json:"is_center"`
	IsAutonomy     bool   `json:"is_autonomy"`
	IsP2pEdge      bool   `json:"is_p2p_edge"`
}

// GetEnv returns the full environment DTO.
func (s *EnvService) GetEnv() *EnvDTO {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return &EnvDTO{
		PlatformType:   string(s.platformType),
		PlatformNodeID: s.nodeID,
		NodeID:         s.nodeID,
		InstID:         s.instID,
		DeployMode:     s.deployMode,
		IsCenter:       s.platformType == PlatformCenter,
		IsAutonomy:     s.platformType == PlatformAutonomy,
		IsP2pEdge:      s.platformType == PlatformAutonomy,
	}
}

// NormalizePlatformType normalizes various mode strings to a PlatformType.
// Unknown values default to CENTER.
func NormalizePlatformType(mode string) PlatformType {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "center", "master", "":
		return PlatformCenter
	case "edge", "lite":
		return PlatformEdge
	case "autonomy", "p2p", "p2p_edge", "p2pedge":
		return PlatformAutonomy
	case "test":
		return PlatformTest
	default:
		return PlatformCenter
	}
}

var (
	globalEnvMu sync.RWMutex
	globalEnv   *EnvService
)

// SetGlobalEnv installs the process-wide EnvService (done once in wire) so
// services constructed without an explicit EnvService can query the platform.
func SetGlobalEnv(env *EnvService) {
	globalEnvMu.Lock()
	globalEnv = env
	globalEnvMu.Unlock()
}

// GlobalEnv returns the process-wide EnvService; before wiring (e.g. in unit
// tests) it returns a CENTER environment with node "kuscia-system".
func GlobalEnv() *EnvService {
	globalEnvMu.RLock()
	env := globalEnv
	globalEnvMu.RUnlock()
	if env == nil {
		return NewEnvService("CENTER", "kuscia-system", "", nil)
	}
	return env
}
