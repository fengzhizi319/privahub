package service

import (
	"context"
	"testing"
	"time"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/internal/dao/repository"
	"github.com/fengzhizi319/privahub/pkg/auth"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupAuthExtTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	err = db.AutoMigrate(
		&model.UserAccountsDO{},
		&model.UserTokensDO{},
	)
	if err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}
	return db
}

func TestAuthService_VerifyPassword_PlainPassword(t *testing.T) {
	db := setupAuthExtTestDB(t)
	jwtManager := auth.NewJWTManager("test-secret", time.Hour, 24*time.Hour)
	svc := NewAuthService(
		repository.NewUserAccountsRepo(db),
		repository.NewUserTokensRepo(db),
		jwtManager,
	)

	// Test plain password verification (hash-then-compare path)
	storedHash := HashPassword("mypassword")
	if !svc.verifyPassword("mypassword", storedHash) {
		t.Error("verifyPassword should return true for correct plain password")
	}
	if svc.verifyPassword("wrongpassword", storedHash) {
		t.Error("verifyPassword should return false for wrong plain password")
	}
}

func TestAuthService_VerifyPassword_PreHashedPassword(t *testing.T) {
	db := setupAuthExtTestDB(t)
	jwtManager := auth.NewJWTManager("test-secret", time.Hour, 24*time.Hour)
	svc := NewAuthService(
		repository.NewUserAccountsRepo(db),
		repository.NewUserTokensRepo(db),
		jwtManager,
	)

	// Test pre-hashed password verification (direct comparison path)
	// This simulates the Java SecretPad frontend which sends SHA-256 hex
	storedHash := HashPassword("mypassword")
	preHashedInput := HashPassword("mypassword") // Frontend sends this
	if !svc.verifyPassword(preHashedInput, storedHash) {
		t.Error("verifyPassword should return true for matching pre-hashed password")
	}

	wrongPreHashed := HashPassword("wrongpassword")
	if svc.verifyPassword(wrongPreHashed, storedHash) {
		t.Error("verifyPassword should return false for wrong pre-hashed password")
	}
}

func TestAuthService_Login_AccountLockout(t *testing.T) {
	db := setupAuthExtTestDB(t)
	jwtManager := auth.NewJWTManager("test-secret", time.Hour, 24*time.Hour)
	svc := NewAuthService(
		repository.NewUserAccountsRepo(db),
		repository.NewUserTokensRepo(db),
		jwtManager,
	)

	// Create user
	admin := &model.UserAccountsDO{
		Name:         "locktest",
		PasswordHash: HashPassword("correctpass"),
		OwnerType:    "CENTER",
		OwnerID:      "kuscia-system",
	}
	db.Create(admin)

	// Fail 5 times
	for i := 0; i < 5; i++ {
		_, err := svc.Login(context.Background(), &LoginRequest{
			Username: "locktest",
			Password: "wrongpass",
		})
		if err != ErrInvalidPassword {
			t.Errorf("attempt %d: expected ErrInvalidPassword, got %v", i+1, err)
		}
	}

	// 6th attempt should fail with ErrUserLocked
	_, err := svc.Login(context.Background(), &LoginRequest{
		Username: "locktest",
		Password: "correctpass", // Even correct password should fail
	})
	if err != ErrUserLocked {
		t.Errorf("expected ErrUserLocked after 5 failed attempts, got %v", err)
	}
}

func TestAuthService_Login_ResetFailedAttempts(t *testing.T) {
	db := setupAuthExtTestDB(t)
	jwtManager := auth.NewJWTManager("test-secret", time.Hour, 24*time.Hour)
	svc := NewAuthService(
		repository.NewUserAccountsRepo(db),
		repository.NewUserTokensRepo(db),
		jwtManager,
	)

	// Create user
	admin := &model.UserAccountsDO{
		Name:         "resettest",
		PasswordHash: HashPassword("correctpass"),
		OwnerType:    "CENTER",
		OwnerID:      "kuscia-system",
	}
	db.Create(admin)

	// Fail 3 times
	for i := 0; i < 3; i++ {
		svc.Login(context.Background(), &LoginRequest{
			Username: "resettest",
			Password: "wrongpass",
		})
	}

	// Successful login should reset counter
	_, err := svc.Login(context.Background(), &LoginRequest{
		Username: "resettest",
		Password: "correctpass",
	})
	if err != nil {
		t.Fatalf("Login should succeed: %v", err)
	}

	// Verify counter was reset
	var user model.UserAccountsDO
	db.Where("name = ?", "resettest").First(&user)
	if user.FailedAttempts != nil && *user.FailedAttempts != 0 {
		t.Errorf("expected failed attempts to be reset to 0, got %d", *user.FailedAttempts)
	}
}

