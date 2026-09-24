package service

import (
	"context"
	"testing"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/internal/dao/repository"
	"github.com/fengzhizi319/privahub/pkg/auth"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	err = db.AutoMigrate(
		&model.UserAccountsDO{},
		&model.UserTokensDO{},
		&model.ProjectJobDO{},
		&model.ProjectJobTaskDO{},
		&model.ProjectJobTaskLogDO{},
	)
	if err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}
	return db
}

func TestAuthService_Login_Success(t *testing.T) {
	db := setupTestDB(t)

	admin := &model.UserAccountsDO{
		Name:         "admin",
		PasswordHash: HashPassword("12345678"),
		OwnerType:    "CENTER",
		OwnerID:      "kuscia-system",
	}
	if err := db.Create(admin).Error; err != nil {
		t.Fatalf("failed to seed admin: %v", err)
	}

	jwtManager := auth.NewJWTManager("test-secret", 3600_000_000_000, 86400_000_000_000)
	svc := NewAuthService(
		repository.NewUserAccountsRepo(db),
		repository.NewUserTokensRepo(db),
		jwtManager,
	)

	// Clients send SHA-256(password); plaintext is only accepted in dev profiles.
	resp, err := svc.Login(context.Background(), &LoginRequest{
		Username:     "admin",
		PasswordHash: HashPassword("12345678"),
	})
	if err != nil {
		t.Fatalf("Login failed: %v", err)
	}
	if resp.AccessToken == "" {
		t.Error("expected non-empty access token")
	}
	if resp.User.Name != "admin" {
		t.Errorf("expected user name 'admin', got %q", resp.User.Name)
	}
}

func TestAuthService_Login_WrongPassword(t *testing.T) {
	db := setupTestDB(t)

	admin := &model.UserAccountsDO{
		Name:         "admin",
		PasswordHash: HashPassword("12345678"),
		OwnerType:    "CENTER",
		OwnerID:      "kuscia-system",
	}
	if err := db.Create(admin).Error; err != nil {
		t.Fatalf("failed to seed admin: %v", err)
	}

	jwtManager := auth.NewJWTManager("test-secret", 3600_000_000_000, 86400_000_000_000)
	svc := NewAuthService(
		repository.NewUserAccountsRepo(db),
		repository.NewUserTokensRepo(db),
		jwtManager,
	)

	_, err := svc.Login(context.Background(), &LoginRequest{
		Username: "admin",
		Password: "wrong-password",
	})
	if err == nil {
		t.Fatal("expected error for wrong password")
	}
}
