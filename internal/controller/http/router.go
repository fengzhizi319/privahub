// Package http provides the HTTP router and handler registration for SecretPad-Go.
package http

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/fengzhizi319/privahub/internal/controller/http/apidocs"
	"github.com/fengzhizi319/privahub/internal/controller/http/middleware"
	v1 "github.com/fengzhizi319/privahub/internal/controller/http/v1"
	"github.com/fengzhizi319/privahub/internal/service"
	"github.com/fengzhizi319/privahub/internal/wire"
	"github.com/fengzhizi319/privahub/pkg/errcode"
	"github.com/fengzhizi319/privahub/pkg/response"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
)

// NewRouter creates and configures the Gin engine with all routes registered.
func NewRouter(log *zap.Logger, app *wire.App) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()

	// Global middleware chain
	// CaseKeysResponse must be outermost so every /api JSON response gets
	// camelCase twin keys added (frontend contract is uniformly camelCase while
	// legacy Go DTOs marshal snake_case).
	r.Use(middleware.CaseKeysResponse())
	r.Use(middleware.TraceID())
	r.Use(middleware.Recovery(log))
	r.Use(middleware.Metrics())
	r.Use(middleware.CORS(app.Config.Server.CORSAllowedOrigins...))
	r.Use(middleware.SecurityHeaders())
	r.Use(middleware.ExtraHeaders(app.Config.Server.ExtraResponseHeaders))
	// Inner-port-only paths answer 404 on the main port (Java inner-port.path).
	r.Use(middleware.BlockInnerPaths(app.Config.Server.InnerPortPaths))
	// EDGE request forwarding (Java EdgeRequestFilter) + disabled routes
	// (Java unregister-mapping.path), see wire/sync_wire.go.
	if app.Sync != nil && len(app.Sync.MainMiddlewares) > 0 {
		r.Use(app.Sync.MainMiddlewares...)
	}
	r.Use(middleware.AuditLog(log))
	// Coarse per-IP limiter (server.rate_limit.global_burst/global_rps).
	if rl := app.Config.Server.RateLimit; rl.GlobalBurst > 0 && rl.GlobalRPS > 0 {
		r.Use(middleware.RateLimiter(rl.GlobalBurst, rl.GlobalRPS))
	}
	// Global body limit (server.max_body_size, default 32MB); upload routes use
	// server.upload_max_size (default unlimited, Java upload-file.max-request-size=-1).
	r.Use(middleware.BodyLimitWithOverrides(app.Config.Server.MaxBodySize, uploadLimits(app)))
	// CaseKeysRequest runs after BodyLimit so it reads the size-limited body; it
	// additively injects snake_case/camelCase twin keys into JSON request bodies
	// so handlers binding either convention receive the frontend's camelCase fields.
	r.Use(middleware.CaseKeysRequest())

	// Health check (no auth required)
	r.GET("/api/v1alpha1/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status": gin.H{"code": 0, "msg": "success"},
			"data":   gin.H{"status": "healthy", "service": "secretpad-go"},
		})
	})

	// Prometheus metrics endpoint (observability.metrics_access: public in
	// dev/test, loopback / allowlist / bearer token otherwise).
	if hs := app.MetricsHandlers(gin.WrapH(promhttp.Handler())); len(hs) > 0 {
		r.GET("/metrics", hs...)
	}

	// API v1alpha1 group (matches Java SecretPad route prefix)
	api := r.Group("/api/v1alpha1")
	{
		// Public auth aliases: the frontend calls these Java SecretPad paths
		// first, keep them working without JWT.
		api.POST("/user/login", app.LoginRateLimitMiddleware(), app.AuthHandler.Login)
		// Java LoginConfiguration excludes inst/node/register from login: the
		// lite node's util.sh calls it and is authenticated by the inst token.
		api.POST("/inst/node/register", app.InstHandler.RegisterInstNode)

		// Protected routes (JWT auth required)
		protected := api.Group("")
		protected.Use(app.AuthMiddleware(), app.RBACMiddleware(), app.DataResourceMiddleware(), app.UserRateLimitMiddleware())
		{
			// Logout requires JWT to identify the user whose session is invalidated.
			protected.POST("/user/logout", app.AuthHandler.Logout)

			registerProjectRoutes(protected, app.ProjectHandler)
			registerGraphRoutes(protected, app.GraphHandler)
			registerJobRoutes(protected, app.JobHandler)
			registerNodeRoutes(protected, app.NodeHandler)
			registerDatatableRoutes(protected, app.DatatableHandler)
			registerDatasourceRoutes(protected, app.DatasourceHandler)
			registerUserRoutes(protected, app.AuthHandler, app.UserHandler)
			registerModelRoutes(protected, app.ModelHandler)
			registerServingRoutes(protected, app.ModelHandler)
			registerInstRoutes(protected, app.InstHandler)
			registerScheduledRoutes(protected, app.ScheduledHandler)
			registerComponentRoutes(protected, app.MiscHandler)
			registerNodeRouteRoutes(protected, app.NodeRouteHandler)
			registerApprovalRoutes(protected, app.ApprovalHandler)
			registerMessageRoutes(protected, app.MessageHandler)
		}
	}

	// P2P mode routes (JWT auth required — these are frontend-facing management APIs)
	p2p := api.Group("/p2p")
	p2p.Use(app.AuthMiddleware(), app.RBACMiddleware(), app.DataResourceMiddleware())
	{
		registerP2PRoutes(p2p, app.P2PHandler)
	}

	// Auxiliary routes
	aux := api.Group("")
	aux.Use(app.AuthMiddleware(), app.RBACMiddleware(), app.DataResourceMiddleware(), app.UserRateLimitMiddleware())
	{
		registerDataRoutes(aux, app.DataHandler)
		registerNodeUserRoutes(aux, app)
		registerFeatureTableRoutes(aux, app)
		registerGraphDatasourceRoutes(aux, app)
		registerEdgeDataSyncRoutes(aux, app)
	}

	// Java SecretPad-compatible auth routes (AuthController maps to /api)
	authGroup := r.Group("/api")
	registerAuthRoutes(authGroup, app.AuthHandler, app.LoginRateLimitMiddleware(), app.AuthMiddleware(), app.RBACMiddleware())

	// OpenAPI 3 document (server.api_docs, Java springdoc).
	registerAPIDocs(api, app)

	// Env/platform info (public)
	r.GET("/api/v1alpha1/env", func(c *gin.Context) {
		response.OK(c, app.EnvService.GetEnv())
	})

	// SPA static file serving for production
	spaDir := os.Getenv("PRIVAHUB_WEB_DIR")
	if spaDir == "" {
		spaDir = "./web/dist"
	}
	if info, err := os.Stat(spaDir); err == nil && info.IsDir() {
		// Serve static assets
		r.Static("/assets", filepath.Join(spaDir, "assets"))
		r.StaticFile("/favicon.ico", filepath.Join(spaDir, "favicon.ico"))

		// SPA fallback: all non-API GET routes serve index.html
		r.NoRoute(func(c *gin.Context) {
			path := c.Request.URL.Path
			if strings.HasPrefix(path, "/api/") {
				c.JSON(http.StatusNotFound, gin.H{
					"status": gin.H{"code": 202011504, "msg": "resource not found"},
				})
				return
			}
			c.File(filepath.Join(spaDir, "index.html"))
		})
	} else {
		// No frontend build — return JSON 404
		r.NoRoute(func(c *gin.Context) {
			c.JSON(http.StatusNotFound, gin.H{
				"status": gin.H{"code": 202011504, "msg": "resource not found"},
			})
		})
	}

	return r
}

