package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestKusciaNodeEnvKey(t *testing.T) {
	cases := map[string]string{"kuscia-system": "KUSCIA_SYSTEM", "alice": "ALICE", "org.b-1": "ORG_B_1"}
	for in, want := range cases {
		if got := KusciaNodeEnvKey(in); got != want {
			t.Errorf("KusciaNodeEnvKey(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestApplyNodeEnvOverrides(t *testing.T) {
	kc := KusciaConfig{Nodes: []KusciaNodeConfig{
		{DomainID: "kuscia-system", Host: "127.0.0.1", Port: 18083},
		{DomainID: "alice", Host: "127.0.0.1", Port: 18083, Protocol: "notls"},
	}}
	err := kc.applyNodeEnvOverrides([]string{
		"PRIVAHUB_KUSCIA_NODES_KUSCIA_SYSTEM_PORT=28083",
		"PRIVAHUB_KUSCIA_NODES_KUSCIA_SYSTEM_HTTP_PORT=28082",
		"PRIVAHUB_KUSCIA_NODES_ALICE_HOST=10.0.0.2",
		"PRIVAHUB_KUSCIA_NODES_ALICE_PROTOCOL=tls",
		"PRIVAHUB_KUSCIA_NODES_BOB_PORT=1",  // unknown domain: ignored
		"PRIVAHUB_KUSCIA_NODES_ALICE_PORT=", // empty: ignored
		"OTHER=1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n := kc.Nodes[0]; n.Port != 28083 || n.HTTPPort != 28082 || n.Host != "127.0.0.1" {
		t.Errorf("kuscia-system override not applied: %+v", n)
	}
	if n := kc.Nodes[1]; n.Host != "10.0.0.2" || n.Protocol != "tls" || n.Port != 18083 || n.HTTPPort != 0 {
		t.Errorf("alice override not applied: %+v", n)
	}
}

func TestApplyNodeEnvOverrides_InvalidPort(t *testing.T) {
	for _, v := range []string{"abc", "0", "70000"} {
		kc := KusciaConfig{Nodes: []KusciaNodeConfig{{DomainID: "alice"}}}
		if err := kc.applyNodeEnvOverrides([]string{"PRIVAHUB_KUSCIA_NODES_ALICE_HTTP_PORT=" + v}); err == nil {
			t.Errorf("expected error for port %q", v)
		}
	}
}

func TestLoad_KusciaNodeEnvOverride(t *testing.T) {
	cfgFile := filepath.Join(t.TempDir(), "privahub.yaml")
	content := `
server:
  mode: master
database:
  driver: sqlite
  dsn: ":memory:"
auth:
  jwt_secret: a-very-long-random-secret-value-123456
kuscia:
  api_port: 18083
  http_port: 18082
  nodes:
    - domain_id: "alice"
      mode: "lite"
      host: "127.0.0.1"
      port: 18083
`
	if err := os.WriteFile(cfgFile, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PRIVAHUB_KUSCIA_HTTP_PORT", "28082")
	t.Setenv("PRIVAHUB_KUSCIA_NODES_ALICE_PORT", "28083")
	t.Setenv("PRIVAHUB_KUSCIA_NODES_ALICE_HTTP_PORT", "28082")
	cfg, err := Load(cfgFile)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.Kuscia.HTTPPort != 28082 {
		t.Errorf("kuscia.http_port = %d, want 28082", cfg.Kuscia.HTTPPort)
	}
	if n := cfg.Kuscia.Nodes[0]; n.Port != 28083 || n.HTTPPort != 28082 {
		t.Errorf("alice node = %+v, want port 28083 http_port 28082", n)
	}
}
