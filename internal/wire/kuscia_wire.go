package wire

import (
	"context"
	"time"

	v1 "github.com/fengzhizi319/privahub/internal/controller/http/v1"
	"github.com/fengzhizi319/privahub/internal/dao/repository"
	"github.com/fengzhizi319/privahub/internal/service"
	"github.com/fengzhizi319/privahub/pkg/config"
	"github.com/fengzhizi319/privahub/pkg/kuscia"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// NewKusciaClients builds the default KusciaAPI HTTP client (kuscia.api_address
// + kuscia.http_port, TLS/mTLS/Token from config) and the multi-node client
// pool (Java DynamicKusciaChannelProvider): kuscia.nodes from config are
// registered statically, runtime registrations are loaded from kuscia.pool_file.
func NewKusciaClients(cfg *config.Config, log *zap.Logger) (*kuscia.Client, *kuscia.ClientPool) {
	if log == nil {
		log = zap.NewNop()
	}
	kc := cfg.Kuscia
	httpPort := kc.HTTPPort
	if httpPort == 0 {
		httpPort = 8082
	}
	timeout := kc.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	base := kuscia.ClientConfig{
		Host:               kc.APIAddress,
		Port:               httpPort,
		Protocol:           kc.Protocol,
		Timeout:            timeout,
		CertFile:           kc.CertFile,
		KeyFile:            kc.KeyFile,
		CAFile:             kc.CAFile,
		Token:              kc.Token,
		InsecureSkipVerify: kc.InsecureSkipVerify,
	}
	client := kuscia.NewClient(&base)
	if err := client.Err(); err != nil {
		log.Error("kuscia client init failed; requests will fail until fixed", zap.Error(err))
	}

	pool := kuscia.NewClientPool(client, kc.PoolFile)
	for _, n := range kc.Nodes {
		if n.DomainID == "" {
			continue
		}
		port := n.HTTPPort
		if port == 0 {
			port = httpPort
		}
		ep := kuscia.NodeEndpoint{
			DomainID: n.DomainID,
			Mode:     n.Mode,
			ClientConfig: kuscia.ClientConfig{
				Host: wireFirstNonEmpty(n.Host, kc.APIAddress), Port: port,
				Protocol: wireFirstNonEmpty(n.Protocol, kc.Protocol), Timeout: timeout,
				CertFile: wireFirstNonEmpty(n.CertFile, kc.CertFile), KeyFile: wireFirstNonEmpty(n.KeyFile, kc.KeyFile),
				CAFile: wireFirstNonEmpty(n.CAFile, kc.CAFile), Token: wireFirstNonEmpty(n.Token, kc.Token),
				InsecureSkipVerify: kc.InsecureSkipVerify,
			},
		}
		if err := pool.Register(ep); err != nil {
			log.Warn("register kuscia node from config failed", zap.String("domain_id", n.DomainID), zap.Error(err))
		}
	}
	if err := pool.Load(); err != nil {
		log.Warn("load persisted kuscia nodes failed", zap.Error(err))
	}
	return client, pool
}

func wireFirstNonEmpty(vs ...string) string {
	for _, v := range vs {
		if v != "" {
			return v
		}
	}
	return ""
}

// nodeDomainDeps are the node/route/inst/certificate/dataproxy components.
type nodeDomainDeps struct {
	InstService        *service.InstService
	InstHandler        *v1.InstHandler
	CertificateService *service.CertificateService
	DataProxyService   *service.DataProxyService
}

// wireNodeDomain configures NodeService with the client pool / env / data
// proxy, builds InstService + handler and CertificateService, and starts the
// DataProxy startup task (Java DbDataInit) plus the register listener (Java
// KusciaRegisterListener).
func wireNodeDomain(db *gorm.DB, cfg *config.Config, log *zap.Logger, nodeService *service.NodeService,
	kusciaClient *kuscia.Client, pool *kuscia.ClientPool, env *service.EnvService) nodeDomainDeps {
	if log == nil {
		log = zap.NewNop()
	}
	localNodeID := ""
	autonomy := false
	instID := ""
	if env != nil {
		localNodeID = env.GetPlatformNodeId()
		autonomy = env.IsAutonomy()
		instID = env.GetInstID()
	}
	dataProxy := service.NewDataProxyService(kusciaClient, repository.NewNodeRepo(db), cfg.Kuscia.DataProxy, localNodeID, log)
	dataProxy.SetPool(pool)

	nodeService.Configure(service.NodeServiceOptions{
		Pool: pool, Env: env, DataProxy: dataProxy, Protocol: cfg.Kuscia.Protocol, Log: log,
		TeeNodeID: cfg.Data.Tee.DomainID,
	})

	pool.OnChange(func(ep kuscia.NodeEndpoint, registered bool) {
		if !registered || !dataProxy.Enabled() {
			return
		}
		go func(domainID string) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			dataProxy.UpdateDomain(ctx, domainID)
		}(ep.DomainID)
	})
	if dataProxy.Enabled() {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			dataProxy.Run(ctx, autonomy, instID)
		}()
	}

	instService := service.NewInstService(db, nodeService, cfg.Kuscia.CertsDir, cfg.Kuscia.CAFile, log)
	return nodeDomainDeps{
		InstService:        instService,
		InstHandler:        v1.NewInstHandler(db, instService),
		CertificateService: service.NewCertificateService(nodeService),
		DataProxyService:   dataProxy,
	}
}
