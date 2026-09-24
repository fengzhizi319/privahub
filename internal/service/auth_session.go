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
	"context"
	"strings"
	"time"

	"github.com/fengzhizi319/privahub/internal/audit"
	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/pkg/auth"
	"gorm.io/gorm"
)

// storeSession saves the session row. Only digests are stored: the access
// token as auth.TokenDigest and the refresh JTI as auth.RefreshJTIDigest; the
// session_data snapshot never contains the token.
func (s *AuthService) storeSession(ctx context.Context, uc *auth.UserContext, pair *auth.TokenPair) error {
	now := s.now()
	snapshot := *uc
	snapshot.Token = ""
	refreshExp := pair.RefreshExpiresAt
	return s.tokenRepo.Create(ctx, &model.UserTokensDO{
		Name:             uc.Name,
		Token:            auth.TokenDigest(pair.AccessToken),
		GmtToken:         &now,
		SessionData:      snapshot.ToJSON(),
		SessionID:        uc.SessionID,
		RefreshJTI:       auth.RefreshJTIDigest(pair.RefreshJTI),
		PwdVer:           uc.PwdVer,
		GmtRefreshExpire: &refreshExp,
	})
}

// Logout deletes the current session (only the given token), like Java
// userTokensRepository.deleteByNameAndToken.
func (s *AuthService) Logout(ctx context.Context, username, token string) error {
	if s.db == nil {
		return s.tokenRepo.DeleteByName(ctx, username)
	}
	err := s.db.WithContext(ctx).Unscoped().
		Where("name = ? AND token = ?", username, auth.TokenDigest(token)).
		Delete(&model.UserTokensDO{}).Error
	result := audit.ResultSuccess
	if err != nil {
		result = audit.ResultFailure
	}
	recordAuthEvent(ctx, audit.ActionLogout, username, "", result, "")
	return err
}

// ClearUserSessions deletes all sessions of a user (after password change).
func (s *AuthService) ClearUserSessions(ctx context.Context, username string) error {
	return ClearUserSessions(ctx, s.db, username)
}

// ClearUserSessions deletes all user_tokens rows of a user.
func ClearUserSessions(ctx context.Context, db *gorm.DB, username string) error {
	if db == nil {
		return nil
	}
	return db.WithContext(ctx).Unscoped().Where("name = ?", username).Delete(&model.UserTokensDO{}).Error
}

// revokeSession deletes one session row.
func (s *AuthService) revokeSession(ctx context.Context, row *model.UserTokensDO, reason string) {
	if s.db != nil {
		_ = s.db.WithContext(ctx).Unscoped().Delete(&model.UserTokensDO{}, row.ID).Error
	}
	recordAuthEvent(ctx, auditActionSessionRevoked, row.Name, "", audit.ResultSuccess, reason)
}

// sessionExpired checks the sliding idle timeout and the absolute session end.
func (s *AuthService) sessionExpired(row *model.UserTokensDO, now time.Time) bool {
	if row.GmtToken == nil || now.Sub(*row.GmtToken) > s.opts.SessionIdleTimeout {
		return true
	}
	return row.GmtRefreshExpire != nil && now.After(*row.GmtRefreshExpire)
}

// accountState returns the current password version and must-change flag of
// an account; ok=false when the account no longer exists.
func (s *AuthService) accountState(ctx context.Context, name string) (pwdVer int64, mustChange bool, ok bool) {
	u, err := s.userRepo.FindByName(ctx, name)
	if err != nil || u == nil {
		return 0, false, false
	}
	return u.PasswordVersion, u.MustChangePassword, true
}

