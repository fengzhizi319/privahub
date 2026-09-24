package config

import "time"

// SyncConfig mirrors the Java data-sync / edge / init properties:
//
//	data.sync                       -> sync.tables
//	secretpad.datasync.center       -> sync.center (default: true on CENTER)
//	secretpad.datasync.p2p          -> sync.p2p    (default: true on AUTONOMY)
//	edge.sync.enabled               -> sync.edge_enabled (default true on EDGE)
//	secretpad.sync-path             -> sync.p2p_queue_dir (default ./config/sync/)
//	edge.forward / edge.include     -> sync.edge_forward / sync.edge_include
//	unregister-mapping.path         -> sync.disabled_routes
//	secretpad.tee / KUSCIA_API_LITE_TEE_ADDRESS ... -> sync.tee.*
//
// Every list falls back to the Java defaults of the active platform when empty.
type SyncConfig struct {
	// Tables are the synchronized tables (Go table names such as "project" or
	// Java entity class names such as
	// org.secretflow.secretpad.persistence.entity.ProjectDO).
	Tables []string `mapstructure:"tables"`
	// Center enables the CENTER producer (GORM hooks -> SSE push).
	Center *bool `mapstructure:"center"`
	// P2P enables the AUTONOMY producer (GORM hooks -> /api/v1alpha1/data/sync).
	P2P *bool `mapstructure:"p2p"`
	// EdgeEnabled enables the EDGE SSE client (Java edge.sync.enabled).
	EdgeEnabled *bool `mapstructure:"edge_enabled"`
	// EdgeReconnectInterval is the EDGE reconnect period (Java EdgeSyncTask 5s).
	EdgeReconnectInterval time.Duration `mapstructure:"edge_reconnect_interval"`
	// EdgeIdleTimeout drops the SSE stream when nothing (not even a ping) was
	// received for this long (default 60s; center pings every 15s).
	EdgeIdleTimeout time.Duration `mapstructure:"edge_idle_timeout"`
	// DispatchInterval is the change dispatcher period (Java DbChangeEventListener 3s).
	DispatchInterval time.Duration `mapstructure:"dispatch_interval"`
	// PingInterval is the SSE heartbeat period (Java 15s).
	PingInterval time.Duration `mapstructure:"ping_interval"`
	// MaxRetries bounds the resend attempts of a failed change (Java 3).
	MaxRetries int `mapstructure:"max_retries"`
	// P2PQueueDir persists pending P2P changes per destination (Java secretpad.sync-path).
	P2PQueueDir string `mapstructure:"p2p_queue_dir"`
	// HTTPTimeout bounds node-to-node sync calls (vote_sync, data/sync).
	HTTPTimeout time.Duration `mapstructure:"http_timeout"`

	// EdgeForward lists the EDGE write endpoints proxied to the center
	// (Java edge.forward).
	EdgeForward []string `mapstructure:"edge_forward"`
	// EdgeInclude lists the /api/v1alpha1 endpoints served on EDGE; any other
	// /api/v1alpha1 path answers 404 (Java edge.include).
	EdgeInclude []string `mapstructure:"edge_include"`
	// EdgeIncludeExtra is appended to EdgeInclude (Go-only aliases such as
	// /api/v1alpha1/user/login); defaults to DefaultEdgeIncludeExtra.
	EdgeIncludeExtra []string `mapstructure:"edge_include_extra"`

	// DisabledRoutes answer 404 (Java DynamicBeanRegisterInit +
	// unregister-mapping.path). Empty by default (opt-in, the Go frontend may
	// still call some of them); the Java AUTONOMY list is
	// DefaultP2PDisabledRoutes and is used when DisableJavaP2PRoutes is true.
	DisabledRoutes []string `mapstructure:"disabled_routes"`
	// DisableJavaP2PRoutes applies DefaultP2PDisabledRoutes on AUTONOMY.
	DisableJavaP2PRoutes bool `mapstructure:"disable_java_p2p_routes"`

	// InstName is the AUTONOMY institution name created by P2pDataInit
	// (Java secretpad.inst-name, default DEFAULT_INST).
	InstName string `mapstructure:"inst_name"`

	// Tee configures the CENTER TEE node init (Java TeeResourceInit).
	Tee TeeInitConfig `mapstructure:"tee"`
}

