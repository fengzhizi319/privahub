// Copyright 2026 The Privahub Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/fengzhizi319/privahub/pkg/sm3"
)

// Server-side password storage (国密 SM3 iterated KDF).
//
// Protocol: the browser never sends the plaintext password; it sends the
// client hash H = lowercase hex SHA-256(password) (and, for accounts migrated
// from Java SecretPad, H_sm3 = lowercase hex SM3(password)). Before this KDF the
// server stored H verbatim, so a leaked user_accounts table was directly usable
// to log in ("pass-the-hash"). The server now stores
//
//	sm3kdf:v1:<salt-hex>:<iterations>:<digest-hex>
//	digest = SM3^N(salt ‖ H)   (h1 = SM3(salt‖H), h_i = SM3(h_{i-1}), N rounds)
//
// with a 128-bit random salt per hash and N = 1,000,000 by default. The client
// protocol is unchanged (the input is still H); a leaked row now costs N SM3
// evaluations per guess and cannot be replayed at the login endpoint.
//
// Legacy rows are handled transparently:
//   - raw 64-hex rows (SHA-256 hex written by earlier Go builds, or SM3 hex
//     written by Java SecretPad) are wrapped offline at startup into
//     "sm3kdf-w:v1:..." = KDF(rawhex). Because the inner hash family is unknown,
//     a wrapped row is verified against KDF(H) and, failing that, KDF(H_sm3);
//     on success the login rewrites it to canonical sm3kdf:v1 over H.
//   - unwrapped raw rows (if any remain) are compared in constant time the old
//     way and immediately upgraded on success.
const (
	// PasswordKDFPrefix is the canonical stored-password format prefix.
	PasswordKDFPrefix = "sm3kdf:v1:"
	// PasswordKDFWrappedPrefix marks a legacy raw hash wrapped offline by the
	// startup migration (inner hash: SHA-256 hex or legacy Java SM3 hex).
	PasswordKDFWrappedPrefix = "sm3kdf-w:v1:"

	passwordKDFSaltSize = 16

	// DefaultPasswordKDFIterations is the production work factor
	// (~0.25 s per verification on a current x86 core).
	DefaultPasswordKDFIterations = 1_000_000
	// MinPasswordKDFIterations is the smallest work factor accepted from
	// configuration; lower configured values are raised to it.
	MinPasswordKDFIterations = 200_000
	// maxPasswordKDFIterations bounds the iteration count parsed from a stored
	// hash so a corrupted/malicious row cannot pin a CPU for minutes.
	maxPasswordKDFIterations = 50_000_000

	// MinPasswordVerifyBudget is the lower bound of one verification at the
	// production work factor (regression gate: if the default ever drops so
	// that a verification is faster than this, recalibrate instead of lowering
	// the threshold).
	MinPasswordVerifyBudget = 50 * time.Millisecond
)

var (
	passwordKDFIterations atomic.Int64
	// testIterationsOverride, when > 0, wins over ConfigurePasswordKDF (tests).
	testIterationsOverride atomic.Int64
)

func init() { passwordKDFIterations.Store(DefaultPasswordKDFIterations) }

// ConfigurePasswordKDF sets the work factor used for new hashes from
// configuration (0 = default). Values below MinPasswordKDFIterations are
// raised to the minimum. It returns the effective iteration count.
func ConfigurePasswordKDF(iterations int) int {
	if testIterationsOverride.Load() > 0 {
		return int(testIterationsOverride.Load())
	}
	if iterations <= 0 {
		iterations = DefaultPasswordKDFIterations
	}
	if iterations < MinPasswordKDFIterations {
		iterations = MinPasswordKDFIterations
	}
	if iterations > maxPasswordKDFIterations {
		iterations = maxPasswordKDFIterations
	}
	passwordKDFIterations.Store(int64(iterations))
	return iterations
}

// SetPasswordKDFIterationsForTesting lowers the work factor for unit tests
// (bypasses the configured minimum). Never call it from production code.
func SetPasswordKDFIterationsForTesting(n int) {
	testIterationsOverride.Store(int64(n))
	if n > 0 {
		passwordKDFIterations.Store(int64(n))
	} else {
		passwordKDFIterations.Store(DefaultPasswordKDFIterations)
	}
}

// PasswordKDFIterations returns the work factor used for new hashes.
func PasswordKDFIterations() int { return int(passwordKDFIterations.Load()) }

var clientHashRe = regexp.MustCompile(`^[0-9a-fA-F]{64}$`)

// IsClientHash reports whether s looks like a client password hash (64 hex).
func IsClientHash(s string) bool { return clientHashRe.MatchString(s) }

// NormalizeClientHash lowercases/trims a client hash.
func NormalizeClientHash(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

// HashClientPassword derives the stored form of a client hash (SHA-256 hex).
func HashClientPassword(clientHash string) (string, error) {
	return deriveEncoded(PasswordKDFPrefix, NormalizeClientHash(clientHash), PasswordKDFIterations())
}

// WrapLegacyHash wraps a legacy raw 64-hex stored hash (offline migration).
func WrapLegacyHash(rawHex string) (string, error) {
	return deriveEncoded(PasswordKDFWrappedPrefix, NormalizeClientHash(rawHex), PasswordKDFIterations())
}

func deriveEncoded(prefix, input string, iterations int) (string, error) {
	salt := make([]byte, passwordKDFSaltSize)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("auth: generate password salt: %w", err)
	}
	d := iterateSM3(salt, input, iterations)
	return prefix + hex.EncodeToString(salt) + ":" + strconv.Itoa(iterations) + ":" + hex.EncodeToString(d[:]), nil
}