// Route registration - additional endpoint groups.
func registerProjectRoutes(rg *gin.RouterGroup, h *v1.ProjectHandler) {
	rg.POST("/project/create", h.Create)
	rg.POST("/project/update", h.Update)
	rg.POST("/project/delete", h.Delete)
	rg.POST("/project/list", h.List)
	rg.POST("/project/get", h.Get)
	rg.POST("/project/node/add", h.AddNode)
	rg.POST("/project/inst/add", h.AddInst)
	rg.POST("/project/archive", h.Archive)
	rg.POST("/project/datatable/add", h.AddDatatable)
	rg.POST("/project/datatable/delete", h.DeleteDatatable)
	rg.POST("/project/datatable/get", h.GetDatatable)
	rg.POST("/project/tee/list", h.TeeList)
	rg.POST("/project/getOutTable", h.GetOutTable)
	rg.POST("/project/update/tableConfig", h.UpdateTableConfig)
	rg.POST("/project/datasource/list", h.DatasourceList)
}

func registerGraphRoutes(rg *gin.RouterGroup, h *v1.GraphHandler) {
	rg.POST("/graph/create", h.Create)
	rg.POST("/graph/list", h.List)
	rg.POST("/graph/detail", h.Detail)
	rg.POST("/graph/delete", h.Delete)
	rg.POST("/graph/update", h.FullUpdate)
	rg.POST("/graph/meta/update", h.UpdateMeta)
	rg.POST("/graph/node/update", h.UpdateNode)
	rg.POST("/graph/start", h.Start)
	rg.POST("/graph/stop", h.Stop)
	rg.POST("/graph/node/status", h.NodeStatus)
	rg.POST("/graph/node/output", h.NodeOutput)
	rg.POST("/graph/node/logs", h.NodeLogs)
	rg.POST("/graph/node/max_index", h.NodeMaxIndex)
}

