// Package config provides configuration loading and management for Privahub.
// It supports multiple deployment modes (master, lite, autonomy) and environment-specific overrides.
package config

import (
	"fmt"
	"net"
	"os"
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
	// Data holds datasource/datatable/upload/TEE/cloud-log settings (data.go).
	Data DataPlatformConfig `mapstructure:"data"`
	// Sync holds center/edge/P2P data-sync, edge request forwarding and
	// startup-init settings (sync.go).
	Sync SyncConfig `mapstructure:"sync"`

	// Profile is the active profile (PRIVAHUB_PROFILE / "profile" key), e.g.
	// dev, edge, p2p, test. Empty means the base (production) configuration.
	Profile string `mapstructure:"profile"`
}

// IsDev reports whether a development/test profile is active. Dev profiles
// keep convenience defaults (admin/12345678, built-in JWT secret, plaintext
// password login); every other profile — including no profile — is treated
// as production.
func (c *Config) IsDev() bool {
	switch strings.ToLower(c.Profile) {
	case "dev", "test", "local":
		return true
	}
	return false
}

// ServerConfig holds HTTP/gRPC server settings.
type ServerConfig struct {
	// Mode is the platform type. Accepted (case-insensitive): CENTER/master,
	// EDGE/lite, AUTONOMY/P2P, TEST. Normalized by service.NormalizePlatformType.
	Mode            string        `mapstructure:"mode"`
	HTTPPort        int           `mapstructure:"http_port"`
	InnerPort       int           `mapstructure:"inner_port"` // cluster-internal port (no auth)
	GRPCPort        int           `mapstructure:"grpc_port"`
	ReadTimeout     time.Duration `mapstructure:"read_timeout"`
	WriteTimeout    time.Duration `mapstructure:"write_timeout"`
	ShutdownTimeout time.Duration `mapstructure:"shutdown_timeout"`

	// NodeID is this platform's node id (Java secretpad.node-id). Defaults to
	// kuscia.namespace when empty.
	NodeID string `mapstructure:"node_id"`
	// DeployMode is Java secretpad.deploy-mode: MPC | TEE | ALL-IN-ONE.
	DeployMode string `mapstructure:"deploy_mode"`
	// InnerBindAddress is the listen address of the inner (cluster) port.
	// Default 127.0.0.1; set 0.0.0.0 when Kuscia's gateway reaches it from
	// another host/container.
	InnerBindAddress string `mapstructure:"inner_bind_address"`
	// InnerAuth protects the inner port when it is bound to a non-loopback
	// address (see InnerAuthConfig).
	InnerAuth InnerAuthConfig `mapstructure:"inner_auth"`
	// InnerPortPaths are only served on the inner port; the main port answers
	// 404 for them (Java inner-port.path).
	InnerPortPaths []string `mapstructure:"inner_port_paths"`
	// CenterPlatformService is the Host header used when proxying edge
	// requests to the center through the Kuscia gateway
	// (Java secretpad.center-platform-service).
	CenterPlatformService string `mapstructure:"center_platform_service"`
	// CORSAllowedOrigins lists allowed CORS origins ("*" allows any origin).
	CORSAllowedOrigins []string `mapstructure:"cors_allowed_origins"`
	// ExtraResponseHeaders are added to every response
	// (Java secretpad.response.extra-headers).
	ExtraResponseHeaders map[string]string `mapstructure:"extra_response_headers"`
	// MaxBodySize is the default request body limit in bytes (<=0: unlimited).
	MaxBodySize int64 `mapstructure:"max_body_size"`
	// UploadMaxSize is the body limit for upload routes (<=0: unlimited,
	// Java upload-file.max-request-size=-1).
	UploadMaxSize int64 `mapstructure:"upload_max_size"`
	// UploadPaths are the routes subject to UploadMaxSize instead of MaxBodySize.
	UploadPaths []string `mapstructure:"upload_paths"`
	// RateLimit configures request throttling (Java RateLimitUtil.verifyRate).
	RateLimit RateLimitConfig `mapstructure:"rate_limit"`
	// APIDocs configures the OpenAPI document endpoint (Java springdoc).
	APIDocs APIDocsConfig `mapstructure:"api_docs"`
}

