package service

import (
	"context"
	"testing"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/internal/dao/repository"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupUserTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	err = db.AutoMigrate(
		&model.UserAccountsDO{},
		&model.SysUserPermissionRelDO{},
		&model.SysUserNodeRelDO{},
	)
	if err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}
	return db
}

func TestUserService_CreateUser_Success(t *testing.T) {
	db := setupUserTestDB(t)
	svc := NewUserService(
		repository.NewUserAccountsRepo(db),
		repository.NewSysUserPermissionRepo(db),
		repository.NewSysUserNodeRepo(db),
		db,
	)

	user, err := svc.CreateUser(context.Background(), &CreateUserRequest{
		Name:      "testuser",
		Password:  "password123",
		OwnerType: "CENTER",
		OwnerID:   "kuscia-system",
		RoleCodes: []string{"ADMIN"},
		NodeIDs:   []string{"alice", "bob"},
	})
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}
	if user.Name != "testuser" {
		t.Errorf("expected name 'testuser', got %q", user.Name)
	}
	if user.OwnerType != "CENTER" {
		t.Errorf("expected owner_type 'CENTER', got %q", user.OwnerType)
	}

	// Verify password was hashed
	var dbUser model.UserAccountsDO
	db.Where("name = ?", "testuser").First(&dbUser)
	if dbUser.PasswordHash == "password123" {
		t.Error("password should be hashed, not stored in plain text")
	}
	if dbUser.PasswordHash != HashPassword("password123") {
		t.Error("password hash mismatch")
	}
}

func TestUserService_CreateUser_DuplicateUser(t *testing.T) {
	db := setupUserTestDB(t)
	svc := NewUserService(
		repository.NewUserAccountsRepo(db),
		repository.NewSysUserPermissionRepo(db),
		repository.NewSysUserNodeRepo(db),
		db,
	)

	// Create first user
	svc.CreateUser(context.Background(), &CreateUserRequest{
		Name:     "dupuser",
		Password: "pass1",
	})

	// Try to create duplicate
	_, err := svc.CreateUser(context.Background(), &CreateUserRequest{
		Name:     "dupuser",
		Password: "pass2",
	})
	if err != ErrUserAlreadyExists {
		t.Errorf("expected ErrUserAlreadyExists, got %v", err)
	}
}

func TestUserService_ListUsers(t *testing.T) {
	db := setupUserTestDB(t)
	svc := NewUserService(
		repository.NewUserAccountsRepo(db),
		repository.NewSysUserPermissionRepo(db),
		repository.NewSysUserNodeRepo(db),
		db,
	)

	// Create users
	svc.CreateUser(context.Background(), &CreateUserRequest{Name: "user1", Password: "pass1"})
	svc.CreateUser(context.Background(), &CreateUserRequest{Name: "user2", Password: "pass2"})
	svc.CreateUser(context.Background(), &CreateUserRequest{Name: "user3", Password: "pass3"})

	resp, err := svc.ListUsers(context.Background())
	if err != nil {
		t.Fatalf("ListUsers failed: %v", err)
	}
	if resp.Total != 3 {
		t.Errorf("expected 3 users, got %d", resp.Total)
	}
	if len(resp.Users) != 3 {
		t.Errorf("expected 3 users in list, got %d", len(resp.Users))
	}
}

func TestUserService_ListUsers_Multiple(t *testing.T) {
	db := setupUserTestDB(t)
	svc := NewUserService(
		repository.NewUserAccountsRepo(db),
		repository.NewSysUserPermissionRepo(db),
		repository.NewSysUserNodeRepo(db),
		db,
	)

	// Create 5 users
	for i := 1; i <= 5; i++ {
		svc.CreateUser(context.Background(), &CreateUserRequest{
			Name:     "user" + string(rune('0'+i)),
			Password: "pass",
		})
	}

	resp, err := svc.ListUsers(context.Background())
	if err != nil {
		t.Fatalf("ListUsers failed: %v", err)
	}
	if resp.Total != 5 {
		t.Errorf("expected total 5, got %d", resp.Total)
	}
	if len(resp.Users) != 5 {
		t.Errorf("expected 5 users, got %d", len(resp.Users))
	}
}

