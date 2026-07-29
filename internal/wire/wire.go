// Package wire provides dependency injection setup using Google Wire.
package wire

import (
	"time"

	"github.com/fengzhizi319/privahub/internal/controller/http/middleware"
	v1 "github.com/fengzhizi319/privahub/internal/controller/http/v1"
	"github.com/fengzhizi319/privahub/internal/dao/repository"
	"github.com/fengzhizi319/privahub/internal/service"
	"github.com/fengzhizi319/privahub/pkg/auth"
	"github.com/fengzhizi319/privahub/pkg/config"
	"github.com/fengzhizi319/privahub/pkg/kuscia"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Providers is a set of all dependency providers.
var Providers = []interface{}{
	// Repositories
	repository.NewUserAccountsRepo,
	repository.NewUserTokensRepo,
	repository.NewNodeRepo,
	repository.NewNodeRouteRepo,
	repository.NewProjectRepo,
	repository.NewInstRepo,
	repository.NewJobRepo,
	repository.NewTaskRepo,
	repository.NewTaskLogRepo,
	repository.NewGraphRepo,
	repository.NewGraphNodeRepo,
	repository.NewSysUserPermissionRepo,
	repository.NewSysUserNodeRepo,

	// Services
	NewAuthService,

	// Handlers
	v1.NewAuthHandler,
}

// NewAuthService creates an AuthService with the required dependencies.
func NewAuthService(
	db *gorm.DB,
	cfg *config.Config,
) *service.AuthService {
	userRepo := repository.NewUserAccountsRepo(db)
	tokenRepo := repository.NewUserTokensRepo(db)

	accessExpiry := cfg.Auth.AccessTokenExpiry
	if accessExpiry == 0 {
		accessExpiry = 2 * time.Hour
	}
	refreshExpiry := cfg.Auth.RefreshTokenExpiry
	if refreshExpiry == 0 {
		refreshExpiry = 168 * time.Hour // 7 days
	}

	jwtManager := auth.NewJWTManager(cfg.Auth.JWTSecret, accessExpiry, refreshExpiry)

	return service.NewAuthService(userRepo, tokenRepo, jwtManager)
}

// NewJWTManager creates a JWTManager from config.
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

// NewAuthMiddleware creates the JWT auth middleware.
func NewAuthMiddleware(cfg *config.Config) func(*auth.JWTManager) func(c interface{}) {
	return func(jwtManager *auth.JWTManager) func(c interface{}) {
		return func(c interface{}) {
			// Middleware is created in router via middleware.JWTAuth
		}
	}
}

// App holds all initialized dependencies.
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
	VoteService       *service.VoteService
	UserService       *service.UserService
	DatasourceService *service.DatasourceService
	ModelService      *service.ModelService
	AuthHandler       *v1.AuthHandler
	NodeHandler       *v1.NodeHandler
	ProjectHandler    *v1.ProjectHandler
	GraphHandler      *v1.GraphHandler
	JobHandler        *v1.JobHandler
	DatatableHandler  *v1.DatatableHandler
	VoteHandler       *v1.VoteHandler
	UserHandler       *v1.UserHandler
	DatasourceHandler *v1.DatasourceHandler
	ModelHandler      *v1.ModelHandler
	MiscHandler       *v1.MiscHandler
	NodeRouteHandler  *v1.NodeRouteHandler
	ApprovalHandler   *v1.ApprovalHandler
	MessageHandler    *v1.MessageHandler
	P2PHandler        *v1.P2PHandler
	DataHandler       *v1.DataHandler
	ScheduledHandler  *v1.ScheduledHandler

	// New services
	NodeUserService        *service.NodeUserService
	FeatureTableService    *service.FeatureTableService
	GraphDatasourceService *service.GraphDatasourceService
	EdgeDataSyncService    *service.EdgeDataSyncService
	SseServer              *service.SseServer
	EnvService             *service.EnvService
	DataDirService         *service.DataDirectoryService
}

