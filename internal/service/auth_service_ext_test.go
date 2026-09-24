package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/internal/dao/repository"
	"github.com/fengzhizi319/privahub/pkg/auth"
	"github.com/fengzhizi319/privahub/pkg/errcode"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupAuthExtTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	err = db.AutoMigrate(
		&model.UserAccountsDO{},
		&model.UserTokensDO{},
		&model.ProjectNodeDO{},
		&model.InstDO{},
		&model.NodeDO{},
		&model.SysResourceDO{},
		&model.SysRoleResourceRelDO{},
		&model.SysUserPermissionRelDO{},
	)
	if err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}
	return db
}

func newTestAuthService(t *testing.T) (*AuthService, *gorm.DB) {
	t.Helper()
	db := setupAuthExtTestDB(t)
	jwtManager := auth.NewJWTManager("test-secret", time.Hour, 24*time.Hour)
	svc := NewAuthService(repository.NewUserAccountsRepo(db), repository.NewUserTokensRepo(db), jwtManager)
	env := NewEnvService("master", "kuscia-system", "", nil)
	svc.SetEnv(env)
	return svc, db
}

func createUser(t *testing.T, db *gorm.DB, name, password, ownerType, ownerID string) {
	t.Helper()
	if err := db.Create(&model.UserAccountsDO{
		Name: name, PasswordHash: HashPassword(password), OwnerType: ownerType, OwnerID: ownerID,
	}).Error; err != nil {
		t.Fatal(err)
	}
}

func authCode(err error) int {
	var ae *AuthError
	if errors.As(err, &ae) && ae.Code != nil {
		return ae.Code.Code
	}
	return -1
}

func TestAuthService_VerifyPassword_PlaintextOnlyInDev(t *testing.T) {
	svc, _ := newTestAuthService(t)
	stored := HashPassword("mypassword")
	if svc.verifyPassword("mypassword", stored) {
		t.Error("plaintext password must be rejected outside dev")
	}
	if !svc.verifyPassword(HashPassword("mypassword"), stored) {
		t.Error("SHA-256 hex must be accepted")
	}
	svc.SetOptions(AuthOptions{AllowPlaintextPassword: true})
	if !svc.verifyPassword("mypassword", stored) {
		t.Error("plaintext should be accepted in dev")
	}
	if svc.verifyPassword("wrong", stored) {
		t.Error("wrong password accepted")
	}
}

func TestAuthService_Login_UserContextDTO(t *testing.T) {
	svc, db := newTestAuthService(t)
	createUser(t, db, "admin", "12345678", "CENTER", "kuscia-system")
	resp, err := svc.Login(context.Background(), &LoginRequest{Username: "admin", PasswordHash: HashPassword("12345678")})
	if err != nil {
		t.Fatalf("login failed: %v", err)
	}
	if resp.Token == "" || resp.Token != resp.AccessToken {
		t.Error("token must equal access_token")
	}
	if resp.PlatformType != "CENTER" || resp.PlatformNodeID != "kuscia-system" {
		t.Errorf("platform fields wrong: %+v", resp.UserContext)
	}
	if resp.OwnerType != "CENTER" || resp.OwnerID != "kuscia-system" || resp.DeployMode == "" {
		t.Errorf("owner fields wrong: %+v", resp.UserContext)
	}
	if resp.ProjectIDs == nil || resp.APIResources == nil {
		t.Error("projectIds/apiResources must be non-nil arrays")
	}
	// Session row stores only the token digest and a token-free snapshot.
	var row model.UserTokensDO
	if err := db.Where("token = ?", auth.TokenDigest(resp.Token)).First(&row).Error; err != nil {
		t.Fatalf("session row not stored: %v", err)
	}
	if row.SessionData == "" || row.SessionID == "" || row.RefreshJTI == "" {
		t.Error("session data/id/refresh jti missing")
	}
	if strings.Contains(row.SessionData, resp.Token) || strings.Contains(row.Token, ".") {
		t.Error("raw token must not be persisted")
	}
}