// iterateSM3 computes SM3^N(salt ‖ input).
func iterateSM3(salt []byte, input string, iterations int) [32]byte {
	buf := make([]byte, 0, len(salt)+len(input))
	buf = append(buf, salt...)
	buf = append(buf, input...)
	d := sm3.Sum(buf)
	for i := 1; i < iterations; i++ {
		d = sm3.Sum(d[:])
	}
	return d
}

// PasswordFormat classifies a stored password hash.
type PasswordFormat int

// Stored password formats.
const (
	FormatUnknown   PasswordFormat = iota
	FormatKDF                      // sm3kdf:v1 over the SHA-256 client hash
	FormatWrapped                  // sm3kdf-w:v1 over a legacy raw hash
	FormatLegacyRaw                // raw 64-hex (SHA-256 or Java SM3)
)

// StoredPasswordFormat returns the format of a stored hash.
func StoredPasswordFormat(stored string) PasswordFormat {
	switch {
	case strings.HasPrefix(stored, PasswordKDFPrefix):
		return FormatKDF
	case strings.HasPrefix(stored, PasswordKDFWrappedPrefix):
		return FormatWrapped
	case IsClientHash(stored):
		return FormatLegacyRaw
	}
	return FormatUnknown
}

type kdfParams struct {
	salt       []byte
	iterations int
	digest     []byte
}

func parseKDF(stored, prefix string) (kdfParams, bool) {
	rest, ok := strings.CutPrefix(stored, prefix)
	if !ok {
		return kdfParams{}, false
	}
	parts := strings.Split(rest, ":")
	if len(parts) != 3 {
		return kdfParams{}, false
	}
	salt, err := hex.DecodeString(parts[0])
	if err != nil || len(salt) != passwordKDFSaltSize {
		return kdfParams{}, false
	}
	n, err := strconv.Atoi(parts[1])
	if err != nil || n <= 0 || n > maxPasswordKDFIterations {
		return kdfParams{}, false
	}
	digest, err := hex.DecodeString(parts[2])
	if err != nil || len(digest) != 32 {
		return kdfParams{}, false
	}
	return kdfParams{salt: salt, iterations: n, digest: digest}, true
}

func (p kdfParams) matches(input string) bool {
	d := iterateSM3(p.salt, input, p.iterations)
	return hmac.Equal(d[:], p.digest)
}

// PasswordCandidates are the client-supplied credentials of one attempt.
type PasswordCandidates struct {
	// SHA256 is the lowercase hex SHA-256(password) (standard scheme).
	SHA256 string
	// SM3 is the lowercase hex SM3(password) (legacy Java accounts; optional).
	SM3 string
}

// VerifyResult reports the outcome of VerifyStoredPassword.
type VerifyResult struct {
	// OK is true when a candidate matched.
	OK bool
	// NeedsRehash is true when the stored form should be rewritten to the
	// canonical KDF over the SHA-256 client hash (legacy / wrapped / weaker
	// work factor).
	NeedsRehash bool
}

// VerifyStoredPassword checks the candidates against a stored hash in
// constant time. Unknown/corrupt formats never match (but still spend one
// KDF evaluation so timing does not reveal the format).
func VerifyStoredPassword(stored string, c PasswordCandidates) VerifyResult {
	sha := NormalizeClientHash(c.SHA256)
	sm := NormalizeClientHash(c.SM3)
	switch StoredPasswordFormat(stored) {
	case FormatKDF:
		p, ok := parseKDF(stored, PasswordKDFPrefix)
		if !ok {
			DummyPasswordWork()
			return VerifyResult{}
		}
		if sha == "" {
			DummyPasswordWork()
			return VerifyResult{}
		}
		if p.matches(sha) {
			return VerifyResult{OK: true, NeedsRehash: p.iterations < PasswordKDFIterations()}
		}
		return VerifyResult{}
	case FormatWrapped:
		p, ok := parseKDF(stored, PasswordKDFWrappedPrefix)
		if !ok {
			DummyPasswordWork()
			return VerifyResult{}
		}
		if sha != "" && p.matches(sha) {
			return VerifyResult{OK: true, NeedsRehash: true}
		}
		if sm != "" && IsClientHash(sm) && p.matches(sm) {
			return VerifyResult{OK: true, NeedsRehash: true}
		}
		return VerifyResult{}
	case FormatLegacyRaw:
		// Spend the same work as a KDF row so the legacy path is not a
		// timing oracle, then compare the raw hashes in constant time.
		DummyPasswordWork()
		st := strings.ToLower(stored)
		if sha != "" && hmac.Equal([]byte(sha), []byte(st)) {
			return VerifyResult{OK: true, NeedsRehash: true}
		}
		if sm != "" && hmac.Equal([]byte(sm), []byte(st)) {
			return VerifyResult{OK: true, NeedsRehash: true}
		}
		return VerifyResult{}
	}
	DummyPasswordWork()
	return VerifyResult{}
}

// dummySalt is a fixed salt for DummyPasswordWork.
var dummySalt = make([]byte, passwordKDFSaltSize)

// DummyPasswordWork performs one KDF evaluation with the current work factor
// and discards the result. It equalises timing for unknown accounts so user
// enumeration by response time is not possible.
func DummyPasswordWork() {
	_ = iterateSM3(dummySalt, "0000000000000000000000000000000000000000000000000000000000000000", PasswordKDFIterations())
}
