package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidate_PlatformModes(t *testing.T) {
	for _, m := range []string{"master", "CENTER", "lite", "EDGE", "autonomy", "P2P", "TEST"} {
		c := &Config{Server: ServerConfig{Mode: m, HTTPPort: 8080}, Database: DatabaseConfig{Driver: "sqlite"},
			Auth: AuthConfig{JWTSecret: strongSecret}}
		if err := c.Validate(); err != nil {
			t.Errorf("mode %q should be valid: %v", m, err)
		}
	}
}

func TestValidate_JWTSecretRequiredInNonDevProfile(t *testing.T) {
	c := &Config{Profile: "edge", Server: ServerConfig{Mode: "lite", HTTPPort: 8080}, Database: DatabaseConfig{Driver: "sqlite"}}
	if err := c.Validate(); err == nil {
		t.Fatal("empty jwt secret must fail for a non-dev profile")
	}
	c.Profile = "dev"
	if err := c.Validate(); err != nil || c.Auth.JWTSecret == "" {
		t.Fatalf("dev profile should default the secret: %v", err)
	}
}

const strongSecret = "a-very-long-random-secret-value-123456"

func baseConfig(profile string) *Config {
	return &Config{Profile: profile, Server: ServerConfig{Mode: "master", HTTPPort: 8080, InnerPort: 9001, InnerBindAddress: "127.0.0.1"},
		Database: DatabaseConfig{Driver: "sqlite"}, Auth: AuthConfig{JWTSecret: strongSecret}}
}

// No profile (and every non-dev profile) is production for secrets.
func TestValidate_ProductionSecrets(t *testing.T) {
	no := false
	cases := []struct {
		name    string
		profile string
		mutate  func(*Config)
		wantErr bool
	}{
		{"strong secret, no profile", "", nil, false},
		{"empty secret, no profile", "", func(c *Config) { c.Auth.JWTSecret = "" }, true},
		{"sample secret, no profile", "", func(c *Config) { c.Auth.JWTSecret = DevJWTSecret }, true},
		{"sample secret, edge profile", "edge", func(c *Config) { c.Auth.JWTSecret = "${JWT_SECRET:change-me-in-production}" }, true},
		{"short secret", "", func(c *Config) { c.Auth.JWTSecret = "short" }, true},
		{"sample aes key", "", func(c *Config) { c.Crypto.AESKey = SampleAESKey }, true},
		{"custom aes key", "", func(c *Config) { c.Crypto.AESKey = "0123456789abcdef0123456789abcdeX" }, false},
		{"auth disabled in production", "", func(c *Config) { c.Auth.Enabled = &no }, true},
		{"dev: sample secret + aes + auth disabled", "dev", func(c *Config) {
			c.Auth.JWTSecret, c.Crypto.AESKey, c.Auth.Enabled = DevJWTSecret, SampleAESKey, &no
		}, false},
		{"test: empty secret gets default", "test", func(c *Config) { c.Auth.JWTSecret = "" }, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := baseConfig(tc.profile)
			if tc.mutate != nil {
				tc.mutate(c)
			}
			err := c.Validate()
			if (err != nil) != tc.wantErr {
				t.Fatalf("Validate() err = %v, wantErr %v", err, tc.wantErr)
			}
			if err == nil && c.Auth.JWTSecret == "" {
				t.Fatal("secret must be set after validation")
			}
		})
	}
}

