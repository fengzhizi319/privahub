// Privahub is the Go reimplementation of the SecretPad backend.
// It supports three deployment modes: master (center), lite (edge), and autonomy (P2P).
package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	controllerhttp "github.com/fengzhizi319/privahub/internal/controller/http"
	"github.com/fengzhizi319/privahub/internal/dao"
	migration "github.com/fengzhizi319/privahub/internal/dao/migrations"
	"github.com/fengzhizi319/privahub/internal/wire"
	"github.com/fengzhizi319/privahub/pkg/config"
	"github.com/fengzhizi319/privahub/pkg/logger"
	"go.uber.org/zap"
)

var (
	version   = "dev"
	gitCommit = "unknown"
	buildTime = "unknown"
)

func main() {
	// Parse CLI flags
	cfgFile := flag.String("config", "", "path to config file")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Printf("privahub %s (commit: %s, built: %s)\n", version, gitCommit, buildTime)
		os.Exit(0)
	}

	// Load configuration (Validate refuses production starts with sample
	// secrets, auth disabled or an unprotected non-loopback inner port).
	cfg, err := config.Load(*cfgFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		fmt.Fprintln(os.Stderr, "hint: production (no PRIVAHUB_PROFILE) needs PRIVAHUB_AUTH_JWT_SECRET and PRIVAHUB_CRYPTO_AES_KEY; "+
			"local development: PRIVAHUB_PROFILE=dev")
		os.Exit(1)
	}

	// Initialize logger
	log, err := logger.InitLogger(cfg.Observability.LogLevel, cfg.Observability.LogFormat)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to init logger: %v\n", err)
		os.Exit(1)
	}
	defer log.Sync()

	log.Info("Starting Privahub",
		zap.String("version", version),
		zap.String("mode", cfg.Server.Mode),
		zap.String("profile", cfg.Profile),
		zap.Int("http_port", cfg.Server.HTTPPort),
		zap.Int("grpc_port", cfg.Server.GRPCPort),
	)

	// Initialize database
	db, err := dao.NewDB(&cfg.Database, log)
	if err != nil {
		log.Fatal("Failed to initialize database", zap.Error(err))
	}
	log.Info("Database initialized", zap.String("driver", cfg.Database.Driver))

	// Run database migration and seed data
	if err := migration.AutoMigrate(db, log); err != nil {
		log.Fatal("Failed to run database migration", zap.Error(err))
	}
	for _, w := range cfg.SecurityWarnings() {
		log.Warn("SECURITY: " + w)
	}
	if err := migration.SeedDataWithOptions(context.Background(), db, log, wire.SeedOptionsFromConfig(cfg)); err != nil {
		log.Fatal("Failed to seed initial data", zap.Error(err))
	}

	// Initialize application dependencies
	app := wire.NewApp(db, cfg, log)
	log.Info("Application dependencies initialized")

	// Create HTTP router
	router := controllerhttp.NewRouter(log, app)

	// Create inner port router (cluster-internal, no auth)
	innerRouter := controllerhttp.NewInnerRouter(log, app)
	// CENTER: serve the EDGE-forwarded write endpoints on the inner port.
	controllerhttp.MountEdgeForwardTargets(innerRouter, router, app)

	// Configure HTTP server
	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Server.HTTPPort),
		Handler:      router,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
	}

	// Configure inner HTTP server
	innerSrv := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", cfg.Server.InnerBindAddress, cfg.Server.InnerPort),
		Handler:      innerRouter,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
	}

	// Start HTTP server in goroutine
	go func() {
		log.Info("HTTP server listening", zap.String("addr", srv.Addr))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal("HTTP server failed", zap.Error(err))
		}
	}()

	// Inner listener: mTLS when server.inner_auth.tls is configured.
	innerTLS := cfg.Server.InnerAuth.MTLSEnabled()
	if innerTLS {
		tlsCfg, err := innerMTLSConfig(cfg.Server.InnerAuth.TLS.ClientCAFile)
		if err != nil {
			log.Fatal("Inner port mTLS configuration failed", zap.Error(err))
		}
		innerSrv.TLSConfig = tlsCfg
	}
	if !config.IsLoopbackBind(cfg.Server.InnerBindAddress) {
		log.Info("Inner port exposed beyond loopback; protected by server.inner_auth",
			zap.Bool("token", cfg.Server.InnerAuth.Token != "" || cfg.Server.InnerAuth.TokenFile != ""),
			zap.Int("allowed_cidrs", len(cfg.Server.InnerAuth.AllowedCIDRs)), zap.Bool("mtls", innerTLS))
	}

	// Start inner HTTP server in goroutine
	go func() {
		log.Info("Inner HTTP server listening", zap.String("addr", innerSrv.Addr), zap.Bool("mtls", innerTLS))
		var err error
		if innerTLS {
			err = innerSrv.ListenAndServeTLS(cfg.Server.InnerAuth.TLS.CertFile, cfg.Server.InnerAuth.TLS.KeyFile)
		} else {
			err = innerSrv.ListenAndServe()
		}
		if err != nil && err != http.ErrServerClosed {
			log.Fatal("Inner HTTP server failed", zap.Error(err))
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("Shutting down server...")
	ctx, cancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout)
	defer cancel()

	// Stop background services (cron engine, job/serving sync loops)
	app.Shutdown()

	if err := srv.Shutdown(ctx); err != nil {
		log.Error("Server forced to shutdown", zap.Error(err))
	}
	if err := innerSrv.Shutdown(ctx); err != nil {
		log.Error("Inner server forced to shutdown", zap.Error(err))
	}

	// Close database connection
	if sqlDB, err := db.DB(); err == nil {
		sqlDB.Close()
	}

	log.Info("Server exited gracefully")
}

// innerMTLSConfig requires and verifies client certificates signed by caFile.
func innerMTLSConfig(caFile string) (*tls.Config, error) {
	pem, err := os.ReadFile(caFile)
	if err != nil {
		return nil, fmt.Errorf("read client CA: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pem) {
		return nil, fmt.Errorf("no certificates in client CA %s", caFile)
	}
	return &tls.Config{ClientCAs: pool, ClientAuth: tls.RequireAndVerifyClientCert, MinVersion: tls.VersionTLS12}, nil
}
