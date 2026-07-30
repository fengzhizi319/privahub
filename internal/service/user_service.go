package service

import (
	"context"
	"crypto/hmac"
	"errors"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/internal/dao/repository"
	"gorm.io/gorm"
)

// User service errors.
var (
	ErrUserAlreadyExists = errors.New("user already exists")
)

// UserService handles user account management.
type UserService struct {
	userRepo repository.UserAccountsRepository
	permRepo repository.SysUserPermissionRepository
	nodeRepo repository.SysUserNodeRepository
	db       *gorm.DB // used for transactional operations
}

// NewUserService creates a new UserService.
func NewUserService(
	userRepo repository.UserAccountsRepository,
	permRepo repository.SysUserPermissionRepository,
	nodeRepo repository.SysUserNodeRepository,
	db *gorm.DB,
) *UserService {
	return &UserService{
		userRepo: userRepo,
		permRepo: permRepo,
		nodeRepo: nodeRepo,
		db:       db,
	}
}

// --- Request / Response DTOs ---

// CreateUserRequest represents a user creation request.
type CreateUserRequest struct {
	Name      string   `json:"name" binding:"required"`
	Password  string   `json:"password" binding:"required"`
	OwnerType string   `json:"owner_type"`
	OwnerID   string   `json:"owner_id"`
	RoleCodes []string `json:"role_codes"`
	NodeIDs   []string `json:"node_ids"`
}

// UpdateUserRequest represents a user update request.
type UpdateUserRequest struct {
	Name      string   `json:"name" binding:"required"`
	OwnerType string   `json:"owner_type"`
	OwnerID   string   `json:"owner_id"`
	RoleCodes []string `json:"role_codes"`
	NodeIDs   []string `json:"node_ids"`
}

// ResetPasswordRequest represents a password reset request.
type ResetPasswordRequest struct {
	Name        string `json:"name" binding:"required"`
	NewPassword string `json:"new_password" binding:"required"`
}

// UserVO represents a user view object.
type UserVO struct {
	ID        int64    `json:"id"`
	Name      string   `json:"name"`
	OwnerType string   `json:"owner_type"`
	OwnerID   string   `json:"owner_id"`
	RoleCodes []string `json:"role_codes,omitempty"`
	NodeIDs   []string `json:"node_ids,omitempty"`
	GmtCreate string   `json:"gmt_create"`
}

// UserListResponse represents a user list response.
type UserListResponse struct {
	Users []UserVO `json:"users"`
	Total int64    `json:"total"`
}

// --- Service Methods ---

// CreateUser creates a new user account. The user, role assignments, and node
// associations are created atomically within a database transaction to prevent
// orphan records on partial failure.
func (s *UserService) CreateUser(ctx context.Context, req *CreateUserRequest) (*UserVO, error) {
	// Check if user already exists
	existing, _ := s.userRepo.FindByName(ctx, req.Name)
	if existing != nil {
		return nil, ErrUserAlreadyExists
	}

	ownerType := req.OwnerType
	if ownerType == "" {
		ownerType = "CENTER"
	}
	ownerID := req.OwnerID
	if ownerID == "" {
		ownerID = "kuscia-system"
	}

	user := &model.UserAccountsDO{
		Name:         req.Name,
		PasswordHash: HashPassword(req.Password),
		OwnerType:    ownerType,
		OwnerID:      ownerID,
	}

	// Wrap user + roles + nodes creation in a transaction for atomicity
	if s.db != nil {
		if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := tx.Create(user).Error; err != nil {
				return err
			}
			for _, roleCode := range req.RoleCodes {
				rel := &model.SysUserPermissionRelDO{
					UserType:   "USER",
					UserKey:    req.Name,
					TargetType: "ROLE",
					TargetCode: roleCode,
				}
				if err := tx.Create(rel).Error; err != nil {
					return err
				}
			}
			for _, nodeID := range req.NodeIDs {
				rel := &model.SysUserNodeRelDO{
					UserID: req.Name,
					NodeID: nodeID,
				}
				if err := tx.Create(rel).Error; err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
			return nil, err
		}
	} else {
		// Fallback without transaction (when db is nil, e.g. in unit tests)
		if err := s.userRepo.Create(ctx, user); err != nil {
			return nil, err
		}
		for _, roleCode := range req.RoleCodes {
			_ = s.permRepo.Create(ctx, &model.SysUserPermissionRelDO{
				UserType:   "USER",
				UserKey:    req.Name,
				TargetType: "ROLE",
				TargetCode: roleCode,
			})
		}
		for _, nodeID := range req.NodeIDs {
			_ = s.nodeRepo.Create(ctx, &model.SysUserNodeRelDO{
				UserID: req.Name,
				NodeID: nodeID,
			})
		}
	}

	return s.toUserVO(ctx, user), nil
}

// ListUsers lists all users.
func (s *UserService) ListUsers(ctx context.Context) (*UserListResponse, error) {
	users, err := s.userRepo.FindAll(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]UserVO, 0, len(users))
	for i := range users {
		result = append(result, *s.toUserVO(ctx, &users[i]))
	}

	return &UserListResponse{
		Users: result,
		Total: int64(len(result)),
	}, nil
}

