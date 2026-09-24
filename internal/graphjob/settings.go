package graphjob

import (
	"os"
	"strings"
	"sync"

	"github.com/spf13/viper"
)

// DeviceConfig is one SecretFlow cluster device (Java sfclusterDesc.deviceConfig).
type DeviceConfig struct {
	Name   string `mapstructure:"name"`
	Type   string `mapstructure:"type"`
	Config string `mapstructure:"config"`
}

// Settings holds graph execution / component configuration. It is loaded from
// config/graph.yaml (override with PRIVAHUB_GRAPH_CONFIG) and replaces the
// Java application.yaml keys sfclusterDesc, job.max-parallelism,
// secretpad.component.hide, secretpad.version and secretpad.deploy-mode.
type Settings struct {
	// AppImage / SCQLImage are the Kuscia AppImage names used for tasks.
	AppImage  string `mapstructure:"app_image"`
	SCQLImage string `mapstructure:"scql_image"`
	// ServingImage is the Kuscia AppImage of SecretFlow-Serving (Java SECRETFLOW_SERVING_IMAGE).
	ServingImage string `mapstructure:"serving_image"`
	// MaxParallelism is the default when a graph has none.
	MaxParallelism int `mapstructure:"max_parallelism"`
	// HealthCheck enables the node/route readiness check before a run.
	HealthCheck bool `mapstructure:"health_check"`
	// Devices is the ordered SF cluster device list (spu, heu, ...).
	Devices []DeviceConfig `mapstructure:"devices"`
	// CrossSiloCommBackend is ray_fed_config.cross_silo_comm_backend.
	CrossSiloCommBackend string `mapstructure:"cross_silo_comm_backend"`

	// ComponentDir holds CompListDef JSON files ({app}.json).
	ComponentDir string `mapstructure:"component_dir"`
	// I18nDir holds component translation files ({app}.json).
	I18nDir string `mapstructure:"i18n_dir"`
	// LegacyComponentFile is the old flat components.json (fallback only).
	LegacyComponentFile string `mapstructure:"legacy_component_file"`
	// Hide lists "{app}/{domain}/{name}:{version}" components hidden from the UI.
	Hide []string `mapstructure:"hide"`

	// DeployMode is MPC / TEE / ALL-IN-ONE.
	DeployMode string `mapstructure:"deploy_mode"`
	// Versions maps image keys (camelCase, Java ComponentVersion fields) to versions.
	Versions map[string]string `mapstructure:"versions"`
}

const (
	defaultSPUConfig = `{"runtime_config":{"protocol":"SEMI2K","field":"FM128"},"link_desc":{"connect_retry_times":60,"connect_retry_interval_ms":1000,"brpc_channel_protocol":"http","brpc_channel_connection_type":"pooled","recv_timeout_ms":1200000,"http_timeout_ms":1200000}}`
	defaultHEUConfig = `{"mode":"PHEU","schema":"paillier","key_size":2048}`
)

// DefaultSettings returns the Java application.yaml defaults.
func DefaultSettings() *Settings {
	return &Settings{
		AppImage:             "secretflow-image",
		SCQLImage:            "scql-image",
		ServingImage:         "sf-serving-image",
		MaxParallelism:       1,
		HealthCheck:          true,
		Devices:              []DeviceConfig{{Name: "spu", Type: "spu", Config: defaultSPUConfig}, {Name: "heu", Type: "heu", Config: defaultHEUConfig}},
		CrossSiloCommBackend: "brpc_link",
		ComponentDir:         "config/components",
		I18nDir:              "config/i18n",
		LegacyComponentFile:  "config/components.json",
		Hide: []string{
			"secretflow/io/read_data:1.0.0",
			"secretflow/io/write_data:1.0.0",
			"secretflow/io/identity:1.0.0",
			"secretflow/io/data_sink:1.0.0",
			"secretflow/io/data_source:1.0.0",
			"secretflow/model/model_export:1.0.0",
			"secretflow/ml.train/slnn_train:0.0.1",
			"secretflow/ml.predict/slnn_predict:0.0.2",
		},
		DeployMode: "ALL-IN-ONE",
		Versions: map[string]string{
			"secretpadImage":         "0.5.0b0",
			"kusciaImage":            "0.6.0b0",
			"secretflowImage":        "1.4.0b0",
			"secretflowServingImage": "0.2.0b0",
			"teeAppImage":            "0.1.0b0",
			"teeDmImage":             "0.1.0b0",
			"capsuleManagerSimImage": "0.1.2b0",
			"dataProxyImage":         "0.1.0b0",
			"scqlImage":              "0.1.0b0",
		},
	}
}

