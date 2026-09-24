package config

import (
	"fmt"
	"time"

	"github.com/spf13/viper"
)

// MinAuditRetentionDays is the minimum audit.retention_days accepted in
// production (3 years); 0 keeps audit records forever.
const MinAuditRetentionDays = 1095

// AuditConfig configures the tamper-evident security audit trail
// (internal/audit).
type AuditConfig struct {
	// QueueSize is the in-memory event queue capacity.
	QueueSize int `mapstructure:"queue_size"`
	// SpoolDir holds the fsync'd local spool used when the DB write fails
	// (replayed on startup / periodically).
	SpoolDir string `mapstructure:"spool_dir"`
	// SpoolMaxBytes caps the spool; events beyond are dropped (metric).
	SpoolMaxBytes int64 `mapstructure:"spool_max_bytes"`
	// RetentionDays: 0 = keep forever; otherwise >= 1095 in production.
	// Expired records are exported to ArchiveDir as hash-chained segments
	// before deletion.
	RetentionDays int    `mapstructure:"retention_days"`
	ArchiveDir    string `mapstructure:"archive_dir"`
	// RetentionInterval is how often archival runs.
	RetentionInterval time.Duration `mapstructure:"retention_interval"`
	// Anchor periodically writes the chain tail hash outside the database.
	Anchor AuditAnchorConfig `mapstructure:"anchor"`
}

// AuditAnchorConfig configures external anchoring of the audit chain tail.
type AuditAnchorConfig struct {
	// File is an append-only JSON-lines file (ideally on WORM / another host).
	File string `mapstructure:"file"`
	// URL receives a POST {time,seq,hash,node,algorithm}.
	URL string `mapstructure:"url"`
	// Token is sent as "Authorization: Bearer <token>" to URL (use the
	// PRIVAHUB_AUDIT_ANCHOR_TOKEN env var).
	Token    string        `mapstructure:"token"`
	Interval time.Duration `mapstructure:"interval"`
}

// Enabled reports whether any external anchor is configured.
func (a AuditAnchorConfig) Enabled() bool { return a.File != "" || a.URL != "" }

func setAuditDefaults(v *viper.Viper) {
	v.SetDefault("audit.queue_size", 4096)
	v.SetDefault("audit.spool_dir", "./data/audit/spool")
	v.SetDefault("audit.spool_max_bytes", 256<<20)
	v.SetDefault("audit.retention_days", 0)
	v.SetDefault("audit.archive_dir", "./data/audit/archive")
	v.SetDefault("audit.retention_interval", "24h")
	v.SetDefault("audit.anchor.file", "")
	v.SetDefault("audit.anchor.url", "")
	v.SetDefault("audit.anchor.token", "")
	v.SetDefault("audit.anchor.interval", "1h")
}

// validateAudit enforces audit retention rules (production: 0 or >= 1095).
func (c *Config) validateAudit() error {
	a := c.Audit
	if a.RetentionDays < 0 {
		return fmt.Errorf("audit.retention_days must be >= 0 (0 = keep forever)")
	}
	if a.RetentionDays > 0 && a.RetentionDays < MinAuditRetentionDays && !c.IsDev() {
		return fmt.Errorf("audit.retention_days=%d is below the %d-day minimum required in production (use 0 to keep forever)",
			a.RetentionDays, MinAuditRetentionDays)
	}
	if a.RetentionDays > 0 && a.ArchiveDir == "" {
		return fmt.Errorf("audit.archive_dir is required when audit.retention_days is set")
	}
	return nil
}

// AuditWarnings returns startup warnings for the audit configuration.
func (c *Config) AuditWarnings() []string {
	if c.IsDev() || c.Audit.Anchor.Enabled() {
		return nil
	}
	return []string{"audit.anchor.file / audit.anchor.url not configured: the audit hash chain is only self-consistent; " +
		"an attacker with database write access can rewrite it undetected. Configure external anchoring in production."}
}
