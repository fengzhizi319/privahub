// Package dao provides database initialization and connection management
// for the Privahub platform.
//
// Supported drivers:
//   - sqlite: Pure-Go driver (modernc.org/sqlite), zero CGO required.
//     Ideal for development and single-node deployments.
//     DSN format: "privahub.db?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
//   - mysql: Production-grade driver for multi-instance deployments.
//     DSN format: "user:pass@tcp(host:3306)/dbname?charset=utf8mb4&parseTime=True"
//
// Connection pooling is configured via DatabaseConfig.MaxOpenConns and
// MaxIdleConns. For SQLite, MaxOpenConns should typically be 1 to avoid
// SQLITE_BUSY errors under concurrent writes.
package dao

import (
	"fmt"

	"github.com/fengzhizi319/privahub/pkg/config"
	"go.uber.org/zap"
	"gorm.io/driver/mysql"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// NewDB creates a database connection based on configuration.
// It initializes the GORM engine with the appropriate dialector,
// configures connection pooling, and sets the logger to silent mode
// (application-level logging is handled by the service layer).
//
// Returns an error if the driver is unsupported or the connection fails.
func NewDB(cfg *config.DatabaseConfig, logger *zap.Logger) (*gorm.DB, error) {
	var dialector gorm.Dialector

	switch cfg.Driver {
	case "sqlite":
		dialector = sqlite.Open(cfg.DSN)
	case "mysql":
		dialector = mysql.Open(cfg.DSN)
	default:
		return nil, fmt.Errorf("unsupported database driver: %s", cfg.Driver)
	}

	db, err := gorm.Open(dialector, &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect database: %w", err)
	}

	// Configure connection pool
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get underlying sql.DB: %w", err)
	}

	if cfg.MaxOpenConns > 0 {
		sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	}
	if cfg.MaxIdleConns > 0 {
		sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	}

	logger.Info("Database connected successfully",
		zap.String("driver", cfg.Driver),
		zap.Int("max_open_conns", cfg.MaxOpenConns),
		zap.Int("max_idle_conns", cfg.MaxIdleConns),
	)

	return db, nil
}
