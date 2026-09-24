package wire

import (
	"time"

	"github.com/fengzhizi319/privahub/internal/controller/http/middleware"
	v1 "github.com/fengzhizi319/privahub/internal/controller/http/v1"
	migration "github.com/fengzhizi319/privahub/internal/dao/migrations"
	"github.com/fengzhizi319/privahub/internal/service"
	"github.com/fengzhizi319/privahub/pkg/config"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// PlatformNodeID returns the configured platform node id (server.node_id,
// falling back to kuscia.namespace, then "kuscia-system").
func PlatformNodeID(cfg *config.Config) string {
	if cfg.Server.NodeID != "" {
		return cfg.Server.NodeID
	}
	if cfg.Kuscia.Namespace != "" {
		return cfg.Kuscia.Namespace
	}
	return "kuscia-system"
}

// NewEnvServiceFromConfig builds the EnvService with a normalized platform
// type (CENTER/EDGE/AUTONOMY/TEST), node id, deploy mode and — on CENTER —
// the embedded nodes alice/bob/tee (Java EnvServiceImpl.EMBEDDED_NODE).
func NewEnvServiceFromConfig(cfg *config.Config, log *zap.Logger) *service.EnvService {
	env := service.NewEnvService(cfg.Server.Mode, PlatformNodeID(cfg), "", log)
	env.SetDeployMode(cfg.Server.DeployMode)
	if env.IsCenter() {
		env.SetEmbeddedNodes(service.DefaultEmbeddedNodes)
	}
	service.SetGlobalEnv(env)
	return env
}

// SeedOptionsFromConfig derives the seed options (initial admin etc.).
func SeedOptionsFromConfig(cfg *config.Config) migration.SeedOptions {
	return migration.SeedOptions{
		Dev:                 cfg.IsDev(),
		AdminName:           cfg.Auth.AdminName,
		AdminPassword:       cfg.Auth.AdminPassword,
		InitialPasswordFile: cfg.Auth.InitialPasswordFile,
		PlatformType:        string(service.NormalizePlatformType(cfg.Server.Mode)),
		NodeID:              PlatformNodeID(cfg),
	}
}

// wireAuth configures the auth/user services from config and installs the
// global DataResource checker.
func wireAuth(
	db *gorm.DB,
	cfg *config.Config,
	env *service.EnvService,
	authService *service.AuthService,
	userService *service.UserService,
	nodeUserService *service.NodeUserService,
	userHandler *v1.UserHandler,
) *service.DataResourceAuth {
	authService.SetDB(db)
	authService.SetEnv(env)
	authService.SetOptions(service.AuthOptions{
		MaxAttempts:            cfg.Auth.MaxAttempts,
		LockDuration:           time.Duration(cfg.Auth.LockMinutes) * time.Minute,
		SessionIdleTimeout:     cfg.Auth.SessionIdleTimeout,
		AllowPlaintextPassword: cfg.IsDev(),
		DeployMode:             cfg.Server.DeployMode,
	})
	userService.SetResetPasswordPolicy(cfg.Auth.ResetPasswordMaxAttempts,
		time.Duration(cfg.Auth.ResetPasswordLockMinutes)*time.Minute)
	nodeUserService.SetRemote(cfg.Kuscia.Gateway, cfg.Server.CenterPlatformService)
	nodeUserService.SetLocalCenter(env.GetPlatformType() == service.PlatformCenter)
	userHandler.SetNodeUserService(nodeUserService)

	dataAuth := service.NewDataResourceAuth(db, env)
	middleware.SetDataResourceAuth(dataAuth)
	return dataAuth
}

// AuthMiddleware returns the session-backed authentication middleware for
// main-port API routes.
func (a *App) AuthMiddleware() gin.HandlerFunc {
	return middleware.SessionAuth(a.JWTManager, a.AuthService, a.Config.Auth.AuthEnabled())
}

// InnerAuthMiddleware returns the inner-port node authentication middleware.
func (a *App) InnerAuthMiddleware() gin.HandlerFunc {
	return middleware.InnerAuth(a.AuthService)
}

// RBACMiddleware returns the interface-permission middleware.
func (a *App) RBACMiddleware() gin.HandlerFunc {
	return middleware.RBAC(a.DB, a.EnvService, middleware.RBACConfig{Enabled: true})
}

// InnerGuardMiddleware enforces server.inner_auth (source allowlist and
// shared token) on inner-port routes.
func (a *App) InnerGuardMiddleware() gin.HandlerFunc {
	token, _ := a.Config.Server.InnerAuth.ResolvedToken() // validated at load
	return middleware.InnerGuard(a.Config.Server.InnerAuth.AllowedCIDRs, token)
}

// MetricsHandlers returns the middleware chain for GET /metrics according to
// observability.metrics_access, or nil when the endpoint is disabled.
func (a *App) MetricsHandlers(h gin.HandlerFunc) []gin.HandlerFunc {
	o := a.Config.Observability
	switch o.MetricsAccess {
	case config.MetricsDisabled:
		return nil
	case config.MetricsPublic:
		return []gin.HandlerFunc{h}
	case config.MetricsAuth:
		return []gin.HandlerFunc{a.AuthMiddleware(), h}
	default: // restricted (production default)
		return []gin.HandlerFunc{middleware.MetricsGuard(o.MetricsAllowedCIDRs, o.MetricsToken), h}
	}
}

// DataResourceMiddleware enforces the Java @DataResource checks
// (middleware.DataResourceRoutes) on authenticated routes.
func (a *App) DataResourceMiddleware() gin.HandlerFunc {
	white := a.Config.Auth.DataResourceWhitelist
	if len(white) == 0 {
		white = middleware.DefaultAutonomyWhitelist
	}
	return middleware.DataResourceGuard(white)
}

func newUserLimiter(cfg *config.Config) *middleware.KeyedLimiter {
	rl := cfg.Server.RateLimit
	return middleware.NewKeyedLimiter(rl.UserPermits, rl.UserWindow, rl.UserBurst)
}

func newLoginLimiter(cfg *config.Config) *middleware.KeyedLimiter {
	rl := cfg.Server.RateLimit
	return middleware.NewKeyedLimiter(rl.LoginPerMinute, time.Minute, rl.LoginBurst)
}

// UserRateLimitMiddleware throttles the Java verifyRate() endpoints
// (server.rate_limit.user_paths) per user; install it after authentication.
func (a *App) UserRateLimitMiddleware() gin.HandlerFunc {
	return middleware.UserRateLimit(a.userLimiter, a.Config.Server.RateLimit.UserPaths)
}

// LoginRateLimitMiddleware throttles login attempts per client IP.
func (a *App) LoginRateLimitMiddleware() gin.HandlerFunc {
	return middleware.LoginRateLimit(a.loginLimiter)
}
