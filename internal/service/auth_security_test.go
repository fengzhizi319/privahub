package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fengzhizi319/privahub/internal/audit"
	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/pkg/auth"
	"github.com/fengzhizi319/privahub/pkg/errcode"
)

type captureRecorder struct{ events []audit.Event }

func (r *captureRecorder) Record(_ context.Context, e audit.Event) { r.events = append(r.events, e) }

func (r *captureRecorder) has(action, result, reason string) bool {
	for _, e := range r.events {
		if e.Action == action && e.Result == result && (reason == "" || e.Reason == reason) {
			return true
		}
	}
	return false
}

func withAudit(t *testing.T) *captureRecorder {
	t.Helper()
	rec := &captureRecorder{}
	prev := audit.GetRecorder()
	audit.SetRecorder(rec)
	t.Cleanup(func() { audit.SetRecorder(prev) })
	return rec
}

func loginOK(t *testing.T, svc *AuthService, name, pw string) *LoginResponse {
	t.Helper()
	r, err := svc.Login(context.Background(), &LoginRequest{Username: name, PasswordHash: HashPassword(pw), ClientIP: "127.0.0.1"})
	if err != nil {
		t.Fatalf("login %s: %v", name, err)
	}
	return r
}

func TestAuth_LegacyRowUpgradedOnLoginAndAudited(t *testing.T) {
	rec := withAudit(t)
	svc, db := newTestAuthService(t)
	createUser(t, db, "legacy", "Str0ng-Pass#1", "CENTER", "kuscia-system") // raw SHA-256 row
	loginOK(t, svc, "legacy", "Str0ng-Pass#1")
	var u model.UserAccountsDO
	db.Where("name = ?", "legacy").First(&u)
	if auth.StoredPasswordFormat(u.PasswordHash) != auth.FormatKDF {
		t.Fatalf("legacy row not upgraded: %s", u.PasswordHash)
	}
	// Pass-the-hash with the old raw value no longer works.
	if _, err := svc.Login(context.Background(), &LoginRequest{Username: "legacy", PasswordHash: u.PasswordHash}); err == nil {
		t.Fatal("stored value must not be a credential")
	}
	loginOK(t, svc, "legacy", "Str0ng-Pass#1")
	if !rec.has(audit.ActionLogin, audit.ResultSuccess, "") || !rec.has(audit.ActionLogin, audit.ResultFailure, "invalid_credentials") {
		t.Fatalf("login events not audited: %+v", rec.events)
	}
	for _, e := range rec.events {
		if e.Reason == u.PasswordHash || e.Extra != nil {
			t.Fatal("audit events must not carry secrets")
		}
	}
}

func TestAuth_AccountWideLockoutNeedsHigherThreshold(t *testing.T) {
	rec := withAudit(t)
	svc, db := newTestAuthService(t)
	svc.SetOptions(AuthOptions{MaxAttempts: 2, AccountMaxAttempts: 4, LockDuration: 10 * time.Minute})
	createUser(t, db, "admin", "Str0ng-Pass#1", "CENTER", "kuscia-system")
	ctx := context.Background()
	// Two IPs x two failures: each pair locks, and the account reaches 4.
	for _, ip := range []string{"1.1.1.1", "2.2.2.2"} {
		for i := 0; i < 2; i++ {
			_, err := svc.Login(ctx, &LoginRequest{Username: "admin", PasswordHash: HashPassword("bad"), ClientIP: ip})
			if authCode(err) != errcode.AuthLoginFailed.Code {
				t.Fatalf("uniform error expected: %v", err)
			}
		}
	}
	// A third, clean IP is now locked out too (account-wide), but only a
	// caller with the right password learns that.
	_, err := svc.Login(ctx, &LoginRequest{Username: "admin", PasswordHash: HashPassword("bad"), ClientIP: "3.3.3.3"})
	if authCode(err) != errcode.AuthLoginFailed.Code {
		t.Fatalf("wrong password while locked must stay uniform: %v", err)
	}
	_, err = svc.Login(ctx, &LoginRequest{Username: "admin", PasswordHash: HashPassword("Str0ng-Pass#1"), ClientIP: "3.3.3.3"})
	if !errors.Is(err, ErrUserLocked) {
		t.Fatalf("account lock must apply to every IP: %v", err)
	}
	if !rec.has(audit.ActionLockout, audit.ResultFailure, "account_threshold") ||
		!rec.has(audit.ActionLockout, audit.ResultFailure, "account_ip_threshold") {
		t.Fatalf("lockout events missing: %+v", rec.events)
	}
}