func TestUserService_GetUser_Success(t *testing.T) {
	db := setupUserTestDB(t)
	svc := NewUserService(
		repository.NewUserAccountsRepo(db),
		repository.NewSysUserPermissionRepo(db),
		repository.NewSysUserNodeRepo(db),
		db,
	)

	created, _ := svc.CreateUser(context.Background(), &CreateUserRequest{
		Name:      "getuser",
		Password:  "pass",
		OwnerType: "EDGE",
		OwnerID:   "node-1",
		RoleCodes: []string{"USER"},
		NodeIDs:   []string{"alice"},
	})

	user, err := svc.GetUser(context.Background(), created.Name)
	if err != nil {
		t.Fatalf("GetUser failed: %v", err)
	}
	if user.Name != "getuser" {
		t.Errorf("expected name 'getuser', got %q", user.Name)
	}
	if user.OwnerType != "EDGE" {
		t.Errorf("expected owner_type 'EDGE', got %q", user.OwnerType)
	}
}

func TestUserService_GetUser_NotFound(t *testing.T) {
	db := setupUserTestDB(t)
	svc := NewUserService(
		repository.NewUserAccountsRepo(db),
		repository.NewSysUserPermissionRepo(db),
		repository.NewSysUserNodeRepo(db),
		db,
	)

	_, err := svc.GetUser(context.Background(), "nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent user")
	}
}

func TestUserService_ResetPassword_Success(t *testing.T) {
	db := setupUserTestDB(t)
	svc := NewUserService(
		repository.NewUserAccountsRepo(db),
		repository.NewSysUserPermissionRepo(db),
		repository.NewSysUserNodeRepo(db),
		db,
	)

	svc.CreateUser(context.Background(), &CreateUserRequest{
		Name:     "resetuser",
		Password: "oldpass",
	})

	err := svc.ResetPassword(context.Background(), &ResetPasswordRequest{
		Name:        "resetuser",
		NewPassword: "newpass123",
	})
	if err != nil {
		t.Fatalf("ResetPassword failed: %v", err)
	}

	// Verify new password hash
	var dbUser model.UserAccountsDO
	db.Where("name = ?", "resetuser").First(&dbUser)
	if dbUser.PasswordHash != HashPassword("newpass123") {
		t.Error("password hash not updated correctly")
	}
}

func TestUserService_DeleteUser_Success(t *testing.T) {
	db := setupUserTestDB(t)
	svc := NewUserService(
		repository.NewUserAccountsRepo(db),
		repository.NewSysUserPermissionRepo(db),
		repository.NewSysUserNodeRepo(db),
		db,
	)

	svc.CreateUser(context.Background(), &CreateUserRequest{
		Name:      "deleteuser",
		Password:  "pass",
		RoleCodes: []string{"ADMIN"},
		NodeIDs:   []string{"alice"},
	})

	err := svc.DeleteUser(context.Background(), "deleteuser")
	if err != nil {
		t.Fatalf("DeleteUser failed: %v", err)
	}

	// Verify user is deleted
	var count int64
	db.Model(&model.UserAccountsDO{}).Where("name = ?", "deleteuser").Count(&count)
	if count != 0 {
		t.Error("user should be deleted")
	}

	// Verify permissions are deleted
	db.Model(&model.SysUserPermissionRelDO{}).Where("user_key = ?", "deleteuser").Count(&count)
	if count != 0 {
		t.Error("user permissions should be deleted")
	}

	// Verify node relations are deleted
	db.Model(&model.SysUserNodeRelDO{}).Where("user_id = ?", "deleteuser").Count(&count)
	if count != 0 {
		t.Error("user node relations should be deleted")
	}
}

func TestUserService_UpdateUser_Success(t *testing.T) {
	db := setupUserTestDB(t)
	svc := NewUserService(
		repository.NewUserAccountsRepo(db),
		repository.NewSysUserPermissionRepo(db),
		repository.NewSysUserNodeRepo(db),
		db,
	)

	svc.CreateUser(context.Background(), &CreateUserRequest{
		Name:      "updateuser",
		Password:  "pass",
		OwnerType: "CENTER",
		OwnerID:   "old-owner",
		RoleCodes: []string{"USER"},
	})

	err := svc.UpdateUser(context.Background(), &UpdateUserRequest{
		Name:      "updateuser",
		OwnerType: "EDGE",
		OwnerID:   "new-owner",
		RoleCodes: []string{"ADMIN", "USER"},
		NodeIDs:   []string{"bob"},
	})
	if err != nil {
		t.Fatalf("UpdateUser failed: %v", err)
	}

	// Verify update
	user, _ := svc.GetUser(context.Background(), "updateuser")
	if user.OwnerType != "EDGE" {
		t.Errorf("expected owner_type 'EDGE', got %q", user.OwnerType)
	}
	if user.OwnerID != "new-owner" {
		t.Errorf("expected owner_id 'new-owner', got %q", user.OwnerID)
	}
}