func registerJobRoutes(rg *gin.RouterGroup, h *v1.JobHandler) {
	rg.POST("/project/job/create", h.Create)
	rg.POST("/project/job/list", h.List)
	rg.POST("/project/job/detail", h.Detail)
	rg.POST("/project/job/stop", h.Stop)
	rg.POST("/project/job/task/log", h.TaskLogs)
	// Java SecretPad-compatible aliases used by the frontend (client.ts).
	rg.POST("/project/job/get", h.Detail)
	rg.POST("/project/job/task/logs", h.TaskLogs)
	rg.POST("/project/job/task/output", h.TaskOutput)
}

func registerNodeRoutes(rg *gin.RouterGroup, h *v1.NodeHandler) {
	rg.POST("/node/create", h.Create)
	rg.POST("/node/update", h.Update)
	rg.POST("/node/list", h.List)
	rg.POST("/node/get", h.Get)
	rg.POST("/node/delete", h.Delete)
	rg.POST("/node/refresh", h.Refresh)
	rg.POST("/node/token", h.Token)
	rg.POST("/node/newToken", h.NewToken)
	rg.POST("/node/route/create", h.CreateRoute)
	rg.POST("/node/route/list", h.ListRoutes)
	rg.POST("/node/route/delete", h.DeleteRoute)
	rg.POST("/node/page", h.Page)
	rg.POST("/node/result/list", h.ResultList)
	rg.POST("/node/result/detail", h.ResultDetail)
}

func registerDatatableRoutes(rg *gin.RouterGroup, h *v1.DatatableHandler) {
	rg.POST("/datatable/register", h.Register)
	rg.POST("/datatable/list", h.List)
	rg.POST("/datatable/detail", h.Detail)
	rg.POST("/datatable/delete", h.Delete)
	rg.POST("/datatable/grant", h.Grant)
	rg.POST("/datatable/fed/create", h.CreateFedTable)
	rg.POST("/datatable/create", h.Create)
	rg.POST("/datatable/get", h.Get)
	rg.POST("/datatable/pushToTee", h.PushToTee)
}

func registerDatasourceRoutes(rg *gin.RouterGroup, h *v1.DatasourceHandler) {
	rg.POST("/datasource/create", h.Create)
	rg.POST("/datasource/list", h.List)
	rg.POST("/datasource/detail", h.Detail)
	rg.POST("/datasource/delete", h.Delete)
	rg.POST("/datasource/test", h.Test)
	rg.POST("/datasource/nodes", h.Nodes)
}