// InnerAuthConfig secures the inner (node-to-node) port. The inner port
// trusts the kuscia-origin-source header (Java behaviour), so when it listens
// on a non-loopback address at least one of these must be configured; every
// configured mechanism is enforced:
//   - Token / TokenFile: shared secret expected in the X-Privahub-Inner-Token
//     header (sent automatically by this platform's gateway calls when set);
//   - AllowedCIDRs: source IP allowlist (the direct peer address, e.g. the
//     Kuscia gateway / Envoy container network);
//   - TLS: mTLS on the inner listener (server cert + required client certs
//     signed by ClientCAFile).
type InnerAuthConfig struct {
	Token        string   `mapstructure:"token"`
	TokenFile    string   `mapstructure:"token_file"`
	AllowedCIDRs []string `mapstructure:"allowed_cidrs"`
	TLS          struct {
		CertFile     string `mapstructure:"cert_file"`
		KeyFile      string `mapstructure:"key_file"`
		ClientCAFile string `mapstructure:"client_ca_file"`
	} `mapstructure:"tls"`
}

// MTLSEnabled reports whether the inner listener uses mTLS.
func (a InnerAuthConfig) MTLSEnabled() bool {
	return a.TLS.CertFile != "" && a.TLS.KeyFile != "" && a.TLS.ClientCAFile != ""
}

// ResolvedToken returns Token, or the trimmed content of TokenFile.
func (a InnerAuthConfig) ResolvedToken() (string, error) {
	if a.Token != "" {
		return a.Token, nil
	}
	if a.TokenFile == "" {
		return "", nil
	}
	b, err := os.ReadFile(a.TokenFile)
	if err != nil {
		return "", fmt.Errorf("server.inner_auth.token_file: %w", err)
	}
	return strings.TrimSpace(string(b)), nil
}

