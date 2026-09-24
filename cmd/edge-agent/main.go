// privahub-edge-agent is a thin standalone EDGE data-sync daemon: it runs the
// same center->edge SSE client as the main binary (internal/service/sync,
// Java EdgeSyncTask + EdgeDataSyncServiceImpl) against the configured
// database, without the HTTP API. The main binary already runs this client
// when server.mode is EDGE/lite; use the agent only when the API server is
// not running on the edge.
package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/fengzhizi319/privahub/pkg/innerauth"
	"os"
	"os/signal"
	"syscall"

	"go.uber.org/zap"

	"github.com/fengzhizi319/privahub/internal/bootstrap"
	"github.com/fengzhizi319/privahub/internal/dao"
	migration "github.com/fengzhizi319/privahub/internal/dao/migrations"
	"github.com/fengzhizi319/privahub/internal/service"
	datasync "github.com/fengzhizi319/privahub/internal/service/sync"
	"github.com/fengzhizi319/privahub/internal/wire"
	"github.com/fengzhizi319/privahub/pkg/config"
	"github.com/fengzhizi319/privahub/pkg/logger"
)

func main() {
	cfgFile := flag.String("config", "", "path to config file")
	flag.Parse()

	cfg, err := config.Load(*cfgFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		os.Exit(1)
	}
	log, err := logger.InitLogger(cfg.Observability.LogLevel, cfg.Observability.LogFormat)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to init logger: %v\n", err)
		os.Exit(1)
	}
	defer log.Sync()
	// Calls to the center inner port carry server.inner_auth.token.
	if token, err := cfg.Server.InnerAuth.ResolvedToken(); err == nil {
		innerauth.SetOutgoingToken(token)
	}

	db, err := dao.NewDB(&cfg.Database, log)
	if err != nil {
		log.Fatal("failed to initialize database", zap.Error(err))
	}
	if err := migration.AutoMigrate(db, log); err != nil {
		log.Fatal("failed to migrate database", zap.Error(err))
	}

	env := service.NewEnvService(string(service.PlatformEdge), wire.PlatformNodeID(cfg), "", log)
	service.SetGlobalEnv(env)

	kusciaClient, pool := wire.NewKusciaClients(cfg, log)
	deps := bootstrap.Deps{DB: db, Env: env, AdminName: cfg.Auth.AdminName, KusciaProtocol: cfg.Kuscia.Protocol, Log: log}
	if kusciaClient != nil {
		deps.Kuscia = kusciaClient
	}
	if pool != nil {
		deps.Pool = pool
	}
	_ = bootstrap.New(deps).Run(context.Background())

	mgr, err := datasync.NewManager(datasync.Options{
		DB: db, Cfg: cfg.Sync, Env: env, Gateway: cfg.Kuscia.Gateway,
		CenterHost: cfg.Server.CenterPlatformService, Log: log,
	})
	if err != nil {
		log.Fatal("failed to init data sync", zap.Error(err))
	}
	if mgr.Edge == nil {
		log.Fatal("edge data sync disabled (sync.edge_enabled=false)")
	}
	mgr.Start()
	log.Info("privahub-edge-agent running", zap.String("node_id", env.GetPlatformNodeId()), zap.String("gateway", cfg.Kuscia.Gateway))

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	mgr.Stop()
	if sqlDB, err := db.DB(); err == nil {
		_ = sqlDB.Close()
	}
}