func TestAuth_PairLockDoesNotLockOtherIPs(t *testing.T) {
	svc, db := newTestAuthService(t)
	svc.SetOptions(AuthOptions{MaxAttempts: 3, LockDuration: 10 * time.Minute}) // account threshold 12
	createUser(t, db, "admin", "Str0ng-Pass#1", "CENTER", "kuscia-system")
	for i := 0; i < 5; i++ {
		_, _ = svc.Login(context.Background(), &LoginRequest{Username: "admin", PasswordHash: HashPassword("bad"), ClientIP: "6.6.6.6"})
	}
	if _, err := svc.Login(context.Background(), &LoginRequest{Username: "admin", PasswordHash: HashPassword("Str0ng-Pass#1"), ClientIP: "10.0.0.8"}); err != nil {
		t.Fatalf("attacker IP must not lock the admin out elsewhere: %v", err)
	}
}

func TestAuth_RefreshRotationAndReuseDetection(t *testing.T) {
	rec := withAudit(t)
	svc, db := newTestAuthService(t)
	createUser(t, db, "u", "Str0ng-Pass#1", "CENTER", "kuscia-system")
	ctx := context.Background()
	r := loginOK(t, svc, "u", "Str0ng-Pass#1")

	p1, err := svc.RefreshToken(ctx, r.RefreshToken)
	if err != nil {
		t.Fatal(err)
	}
	p2, err := svc.RefreshToken(ctx, p1.RefreshToken)
	if err != nil {
		t.Fatalf("rotated refresh token must work once: %v", err)
	}
	if _, err := svc.ValidateSession(ctx, p2.AccessToken); err != nil {
		t.Fatal(err)
	}
	// Replaying the first (already used) refresh token = theft: session revoked.
	if _, err := svc.RefreshToken(ctx, r.RefreshToken); !errors.Is(err, ErrRefreshTokenReused) {
		t.Fatalf("reuse must be detected: %v", err)
	}
	if _, err := svc.ValidateSession(ctx, p2.AccessToken); err == nil {
		t.Fatal("reuse must revoke the whole session")
	}
	if _, err := svc.RefreshToken(ctx, p2.RefreshToken); err == nil {
		t.Fatal("latest refresh token must be dead after revocation")
	}
	if !rec.has(auditActionRefreshReuse, audit.ResultDenied, "") {
		t.Fatal("reuse must be audited")
	}
	// An access token is not a refresh token.
	r2 := loginOK(t, svc, "u", "Str0ng-Pass#1")
	if _, err := svc.RefreshToken(ctx, r2.AccessToken); err == nil {
		t.Fatal("access token accepted as refresh token")
	}
}

func TestAuth_SessionAbsoluteEnd(t *testing.T) {
	svc, db := newTestAuthService(t)
	createUser(t, db, "u", "Str0ng-Pass#1", "CENTER", "kuscia-system")
	r := loginOK(t, svc, "u", "Str0ng-Pass#1")
	end := time.Now().Add(-time.Second)
	db.Model(&model.UserTokensDO{}).Where("name = ?", "u").Update("gmt_refresh_expire", end)
	if _, err := svc.ValidateSession(context.Background(), r.Token); !errors.Is(err, ErrSessionExpired) {
		t.Fatalf("session past its absolute end must expire: %v", err)
	}
}

