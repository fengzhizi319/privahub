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

package service

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/pkg/auth"
	"github.com/fengzhizi319/privahub/pkg/errcode"
)

// Server-side password policy.
//
// The server only receives SHA-256(password) (see AuthService), so length and
// character-class rules can only be enforced by the frontend. The server
// enforces what it can on hashes:
//   - the value must be a 64-hex client hash (no plaintext in production);
//   - the hash must not belong to the weak-password dictionary
//     (auth.IsWeakPasswordHash);
//   - the new password must differ from the current one and from the
//     previous passwordHistorySize passwords (KDF history, reuse check).

// passwordHistorySize is the number of previous passwords kept for the reuse
// check (in addition to the current one).
const passwordHistorySize = 3

// clientPasswordHash returns the SHA-256 client hash of a request: the
// submitted hash when it is well-formed, otherwise the server-side hash of a
// plaintext password (legacy API callers).
func clientPasswordHash(hash, plain string) string {
	if hexHash.MatchString(strings.TrimSpace(hash)) {
		return strings.ToLower(strings.TrimSpace(hash))
	}
	if plain != "" {
		return HashPassword(plain)
	}
	return ""
}

// checkNewPasswordHash validates a new client hash against the server policy.
func checkNewPasswordHash(clientHash string) error {
	if !hexHash.MatchString(clientHash) {
		return newAuthError(ErrPasswordRequired, errcode.SystemValidationError, "newPasswordHash must be the hex SHA-256 of the password")
	}
	if auth.IsWeakPasswordHash(clientHash) {
		return newAuthError(ErrPasswordTooWeak, errcode.AuthPasswordTooWeak)
	}
	return nil
}

// passwordHistory decodes user_accounts.password_history.
func passwordHistory(u *model.UserAccountsDO) []string {
	if strings.TrimSpace(u.PasswordHistory) == "" {
		return nil
	}
	var h []string
	if err := json.Unmarshal([]byte(u.PasswordHistory), &h); err != nil {
		return nil
	}
	return h
}

// isPasswordReused reports whether clientHash matches the current password or
// one of the remembered previous passwords.
func isPasswordReused(u *model.UserAccountsDO, clientHash string) bool {
	cand := auth.PasswordCandidates{SHA256: clientHash}
	if u.PasswordHash != "" && auth.VerifyStoredPassword(u.PasswordHash, cand).OK {
		return true
	}
	for _, old := range passwordHistory(u) {
		if auth.VerifyStoredPassword(old, cand).OK {
			return true
		}
	}
	return false
}

// newPasswordUpdates derives the stored hash of clientHash and returns the
// column updates of a password change: new KDF hash, history rotated, version
// bumped (invalidates every token), must-change flag set as requested.
func newPasswordUpdates(u *model.UserAccountsDO, clientHash string, mustChange bool, now time.Time) (map[string]interface{}, error) {
	stored, err := auth.HashClientPassword(clientHash)
	if err != nil {
		return nil, err
	}
	hist := passwordHistory(u)
	if u.PasswordHash != "" {
		hist = append([]string{u.PasswordHash}, hist...)
	}
	if len(hist) > passwordHistorySize {
		hist = hist[:passwordHistorySize]
	}
	hb, _ := json.Marshal(hist)
	updates := map[string]interface{}{
		"password_hash":        stored,
		"password_history":     string(hb),
		"password_version":     u.PasswordVersion + 1,
		"must_change_password": mustChange,
		"gmt_password_changed": now,
	}
	u.PasswordHash = stored
	u.PasswordHistory = string(hb)
	u.PasswordVersion++
	u.MustChangePassword = mustChange
	u.GmtPasswordChanged = &now
	return updates, nil
}