func TestValidate_InnerPortExposure(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(*Config)
		wantErr bool
	}{
		{"loopback", nil, false},
		{"0.0.0.0 without auth", func(c *Config) { c.Server.InnerBindAddress = "0.0.0.0" }, true},
		{"empty address binds all", func(c *Config) { c.Server.InnerBindAddress = "" }, true},
		{"0.0.0.0 with token", func(c *Config) {
			c.Server.InnerBindAddress = "0.0.0.0"
			c.Server.InnerAuth.Token = "0123456789abcdef-token"
		}, false},
		{"0.0.0.0 with short token", func(c *Config) {
			c.Server.InnerBindAddress = "0.0.0.0"
			c.Server.InnerAuth.Token = "short"
		}, true},
		{"0.0.0.0 with allowlist", func(c *Config) {
			c.Server.InnerBindAddress = "0.0.0.0"
			c.Server.InnerAuth.AllowedCIDRs = []string{"172.17.0.0/16", "10.1.2.3"}
		}, false},
		{"bad cidr", func(c *Config) { c.Server.InnerAuth.AllowedCIDRs = []string{"nope"} }, true},
		{"0.0.0.0 with mtls", func(c *Config) {
			c.Server.InnerBindAddress = "0.0.0.0"
			c.Server.InnerAuth.TLS.CertFile, c.Server.InnerAuth.TLS.KeyFile, c.Server.InnerAuth.TLS.ClientCAFile = "c", "k", "ca"
		}, false},
		{"partial tls", func(c *Config) { c.Server.InnerAuth.TLS.CertFile = "c" }, true},
		{"bad metrics access", func(c *Config) { c.Observability.MetricsAccess = "open" }, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := baseConfig("")
			if tc.mutate != nil {
				tc.mutate(c)
			}
			if err := c.Validate(); (err != nil) != tc.wantErr {
				t.Fatalf("Validate() err = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestLoad_MetricsAccessDefaults(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "privahub.yaml")
	if err := os.WriteFile(cfgPath, []byte("server:\n  mode: master\nauth:\n  jwt_secret: "+strongSecret+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PRIVAHUB_PROFILE", "")
	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Observability.MetricsAccess != MetricsRestricted {
		t.Errorf("production metrics access = %q", cfg.Observability.MetricsAccess)
	}
	t.Setenv("PRIVAHUB_PROFILE", "dev")
	if cfg, err = Load(cfgPath); err != nil || cfg.Observability.MetricsAccess != MetricsPublic {
		t.Errorf("dev metrics access = %v %v", cfg, err)
	}
}

func TestSecurityWarnings(t *testing.T) {
	no := false
	c := baseConfig("dev")
	c.Auth.Enabled = &no
	if len(c.SecurityWarnings()) == 0 {
		t.Error("auth disabled (dev) should warn")
	}
	c = baseConfig("")
	c.Observability.MetricsAccess = MetricsPublic
	if len(c.SecurityWarnings()) == 0 {
		t.Error("public metrics in production should warn")
	}
	if c = baseConfig(""); len(c.SecurityWarnings()) != 0 {
		t.Errorf("strong config should not warn: %v", c.SecurityWarnings())
	}
}

func TestLoad_AuthAndServerDefaults(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "privahub.yaml")
	if err := os.WriteFile(cfgPath, []byte("server:\n  mode: master\nauth:\n  jwt_secret: "+strongSecret+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Auth.SessionIdleTimeout.Hours() != 24 || cfg.Auth.MaxAttempts != 5 || cfg.Auth.LockMinutes != 30 {
		t.Errorf("auth defaults wrong: %+v", cfg.Auth)
	}
	if !cfg.Auth.AuthEnabled() {
		t.Error("auth enabled by default")
	}
	if cfg.Server.InnerBindAddress != "127.0.0.1" || len(cfg.Server.InnerPortPaths) != 5 {
		t.Errorf("inner port defaults wrong: %q %v", cfg.Server.InnerBindAddress, cfg.Server.InnerPortPaths)
	}
	if cfg.Server.UploadMaxSize != -1 || cfg.Server.MaxBodySize != 32<<20 {
		t.Errorf("body limits wrong: %d %d", cfg.Server.UploadMaxSize, cfg.Server.MaxBodySize)
	}
	if len(cfg.Server.CORSAllowedOrigins) == 0 || len(cfg.Server.ExtraResponseHeaders) == 0 {
		t.Error("cors/extra headers defaults missing")
	}
	if cfg.IsDev() {
		t.Error("no profile is not dev")
	}
}
