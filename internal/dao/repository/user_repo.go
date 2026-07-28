package repository

import (
	"context"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"gorm.io/gorm"
)

// UserAccountsRepo is the GORM implementation of UserAccountsRepository.
type UserAccountsRepo struct {
	*BaseRepo[model.UserAccountsDO]
}

// NewUserAccountsRepo creates a new UserAccountsRepo.
func NewUserAccountsRepo(db *gorm.DB) *UserAccountsRepo {
	return &UserAccountsRepo{BaseRepo: NewBaseRepo[model.UserAccountsDO](db)}
}

// FindByName retrieves a user account by username.
func (r *UserAccountsRepo) FindByName(ctx context.Context, name string) (*model.UserAccountsDO, error) {
	var user model.UserAccountsDO
	err := r.DB().WithContext(ctx).Where("name = ?", name).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// UpdatePassword updates a user's password hash.
func (r *UserAccountsRepo) UpdatePassword(ctx context.Context, name, passwordHash string) error {
	return r.DB().WithContext(ctx).Model(&model.UserAccountsDO{}).
		Where("name = ?", name).
		Update("password_hash", passwordHash).Error
}

// UserTokensRepo is the GORM implementation of UserTokensRepository.
type UserTokensRepo struct {
	*BaseRepo[model.UserTokensDO]
}

// NewUserTokensRepo creates a new UserTokensRepo.
func NewUserTokensRepo(db *gorm.DB) *UserTokensRepo {
	return &UserTokensRepo{BaseRepo: NewBaseRepo[model.UserTokensDO](db)}
}

// FindByToken retrieves a token record by token value.
func (r *UserTokensRepo) FindByToken(ctx context.Context, token string) (*model.UserTokensDO, error) {
	var t model.UserTokensDO
	err := r.DB().WithContext(ctx).Where("token = ?", token).First(&t).Error
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// FindByName retrieves all tokens for a user.
func (r *UserTokensRepo) FindByName(ctx context.Context, name string) ([]model.UserTokensDO, error) {
	var tokens []model.UserTokensDO
	err := r.DB().WithContext(ctx).Where("name = ?", name).Find(&tokens).Error
	return tokens, err
}

// DeleteByName deletes all tokens for a user (logout).
func (r *UserTokensRepo) DeleteByName(ctx context.Context, name string) error {
	return r.DB().WithContext(ctx).Where("name = ?", name).Delete(&model.UserTokensDO{}).Error
}

// SysUserPermissionRepo is the GORM implementation of SysUserPermissionRepository.
type SysUserPermissionRepo struct {
	db *gorm.DB
}

// NewSysUserPermissionRepo creates a new SysUserPermissionRepo.
func NewSysUserPermissionRepo(db *gorm.DB) *SysUserPermissionRepo {
	return &SysUserPermissionRepo{db: db}
}

// FindByUserKey retrieves all permission relations for a user.
func (r *SysUserPermissionRepo) FindByUserKey(ctx context.Context, userKey string) ([]model.SysUserPermissionRelDO, error) {
	var rels []model.SysUserPermissionRelDO
	err := r.db.WithContext(ctx).Where("user_key = ?", userKey).Find(&rels).Error
	return rels, err
}

// Create inserts a new permission relation.
func (r *SysUserPermissionRepo) Create(ctx context.Context, rel *model.SysUserPermissionRelDO) error {
	return r.db.WithContext(ctx).Create(rel).Error
}

// DeleteByUserKey deletes all permission relations for a user.
func (r *SysUserPermissionRepo) DeleteByUserKey(ctx context.Context, userKey string) error {
	return r.db.WithContext(ctx).Where("user_key = ?", userKey).Delete(&model.SysUserPermissionRelDO{}).Error
}

// SysUserNodeRepo is the GORM implementation of SysUserNodeRepository.
type SysUserNodeRepo struct {
	db *gorm.DB
}

// NewSysUserNodeRepo creates a new SysUserNodeRepo.
func NewSysUserNodeRepo(db *gorm.DB) *SysUserNodeRepo {
	return &SysUserNodeRepo{db: db}
}

// FindByUserID retrieves all node relations for a user.
func (r *SysUserNodeRepo) FindByUserID(ctx context.Context, userID string) ([]model.SysUserNodeRelDO, error) {
	var rels []model.SysUserNodeRelDO
	err := r.db.WithContext(ctx).Where("user_id = ?", userID).Find(&rels).Error
	return rels, err
}

// Create inserts a new user-node relation.
func (r *SysUserNodeRepo) Create(ctx context.Context, rel *model.SysUserNodeRelDO) error {
	return r.db.WithContext(ctx).Create(rel).Error
}

// DeleteByUserID deletes all node relations for a user.
func (r *SysUserNodeRepo) DeleteByUserID(ctx context.Context, userID string) error {
	return r.db.WithContext(ctx).Where("user_id = ?", userID).Delete(&model.SysUserNodeRelDO{}).Error
}
