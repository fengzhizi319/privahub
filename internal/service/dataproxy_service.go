package service

import (
	"context"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/internal/dao/repository"
	"github.com/fengzhizi319/privahub/pkg/kuscia"
	"go.uber.org/zap"
)

// DataProxyService manages Kuscia DomainDataSource access_directly flag
// to route data access through DataProxy.
// Corresponds to Java: org.secretflow.secretpad.service.dataproxy.DataProxyService
// (called from DbDataInit at startup and KusciaRegisterListener on register).
type DataProxyService struct {
	kusciaClient     *kuscia.Client
	pool             *kuscia.ClientPool
	nodeRepo         repository.NodeRepository
	dataProxyEnabled bool
	localNodeID      string
	log              *zap.Logger
}

// NewDataProxyService creates a new DataProxyService.
func NewDataProxyService(
	kusciaClient *kuscia.Client,
	nodeRepo repository.NodeRepository,
	dataProxyEnabled bool,
	localNodeID string,
	log *zap.Logger,
) *DataProxyService {
	if log == nil {
		log = zap.NewNop()
	}
	return &DataProxyService{
		kusciaClient:     kusciaClient,
		nodeRepo:         nodeRepo,
		dataProxyEnabled: dataProxyEnabled,
		localNodeID:      localNodeID,
		log:              log,
	}
}

// SetPool makes the service use per-node KusciaAPI channels when registered.
func (s *DataProxyService) SetPool(p *kuscia.ClientPool) { s.pool = p }

// Enabled reports whether DataProxy routing is enabled.
func (s *DataProxyService) Enabled() bool { return s != nil && s.dataProxyEnabled }

func (s *DataProxyService) client(domainID string) *kuscia.Client {
	if s.pool != nil {
		if c, ok := s.pool.Lookup(domainID); ok {
			return c
		}
	}
	return s.kusciaClient
}

// Run is the startup hook (Java DbDataInit → updateDataSourceUseDataProxyInMaster/InP2p):
// in master mode update embedded nodes, in autonomy mode the nodes of instID.
func (s *DataProxyService) Run(ctx context.Context, autonomy bool, instID string) {
	if !s.Enabled() {
		return
	}
	if autonomy {
		s.UpdateDataSourceUseDataProxyInP2p(ctx, instID)
		return
	}
	s.UpdateDataSourceUseDataProxyInMaster(ctx)
}

// UpdateDataSourceUseDataProxyInMaster updates all embedded nodes to route
// data access through DataProxy in Master mode.
func (s *DataProxyService) UpdateDataSourceUseDataProxyInMaster(ctx context.Context) {
	if !s.Enabled() {
		return
	}
	nodes, err := s.nodeRepo.FindByType(ctx, nodeTypeEmbedded)
	if err != nil {
		s.log.Error("failed to list embedded nodes", zap.Error(err))
		return
	}
	for _, n := range nodes {
		s.updateDataSourceUseDataProxyByDomainID(ctx, n.NodeID)
	}
}

// UpdateDataSourceUseDataProxyInP2p updates all nodes belonging to an institution.
func (s *DataProxyService) UpdateDataSourceUseDataProxyInP2p(ctx context.Context, instID string) {
	if !s.Enabled() {
		return
	}
	all, err := s.nodeRepo.FindAll(ctx)
	if err != nil {
		s.log.Error("failed to list nodes for inst", zap.String("inst_id", instID), zap.Error(err))
		return
	}
	for _, node := range filterInstNodes(all, instID, s.localNodeID) {
		s.updateDataSourceUseDataProxyByDomainID(ctx, node.NodeID)
	}
}

// UpdateDomain forces DataProxy access for one domain (Java KusciaRegisterListener).
func (s *DataProxyService) UpdateDomain(ctx context.Context, domainID string) {
	if !s.Enabled() {
		return
	}
	s.updateDataSourceUseDataProxyByDomainID(ctx, domainID)
}

// updateDataSourceUseDataProxyByDomainID lists all data sources for a domain
// and sets access_directly=false for those currently set to true.
func (s *DataProxyService) updateDataSourceUseDataProxyByDomainID(ctx context.Context, domainID string) {
	c := s.client(domainID)
	if c == nil {
		return
	}
	dataSources, err := c.ListDomainDataSource(ctx, domainID)
	if err != nil {
		s.log.Warn("failed to list datasources for domain", zap.String("domain_id", domainID), zap.Error(err))
		return
	}
	accessDirectlyFalse := false
	for _, ds := range dataSources {
		if !ds.AccessDirectly {
			continue
		}
		updateReq := &kuscia.UpdateDomainDataSourceRequest{
			DomainID:       firstNonEmpty(ds.DomainID, domainID),
			DatasourceID:   ds.DatasourceID,
			AccessDirectly: &accessDirectlyFalse,
		}
		if err := c.UpdateDomainDataSource(ctx, updateReq); err != nil {
			s.log.Warn("failed to update datasource to use data proxy",
				zap.String("domain_id", domainID), zap.String("datasource_id", ds.DatasourceID), zap.Error(err))
		} else {
			s.log.Info("updated datasource to use data proxy",
				zap.String("domain_id", domainID), zap.String("datasource_id", ds.DatasourceID))
		}
	}
}

func filterInstNodes(all []model.NodeDO, instID, localNodeID string) []model.NodeDO {
	out := make([]model.NodeDO, 0, len(all))
	for _, n := range all {
		if (instID != "" && n.InstID == instID) || (instID == "" && n.NodeID == localNodeID) {
			out = append(out, n)
		}
	}
	return out
}
