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
//   - Each token carries a unique JTI (JWT ID); the refresh token JTI is stored
//     in the session row so refresh tokens are one-time-use (rotation + reuse
//     detection, see service.AuthService.RefreshToken).
//   - Tokens carry a "kid" header derived from the signing secret. Several
//     verification keys may be configured (auth.jwt_previous_secrets) so the
//     signing secret can be rotated without logging everybody out.
//   - The "pv" claim is the account password version: a password change bumps
//     it and invalidates every token issued before.
package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"sync"
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
	// SessionID ties an access/refresh pair to one user_tokens row so that
	// logout (row deletion) also invalidates the refresh token.
	SessionID string `json:"sid,omitempty"`
	// PwdVer is the account password version at issue time (see
	// user_accounts.password_version); a password change invalidates tokens.
	PwdVer int64 `json:"pv,omitempty"`
}

// TokenPair contains both access and refresh tokens.
type TokenPair struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
	TokenType    string    `json:"token_type"`
	// RefreshJTI / RefreshExpiresAt describe the refresh token (server side
	// only: stored in the session row for one-time-use rotation).
	RefreshJTI       string    `json:"-"`
	RefreshExpiresAt time.Time `json:"-"`
}

// JWTManager handles JWT token generation and validation.
// It is safe for concurrent use by multiple goroutines.
type JWTManager struct {
	mu                 sync.RWMutex
	signKID            string            // kid of the active signing key
	keys               map[string][]byte // kid -> HMAC-SHA256 key (signing + previous)
	accessTokenExpiry  time.Duration     // TTL for access tokens (default 2h)
	refreshTokenExpiry time.Duration     // TTL for refresh tokens (default 7d)
}

// KeyID derives the public key id ("kid") of an HMAC secret. It is a
// truncated SHA-256 over a domain-separated secret: stable across restarts,
// no configuration needed, and it does not reveal the secret.
func KeyID(secret string) string {
	h := sha256.Sum256([]byte("privahub-jwt-kid:v1:" + secret))
	return hex.EncodeToString(h[:8])
}

// NewJWTManager creates a new JWT manager signing with secret.
func NewJWTManager(secret string, accessExpiry, refreshExpiry time.Duration) *JWTManager {
	kid := KeyID(secret)
	return &JWTManager{
		signKID:            kid,
		keys:               map[string][]byte{kid: []byte(secret)},
		accessTokenExpiry:  accessExpiry,
		refreshTokenExpiry: refreshExpiry,
	}
}

// AddVerificationKeys registers previous signing secrets that are still
// accepted for verification (key rotation). Empty secrets are ignored.
func (m *JWTManager) AddVerificationKeys(secrets ...string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, s := range secrets {
		if s = strings.TrimSpace(s); s != "" {
			m.keys[KeyID(s)] = []byte(s)
		}
	}
}

// SigningKeyID returns the kid of the active signing key.
func (m *JWTManager) SigningKeyID() string { return m.signKID }

// GenerateTokenPair generates both access and refresh tokens for a user.
func (m *JWTManager) GenerateTokenPair(userID, username, ownerType, ownerID string) (*TokenPair, error) {
	return m.GenerateTokenPairWithSession(userID, username, ownerType, ownerID, "")
}

// GenerateTokenPairWithSession generates a token pair bound to a session ID.
func (m *JWTManager) GenerateTokenPairWithSession(userID, username, ownerType, ownerID, sessionID string) (*TokenPair, error) {
	return m.GenerateSessionTokens(SessionTokenParams{
		UserID: userID, Username: username, OwnerType: ownerType, OwnerID: ownerID, SessionID: sessionID,
	})
}

// SessionTokenParams are the identity fields embedded in a session token pair.
type SessionTokenParams struct {
	UserID, Username, OwnerType, OwnerID, SessionID string
	PwdVer                                          int64
	// RefreshExpiresAt caps the refresh token expiry (absolute session
	// lifetime across rotations); zero means now + refresh expiry.
	RefreshExpiresAt time.Time
}

