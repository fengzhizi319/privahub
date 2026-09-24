package wire

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/fengzhizi319/privahub/pkg/config"
)

func TestNewKusciaClients_HTTPPortAndNodes(t *testing.T) {
	cfg := &config.Config{}
	cfg.Kuscia.APIAddress = "127.0.0.1"
	cfg.Kuscia.APIPort = 8083 // gRPC port must NOT be used for HTTP
	cfg.Kuscia.Protocol = "notls"
	cfg.Kuscia.PoolFile = filepath.Join(t.TempDir(), "nodes.json")
	cfg.Kuscia.Nodes = []config.KusciaNodeConfig{{DomainID: "alice", Host: "10.0.0.1", Port: 8083}}

	c, pool := NewKusciaClients(cfg, nil)
	if !strings.HasSuffix(c.BaseURL(), ":8082") {
		t.Errorf("default client must use HTTP port 8082, got %s", c.BaseURL())
	}
	alice, ok := pool.Lookup("alice")
	if !ok || alice.BaseURL() != "http://10.0.0.1:8082" {
		t.Errorf("alice client = %v %v", alice, ok)
	}
	if pool.Get("bob") != c {
		t.Error("unknown node must fall back to default client")
	}

	cfg.Kuscia.HTTPPort = 18082
	c, _ = NewKusciaClients(cfg, nil)
	if !strings.HasSuffix(c.BaseURL(), ":18082") {
		t.Errorf("configured http_port ignored: %s", c.BaseURL())
	}
}
