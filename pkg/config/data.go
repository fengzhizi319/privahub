package config

// DataPlatformConfig groups data-management settings that mirror Java
// SecretPad properties:
//
//	secretpad.data.dir-path      -> data.dir
//	(upload size: server.upload_max_size / server.upload_paths, config.go)
//	ip.block.{enable,list}       -> data.ip_block
//	tee.domain-id                -> data.tee.domain_id
//	tee.capsule-manager          -> data.tee.capsule_manager
//	job.max-parallelism          -> data.tee.max_parallelism
//	secretpad.cloud-log.sls.*    -> data.cloud_log.sls
type DataPlatformConfig struct {
	// Dir is the root directory where uploaded CSV files are stored
	// (`${dir}/<nodeId>/`). Falls back to env PRIVAHUB_DATA_DIR, then ./data.
	Dir string `mapstructure:"dir"`
	// IPBlock is the SSRF blacklist applied to user supplied endpoints/URLs.
	IPBlock IPBlockConfig `mapstructure:"ip_block"`
	// Tee holds TEE data-management job settings.
	Tee TeeConfig `mapstructure:"tee"`
	// CloudLog holds the optional SLS cloud log settings.
	CloudLog CloudLogConfig `mapstructure:"cloud_log"`
}

// IPBlockConfig mirrors Java IpBlockConfig (ip.block.enable / ip.block.list).
// Enable is a pointer so an absent setting defaults to enabled (Java default).
type IPBlockConfig struct {
	Enable *bool    `mapstructure:"enable"`
	List   []string `mapstructure:"list"`
}

// DefaultIPBlockList is the Java default ip.block.list (application.yaml).
var DefaultIPBlockList = []string{
	"0.0.0.0/32",
	"127.0.0.1/8",
	"10.0.0.0/8",
	"11.0.0.0/8",
	"30.0.0.0/8",
	"100.64.0.0/10",
	"172.16.0.0/12",
	"192.168.0.0/16",
	"33.0.0.0/8",
}

// Enabled reports whether IP blocking is enabled (default true).
func (c IPBlockConfig) Enabled() bool {
	return c.Enable == nil || *c.Enable
}

// Ranges returns the configured CIDR list, or the Java defaults.
func (c IPBlockConfig) Ranges() []string {
	if len(c.List) == 0 {
		return DefaultIPBlockList
	}
	return c.List
}

// TeeConfig mirrors Java tee.* / job.max-parallelism settings.
type TeeConfig struct {
	DomainID       string `mapstructure:"domain_id"`       // default "tee"
	CapsuleManager string `mapstructure:"capsule_manager"` // default "capsule-manager.#.svc"
	MaxParallelism int32  `mapstructure:"max_parallelism"` // default 1
	// CertDir holds client.pem / client.crt / ca.crt used to sign TEE graph
	// tasks (Java ./config/certs). Default "./config/certs".
	CertDir string `mapstructure:"cert_dir"`
}

// CloudLogConfig holds cloud log provider settings.
type CloudLogConfig struct {
	SLS SLSConfig `mapstructure:"sls"`
}

// SLSConfig mirrors Java LogConfigProperties.SLSConfig. All fields must be set
// for the SLS service to be enabled.
type SLSConfig struct {
	AK      string `mapstructure:"ak"`
	SK      string `mapstructure:"sk"`
	Host    string `mapstructure:"host"`
	Project string `mapstructure:"project"`
}

// Configured reports whether every SLS field is set.
func (c SLSConfig) Configured() bool {
	return c.AK != "" && c.SK != "" && c.Host != "" && c.Project != ""
}
