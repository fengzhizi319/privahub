package config
package config

import (
	"os"
	"testing"
)

func TestLoad_Defaults(t *testing.T) {
	// Create a minimal config file
	tmpFile, err := os.CreateTemp("", "privahub-test-*.yaml")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	content := `
server:
  mode: master
  http_port: 8080
database:
  driver: sqlite
  dsn: ":memory:"
`
	if _, err := tmpFile.WriteString(content); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}
	tmpFile.Close()

	cfg, err := Load(tmpFile.Name())
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// Verify defaults are applied
	if cfg.Server.Mode != "master" {
		t.Errorf("expected mode 'master', got %q", cfg.Server.Mode)
	}
	if cfg.Server.InnerPort != 9001 {
		t.Errorf("expected inner_port 9001, got %d", cfg.Server.InnerPort)
	}
	if cfg.Kuscia.APIPort != 8083 {
		t.Errorf("expected kuscia api_port 8083, got %d", cfg.Kuscia.APIPort)
	}
	if cfg.Auth.JWTSecret == "" {
		t.Error("expected non-empty JWT secret (default should be set)")
	}
}

func TestValidate_InvalidMode(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{
			Mode:     "invalid",
			HTTPPort: 8080,
		},
		Database: DatabaseConfig{
			Driver: "sqlite",
		},
	}

	err := cfg.Validate()
	if err == nil {
		t.Error("expected error for invalid mode")
	}
}

func TestValidate_InvalidDriver(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{
			Mode:     "master",
			HTTPPort: 8080,
		},
		Database: DatabaseConfig{
			Driver: "postgres",
		},
	}

	err := cfg.Validate()
	if err == nil {
		t.Error("expected error for invalid driver")
	}
}

func TestValidate_InvalidPort(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{
			Mode:     "master",
			HTTPPort: 0,
		},
		Database: DatabaseConfig{
			Driver: "sqlite",
		},
	}

	err := cfg.Validate()
	if err == nil {
		t.Error("expected error for invalid port")
	}
}

func TestValidate_EmptyJWTSecret_GetsDefault(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{
			Mode:     "master",
			HTTPPort: 8080,
		},
		Database: DatabaseConfig{
			Driver: "sqlite",
		},
		Auth: AuthConfig{
			JWTSecret: "",
		},
	}

	err := cfg.Validate()
	if err != nil {
		t.Fatalf("Validate failed: %v", err)
	}

	if cfg.Auth.JWTSecret == "" {
		t.Error("expected default JWT secret to be set")
	}
}

func TestValidate_ValidConfig(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{
			Mode:     "master",
			HTTPPort: 8080,
		},
		Database: DatabaseConfig{
			Driver: "sqlite",
		},
		Auth: AuthConfig{
			JWTSecret: "my-secret",
		},
	}

	err := cfg.Validate()
	if err != nil {
		t.Errorf("expected valid config, got error: %v", err)
	}
}

func TestLoad_EnvironmentOverride(t *testing.T) {
	// Set environment variable
	os.Setenv("PRIVAHUB_SERVER_HTTP_PORT", "9999")
	defer os.Unsetenv("PRIVAHUB_SERVER_HTTP_PORT")

	tmpFile, err := os.CreateTemp("", "privahub-test-*.yaml")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	content := `
server:
  mode: master
  http_port: 8080
database:
  driver: sqlite
`
	if _, err := tmpFile.WriteString(content); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}
	tmpFile.Close()

	cfg, err := Load(tmpFile.Name())
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// Environment should override file
	if cfg.Server.HTTPPort != 9999 {
		t.Errorf("expected http_port 9999 from env, got %d", cfg.Server.HTTPPort)
	}
}
