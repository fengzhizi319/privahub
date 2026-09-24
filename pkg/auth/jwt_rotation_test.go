package auth

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestJWT_KidHeaderAndKeyRotation(t *testing.T) {
	old := NewJWTManager("old-secret-0123456789", time.Hour, 24*time.Hour)
	pair, err := old.GenerateSessionTokens(SessionTokenParams{UserID: "1", Username: "u", SessionID: "s", PwdVer: 3})
	if err != nil {
		t.Fatal(err)
	}
	tok, _, _ := jwt.NewParser().ParseUnverified(pair.AccessToken, &Claims{})
	if tok.Header["kid"] != KeyID("old-secret-0123456789") || old.SigningKeyID() != KeyID("old-secret-0123456789") {
		t.Fatalf("kid header = %v", tok.Header["kid"])
	}

	// Rotate: new signing key, old one kept for verification.
	rotated := NewJWTManager("new-secret-0123456789", time.Hour, 24*time.Hour)
	if _, err := rotated.ValidateToken(pair.AccessToken); err == nil {
		t.Fatal("unknown kid must be rejected")
	}
	rotated.AddVerificationKeys("old-secret-0123456789", "")
	c, err := rotated.ValidateToken(pair.AccessToken)
	if err != nil || c.PwdVer != 3 || c.SessionID != "s" {
		t.Fatalf("old token must verify after rotation: %v %+v", err, c)
	}
	np, _ := rotated.GenerateTokenPair("1", "u", "", "")
	if _, err := old.ValidateToken(np.AccessToken); err == nil {
		t.Fatal("old manager must not accept tokens of the new key")
	}
}

func TestJWT_LegacyTokenWithoutKid(t *testing.T) {
	m := NewJWTManager("new-secret-0123456789", time.Hour, time.Hour)
	m.AddVerificationKeys("legacy-secret-0123456789")
	claims := &Claims{RegisteredClaims: jwt.RegisteredClaims{ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))}, Username: "u", TokenType: AccessToken}
	s, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte("legacy-secret-0123456789"))
	if c, err := m.ValidateToken(s); err != nil || c.Username != "u" {
		t.Fatalf("kid-less token signed by a configured key must verify: %v", err)
	}
	s2, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte("attacker"))
	if _, err := m.ValidateToken(s2); err == nil {
		t.Fatal("foreign key accepted")
	}
	// alg=none is rejected.
	s3, _ := jwt.NewWithClaims(jwt.SigningMethodNone, claims).SignedString(jwt.UnsafeAllowNoneSignatureType)
	if _, err := m.ValidateToken(s3); err == nil {
		t.Fatal("alg none accepted")
	}
}

func TestJWT_RefreshCappedBySessionEnd(t *testing.T) {
	m := NewJWTManager("secret-0123456789abcd", 2*time.Hour, 7*24*time.Hour)
	end := time.Now().Add(30 * time.Minute)
	p, _ := m.GenerateSessionTokens(SessionTokenParams{Username: "u", RefreshExpiresAt: end})
	if p.RefreshJTI == "" || p.RefreshExpiresAt.After(end.Add(time.Second)) || p.ExpiresAt.After(end.Add(time.Second)) {
		t.Fatalf("session end not applied: %+v", p)
	}
	c, _ := m.ValidateToken(p.RefreshToken)
	if c.ID != p.RefreshJTI || c.TokenType != RefreshToken {
		t.Fatal("refresh JTI mismatch")
	}
}