func registerAuthRoutes(rg *gin.RouterGroup, h *v1.AuthHandler, loginLimit gin.HandlerFunc, authMW ...gin.HandlerFunc) {
	rg.POST("/login", loginLimit, h.Login)
	// Logout must be authenticated: it deletes only the caller's session.
	rg.POST("/logout", append(authMW, h.Logout)...)
	rg.POST("/user/refresh", h.RefreshToken)
}

// uploadLimits maps upload routes to server.upload_max_size.
func uploadLimits(app *wire.App) map[string]int64 {
	m := make(map[string]int64, len(app.Config.Server.UploadPaths))
	for _, p := range app.Config.Server.UploadPaths {
		m[p] = app.Config.Server.UploadMaxSize
	}
	return m
}

func registerUserRoutes(rg *gin.RouterGroup, authH *v1.AuthHandler, userH *v1.UserHandler) {
	rg.GET("/user/current", authH.GetCurrentUser)
	rg.POST("/user/create", userH.Create)
	rg.POST("/user/list", userH.List)
	rg.POST("/user/get", userH.Get)
	rg.POST("/user/update", userH.Update)
	rg.POST("/user/delete", userH.Delete)
	rg.POST("/user/reset-password", userH.ResetPassword)
	rg.POST("/user/updatePwd", userH.UpdatePwd)
	rg.POST("/user/remote/resetPassword", userH.RemoteResetPassword)
	rg.POST("/user/node/resetPassword", userH.NodeResetPassword)
}

func registerModelRoutes(rg *gin.RouterGroup, h *v1.ModelHandler) {
	// ModelExportController
	rg.POST("/model/pack", h.Pack)
	rg.POST("/model/status", h.PackStatus)
	rg.POST("/model/modelPartyPath", h.ModelPartyPath)
	// ModelManagementController
	rg.POST("/model/page", h.Page)
	rg.POST("/model/detail", h.Detail)
	rg.POST("/model/info", h.Info)
	rg.POST("/model/discard", h.Discard)
	rg.POST("/model/delete", h.Delete)
}

func registerServingRoutes(rg *gin.RouterGroup, h *v1.ModelHandler) {
	rg.POST("/model/serving/create", h.CreateServing)
	rg.POST("/model/serving/detail", h.ServingDetail)
	rg.POST("/model/serving/delete", h.DeleteServing)
}

func registerInstRoutes(rg *gin.RouterGroup, h *v1.InstHandler) {
	rg.POST("/inst/create", h.CreateInst)
	rg.POST("/inst/list", h.ListInsts)
	rg.POST("/inst/get", h.GetInst)
	rg.POST("/inst/node/list", h.ListInstNodes)
	rg.POST("/inst/node/add", h.AddInstNode)
	rg.POST("/inst/node/token", h.InstNodeToken)
	rg.POST("/inst/node/newToken", h.InstNodeNewToken)
	rg.POST("/inst/node/delete", h.DeleteInstNode)
}

func registerScheduledRoutes(rg *gin.RouterGroup, h *v1.ScheduledHandler) {
	// Java ScheduledController contract. The Go-only legacy routes
	// (create/list/delete/pause/resume) were removed: nothing used them.
	rg.POST("/scheduled/id", h.GetID)
	rg.POST("/scheduled/graph/once/success", h.OnceSuccess)
	rg.POST("/scheduled/graph/create", h.GraphCreate)
	rg.POST("/scheduled/page", h.Page)
	rg.POST("/scheduled/offline", h.Offline)
	rg.POST("/scheduled/del", h.Del)
	rg.POST("/scheduled/info", h.Info)
	rg.POST("/scheduled/task/page", h.TaskPage)
	rg.POST("/scheduled/task/stop", h.TaskStop)
	rg.POST("/scheduled/task/rerun", h.TaskReRun)
	rg.POST("/scheduled/task/info", h.TaskInfo)
	rg.POST("/scheduled/job/list", h.JobList)
}

