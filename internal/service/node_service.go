package service

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/internal/dao/repository"
	"github.com/fengzhizi319/privahub/pkg/kuscia"
)

// Node service errors.
var (
	ErrNodeNotFound       = errors.New("node not found")
	ErrNodeAlreadyExists  = errors.New("node already exists")
	ErrRouteNotFound      = errors.New("route not found")
	ErrRouteAlreadyExists = errors.New("route already exists")
)

// NodeService handles node and route management.
type NodeService struct {
	nodeRepo     repository.NodeRepository
	routeRepo    repository.NodeRouteRepository
	kusciaClient *kuscia.Client
}

// NewNodeService creates a new NodeService.
func NewNodeService(nodeRepo repository.NodeRepository, routeRepo repository.NodeRouteRepository, kusciaClient *kuscia.Client) *NodeService {
	return &NodeService{nodeRepo: nodeRepo, routeRepo: routeRepo, kusciaClient: kusciaClient}
}

// CreateNodeRequest represents a node creation request.
type CreateNodeRequest struct {
	NodeID        string `json:"node_id" binding:"required"`
	Name          string `json:"name" binding:"required"`
	Auth          string `json:"auth"`
	Description   string `json:"description"`
	ControlNodeID string `json:"control_node_id"`
	NetAddress    string `json:"net_address"`
	Type          string `json:"type"`
	Mode          int    `json:"mode"`
	MasterNodeID  string `json:"master_node_id"`
}

// UpdateNodeRequest represents a node update request.
type UpdateNodeRequest struct {
	NodeID      string `json:"node_id" binding:"required"`
	Name        string `json:"name"`
	Description string `json:"description"`
	NetAddress  string `json:"net_address"`
	Mode        *int   `json:"mode"`
}

// NodeVO represents a node view object.
type NodeVO struct {
	NodeID        string `json:"node_id"`
	Name          string `json:"name"`
	Auth          string `json:"auth"`
	Description   string `json:"description"`
	ControlNodeID string `json:"control_node_id"`
	NetAddress    string `json:"net_address"`
	Type          string `json:"type"`
	Mode          int    `json:"mode"`
	MasterNodeID  string `json:"master_node_id"`
	Token         string `json:"token,omitempty"`
}

// CreateNode creates a new node.
func (s *NodeService) CreateNode(ctx context.Context, req *CreateNodeRequest) (*NodeVO, error) {
	// Check if node already exists
	existing, err := s.nodeRepo.FindByNodeID(ctx, req.NodeID)
	if err == nil && existing != nil {
		return nil, ErrNodeAlreadyExists
	}

	controlNodeID := req.ControlNodeID
	if controlNodeID == "" {
		controlNodeID = req.NodeID
	}
	nodeType := req.Type
	if nodeType == "" {
		nodeType = "normal"
	}
	masterNodeID := req.MasterNodeID
	if masterNodeID == "" {
		masterNodeID = "master"
	}

	node := &model.NodeDO{
		NodeID:        req.NodeID,
		Name:          req.Name,
		Auth:          req.Auth,
		Description:   req.Description,
		ControlNodeID: controlNodeID,
		NetAddress:    req.NetAddress,
		Type:          nodeType,
		Mode:          req.Mode,
		MasterNodeID:  masterNodeID,
	}

	if err := s.nodeRepo.Create(ctx, node); err != nil {
		return nil, err
	}

	// Register domain in Kuscia (best-effort)
	if s.kusciaClient != nil {
		_ = s.kusciaClient.CreateDomain(ctx, &kuscia.CreateDomainRequest{
			DomainID: req.NodeID,
			Role:     nodeType,
		})
	}

	return s.toNodeVO(node), nil
}

// UpdateNode updates an existing node.
func (s *NodeService) UpdateNode(ctx context.Context, req *UpdateNodeRequest) error {
	node, err := s.nodeRepo.FindByNodeID(ctx, req.NodeID)
	if err != nil {
		return ErrNodeNotFound
	}

	if req.Name != "" {
		node.Name = req.Name
	}
	if req.Description != "" {
		node.Description = req.Description
	}
	if req.NetAddress != "" {
		node.NetAddress = req.NetAddress
	}
	if req.Mode != nil {
		node.Mode = *req.Mode
	}

	return s.nodeRepo.Update(ctx, node)
}

// GetNode retrieves a node by ID.
func (s *NodeService) GetNode(ctx context.Context, nodeID string) (*NodeVO, error) {
	node, err := s.nodeRepo.FindByNodeID(ctx, nodeID)
	if err != nil {
		return nil, ErrNodeNotFound
	}
	return s.toNodeVO(node), nil
}

// ListNodes retrieves all nodes.
func (s *NodeService) ListNodes(ctx context.Context) ([]NodeVO, error) {
	nodes, err := s.nodeRepo.FindAll(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]NodeVO, 0, len(nodes))
	for i := range nodes {
		result = append(result, *s.toNodeVO(&nodes[i]))
	}
	return result, nil
}