func TestAuthService_Login_PasswordHashField(t *testing.T) {
	db := setupAuthExtTestDB(t)
	jwtManager := auth.NewJWTManager("test-secret", time.Hour, 24*time.Hour)
	svc := NewAuthService(
		repository.NewUserAccountsRepo(db),
		repository.NewUserTokensRepo(db),
		jwtManager,
	)

	// Create user
	admin := &model.UserAccountsDO{
		Name:         "hashtest",
		PasswordHash: HashPassword("mypassword"),
		OwnerType:    "CENTER",
		OwnerID:      "kuscia-system",
	}
	db.Create(admin)

	// Login using PasswordHash field (Java SecretPad contract)
	resp, err := svc.Login(context.Background(), &LoginRequest{
		Username:     "hashtest",
		PasswordHash: HashPassword("mypassword"), // Frontend sends pre-hashed
	})
	if err != nil {
		t.Fatalf("Login with PasswordHash failed: %v", err)
	}
	if resp.AccessToken == "" {
		t.Error("expected non-empty access token")
	}
}

func TestAuthService_Logout(t *testing.T) {
	db := setupAuthExtTestDB(t)
	jwtManager := auth.NewJWTManager("test-secret", time.Hour, 24*time.Hour)
	svc := NewAuthService(
		repository.NewUserAccountsRepo(db),
		repository.NewUserTokensRepo(db),
		jwtManager,
	)

	// Create user and login
	admin := &model.UserAccountsDO{
		Name:         "logouttest",
		PasswordHash: HashPassword("pass"),
		OwnerType:    "CENTER",
		OwnerID:      "kuscia-system",
	}
	db.Create(admin)

	loginResp, _ := svc.Login(context.Background(), &LoginRequest{
		Username: "logouttest",
		Password: "pass",
	})

	// Verify token exists
	_, err := svc.ValidateSession(context.Background(), loginResp.AccessToken)
	if err != nil {
		t.Fatalf("ValidateSession failed: %v", err)
	}

	// Logout
	err = svc.Logout(context.Background(), "logouttest")
	if err != nil {
		t.Fatalf("Logout failed: %v", err)
	}

	// Verify token is deleted
	_, err = svc.ValidateSession(context.Background(), loginResp.AccessToken)
	if err == nil {
		t.Error("expected error after logout (token should be deleted)")
	}
}

func TestAuthService_RefreshToken(t *testing.T) {
	db := setupAuthExtTestDB(t)
	jwtManager := auth.NewJWTManager("test-secret", time.Hour, 24*time.Hour)
	svc := NewAuthService(
		repository.NewUserAccountsRepo(db),
		repository.NewUserTokensRepo(db),
		jwtManager,
	)

	// Create user and login
	admin := &model.UserAccountsDO{
		Name:         "refreshtest",
		PasswordHash: HashPassword("pass"),
		OwnerType:    "CENTER",
		OwnerID:      "kuscia-system",
	}
	db.Create(admin)

	loginResp, _ := svc.Login(context.Background(), &LoginRequest{
		Username: "refreshtest",
		Password: "pass",
	})

	// Refresh token
	newPair, err := svc.RefreshToken(context.Background(), loginResp.RefreshToken)
	if err != nil {
		t.Fatalf("RefreshToken failed: %v", err)
	}
	if newPair.AccessToken == "" {
		t.Error("expected non-empty new access token")
	}
	if newPair.AccessToken == loginResp.AccessToken {
		t.Error("new access token should be different from old one")
	}
}

func TestHashPassword_Deterministic(t *testing.T) {
	// Test deterministic hashing
	hash1 := HashPassword("test123")
	hash2 := HashPassword("test123")
	if hash1 != hash2 {
		t.Error("HashPassword should be deterministic")
	}

	// Test different passwords produce different hashes
	hash3 := HashPassword("different")
	if hash1 == hash3 {
		t.Error("different passwords should produce different hashes")
	}

	// Test hash length (SHA-256 produces 64 hex characters)
	if len(hash1) != 64 {
		t.Errorf("expected hash length 64, got %d", len(hash1))
	}
}
