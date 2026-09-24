package wire

import (
	"context"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/fengzhizi319/privahub/internal/bootstrap"
	"github.com/fengzhizi319/privahub/internal/controller/http/middleware"
	v1 "github.com/fengzhizi319/privahub/internal/controller/http/v1"
	"github.com/fengzhizi319/privahub/internal/service"
	datasync "github.com/fengzhizi319/privahub/internal/service/sync"
	"github.com/fengzhizi319/privahub/pkg/config"
	"github.com/fengzhizi319/privahub/pkg/kuscia"
)

// SyncWiring holds the data-sync components, the platform middlewares and
// the startup initializer.
type SyncWiring struct {
	// Manager owns producer/dispatcher/edge client/P2P queues (may be nil if
	// its construction failed).
	Manager *datasync.Manager
	// Handler serves vote_sync/create and data/sync on the inner port.
	Handler *v1.SyncHandler
	// Sender is the service.DbSyncSender (also installed globally).
	Sender service.DbSyncSender
	// MainMiddlewares run on the main port right after BlockInnerPaths:
	// EdgeForward on EDGE and the disabled-route filter.
	MainMiddlewares []gin.HandlerFunc
	// InnerMiddlewares run on the inner port (disabled-route filter).
	InnerMiddlewares []gin.HandlerFunc
	// ForwardTargets are the edge-forwarded paths a CENTER serves on its
	// inner port for node callers (see controllerhttp.MountEdgeForwardTargets).
	ForwardTargets []string
	// BootstrapDone is closed when the startup initializers finished.
	BootstrapDone <-chan struct{}
}

// disabledRoutes returns the configured disabled routes (Java
// DynamicBeanRegisterInit / unregister-mapping.path).
func disabledRoutes(cfg *config.Config, env *service.EnvService) []string {
	out := append([]string(nil), cfg.Sync.DisabledRoutes...)
	if cfg.Sync.DisableJavaP2PRoutes && env.IsAutonomy() {
		out = append(out, config.DefaultP2PDisabledRoutes...)
	}
	return out
}

// wireSync builds the data-sync manager, the DbSyncSender, the platform
// middlewares and runs the startup initializers.
func wireSync(db *gorm.DB, cfg *config.Config, log *zap.Logger, env *service.EnvService,
	sse *service.SseServer, auth *service.AuthService, kusciaClient *kuscia.Client,
	pool *kuscia.ClientPool, dataProxy *service.DataProxyService) *SyncWiring {
	if log == nil {
		log = zap.NewNop()
	}
	w := &SyncWiring{}

	// Startup initializers first: P2pDataInit publishes the inst id used by
	// the P2P producer and the DbSyncSender routing.
	deps := bootstrap.Deps{
		DB: db, Env: env, AdminName: cfg.Auth.AdminName, InstName: cfg.Sync.InstName,
		KusciaProtocol: cfg.Kuscia.Protocol, Log: log.Named("bootstrap"),
		Tee: bootstrap.TeeOptions{
			Enabled: cfg.Sync.Tee.TeeEnabled(), Address: cfg.Sync.Tee.Address,
			Port:     firstPositiveInt(cfg.Sync.Tee.Port, cfg.Kuscia.HTTPPort, 8082),
			Protocol: wireFirstNonEmpty(cfg.Sync.Tee.Protocol, cfg.Kuscia.Protocol), CertDir: cfg.Sync.Tee.CertDir,
		},
	}
	if kusciaClient != nil {
		deps.Kuscia = kusciaClient
	}
	if pool != nil {
		deps.Pool = pool
	}
	if dataProxy != nil {
		deps.DataProxy = dataProxy
	}
	w.BootstrapDone = bootstrap.New(deps).Start(context.Background())

	mgr, err := datasync.NewManager(datasync.Options{
		DB: db, Cfg: cfg.Sync, Env: env, Sse: sse,
		Gateway: cfg.Kuscia.Gateway, CenterHost: cfg.Server.CenterPlatformService, Log: log.Named("datasync"),
	})
	if err != nil {
		log.Error("data sync init failed; sync disabled", zap.Error(err))
	} else {
		w.Manager = mgr
		w.Sender = mgr.Sender
		service.SetDbSyncSender(mgr.Sender)
		w.Handler = v1.NewSyncHandler(mgr.VoteSync, mgr.Consumer, log)
		mgr.Start()
	}
	if w.Handler == nil {
		reg := datasync.DefaultRegistry()
		tables, _ := reg.ResolveTables(datasync.DefaultTables(env.GetPlatformType()))
		w.Handler = v1.NewSyncHandler(datasync.NewVoteSyncService(db, reg, log),
			datasync.NewConsumer(db, reg, tables, env.GetPlatformNodeId(), log).
				SetLocalIdentity(datasync.NewEnvIdentity(db, env), env.IsAutonomy()), log)
	}

	if env.IsEdge() {
		fwd := middleware.EdgeForwardConfig{
			Forward: cfg.Sync.ForwardPaths(), Include: cfg.Sync.IncludePaths(),
			Gateway: cfg.Kuscia.Gateway, CenterHost: cfg.Server.CenterPlatformService,
			NodeID: env.GetPlatformNodeId(), Log: log.Named("edge-forward"),
		}
		if auth != nil {
			fwd.Sessions = auth
		}
		w.MainMiddlewares = append(w.MainMiddlewares, middleware.EdgeForward(fwd))
	}
	if env.IsCenter() {
		w.ForwardTargets = cfg.Sync.ForwardPaths()
	}
	if dis := disabledRoutes(cfg, env); len(dis) > 0 {
		log.Info("routes disabled by config (unregister-mapping)", zap.Strings("paths", dis))
		w.MainMiddlewares = append(w.MainMiddlewares, middleware.BlockInnerPaths(dis))
		w.InnerMiddlewares = append(w.InnerMiddlewares, middleware.BlockInnerPaths(dis))
	}
	return w
}

func firstPositiveInt(vs ...int) int {
	for _, v := range vs {
		if v > 0 {
			return v
		}
	}
	return 0
}

// Stop stops the sync loops.
func (w *SyncWiring) Stop() {
	if w != nil && w.Manager != nil {
		w.Manager.Stop()
	}
}
