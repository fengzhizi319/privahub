// Privahub is the Go reimplementation of the SecretPad backend.
// It supports three deployment modes: master (center), lite (edge), and autonomy (P2P).
package main

import (
	"context"
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
	pauth "github.com/fengzhizi319/privahub/pkg/auth"
	"github.com/fengzhizi319/privahub/pkg/config"
	pcrypto "github.com/fengzhizi319/privahub/pkg/crypto"
	"github.com/fengzhizi319/privahub/pkg/logger"
	"github.com/fengzhizi319/privahub/pkg/tlsutil"
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
		fmt.Fprintln(os.Stderr, "hint: production (no PRIVAHUB_PROFILE) needs PRIVAHUB_AUTH_JWT_SECRET, PRIVAHUB_CRYPTO_AES_KEY, "+
			"main-port TLS (server.tls.* or server.behind_tls_proxy + server.trusted_proxies) and kuscia.protocol tls|mtls; "+
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

	// At-rest envelope keyring (crypto.keys / crypto.aes_key, SM4-GCM +
	// HKDF-SM3). Validate already refused production starts without a key.
	keyring, err := cfg.Crypto.Keyring()
	if err != nil {
		log.Fatal("Invalid crypto configuration", zap.Error(err))
	}
	pcrypto.SetDefault(keyring)
	if keyring == nil {
		log.Warn("SECURITY: no crypto key configured: secrets are stored in the database unencrypted (dev/test only)")
	}

	// Password KDF work factor (auth.password_kdf_iterations); set before the
	// migrations, which wrap legacy raw password hashes with it.
	log.Info("Password storage: SM3 KDF",
		zap.Int("iterations", pauth.ConfigurePasswordKDF(cfg.Auth.PasswordKDFIterations)))

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
	// Encrypt legacy plaintext secrets / rotate to the active key (idempotent).
	if err := migration.EncryptSecretsAtRest(context.Background(), db, log); err != nil {
		log.Fatal("Failed to encrypt secrets at rest", zap.Error(err))
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
	// Default deny: refuse to start when a registered /api route is not
	// classified for RBAC and for data-resource checks.
	if err := controllerhttp.VerifyRouteCoverage(router, innerRouter); err != nil {
		log.Fatal("Route authorization coverage check failed", zap.Error(err))
	}

	// Configure HTTP servers. WriteTimeout does not cut SSE streams: the SSE
	// handlers reset their own write deadline per event (service/sse_server.go).
	srv := newHTTPServer(fmt.Sprintf(":%d", cfg.Server.HTTPPort), router, &cfg.Server)
	innerSrv := newHTTPServer(fmt.Sprintf("%s:%d", cfg.Server.InnerBindAddress, cfg.Server.InnerPort), innerRouter, &cfg.Server)

	// Main listener: plain TCP, TLS, 国密 TLCP or both (server.tls.*).
	mainLn, err := tlsutil.Listen("tcp", srv.Addr, cfg.Server.TLS)
	if err != nil {
		log.Fatal("HTTP listener failed", zap.Error(err))
	}
	if !cfg.Server.TLS.Enabled && !cfg.Server.BehindTLSProxy {
		log.Warn("SECURITY: main port serves plain HTTP (dev/test only; production requires server.tls or server.behind_tls_proxy)")
	}

	// Start HTTP server in goroutine
	go func() {
		log.Info("HTTP server listening", zap.String("addr", srv.Addr),
			zap.Bool("tls", cfg.Server.TLS.Enabled), zap.String("tls_stack", tlsStackName(cfg.Server.TLS)))
		if err := srv.Serve(mainLn); err != nil && err != http.ErrServerClosed {
			log.Fatal("HTTP server failed", zap.Error(err))
		}
	}()

	// Inner listener: mTLS when server.inner_auth.tls is configured.
	innerTLS := cfg.Server.InnerAuth.MTLSEnabled()
	if innerTLS {
		tlsCfg, err := tlsutil.MutualTLSConfig(cfg.Server.InnerAuth.TLS.ClientCAFile)
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

// newHTTPServer applies the listener hardening shared by both ports:
// ReadHeaderTimeout (slowloris), IdleTimeout and MaxHeaderBytes.
func newHTTPServer(addr string, h http.Handler, sc *config.ServerConfig) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           h,
		ReadTimeout:       sc.ReadTimeout,
		ReadHeaderTimeout: durationOr(sc.ReadHeaderTimeout, 5e9),
		WriteTimeout:      sc.WriteTimeout,
		IdleTimeout:       durationOr(sc.IdleTimeout, 120e9),
		MaxHeaderBytes:    intOr(sc.MaxHeaderBytes, 1<<20),
	}
}

func tlsStackName(t config.ServerTLSConfig) string {
	if !t.Enabled {
		return "none"
	}
	return t.NormalizedStack()
}

func durationOr[T ~int64](v T, def T) T {
	if v <= 0 {
		return def
	}
	return v
}

func intOr(v, def int) int {
	if v <= 0 {
		return def
	}
	return v
}
