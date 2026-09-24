package http

import (
	"net/http"

	"github.com/fengzhizi319/privahub/internal/controller/http/middleware"
	"github.com/fengzhizi319/privahub/internal/wire"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// NewInnerRouter creates the inner port router for cluster-internal
// communication (vote sync, data sync, node password reset, SSE sync).
//
// Every API route requires the `kuscia-origin-source` header (set by the
// Kuscia gateway) and runs as a node-scoped virtual user (Java
// LoginInterceptor.processByNodeRpcRequest); interface and data permissions
// are then enforced like on the main port. The listener binds to
// server.inner_bind_address (default 127.0.0.1).
func NewInnerRouter(log *zap.Logger, app *wire.App) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()

	r.Use(middleware.TraceID())
	r.Use(middleware.Recovery(log))
	// ClientIP = direct peer unless server.trusted_proxies says otherwise.
	if err := middleware.ConfigureTrustedProxies(r, app.Config.Server.TrustedProxies, app.Config.Server.TrustedPlatform); err != nil {
		log.Fatal("invalid server.trusted_proxies", zap.Error(err))
	}
	// Body limit on node-to-node requests (server.max_body_size).
	r.Use(middleware.BodyLimitWithOverrides(app.Config.Server.MaxBodySize, nil))
	// Routes disabled by sync.disabled_routes (Java unregister-mapping.path).
	if app.Sync != nil && len(app.Sync.InnerMiddlewares) > 0 {
		r.Use(app.Sync.InnerMiddlewares...)
	}

	// Health check (no auth)
	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "healthy", "port": "inner"})
	})

	// server.inner_auth (allowlist / shared token) runs first; mTLS, when
	// configured, is enforced by the listener (cmd/server).
	nodeAuth := []gin.HandlerFunc{app.InnerGuardMiddleware(), app.InnerAuthMiddleware(), app.RBACMiddleware(), app.DataResourceMiddleware()}

	api := r.Group("/api/v1alpha1", nodeAuth...)
	{
		// Vote sync - inter-node vote synchronization
		api.POST("/vote_sync/create", app.SyncHandler.VoteSyncCreate)

		// Node user password reset - edge (via gateway) resets its own node user
		api.POST("/user/node/resetPassword", app.UserHandler.NodeResetPassword)

		// Data sync - inter-node data synchronization
		api.POST("/data/sync", app.SyncHandler.DataSync)
	}

	// SSE sync endpoint for edge data synchronization
	r.GET("/sync", append(nodeAuth, app.SseServer.HandleSseSync)...)

	return r
}

// MountEdgeForwardTargets registers, on the CENTER inner port, the write
// endpoints an EDGE forwards through the Kuscia gateway (Java edge.forward):
// the edge request arrives with kuscia-origin-source and is served as the
// node virtual user by the same handlers as the main port. It must be called
// after both routers are built.
func MountEdgeForwardTargets(inner, main *gin.Engine, app *wire.App) {
	if app.Sync == nil || len(app.Sync.ForwardTargets) == 0 {
		return
	}
	want := make(map[string]bool, len(app.Sync.ForwardTargets))
	for _, p := range app.Sync.ForwardTargets {
		want[p] = true
	}
	existing := map[string]bool{}
	for _, ri := range inner.Routes() {
		existing[ri.Method+" "+ri.Path] = true
	}
	// CaseKeysRequest runs before the authorization guards so that
	// DataResourceGuard reads the same (validated, twin-expanded) body the
	// handler binds.
	chain := []gin.HandlerFunc{
		middleware.CaseKeysResponse(),
		app.InnerGuardMiddleware(), app.InnerAuthMiddleware(), middleware.CaseKeysRequest(),
		app.RBACMiddleware(), app.DataResourceMiddleware(), app.UserRateLimitMiddleware(),
	}
	for _, ri := range main.Routes() {
		if ri.Method != http.MethodPost || !want[ri.Path] || existing[ri.Method+" "+ri.Path] {
			continue
		}
		inner.POST(ri.Path, append(append([]gin.HandlerFunc{}, chain...), ri.HandlerFunc)...)
	}
}

// VerifyRouteCoverage runs middleware.VerifyRouteCoverage over every route of
// the given engines (cmd/server refuses to start on error).
func VerifyRouteCoverage(engines ...*gin.Engine) error {
	var all gin.RoutesInfo
	for _, e := range engines {
		all = append(all, e.Routes()...)
	}
	return middleware.VerifyRouteCoverage(all)
}