func TestAuth_PasswordChangeInvalidatesTokensAndHistory(t *testing.T) {
	rec := withAudit(t)
	svc, db := newTestAuthService(t)
	us, _ := newPwdTestUserServiceDB(db)
	createUser(t, db, "u", "Str0ng-Pass#1", "CENTER", "kuscia-system")
	ctx := context.Background()
	r := loginOK(t, svc, "u", "Str0ng-Pass#1")

	change := func(oldPw, newPw string) error {
		return us.UpdatePassword(ctx, &UpdatePasswordRequest{Name: "u", OldPasswordHash: HashPassword(oldPw),
			NewPasswordHash: HashPassword(newPw), ConfirmPasswordHash: HashPassword(newPw)})
	}
	// Weak new password rejected (after the old one is proven).
	if err := change("Str0ng-Pass#1", "12345678"); authCode(err) != errcode.AuthPasswordTooWeak.Code {
		t.Fatalf("weak password: %v", err)
	}
	if err := change("Str0ng-Pass#1", "Str0ng-Pass#2"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ValidateSession(ctx, r.Token); err == nil {
		t.Fatal("old session must be invalid after password change")
	}
	// Password version bump alone also kills a (hypothetically surviving) session.
	r2 := loginOK(t, svc, "u", "Str0ng-Pass#2")
	db.Model(&model.UserAccountsDO{}).Where("name = ?", "u").Update("password_version", 99)
	if _, err := svc.ValidateSession(ctx, r2.Token); !errors.Is(err, ErrSessionExpired) {
		t.Fatalf("stale pwd_ver must be rejected: %v", err)
	}
	db.Model(&model.UserAccountsDO{}).Where("name = ?", "u").Update("password_version", 1)

	// Reuse of the current or the last passwords is rejected.
	if err := change("Str0ng-Pass#2", "Str0ng-Pass#1"); authCode(err) != errcode.AuthPasswordReused.Code {
		t.Fatalf("reuse of previous password: %v", err)
	}
	for i, pw := range []string{"Str0ng-Pass#3", "Str0ng-Pass#4", "Str0ng-Pass#5", "Str0ng-Pass#6"} {
		prev := []string{"Str0ng-Pass#2", "Str0ng-Pass#3", "Str0ng-Pass#4", "Str0ng-Pass#5"}[i]
		if err := change(prev, pw); err != nil {
			t.Fatalf("change %d: %v", i, err)
		}
	}
	// #1 dropped out of the history (current + 3 previous are remembered).
	if err := change("Str0ng-Pass#6", "Str0ng-Pass#1"); err != nil {
		t.Fatalf("old password beyond the history must be allowed: %v", err)
	}
	if !rec.has(audit.ActionPasswordChange, audit.ResultSuccess, "") || !rec.has(audit.ActionPasswordChange, audit.ResultFailure, "policy_reuse") {
		t.Fatal("password change events missing")
	}
}

func TestAuth_MustChangePasswordFlow(t *testing.T) {
	svc, db := newTestAuthService(t)
	us, _ := newPwdTestUserServiceDB(db)
	dir := t.TempDir()
	file := filepath.Join(dir, "initial-admin-password")
	_ = os.WriteFile(file, []byte("x\n"), 0o600)
	us.SetInitialPasswordFile("admin", file, nil)
	createUser(t, db, "admin", "Str0ng-Pass#1", "CENTER", "kuscia-system")
	ctx := context.Background()

	// Admin reset forces a change.
	if err := us.ResetPassword(ctx, &ResetPasswordRequest{Name: "admin", NewPasswordHash: HashPassword("Reset-Pass#9")}); err != nil {
		t.Fatal(err)
	}
	if err := us.ResetPassword(ctx, &ResetPasswordRequest{Name: "admin", NewPasswordHash: HashPassword("admin123")}); authCode(err) != errcode.AuthPasswordTooWeak.Code {
		t.Fatalf("reset with weak password: %v", err)
	}
	r := loginOK(t, svc, "admin", "Reset-Pass#9")
	if !r.MustChangePassword {
		t.Fatal("login must report mustChangePassword")
	}
	uc, err := svc.ValidateSession(ctx, r.Token)
	if err != nil || !uc.MustChangePassword {
		t.Fatalf("session must carry the flag: %v", err)
	}
	if err := us.UpdatePassword(ctx, &UpdatePasswordRequest{Name: "admin", OldPasswordHash: HashPassword("Reset-Pass#9"),
		NewPasswordHash: HashPassword("Mine-Pass#7"), ConfirmPasswordHash: HashPassword("Mine-Pass#7")}); err != nil {
		t.Fatal(err)
	}
	if r := loginOK(t, svc, "admin", "Mine-Pass#7"); r.MustChangePassword {
		t.Fatal("flag must be cleared after the change")
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatal("initial admin password file must be deleted after the first change")
	}
}

func TestNodeUser_ResetPolicy(t *testing.T) {
	db := setupAuthExtTestDB(t)
	_ = db.AutoMigrate(&model.SysUserNodeRelDO{})
	seedNodeUserTestNodes(db)
	svc := NewNodeUserService(db)
	ctx := context.Background()
	if err := svc.Create(ctx, &NodeUserCreateRequest{NodeIDAlt: "alice", Name: "w", PasswordHash: HashPassword("12345678")}); !errors.Is(err, ErrNodeUserWeakPwd) {
		t.Fatalf("weak node user password: %v", err)
	}
	if err := svc.Create(ctx, &NodeUserCreateRequest{NodeIDAlt: "alice", Name: "n", PasswordHash: HashPassword("Node-Pass#1"), MustChangePassword: true}); err != nil {
		t.Fatal(err)
	}
	var u model.UserAccountsDO
	db.Where("name = ?", "n").First(&u)
	if !u.MustChangePassword || auth.StoredPasswordFormat(u.PasswordHash) != auth.FormatKDF {
		t.Fatalf("node user: %+v", u)
	}
	req := func(oldPw, newPw string) *ResetNodeUserPwdRequest {
		return &ResetNodeUserPwdRequest{NodeID: "alice", Name: "n", PasswordHash: HashPassword(oldPw), NewPasswordHash: HashPassword(newPw)}
	}
	if err := svc.ResetPassword(ctx, req("Node-Pass#1", "Node-Pass#1")); !errors.Is(err, ErrNodeUserReusedPwd) {
		t.Fatalf("reuse: %v", err)
	}
	if err := svc.ResetPassword(ctx, req("Node-Pass#1", "Node-Pass#2")); err != nil {
		t.Fatal(err)
	}
	db.Where("name = ?", "n").First(&u)
	if u.MustChangePassword || u.PasswordVersion != 1 || !storedMatches(u.PasswordHash, "Node-Pass#2") {
		t.Fatalf("after reset: %+v", u)
	}
}