// RefreshToken rotates the token pair of a still-valid session. Refresh
// tokens are one-time-use: the session row stores the digest of the only
// valid refresh JTI; presenting an older (already rotated) refresh token is
// treated as token theft and revokes the whole session (reuse detection).
func (s *AuthService) RefreshToken(ctx context.Context, refreshToken string) (*auth.TokenPair, error) {
	claims, err := s.jwtManager.ValidateToken(refreshToken)
	if err != nil {
		return nil, err
	}
	if claims.TokenType != auth.RefreshToken {
		return nil, auth.ErrInvalidToken
	}
	if s.db == nil || claims.SessionID == "" {
		return nil, ErrTokenNotFound
	}
	var row model.UserTokensDO
	if err := s.db.WithContext(ctx).
		Where("name = ? AND session_id = ?", claims.Username, claims.SessionID).
		First(&row).Error; err != nil {
		return nil, ErrTokenNotFound
	}
	presented := auth.RefreshJTIDigest(claims.ID)
	if row.RefreshJTI != "" && !constEq(row.RefreshJTI, presented) {
		s.revokeSession(ctx, &row, "refresh_token_reuse")
		recordAuthEvent(ctx, auditActionRefreshReuse, row.Name, "", audit.ResultDenied, "refresh_token_reuse")
		return nil, ErrRefreshTokenReused
	}
	now := s.now()
	if s.sessionExpired(&row, now) {
		s.revokeSession(ctx, &row, "expired")
		return nil, ErrSessionExpired
	}
	if pv, _, ok := s.accountState(ctx, row.Name); !ok || pv != row.PwdVer || pv != claims.PwdVer {
		s.revokeSession(ctx, &row, "password_changed")
		return nil, ErrSessionExpired
	}
	sessionEnd := claims.ExpiresAt.Time
	if row.GmtRefreshExpire != nil {
		sessionEnd = *row.GmtRefreshExpire
	}
	pair, err := s.jwtManager.GenerateSessionTokens(auth.SessionTokenParams{
		UserID: claims.UserID, Username: claims.Username, OwnerType: claims.OwnerType,
		OwnerID: claims.OwnerID, SessionID: claims.SessionID, PwdVer: claims.PwdVer,
		RefreshExpiresAt: sessionEnd,
	})
	if err != nil {
		return nil, err
	}
	// Compare-and-swap on the refresh JTI: of two concurrent refreshes with
	// the same token only one wins; the loser is a reuse.
	q := s.db.WithContext(ctx).Model(&model.UserTokensDO{}).Where("id = ?", row.ID)
	if row.RefreshJTI == "" {
		q = q.Where("(refresh_jti = '' OR refresh_jti IS NULL)")
	} else {
		q = q.Where("refresh_jti = ?", row.RefreshJTI)
	}
	res := q.Updates(map[string]interface{}{
		"token":       auth.TokenDigest(pair.AccessToken),
		"refresh_jti": auth.RefreshJTIDigest(pair.RefreshJTI),
		"gmt_token":   &now,
	})
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		s.revokeSession(ctx, &row, "refresh_token_reuse")
		recordAuthEvent(ctx, auditActionRefreshReuse, row.Name, "", audit.ResultDenied, "concurrent_refresh")
		return nil, ErrRefreshTokenReused
	}
	return pair, nil
}

// ValidateSession checks token against user_tokens with sliding expiry and
// returns the session UserContext (Java LoginInterceptor.processByUserRequest).
// The session is rejected (and deleted) when idle, past its absolute end, or
// when the account password version changed since login.
func (s *AuthService) ValidateSession(ctx context.Context, token string) (*auth.UserContext, error) {
	if token == "" {
		return nil, ErrTokenNotFound
	}
	row, err := s.tokenRepo.FindByToken(ctx, auth.TokenDigest(token))
	if err != nil || row == nil {
		return nil, ErrTokenNotFound
	}
	now := s.now()
	if s.sessionExpired(row, now) {
		if s.db != nil {
			_ = s.db.WithContext(ctx).Unscoped().Delete(row).Error
		}
		return nil, ErrSessionExpired
	}
	pv, mustChange, ok := s.accountState(ctx, row.Name)
	if !ok || pv != row.PwdVer {
		s.revokeSession(ctx, row, "password_changed")
		return nil, ErrSessionExpired
	}
	// Slide the expiry; throttle writes to at most one per minute per session.
	if now.Sub(*row.GmtToken) > time.Minute && s.db != nil {
		_ = s.db.WithContext(ctx).Model(&model.UserTokensDO{}).
			Where("id = ?", row.ID).Update("gmt_token", now).Error
	}
	if strings.TrimSpace(row.SessionData) == "" {
		return nil, ErrTokenNotFound
	}
	uc, err := auth.UserContextFromJSON(row.SessionData)
	if err != nil {
		return nil, ErrTokenNotFound
	}
	uc.Token = token
	uc.SessionID = row.SessionID
	uc.MustChangePassword = mustChange
	uc.PwdVer = pv
	return uc, nil
}
