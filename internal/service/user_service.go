package service

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/internal/dao/repository"
	"github.com/fengzhizi319/privahub/pkg/errcode"
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

	// updatePwd lockout policy (Java secretpad.reset-password-error-*).
	resetMaxAttempts int
	resetLock        time.Duration
	now              func() time.Time
}

// SetResetPasswordPolicy configures the updatePwd failure lockout.
func (s *UserService) SetResetPasswordPolicy(maxAttempts int, lock time.Duration) {
	if maxAttempts > 0 {
		s.resetMaxAttempts = maxAttempts
	}
	if lock > 0 {
		s.resetLock = lock
	}
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

		resetMaxAttempts: 5,
		resetLock:        60 * time.Minute,
		now:              time.Now,
	}
}

// --- Request / Response DTOs ---

// CreateUserRequest represents a user creation request. PasswordHash (SHA-256
// hex, preferred) or Password (plaintext, hashed server side) is required.
type CreateUserRequest struct {
	Name         string   `json:"name" binding:"required"`
	Password     string   `json:"password"`
	PasswordHash string   `json:"passwordHash"`
	OwnerType    string   `json:"owner_type"`
	OwnerID      string   `json:"owner_id"`
	RoleCodes    []string `json:"role_codes"`
	NodeIDs      []string `json:"node_ids"`
}

// UpdateUserRequest represents a user update request.
type UpdateUserRequest struct {
	Name      string   `json:"name" binding:"required"`
	OwnerType string   `json:"owner_type"`
	OwnerID   string   `json:"owner_id"`
	RoleCodes []string `json:"role_codes"`
	NodeIDs   []string `json:"node_ids"`
}

// ResetPasswordRequest represents an admin password reset request.
// NewPasswordHash (SHA-256 hex) is preferred; NewPassword is hashed server side.
type ResetPasswordRequest struct {
	Name            string `json:"name" binding:"required"`
	NewPassword     string `json:"new_password"`
	NewPasswordHash string `json:"newPasswordHash"`
}

// clientPasswordHash returns the stored-form hash from a client hash (SHA-256
// hex) or, failing that, hashes a plaintext password.
func clientPasswordHash(hash, plain string) string {
	if hexHash.MatchString(hash) {
		return strings.ToLower(hash)
	}
	if plain != "" {
		return HashPassword(plain)
	}
	return ""
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
	passwordHash := clientPasswordHash(req.PasswordHash, req.Password)
	if passwordHash == "" {
		return nil, ErrPasswordRequired
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
		PasswordHash: passwordHash,
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
	h := clientPasswordHash(req.NewPasswordHash, req.NewPassword)
	if h == "" {
		return ErrPasswordRequired
	}
	if err := s.userRepo.UpdatePassword(ctx, req.Name, h); err != nil {
		return err
	}
	return ClearUserSessions(ctx, s.db, req.Name)
}

// GetUser retrieves a user by name.
func (s *UserService) GetUser(ctx context.Context, name string) (*UserVO, error) {
	user, err := s.userRepo.FindByName(ctx, name)
	if err != nil {
		return nil, ErrUserNotFound
	}
	return s.toUserVO(ctx, user), nil
}

// UpdatePasswordRequest is the Java UserUpdatePwdRequest. Name is always
// taken from the session by the handler. OldPasswordHashSm3 is optional and
// only used for legacy (Java, SM3-hashed) accounts.
type UpdatePasswordRequest struct {
	Name                string `json:"name"`
	OldPasswordHash     string `json:"oldPasswordHash"`
	NewPasswordHash     string `json:"newPasswordHash"`
	ConfirmPasswordHash string `json:"confirmPasswordHash"`
	OldPasswordHashSm3  string `json:"oldPasswordHashSm3"`
}

// UpdatePassword implements Java UserServiceImpl.updatePwd: reset-lock check,
// new==confirm, new!=old, old hash verification with failure lockout, and on
// success clears all sessions of the user. Errors are *AuthError.
func (s *UserService) UpdatePassword(ctx context.Context, req *UpdatePasswordRequest) error {
	if req.OldPasswordHash == "" || req.NewPasswordHash == "" || req.ConfirmPasswordHash == "" {
		return newAuthError(ErrPasswordRequired, errcode.SystemValidationError, "oldPasswordHash/newPasswordHash/confirmPasswordHash required")
	}
	user, err := s.userRepo.FindByName(ctx, req.Name)
	if err != nil || user == nil {
		return newAuthError(ErrUserNotFound, errcode.AuthUserNotFound, "0")
	}
	now := s.now()
	if user.GmtPasswdResetRelease != nil {
		if user.GmtPasswdResetRelease.After(now) {
			if user.PasswdResetFailedAttempts != nil && *user.PasswdResetFailedAttempts >= s.resetMaxAttempts {
				return newAuthError(ErrUserLocked, errcode.AuthUserIsLocked, minutesLeft(*user.GmtPasswdResetRelease, now))
			}
		} else {
			// Lock expired: release it and start counting again.
			user.GmtPasswdResetRelease = nil
			user.PasswdResetFailedAttempts = nil
			if err := s.userRepo.Update(ctx, user); err != nil {
				return err
			}
		}
	}
	if req.NewPasswordHash != req.ConfirmPasswordHash {
		return newAuthError(ErrInvalidPassword, errcode.UserUpdatePasswordErrorInconsistent)
	}
	if req.OldPasswordHash == req.NewPasswordHash {
		return newAuthError(ErrInvalidPassword, errcode.UserUpdatePasswordErrorSame)
	}
	stored := strings.ToLower(user.PasswordHash)
	if constEq(strings.ToLower(req.OldPasswordHash), stored) ||
		(req.OldPasswordHashSm3 != "" && constEq(strings.ToLower(req.OldPasswordHashSm3), stored)) {
		user.GmtPasswdResetRelease = nil
		user.PasswdResetFailedAttempts = nil
		user.PasswordHash = strings.ToLower(req.NewPasswordHash)
		if err := s.userRepo.Update(ctx, user); err != nil {
			return err
		}
		return ClearUserSessions(ctx, s.db, user.Name)
	}

	attempts := 1
	if user.PasswdResetFailedAttempts != nil {
		attempts = *user.PasswdResetFailedAttempts + 1
	}
	user.PasswdResetFailedAttempts = &attempts
	if attempts >= s.resetMaxAttempts {
		release := now.Add(s.resetLock)
		user.GmtPasswdResetRelease = &release
		_ = s.userRepo.Update(ctx, user)
		return newAuthError(ErrUserLocked, errcode.AuthResetPasswordIsLocked, strconv.Itoa(int(s.resetLock.Minutes())))
	}
	_ = s.userRepo.Update(ctx, user)
	return newAuthError(ErrInvalidPassword, errcode.AuthUserPasswordError, strconv.Itoa(s.resetMaxAttempts-attempts))
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