// TeeInitConfig mirrors the TeeResourceInit properties.
type TeeInitConfig struct {
	// Enabled mirrors secretpad.tee (default true).
	Enabled *bool `mapstructure:"enabled"`
	// Address / Port / Protocol mirror KUSCIA_API_LITE_TEE_ADDRESS /
	// KUSCIA_API_PORT / KUSCIA_PROTOCOL. Port is the KusciaAPI HTTP port used
	// by the Go client (default kuscia.http_port).
	Address  string `mapstructure:"address"`
	Port     int    `mapstructure:"port"`
	Protocol string `mapstructure:"protocol"`
	// CertDir holds client.crt/client.pem/ca.crt/token (default config/certs/tee).
	CertDir string `mapstructure:"cert_dir"`
}

func boolOr(p *bool, def bool) bool {
	if p == nil {
		return def
	}
	return *p
}

// TeeEnabled reports whether TEE init is enabled (default true).
func (t TeeInitConfig) TeeEnabled() bool { return boolOr(t.Enabled, true) }

// CenterEnabled reports whether the CENTER producer runs (default true).
func (s SyncConfig) CenterEnabled() bool { return boolOr(s.Center, true) }

// P2PEnabled reports whether the AUTONOMY producer runs (default true).
func (s SyncConfig) P2PEnabled() bool { return boolOr(s.P2P, true) }

// EdgeSyncEnabled reports whether the EDGE SSE client runs (default true).
func (s SyncConfig) EdgeSyncEnabled() bool { return boolOr(s.EdgeEnabled, true) }

func durOr(d, def time.Duration) time.Duration {
	if d <= 0 {
		return def
	}
	return d
}

// EdgeReconnect returns the EDGE reconnect period (default 5s).
func (s SyncConfig) EdgeReconnect() time.Duration {
	return durOr(s.EdgeReconnectInterval, 5*time.Second)
}

// EdgeIdle returns the EDGE idle stream timeout (default 60s).
func (s SyncConfig) EdgeIdle() time.Duration { return durOr(s.EdgeIdleTimeout, 60*time.Second) }

// Dispatch returns the change dispatcher period (default 3s).
func (s SyncConfig) Dispatch() time.Duration { return durOr(s.DispatchInterval, 3*time.Second) }

// Ping returns the SSE heartbeat period (default 15s).
func (s SyncConfig) Ping() time.Duration { return durOr(s.PingInterval, 15*time.Second) }

// Timeout returns the node-to-node HTTP timeout (default 10s).
func (s SyncConfig) Timeout() time.Duration { return durOr(s.HTTPTimeout, 10*time.Second) }

// Retries returns the resend bound (default 3).
func (s SyncConfig) Retries() int {
	if s.MaxRetries <= 0 {
		return 3
	}
	return s.MaxRetries
}

// QueueDir returns the P2P queue directory (default ./config/sync/).
func (s SyncConfig) QueueDir() string {
	if s.P2PQueueDir == "" {
		return "./config/sync/"
	}
	return s.P2PQueueDir
}

// ForwardPaths returns the EDGE forward list (default Java edge.forward).
func (s SyncConfig) ForwardPaths() []string {
	if len(s.EdgeForward) == 0 {
		return DefaultEdgeForward
	}
	return s.EdgeForward
}

// IncludePaths returns the EDGE include list plus the Go extras.
func (s SyncConfig) IncludePaths() []string {
	base := s.EdgeInclude
	if len(base) == 0 {
		base = DefaultEdgeInclude
	}
	extra := s.EdgeIncludeExtra
	if extra == nil {
		extra = DefaultEdgeIncludeExtra
	}
	out := make([]string, 0, len(base)+len(extra))
	out = append(out, base...)
	return append(out, extra...)
}

