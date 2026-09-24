package service

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func signLegacy(t *testing.T, instID, nodeID string, issued time.Time) string {
	t.Helper()
	tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"aud": []string{nodeID}, "timestamp": issued.UnixMilli(), "signNonce": "n",
		"exp": issued.Add(instTokenExpire).Unix(),
	}).SignedString([]byte(instID))
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

func TestInstToken_ServerSecret(t *testing.T) {
	legacyBefore := signLegacy(t, "inst1", "node1", time.Now().Add(-time.Minute))
	SetInstTokenSecret("server-secret-0123456789abcdef")
	t.Cleanup(func() { SetInstTokenSecret("") })

	tok, err := SignInstToken("inst1", "node1")
	if err != nil {
		t.Fatal(err)
	}
	if !VerifyInstToken(tok, "inst1", "node1") {
		t.Fatal("new token must verify")
	}
	if VerifyInstToken(tok, "inst1", "node2") || VerifyInstToken(tok, "inst2", "node1") {
		t.Fatal("audience / inst must be bound")
	}
	// The token is not signed with the public inst id.
	if _, err := jwt.Parse(tok, func(*jwt.Token) (interface{}, error) { return []byte("inst1"), nil }); err == nil {
		t.Fatal("token verifies with the inst id as key")
	}
	// A different server secret invalidates it.
	SetInstTokenSecret("another-secret-0123456789abcdef")
	if VerifyInstToken(tok, "inst1", "node1") {
		t.Fatal("token must not verify under another server secret")
	}
	// Backward compatibility: a legacy token issued before the secret was
	// installed still verifies until it expires; a freshly forged one does not.
	if !VerifyInstToken(legacyBefore, "inst1", "node1") {
		t.Fatal("legacy token issued before the upgrade must still verify")
	}
	forged := signLegacy(t, "inst1", "node1", time.Now().Add(time.Second))
	if VerifyInstToken(forged, "inst1", "node1") {
		t.Fatal("instId-signed token issued after the upgrade must be rejected")
	}
	// Legacy verification never accepts alg=none / other algs.
	none, _ := jwt.NewWithClaims(jwt.SigningMethodNone, jwt.MapClaims{"aud": []string{"node1"},
		"exp": time.Now().Add(time.Minute).Unix()}).SignedString(jwt.UnsafeAllowNoneSignatureType)
	if VerifyInstToken(none, "inst1", "node1") {
		t.Fatal("alg=none accepted")
	}
}