func registerComponentRoutes(rg *gin.RouterGroup, h *v1.MiscHandler) {
	rg.POST("/component/list", h.ListComponents)
	rg.POST("/component/version", h.ComponentVersion)
	rg.POST("/component/i18n", h.ComponentI18n)
	rg.POST("/component/batch", h.ComponentBatch)
	rg.POST("/version/list", h.VersionList)
}

func registerP2PRoutes(rg *gin.RouterGroup, h *v1.P2PHandler) {
	rg.POST("/project/create", h.ProjectCreate)
	rg.POST("/project/list", h.ProjectList)
	rg.POST("/project/update", h.ProjectUpdate)
	rg.POST("/project/archive", h.ProjectArchive)
	rg.POST("/project/participants", h.ProjectParticipants)
	rg.POST("/node/create", h.NodeCreate)
	rg.POST("/node/delete", h.NodeDelete)
	// The Go-only /p2p/data/sync route was removed: nothing called it; the
	// P2P consumer is the inner-port /api/v1alpha1/data/sync.
}

func registerDataRoutes(rg *gin.RouterGroup, h *v1.DataHandler) {
	// Data endpoints
	rg.POST("/data/upload", h.Upload)
	rg.POST("/data/create", h.Create)
	rg.POST("/data/download", h.Download)
	// data/sync and vote_sync/create are inner-port only (inner_router.go).

	// Feature datasource
	rg.POST("/feature_datasource/create", h.FeatureDatasourceCreate)
	rg.POST("/feature_datasource/auth/list", h.FeatureDatasourceAuthList)

	// Cloud log
	rg.POST("/cloud_log/sls", h.CloudLogSls)

}

func registerNodeRouteRoutes(rg *gin.RouterGroup, h *v1.NodeRouteHandler) {
	rg.POST("/nodeRoute/page", h.Page)
	rg.POST("/nodeRoute/get", h.Get)
	rg.POST("/nodeRoute/update", h.Update)
	rg.POST("/nodeRoute/listNode", h.ListNode)
	rg.POST("/nodeRoute/refresh", h.Refresh)
	rg.POST("/nodeRoute/delete", h.Delete)
}

func registerApprovalRoutes(rg *gin.RouterGroup, h *v1.ApprovalHandler) {
	rg.POST("/approval/create", h.Create)
	rg.POST("/approval/pull/status", h.PullStatus)
}

func registerMessageRoutes(rg *gin.RouterGroup, h *v1.MessageHandler) {
	rg.POST("/message/list", h.List)
	rg.POST("/message/detail", h.Detail)
	rg.POST("/message/reply", h.Reply)
	rg.POST("/message/pending", h.Pending)
}

// registerNodeUserRoutes registers the Go-only /nodeUser/* management routes
// (ADMIN role required, see middleware.AdminOnlyPaths).
func registerNodeUserRoutes(rg *gin.RouterGroup, app *wire.App) {
	rg.POST("/nodeUser/create", func(c *gin.Context) {
		var req service.NodeUserCreateRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			response.FailArgs(c, errcode.SystemValidationError, err.Error())
			return
		}
		if err := app.NodeUserService.Create(c.Request.Context(), &req); err != nil {
			code, msg := service.NodeUserErrorCode(err, &req)
			response.FailArgs(c, code, msg)
			return
		}
		response.OK(c, nil)
	})
	rg.POST("/nodeUser/resetPassword", app.UserHandler.NodeResetPassword)
	rg.POST("/nodeUser/list", func(c *gin.Context) {
		var req service.NodeUserListRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			response.FailArgs(c, errcode.SystemValidationError, err.Error())
			return
		}
		users, err := app.NodeUserService.ListByNodeId(c.Request.Context(), &req)
		if err != nil {
			response.FailWithMsg(c, errcode.SystemError, err.Error())
			return
		}
		response.OK(c, users)
	})
}

