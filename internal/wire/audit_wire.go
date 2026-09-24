package wire

import (
	"context"
	"os"
	"time"

	"github.com/fengzhizi319/privahub/internal/audit"
	v1 "github.com/fengzhizi319/privahub/internal/controller/http/v1"
	"github.com/fengzhizi319/privahub/pkg/config"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// AuditOptionsFromConfig maps config.AuditConfig to audit.Options.
func AuditOptionsFromConfig(cfg *config.Config) audit.Options {
	a := cfg.Audit
	host, _ := os.Hostname()
	return audit.Options{
		QueueSize:         a.QueueSize,
		SpoolDir:          a.SpoolDir,
		SpoolMaxBytes:     a.SpoolMaxBytes,
		RetentionDays:     a.RetentionDays,
		ArchiveDir:        a.ArchiveDir,
		RetentionInterval: a.RetentionInterval,
		AnchorFile:        a.Anchor.File,
		AnchorURL:         a.Anchor.URL,
		AnchorToken:       a.Anchor.Token,
		AnchorInterval:    a.Anchor.Interval,
		NodeID:            host,
	}
}

// wireAudit starts the audit trail and installs it as the global recorder
// (audit.Record). A failure is logged and auditing is disabled rather than
// preventing startup.
func wireAudit(db *gorm.DB, cfg *config.Config, log *zap.Logger) (*audit.Service, *v1.AuditHandler) {
	for _, w := range cfg.AuditWarnings() {
		log.Warn("SECURITY: " + w)
	}
	svc, err := audit.Init(db, log.Named("audit"), AuditOptionsFromConfig(cfg))
	if err != nil {
		log.Error("audit trail initialization failed; audit events will NOT be recorded", zap.Error(err))
		return nil, v1.NewAuditHandler(nil, db, cfg.Audit.Anchor.File, cfg.Audit.ArchiveDir)
	}
	return svc, v1.NewAuditHandler(svc, db, cfg.Audit.Anchor.File, cfg.Audit.ArchiveDir)
}

// closeAudit drains the audit queue on shutdown.
func closeAudit(svc *audit.Service) {
	if svc == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = svc.Close(ctx)
}
