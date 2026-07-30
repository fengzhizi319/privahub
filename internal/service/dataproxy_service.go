package service

import (
	"context"

	"github.com/fengzhizi319/privahub/internal/dao/repository"
	"github.com/fengzhizi319/privahub/pkg/kuscia"
	"go.uber.org/zap"
)

// DataProxyService manages Kuscia DomainDataSource access_directly flag
// to route data access through DataProxy.
// Corresponds to Java: org.secretflow.secretpad.service.dataproxy.DataProxyService
type DataProxyService struct {
	kusciaClient     *kuscia.Client
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

// UpdateDataSourceUseDataProxyInMaster updates all embedded nodes (alice/bob)
// to route data access through DataProxy in Master mode.
// Called after node registration when data-proxy is enabled.
func (s *DataProxyService) UpdateDataSourceUseDataProxyInMaster(ctx context.Context) {
	if !s.dataProxyEnabled {
		return
	}

	// In Master mode, update embedded nodes (alice, bob)
	embeddedNodes := []string{"alice", "bob"}
	for _, nodeID := range embeddedNodes {
		s.updateDataSourceUseDataProxyByDomainID(ctx, nodeID)
	}
}

// UpdateDataSourceUseDataProxyInP2p updates all nodes belonging to an institution
// to route data access through DataProxy in P2P mode.
func (s *DataProxyService) UpdateDataSourceUseDataProxyInP2p(ctx context.Context, instID string) {
	if !s.dataProxyEnabled {
		return
	}

	// In P2P mode, find nodes by control node (inst mapping)
	nodes, err := s.nodeRepo.FindByControlNodeID(ctx, instID)
	if err != nil {
		// Fallback: try all nodes
		allNodes, err2 := s.nodeRepo.FindAll(ctx)
		if err2 != nil {
			s.log.Error("failed to list nodes for inst",
				zap.String("inst_id", instID),
				zap.Error(err),
			)
			return
		}
		for _, node := range allNodes {
			s.updateDataSourceUseDataProxyByDomainID(ctx, node.NodeID)
		}
		return
	}
	for _, node := range nodes {
		s.updateDataSourceUseDataProxyByDomainID(ctx, node.NodeID)
	}
}

// updateDataSourceUseDataProxyByDomainID lists all data sources for a domain
// and sets access_directly=false for those currently set to true.
// This forces data access to go through the DataProxy service.
func (s *DataProxyService) updateDataSourceUseDataProxyByDomainID(ctx context.Context, domainID string) {
	dataSources, err := s.kusciaClient.ListDomainDataSource(ctx, domainID)
	if err != nil {
		s.log.Error("failed to list datasources for domain",
			zap.String("domain_id", domainID),
			zap.Error(err),
		)
		return
	}

	accessDirectlyFalse := false
	for _, ds := range dataSources {
		if ds.AccessDirectly {
			updateReq := &kuscia.UpdateDomainDataSourceRequest{
				DomainID:       ds.DomainID,
				DatasourceID:   ds.DatasourceID,
				AccessDirectly: &accessDirectlyFalse,
			}
			if err := s.kusciaClient.UpdateDomainDataSource(ctx, updateReq); err != nil {
				s.log.Error("failed to update datasource to use data proxy",
					zap.String("domain_id", domainID),
					zap.String("datasource_id", ds.DatasourceID),
					zap.Error(err),
				)
			} else {
				s.log.Info("updated datasource to use data proxy",
					zap.String("domain_id", domainID),
					zap.String("datasource_id", ds.DatasourceID),
				)
			}
		}
	}
}