func registerFeatureTableRoutes(rg *gin.RouterGroup, app *wire.App) {
	rg.POST("/featureTable/create", func(c *gin.Context) {
		var req service.CreateFeatureDatasourceRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			response.Fail(c, errcode.ParamError)
			return
		}
		if err := app.FeatureTableService.CreateFeatureTable(c.Request.Context(), &req); err != nil {
			response.Fail(c, errcode.SystemError)
			return
		}
		response.OK(c, nil)
	})
	rg.POST("/featureTable/list", func(c *gin.Context) {
		var req struct {
			NodeID string `json:"node_id"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			response.Fail(c, errcode.ParamError)
			return
		}
		list, err := app.FeatureTableService.FeatureDatasourceList(c.Request.Context(), req.NodeID)
		if err != nil {
			response.Fail(c, errcode.SystemError)
			return
		}
		response.OK(c, list)
	})
	rg.POST("/featureTable/project/list", func(c *gin.Context) {
		var req struct {
			NodeID    string `json:"node_id"`
			ProjectID string `json:"project_id"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			response.Fail(c, errcode.ParamError)
			return
		}
		list, err := app.FeatureTableService.ProjectFeatureTableList(c.Request.Context(), req.NodeID, req.ProjectID)
		if err != nil {
			response.Fail(c, errcode.SystemError)
			return
		}
		response.OK(c, list)
	})
}

func registerGraphDatasourceRoutes(rg *gin.RouterGroup, app *wire.App) {
	rg.POST("/graphDatasource/bind", func(c *gin.Context) {
		var req service.GraphDatasourceBindRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			response.Fail(c, errcode.ParamError)
			return
		}
		if err := app.GraphDatasourceService.Bind(c.Request.Context(), &req); err != nil {
			response.Fail(c, errcode.SystemError)
			return
		}
		response.OK(c, nil)
	})
	rg.POST("/graphDatasource/unbind", func(c *gin.Context) {
		var req struct {
			ProjectID string `json:"project_id" binding:"required"`
			GraphID   string `json:"graph_id" binding:"required"`
			DomainID  string `json:"domain_id" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			response.Fail(c, errcode.ParamError)
			return
		}
		if err := app.GraphDatasourceService.Unbind(c.Request.Context(), req.ProjectID, req.GraphID, req.DomainID); err != nil {
			response.Fail(c, errcode.SystemError)
			return
		}
		response.OK(c, nil)
	})
	rg.POST("/graphDatasource/list", func(c *gin.Context) {
		var req struct {
			ProjectID string `json:"project_id" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			response.Fail(c, errcode.ParamError)
			return
		}
		list, err := app.GraphDatasourceService.ListByProject(c.Request.Context(), req.ProjectID)
		if err != nil {
			response.Fail(c, errcode.SystemError)
			return
		}
		response.OK(c, list)
	})
}

func registerEdgeDataSyncRoutes(rg *gin.RouterGroup, app *wire.App) {
	rg.POST("/edgeDataSync/log", func(c *gin.Context) {
		logs, err := app.EdgeDataSyncService.GetSyncLogs(c.Request.Context())
		if err != nil {
			response.Fail(c, errcode.SystemError)
			return
		}
		response.OK(c, logs)
	})
	// The SSE sync endpoint is GET /sync on the inner port (inner_router.go).
}

// registerAPIDocs serves the OpenAPI 3 document at GET /api/v1alpha1/docs when
// server.api_docs.enabled (default: dev/test profiles). Unless
// server.api_docs.public is set, a logged-in session is required.
func registerAPIDocs(api *gin.RouterGroup, app *wire.App) {
	cfg := app.Config.Server.APIDocs
	if !cfg.Enabled {
		return
	}
	if cfg.Public {
		api.GET("/docs", apidocs.Handler)
		return
	}
	api.GET("/docs", app.AuthMiddleware(), apidocs.Handler)
}
