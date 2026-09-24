package config

import (
	"os"
	"strings"
	"testing"
)

const testInnerToken = "inner-token-0123456789abcdef"

const testAESKey = "0f1e2d3c4b5a69788796a5b4c3d2e1f00f1e2d3c4b5a69788796a5b4c3d2e1f0"

// TestMain gives Load()-based tests (no profile == production) a secure
// transport baseline: TLS proxy + trusted proxy, Kuscia TLS, an at-rest key.
func TestMain(m *testing.M) {
	for k, v := range map[string]string{
		"PRIVAHUB_SERVER_BEHIND_TLS_PROXY": "true",
		"PRIVAHUB_SERVER_TRUSTED_PROXIES":  "127.0.0.1",
		"PRIVAHUB_KUSCIA_PROTOCOL":         "tls",
		"PRIVAHUB_CRYPTO_AES_KEY":          testAESKey,
		"PRIVAHUB_SERVER_INNER_AUTH_TOKEN": testInnerToken,
	} {
		if _, set := os.LookupEnv(k); !set {
			_ = os.Setenv(k, v)
		}
	}
	os.Exit(m.Run())
}

// secureTransport makes a hand-built config satisfy the production transport
// baseline.
func secureTransport(c *Config) *Config {
	c.Server.BehindTLSProxy = true
	c.Server.TrustedProxies = []string{"10.0.0.1"}
	c.Kuscia.Protocol = "mtls"
	if c.Crypto.AESKey == "" {
		c.Crypto.AESKey = testAESKey
	}
	// Production requires inner_auth even on a loopback inner port.
	if c.Server.InnerAuth.Token == "" && len(c.Server.InnerAuth.AllowedCIDRs) == 0 {
		c.Server.InnerAuth.Token = testInnerToken
	}
	return c
}

func TestValidate_TransportSecurityProduction(t *testing.T) {
	cases := []struct {
		name    string
		profile string
		mutate  func(*Config)
		wantErr string
	}{
		{"secure baseline", "", nil, ""},
		{"plain http main port", "", func(c *Config) { c.Server.BehindTLSProxy = false }, "plain HTTP"},
		{"proxy without trusted proxies", "", func(c *Config) { c.Server.TrustedProxies = nil }, "trusted_proxies"},
		{"main port tls", "", func(c *Config) {
			c.Server.BehindTLSProxy, c.Server.TrustedProxies = false, nil
			c.Server.TLS = ServerTLSConfig{Enabled: true, CertFile: "c", KeyFile: "k"}
		}, ""},
		{"tls without cert", "", func(c *Config) { c.Server.TLS = ServerTLSConfig{Enabled: true} }, "cert_file"},
		{"tls 1.1 refused", "", func(c *Config) {
			c.Server.TLS = ServerTLSConfig{Enabled: true, CertFile: "c", KeyFile: "k", MinVersion: "1.1"}
		}, "min_version"},
		{"tlcp needs sm2 certs", "", func(c *Config) {
			c.Server.TLS = ServerTLSConfig{Enabled: true, Stack: "tlcp"}
		}, "tlcp"},
		{"bad stack", "", func(c *Config) {
			c.Server.TLS = ServerTLSConfig{Enabled: true, CertFile: "c", KeyFile: "k", Stack: "ssl"}
		}, "stack"},
		{"kuscia notls", "", func(c *Config) { c.Kuscia.Protocol = "notls" }, "kuscia.protocol"},
		{"kuscia node notls", "", func(c *Config) {
			c.Kuscia.Nodes = []KusciaNodeConfig{{DomainID: "alice", Protocol: "notls"}}
		}, "kuscia.nodes"},
		{"kuscia insecure skip verify", "", func(c *Config) { c.Kuscia.InsecureSkipVerify = true }, "insecure_skip_verify"},
		{"no at-rest key", "", func(c *Config) { c.Crypto.AESKey = "" }, "at-rest"},
		{"versioned keys", "", func(c *Config) {
			c.Crypto.AESKey = ""
			c.Crypto.Keys = map[string]string{"v1": testAESKey, "v2": strings.Repeat("ab", 32)}
			c.Crypto.ActiveVersion = "v2"
		}, ""},
		{"several keys need active version", "", func(c *Config) {
			c.Crypto.AESKey = ""
			c.Crypto.Keys = map[string]string{"v2": testAESKey, "v3": strings.Repeat("ab", 32)}
		}, "active_version"},
		{"dev allows notls + plain http", "dev", func(c *Config) {
			c.Server.BehindTLSProxy, c.Kuscia.Protocol, c.Crypto.AESKey = false, "notls", ""
		}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := baseConfig(tc.profile)
			if tc.mutate != nil {
				tc.mutate(c)
			}
			err := c.Validate()
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("err = %v, want containing %q", err, tc.wantErr)
			}
		})
	}
}

func TestUploadMaxSizeNormalization(t *testing.T) {
	c := baseConfig("")
	if err := c.Validate(); err != nil || c.Server.UploadMaxSize != DefaultUploadMaxSize {
		t.Fatalf("0 => default: %d %v", c.Server.UploadMaxSize, err)
	}
	c = baseConfig("")
	c.Server.UploadMaxSize = -1
	if err := c.Validate(); err != nil || c.Server.UploadMaxSize != -1 {
		t.Fatalf("-1 stays unlimited: %d %v", c.Server.UploadMaxSize, err)
	}
}

func TestTLSMinVersion(t *testing.T) {
	for in, ok := range map[string]bool{"": true, "1.2": true, "TLS1.3": true, "1.0": false, "1.1": false} {
		if _, err := (ServerTLSConfig{MinVersion: in}).TLSMinVersion(); (err == nil) != ok {
			t.Errorf("%q: err=%v", in, err)
		}
	}
}
