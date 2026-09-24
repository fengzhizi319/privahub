package service

import (
	"testing"
)

func TestEnvService_GetPlatformType(t *testing.T) {
	svc := NewEnvService("center", "node-1", "inst-1", nil)
	if svc.GetPlatformType() != PlatformCenter {
		t.Errorf("expected center, got %s", svc.GetPlatformType())
	}
}

func TestEnvService_GetPlatformNodeId(t *testing.T) {
	svc := NewEnvService("autonomy", "alice", "inst-1", nil)
	if svc.GetPlatformNodeId() != "alice" {
		t.Errorf("expected alice, got %s", svc.GetPlatformNodeId())
	}
}

func TestEnvService_GetInstID(t *testing.T) {
	svc := NewEnvService("center", "node-1", "my-inst", nil)
	if svc.GetInstID() != "my-inst" {
		t.Errorf("expected my-inst, got %s", svc.GetInstID())
	}
}

func TestEnvService_PlatformChecks(t *testing.T) {
	tests := []struct {
		platform   string
		isCenter   bool
		isAutonomy bool
		isLite     bool
		isP2pEdge  bool
	}{
		{"center", true, false, false, false},
		{"master", true, false, false, false},
		{"autonomy", false, true, false, true},
		{"P2P", false, true, false, true},
		{"lite", false, false, true, false},
		{"EDGE", false, false, true, false},
		{"p2p_edge", false, true, false, true},
	}

	for _, tt := range tests {
		svc := NewEnvService(tt.platform, "n", "i", nil)
		if svc.IsCenter() != tt.isCenter {
			t.Errorf("platform %s: IsCenter() = %v, want %v", tt.platform, svc.IsCenter(), tt.isCenter)
		}
		if svc.IsAutonomy() != tt.isAutonomy {
			t.Errorf("platform %s: IsAutonomy() = %v, want %v", tt.platform, svc.IsAutonomy(), tt.isAutonomy)
		}
		if svc.IsLite() != tt.isLite {
			t.Errorf("platform %s: IsLite() = %v, want %v", tt.platform, svc.IsLite(), tt.isLite)
		}
		if svc.IsP2pEdge() != tt.isP2pEdge {
			t.Errorf("platform %s: IsP2pEdge() = %v, want %v", tt.platform, svc.IsP2pEdge(), tt.isP2pEdge)
		}
	}
}

func TestEnvService_IsCurrentNodeEnvironment(t *testing.T) {
	svc := NewEnvService("center", "alice", "inst-1", nil)

	if !svc.IsCurrentNodeEnvironment("alice") {
		t.Error("expected true for current node")
	}
	if svc.IsCurrentNodeEnvironment("bob") {
		t.Error("expected false for different node")
	}
}

func TestEnvService_IsCurrentInstEnvironment(t *testing.T) {
	svc := NewEnvService("center", "alice", "inst-1", nil)

	if !svc.IsCurrentInstEnvironment("inst-1") {
		t.Error("expected true for current inst")
	}
	if svc.IsCurrentInstEnvironment("inst-2") {
		t.Error("expected false for different inst")
	}
}

func TestEnvService_IsNodeInCurrentInst_Center(t *testing.T) {
	svc := NewEnvService("center", "alice", "inst-1", nil)

	// In center mode, all nodes are in current inst
	if !svc.IsNodeInCurrentInst("any-node") {
		t.Error("center mode should return true for any node")
	}
}

func TestEnvService_IsNodeInCurrentInst_Autonomy(t *testing.T) {
	svc := NewEnvService("autonomy", "alice", "inst-1", nil)
	svc.SetEmbeddedNodes([]string{"embedded-1", "embedded-2"})

	if !svc.IsNodeInCurrentInst("alice") {
		t.Error("expected true for platform node")
	}
	if !svc.IsNodeInCurrentInst("embedded-1") {
		t.Error("expected true for embedded node")
	}
	if svc.IsNodeInCurrentInst("remote-node") {
		t.Error("expected false for remote node")
	}
}

