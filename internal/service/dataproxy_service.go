package service

import (
	"context"
	"log"

	"github.com/fengzhizi319/privahub/internal/dao/repository"
	"github.com/fengzhizi319/privahub/pkg/kuscia"
)

// DataProxyService manages Kuscia DomainDataSource access_directly flag
// to route data access through DataProxy.
// Corresponds to Java: org.secretflow.secretpad.service.dataproxy.DataProxyService
type DataProxyService struct {
	kusciaClient   *kuscia.Client
	nodeRepo       repository.NodeRepository
	dataProxyEnabled bool
	localNodeID    string
}

// NewDataProxyService creates a new DataProxyService.
func NewDataProxyService(
	kusciaClient *kuscia.Client,
	nodeRepo repository.NodeRepository,
	dataProxyEnabled bool,
	localNodeID string,
) *DataProxyService {
	return &DataProxyService{
		kusciaClient:     kusciaClient,
		nodeRepo:         nodeRepo,
		dataProxyEnabled: dataProxyEnabled,
		localNodeID:      localNodeID,
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
			log.Printf("[DataProxy] failed to list nodes for inst %s: %v", instID, err)
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
		log.Printf("[DataProxy] failed to list datasources for domain %s: %v", domainID, err)
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
				log.Printf("[DataProxy] failed to update datasource %s/%s: %v", domainID, ds.DatasourceID, err)
			} else {
				log.Printf("[DataProxy] updated datasource %s/%s to use data proxy", domainID, ds.DatasourceID)
			}
		}
	}
}
