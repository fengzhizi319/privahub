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
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"sync"
	"unicode"
)

// Server-side weak password check.
//
// LIMITATION (by design of the hashed-password protocol): the server never
// sees the plaintext password, only SHA-256(password). It therefore cannot
// enforce length / character-class rules — those stay in the frontend
// (@secretpad/utils validatePasswordChange) and can be bypassed by a client
// that calls the API directly. What the server CAN do is reject hashes of a
// known weak-password dictionary: it hashes each dictionary word (plus simple
// case variants) with SHA-256 and compares the submitted client hash. A direct
// API client can still set any password that is not in the dictionary.

// weakPasswords is the built-in dictionary (ported from PrivShield plus the
// SecretPad/Kuscia defaults and common Chinese-locale choices).
var weakPasswords = []string{
	// PrivShield list
	"password", "12345678", "123456789", "1234567890", "qwerty123456", "admin123456",
	"iloveyou1234", "abc12345678", "password1234", "letmein12345",
	"welcome12345", "monkey123456", "dragon123456", "master123456",
	"123456789012", "0987654321ab", "password12345", "adminadmin12",
	"rootadmin123", "administrator", "guest123456", "operator123",
	"default12345", "pass12345678", "system123456", "security123",
	"privshield123", "database123", "oracle123456", "service1234",
	"changeme1234", "trustno11234", "superman1234", "sunshine1234",
	"princess1234", "football1234", "baseball1234", "shadow123456",
	"michael12345", "jordan123456", "harley123456", "ranger123456",
	"freedom12345", "computer1234", "starwars1234", "trustme12345",
	"hunter212345", "buster123456", "silver123456", "ginger123456",
	// Common top passwords
	"123456", "1234567", "12345", "123123", "111111", "000000", "666666", "888888",
	"11111111", "00000000", "88888888", "66666666", "12341234", "87654321",
	"qwerty", "qwertyui", "qwerty123", "qwe123456", "1q2w3e4r", "1qaz2wsx", "1q2w3e4r5t",
	"abc123", "abc123456", "abcd1234", "a1234567", "a12345678", "aa123456",
	"password1", "password123", "passw0rd", "p@ssw0rd", "p@ssword", "pa$$w0rd",
	"admin", "admin123", "admin1234", "admin@123", "admin12345", "admin888",
	"root", "root123", "root1234", "root@123", "toor",
	"test", "test123", "test1234", "test12345", "guest", "guest123",
	"welcome", "welcome1", "welcome123", "letmein", "iloveyou", "changeme",
	"woaini", "woaini1314", "5201314", "1314520", "zxcvbnm", "asdfghjkl", "qazwsx",
	// Platform defaults (must never be kept in production)
	"secretpad", "secretpad123", "secretflow", "secretflow123", "kuscia", "kuscia123",
	"privahub", "privahub123", "Privahub@123", "Secretpad@123", "Admin@123", "Admin@1234",
	"Aa123456", "Aa12345678", "Abc12345", "Abc@1234", "Abc123456", "Qwer1234", "Abcd@1234",
}

var (
	weakOnce   sync.Once
	weakHashes map[string]struct{}
	weakMu     sync.RWMutex
)

func buildWeakHashes() {
	weakHashes = make(map[string]struct{}, len(weakPasswords)*4)
	for _, w := range weakPasswords {
		addWeakLocked(w)
	}
}

func addWeakLocked(w string) {
	w = strings.TrimSpace(w)
	if w == "" {
		return
	}
	for _, v := range weakVariants(w) {
		h := sha256.Sum256([]byte(v))
		weakHashes[hex.EncodeToString(h[:])] = struct{}{}
	}
}

// weakVariants returns w, lower(w), upper(w) and Capitalized(lower(w)).
func weakVariants(w string) []string {
	lower := strings.ToLower(w)
	out := []string{w, lower, strings.ToUpper(w)}
	if r := []rune(lower); len(r) > 0 {
		r[0] = unicode.ToUpper(r[0])
		out = append(out, string(r))
	}
	return out
}

// RegisterWeakPasswords extends the dictionary at runtime (e.g. from a file).
func RegisterWeakPasswords(words ...string) {
	weakOnce.Do(buildWeakHashes)
	weakMu.Lock()
	defer weakMu.Unlock()
	for _, w := range words {
		addWeakLocked(w)
	}
}

// IsWeakPasswordHash reports whether a client hash (hex SHA-256 of the
// password) belongs to the weak-password dictionary.
func IsWeakPasswordHash(clientHash string) bool {
	weakOnce.Do(buildWeakHashes)
	weakMu.RLock()
	defer weakMu.RUnlock()
	_, ok := weakHashes[NormalizeClientHash(clientHash)]
	return ok
}
