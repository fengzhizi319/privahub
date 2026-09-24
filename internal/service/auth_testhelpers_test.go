package service

import (
	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/internal/dao/repository"
	"github.com/fengzhizi319/privahub/pkg/auth"
	"gorm.io/gorm"
)

// verifyPassword checks a login credential (hash, or plaintext in dev)
// against a stored hash through the login candidate rules.
func (s *AuthService) verifyPassword(password, stored string) bool {
	return auth.VerifyStoredPassword(stored, s.loginCandidates(&LoginRequest{Password: password})).OK
}

// storedMatches reports whether a stored (KDF) password hash verifies plain.
func storedMatches(stored, plain string) bool {
	return auth.VerifyStoredPassword(stored, auth.PasswordCandidates{SHA256: HashPassword(plain)}).OK
}

// newPwdTestUserServiceDB builds a UserService on an existing test DB.
func newPwdTestUserServiceDB(db *gorm.DB) (*UserService, *gorm.DB) {
	_ = db.AutoMigrate(&model.SysUserNodeRelDO{}, &model.SysUserPermissionRelDO{})
	return NewUserService(repository.NewUserAccountsRepo(db), repository.NewSysUserPermissionRepo(db),
		repository.NewSysUserNodeRepo(db), db), db
}