// NewApp creates and initializes the application with all dependencies.
func NewApp(db *gorm.DB, cfg *config.Config) *App {
	jwtManager := NewJWTManager(cfg)

	// Kuscia HTTP client — prefer http_port, fall back to api_port
	kusciaPort := cfg.Kuscia.HTTPPort
	if kusciaPort == 0 {
		kusciaPort = cfg.Kuscia.APIPort
	}
	kusciaClient := kuscia.NewClient(&kuscia.ClientConfig{
		Host:     cfg.Kuscia.APIAddress,
		Port:     kusciaPort,
		Protocol: cfg.Kuscia.Protocol,
		Timeout:  30 * time.Second,
	})

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
	voteRequestRepo := repository.NewVoteRequestRepo(db)
	voteInviteRepo := repository.NewVoteInviteRepo(db)
	permRepo := repository.NewSysUserPermissionRepo(db)
	sysUserNodeRepo := repository.NewSysUserNodeRepo(db)

	// Services
	authService := service.NewAuthService(userRepo, tokenRepo, jwtManager)
	nodeService := service.NewNodeService(nodeRepo, routeRepo, kusciaClient)
	projectService := service.NewProjectService(projectRepo, projectInstRepo, projectNodeRepo, datatableRepo, db)
	graphService := service.NewGraphService(graphRepo, graphNodeRepo, jobRepo, taskRepo, taskLogRepo, kusciaClient)
	jobService := service.NewJobService(jobRepo, taskRepo, taskLogRepo, graphRepo, graphNodeRepo, kusciaClient)
	datatableService := service.NewDatatableService(datatableRepo, fedTableRepo, db, kusciaClient)
	voteService := service.NewVoteService(voteRequestRepo, voteInviteRepo)
	userService := service.NewUserService(userRepo, permRepo, sysUserNodeRepo)
	datasourceService := service.NewDatasourceService(db, kusciaClient)
	modelService := service.NewModelService(db, kusciaClient)

	// Handlers
	authHandler := v1.NewAuthHandler(authService)
	nodeHandler := v1.NewNodeHandler(nodeService)
	projectHandler := v1.NewProjectHandler(projectService, datatableService, nodeService)
	graphHandler := v1.NewGraphHandler(graphService)
	jobHandler := v1.NewJobHandler(jobService)
	datatableHandler := v1.NewDatatableHandler(datatableService, kusciaClient)
	voteHandler := v1.NewVoteHandler(voteService)
	userHandler := v1.NewUserHandler(userService)
	datasourceHandler := v1.NewDatasourceHandler(datasourceService)
	modelHandler := v1.NewModelHandler(modelService, kusciaClient)
	miscHandler := v1.NewMiscHandler(db, kusciaClient)
	nodeRouteHandler := v1.NewNodeRouteHandler(db, kusciaClient)
	approvalHandler := v1.NewApprovalHandler(db)
	messageHandler := v1.NewMessageHandler(db)
	p2pHandler := v1.NewP2PHandler(db, kusciaClient)
	dataHandler := v1.NewDataHandler(db, kusciaClient)

	// Scheduled service with cron engine
	scheduledService := service.NewScheduledService(db, nil, graphService)
	scheduledHandler := v1.NewScheduledHandler(scheduledService)

	// New services
	nodeUserService := service.NewNodeUserService(db)
	featureTableService := service.NewFeatureTableService(db)
	graphDatasourceService := service.NewGraphDatasourceService(db)
	edgeDataSyncService := service.NewEdgeDataSyncService(db)
	sseServer := service.NewSseServer(nil)
	envService := service.NewEnvService(cfg.Server.Mode, cfg.Kuscia.Namespace, "", nil)
	dataDirService := service.NewDataDirectoryService("")

	// Background job status sync service
	jobSyncService := service.NewJobStatusSyncService(db, kusciaClient, nil)
	jobSyncService.Start()

	servingSyncService := service.NewServingStatusSyncService(db, kusciaClient, nil)
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
		VoteService:       voteService,
		UserService:       userService,
		DatasourceService: datasourceService,
		ModelService:      modelService,
		AuthHandler:       authHandler,
		NodeHandler:       nodeHandler,
		ProjectHandler:    projectHandler,
		GraphHandler:      graphHandler,
		JobHandler:        jobHandler,
		DatatableHandler:  datatableHandler,
		VoteHandler:       voteHandler,
		UserHandler:       userHandler,
		DatasourceHandler: datasourceHandler,
		ModelHandler:      modelHandler,
		MiscHandler:       miscHandler,
		NodeRouteHandler:  nodeRouteHandler,
		ApprovalHandler:   approvalHandler,
		MessageHandler:    messageHandler,
		P2PHandler:        p2pHandler,
		DataHandler:       dataHandler,
		ScheduledHandler:  scheduledHandler,

		NodeUserService:        nodeUserService,
		FeatureTableService:    featureTableService,
		GraphDatasourceService: graphDatasourceService,
		EdgeDataSyncService:    edgeDataSyncService,
		SseServer:              sseServer,
		EnvService:             envService,
		DataDirService:         dataDirService,
	}
}

// GetAuthMiddleware returns the JWT authentication middleware.
func (a *App) GetAuthMiddleware() gin.HandlerFunc {
	return middleware.JWTAuth(a.JWTManager)
}