func TestAuthService_Login_EdgeUserContext(t *testing.T) {
	svc, db := newTestAuthService(t)
	createUser(t, db, "edgeu", "pw", "EDGE", "alice")
	db.Create(&model.ProjectNodeDO{ProjectID: "p1", NodeID: "alice"})
	db.Create(&model.SysResourceDO{ResourceType: "API", ResourceCode: "PRJ_LIST", ResourceName: "PRJ_LIST"})
	db.Create(&model.SysRoleResourceRelDO{RoleCode: "EDGE_USER", ResourceCode: "PRJ_LIST"})
	db.Create(&model.SysUserPermissionRelDO{UserType: "EDGE_USER", UserKey: "edgeu", TargetType: "ROLE", TargetCode: "EDGE_USER"})
	db.Create(&model.NodeDO{NodeID: "alice", Name: "Alice Node", ControlNodeID: "alice"})

	resp, err := svc.Login(context.Background(), &LoginRequest{Username: "edgeu", PasswordHash: HashPassword("pw")})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.ProjectIDs) != 1 || resp.ProjectIDs[0] != "p1" {
		t.Errorf("projectIds = %v", resp.ProjectIDs)
	}
	if len(resp.APIResources) != 1 || resp.APIResources[0] != "PRJ_LIST" {
		t.Errorf("apiResources = %v", resp.APIResources)
	}
	if resp.OwnerName != "Alice Node" {
		t.Errorf("ownerName = %q", resp.OwnerName)
	}
}

func TestAuthService_Login_PasswordFieldIsHashAlias(t *testing.T) {
	svc, db := newTestAuthService(t)
	createUser(t, db, "u", "pw", "CENTER", "kuscia-system")
	// The v1alpha1 frontend route sends the SHA-256 hex in `password`.
	if _, err := svc.Login(context.Background(), &LoginRequest{Username: "u", Password: HashPassword("pw")}); err != nil {
		t.Fatalf("hash in password field should work: %v", err)
	}
	// Plaintext is rejected outside dev.
	if _, err := svc.Login(context.Background(), &LoginRequest{Username: "u", Password: "pw"}); err == nil {
		t.Fatal("plaintext login must fail outside dev")
	}
}

func TestAuthService_Login_LegacySm3UpgradesHash(t *testing.T) {
	svc, db := newTestAuthService(t)
	const sm3abc = "66c7f0f462eeedd9d1f2d46bdc10e4e24167c4875cf2f7a2297da02b8f4ba8e0" // SM3("abc")
	db.Create(&model.UserAccountsDO{Name: "legacy", PasswordHash: sm3abc, OwnerType: "CENTER", OwnerID: "kuscia-system"})

	// Without the SM3 field the SHA-256 hash does not match.
	if _, err := svc.Login(context.Background(), &LoginRequest{Username: "legacy", PasswordHash: HashPassword("abc")}); err == nil {
		t.Fatal("expected failure without passwordHashSm3")
	}
	if _, err := svc.Login(context.Background(), &LoginRequest{
		Username: "legacy", PasswordHash: HashPassword("abc"), PasswordHashSm3: sm3abc,
	}); err != nil {
		t.Fatalf("legacy login failed: %v", err)
	}
	var u model.UserAccountsDO
	db.Where("name = ?", "legacy").First(&u)
	if auth.StoredPasswordFormat(u.PasswordHash) != auth.FormatKDF ||
		!auth.VerifyStoredPassword(u.PasswordHash, auth.PasswordCandidates{SHA256: HashPassword("abc")}).OK {
		t.Errorf("hash not upgraded to the SHA-256 KDF form: %s", u.PasswordHash)
	}
	// Subsequent login with only SHA-256 works.
	if _, err := svc.Login(context.Background(), &LoginRequest{Username: "legacy", PasswordHash: HashPassword("abc")}); err != nil {
		t.Fatalf("login after upgrade failed: %v", err)
	}
}

