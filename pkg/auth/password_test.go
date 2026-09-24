package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
	"time"

	"github.com/fengzhizi319/privahub/pkg/sm3"
)

func sha(p string) string { h := sha256.Sum256([]byte(p)); return hex.EncodeToString(h[:]) }

// Known-answer vectors computed independently (Python hashlib 'sm3'):
// digest = SM3^N(salt ‖ hex(SHA-256("Passw0rd!Strong"))), salt = 00..0f.
func TestIterateSM3_KnownAnswer(t *testing.T) {
	salt := make([]byte, 16)
	for i := range salt {
		salt[i] = byte(i)
	}
	in := sha("Passw0rd!Strong")
	if in != "2890c0aa26fbad69389bf01a8a8dc2bddc37d528f77393f6b626c029fa19546c" {
		t.Fatalf("sha256 input = %s", in)
	}
	for _, v := range []struct {
		n    int
		want string
	}{
		{1, "acd19430c08e10a677b42cf3e8c048f71628bbd3afb6715009c71e100e157e8c"},
		{2, "038d19bbe027aa21ca321044cc0d8b6e390efe1aab6be7554fde46c220d073cb"},
		{1000, "ee9333191ca258513782079706f439ff32d28d82f43dc38f38d93311c6bfb70d"},
	} {
		d := iterateSM3(salt, in, v.n)
		if got := hex.EncodeToString(d[:]); got != v.want {
			t.Errorf("N=%d: got %s want %s", v.n, got, v.want)
		}
	}
	// Encoded form verifies.
	d := iterateSM3(salt, in, 1000)
	enc := PasswordKDFPrefix + hex.EncodeToString(salt) + ":1000:" + hex.EncodeToString(d[:])
	if !VerifyStoredPassword(enc, PasswordCandidates{SHA256: strings.ToUpper(in)}).OK {
		t.Fatal("known-answer encoded hash must verify (case-insensitive client hash)")
	}
}

func TestHashClientPassword_RoundTrip(t *testing.T) {
	h, err := HashClientPassword(sha("s3cret"))
	if err != nil {
		t.Fatal(err)
	}
	if StoredPasswordFormat(h) != FormatKDF || len(h) > 128 {
		t.Fatalf("unexpected encoding %q (len %d)", h, len(h))
	}
	h2, _ := HashClientPassword(sha("s3cret"))
	if h == h2 {
		t.Fatal("salt must be random per hash")
	}
	if r := VerifyStoredPassword(h, PasswordCandidates{SHA256: sha("s3cret")}); !r.OK || r.NeedsRehash {
		t.Fatalf("verify = %+v", r)
	}
	if VerifyStoredPassword(h, PasswordCandidates{SHA256: sha("wrong")}).OK {
		t.Fatal("wrong password accepted")
	}
	// The stored value itself is not a credential (no pass-the-hash).
	if VerifyStoredPassword(h, PasswordCandidates{SHA256: h}).OK {
		t.Fatal("stored hash must not verify as a credential")
	}
}

func TestVerifyStoredPassword_LegacyAndWrapped(t *testing.T) {
	shaHex := sha("abc")
	sm3Hex := sm3.SumHex([]byte("abc"))

	// Raw legacy SHA-256 and Java SM3 rows verify once and ask for a rehash.
	if r := VerifyStoredPassword(shaHex, PasswordCandidates{SHA256: shaHex}); !r.OK || !r.NeedsRehash {
		t.Fatalf("legacy sha row: %+v", r)
	}
	if r := VerifyStoredPassword(sm3Hex, PasswordCandidates{SHA256: shaHex, SM3: sm3Hex}); !r.OK || !r.NeedsRehash {
		t.Fatalf("legacy sm3 row: %+v", r)
	}
	if VerifyStoredPassword(sm3Hex, PasswordCandidates{SHA256: shaHex}).OK {
		t.Fatal("legacy sm3 row needs the SM3 candidate")
	}

	// Offline-wrapped rows (unknown inner hash family).
	wSha, _ := WrapLegacyHash(shaHex)
	wSm3, _ := WrapLegacyHash(sm3Hex)
	if StoredPasswordFormat(wSha) != FormatWrapped {
		t.Fatalf("wrapped format: %s", wSha)
	}
	if r := VerifyStoredPassword(wSha, PasswordCandidates{SHA256: shaHex}); !r.OK || !r.NeedsRehash {
		t.Fatalf("wrapped sha: %+v", r)
	}
	if r := VerifyStoredPassword(wSm3, PasswordCandidates{SHA256: shaHex, SM3: sm3Hex}); !r.OK || !r.NeedsRehash {
		t.Fatalf("wrapped sm3: %+v", r)
	}
	if VerifyStoredPassword(wSm3, PasswordCandidates{SHA256: shaHex}).OK {
		t.Fatal("wrapped sm3 row must not verify without the SM3 candidate")
	}
	// A leaked wrapped row cannot be replayed.
	if VerifyStoredPassword(wSha, PasswordCandidates{SHA256: wSha}).OK || VerifyStoredPassword(wSha, PasswordCandidates{SHA256: shaHex[:63] + "0"}).OK {
		t.Fatal("wrapped row accepted a wrong credential")
	}
}

