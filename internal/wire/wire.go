// Package wire provides manual dependency injection for the Privahub application.
// All services, repositories, and handlers are constructed in NewApp with explicit
// wiring, avoiding the complexity of a DI framework while maintaining testability.
package wire

import (
	"time"

	"github.com/fengzhizi319/privahub/internal/controller/http/middleware"
	v1 "github.com/fengzhizi319/privahub/internal/controller/http/v1"
	"github.com/fengzhizi319/privahub/internal/dao/repository"
	"github.com/fengzhizi319/privahub/internal/service"
	"github.com/fengzhizi319/privahub/pkg/auth"
	"github.com/fengzhizi319/privahub/pkg/config"
	"github.com/fengzhizi319/privahub/pkg/innerauth"
	"github.com/fengzhizi319/privahub/pkg/kuscia"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// NewJWTManager creates a JWTManager from config with default expiry values.
func NewJWTManager(cfg *config.Config) *auth.JWTManager {
	accessExpiry := cfg.Auth.AccessTokenExpiry
	if accessExpiry == 0 {
		accessExpiry = 2 * time.Hour
	}
	refreshExpiry := cfg.Auth.RefreshTokenExpiry
	if refreshExpiry == 0 {
		refreshExpiry = 168 * time.Hour
	}
	return auth.NewJWTManager(cfg.Auth.JWTSecret, accessExpiry, refreshExpiry)
}

// App holds all initialized dependencies for the Privahub application.
// It acts as a poor-man's DI container: all services and handlers are
// constructed once at startup and shared across the HTTP layer.
type App struct {
	DB                *gorm.DB
	Config            *config.Config
	JWTManager        *auth.JWTManager
	KusciaClient      *kuscia.Client
	AuthService       *service.AuthService
	NodeService       *service.NodeService
	ProjectService    *service.ProjectService
	GraphService      *service.GraphService
	JobService        *service.JobService
	DatatableService  *service.DatatableService
	UserService       *service.UserService
	DatasourceService *service.DatasourceService
	ModelService      *service.ModelService
	AuthHandler       *v1.AuthHandler
	NodeHandler       *v1.NodeHandler
	ProjectHandler    *v1.ProjectHandler
	GraphHandler      *v1.GraphHandler
	JobHandler        *v1.JobHandler
	DatatableHandler  *v1.DatatableHandler
	UserHandler       *v1.UserHandler
	DatasourceHandler *v1.DatasourceHandler
	ModelHandler      *v1.ModelHandler
	MiscHandler       *v1.MiscHandler
	NodeRouteHandler  *v1.NodeRouteHandler
	InstHandler       *v1.InstHandler

	// Kuscia multi-node client pool + node domain services
	KusciaPool         *kuscia.ClientPool
	InstService        *service.InstService
	CertificateService *service.CertificateService
	DataProxyService   *service.DataProxyService
	ApprovalHandler    *v1.ApprovalHandler
	MessageHandler     *v1.MessageHandler
	P2PHandler         *v1.P2PHandler
	DataHandler        *v1.DataHandler
	ScheduledHandler   *v1.ScheduledHandler

	// New services
	NodeUserService        *service.NodeUserService
	FeatureTableService    *service.FeatureTableService
	GraphDatasourceService *service.GraphDatasourceService
	EdgeDataSyncService    *service.EdgeDataSyncService
	SseServer              *service.SseServer
	// Sync holds data-sync (SSE producer / edge client / P2P queues /
	// DbSyncSender), platform middlewares and startup init (sync_wire.go).
	Sync        *SyncWiring
	SyncHandler *v1.SyncHandler
	EnvService  *service.EnvService
	// DataResourceAuth checks node/project/inst data permissions (Java
	// DataResourceAspect); handlers use middleware.RequireDataResource.
	DataResourceAuth *service.DataResourceAuth
	// Data holds datasource/datatable/data/feature/cloud-log/partition-rule
	// services configured from cfg.Data (data_wire.go).
	Data *DataWiring
	// Vote holds the vote/approval/message/P2P services and monitors (vote_wire.go).
	Vote *VoteWiring

	// TeePullService pulls TEE results after TEE_DOWNLOAD approval
	// (implements votehandler.TeeResultPuller).
	TeePullService *service.TeePullService

	// Background services (for graceful shutdown)
	ScheduledService   *service.ScheduledService
	JobSyncService     *service.JobStatusSyncService
	ServingSyncService *service.ServingStatusSyncService

	// Rate limiters (Java RateLimitUtil), see auth_wire.go.
	userLimiter  *middleware.KeyedLimiter
	loginLimiter *middleware.KeyedLimiter
}

// NewApp creates and initializes the application with all dependencies.
// The log parameter is passed to background services for operational logging.
func NewApp(db *gorm.DB, cfg *config.Config, log *zap.Logger) *App {
	jwtManager := NewJWTManager(cfg)
	// Outgoing node-to-node calls carry server.inner_auth.token.
	if token, err := cfg.Server.InnerAuth.ResolvedToken(); err == nil {
		innerauth.SetOutgoingToken(token)
	}

	// Kuscia HTTP client (kuscia.http_port, default 8082) + multi-node client pool.
	kusciaClient, kusciaPool := NewKusciaClients(cfg, log)

	// Repositories
	userRepo := repository.NewUserAccountsRepo(db)
	tokenRepo := repository.NewUserTokensRepo(db)
	nodeRepo := repository.NewNodeRepo(db)
	routeRepo := repository.NewNodeRouteRepo(db)
	projectRepo := repository.NewProjectRepo(db)
	projectInstRepo := repository.NewProjectInstRepo(db)
	projectNodeRepo := repository.NewProjectNodeRepo(db)
	graphRepo := repository.NewGraphRepo(db)
	graphNodeRepo := repository.NewGraphNodeRepo(db)
	jobRepo := repository.NewJobRepo(db)
	taskRepo := repository.NewTaskRepo(db)
	taskLogRepo := repository.NewTaskLogRepo(db)
	datatableRepo := repository.NewDatatableRepo(db)
	fedTableRepo := repository.NewFedTableRepo(db)
	permRepo := repository.NewSysUserPermissionRepo(db)
	sysUserNodeRepo := repository.NewSysUserNodeRepo(db)

	// Services
	authService := service.NewAuthService(userRepo, tokenRepo, jwtManager)
	nodeService := service.NewNodeService(nodeRepo, routeRepo, kusciaClient, db)
	projectService := service.NewProjectService(projectRepo, projectInstRepo, projectNodeRepo, datatableRepo, db, kusciaClient)
	graphService := service.NewGraphService(graphRepo, graphNodeRepo, jobRepo, taskRepo, taskLogRepo, kusciaClient, db)
	jobService := service.NewJobService(jobRepo, taskRepo, taskLogRepo, graphRepo, graphNodeRepo, kusciaClient, db)
	datatableService := service.NewDatatableService(datatableRepo, fedTableRepo, db, kusciaClient)
	userService := service.NewUserService(userRepo, permRepo, sysUserNodeRepo, db)
	datasourceService := service.NewDatasourceService(db, kusciaClient)
	modelService := service.NewModelService(db, kusciaClient)

	// Per-node KusciaAPI routing (Java DynamicKusciaChannelProvider.createStub):
	// job submission / sync / schedule calls go to the channel of the job
	// initiator / party node, falling back to the default client.
	kusciaRouter := service.NewKusciaRouter(kusciaPool, kusciaClient, db)
	if kusciaClient != nil {
		graphService.SetKuscia(kusciaRouter)
		jobService.SetKuscia(kusciaRouter)
	}

	// Handlers
	authHandler := v1.NewAuthHandler(authService)
	nodeHandler := v1.NewNodeHandler(nodeService)
	projectHandler := v1.NewProjectHandler(projectService, datatableService, nodeService)
	graphHandler := v1.NewGraphHandler(graphService)
	jobHandler := v1.NewJobHandler(jobService).WithGraphService(graphService)
	datatableHandler := v1.NewDatatableHandler(datatableService, kusciaClient)
	userHandler := v1.NewUserHandler(userService)
	datasourceHandler := v1.NewDatasourceHandler(datasourceService)
	modelHandler := v1.NewModelHandler(modelService, kusciaClient)
	miscHandler := v1.NewMiscHandler(db, kusciaClient)
	nodeRouteHandler := v1.NewNodeRouteHandlerWithService(nodeService)
	dataHandler := v1.NewDataHandler(db, kusciaClient)

	// Scheduled (periodic) graph jobs: DB-backed engine + status write-back.
	// The graph service is used as scheduled-mode renderer when it implements
	// service.ScheduledGraphRenderer; the job sync service may forward Kuscia
	// status of schedule jobs via service.CurrentScheduleJobStatusHook().
	scheduledService := service.NewScheduledService(db, log, graphService, kusciaClient, service.ScheduledOptionsFromEnv())
	if kusciaClient != nil {
		scheduledService.SetKusciaClient(kusciaRouter)
	}
	service.RegisterScheduleJobStatusHook(scheduledService)
	scheduledService.Start()
	scheduledHandler := v1.NewScheduledHandler(scheduledService)

	// New services
	nodeUserService := service.NewNodeUserService(db)
	featureTableService := service.NewFeatureTableService(db)
	graphDatasourceService := service.NewGraphDatasourceService(db)
	edgeDataSyncService := service.NewEdgeDataSyncService(db)
	// Bug78 fix: pass logger to SseServer for operational visibility.
	sseServer := service.NewSseServerWithInterval(log, cfg.Sync.Ping())
	envService := NewEnvServiceFromConfig(cfg, log)
	dataResourceAuth := wireAuth(db, cfg, envService, authService, userService, nodeUserService, userHandler)
	// Node / route / inst / certificate / dataproxy wiring (pool-aware).
	nodeDomain := wireNodeDomain(db, cfg, log, nodeService, kusciaClient, kusciaPool, envService)
	// Vote / approval / message / P2P services + vote status monitors (vote_wire.go).
	voteWiring := wireVote(db, log, nodeService, nodeDomain.CertificateService, kusciaClient, envService)
	approvalHandler := v1.NewApprovalHandler(voteWiring.Service)
	messageHandler := v1.NewMessageHandler(voteWiring.Service)
	p2pHandler := v1.NewP2PHandler(db, voteWiring.P2P, log)
	// Datasource / datatable / data / feature / cloud-log wiring (data_wire.go).
	dataWiring := wireData(db, cfg, log, kusciaClient, kusciaPool, envService, datatableService, datasourceService, featureTableService, dataHandler)
	datatableService.WithNodeService(nodeService)
	// Data sync + platform middlewares + startup init (sync_wire.go).
	syncWiring := wireSync(db, cfg, log, envService, sseServer, authService, kusciaClient, kusciaPool, nodeDomain.DataProxyService)
	// TEE graph execution / datatable auth / result pull (tee_graph.go, tee_pull.go).
	teePullService := wireTee(cfg, kusciaPool, graphService, projectService, datatableService)
	// Approved TEE_DOWNLOAD votes pull the result from the TEE node.
	voteWiring.Service.SetTeeResultPuller(teePullService)

	// Background job status sync service
	jobSyncService := service.NewJobStatusSyncService(db, kusciaClient, log)
	if kusciaClient != nil {
		// Polling resolves each job's channel; one watch per pool node.
		jobSyncService.SetKuscia(kusciaRouter)
		jobSyncService.SetPool(kusciaPool)
	}
	// Model export (pack) jobs: status sync writes project_model_pack on success.
	modelService.WithKusciaPool(kusciaPool).WithEnv(envService).WithGraphService(graphService).WithLogger(log)
	jobSyncService.RegisterHandler(modelService)
	jobSyncService.Start()

	// Serving status goes through ModelService, which routes per party node
	// (ModelService.kc -> ClientPool) with the default client as fallback.
	servingSyncService := service.NewServingStatusSyncService(db, kusciaClient, log).WithModelService(modelService)
	servingSyncService.Start()

	return &App{
		DB:                db,
		Config:            cfg,
		JWTManager:        jwtManager,
		KusciaClient:      kusciaClient,
		AuthService:       authService,
		NodeService:       nodeService,
		ProjectService:    projectService,
		GraphService:      graphService,
		JobService:        jobService,
		DatatableService:  datatableService,
		UserService:       userService,
		DatasourceService: datasourceService,
		ModelService:      modelService,
		AuthHandler:       authHandler,
		NodeHandler:       nodeHandler,
		ProjectHandler:    projectHandler,
		GraphHandler:      graphHandler,
		JobHandler:        jobHandler,
		DatatableHandler:  datatableHandler,
		UserHandler:       userHandler,
		DatasourceHandler: datasourceHandler,
		ModelHandler:      modelHandler,
		MiscHandler:       miscHandler,
		NodeRouteHandler:  nodeRouteHandler,
		InstHandler:       nodeDomain.InstHandler,

		KusciaPool:         kusciaPool,
		InstService:        nodeDomain.InstService,
		CertificateService: nodeDomain.CertificateService,
		DataProxyService:   nodeDomain.DataProxyService,
		ApprovalHandler:    approvalHandler,
		MessageHandler:     messageHandler,
		P2PHandler:         p2pHandler,
		DataHandler:        dataHandler,
		ScheduledHandler:   scheduledHandler,

		NodeUserService:        nodeUserService,
		FeatureTableService:    featureTableService,
		GraphDatasourceService: graphDatasourceService,
		EdgeDataSyncService:    edgeDataSyncService,
		SseServer:              sseServer,
		Sync:                   syncWiring,
		SyncHandler:            syncWiring.Handler,
		EnvService:             envService,
		DataResourceAuth:       dataResourceAuth,
		Data:                   dataWiring,
		Vote:                   voteWiring,
		TeePullService:         teePullService,

		ScheduledService:   scheduledService,
		JobSyncService:     jobSyncService,
		ServingSyncService: servingSyncService,

		userLimiter:  newUserLimiter(cfg),
		loginLimiter: newLoginLimiter(cfg),
	}
}

// Shutdown gracefully stops all background services.
func (a *App) Shutdown() {
	if a.DatatableService != nil {
		a.DatatableService.Stop() // TEE job status poller
	}
	if a.ScheduledService != nil {
		a.ScheduledService.Stop()
	}
	if a.JobSyncService != nil {
		a.JobSyncService.Stop()
	}
	if a.ServingSyncService != nil {
		a.ServingSyncService.Stop()
	}
	a.Vote.Stop()
	// Bug79 fix: stop SSE heartbeat goroutine and close all sessions.
	a.Sync.Stop()
	if a.SseServer != nil {
		a.SseServer.Stop()
	}
}