func TestAuthService_Login_AccountLockoutAndExpiry(t *testing.T) {
	svc, db := newTestAuthService(t)
	svc.SetOptions(AuthOptions{MaxAttempts: 3, LockDuration: 10 * time.Minute})
	createUser(t, db, "locktest", "correctpass", "CENTER", "kuscia-system")
	ctx := context.Background()
	wrong := &LoginRequest{Username: "locktest", PasswordHash: HashPassword("wrong"), ClientIP: "10.0.0.1"}

	// Every failure — including the one that locks — is the uniform error.
	for i := 0; i < 4; i++ {
		_, err := svc.Login(ctx, wrong)
		if !errors.Is(err, ErrInvalidPassword) || authCode(err) != errcode.AuthLoginFailed.Code {
			t.Fatalf("attempt %d: %v", i+1, err)
		}
	}
	// "Locked" is only revealed to the caller that knows the password.
	good := &LoginRequest{Username: "locktest", PasswordHash: HashPassword("correctpass"), ClientIP: "10.0.0.1"}
	if _, err := svc.Login(ctx, good); !errors.Is(err, ErrUserLocked) || authCode(err) != errcode.AuthUserIsLocked.Code {
		t.Fatalf("locked pair must reject correct password with USER_IS_LOCKED: %v", err)
	}
	// Another client IP is not affected by this pair lock.
	other := *good
	other.ClientIP = "10.0.0.2"
	if _, err := svc.Login(ctx, &other); err != nil {
		t.Fatalf("other IP must still log in: %v", err)
	}

	// After lock expiry the counter resets and login works.
	svc.now = func() time.Time { return time.Now().Add(11 * time.Minute) }
	if _, err := svc.Login(ctx, good); err != nil {
		t.Fatalf("login after lock expiry failed: %v", err)
	}
	var u model.UserAccountsDO
	db.Where("name = ?", "locktest").First(&u)
	if u.FailedAttempts != nil || u.LockedInvalidTime != nil {
		t.Error("counters not reset")
	}
}

func TestAuthService_Login_UnknownUserUniformAndCounted(t *testing.T) {
	svc, _ := newTestAuthService(t)
	svc.SetOptions(AuthOptions{MaxAttempts: 2, LockDuration: time.Minute})
	ctx := context.Background()
	req := &LoginRequest{Username: "ghost", PasswordHash: HashPassword("x"), ClientIP: "1.1.1.1"}
	for i := 0; i < 4; i++ {
		_, err := svc.Login(ctx, req)
		if authCode(err) != errcode.AuthLoginFailed.Code {
			t.Fatalf("unknown user must always get the uniform code (attempt %d): %v", i+1, err)
		}
	}
	if _, ok := svc.throttle.lockedUntil(throttleKey("ghost", "1.1.1.1"), time.Now()); !ok {
		t.Fatal("unknown user pair must be counted/locked like a real account")
	}
	svc.now = func() time.Time { return time.Now().Add(2 * time.Minute) }
	if _, ok := svc.throttle.lockedUntil(throttleKey("ghost", "1.1.1.1"), svc.now()); ok {
		t.Fatal("lock should expire")
	}
}

