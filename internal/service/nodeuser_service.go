package service

import (
	"context"
	"errors"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"gorm.io/gorm"
)

// NodeUser service errors.
var (
	ErrNodeUserNotFound = errors.New("node user not found")
	ErrNodeUserExists   = errors.New("node user already exists")
)

// NodeUserService handles node-specific user management.
type NodeUserService struct {
	db *gorm.DB
}

// NewNodeUserService creates a new NodeUserService.
func NewNodeUserService(db *gorm.DB) *NodeUserService {
	return &NodeUserService{db: db}
}

// --- DTOs ---

// NodeUserCreateRequest represents a node user creation request.
type NodeUserCreateRequest struct {
	NodeID      string `json:"node_id"`
	NodeIDAlt   string `json:"nodeId"`
	UserName    string `json:"user_name"`
	UserNameAlt string `json:"userName"`
	Password    string `json:"password"`
}

// ResetNodeUserPwdRequest represents a node user password reset request.
type ResetNodeUserPwdRequest struct {
	NodeID      string `json:"node_id"`
	NodeIDAlt   string `json:"nodeId"`
	UserName    string `json:"user_name"`
	UserNameAlt string `json:"userName"`
	Password    string `json:"password"`
}

// NodeUserListRequest represents a node user list request.
type NodeUserListRequest struct {
	NodeID    string `json:"node_id"`
	NodeIDAlt string `json:"nodeId"`
}

// NodeUserVO represents a node user view object.
type NodeUserVO struct {
	Name      string `json:"name"`
	NodeID    string `json:"node_id"`
	GmtCreate string `json:"gmt_create"`
}

// --- Service Methods ---

// Create creates a new node user account.
func (s *NodeUserService) Create(ctx context.Context, req *NodeUserCreateRequest) error {
	if req.NodeID == "" {
		req.NodeID = req.NodeIDAlt
	}
	if req.UserName == "" {
		req.UserName = req.UserNameAlt
	}
	// Check if user already exists for this node
	var count int64
	s.db.WithContext(ctx).Model(&model.UserAccountsDO{}).
		Where("name = ? AND owner_type = ? AND owner_id = ?", req.UserName, "EDGE", req.NodeID).
		Count(&count)
	if count > 0 {
		return ErrNodeUserExists
	}

	user := &model.UserAccountsDO{
		Name:         req.UserName,
		PasswordHash: HashPassword(req.Password),
		OwnerType:    "EDGE",
		OwnerID:      req.NodeID,
	}

	if err := s.db.WithContext(ctx).Create(user).Error; err != nil {
		return err
	}

	// Create user-node relationship
	rel := &model.SysUserNodeRelDO{
		UserID: req.UserName,
		NodeID: req.NodeID,
	}
	return s.db.WithContext(ctx).Create(rel).Error
}

// ResetPassword resets a node user's password.
func (s *NodeUserService) ResetPassword(ctx context.Context, req *ResetNodeUserPwdRequest) error {
	if req.NodeID == "" {
		req.NodeID = req.NodeIDAlt
	}
	if req.UserName == "" {
		req.UserName = req.UserNameAlt
	}
	var user model.UserAccountsDO
	if err := s.db.WithContext(ctx).
		Where("name = ? AND owner_type = ? AND owner_id = ?", req.UserName, "EDGE", req.NodeID).
		First(&user).Error; err != nil {
		return ErrNodeUserNotFound
	}

	return s.db.WithContext(ctx).Model(&user).
		Update("password_hash", HashPassword(req.Password)).Error
}

// ListByNodeId lists all users for a specific node.
func (s *NodeUserService) ListByNodeId(ctx context.Context, req *NodeUserListRequest) ([]NodeUserVO, error) {
	if req.NodeID == "" {
		req.NodeID = req.NodeIDAlt
	}
	var users []model.UserAccountsDO
	if err := s.db.WithContext(ctx).
		Where("owner_type = ? AND owner_id = ?", "EDGE", req.NodeID).
		Order("gmt_create DESC").
		Find(&users).Error; err != nil {
		return nil, err
	}

	result := make([]NodeUserVO, 0, len(users))
	for _, u := range users {
		result = append(result, NodeUserVO{
			Name:      u.Name,
			NodeID:    req.NodeID,
			GmtCreate: u.GmtCreate.Format("2006-01-02 15:04:05"),
		})
	}
	return result, nil
}
