// Package auth provides JWT-based authentication and authorization services
// for the Privahub platform.
//
// Architecture:
//   - JWTManager handles token generation, validation, and refresh using HMAC-SHA256.
//   - TokenPair implements the access/refresh token pattern: short-lived access tokens
//     (default 2h) for API calls, long-lived refresh tokens (default 7d) for renewal.
//   - Context helpers propagate authenticated user identity through the request chain.
//
// Security considerations:
//   - The signing secret must be configured via auth.jwt_secret in production.
//   - Token type (access vs refresh) is embedded in claims to prevent token misuse.
//   - Each token carries a unique JTI (JWT ID) for potential revocation tracking.
package auth

import (
	"context"
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// Common errors for authentication.
var (
	ErrInvalidToken  = errors.New("invalid token")
	ErrExpiredToken  = errors.New("token expired")
	ErrInvalidClaims = errors.New("invalid token claims")
)

// TokenType represents the type of JWT token.
type TokenType string

const (
	// AccessToken is a short-lived token for API access.
	AccessToken TokenType = "access"
	// RefreshToken is a long-lived token for obtaining new access tokens.
	RefreshToken TokenType = "refresh"
)

// Claims represents the JWT claims for Privahub.
// It extends the standard RegisteredClaims with user identity fields
// required by the SecretPad-compatible API contract.
type Claims struct {
	jwt.RegisteredClaims
	UserID    string    `json:"user_id"`    // Numeric user ID as string
	Username  string    `json:"username"`   // Login name (e.g. "admin")
	OwnerType string    `json:"owner_type"` // CENTER | EDGE | PARTNER
	OwnerID   string    `json:"owner_id"`   // Owning node/institution ID
	TokenType TokenType `json:"token_type"` // access | refresh
}

// TokenPair contains both access and refresh tokens.
type TokenPair struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
	TokenType    string    `json:"token_type"`
}

// JWTManager handles JWT token generation and validation.
// It is safe for concurrent use by multiple goroutines.
type JWTManager struct {
	secretKey          []byte        // HMAC-SHA256 signing key
	accessTokenExpiry  time.Duration // TTL for access tokens (default 2h)
	refreshTokenExpiry time.Duration // TTL for refresh tokens (default 7d)
}

// NewJWTManager creates a new JWT manager.
func NewJWTManager(secret string, accessExpiry, refreshExpiry time.Duration) *JWTManager {
	return &JWTManager{
		secretKey:          []byte(secret),
		accessTokenExpiry:  accessExpiry,
		refreshTokenExpiry: refreshExpiry,
	}
}

// GenerateTokenPair generates both access and refresh tokens for a user.
func (m *JWTManager) GenerateTokenPair(userID, username, ownerType, ownerID string) (*TokenPair, error) {
	now := time.Now()
	accessExpiresAt := now.Add(m.accessTokenExpiry)

	// Generate access token
	accessToken, err := m.generateToken(userID, username, ownerType, ownerID, AccessToken, accessExpiresAt)
	if err != nil {
		return nil, err
	}

	// Generate refresh token
	refreshToken, err := m.generateToken(userID, username, ownerType, ownerID, RefreshToken, now.Add(m.refreshTokenExpiry))
	if err != nil {
		return nil, err
	}

	return &TokenPair{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresAt:    accessExpiresAt,
		TokenType:    "Bearer",
	}, nil
}

// generateToken creates a single JWT token.
func (m *JWTManager) generateToken(userID, username, ownerType, ownerID string, tokenType TokenType, expiresAt time.Time) (string, error) {
	claims := &Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        uuid.New().String(),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			Issuer:    "secretpad-go",
		},
		UserID:    userID,
		Username:  username,
		OwnerType: ownerType,
		OwnerID:   ownerID,
		TokenType: tokenType,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(m.secretKey)
}

// ValidateToken validates a JWT token string and returns its claims.
// It verifies the HMAC signature, checks expiration, and ensures the
// signing method is HMAC (preventing algorithm confusion attacks).
// Returns ErrExpiredToken for expired tokens, ErrInvalidToken for
// malformed/tampered tokens, and ErrInvalidClaims for type assertion failures.
func (m *JWTManager) ValidateToken(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrInvalidToken
		}
		return m.secretKey, nil
	})

	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrExpiredToken
		}
		return nil, ErrInvalidToken
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, ErrInvalidClaims
	}

	return claims, nil
}

// RefreshAccessToken validates a refresh token and generates a new token pair.
// It enforces that the provided token is actually a refresh token (not an access
// token) to prevent privilege escalation via token type confusion.
func (m *JWTManager) RefreshAccessToken(refreshTokenString string) (*TokenPair, error) {
	claims, err := m.ValidateToken(refreshTokenString)
	if err != nil {
		return nil, err
	}

	if claims.TokenType != RefreshToken {
		return nil, ErrInvalidToken
	}

	return m.GenerateTokenPair(claims.UserID, claims.Username, claims.OwnerType, claims.OwnerID)
}

// Context keys for storing auth information.
type contextKey string

const (
	// ClaimsContextKey is the context key for JWT claims.
	ClaimsContextKey contextKey = "claims"
	// UserIDContextKey is the context key for user ID.
	UserIDContextKey contextKey = "user_id"
	// UsernameContextKey is the context key for username.
	UsernameContextKey contextKey = "username"
)

// ContextWithClaims adds claims to the context.
func ContextWithClaims(ctx context.Context, claims *Claims) context.Context {
	ctx = context.WithValue(ctx, ClaimsContextKey, claims)
	ctx = context.WithValue(ctx, UserIDContextKey, claims.UserID)
	ctx = context.WithValue(ctx, UsernameContextKey, claims.Username)
	return ctx
}

// ClaimsFromContext retrieves claims from the context.
func ClaimsFromContext(ctx context.Context) (*Claims, bool) {
	claims, ok := ctx.Value(ClaimsContextKey).(*Claims)
	return claims, ok
}

// UserIDFromContext retrieves user ID from the context.
func UserIDFromContext(ctx context.Context) (string, bool) {
	userID, ok := ctx.Value(UserIDContextKey).(string)
	return userID, ok
}

// UsernameFromContext retrieves username from the context.
func UsernameFromContext(ctx context.Context) (string, bool) {
	username, ok := ctx.Value(UsernameContextKey).(string)
	return username, ok
}
