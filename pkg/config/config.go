// Package config provides configuration loading and management for Privahub.
// It supports multiple deployment modes (master, lite, autonomy) and environment-specific overrides.
package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Config is the root configuration structure for SecretPad-Go.
type Config struct {
	Server        ServerConfig        `mapstructure:"server"`
	Database      DatabaseConfig      `mapstructure:"database"`
	Crypto        CryptoConfig        `mapstructure:"crypto"`
	Kuscia        KusciaConfig        `mapstructure:"kuscia"`
	Observability ObservabilityConfig `mapstructure:"observability"`
	Auth          AuthConfig          `mapstructure:"auth"`
}

// ServerConfig holds HTTP/gRPC server settings.
type ServerConfig struct {
	Mode            string        `mapstructure:"mode"` // master, lite, autonomy
	HTTPPort        int           `mapstructure:"http_port"`
	InnerPort       int           `mapstructure:"inner_port"` // cluster-internal port (no auth)
	GRPCPort        int           `mapstructure:"grpc_port"`
	ReadTimeout     time.Duration `mapstructure:"read_timeout"`
	WriteTimeout    time.Duration `mapstructure:"write_timeout"`
	ShutdownTimeout time.Duration `mapstructure:"shutdown_timeout"`
}

// DatabaseConfig holds database connection settings.
type DatabaseConfig struct {
	Driver       string `mapstructure:"driver"` // sqlite, mysql
	DSN          string `mapstructure:"dsn"`
	MaxOpenConns int    `mapstructure:"max_open_conns"`
	MaxIdleConns int    `mapstructure:"max_idle_conns"`
	MigrateDir   string `mapstructure:"migrate_dir"`
}

// CryptoConfig holds encryption key settings.
type CryptoConfig struct {
	AESKey string `mapstructure:"aes_key"` // 32 bytes for AES-256
}

// KusciaNodeConfig represents a single Kuscia node connection.
type KusciaNodeConfig struct {
	DomainID string `mapstructure:"domain_id"`
	Mode     string `mapstructure:"mode"` // master, lite, autonomy
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	Protocol string `mapstructure:"protocol"` // tls, notls
	CertFile string `mapstructure:"cert_file"`
	KeyFile  string `mapstructure:"key_file"`
	Token    string `mapstructure:"token"`
}

// KusciaConfig holds Kuscia control plane connection settings.
type KusciaConfig struct {
	Kubeconfig string             `mapstructure:"kubeconfig"`
	Namespace  string             `mapstructure:"namespace"`
	APIAddress string             `mapstructure:"api_address"`
	APIPort    int                `mapstructure:"api_port"`  // gRPC port (default 8083)
	HTTPPort   int                `mapstructure:"http_port"` // HTTP external port (default 8082)
	Protocol   string             `mapstructure:"protocol"`  // tls, notls
	Gateway    string             `mapstructure:"gateway"`   // Envoy gateway address
	Nodes      []KusciaNodeConfig `mapstructure:"nodes"`     // multi-node configuration
}

// ObservabilityConfig holds metrics and logging settings.
type ObservabilityConfig struct {
	EnableMetrics bool   `mapstructure:"enable_metrics"`
	MetricsPort   int    `mapstructure:"metrics_port"`
	LogLevel      string `mapstructure:"log_level"`
	LogFormat     string `mapstructure:"log_format"` // json, console
}

// AuthConfig holds JWT authentication settings.
type AuthConfig struct {
	JWTSecret          string        `mapstructure:"jwt_secret"`
	AccessTokenExpiry  time.Duration `mapstructure:"access_token_expiry"`
	RefreshTokenExpiry time.Duration `mapstructure:"refresh_token_expiry"`
}

// Load reads configuration from file and environment variables.
// Configuration file search order: ./config/privahub.yaml, /etc/privahub/privahub.yaml
// Environment variables with prefix PRIVAHUB_ override file values.
// Profile support: set PRIVAHUB_PROFILE=dev|edge|p2p|test to load privahub-{profile}.yaml overrides.
func Load(cfgFile string) (*Config, error) {
	v := viper.New()

	// Set defaults
	setDefaults(v)

	// Config file
	if cfgFile != "" {
		v.SetConfigFile(cfgFile)
	} else {
		v.SetConfigName("privahub")
		v.SetConfigType("yaml")
		v.AddConfigPath("./config")
		v.AddConfigPath("/etc/privahub")
		v.AddConfigPath(".")
	}

	// Environment variable override
	v.SetEnvPrefix("PRIVAHUB")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// Read base config file (ignore if not found)
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}
	}

	// Load profile-specific overrides (e.g., privahub-dev.yaml)
	profile := v.GetString("profile")
	if profile == "" {
		profile = v.GetString("PRIVAHUB_PROFILE") // fallback to env
	}
	if profile != "" {
		pv := viper.New()
		pv.SetConfigName("privahub-" + profile)
		pv.SetConfigType("yaml")
		pv.AddConfigPath("./config")
		pv.AddConfigPath("/etc/privahub")
		pv.AddConfigPath(".")
		if err := pv.ReadInConfig(); err == nil {
			if err := v.MergeConfigMap(pv.AllSettings()); err != nil {
				return nil, fmt.Errorf("failed to merge profile config %q: %w", profile, err)
			}
		}
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return &cfg, nil
}

// Validate checks configuration for required fields and valid values.
func (c *Config) Validate() error {
	validModes := map[string]bool{"master": true, "lite": true, "autonomy": true}
	if !validModes[c.Server.Mode] {
		return fmt.Errorf("invalid server mode %q, must be one of: master, lite, autonomy", c.Server.Mode)
	}

	validDrivers := map[string]bool{"sqlite": true, "mysql": true}
	if !validDrivers[c.Database.Driver] {
		return fmt.Errorf("invalid database driver %q, must be one of: sqlite, mysql", c.Database.Driver)
	}

	if c.Server.HTTPPort <= 0 || c.Server.HTTPPort > 65535 {
		return fmt.Errorf("invalid http_port %d", c.Server.HTTPPort)
	}

	// Security: JWT secret must not be empty in production. A default dev
	// secret is provided for convenience but should be overridden.
	if c.Auth.JWTSecret == "" {
		c.Auth.JWTSecret = "privahub-dev-secret-change-in-production"
	}

	return nil
}

func setDefaults(v *viper.Viper) {
	v.SetDefault("server.mode", "master")
	v.SetDefault("server.http_port", 8080)
	v.SetDefault("server.inner_port", 9001)
	v.SetDefault("server.grpc_port", 9090)
	v.SetDefault("server.read_timeout", "10s")
	v.SetDefault("server.write_timeout", "10s")
	v.SetDefault("server.shutdown_timeout", "5s")

	v.SetDefault("database.driver", "sqlite")
	v.SetDefault("database.dsn", "privahub.db?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
	v.SetDefault("database.max_open_conns", 50)
	v.SetDefault("database.max_idle_conns", 10)

	v.SetDefault("kuscia.namespace", "kuscia-system")
	v.SetDefault("kuscia.api_address", "127.0.0.1")
	v.SetDefault("kuscia.api_port", 8083)
	v.SetDefault("kuscia.http_port", 8082)
	v.SetDefault("kuscia.protocol", "notls")

	v.SetDefault("observability.enable_metrics", true)
	v.SetDefault("observability.metrics_port", 9091)
	v.SetDefault("observability.log_level", "info")
	v.SetDefault("observability.log_format", "json")

	v.SetDefault("auth.access_token_expiry", "2h")
	v.SetDefault("auth.refresh_token_expiry", "168h") // 7 days
}