func TestAuthService_SessionSlidingExpiryAndLogout(t *testing.T) {
	svc, db := newTestAuthService(t)
	svc.SetOptions(AuthOptions{SessionIdleTimeout: time.Hour})
	createUser(t, db, "sess", "pw", "CENTER", "kuscia-system")
	ctx := context.Background()
	r1, err := svc.Login(ctx, &LoginRequest{Username: "sess", PasswordHash: HashPassword("pw")})
	if err != nil {
		t.Fatal(err)
	}
	r2, _ := svc.Login(ctx, &LoginRequest{Username: "sess", PasswordHash: HashPassword("pw")})

	uc, err := svc.ValidateSession(ctx, r1.Token)
	if err != nil || uc.Name != "sess" {
		t.Fatalf("validate failed: %v", err)
	}

	// Slide: 50 minutes later the session is used and refreshed.
	base := time.Now()
	svc.now = func() time.Time { return base.Add(50 * time.Minute) }
	if _, err := svc.ValidateSession(ctx, r1.Token); err != nil {
		t.Fatalf("session should still be valid: %v", err)
	}
	// 100 minutes after login but only 50 after the last use -> still valid.
	svc.now = func() time.Time { return base.Add(100 * time.Minute) }
	if _, err := svc.ValidateSession(ctx, r1.Token); err != nil {
		t.Fatalf("sliding expiry not applied: %v", err)
	}
	// r2 was never used for 100 minutes -> expired.
	if _, err := svc.ValidateSession(ctx, r2.Token); !errors.Is(err, ErrSessionExpired) {
		t.Fatalf("idle session should expire: %v", err)
	}

	// Logout deletes only the current token.
	r3, _ := svc.Login(ctx, &LoginRequest{Username: "sess", PasswordHash: HashPassword("pw")})
	if err := svc.Logout(ctx, "sess", r1.Token); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ValidateSession(ctx, r1.Token); err == nil {
		t.Error("logged out token must be rejected")
	}
	if _, err := svc.ValidateSession(ctx, r3.Token); err != nil {
		t.Errorf("other sessions must survive logout: %v", err)
	}
}

func TestAuthService_RefreshToken(t *testing.T) {
	svc, db := newTestAuthService(t)
	createUser(t, db, "refreshtest", "pass", "CENTER", "kuscia-system")
	ctx := context.Background()
	loginResp, err := svc.Login(ctx, &LoginRequest{Username: "refreshtest", PasswordHash: HashPassword("pass")})
	if err != nil {
		t.Fatal(err)
	}
	newPair, err := svc.RefreshToken(ctx, loginResp.RefreshToken)
	if err != nil {
		t.Fatalf("RefreshToken failed: %v", err)
	}
	if newPair.AccessToken == "" || newPair.AccessToken == loginResp.AccessToken {
		t.Error("expected a new access token")
	}
	if _, err := svc.ValidateSession(ctx, newPair.AccessToken); err != nil {
		t.Errorf("refreshed token should be a valid session: %v", err)
	}
	if _, err := svc.ValidateSession(ctx, loginResp.AccessToken); err == nil {
		t.Error("old access token should be replaced")
	}
	// After logout the refresh token is useless.
	_ = svc.Logout(ctx, "refreshtest", newPair.AccessToken)
	if _, err := svc.RefreshToken(ctx, newPair.RefreshToken); err == nil {
		t.Error("refresh after logout must fail")
	}
}

func TestAuthService_NodeVirtualUser(t *testing.T) {
	svc, db := newTestAuthService(t)
	db.Create(&model.ProjectNodeDO{ProjectID: "p9", NodeID: "bob"})
	db.Create(&model.SysResourceDO{ResourceType: "API", ResourceCode: "NODE_USER_RESET_PWD", ResourceName: "x"})
	db.Create(&model.SysRoleResourceRelDO{RoleCode: "EDGE_NODE", ResourceCode: "NODE_USER_RESET_PWD"})
	db.Create(&model.SysUserPermissionRelDO{UserType: "NODE", UserKey: "bob", TargetType: "ROLE", TargetCode: "EDGE_NODE"})
	u := svc.BuildNodeVirtualUser(context.Background(), "bob")
	if !u.IsVirtualNode() || u.OwnerID != "bob" || u.OwnerType != "EDGE" || u.PlatformType != "EDGE" {
		t.Errorf("unexpected virtual user %+v", u)
	}
	if !u.ContainProjectID("p9") || !u.ContainInterfaceResource("NODE_USER_RESET_PWD") {
		t.Errorf("virtual user permissions missing: %+v", u)
	}
}

func TestHashPassword_Deterministic(t *testing.T) {
	hash1 := HashPassword("test123")
	if hash1 != HashPassword("test123") || hash1 == HashPassword("different") || len(hash1) != 64 {
		t.Error("HashPassword must be deterministic SHA-256 hex")
	}
}