// DefaultEdgeForward is Java application-edge.yaml edge.forward.
var DefaultEdgeForward = []string{
	"/api/v1alpha1/project/datatable/add",
	"/api/v1alpha1/project/datatable/delete",
	"/api/v1alpha1/approval/create",
	"/api/v1alpha1/node/update",
	"/api/v1alpha1/datatable/delete",
	"/api/v1alpha1/nodeRoute/update",
	"/api/v1alpha1/nodeRoute/delete",
	"/api/v1alpha1/project/update/tableConfig",
	"/api/v1alpha1/data/create",
	"/api/v1alpha1/feature_datasource/create",
	"/api/v1alpha1/datatable/create",
}

// DefaultEdgeInclude is Java application-edge.yaml edge.include.
var DefaultEdgeInclude = []string{
	"/api/login",
	"/api/logout",
	"/api/v1alpha1/component/i18n",
	"/api/v1alpha1/node/get",
	"/api/v1alpha1/node/list",
	"/api/v1alpha1/node/update",
	"/api/v1alpha1/node/result/list",
	"/api/v1alpha1/node/result/detail",
	"/api/v1alpha1/message/pending",
	"/api/v1alpha1/message/list",
	"/api/v1alpha1/message/detail",
	"/api/v1alpha1/message/reply",
	"/api/v1alpha1/datatable/list",
	"/api/v1alpha1/datatable/get",
	"/api/v1alpha1/datatable/pushToTee",
	"/api/v1alpha1/datatable/delete",
	"/api/v1alpha1/datasource/list",
	"/api/v1alpha1/datasource/create",
	"/api/v1alpha1/datasource/delete",
	"/api/v1alpha1/datasource/detail",
	"/api/v1alpha1/data/upload",
	"/api/v1alpha1/data/create",
	"/api/v1alpha1/data/download",
	"/api/v1alpha1/feature_datasource/create",
	"/api/v1alpha1/project/list",
	"/api/v1alpha1/project/datatable/add",
	"/api/v1alpha1/project/datatable/delete",
	"/api/v1alpha1/project/datatable/get",
	"/api/v1alpha1/nodeRoute/page",
	"/api/v1alpha1/nodeRoute/update",
	"/api/v1alpha1/nodeRoute/delete",
	"/api/v1alpha1/nodeRoute/refresh",
	"/api/v1alpha1/nodeRoute/listNode",
	"/api/v1alpha1/nodeRoute/get",
	"/api/v1alpha1/user/remote/resetPassword",
	"/api/v1alpha1/user/updatePwd",
	"/api/v1alpha1/approval/create",
	"/api/v1alpha1/user/get",
	"/api/v1alpha1/component/list",
	"/api/v1alpha1/component/batch",
	"/api/v1alpha1/version/list",
}

// DefaultEdgeIncludeExtra are Go-only /api/v1alpha1 routes the Go frontend
// needs on EDGE (login/logout/current-user aliases, env, health).
var DefaultEdgeIncludeExtra = []string{
	"/api/v1alpha1/user/login",
	"/api/v1alpha1/user/logout",
	"/api/v1alpha1/user/current",
	"/api/v1alpha1/env",
	"/api/v1alpha1/healthz",
}

// DefaultP2PDisabledRoutes is Java application-p2p.yaml unregister-mapping.path.
var DefaultP2PDisabledRoutes = []string{
	"/sync",
	"/api/v1alpha1/user/node/resetPassword",
	"/api/v1alpha1/user/remote/resetPassword",
	"/api/v1alpha1/project/inst/add",
	"/api/v1alpha1/project/node/add",
	"/api/v1alpha1/project/delete",
	"/api/v1alpha1/approval/pull/status",
	"/api/v1alpha1/vote_sync/create",
	"/api/v1alpha1/datatable/pushToTee",
	"/api/v1alpha1/project/create",
	"/api/v1alpha1/project/list",
	"/api/v1alpha1/project/update",
	"/api/v1alpha1/project/tee/list",
	"/api/v1alpha1/node/create",
	"/api/v1alpha1/node/delete",
	"/api/v1alpha1/node/newToken",
	"/api/v1alpha1/node/refresh",
	"/api/v1alpha1/node/token",
	"/api/v1alpha1/node/page",
	"/api/v1alpha1/nodeRoute/delete",
	"/api/v1alpha1/nodeRoute/listNode",
}
