// Package auth provides JWT-based authentication and authorization services.
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

// Claims represents the JWT claims for SecretPad-Go.
type Claims struct {
	jwt.RegisteredClaims
	UserID    string    `json:"user_id"`
	Username  string    `json:"username"`
	OwnerType string    `json:"owner_type"`
	OwnerID   string    `json:"owner_id"`
	TokenType TokenType `json:"token_type"`
}

// TokenPair contains both access and refresh tokens.
type TokenPair struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
	TokenType    string    `json:"token_type"`
}

// JWTManager handles JWT token generation and validation.
type JWTManager struct {
	secretKey          []byte
	accessTokenExpiry  time.Duration
	refreshTokenExpiry time.Duration
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

// ValidateToken validates a JWT token and returns its claims.
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

// RefreshAccessToken validates a refresh token and generates a new access token.
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