// GenerateSessionTokens issues an access + refresh token pair.
func (m *JWTManager) GenerateSessionTokens(p SessionTokenParams) (*TokenPair, error) {
	now := time.Now()
	accessExpiresAt := now.Add(m.accessTokenExpiry)
	refreshExpiresAt := now.Add(m.refreshTokenExpiry)
	if !p.RefreshExpiresAt.IsZero() && p.RefreshExpiresAt.Before(refreshExpiresAt) {
		refreshExpiresAt = p.RefreshExpiresAt
	}
	if accessExpiresAt.After(refreshExpiresAt) {
		accessExpiresAt = refreshExpiresAt
	}

	accessToken, _, err := m.generateToken(p, AccessToken, now, accessExpiresAt)
	if err != nil {
		return nil, err
	}
	refreshToken, refreshJTI, err := m.generateToken(p, RefreshToken, now, refreshExpiresAt)
	if err != nil {
		return nil, err
	}

	return &TokenPair{
		AccessToken:      accessToken,
		RefreshToken:     refreshToken,
		ExpiresAt:        accessExpiresAt,
		TokenType:        "Bearer",
		RefreshJTI:       refreshJTI,
		RefreshExpiresAt: refreshExpiresAt,
	}, nil
}

// generateToken creates a single JWT token and returns it with its JTI.
func (m *JWTManager) generateToken(p SessionTokenParams, tokenType TokenType, now, expiresAt time.Time) (string, string, error) {
	jti := uuid.New().String()
	claims := &Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        jti,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			Issuer:    "secretpad-go",
		},
		UserID:    p.UserID,
		Username:  p.Username,
		OwnerType: p.OwnerType,
		OwnerID:   p.OwnerID,
		TokenType: tokenType,
		SessionID: p.SessionID,
		PwdVer:    p.PwdVer,
	}

	m.mu.RLock()
	kid, key := m.signKID, m.keys[m.signKID]
	m.mu.RUnlock()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	token.Header["kid"] = kid
	s, err := token.SignedString(key)
	return s, jti, err
}

// ValidateToken validates a JWT token string and returns its claims.
// It verifies the HMAC signature, checks expiration, and ensures the
// signing method is HMAC (preventing algorithm confusion attacks).
// Returns ErrExpiredToken for expired tokens, ErrInvalidToken for
// malformed/tampered tokens, and ErrInvalidClaims for type assertion failures.
func (m *JWTManager) ValidateToken(tokenString string) (*Claims, error) {
	claims, err := m.parse(tokenString)
	if err != nil {
		return nil, err
	}
	return claims, nil
}

// parse verifies the token with the key selected by its "kid" header. Tokens
// issued before kid support (no header) are tried against every configured key.
func (m *JWTManager) parse(tokenString string) (*Claims, error) {
	m.mu.RLock()
	candidates := make([][]byte, 0, len(m.keys))
	kid := unverifiedKID(tokenString)
	if kid != "" {
		if k, ok := m.keys[kid]; ok {
			candidates = append(candidates, k)
		}
	} else {
		candidates = append(candidates, m.keys[m.signKID])
		for id, k := range m.keys {
			if id != m.signKID {
				candidates = append(candidates, k)
			}
		}
	}
	m.mu.RUnlock()
	if len(candidates) == 0 {
		return nil, ErrInvalidToken // unknown kid (retired key)
	}

	var token *jwt.Token
	var err error
	for _, key := range candidates {
		key := key
		token, err = jwt.ParseWithClaims(tokenString, &Claims{}, func(t *jwt.Token) (interface{}, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, ErrInvalidToken
			}
			return key, nil
		}, jwt.WithValidMethods([]string{"HS256", "HS384", "HS512"}))
		if err == nil || !errors.Is(err, jwt.ErrTokenSignatureInvalid) {
			break
		}
	}
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

// unverifiedKID returns the kid header of a token without verifying it.
func unverifiedKID(tokenString string) string {
	t, _, err := jwt.NewParser().ParseUnverified(tokenString, &Claims{})
	if err != nil {
		return ""
	}
	kid, _ := t.Header["kid"].(string)
	return kid
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

	return m.GenerateSessionTokens(SessionTokenParams{
		UserID: claims.UserID, Username: claims.Username, OwnerType: claims.OwnerType,
		OwnerID: claims.OwnerID, SessionID: claims.SessionID, PwdVer: claims.PwdVer,
		RefreshExpiresAt: claims.ExpiresAt.Time,
	})
}

// AccessTokenExpiry returns the configured access token lifetime.
func (m *JWTManager) AccessTokenExpiry() time.Duration { return m.accessTokenExpiry }

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