var (
	settingsOnce sync.Once
	settings     *Settings
	settingsMu   sync.RWMutex
)

// LoadSettings loads settings from path (yaml); missing keys keep defaults.
func LoadSettings(path string) *Settings {
	s := DefaultSettings()
	if path == "" {
		return s
	}
	if _, err := os.Stat(path); err != nil {
		return s
	}
	v := viper.New()
	v.SetConfigFile(path)
	if err := v.ReadInConfig(); err != nil {
		return s
	}
	sub := v
	if v.IsSet("graph") {
		sub = v.Sub("graph")
	}
	defaults := s.Versions
	s.Versions = nil
	if err := sub.Unmarshal(s); err != nil {
		return DefaultSettings()
	}
	// viper lower-cases map keys: restore the canonical camelCase names.
	merged := make(map[string]string, len(defaults))
	for k, v := range defaults {
		merged[k] = v
	}
	for k, v := range s.Versions {
		canonical := k
		for dk := range defaults {
			if strings.EqualFold(dk, k) {
				canonical = dk
			}
		}
		merged[canonical] = v
	}
	s.Versions = merged
	if len(s.Devices) == 0 {
		s.Devices = DefaultSettings().Devices
	}
	if s.MaxParallelism <= 0 {
		s.MaxParallelism = 1
	}
	return s
}

// GetSettings returns the process-wide settings (lazy loaded).
func GetSettings() *Settings {
	settingsOnce.Do(func() {
		path := os.Getenv("PRIVAHUB_GRAPH_CONFIG")
		if path == "" {
			path = "config/graph.yaml"
		}
		loaded := LoadSettings(path)
		settingsMu.Lock()
		if settings == nil {
			settings = loaded
		}
		settingsMu.Unlock()
	})
	settingsMu.RLock()
	defer settingsMu.RUnlock()
	return settings
}

// SetSettings overrides the process-wide settings (tests / wiring).
func SetSettings(s *Settings) {
	settingsOnce.Do(func() {})
	settingsMu.Lock()
	settings = s
	settingsMu.Unlock()
}

// VersionsForDeployMode mirrors ComponentServiceImpl.listComponentVersion.
func (s *Settings) VersionsForDeployMode() map[string]string {
	keys := map[string][]string{
		"MPC":        {"secretpadImage", "secretflowImage", "secretflowServingImage", "kusciaImage", "dataProxyImage", "scqlImage"},
		"TEE":        {"teeDmImage", "teeAppImage", "capsuleManagerSimImage"},
		"ALL-IN-ONE": {"teeDmImage", "teeAppImage", "capsuleManagerSimImage", "secretpadImage", "secretflowServingImage", "kusciaImage", "secretflowImage", "dataProxyImage", "scqlImage"},
	}
	mode := strings.ToUpper(strings.ReplaceAll(s.DeployMode, "_", "-"))
	ks, ok := keys[mode]
	if !ok {
		return nil
	}
	out := make(map[string]string, len(ks))
	for _, k := range ks {
		// viper lower-cases map keys; accept both spellings.
		if v, ok := s.Versions[k]; ok {
			out[k] = v
		} else if v, ok := s.Versions[strings.ToLower(k)]; ok {
			out[k] = v
		}
	}
	return out
}

// IsHidden reports whether "{app}/{domain}/{name}:{version}" (or "{app}/{key}") is hidden.
func (s *Settings) IsHidden(key string) bool {
	for _, h := range s.Hide {
		if h == key {
			return true
		}
	}
	return false
}
