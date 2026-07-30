// Package service provides business logic services for Privahub.
//
// This package implements the core domain logic including:
//   - Authentication & session management (AuthService)
//   - Project/graph/job lifecycle management
//   - Node and route management with Kuscia integration
//   - Datatable and datasource operations
//   - Vote/approval workflows for cross-institution collaboration
//   - Background sync services for Kuscia state reconciliation
//
// All services follow the constructor injection pattern and accept
// repository interfaces for testability.
package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"
	"time"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/internal/dao/repository"
	"github.com/fengzhizi319/privahub/pkg/auth"
)

// Common errors for auth service.
var (
	ErrUserNotFound    = errors.New("user not found")
	ErrInvalidPassword = errors.New("invalid password")
	ErrUserLocked      = errors.New("user account locked")
	ErrTokenNotFound   = errors.New("token not found")
)

// AuthService handles user authentication and session management.
// It implements the Java SecretPad authentication contract:
//   - Password verification via SHA-256 hash comparison
//   - Account lockout after 5 failed attempts (30-minute cooldown)
//   - JWT token pair generation with database-backed session tracking
//   - Token refresh flow for seamless session renewal
type AuthService struct {
	userRepo   repository.UserAccountsRepository
	tokenRepo  repository.UserTokensRepository
	jwtManager *auth.JWTManager
}

// NewAuthService creates a new AuthService.
func NewAuthService(
	userRepo repository.UserAccountsRepository,
	tokenRepo repository.UserTokensRepository,
	jwtManager *auth.JWTManager,
) *AuthService {
	return &AuthService{
		userRepo:   userRepo,
		tokenRepo:  tokenRepo,
		jwtManager: jwtManager,
	}
}

// LoginRequest represents a login request.
type LoginRequest struct {
	Username string `json:"name" binding:"required"`
	Password string `json:"password"`
	// PasswordHash is the SHA-256 hex of the password, as sent by the
	// frontend (Java SecretPad contract). Either Password or PasswordHash
	// must be present.
	PasswordHash string `json:"passwordHash"`
}

// LoginResponse represents a login response.
type LoginResponse struct {
	*auth.TokenPair
	User *UserInfo `json:"user"`
}

// UserInfo represents basic user information.
type UserInfo struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	OwnerType string `json:"owner_type"`
	OwnerID   string `json:"owner_id"`
}

// Login authenticates a user and returns a token pair.
func (s *AuthService) Login(ctx context.Context, req *LoginRequest) (*LoginResponse, error) {
	// Find user
	user, err := s.userRepo.FindByName(ctx, req.Username)
	if err != nil {
		return nil, ErrUserNotFound
	}

	// Check if account is locked
	if user.LockedInvalidTime != nil && user.LockedInvalidTime.After(time.Now()) {
		return nil, ErrUserLocked
	}

	// Verify password: accept either the plain password or its SHA-256 hex
	// (the frontend sends the hash per the Java SecretPad contract).
	password := req.Password
	if password == "" {
		password = req.PasswordHash
	}
	if password == "" || !s.verifyPassword(password, user.PasswordHash) {
		// Increment failed attempts
		s.incrementFailedAttempts(ctx, user)
		return nil, ErrInvalidPassword
	}

	// Reset failed attempts on successful login
	s.resetFailedAttempts(ctx, user)

	// Generate JWT token pair
	tokenPair, err := s.jwtManager.GenerateTokenPair(
		strconv.FormatInt(user.ID, 10),
		user.Name,
		user.OwnerType,
		user.OwnerID,
	)
	if err != nil {
		return nil, err
	}

	// Store token in database for session management
	s.storeToken(ctx, user.Name, tokenPair.AccessToken)

	return &LoginResponse{
		TokenPair: tokenPair,
		User: &UserInfo{
			ID:        user.ID,
			Name:      user.Name,
			OwnerType: user.OwnerType,
			OwnerID:   user.OwnerID,
		},
	}, nil
}

// Logout invalidates a user's session.
func (s *AuthService) Logout(ctx context.Context, username string) error {
	return s.tokenRepo.DeleteByName(ctx, username)
}

// RefreshToken refreshes an access token using a refresh token.
func (s *AuthService) RefreshToken(ctx context.Context, refreshToken string) (*auth.TokenPair, error) {
	return s.jwtManager.RefreshAccessToken(refreshToken)
}

// ValidateSession validates a token against stored sessions.
func (s *AuthService) ValidateSession(ctx context.Context, token string) (*model.UserTokensDO, error) {
	return s.tokenRepo.FindByToken(ctx, token)
}

// verifyPassword checks if the plain password matches the stored hash.
// It supports two verification paths for backward compatibility:
//  1. Direct comparison: the frontend may send the pre-hashed value
//     (Java SecretPad contract sends SHA-256 hex from the client).
//  2. Hash-then-compare: if the raw password is sent, compute its
//     SHA-256 and compare against the stored hash.
//
// Uses hmac.Equal for constant-time comparison to prevent timing attacks.
func (s *AuthService) verifyPassword(plainPassword, hashedPassword string) bool {
	// Constant-time comparison for the direct (pre-hashed) path
	if hmac.Equal([]byte(plainPassword), []byte(hashedPassword)) {
		return true
	}
	// Java backend uses SHA-256 for password hashing
	hash := sha256.Sum256([]byte(plainPassword))
	computedHash := hex.EncodeToString(hash[:])
	return hmac.Equal([]byte(computedHash), []byte(hashedPassword))
}

// HashPassword creates a SHA-256 hash of a password.
func HashPassword(password string) string {
	hash := sha256.Sum256([]byte(password))
	return hex.EncodeToString(hash[:])
}

// storeToken saves a token to the database.
func (s *AuthService) storeToken(ctx context.Context, username, token string) {
	now := time.Now()
	tokenDO := &model.UserTokensDO{
		Name:     username,
		Token:    token,
		GmtToken: &now,
	}
	// Ignore error - token storage is best-effort
	_ = s.tokenRepo.Create(ctx, tokenDO)
}

// incrementFailedAttempts increments the failed login counter.
func (s *AuthService) incrementFailedAttempts(ctx context.Context, user *model.UserAccountsDO) {
	attempts := 0
	if user.FailedAttempts != nil {
		attempts = *user.FailedAttempts
	}
	attempts++
	user.FailedAttempts = &attempts

	// Lock account after 5 failed attempts
	if attempts >= 5 {
		lockTime := time.Now().Add(30 * time.Minute)
		user.LockedInvalidTime = &lockTime
	}

	_ = s.userRepo.Update(ctx, user)
}

// resetFailedAttempts resets the failed login counter.
func (s *AuthService) resetFailedAttempts(ctx context.Context, user *model.UserAccountsDO) {
	if user.FailedAttempts != nil && *user.FailedAttempts > 0 {
		zero := 0
		user.FailedAttempts = &zero
		user.LockedInvalidTime = nil
		_ = s.userRepo.Update(ctx, user)
	}
}