// IsLoopbackBind reports whether addr (a listen host) only accepts local
// connections.
func IsLoopbackBind(addr string) bool {
	h := strings.Trim(strings.TrimSpace(addr), "[]")
	if h == "localhost" {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && ip.IsLoopback()
}

// RateLimitConfig mirrors Java RateLimitUtil: user-scoped limits on sensitive
// create endpoints (verifyRate(): 5 permits / 60s per user name, Guava
// RateLimiter semantics, i.e. evenly spaced with a burst of one) and an
// IP-scoped login limit (secretpad.login-rate-limit-per-minute, default 20).
type RateLimitConfig struct {
	// UserPermits / UserWindow: permits per window per user (<=0 disables).
	UserPermits float64       `mapstructure:"user_permits"`
	UserWindow  time.Duration `mapstructure:"user_window"`
	// UserBurst is the bucket capacity (Java/Guava behaves like 1).
	UserBurst float64 `mapstructure:"user_burst"`
	// UserPaths are the routes guarded by the user limiter (Java callers of
	// verifyRate()). All paths share one bucket per user, like Java.
	UserPaths []string `mapstructure:"user_paths"`
	// LoginPerMinute limits login attempts per client IP (<=0 disables).
	LoginPerMinute float64 `mapstructure:"login_per_minute"`
	// LoginBurst is the login bucket capacity (Java/Guava behaves like 1).
	LoginBurst float64 `mapstructure:"login_burst"`
	// GlobalBurst / GlobalRPS configure the coarse per-IP limiter applied to
	// every request (Go-only safety net; <=0 disables).
	GlobalBurst float64 `mapstructure:"global_burst"`
	GlobalRPS   float64 `mapstructure:"global_rps"`
}

// APIDocsConfig controls GET /api/v1alpha1/docs (OpenAPI 3 document).
type APIDocsConfig struct {
	// Enabled serves the document (default: dev/test profiles only).
	Enabled bool `mapstructure:"enabled"`
	// Public serves it without authentication (default false: login required).
	Public bool `mapstructure:"public"`
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
	Port     int    `mapstructure:"port"`      // KusciaAPI gRPC port (informational)
	HTTPPort int    `mapstructure:"http_port"` // KusciaAPI HTTP port; 0 => kuscia.http_port
	Protocol string `mapstructure:"protocol"`  // tls, mtls, notls
	CertFile string `mapstructure:"cert_file"`
	KeyFile  string `mapstructure:"key_file"`
	CAFile   string `mapstructure:"ca_file"`
	Token    string `mapstructure:"token"` // token value or token file path
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

	// KusciaAPI client security (Java kusciaapi.tls.* / token).
	CertFile           string        `mapstructure:"cert_file"`            // client cert (mTLS)
	KeyFile            string        `mapstructure:"key_file"`             // client key (mTLS)
	CAFile             string        `mapstructure:"ca_file"`              // server CA
	Token              string        `mapstructure:"token"`                // Token header value or file path
	InsecureSkipVerify bool          `mapstructure:"insecure_skip_verify"` // dev only
	Timeout            time.Duration `mapstructure:"timeout"`              // request timeout (default 30s)

	// CertsDir stores certs uploaded by inst node/register (Java secretpad.certs.dir-path).
	CertsDir string `mapstructure:"certs_dir"`
	// PoolFile persists runtime-registered KusciaAPI endpoints (Java secretpad.kuscia-path).
	PoolFile string `mapstructure:"pool_file"`
	// DataProxy enables forcing DomainDataSource access_directly=false (Java secretpad.data-proxy).
	DataProxy bool `mapstructure:"data_proxy"`
}

// ObservabilityConfig holds metrics and logging settings.
type ObservabilityConfig struct {
	EnableMetrics bool   `mapstructure:"enable_metrics"`
	MetricsPort   int    `mapstructure:"metrics_port"`
	LogLevel      string `mapstructure:"log_level"`
	LogFormat     string `mapstructure:"log_format"` // json, console
	// MetricsAccess controls GET /metrics:
	//   public     - no restriction (default in dev/test profiles);
	//   restricted - loopback or MetricsAllowedCIDRs sources, or
	//                "Authorization: Bearer <MetricsToken>" (default otherwise);
	//   auth       - a logged-in session is required;
	//   disabled   - the endpoint is not served.
	MetricsAccess       string   `mapstructure:"metrics_access"`
	MetricsAllowedCIDRs []string `mapstructure:"metrics_allowed_cidrs"`
	MetricsToken        string   `mapstructure:"metrics_token"`
}

// Metrics access modes (observability.metrics_access).
const (
	MetricsPublic     = "public"
	MetricsRestricted = "restricted"
	MetricsAuth       = "auth"
	MetricsDisabled   = "disabled"
)

// AuthConfig holds JWT authentication settings.
type AuthConfig struct {
	JWTSecret          string        `mapstructure:"jwt_secret"`
	AccessTokenExpiry  time.Duration `mapstructure:"access_token_expiry"`
	RefreshTokenExpiry time.Duration `mapstructure:"refresh_token_expiry"`

	// Enabled=false disables authentication and injects a platform admin
	// (Java secretpad.auth.enabled). Default true.
	Enabled *bool `mapstructure:"enabled"`
	// SessionIdleTimeout is the sliding session expiry (Java LoginInterceptor
	// EXPIRE = 24h): a session unused for longer is rejected.
	SessionIdleTimeout time.Duration `mapstructure:"session_idle_timeout"`
	// MaxAttempts / LockMinutes: login lockout (Java
	// secretpad.account-error-max-attempts / account-error-lock-time-minutes).
	MaxAttempts int `mapstructure:"max_attempts"`
	LockMinutes int `mapstructure:"lock_minutes"`
	// ResetPasswordMaxAttempts / ResetPasswordLockMinutes: updatePwd lockout
	// (Java secretpad.reset-password-error-*).
	ResetPasswordMaxAttempts int `mapstructure:"reset_password_max_attempts"`
	ResetPasswordLockMinutes int `mapstructure:"reset_password_lock_minutes"`
	// AdminName / AdminPassword seed the initial admin (Java
	// secretpad.auth.pad_name / pad_pwd). In non-dev profiles an unset
	// password is replaced by a random one written to InitialPasswordFile (0600).
	AdminName           string `mapstructure:"admin_name"`
	AdminPassword       string `mapstructure:"admin_password"`
	InitialPasswordFile string `mapstructure:"initial_password_file"`
	// DataResourceWhitelist lists routes on which AUTONOMY (P2P) users skip
	// the data-resource (node/project owner) check (Java white-list.paths).
	// Empty uses the Java default list (middleware.DefaultAutonomyWhitelist).
	DataResourceWhitelist []string `mapstructure:"data_resource_whitelist"`
}

// AuthEnabled reports whether authentication is enabled (default true).
func (a AuthConfig) AuthEnabled() bool { return a.Enabled == nil || *a.Enabled }

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
	cfg.Profile = profile
	if cfg.Observability.MetricsAccess == "" {
		// /metrics is open only in development profiles (upgrade over Java,
		// whose actuator endpoints are protected by Spring Security).
		cfg.Observability.MetricsAccess = MetricsRestricted
		if cfg.IsDev() {
			cfg.Observability.MetricsAccess = MetricsPublic
		}
	}
	if !v.IsSet("server.api_docs.enabled") {
		// OpenAPI docs are served by default only in development profiles.
		cfg.Server.APIDocs.Enabled = cfg.IsDev()
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return &cfg, nil
}

// Validate checks configuration for required fields and valid values.
func (c *Config) Validate() error {
	validModes := map[string]bool{
		"master": true, "center": true, "lite": true, "edge": true,
		"autonomy": true, "p2p": true, "test": true,
	}
	if !validModes[strings.ToLower(c.Server.Mode)] {
		return fmt.Errorf("invalid server mode %q, must be one of: CENTER(master), EDGE(lite), AUTONOMY(p2p), TEST", c.Server.Mode)
	}

	validDrivers := map[string]bool{"sqlite": true, "mysql": true}
	if !validDrivers[c.Database.Driver] {
		return fmt.Errorf("invalid database driver %q, must be one of: sqlite, mysql", c.Database.Driver)
	}

	if c.Server.HTTPPort <= 0 || c.Server.HTTPPort > 65535 {
		return fmt.Errorf("invalid http_port %d", c.Server.HTTPPort)
	}

	return c.validateSecurity()
}

// profileHint is appended to production-secret errors.
const profileHint = " (for local development start with PRIVAHUB_PROFILE=dev, which keeps the built-in dev defaults)"

// validateSecurity enforces the production security baseline. Every profile
// except dev/test/local — including no profile at all — is production:
//   - auth.jwt_secret must be set and not a well-known sample (>= 16 chars);
//   - crypto.aes_key must not be the sample key shipped in privahub.yaml;
//   - auth.enabled=false is refused;
//
// and, in every profile, a non-loopback inner port needs inner_auth.
func (c *Config) validateSecurity() error {
	if c.IsDev() {
		if c.Auth.JWTSecret == "" {
			c.Auth.JWTSecret = DevJWTSecret
		}
	} else {
		switch {
		case c.Auth.JWTSecret == "":
			return fmt.Errorf("auth.jwt_secret is not configured: set PRIVAHUB_AUTH_JWT_SECRET to a random secret (e.g. `openssl rand -hex 32`)" + profileHint)
		case insecureJWTSecrets[c.Auth.JWTSecret] || len(c.Auth.JWTSecret) < 16:
			return fmt.Errorf("auth.jwt_secret is a well-known sample or shorter than 16 characters: set PRIVAHUB_AUTH_JWT_SECRET to a random secret (e.g. `openssl rand -hex 32`)" + profileHint)
		}
		if insecureAESKeys[c.Crypto.AESKey] {
			return fmt.Errorf("crypto.aes_key is the sample key from privahub.yaml: set PRIVAHUB_CRYPTO_AES_KEY to 32 random bytes (e.g. `openssl rand -hex 16`)" + profileHint)
		}
		if !c.Auth.AuthEnabled() {
			return fmt.Errorf("auth.enabled=false disables authentication and is only allowed with PRIVAHUB_PROFILE=dev|test")
		}
	}
	if err := c.validateInnerAuth(); err != nil {
		return err
	}
	switch c.Observability.MetricsAccess {
	case "", MetricsPublic, MetricsRestricted, MetricsAuth, MetricsDisabled:
	default:
		return fmt.Errorf("invalid observability.metrics_access %q, must be one of: public, restricted, auth, disabled", c.Observability.MetricsAccess)
	}
	for _, cidr := range c.Observability.MetricsAllowedCIDRs {
		if _, err := ParseCIDROrIP(cidr); err != nil {
			return fmt.Errorf("observability.metrics_allowed_cidrs: %w", err)
		}
	}
	return nil
}

// validateInnerAuth requires inner_auth when the inner port is reachable
// from other hosts, and validates its settings.
func (c *Config) validateInnerAuth() error {
	a := c.Server.InnerAuth
	for _, cidr := range a.AllowedCIDRs {
		if _, err := ParseCIDROrIP(cidr); err != nil {
			return fmt.Errorf("server.inner_auth.allowed_cidrs: %w", err)
		}
	}
	tls := a.TLS
	if (tls.CertFile != "" || tls.KeyFile != "" || tls.ClientCAFile != "") && !a.MTLSEnabled() {
		return fmt.Errorf("server.inner_auth.tls needs cert_file, key_file and client_ca_file together")
	}
	token, err := a.ResolvedToken()
	if err != nil {
		return err
	}
	if token != "" && len(token) < 16 {
		return fmt.Errorf("server.inner_auth.token must be at least 16 characters")
	}
	// An empty address with a port binds every interface.
	if IsLoopbackBind(c.Server.InnerBindAddress) || (c.Server.InnerBindAddress == "" && c.Server.InnerPort == 0) {
		return nil
	}
	if token == "" && len(a.AllowedCIDRs) == 0 && !a.MTLSEnabled() {
		return fmt.Errorf("server.inner_bind_address=%q exposes the inner port, which trusts the kuscia-origin-source header: "+
			"configure server.inner_auth (token / token_file, allowed_cidrs of the Kuscia gateway, or tls for mTLS), "+
			"or bind it to 127.0.0.1", c.Server.InnerBindAddress)
	}
	return nil
}

// ParseCIDROrIP parses "10.0.0.0/8" or a single IP ("10.1.2.3" => /32 or /128).
func ParseCIDROrIP(s string) (*net.IPNet, error) {
	s = strings.TrimSpace(s)
	if strings.Contains(s, "/") {
		_, n, err := net.ParseCIDR(s)
		return n, err
	}
	ip := net.ParseIP(s)
	if ip == nil {
		return nil, fmt.Errorf("invalid IP or CIDR %q", s)
	}
	bits := 32
	if ip.To4() == nil {
		bits = 128
	}
	return &net.IPNet{IP: ip, Mask: net.CIDRMask(bits, bits)}, nil
}

// SampleAESKey is the placeholder crypto.aes_key shipped in privahub.yaml.
const SampleAESKey = "12345678901234567890123456789012"

var insecureAESKeys = map[string]bool{SampleAESKey: true}

// DevJWTSecret is the built-in development JWT secret.
const DevJWTSecret = "privahub-dev-secret-change-in-production"

// insecureJWTSecrets are well-known secrets shipped in sample configs.
var insecureJWTSecrets = map[string]bool{
	DevJWTSecret:                            true,
	"dev-secret-do-not-use-in-production":   true,
	"change-me-in-production":               true,
	"${JWT_SECRET:change-me-in-production}": true,
}

// SecurityWarnings returns non-fatal security findings for the loaded
// configuration (logged at startup). Fatal ones are rejected by Validate.
func (c *Config) SecurityWarnings() []string {
	var w []string
	if c.IsDev() {
		if !c.Auth.AuthEnabled() {
			w = append(w, "auth.enabled=false: authentication is DISABLED (dev/test only)")
		}
	}
	if c.Observability.MetricsAccess == MetricsPublic && !c.IsDev() {
		w = append(w, "observability.metrics_access=public: /metrics is served without restriction")
	}
	return w
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
	v.SetDefault("kuscia.timeout", "30s")
	v.SetDefault("kuscia.certs_dir", "./config/certs/")
	v.SetDefault("kuscia.pool_file", "./config/kuscia/nodes.json")

	v.SetDefault("observability.enable_metrics", true)
	v.SetDefault("observability.metrics_port", 9091)
	v.SetDefault("observability.log_level", "info")
	v.SetDefault("observability.log_format", "json")

	v.SetDefault("server.inner_bind_address", "127.0.0.1")
	// Registered so that PRIVAHUB_* environment variables apply even when
	// the config file does not mention the key (viper AutomaticEnv only
	// overrides known keys).
	v.SetDefault("auth.jwt_secret", "")
	v.SetDefault("crypto.aes_key", "")
	v.SetDefault("server.inner_auth.token", "")
	v.SetDefault("server.inner_auth.token_file", "")
	v.SetDefault("server.inner_auth.allowed_cidrs", []string{})
	v.SetDefault("server.inner_auth.tls.cert_file", "")
	v.SetDefault("server.inner_auth.tls.key_file", "")
	v.SetDefault("server.inner_auth.tls.client_ca_file", "")
	v.SetDefault("observability.metrics_access", "")
	v.SetDefault("observability.metrics_allowed_cidrs", []string{})
	v.SetDefault("observability.metrics_token", "")
	v.SetDefault("server.deploy_mode", "ALL-IN-ONE")
	v.SetDefault("server.center_platform_service", "secretpad.master.svc")
	v.SetDefault("server.inner_port_paths", []string{
		"/api/v1alpha1/vote_sync/create",
		"/api/v1alpha1/user/node/resetPassword",
		"/sync",
		"/api/v1alpha1/data/sync",
		// Go also exposed the SSE sync under /api/v1alpha1 on the main port.
		"/api/v1alpha1/sync",
	})
	v.SetDefault("server.cors_allowed_origins", []string{"http://localhost:8000", "http://127.0.0.1:8000"})
	v.SetDefault("server.extra_response_headers", map[string]string{
		"Content-Security-Policy": "base-uri 'self';frame-src 'self';worker-src blob: 'self' data:;object-src 'self';",
	})
	v.SetDefault("server.max_body_size", 32<<20)
	v.SetDefault("server.upload_max_size", -1)
	v.SetDefault("server.upload_paths", []string{
		"/api/v1alpha1/data/upload",
		"/api/v1alpha1/inst/node/register",
	})

	v.SetDefault("server.rate_limit.user_permits", 5)
	v.SetDefault("server.rate_limit.user_window", "60s")
	v.SetDefault("server.rate_limit.user_burst", 1)
	v.SetDefault("server.rate_limit.user_paths", []string{
		"/api/v1alpha1/inst/node/newToken",        // InstServiceImpl.newToken
		"/api/v1alpha1/datasource/create",         // DatasourceServiceImpl.createDatasource
		"/api/v1alpha1/datatable/create",          // DatatableServiceImpl.createDataTable
		"/api/v1alpha1/feature_datasource/create", // FeatureTableServiceImpl.createFeatureTable
	})
	v.SetDefault("server.rate_limit.login_per_minute", 20)
	v.SetDefault("server.rate_limit.login_burst", 1)
	v.SetDefault("server.rate_limit.global_burst", 100)
	v.SetDefault("server.rate_limit.global_rps", 20)

	v.SetDefault("auth.session_idle_timeout", "24h")
	v.SetDefault("auth.max_attempts", 5)
	v.SetDefault("auth.lock_minutes", 30)
	v.SetDefault("auth.reset_password_max_attempts", 5)
	v.SetDefault("auth.reset_password_lock_minutes", 60)
	v.SetDefault("auth.admin_name", "admin")
	v.SetDefault("auth.initial_password_file", "./config/initial-admin-password")

	// Access tokens are additionally bound to a DB session with a sliding
	// idle timeout (auth.session_idle_timeout, 24h like Java); the JWT expiry is
	// the absolute session cap.
	v.SetDefault("auth.access_token_expiry", "168h")
	v.SetDefault("auth.refresh_token_expiry", "168h") // 7 days
}