func TestEnvService_SetEmbeddedNodes(t *testing.T) {
	// Java: embedded nodes only exist on the CENTER platform.
	svc := NewEnvService("center", "alice", "inst-1", nil)

	svc.SetEmbeddedNodes([]string{"n1", "n2"})
	if !svc.IsEmbeddedNode("n1") {
		t.Error("expected n1 to be embedded")
	}
	if !svc.IsEmbeddedNode("n2") {
		t.Error("expected n2 to be embedded")
	}

	// Replace embedded nodes
	svc.SetEmbeddedNodes([]string{"n3"})
	if svc.IsEmbeddedNode("n1") {
		t.Error("expected n1 to no longer be embedded after reset")
	}
	if !svc.IsEmbeddedNode("n3") {
		t.Error("expected n3 to be embedded")
	}
}

func TestEnvService_FindLocalNodeId_Comprehensive(t *testing.T) {
	svc := NewEnvService("autonomy", "alice", "inst-1", nil)
	svc.SetEmbeddedNodes([]string{"embedded-1"})

	// Platform node in list
	local := svc.FindLocalNodeId([]string{"bob", "alice", "carol"})
	if local != "alice" {
		t.Errorf("expected alice, got %q", local)
	}

	// Embedded node in list
	local = svc.FindLocalNodeId([]string{"bob", "embedded-1", "carol"})
	if local != "embedded-1" {
		t.Errorf("expected embedded-1, got %q", local)
	}

	// No local node
	local = svc.FindLocalNodeId([]string{"bob", "carol"})
	if local != "" {
		t.Errorf("expected empty string, got %q", local)
	}
}

func TestEnvService_FindLocalNodeId_CenterFallback(t *testing.T) {
	svc := NewEnvService("center", "master", "inst-1", nil)

	// Center mode falls back to first node
	local := svc.FindLocalNodeId([]string{"alice", "bob"})
	if local != "alice" {
		t.Errorf("expected alice (first node fallback), got %q", local)
	}

	// Empty list
	local = svc.FindLocalNodeId([]string{})
	if local != "" {
		t.Errorf("expected empty string for empty list, got %q", local)
	}
}

func TestEnvService_GetEnv(t *testing.T) {
	svc := NewEnvService("autonomy", "alice", "inst-1", nil)
	env := svc.GetEnv()

	if env.PlatformType != "AUTONOMY" {
		t.Errorf("expected platformType 'AUTONOMY', got %q", env.PlatformType)
	}
	if env.PlatformNodeID != "alice" {
		t.Errorf("expected platformNodeId 'alice', got %q", env.PlatformNodeID)
	}
	if env.NodeID != "alice" {
		t.Errorf("expected node_id 'alice', got %q", env.NodeID)
	}
	if env.InstID != "inst-1" {
		t.Errorf("expected inst_id 'inst-1', got %q", env.InstID)
	}
	if env.IsCenter {
		t.Error("expected IsCenter false")
	}
	if !env.IsAutonomy {
		t.Error("expected IsAutonomy true")
	}
	if !env.IsP2pEdge {
		t.Error("expected IsP2pEdge true (alias of autonomy)")
	}
}

func TestEnvService_EmbeddedOnlyOnCenter(t *testing.T) {
	c := NewEnvService("master", "kuscia-system", "", nil)
	c.SetEmbeddedNodes(DefaultEmbeddedNodes)
	if !c.IsEmbeddedNode("tee") || !c.IsCurrentNodeEnvironment("alice") {
		t.Error("center should treat alice/bob/tee as embedded")
	}
	e := NewEnvService("edge", "alice", "", nil)
	e.SetEmbeddedNodes(DefaultEmbeddedNodes)
	if e.IsEmbeddedNode("bob") {
		t.Error("edge platform has no embedded nodes")
	}
}

func TestNormalizePlatformType_Comprehensive(t *testing.T) {
	tests := []struct {
		input    string
		expected PlatformType
	}{
		{"center", PlatformCenter},
		{"master", PlatformCenter},
		{"CENTER", PlatformCenter},
		{"autonomy", PlatformAutonomy},
		{"AUTONOMY", PlatformAutonomy},
		{"p2p", PlatformAutonomy},
		{"lite", PlatformEdge},
		{"EDGE", PlatformEdge},
		{"p2p_edge", PlatformAutonomy},
		{"p2pedge", PlatformAutonomy},
		{"edge", PlatformEdge},
		{"TEST", PlatformTest},
		{"unknown", PlatformCenter}, // default
		{"", PlatformCenter},        // default
	}

	for _, tt := range tests {
		got := NormalizePlatformType(tt.input)
		if got != tt.expected {
			t.Errorf("NormalizePlatformType(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}