// UpdateUser updates a user's roles and nodes.
// Role and node updates use delete-then-insert wrapped in a database transaction
// to prevent data loss on partial failure.
func (s *UserService) UpdateUser(ctx context.Context, req *UpdateUserRequest) error {
	user, err := s.userRepo.FindByName(ctx, req.Name)
	if err != nil {
		return ErrUserNotFound
	}

	if req.OwnerType != "" {
		user.OwnerType = req.OwnerType
	}
	if req.OwnerID != "" {
		user.OwnerID = req.OwnerID
	}
	if err := s.userRepo.Update(ctx, user); err != nil {
		return err
	}

	// Use transaction when db is available to ensure atomicity of delete-then-insert
	if s.db != nil && (req.RoleCodes != nil || req.NodeIDs != nil) {
		return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if req.RoleCodes != nil {
				if err := tx.Where("user_key = ?", req.Name).Delete(&model.SysUserPermissionRelDO{}).Error; err != nil {
					return err
				}
				for _, roleCode := range req.RoleCodes {
					rel := &model.SysUserPermissionRelDO{
						UserType:   "USER",
						UserKey:    req.Name,
						TargetType: "ROLE",
						TargetCode: roleCode,
					}
					if err := tx.Create(rel).Error; err != nil {
						return err
					}
				}
			}
			if req.NodeIDs != nil {
				if err := tx.Where("user_id = ?", req.Name).Delete(&model.SysUserNodeRelDO{}).Error; err != nil {
					return err
				}
				for _, nodeID := range req.NodeIDs {
					rel := &model.SysUserNodeRelDO{
						UserID: req.Name,
						NodeID: nodeID,
					}
					if err := tx.Create(rel).Error; err != nil {
						return err
					}
				}
			}
			return nil
		})
	}

	// Fallback without transaction (when db is nil, e.g. in unit tests)
	if req.RoleCodes != nil {
		_ = s.permRepo.DeleteByUserKey(ctx, req.Name)
		for _, roleCode := range req.RoleCodes {
			_ = s.permRepo.Create(ctx, &model.SysUserPermissionRelDO{
				UserType:   "USER",
				UserKey:    req.Name,
				TargetType: "ROLE",
				TargetCode: roleCode,
			})
		}
	}
	if req.NodeIDs != nil {
		_ = s.nodeRepo.DeleteByUserID(ctx, req.Name)
		for _, nodeID := range req.NodeIDs {
			_ = s.nodeRepo.Create(ctx, &model.SysUserNodeRelDO{
				UserID: req.Name,
				NodeID: nodeID,
			})
		}
	}

	return nil
}

// DeleteUser deletes a user account and all associated permissions and node
// relationships atomically within a database transaction.
func (s *UserService) DeleteUser(ctx context.Context, name string) error {
	user, err := s.userRepo.FindByName(ctx, name)
	if err != nil {
		return ErrUserNotFound
	}

	// Use transaction when db is available to ensure atomicity
	if s.db != nil {
		return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := tx.Where("user_key = ?", name).Delete(&model.SysUserPermissionRelDO{}).Error; err != nil {
				return err
			}
			if err := tx.Where("user_id = ?", name).Delete(&model.SysUserNodeRelDO{}).Error; err != nil {
				return err
			}
			return tx.Delete(&model.UserAccountsDO{}, user.ID).Error
		})
	}

	// Fallback without transaction (when db is nil, e.g. in unit tests)
	_ = s.permRepo.DeleteByUserKey(ctx, name)
	_ = s.nodeRepo.DeleteByUserID(ctx, name)
	return s.userRepo.Delete(ctx, user.ID)
}

// ResetPassword resets a user's password.
func (s *UserService) ResetPassword(ctx context.Context, req *ResetPasswordRequest) error {
	_, err := s.userRepo.FindByName(ctx, req.Name)
	if err != nil {
		return ErrUserNotFound
	}

	return s.userRepo.UpdatePassword(ctx, req.Name, HashPassword(req.NewPassword))
}

// GetUser retrieves a user by name.
func (s *UserService) GetUser(ctx context.Context, name string) (*UserVO, error) {
	user, err := s.userRepo.FindByName(ctx, name)
	if err != nil {
		return nil, ErrUserNotFound
	}
	return s.toUserVO(ctx, user), nil
}

// UpdatePasswordRequest represents a password update request (self-service).
type UpdatePasswordRequest struct {
	Name        string `json:"name" binding:"required"`
	OldPassword string `json:"old_password" binding:"required"`
	NewPassword string `json:"new_password" binding:"required"`
}

// UpdatePassword updates a user's password with old password verification.
func (s *UserService) UpdatePassword(ctx context.Context, req *UpdatePasswordRequest) error {
	user, err := s.userRepo.FindByName(ctx, req.Name)
	if err != nil {
		return ErrUserNotFound
	}

	// Verify old password using constant-time comparison to prevent timing attacks
	if !hmac.Equal([]byte(HashPassword(req.OldPassword)), []byte(user.PasswordHash)) {
		return errors.New("old password incorrect")
	}

	return s.userRepo.UpdatePassword(ctx, req.Name, HashPassword(req.NewPassword))
}

func (s *UserService) toUserVO(ctx context.Context, user *model.UserAccountsDO) *UserVO {
	vo := &UserVO{
		ID:        user.ID,
		Name:      user.Name,
		OwnerType: user.OwnerType,
		OwnerID:   user.OwnerID,
		GmtCreate: user.GmtCreate.Format("2006-01-02 15:04:05"),
	}

	// Get roles
	perms, err := s.permRepo.FindByUserKey(ctx, user.Name)
	if err == nil {
		roles := make([]string, 0, len(perms))
		for _, p := range perms {
			roles = append(roles, p.TargetCode)
		}
		vo.RoleCodes = roles
	}

	// Get nodes
	nodes, err := s.nodeRepo.FindByUserID(ctx, user.Name)
	if err == nil {
		nodeIDs := make([]string, 0, len(nodes))
		for _, n := range nodes {
			nodeIDs = append(nodeIDs, n.NodeID)
		}
		vo.NodeIDs = nodeIDs
	}

	return vo
}
