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

package migration

import (
	"encoding/json"
	"runtime"
	"sync"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/pkg/auth"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// MigrateAuthSecurity hardens credential storage in place (idempotent):
//
//  1. user_accounts.password_hash rows still holding a raw 64-hex client hash
//     (SHA-256 from earlier Go builds, or SM3 from Java SecretPad) are wrapped
//     as sm3kdf-w:v1 = SM3-KDF(rawhash), so a leaked database is no longer a
//     login credential. The next successful login rewrites the row to the
//     canonical sm3kdf:v1 format.
//  2. user_tokens.token rows holding a raw token are replaced by
//     auth.TokenDigest(token), and the raw token is removed from the stored
//     session_data snapshot.
func MigrateAuthSecurity(db *gorm.DB, log *zap.Logger) error {
	if err := wrapLegacyPasswordHashes(db, log); err != nil {
		return err
	}
	return digestLegacyTokens(db, log)
}

func wrapLegacyPasswordHashes(db *gorm.DB, log *zap.Logger) error {
	var users []model.UserAccountsDO
	if err := db.Unscoped().Select("id", "name", "password_hash").Find(&users).Error; err != nil {
		return err
	}
	var todo []model.UserAccountsDO
	for _, u := range users {
		if auth.StoredPasswordFormat(u.PasswordHash) == auth.FormatLegacyRaw {
			todo = append(todo, u)
		}
	}
	if len(todo) == 0 {
		return nil
	}
	if log != nil {
		log.Info("wrapping legacy password hashes with the SM3 KDF",
			zap.Int("accounts", len(todo)), zap.Int("iterations", auth.PasswordKDFIterations()))
	}
	// The KDF is CPU bound: derive in parallel, write sequentially.
	type result struct {
		id      int64
		wrapped string
		err     error
	}
	results := make([]result, len(todo))
	workers := runtime.GOMAXPROCS(0)
	var wg sync.WaitGroup
	ch := make(chan int)
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range ch {
				wrapped, err := auth.WrapLegacyHash(todo[i].PasswordHash)
				results[i] = result{id: todo[i].ID, wrapped: wrapped, err: err}
			}
		}()
	}
	for i := range todo {
		ch <- i
	}
	close(ch)
	wg.Wait()
	for i, r := range results {
		if r.err != nil {
			return r.err
		}
		// Conditional update: never overwrite a row changed concurrently.
		if err := db.Unscoped().Model(&model.UserAccountsDO{}).
			Where("id = ? AND password_hash = ?", r.id, todo[i].PasswordHash).
			Update("password_hash", r.wrapped).Error; err != nil {
			return err
		}
	}
	return nil
}

func digestLegacyTokens(db *gorm.DB, log *zap.Logger) error {
	var rows []model.UserTokensDO
	if err := db.Unscoped().Select("id", "token", "session_data").Find(&rows).Error; err != nil {
		return err
	}
	n := 0
	for _, r := range rows {
		updates := map[string]interface{}{}
		if r.Token != "" && !auth.IsTokenDigest(r.Token) {
			updates["token"] = auth.TokenDigest(r.Token)
		}
		if sd, changed := stripSessionToken(r.SessionData); changed {
			updates["session_data"] = sd
		}
		if len(updates) == 0 {
			continue
		}
		if err := db.Unscoped().Model(&model.UserTokensDO{}).Where("id = ?", r.ID).Updates(updates).Error; err != nil {
			return err
		}
		n++
	}
	if n > 0 && log != nil {
		log.Info("digested legacy session tokens", zap.Int("sessions", n))
	}
	return nil
}

// stripSessionToken removes a non-empty "token" field from a session_data
// JSON snapshot.
func stripSessionToken(sd string) (string, bool) {
	if sd == "" {
		return sd, false
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal([]byte(sd), &m); err != nil {
		return sd, false
	}
	raw, ok := m["token"]
	if !ok || string(raw) == `""` || string(raw) == "null" {
		return sd, false
	}
	m["token"] = json.RawMessage(`""`)
	b, err := json.Marshal(m)
	if err != nil {
		return sd, false
	}
	return string(b), true
}