// DeleteNode deletes a node by ID.
func (s *NodeService) DeleteNode(ctx context.Context, nodeID string) error {
	node, err := s.nodeRepo.FindByNodeID(ctx, nodeID)
	if err != nil {
		return ErrNodeNotFound
	}
	return s.nodeRepo.Delete(ctx, node.ID)
}

// GenerateToken generates a deployment token for a node.
func (s *NodeService) GenerateToken(ctx context.Context, nodeID string) (string, error) {
	node, err := s.nodeRepo.FindByNodeID(ctx, nodeID)
	if err != nil {
		return "", ErrNodeNotFound
	}

	token := uuid.New().String()
	node.Token = token
	if err := s.nodeRepo.Update(ctx, node); err != nil {
		return "", err
	}

	return token, nil
}

// CreateRouteRequest represents a route creation request.
type CreateRouteRequest struct {
	SrcNodeID     string `json:"src_node_id" binding:"required"`
	DstNodeID     string `json:"dst_node_id" binding:"required"`
	SrcNetAddress string `json:"src_net_address"`
	DstNetAddress string `json:"dst_net_address"`
}

// RouteVO represents a route view object.
type RouteVO struct {
	RouteID       string `json:"route_id"`
	SrcNodeID     string `json:"src_node_id"`
	DstNodeID     string `json:"dst_node_id"`
	SrcNetAddress string `json:"src_net_address"`
	DstNetAddress string `json:"dst_net_address"`
}

// CreateRoute creates a new node route (bidirectional).
func (s *NodeService) CreateRoute(ctx context.Context, req *CreateRouteRequest) error {
	// Check if route already exists
	existing, err := s.routeRepo.FindByPair(ctx, req.SrcNodeID, req.DstNodeID)
	if err == nil && existing != nil {
		return ErrRouteAlreadyExists
	}

	routeID := uuid.New().String()[:8]

	// Create forward route
	forward := &model.NodeRouteDO{
		RouteID:       routeID,
		SrcNodeID:     req.SrcNodeID,
		DstNodeID:     req.DstNodeID,
		SrcNetAddress: req.SrcNetAddress,
		DstNetAddress: req.DstNetAddress,
	}
	if err := s.routeRepo.Create(ctx, forward); err != nil {
		return err
	}

	// Create reverse route
	reverse := &model.NodeRouteDO{
		RouteID:       routeID,
		SrcNodeID:     req.DstNodeID,
		DstNodeID:     req.SrcNodeID,
		SrcNetAddress: req.DstNetAddress,
		DstNetAddress: req.SrcNetAddress,
	}
	if err := s.routeRepo.Create(ctx, reverse); err != nil {
		return err
	}

	// Register route in Kuscia (best-effort)
	if s.kusciaClient != nil {
		_ = s.kusciaClient.CreateDomainRoute(ctx, &kuscia.CreateDomainRouteRequest{
			Source:                req.SrcNodeID,
			Destination:           req.DstNodeID,
			SourceNetAddress:      req.SrcNetAddress,
			DestinationNetAddress: req.DstNetAddress,
		})
	}

	return nil
}

// ListRoutes retrieves all routes for a node.
func (s *NodeService) ListRoutes(ctx context.Context, nodeID string) ([]RouteVO, error) {
	routes, err := s.routeRepo.FindBySrcNodeID(ctx, nodeID)
	if err != nil {
		return nil, err
	}

	result := make([]RouteVO, 0, len(routes))
	for i := range routes {
		result = append(result, RouteVO{
			RouteID:       routes[i].RouteID,
			SrcNodeID:     routes[i].SrcNodeID,
			DstNodeID:     routes[i].DstNodeID,
			SrcNetAddress: routes[i].SrcNetAddress,
			DstNetAddress: routes[i].DstNetAddress,
		})
	}
	return result, nil
}

// DeleteRoute deletes a route between two nodes.
func (s *NodeService) DeleteRoute(ctx context.Context, srcNodeID, dstNodeID string) error {
	route, err := s.routeRepo.FindByPair(ctx, srcNodeID, dstNodeID)
	if err != nil {
		return ErrRouteNotFound
	}

	// Delete forward route
	if err := s.routeRepo.Delete(ctx, route.ID); err != nil {
		return err
	}

	// Delete reverse route
	reverse, err := s.routeRepo.FindByPair(ctx, dstNodeID, srcNodeID)
	if err == nil && reverse != nil {
		_ = s.routeRepo.Delete(ctx, reverse.ID)
	}

	return nil
}

func (s *NodeService) toNodeVO(node *model.NodeDO) *NodeVO {
	return &NodeVO{
		NodeID:        node.NodeID,
		Name:          node.Name,
		Auth:          node.Auth,
		Description:   node.Description,
		ControlNodeID: node.ControlNodeID,
		NetAddress:    node.NetAddress,
		Type:          node.Type,
		Mode:          node.Mode,
		MasterNodeID:  node.MasterNodeID,
	}
}
