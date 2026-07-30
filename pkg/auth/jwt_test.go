package auth

import (
	"context"
	"testing"
	"time"
)

func TestJWTManager_GenerateTokenPair(t *testing.T) {
	m := NewJWTManager("test-secret-key", 1*time.Hour, 24*time.Hour)

	pair, err := m.GenerateTokenPair("123", "admin", "CENTER", "kuscia-system")
	if err != nil {
		t.Fatalf("GenerateTokenPair failed: %v", err)
	}

	if pair.AccessToken == "" {
		t.Error("expected non-empty access token")
	}
	if pair.RefreshToken == "" {
		t.Error("expected non-empty refresh token")
	}
	if pair.TokenType != "Bearer" {
		t.Errorf("expected token type 'Bearer', got %q", pair.TokenType)
	}
	if pair.ExpiresAt.Before(time.Now()) {
		t.Error("expected ExpiresAt to be in the future")
	}
}

func TestJWTManager_ValidateToken_Success(t *testing.T) {
	m := NewJWTManager("test-secret-key", 1*time.Hour, 24*time.Hour)

	pair, _ := m.GenerateTokenPair("123", "admin", "CENTER", "kuscia-system")

	claims, err := m.ValidateToken(pair.AccessToken)
	if err != nil {
		t.Fatalf("ValidateToken failed: %v", err)
	}

	if claims.UserID != "123" {
		t.Errorf("expected UserID '123', got %q", claims.UserID)
	}
	if claims.Username != "admin" {
		t.Errorf("expected Username 'admin', got %q", claims.Username)
	}
	if claims.OwnerType != "CENTER" {
		t.Errorf("expected OwnerType 'CENTER', got %q", claims.OwnerType)
	}
	if claims.TokenType != AccessToken {
		t.Errorf("expected TokenType 'access', got %q", claims.TokenType)
	}
}

func TestJWTManager_ValidateToken_Expired(t *testing.T) {
	// Create a manager with very short expiry
	m := NewJWTManager("test-secret-key", -1*time.Hour, 24*time.Hour)

	pair, _ := m.GenerateTokenPair("123", "admin", "CENTER", "kuscia-system")

	_, err := m.ValidateToken(pair.AccessToken)
	if err != ErrExpiredToken {
		t.Errorf("expected ErrExpiredToken, got %v", err)
	}
}

func TestJWTManager_ValidateToken_InvalidSignature(t *testing.T) {
	m1 := NewJWTManager("secret-1", 1*time.Hour, 24*time.Hour)
	m2 := NewJWTManager("secret-2", 1*time.Hour, 24*time.Hour)

	pair, _ := m1.GenerateTokenPair("123", "admin", "CENTER", "kuscia-system")

	// Validate with a different secret should fail
	_, err := m2.ValidateToken(pair.AccessToken)
	if err != ErrInvalidToken {
		t.Errorf("expected ErrInvalidToken, got %v", err)
	}
}

func TestJWTManager_ValidateToken_MalformedToken(t *testing.T) {
	m := NewJWTManager("test-secret-key", 1*time.Hour, 24*time.Hour)

	_, err := m.ValidateToken("not-a-valid-jwt")
	if err != ErrInvalidToken {
		t.Errorf("expected ErrInvalidToken, got %v", err)
	}
}

func TestJWTManager_RefreshAccessToken_Success(t *testing.T) {
	m := NewJWTManager("test-secret-key", 1*time.Hour, 24*time.Hour)

	pair, _ := m.GenerateTokenPair("123", "admin", "CENTER", "kuscia-system")

	newPair, err := m.RefreshAccessToken(pair.RefreshToken)
	if err != nil {
		t.Fatalf("RefreshAccessToken failed: %v", err)
	}

	if newPair.AccessToken == "" {
		t.Error("expected non-empty new access token")
	}
	if newPair.RefreshToken == "" {
		t.Error("expected non-empty new refresh token")
	}
}

func TestJWTManager_RefreshAccessToken_WithAccessToken_ShouldFail(t *testing.T) {
	m := NewJWTManager("test-secret-key", 1*time.Hour, 24*time.Hour)

	pair, _ := m.GenerateTokenPair("123", "admin", "CENTER", "kuscia-system")

	// Using access token for refresh should fail (token type mismatch)
	_, err := m.RefreshAccessToken(pair.AccessToken)
	if err != ErrInvalidToken {
		t.Errorf("expected ErrInvalidToken when using access token for refresh, got %v", err)
	}
}

func TestContextHelpers(t *testing.T) {
	claims := &Claims{
		UserID:   "123",
		Username: "admin",
	}

	ctx := ContextWithClaims(context.Background(), claims)

	// Test ClaimsFromContext
	retrievedClaims, ok := ClaimsFromContext(ctx)
	if !ok {
		t.Fatal("expected claims in context")
	}
	if retrievedClaims.UserID != "123" {
		t.Errorf("expected UserID '123', got %q", retrievedClaims.UserID)
	}

	// Test UserIDFromContext
	userID, ok := UserIDFromContext(ctx)
	if !ok {
		t.Fatal("expected user_id in context")
	}
	if userID != "123" {
		t.Errorf("expected user_id '123', got %q", userID)
	}

	// Test UsernameFromContext
	username, ok := UsernameFromContext(ctx)
	if !ok {
		t.Fatal("expected username in context")
	}
	if username != "admin" {
		t.Errorf("expected username 'admin', got %q", username)
	}
}

func TestContextHelpers_Empty(t *testing.T) {
	ctx := context.Background()

	_, ok := ClaimsFromContext(ctx)
	if ok {
		t.Error("expected no claims in empty context")
	}

	_, ok = UserIDFromContext(ctx)
	if ok {
		t.Error("expected no user_id in empty context")
	}

	_, ok = UsernameFromContext(ctx)
	if ok {
		t.Error("expected no username in empty context")
	}
}