func TestVerifyStoredPassword_CorruptFormatsNeverMatch(t *testing.T) {
	good, _ := HashClientPassword(sha("x"))
	parts := strings.Split(strings.TrimPrefix(good, PasswordKDFPrefix), ":")
	for _, bad := range []string{
		"", "plaintext", PasswordKDFPrefix, PasswordKDFPrefix + "zz:10:" + parts[2],
		PasswordKDFPrefix + parts[0] + ":0:" + parts[2],
		PasswordKDFPrefix + parts[0] + ":999999999999:" + parts[2],
		PasswordKDFPrefix + parts[0] + ":" + parts[1] + ":abcd",
		PasswordKDFPrefix + parts[0] + ":" + parts[1],
	} {
		if VerifyStoredPassword(bad, PasswordCandidates{SHA256: sha("x")}).OK {
			t.Errorf("corrupt %q matched", bad)
		}
	}
}

func TestVerifyStoredPassword_WeakerWorkFactorNeedsRehash(t *testing.T) {
	SetPasswordKDFIterationsForTesting(4)
	h, _ := HashClientPassword(sha("p"))
	SetPasswordKDFIterationsForTesting(8)
	defer SetPasswordKDFIterationsForTesting(16)
	if r := VerifyStoredPassword(h, PasswordCandidates{SHA256: sha("p")}); !r.OK || !r.NeedsRehash {
		t.Fatalf("lower work factor should verify and request rehash: %+v", r)
	}
}

func TestConfigurePasswordKDF_MinimumEnforced(t *testing.T) {
	SetPasswordKDFIterationsForTesting(0) // disable test override
	defer SetPasswordKDFIterationsForTesting(16)
	if got := ConfigurePasswordKDF(10); got != MinPasswordKDFIterations {
		t.Errorf("below minimum: %d", got)
	}
	if got := ConfigurePasswordKDF(0); got != DefaultPasswordKDFIterations {
		t.Errorf("default: %d", got)
	}
	if got := ConfigurePasswordKDF(2_000_000); got != 2_000_000 || PasswordKDFIterations() != 2_000_000 {
		t.Errorf("explicit: %d", got)
	}
	ConfigurePasswordKDF(0)
}

// TestPasswordVerify_MinTimeBudget is the work-factor regression gate: one
// verification at the production default must cost at least
// MinPasswordVerifyBudget.
func TestPasswordVerify_MinTimeBudget(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: production work factor")
	}
	salt := make([]byte, passwordKDFSaltSize)
	d := iterateSM3(salt, sha("x"), DefaultPasswordKDFIterations)
	enc := PasswordKDFPrefix + hex.EncodeToString(salt) + ":" + "1000000" + ":" + hex.EncodeToString(d[:])
	start := time.Now()
	if !VerifyStoredPassword(enc, PasswordCandidates{SHA256: sha("x")}).OK {
		t.Fatal("verify failed")
	}
	if el := time.Since(start); el < MinPasswordVerifyBudget {
		t.Fatalf("verification took %v < budget %v: raise the work factor", el, MinPasswordVerifyBudget)
	}
}

func TestIsWeakPasswordHash(t *testing.T) {
	for _, w := range []string{"12345678", "password", "Password", "PASSWORD", "admin123", "Admin@123", "qwerty123"} {
		if !IsWeakPasswordHash(sha(w)) {
			t.Errorf("%q should be weak", w)
		}
	}
	if !IsWeakPasswordHash(strings.ToUpper(sha("12345678"))) {
		t.Error("hash case must not matter")
	}
	for _, s := range []string{"Str0ng-Pass#1", "c0rrect-h0rse-battery"} {
		if IsWeakPasswordHash(sha(s)) {
			t.Errorf("%q should not be weak", s)
		}
	}
	RegisterWeakPasswords("CompanyName2026")
	if !IsWeakPasswordHash(sha("companyname2026")) {
		t.Error("registered word (variant) must be weak")
	}
}

func TestTokenDigest(t *testing.T) {
	d := TokenDigest("a.b.c")
	if !IsTokenDigest(d) || d == TokenDigest("a.b.d") || IsTokenDigest("a.b.c") {
		t.Fatal("token digest properties")
	}
	if RefreshJTIDigest("") != "" || RefreshJTIDigest("j") == TokenDigest("j") {
		t.Fatal("refresh digest must be domain separated")
	}
}
