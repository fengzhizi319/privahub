package http

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/fengzhizi319/privahub/internal/controller/http/middleware"
	"github.com/fengzhizi319/privahub/internal/wire"
	"go.uber.org/zap"
)

// NewInnerRouter creates the inner port router for cluster-internal communication.
// These endpoints are accessible without JWT authentication and are used for
// inter-node communication (vote sync, data sync, password reset).
func NewInnerRouter(log *zap.Logger, app *wire.App) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()

	r.Use(middleware.TraceID())
	r.Use(middleware.Recovery(log))

	// Health check
	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "healthy", "port": "inner"})
	})

	// Inner API routes (no auth required, cluster-internal only)
	api := r.Group("/api/v1alpha1")
	{
		// Vote sync - inter-node vote synchronization
		api.POST("/vote_sync/create", app.DataHandler.VoteSyncCreate)

		// Node user password reset - center resets edge node password
		api.POST("/user/node/resetPassword", app.UserHandler.NodeResetPassword)

		// Data sync - inter-node data synchronization
		api.POST("/data/sync", app.DataHandler.Sync)
	}

	// SSE sync endpoint for edge data synchronization
	r.GET("/sync", app.SseServer.HandleSseSync)

	return r
}
